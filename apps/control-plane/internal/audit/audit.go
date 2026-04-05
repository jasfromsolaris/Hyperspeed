package audit

import (
	"context"
	"database/sql"
	"time"

	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

func Open(ctx context.Context, path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	s := &Store{db: db}
	if err := s.migrate(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) migrate(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS claims (
			slug TEXT NOT NULL PRIMARY KEY,
			ipv4 TEXT NOT NULL,
			cf_record_id TEXT NOT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)
	`)
	return err
}

func (s *Store) UpsertClaim(ctx context.Context, slug, ipv4, cfRecordID string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO claims (slug, ipv4, cf_record_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(slug) DO UPDATE SET
			ipv4 = excluded.ipv4,
			cf_record_id = excluded.cf_record_id,
			updated_at = excluded.updated_at
	`, slug, ipv4, cfRecordID, now, now)
	return err
}

func (s *Store) DeleteClaim(ctx context.Context, slug string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM claims WHERE slug = ?`, slug)
	return err
}

func (s *Store) GetRecordID(ctx context.Context, slug string) (string, bool, error) {
	var id string
	err := s.db.QueryRowContext(ctx, `SELECT cf_record_id FROM claims WHERE slug = ?`, slug).Scan(&id)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return id, true, nil
}

func (s *Store) Close() error { return s.db.Close() }
