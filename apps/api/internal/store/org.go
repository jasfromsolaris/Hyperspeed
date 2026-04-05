package store

import (
	"context"
	"database/sql"
	"errors"
	"hash/fnv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// ErrSelfHostOrgAlreadyExists is returned when a second organization cannot be created (one organization per database).
var ErrSelfHostOrgAlreadyExists = errors.New("self host organization already exists")

func singleOrgAdvisoryLockKey() int64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte("hyperspeed:single_org"))
	return int64(h.Sum64())
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

// IsUniqueViolation reports PostgreSQL unique_violation (23505).
func IsUniqueViolation(err error) bool {
	return isUniqueViolation(err)
}

type MemberRole string

const (
	RoleAdmin  MemberRole = "admin"
	RoleMember MemberRole = "member"
)

type Organization struct {
	ID                   uuid.UUID `json:"id"`
	Name                 string    `json:"name"`
	Slug                 string    `json:"slug"`
	DatasetsEnabled      bool      `json:"datasets_enabled"`
	OpenSignupsEnabled   bool      `json:"open_signups_enabled"`
	CreatedAt            time.Time `json:"created_at"`
	IntendedPublicURL     *string `json:"intended_public_url,omitempty"`
	GiftedSubdomainSlug   *string `json:"gifted_subdomain_slug,omitempty"`
	PublicOriginOverride  *string `json:"public_origin_override,omitempty"`
}

type OrgFeatures struct {
	DatasetsEnabled    bool `json:"datasets_enabled"`
	OpenSignupsEnabled bool `json:"open_signups_enabled"`
}

type OrgMember struct {
	OrganizationID uuid.UUID  `json:"organization_id"`
	UserID         uuid.UUID  `json:"user_id"`
	Role           MemberRole `json:"role"`
}

type OrgMemberWithUser struct {
	OrganizationID   uuid.UUID  `json:"organization_id"`
	UserID           uuid.UUID  `json:"user_id"`
	Role             MemberRole `json:"role"`
	Email            string     `json:"email"`
	DisplayName      *string    `json:"display_name,omitempty"`
	LastSeenAt       time.Time  `json:"last_seen_at"`
	IsServiceAccount bool       `json:"is_service_account"`
	// Populated when IsServiceAccount (LEFT JOIN service_accounts).
	ServiceAccountProvider *string `json:"service_account_provider,omitempty"`
	OpenRouterModel        *string `json:"openrouter_model,omitempty"`
	CursorDefaultRepoURL   *string `json:"cursor_default_repo_url,omitempty"`
}

// CountOrganizations returns the total number of organization rows in the database.
func (s *Store) CountOrganizations(ctx context.Context) (int64, error) {
	var n int64
	err := s.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM organizations`).Scan(&n)
	return n, err
}

// CreateOrganizationSelfHostedOne creates the singleton organization for this deployment in one transaction
// (advisory lock + global count check + org + member + system roles). Caller retries with a new slug on unique violations.
func (s *Store) CreateOrganizationSelfHostedOne(ctx context.Context, name, slug string, userID uuid.UUID) (Organization, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return Organization{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, singleOrgAdvisoryLockKey()); err != nil {
		return Organization{}, err
	}

	var n int64
	if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM organizations`).Scan(&n); err != nil {
		return Organization{}, err
	}
	if n >= 1 {
		return Organization{}, ErrSelfHostOrgAlreadyExists
	}

	var o Organization
	var ipu, gss, poo sql.NullString
	err = tx.QueryRow(ctx, `
		INSERT INTO organizations (name, slug) VALUES ($1, $2)
		RETURNING id, name, slug, datasets_enabled, open_signups_enabled, created_at,
			intended_public_url, gifted_subdomain_slug, public_origin_override
	`, name, slug).Scan(&o.ID, &o.Name, &o.Slug, &o.DatasetsEnabled, &o.OpenSignupsEnabled, &o.CreatedAt, &ipu, &gss, &poo)
	if err != nil {
		return Organization{}, err
	}
	o.IntendedPublicURL = nullStrPtr(ipu)
	o.GiftedSubdomainSlug = nullStrPtr(gss)
	o.PublicOriginOverride = nullStrPtr(poo)

	if _, err := tx.Exec(ctx, `
		INSERT INTO organization_members (organization_id, user_id, role)
		VALUES ($1, $2, $3)
		ON CONFLICT (organization_id, user_id) DO UPDATE SET role = EXCLUDED.role
	`, o.ID, userID, RoleAdmin); err != nil {
		return Organization{}, err
	}

	if err := s.ensureSystemRolesTx(ctx, tx, o.ID); err != nil {
		return Organization{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return Organization{}, err
	}
	return o, nil
}

