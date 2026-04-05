package store

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// SpaceGitLink is a space ↔ Git remote binding (one per space).
type SpaceGitLink struct {
	SpaceID         uuid.UUID
	RemoteURL       string
	Branch          string
	RootFolderID    *uuid.UUID
	TokenCiphertext *string
	TokenLast4      *string
	LastCommitSHA   *string
	LastError       *string
	LastSyncAt      *time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

func (s *Store) GetSpaceGitLink(ctx context.Context, spaceID uuid.UUID) (SpaceGitLink, error) {
	var l SpaceGitLink
	err := s.Pool.QueryRow(ctx, `
		SELECT space_id, remote_url, branch, root_folder_id, token_ciphertext, token_last4,
		       last_commit_sha, last_error, last_sync_at, created_at, updated_at
		FROM space_git_links WHERE space_id = $1
	`, spaceID).Scan(
		&l.SpaceID, &l.RemoteURL, &l.Branch, &l.RootFolderID, &l.TokenCiphertext, &l.TokenLast4,
		&l.LastCommitSHA, &l.LastError, &l.LastSyncAt, &l.CreatedAt, &l.UpdatedAt,
	)
	return l, err
}

func (s *Store) UpsertSpaceGitLink(ctx context.Context, l SpaceGitLink) error {
	_, err := s.Pool.Exec(ctx, `
		INSERT INTO space_git_links (
			space_id, remote_url, branch, root_folder_id, token_ciphertext, token_last4,
			created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, now(), now()
		)
		ON CONFLICT (space_id) DO UPDATE SET
			remote_url = EXCLUDED.remote_url,
			branch = EXCLUDED.branch,
			root_folder_id = EXCLUDED.root_folder_id,
			token_ciphertext = COALESCE(EXCLUDED.token_ciphertext, space_git_links.token_ciphertext),
			token_last4 = COALESCE(EXCLUDED.token_last4, space_git_links.token_last4),
			updated_at = now()
	`, l.SpaceID, l.RemoteURL, l.Branch, l.RootFolderID, l.TokenCiphertext, l.TokenLast4)
	return err
}

func (s *Store) DeleteSpaceGitLink(ctx context.Context, spaceID uuid.UUID) error {
	tag, err := s.Pool.Exec(ctx, `DELETE FROM space_git_links WHERE space_id = $1`, spaceID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (s *Store) UpdateSpaceGitLinkSyncOK(ctx context.Context, spaceID uuid.UUID, commitSHA string, syncAt time.Time) error {
	_, err := s.Pool.Exec(ctx, `
		UPDATE space_git_links SET
			last_commit_sha = $2,
			last_error = NULL,
			last_sync_at = $3,
			updated_at = now()
		WHERE space_id = $1
	`, spaceID, commitSHA, syncAt)
	return err
}

func (s *Store) UpdateSpaceGitLinkSyncError(ctx context.Context, spaceID uuid.UUID, errMsg string) error {
	var arg any
	if strings.TrimSpace(errMsg) == "" {
		arg = nil
	} else {
		arg = errMsg
	}
	_, err := s.Pool.Exec(ctx, `
		UPDATE space_git_links SET last_error = $2, updated_at = now() WHERE space_id = $1
	`, spaceID, arg)
	return err
}
