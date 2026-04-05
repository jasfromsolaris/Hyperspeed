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

// OrgOpenRouterIntegrationMeta is safe for API clients.
type OrgOpenRouterIntegrationMeta struct {
	Configured    bool       `json:"configured"`
	LastRotatedAt *time.Time `json:"last_rotated_at,omitempty"`
	Hint          string     `json:"hint,omitempty"`
}

func (s *Store) GetOrgOpenRouterIntegrationMeta(ctx context.Context, orgID uuid.UUID) (OrgOpenRouterIntegrationMeta, error) {
	var enc *string
	var hint *string
	var updatedAt *time.Time
	err := s.Pool.QueryRow(ctx, `
		SELECT openrouter_api_key_enc, openrouter_api_key_hint, openrouter_api_key_updated_at
		FROM organizations WHERE id = $1
	`, orgID).Scan(&enc, &hint, &updatedAt)
	if err != nil {
		return OrgOpenRouterIntegrationMeta{}, err
	}
	var m OrgOpenRouterIntegrationMeta
	if enc != nil && strings.TrimSpace(*enc) != "" {
		m.Configured = true
		m.LastRotatedAt = updatedAt
		if hint != nil && *hint != "" {
			m.Hint = "****" + *hint
		}
	}
	return m, nil
}

func (s *Store) SetOrgOpenRouterAPIKey(ctx context.Context, orgID uuid.UUID, plaintext string, key32 []byte, updatedBy uuid.UUID) error {
	plaintext = strings.TrimSpace(plaintext)
	if plaintext == "" {
		return errors.New("empty api key")
	}
	if len(key32) != 32 {
		return errors.New("invalid encryption key length")
	}
	hint := ""
	if len(plaintext) >= 4 {
		hint = plaintext[len(plaintext)-4:]
	}
	enc, err := secrets.EncryptString(key32, plaintext)
	if err != nil {
		return err
	}
	tag, err := s.Pool.Exec(ctx, `
		UPDATE organizations
		SET openrouter_api_key_enc = $2,
		    openrouter_api_key_hint = $3,
		    openrouter_api_key_updated_at = now(),
		    openrouter_api_key_updated_by = $4
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

func (s *Store) ClearOrgOpenRouterAPIKey(ctx context.Context, orgID uuid.UUID) error {
	tag, err := s.Pool.Exec(ctx, `
		UPDATE organizations
		SET openrouter_api_key_enc = NULL,
		    openrouter_api_key_hint = NULL,
		    openrouter_api_key_updated_at = NULL,
		    openrouter_api_key_updated_by = NULL
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

// DecryptedOrgOpenRouterAPIKey returns plaintext or ("", nil) if not configured.
func (s *Store) DecryptedOrgOpenRouterAPIKey(ctx context.Context, orgID uuid.UUID, key32 []byte) (string, error) {
	if len(key32) != 32 {
		return "", nil
	}
	var enc *string
	err := s.Pool.QueryRow(ctx, `
		SELECT openrouter_api_key_enc FROM organizations WHERE id = $1
	`, orgID).Scan(&enc)
	if err != nil {
		return "", err
	}
	if enc == nil || strings.TrimSpace(*enc) == "" {
		return "", nil
	}
	return secrets.DecryptString(key32, *enc)
}
