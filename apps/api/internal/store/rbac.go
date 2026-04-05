package store

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func isOwnerSystemRoleName(name string) bool {
	return strings.EqualFold(strings.TrimSpace(name), "Owner")
}

// ErrSystemRoleImmutable is returned when renaming a built-in system role.
var ErrSystemRoleImmutable = errors.New("system role cannot be renamed")

// ErrCannotDeleteOwnerRole is returned when attempting to delete the Owner system role.
var ErrCannotDeleteOwnerRole = errors.New("cannot delete owner role")

type Role struct {
	ID             uuid.UUID `json:"id"`
	OrganizationID uuid.UUID `json:"organization_id"`
	Name           string    `json:"name"`
	IsSystem       bool      `json:"is_system"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// EnsureSystemRoles creates baseline roles for an org if missing.
// This is idempotent and safe to call frequently.
func (s *Store) EnsureSystemRoles(ctx context.Context, orgID uuid.UUID) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := s.ensureSystemRolesTx(ctx, tx, orgID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) ensureSystemRolesTx(ctx context.Context, tx pgx.Tx, orgID uuid.UUID) error {
	// Keep names stable; the UI can treat these specially via is_system.
	system := []struct {
		Name  string
		Perms []string
	}{
		{
			Name: "Owner",
			Perms: []string{
				"org.manage",
				"org.members.manage",
				"space.create",
				"space.delete",
				"space.members.manage",
				"board.read",
				"board.write",
				"tasks.read",
				"tasks.write",
				"chat.read",
				"chat.write",
				"files.read",
				"files.write",
				"files.delete",
				"terminal.use",
				"ssh_connections.manage",
				"agent.tools.invoke",
				"datasets.read",
				"datasets.write",
			},
		},
	}

	for _, r := range system {
		var roleID uuid.UUID
		err := tx.QueryRow(ctx, `
			INSERT INTO roles (organization_id, name, is_system)
			VALUES ($1, $2, true)
			ON CONFLICT (organization_id, name) DO UPDATE SET updated_at = now()
			RETURNING id
		`, orgID, r.Name).Scan(&roleID)
		if err != nil {
			return err
		}
		// Ensure permission set matches exactly.
		if _, err := tx.Exec(ctx, `DELETE FROM role_permissions WHERE role_id = $1`, roleID); err != nil {
			return err
		}
		for _, p := range r.Perms {
			if _, err := tx.Exec(ctx, `
				INSERT INTO role_permissions (role_id, permission)
				VALUES ($1, $2)
				ON CONFLICT DO NOTHING
			`, roleID, p); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Store) ListRoles(ctx context.Context, orgID uuid.UUID) ([]Role, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT id, organization_id, name, is_system, created_at, updated_at
		FROM roles
		WHERE organization_id = $1
		ORDER BY is_system DESC, name ASC
	`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Role
	for rows.Next() {
		var r Role
		if err := rows.Scan(&r.ID, &r.OrganizationID, &r.Name, &r.IsSystem, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) CreateRole(ctx context.Context, orgID uuid.UUID, name string) (Role, error) {
	var r Role
	err := s.Pool.QueryRow(ctx, `
		INSERT INTO roles (organization_id, name, is_system)
		VALUES ($1, $2, false)
		RETURNING id, organization_id, name, is_system, created_at, updated_at
	`, orgID, name).Scan(&r.ID, &r.OrganizationID, &r.Name, &r.IsSystem, &r.CreatedAt, &r.UpdatedAt)
	return r, err
}

func (s *Store) RenameRole(ctx context.Context, orgID, roleID uuid.UUID, name string) (Role, error) {
	var isSys bool
	err := s.Pool.QueryRow(ctx, `
		SELECT is_system FROM roles WHERE id = $1 AND organization_id = $2
	`, roleID, orgID).Scan(&isSys)
	if err != nil {
		return Role{}, err
	}
	if isSys {
		return Role{}, ErrSystemRoleImmutable
	}
	var r Role
	err = s.Pool.QueryRow(ctx, `
		UPDATE roles SET name = $3, updated_at = now()
		WHERE id = $1 AND organization_id = $2
		RETURNING id, organization_id, name, is_system, created_at, updated_at
	`, roleID, orgID, name).Scan(&r.ID, &r.OrganizationID, &r.Name, &r.IsSystem, &r.CreatedAt, &r.UpdatedAt)
	return r, err
}

func (s *Store) DeleteRole(ctx context.Context, orgID, roleID uuid.UUID) error {
	var name string
	var isSys bool
	err := s.Pool.QueryRow(ctx, `
		SELECT name, is_system FROM roles WHERE id = $1 AND organization_id = $2
	`, roleID, orgID).Scan(&name, &isSys)
	if err != nil {
		return err
	}
	if isSys && isOwnerSystemRoleName(name) {
		return ErrCannotDeleteOwnerRole
	}
	ct, err := s.Pool.Exec(ctx, `
		DELETE FROM roles
		WHERE id = $1 AND organization_id = $2
		  AND NOT (is_system AND lower(trim(name)) = 'owner')
	`, roleID, orgID)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (s *Store) RolePermissions(ctx context.Context, roleID uuid.UUID) ([]string, error) {
	rows, err := s.Pool.Query(ctx, `SELECT permission FROM role_permissions WHERE role_id = $1 ORDER BY permission`, roleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Store) ReplaceRolePermissions(ctx context.Context, orgID, roleID uuid.UUID, perms []string) error {
	// Ensure role belongs to org.
	var dummy int
	if err := s.Pool.QueryRow(ctx, `SELECT 1 FROM roles WHERE id = $1 AND organization_id = $2`, roleID, orgID).Scan(&dummy); err != nil {
		return err
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `DELETE FROM role_permissions WHERE role_id = $1`, roleID); err != nil {
		return err
	}
	for _, p := range perms {
		if _, err := tx.Exec(ctx, `
			INSERT INTO role_permissions (role_id, permission)
			VALUES ($1, $2)
			ON CONFLICT DO NOTHING
		`, roleID, p); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE roles SET updated_at = now() WHERE id = $1`, roleID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) SystemRoleIDByName(ctx context.Context, orgID uuid.UUID, name string) (uuid.UUID, error) {
	var id uuid.UUID
	err := s.Pool.QueryRow(ctx, `
		SELECT id FROM roles WHERE organization_id = $1 AND name = $2 AND is_system = true
	`, orgID, name).Scan(&id)
	return id, err
}

func (s *Store) ReplaceMemberRoles(ctx context.Context, orgID, userID uuid.UUID, roleIDs []uuid.UUID) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `DELETE FROM member_roles WHERE organization_id = $1 AND user_id = $2`, orgID, userID); err != nil {
		return err
	}
	for _, rid := range roleIDs {
		if _, err := tx.Exec(ctx, `
			INSERT INTO member_roles (organization_id, user_id, role_id)
			VALUES ($1, $2, $3)
			ON CONFLICT DO NOTHING
		`, orgID, userID, rid); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (s *Store) MemberRoleIDs(ctx context.Context, orgID, userID uuid.UUID) ([]uuid.UUID, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT role_id FROM member_roles WHERE organization_id = $1 AND user_id = $2
	`, orgID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func (s *Store) ListUserIDsForRole(ctx context.Context, orgID, roleID uuid.UUID) ([]uuid.UUID, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT user_id
		FROM member_roles
		WHERE organization_id = $1 AND role_id = $2
	`, orgID, roleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func (s *Store) MemberHasPermission(ctx context.Context, orgID, userID uuid.UUID, perm string) (bool, error) {
	var ok bool
	err := s.Pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1
			FROM member_roles mr
			JOIN role_permissions rp ON rp.role_id = mr.role_id
			WHERE mr.organization_id = $1 AND mr.user_id = $2 AND rp.permission = $3
		)
	`, orgID, userID, perm).Scan(&ok)
	return ok, err
}

// EnsureLegacyRoleMapped ensures a user has at least one member_roles entry based on
// their legacy organization_members.role, to support a smooth transition.
func (s *Store) EnsureLegacyRoleMapped(ctx context.Context, orgID, userID uuid.UUID) error {
	ids, err := s.MemberRoleIDs(ctx, orgID, userID)
	if err != nil {
		return err
	}
	if len(ids) > 0 {
		return nil
	}
	legacy, err := s.MemberRole(ctx, orgID, userID)
	if err != nil {
		return err
	}
	if err := s.EnsureSystemRoles(ctx, orgID); err != nil {
		return err
	}
	// Back-compat: map legacy org "admin" to the new system "Owner" role.
	// Legacy "member" gets no automatic role assignment.
	if legacy != RoleAdmin {
		return nil
	}
	target := "Owner"
	rid, err := s.SystemRoleIDByName(ctx, orgID, target)
	if err != nil {
		return err
	}
	// Assign.
	_, err = s.Pool.Exec(ctx, `
		INSERT INTO member_roles (organization_id, user_id, role_id)
		VALUES ($1, $2, $3)
		ON CONFLICT DO NOTHING
	`, orgID, userID, rid)
	return err
}

var ErrRoleNameTaken = errors.New("role name taken")