func (s *Store) AddMember(ctx context.Context, orgID, userID uuid.UUID, role MemberRole) error {
	_, err := s.Pool.Exec(ctx, `
		INSERT INTO organization_members (organization_id, user_id, role)
		VALUES ($1, $2, $3)
		ON CONFLICT (organization_id, user_id) DO UPDATE SET role = EXCLUDED.role
	`, orgID, userID, role)
	return err
}

func (s *Store) ListOrganizationsForUser(ctx context.Context, userID uuid.UUID) ([]Organization, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT o.id, o.name, o.slug, o.datasets_enabled, o.open_signups_enabled, o.created_at,
			o.intended_public_url, o.gifted_subdomain_slug, o.public_origin_override
		FROM organizations o
		JOIN organization_members m ON m.organization_id = o.id
		WHERE m.user_id = $1
		ORDER BY o.name
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Organization
	for rows.Next() {
		var o Organization
		var ipu, gss, poo sql.NullString
		if err := rows.Scan(&o.ID, &o.Name, &o.Slug, &o.DatasetsEnabled, &o.OpenSignupsEnabled, &o.CreatedAt, &ipu, &gss, &poo); err != nil {
			return nil, err
		}
		o.IntendedPublicURL = nullStrPtr(ipu)
		o.GiftedSubdomainSlug = nullStrPtr(gss)
		o.PublicOriginOverride = nullStrPtr(poo)
		out = append(out, o)
	}
	return out, rows.Err()
}

func (s *Store) GetOrganization(ctx context.Context, orgID uuid.UUID) (Organization, error) {
	var o Organization
	var ipu, gss, poo sql.NullString
	err := s.Pool.QueryRow(ctx, `
		SELECT id, name, slug, datasets_enabled, open_signups_enabled, created_at,
			intended_public_url, gifted_subdomain_slug, public_origin_override
		FROM organizations WHERE id = $1
	`, orgID).Scan(&o.ID, &o.Name, &o.Slug, &o.DatasetsEnabled, &o.OpenSignupsEnabled, &o.CreatedAt, &ipu, &gss, &poo)
	if err != nil {
		return o, err
	}
	o.IntendedPublicURL = nullStrPtr(ipu)
	o.GiftedSubdomainSlug = nullStrPtr(gss)
	o.PublicOriginOverride = nullStrPtr(poo)
	return o, nil
}

func (s *Store) OrgFeatures(ctx context.Context, orgID uuid.UUID) (OrgFeatures, error) {
	var f OrgFeatures
	err := s.Pool.QueryRow(ctx, `
		SELECT datasets_enabled, open_signups_enabled FROM organizations WHERE id = $1
	`, orgID).Scan(&f.DatasetsEnabled, &f.OpenSignupsEnabled)
	return f, err
}

func (s *Store) SetOrgFeatures(ctx context.Context, orgID uuid.UUID, f OrgFeatures) (OrgFeatures, error) {
	var out OrgFeatures
	err := s.Pool.QueryRow(ctx, `
		UPDATE organizations
		SET datasets_enabled = $2, open_signups_enabled = $3
		WHERE id = $1
		RETURNING datasets_enabled, open_signups_enabled
	`, orgID, f.DatasetsEnabled, f.OpenSignupsEnabled).Scan(&out.DatasetsEnabled, &out.OpenSignupsEnabled)
	return out, err
}

