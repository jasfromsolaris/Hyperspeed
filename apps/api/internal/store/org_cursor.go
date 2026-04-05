package store

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"hyperspeed/api/internal/secrets"
)

// OrgCursorIntegrationMeta is safe to return to API clients (no secrets).
type OrgCursorIntegrationMeta struct {
	Configured    bool       `json:"configured"`
	LastRotatedAt *time.Time `json:"last_rotated_at,omitempty"`
	Hint          string     `json:"hint,omitempty"`
}

func (s *Store) GetOrgCursorIntegrationMeta(ctx context.Context, orgID uuid.UUID) (OrgCursorIntegrationMeta, error) {
	var enc *string
	var hint *string
	var updatedAt *time.Time
	err := s.Pool.QueryRow(ctx, `
		SELECT cursor_api_key_enc, cursor_api_key_hint, cursor_api_key_updated_at
		FROM organizations WHERE id = $1
	`, orgID).Scan(&enc, &hint, &updatedAt)
	if err != nil {
		return OrgCursorIntegrationMeta{}, err
	}
	var m OrgCursorIntegrationMeta
	if enc != nil && strings.TrimSpace(*enc) != "" {
		m.Configured = true
		m.LastRotatedAt = updatedAt
		if hint != nil && *hint != "" {
			m.Hint = "****" + *hint
		}
	}
	return m, nil
}

// SetOrgCursorAPIKey encrypts plaintext with key32 (32-byte AES key) and stores ciphertext + last-4 hint.
func (s *Store) SetOrgCursorAPIKey(ctx context.Context, orgID uuid.UUID, plaintextAPIKey string, key32 []byte, updatedBy uuid.UUID) error {
	plaintextAPIKey = strings.TrimSpace(plaintextAPIKey)
	if plaintextAPIKey == "" {
		return errors.New("empty api key")
	}
	if len(key32) != 32 {
		return errors.New("invalid encryption key length")
	}
	hint := ""
	if len(plaintextAPIKey) >= 4 {
		hint = plaintextAPIKey[len(plaintextAPIKey)-4:]
	}
	enc, err := secrets.EncryptString(key32, plaintextAPIKey)
	if err != nil {
		return err
	}
	tag, err := s.Pool.Exec(ctx, `
		UPDATE organizations
		SET cursor_api_key_enc = $2,
		    cursor_api_key_hint = $3,
		    cursor_api_key_updated_at = now(),
		    cursor_api_key_updated_by = $4
		WHERE id = $1
	`, orgID, enc, nullIfEmpty(hint), updatedBy)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (s *Store) ClearOrgCursorAPIKey(ctx context.Context, orgID uuid.UUID) error {
	tag, err := s.Pool.Exec(ctx, `
		UPDATE organizations
		SET cursor_api_key_enc = NULL,
		    cursor_api_key_hint = NULL,
		    cursor_api_key_updated_at = NULL,
		    cursor_api_key_updated_by = NULL
		WHERE id = $1
	`, orgID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

// DecryptedOrgCursorAPIKey returns plaintext or ("", nil) if not configured.
func (s *Store) DecryptedOrgCursorAPIKey(ctx context.Context, orgID uuid.UUID, key32 []byte) (string, error) {
	if len(key32) != 32 {
		return "", nil
	}
	var enc *string
	err := s.Pool.QueryRow(ctx, `
		SELECT cursor_api_key_enc FROM organizations WHERE id = $1
	`, orgID).Scan(&enc)
	if err != nil {
		return "", err
	}
	if enc == nil || strings.TrimSpace(*enc) == "" {
		return "", nil
	}
	return secrets.DecryptString(key32, *enc)
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}
