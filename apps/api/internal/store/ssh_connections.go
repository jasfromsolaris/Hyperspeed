package store

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type SSHConnection struct {
	ID             uuid.UUID `json:"id"`
	OrganizationID uuid.UUID `json:"organization_id"`
	OwnerUserID    uuid.UUID `json:"owner_user_id"`
	Name           string    `json:"name"`
	Host           string    `json:"host"`
	Port           int       `json:"port"`
	Username       string    `json:"username"`
	AuthMethod     string    `json:"auth_method"`
	HasPassword    bool      `json:"has_password"`
	HasKey         bool      `json:"has_key"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type SSHConnectionSecret struct {
	SSHConnection
	PrivateKeyEnc *string
	PassphraseEnc *string
	PasswordEnc   *string
}

func (s *Store) ListSSHConnections(ctx context.Context, orgID, ownerUserID uuid.UUID) ([]SSHConnection, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT
			id, organization_id, owner_user_id, name, host, port, username,
			auth_method,
			(password_enc IS NOT NULL AND length(password_enc) > 0) AS has_password,
			(private_key_enc IS NOT NULL AND length(private_key_enc) > 0) AS has_key,
			created_at, updated_at
		FROM ssh_connections
		WHERE organization_id = $1 AND owner_user_id = $2
		ORDER BY created_at DESC
	`, orgID, ownerUserID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []SSHConnection
	for rows.Next() {
		var c SSHConnection
		if err := rows.Scan(
			&c.ID,
			&c.OrganizationID,
			&c.OwnerUserID,
			&c.Name,
			&c.Host,
			&c.Port,
			&c.Username,
			&c.AuthMethod,
			&c.HasPassword,
			&c.HasKey,
			&c.CreatedAt,
			&c.UpdatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) CreateSSHConnection(
	ctx context.Context,
	orgID, ownerUserID uuid.UUID,
	name, host string,
	port int,
	username string,
	authMethod string,
	privateKeyEnc *string,
	passphraseEnc *string,
	passwordEnc *string,
) (SSHConnection, error) {
	var c SSHConnection
	err := s.Pool.QueryRow(ctx, `
		INSERT INTO ssh_connections
			(organization_id, owner_user_id, name, host, port, username, auth_method, private_key_enc, passphrase_enc, password_enc)
		VALUES
			($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		RETURNING
			id, organization_id, owner_user_id, name, host, port, username,
			auth_method,
			(password_enc IS NOT NULL AND length(password_enc) > 0) AS has_password,
			(private_key_enc IS NOT NULL AND length(private_key_enc) > 0) AS has_key,
			created_at, updated_at
	`, orgID, ownerUserID, name, host, port, username, authMethod, privateKeyEnc, passphraseEnc, passwordEnc).Scan(
		&c.ID,
		&c.OrganizationID,
		&c.OwnerUserID,
		&c.Name,
		&c.Host,
		&c.Port,
		&c.Username,
		&c.AuthMethod,
		&c.HasPassword,
		&c.HasKey,
		&c.CreatedAt,
		&c.UpdatedAt,
	)
	return c, err
}

func (s *Store) DeleteSSHConnection(ctx context.Context, orgID, ownerUserID, connID uuid.UUID) (bool, error) {
	tag, err := s.Pool.Exec(ctx, `
		DELETE FROM ssh_connections
		WHERE id = $1 AND organization_id = $2 AND owner_user_id = $3
	`, connID, orgID, ownerUserID)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

func (s *Store) GetSSHConnectionSecret(ctx context.Context, orgID, ownerUserID, connID uuid.UUID) (SSHConnectionSecret, error) {
	var c SSHConnectionSecret
	err := s.Pool.QueryRow(ctx, `
		SELECT
			id, organization_id, owner_user_id, name, host, port, username,
			auth_method,
			(password_enc IS NOT NULL AND length(password_enc) > 0) AS has_password,
			(private_key_enc IS NOT NULL AND length(private_key_enc) > 0) AS has_key,
			private_key_enc, passphrase_enc, password_enc,
			created_at, updated_at
		FROM ssh_connections
		WHERE id = $1 AND organization_id = $2 AND owner_user_id = $3
	`, connID, orgID, ownerUserID).Scan(
		&c.ID,
		&c.OrganizationID,
		&c.OwnerUserID,
		&c.Name,
		&c.Host,
		&c.Port,
		&c.Username,
		&c.AuthMethod,
		&c.HasPassword,
		&c.HasKey,
		&c.PrivateKeyEnc,
		&c.PassphraseEnc,
		&c.PasswordEnc,
		&c.CreatedAt,
		&c.UpdatedAt,
	)
	return c, err
}

func (s *Store) UpdateSSHConnection(
	ctx context.Context,
	orgID, ownerUserID, connID uuid.UUID,
	name, host *string,
	port *int,
	username *string,
	authMethod *string,
	privateKeyEnc *string,
	setPassphrase bool,
	passphraseEnc *string,
	setPassword bool,
	passwordEnc *string,
) (SSHConnection, error) {
	var c SSHConnection
	err := s.Pool.QueryRow(ctx, `
		UPDATE ssh_connections
		SET
			name = COALESCE($4, name),
			host = COALESCE($5, host),
			port = COALESCE($6, port),
			username = COALESCE($7, username),
			auth_method = COALESCE($8, auth_method),
			private_key_enc = COALESCE($9, private_key_enc),
			passphrase_enc = CASE
				WHEN $10::boolean IS TRUE THEN $11
				ELSE passphrase_enc
			END,
			password_enc = CASE
				WHEN $12::boolean IS TRUE THEN $13
				ELSE password_enc
			END,
			updated_at = now()
		WHERE id = $1 AND organization_id = $2 AND owner_user_id = $3
		RETURNING
			id, organization_id, owner_user_id, name, host, port, username,
			auth_method,
			(password_enc IS NOT NULL AND length(password_enc) > 0) AS has_password,
			(private_key_enc IS NOT NULL AND length(private_key_enc) > 0) AS has_key,
			created_at, updated_at
	`, connID, orgID, ownerUserID,
		name, host, port, username,
		authMethod,
		privateKeyEnc,
		setPassphrase, passphraseEnc,
		setPassword, passwordEnc,
	).Scan(
		&c.ID,
		&c.OrganizationID,
		&c.OwnerUserID,
		&c.Name,
		&c.Host,
		&c.Port,
		&c.Username,
		&c.AuthMethod,
		&c.HasPassword,
		&c.HasKey,
		&c.CreatedAt,
		&c.UpdatedAt,
	)
	if err != nil {
		return SSHConnection{}, err
	}
	return c, nil
}