func (s *Store) MemberRole(ctx context.Context, orgID, userID uuid.UUID) (MemberRole, error) {
	var r MemberRole
	err := s.Pool.QueryRow(ctx, `
		SELECT role FROM organization_members WHERE organization_id = $1 AND user_id = $2
	`, orgID, userID).Scan(&r)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", pgx.ErrNoRows
	}
	return r, err
}

func (s *Store) OrganizationIDBySpace(ctx context.Context, spaceID uuid.UUID) (uuid.UUID, error) {
	var oid uuid.UUID
	err := s.Pool.QueryRow(ctx, `SELECT organization_id FROM spaces WHERE id = $1`, spaceID).Scan(&oid)
	return oid, err
}

func (s *Store) ListOrgMembersWithUser(ctx context.Context, orgID uuid.UUID) ([]OrgMemberWithUser, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT m.organization_id, m.user_id, m.role, u.email, u.display_name, u.last_seen_at,
			(sa.id IS NOT NULL) AS is_service_account,
			sa.provider,
			sa.openrouter_model,
			sa.cursor_default_repo_url
		FROM organization_members m
		JOIN users u ON u.id = m.user_id
		LEFT JOIN service_accounts sa ON sa.organization_id = m.organization_id AND sa.user_id = m.user_id
		WHERE m.organization_id = $1
		ORDER BY u.email
	`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []OrgMemberWithUser
	for rows.Next() {
		var r OrgMemberWithUser
		var provNS, orModelNS, curRepoNS sql.NullString
		if err := rows.Scan(
			&r.OrganizationID, &r.UserID, &r.Role, &r.Email, &r.DisplayName, &r.LastSeenAt, &r.IsServiceAccount,
			&provNS, &orModelNS, &curRepoNS,
		); err != nil {
			return nil, err
		}
		if provNS.Valid {
			v := provNS.String
			r.ServiceAccountProvider = &v
		}
		if orModelNS.Valid {
			v := orModelNS.String
			r.OpenRouterModel = &v
		}
		if curRepoNS.Valid {
			v := curRepoNS.String
			r.CursorDefaultRepoURL = &v
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// EnrichOrgMembersWithServiceAccountDetails loads service_accounts for the org and merges
// provider/model/repo into member rows. Use when list queries omit joined columns (older
// binaries) or fields are null in JSON.
func (s *Store) EnrichOrgMembersWithServiceAccountDetails(ctx context.Context, orgID uuid.UUID, members []OrgMemberWithUser) error {
	if len(members) == 0 {
		return nil
	}
	rows, err := s.Pool.Query(ctx, `
		SELECT user_id, provider, openrouter_model, cursor_default_repo_url
		FROM service_accounts
		WHERE organization_id = $1
	`, orgID)
	if err != nil {
		return err
	}
	defer rows.Close()

	type saRow struct {
		userID   uuid.UUID
		prov     string
		orModel  sql.NullString
		curRepo  sql.NullString
	}
	byUser := make(map[uuid.UUID]saRow)
	for rows.Next() {
		var r saRow
		if err := rows.Scan(&r.userID, &r.prov, &r.orModel, &r.curRepo); err != nil {
			return err
		}
		byUser[r.userID] = r
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for i := range members {
		sa, ok := byUser[members[i].UserID]
		if !ok {
			continue
		}
		members[i].IsServiceAccount = true
		if sa.prov != "" {
			p := sa.prov
			members[i].ServiceAccountProvider = &p
		}
		if sa.orModel.Valid {
			v := sa.orModel.String
			members[i].OpenRouterModel = &v
		}
		if sa.curRepo.Valid {
			v := sa.curRepo.String
			members[i].CursorDefaultRepoURL = &v
		}
	}
	return nil
}

func (s *Store) RemoveMember(ctx context.Context, orgID, userID uuid.UUID) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `
		DELETE FROM member_roles
		WHERE organization_id = $1 AND user_id = $2
	`, orgID, userID); err != nil {
		return err
	}

	ct, err := tx.Exec(ctx, `
		DELETE FROM organization_members
		WHERE organization_id = $1 AND user_id = $2
	`, orgID, userID)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return tx.Commit(ctx)
}

func nullStrPtr(ns sql.NullString) *string {
	if !ns.Valid {
		return nil
	}
	s := ns.String
	return &s
}

// SetOrgIntendedPublicURL updates the BYO URL hint (bootstrap or admin).
func (s *Store) SetOrgIntendedPublicURL(ctx context.Context, orgID uuid.UUID, url *string) error {
	_, err := s.Pool.Exec(ctx, `
		UPDATE organizations SET intended_public_url = $2 WHERE id = $1
	`, orgID, url)
	return err
}

// SetOrgGiftedSubdomainSlug records the last successful gifted subdomain claim.
func (s *Store) SetOrgGiftedSubdomainSlug(ctx context.Context, orgID uuid.UUID, slug *string) error {
	_, err := s.Pool.Exec(ctx, `
		UPDATE organizations SET gifted_subdomain_slug = $2 WHERE id = $1
	`, orgID, slug)
	return err
}

// ClearOrgGiftedSubdomainSlugIfMatch clears the slug when it matches (returns false if no row updated).
func (s *Store) ClearOrgGiftedSubdomainSlugIfMatch(ctx context.Context, orgID uuid.UUID, expectedSlug string) (bool, error) {
	ct, err := s.Pool.Exec(ctx, `
		UPDATE organizations
		SET gifted_subdomain_slug = NULL
		WHERE id = $1 AND gifted_subdomain_slug = $2
	`, orgID, expectedSlug)
	if err != nil {
		return false, err
	}
	return ct.RowsAffected() > 0, nil
}

// OrgIDByGiftedSubdomainSlug returns the org id that owns this gifted slug, if any.
func (s *Store) OrgIDByGiftedSubdomainSlug(ctx context.Context, slug string) (uuid.UUID, error) {
	var id uuid.UUID
	err := s.Pool.QueryRow(ctx, `
		SELECT id FROM organizations WHERE gifted_subdomain_slug = $1
	`, slug).Scan(&id)
	return id, err
}

// UpdateOrgIntendedPublicURL sets intended_public_url and returns the full organization row.
// When applyRuntimeOrigin is true, public_origin_override is set to runtimeOrigin (use nil to clear the column).
// When applyRuntimeOrigin is false, public_origin_override is unchanged.
func (s *Store) UpdateOrgIntendedPublicURL(ctx context.Context, orgID uuid.UUID, url *string, applyRuntimeOrigin bool, runtimeOrigin *string) (Organization, error) {
	var o Organization
	var ipu, gss, poo sql.NullString
	err := s.Pool.QueryRow(ctx, `
		UPDATE organizations SET
			intended_public_url = $2,
			public_origin_override = CASE WHEN $4::bool THEN $3 ELSE public_origin_override END
		WHERE id = $1
		RETURNING id, name, slug, datasets_enabled, open_signups_enabled, created_at,
			intended_public_url, gifted_subdomain_slug, public_origin_override
	`, orgID, url, runtimeOrigin, applyRuntimeOrigin).Scan(&o.ID, &o.Name, &o.Slug, &o.DatasetsEnabled, &o.OpenSignupsEnabled, &o.CreatedAt, &ipu, &gss, &poo)
	if err != nil {
		return Organization{}, err
	}
	o.IntendedPublicURL = nullStrPtr(ipu)
	o.GiftedSubdomainSlug = nullStrPtr(gss)
	o.PublicOriginOverride = nullStrPtr(poo)
	return o, nil
}

// GetSingletonPublicOriginOverride returns public_origin_override for the singleton organization row (single-org deployments).
func (s *Store) GetSingletonPublicOriginOverride(ctx context.Context) (*string, error) {
	var poo sql.NullString
	err := s.Pool.QueryRow(ctx, `SELECT public_origin_override FROM organizations LIMIT 1`).Scan(&poo)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	if !poo.Valid {
		return nil, nil
	}
	t := strings.TrimSpace(poo.String)
	if t == "" {
		return nil, nil
	}
	return &t, nil
}
