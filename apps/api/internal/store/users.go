package store

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type User struct {
	ID        uuid.UUID `json:"id"`
	Email     string    `json:"email"`
	DisplayName *string `json:"display_name,omitempty"`
	LastSeenAt time.Time `json:"last_seen_at"`
	CreatedAt time.Time `json:"created_at"`
}

func (s *Store) CreateUser(ctx context.Context, email, passwordHash string, displayName *string) (User, error) {
	var u User
	err := s.Pool.QueryRow(ctx, `
		INSERT INTO users (email, password_hash, display_name)
		VALUES ($1, $2, $3)
		RETURNING id, email, display_name, last_seen_at, created_at
	`, email, passwordHash, displayName).Scan(&u.ID, &u.Email, &u.DisplayName, &u.LastSeenAt, &u.CreatedAt)
	return u, err
}

func (s *Store) UserByEmail(ctx context.Context, email string) (uuid.UUID, string, error) {
	var id uuid.UUID
	var hash string
	err := s.Pool.QueryRow(ctx, `SELECT id, password_hash FROM users WHERE email = $1`, email).Scan(&id, &hash)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, "", pgx.ErrNoRows
	}
	return id, hash, err
}

func (s *Store) UserByID(ctx context.Context, id uuid.UUID) (User, error) {
	var u User
	err := s.Pool.QueryRow(ctx, `SELECT id, email, display_name, last_seen_at, created_at FROM users WHERE id = $1`, id).Scan(&u.ID, &u.Email, &u.DisplayName, &u.LastSeenAt, &u.CreatedAt)
	return u, err
}

func (s *Store) UpdateUserDisplayName(ctx context.Context, id uuid.UUID, displayName *string) (User, error) {
	var u User
	err := s.Pool.QueryRow(ctx, `
		UPDATE users
		SET display_name = $1
		WHERE id = $2
		RETURNING id, email, display_name, last_seen_at, created_at
	`, displayName, id).Scan(&u.ID, &u.Email, &u.DisplayName, &u.LastSeenAt, &u.CreatedAt)
	return u, err
}

func (s *Store) UpdateLastSeen(ctx context.Context, userID uuid.UUID) error {
	_, err := s.Pool.Exec(ctx, `UPDATE users SET last_seen_at = now() WHERE id = $1`, userID)
	return err
}

func (s *Store) SaveRefreshToken(ctx context.Context, userID uuid.UUID, tokenHash string, expiresAt time.Time) error {
	_, err := s.Pool.Exec(ctx, `
		INSERT INTO refresh_tokens (user_id, token_hash, expires_at)
		VALUES ($1, $2, $3)
	`, userID, tokenHash, expiresAt)
	return err
}

func (s *Store) DeleteRefreshToken(ctx context.Context, tokenHash string) error {
	ct, err := s.Pool.Exec(ctx, `DELETE FROM refresh_tokens WHERE token_hash = $1`, tokenHash)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (s *Store) UserIDByRefreshToken(ctx context.Context, tokenHash string) (uuid.UUID, error) {
	var uid uuid.UUID
	err := s.Pool.QueryRow(ctx, `
		SELECT user_id FROM refresh_tokens WHERE token_hash = $1 AND expires_at > now()
	`, tokenHash).Scan(&uid)
	return uid, err
}

// DeleteUser removes a user row (used to roll back failed bootstrap). Fails if FKs block.
func (s *Store) DeleteUser(ctx context.Context, id uuid.UUID) error {
	_, err := s.Pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, id)
	return err
}
