package store

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"
)

const (
	ProviderOpenRouter = "openrouter"
	ProviderCursor     = "cursor"
)

type ServiceAccount struct {
	ID                   uuid.UUID  `json:"id"`
	OrganizationID       uuid.UUID  `json:"organization_id"`
	UserID               uuid.UUID  `json:"user_id"`
	Name                 string     `json:"name"`
	CreatedBy            uuid.UUID  `json:"created_by"`
	CreatedAt            time.Time  `json:"created_at"`
	Provider             string     `json:"provider"`
	OpenRouterModel      *string    `json:"openrouter_model,omitempty"`
	CursorDefaultRepoURL *string    `json:"cursor_default_repo_url,omitempty"`
	CursorDefaultRef     *string    `json:"cursor_default_ref,omitempty"`
}

// CreateServiceAccountInput configures a new AI staff member.
type CreateServiceAccountInput struct {
	Name                 string
	Provider             string // openrouter | cursor; default openrouter
	OpenRouterModel      *string
	CursorDefaultRepoURL *string
	CursorDefaultRef     *string
}

// PatchServiceAccountInput updates provider-specific fields (partial).
type PatchServiceAccountInput struct {
	Provider             *string
	OpenRouterModel      *string
	CursorDefaultRepoURL *string
	CursorDefaultRef     *string
}

func newServiceAccountToken() (raw string, hash string, err error) {
	var b [32]byte
	if _, err = rand.Read(b[:]); err != nil {
		return "", "", err
	}
	raw = "sa_" + hex.EncodeToString(b[:])
	h := sha256.Sum256([]byte(raw))
	hash = hex.EncodeToString(h[:])
	return raw, hash, nil
}

func (s *Store) CreateServiceAccount(ctx context.Context, orgID, createdBy uuid.UUID, name string) (sa ServiceAccount, rawToken string, err error) {
	return s.CreateServiceAccountWithOptions(ctx, orgID, createdBy, CreateServiceAccountInput{Name: name})
}

