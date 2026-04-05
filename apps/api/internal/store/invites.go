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
)

type OrgInvite struct {
	ID             uuid.UUID  `json:"id"`
	OrganizationID uuid.UUID  `json:"organization_id"`
	Email          *string    `json:"email,omitempty"`
	CreatedBy      uuid.UUID  `json:"created_by"`
	CreatedAt      time.Time  `json:"created_at"`
	ExpiresAt      time.Time  `json:"expires_at"`
	ConsumedAt     *time.Time `json:"consumed_at,omitempty"`
	ConsumedBy     *uuid.UUID `json:"consumed_by,omitempty"`
}

func newInviteToken() (raw string, hash string, err error) {
	var b [32]byte
	if _, err = rand.Read(b[:]); err != nil {
		return "", "", err
	}
	raw = hex.EncodeToString(b[:])
	h := sha256.Sum256([]byte(raw))
	hash = hex.EncodeToString(h[:])
	return raw, hash, nil
}

func (s *Store) CreateOrgInvite(ctx context.Context, orgID, createdBy uuid.UUID, email *string, ttl time.Duration) (invite OrgInvite, rawToken string, err error) {
	raw, hash, err := newInviteToken()
	if err != nil {
		return OrgInvite{}, "", err
	}
	var em *string
	if email != nil {
		v := strings.TrimSpace(strings.ToLower(*email))
		if v != "" {
			em = &v
		}
	}
	exp := time.Now().Add(ttl)
	err = s.Pool.QueryRow(ctx, `
		INSERT INTO org_invites (organization_id, email, token_hash, created_by, expires_at)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, organization_id, email, created_by, created_at, expires_at, consumed_at, consumed_by
	`, orgID, em, hash, createdBy, exp).Scan(
		&invite.ID, &invite.OrganizationID, &invite.Email, &invite.CreatedBy, &invite.CreatedAt,
		&invite.ExpiresAt, &invite.ConsumedAt, &invite.ConsumedBy,
	)
	if err != nil {
		return OrgInvite{}, "", err
	}
	return invite, raw, nil
}

func (s *Store) ConsumeOrgInvite(ctx context.Context, rawToken string, userID uuid.UUID) (OrgInvite, error) {
	hash := sha256.Sum256([]byte(rawToken))
	h := hex.EncodeToString(hash[:])

	var inv OrgInvite
	err := s.Pool.QueryRow(ctx, `
		UPDATE org_invites
		SET consumed_at = now(), consumed_by = $2
		WHERE token_hash = $1
		  AND consumed_at IS NULL
		  AND expires_at > now()
		RETURNING id, organization_id, email, created_by, created_at, expires_at, consumed_at, consumed_by
	`, h, userID).Scan(
		&inv.ID, &inv.OrganizationID, &inv.Email, &inv.CreatedBy, &inv.CreatedAt,
		&inv.ExpiresAt, &inv.ConsumedAt, &inv.ConsumedBy,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return OrgInvite{}, pgx.ErrNoRows
		}
		return OrgInvite{}, err
	}
	return inv, nil
}