func (s *Store) CreateServiceAccountWithOptions(ctx context.Context, orgID, createdBy uuid.UUID, in CreateServiceAccountInput) (sa ServiceAccount, rawToken string, err error) {
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return ServiceAccount{}, "", pgx.ErrNoRows
	}
	prov := strings.TrimSpace(strings.ToLower(in.Provider))
	if prov == "" {
		prov = ProviderOpenRouter
	}
	if prov != ProviderOpenRouter && prov != ProviderCursor {
		return ServiceAccount{}, "", pgx.ErrNoRows
	}
	// Create a backing user so the rest of the system can treat this as a principal.
	var pw [32]byte
	if _, err := rand.Read(pw[:]); err != nil {
		return ServiceAccount{}, "", err
	}
	pwHashBytes, err := bcrypt.GenerateFromPassword(pw[:], bcrypt.DefaultCost)
	if err != nil {
		return ServiceAccount{}, "", err
	}
	pwHash := string(pwHashBytes)
	email := "sa+" + uuid.NewString() + "@hyperspeed.local"
	u, err := s.CreateUser(ctx, email, pwHash, &name)
	if err != nil {
		return ServiceAccount{}, "", err
	}

	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return ServiceAccount{}, "", err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `
		INSERT INTO organization_members (organization_id, user_id, role)
		VALUES ($1, $2, $3)
		ON CONFLICT (organization_id, user_id) DO NOTHING
	`, orgID, u.ID, RoleMember); err != nil {
		return ServiceAccount{}, "", err
	}

	var orModel, curRepo, curRef *string
	if in.OpenRouterModel != nil && strings.TrimSpace(*in.OpenRouterModel) != "" {
		v := strings.TrimSpace(*in.OpenRouterModel)
		orModel = &v
	}
	if in.CursorDefaultRepoURL != nil && strings.TrimSpace(*in.CursorDefaultRepoURL) != "" {
		v := strings.TrimSpace(*in.CursorDefaultRepoURL)
		curRepo = &v
	}
	if in.CursorDefaultRef != nil && strings.TrimSpace(*in.CursorDefaultRef) != "" {
		v := strings.TrimSpace(*in.CursorDefaultRef)
		curRef = &v
	}

	err = tx.QueryRow(ctx, `
		INSERT INTO service_accounts (organization_id, user_id, name, created_by, provider, openrouter_model, cursor_default_repo_url, cursor_default_ref)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id, organization_id, user_id, name, created_by, created_at, provider, openrouter_model, cursor_default_repo_url, cursor_default_ref
	`, orgID, u.ID, name, createdBy, prov, orModel, curRepo, curRef).Scan(
		&sa.ID, &sa.OrganizationID, &sa.UserID, &sa.Name, &sa.CreatedBy, &sa.CreatedAt,
		&sa.Provider, &sa.OpenRouterModel, &sa.CursorDefaultRepoURL, &sa.CursorDefaultRef,
	)
	if err != nil {
		return ServiceAccount{}, "", err
	}

	raw, hash, err := newServiceAccountToken()
	if err != nil {
		return ServiceAccount{}, "", err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO service_account_tokens (service_account_id, token_hash)
		VALUES ($1, $2)
	`, sa.ID, hash); err != nil {
		return ServiceAccount{}, "", err
	}

	if err := insertDefaultServiceAccountProfileTx(ctx, tx, sa.ID, createdBy); err != nil {
		return ServiceAccount{}, "", err
	}

	if err := tx.Commit(ctx); err != nil {
		return ServiceAccount{}, "", err
	}

	return sa, raw, nil
}

func (s *Store) ListServiceAccounts(ctx context.Context, orgID uuid.UUID) ([]ServiceAccount, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT id, organization_id, user_id, name, created_by, created_at,
		       provider, openrouter_model, cursor_default_repo_url, cursor_default_ref
		FROM service_accounts
		WHERE organization_id = $1
		ORDER BY created_at DESC
	`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ServiceAccount
	for rows.Next() {
		var sa ServiceAccount
		if err := rows.Scan(
			&sa.ID, &sa.OrganizationID, &sa.UserID, &sa.Name, &sa.CreatedBy, &sa.CreatedAt,
			&sa.Provider, &sa.OpenRouterModel, &sa.CursorDefaultRepoURL, &sa.CursorDefaultRef,
		); err != nil {
			return nil, err
		}
		out = append(out, sa)
	}
	return out, rows.Err()
}

func (s *Store) GetServiceAccountInOrg(ctx context.Context, orgID, saID uuid.UUID) (ServiceAccount, error) {
	var sa ServiceAccount
	err := s.Pool.QueryRow(ctx, `
		SELECT id, organization_id, user_id, name, created_by, created_at,
		       provider, openrouter_model, cursor_default_repo_url, cursor_default_ref
		FROM service_accounts
		WHERE id = $1 AND organization_id = $2
	`, saID, orgID).Scan(
		&sa.ID, &sa.OrganizationID, &sa.UserID, &sa.Name, &sa.CreatedBy, &sa.CreatedAt,
		&sa.Provider, &sa.OpenRouterModel, &sa.CursorDefaultRepoURL, &sa.CursorDefaultRef,
	)
	return sa, err
}

// GetServiceAccountByUserInOrg returns the service account row for a backing user id in the org.
func (s *Store) GetServiceAccountByUserInOrg(ctx context.Context, orgID, userID uuid.UUID) (ServiceAccount, error) {
	var sa ServiceAccount
	err := s.Pool.QueryRow(ctx, `
		SELECT id, organization_id, user_id, name, created_by, created_at,
		       provider, openrouter_model, cursor_default_repo_url, cursor_default_ref
		FROM service_accounts
		WHERE organization_id = $1 AND user_id = $2
	`, orgID, userID).Scan(
		&sa.ID, &sa.OrganizationID, &sa.UserID, &sa.Name, &sa.CreatedBy, &sa.CreatedAt,
		&sa.Provider, &sa.OpenRouterModel, &sa.CursorDefaultRepoURL, &sa.CursorDefaultRef,
	)
	return sa, err
}

func (s *Store) PatchServiceAccount(ctx context.Context, orgID, saID uuid.UUID, in PatchServiceAccountInput) (ServiceAccount, error) {
	cur, err := s.GetServiceAccountInOrg(ctx, orgID, saID)
	if err != nil {
		return ServiceAccount{}, err
	}
	next := cur
	if in.Provider != nil {
		p := strings.TrimSpace(strings.ToLower(*in.Provider))
		if p == ProviderOpenRouter || p == ProviderCursor {
			next.Provider = p
		}
	}
	if in.OpenRouterModel != nil {
		v := strings.TrimSpace(*in.OpenRouterModel)
		if v == "" {
			next.OpenRouterModel = nil
		} else {
			next.OpenRouterModel = &v
		}
	}
	if in.CursorDefaultRepoURL != nil {
		v := strings.TrimSpace(*in.CursorDefaultRepoURL)
		if v == "" {
			next.CursorDefaultRepoURL = nil
		} else {
			next.CursorDefaultRepoURL = &v
		}
	}
	if in.CursorDefaultRef != nil {
		v := strings.TrimSpace(*in.CursorDefaultRef)
		if v == "" {
			next.CursorDefaultRef = nil
		} else {
			next.CursorDefaultRef = &v
		}
	}
	var sa ServiceAccount
	err = s.Pool.QueryRow(ctx, `
		UPDATE service_accounts
		SET provider = $3,
		    openrouter_model = $4,
		    cursor_default_repo_url = $5,
		    cursor_default_ref = $6
		WHERE id = $1 AND organization_id = $2
		RETURNING id, organization_id, user_id, name, created_by, created_at,
		          provider, openrouter_model, cursor_default_repo_url, cursor_default_ref
	`, saID, orgID, next.Provider, next.OpenRouterModel, next.CursorDefaultRepoURL, next.CursorDefaultRef).Scan(
		&sa.ID, &sa.OrganizationID, &sa.UserID, &sa.Name, &sa.CreatedBy, &sa.CreatedAt,
		&sa.Provider, &sa.OpenRouterModel, &sa.CursorDefaultRepoURL, &sa.CursorDefaultRef,
	)
	return sa, err
}

func (s *Store) DeleteServiceAccount(ctx context.Context, orgID, saID uuid.UUID) error {
	// Remove service account and its org membership in one transaction so
	// deleted AI staff no longer shows in Org Roles > Members.
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var userID uuid.UUID
	if err := tx.QueryRow(ctx, `
		SELECT user_id
		FROM service_accounts
		WHERE id = $1 AND organization_id = $2
	`, saID, orgID).Scan(&userID); err != nil {
		if err == pgx.ErrNoRows {
			return pgx.ErrNoRows
		}
		return err
	}

	if _, err := tx.Exec(ctx, `
		DELETE FROM service_accounts
		WHERE id = $1 AND organization_id = $2
	`, saID, orgID); err != nil {
		return err
	}

	// Remove role assignments and org member row for the backing principal.
	if _, err := tx.Exec(ctx, `
		DELETE FROM member_roles
		WHERE organization_id = $1 AND user_id = $2
	`, orgID, userID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		DELETE FROM organization_members
		WHERE organization_id = $1 AND user_id = $2
	`, orgID, userID); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (s *Store) ServiceAccountUserIDByToken(ctx context.Context, rawToken string) (uuid.UUID, error) {
	h := sha256.Sum256([]byte(rawToken))
	hash := hex.EncodeToString(h[:])
	var uid uuid.UUID
	err := s.Pool.QueryRow(ctx, `
		SELECT sa.user_id
		FROM service_account_tokens t
		JOIN service_accounts sa ON sa.id = t.service_account_id
		WHERE t.token_hash = $1
		  AND (t.expires_at IS NULL OR t.expires_at > now())
	`, hash).Scan(&uid)
	return uid, err
}

func (s *Store) TouchServiceAccountToken(ctx context.Context, rawToken string) {
	h := sha256.Sum256([]byte(rawToken))
	hash := hex.EncodeToString(h[:])
	_, _ = s.Pool.Exec(ctx, `UPDATE service_account_tokens SET last_used_at = now() WHERE token_hash = $1`, hash)
}

// BackfillServiceAccountMemberRoles is a no-op: AI staff accounts have no default RBAC roles;
// owners assign roles explicitly (same as human members).
func (s *Store) BackfillServiceAccountMemberRoles(ctx context.Context) error {
	_ = ctx
	return nil
}

