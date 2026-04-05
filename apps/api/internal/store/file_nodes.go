package store

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type FileNodeKind string

const (
	FileNodeFolder FileNodeKind = "folder"
	FileNodeFile   FileNodeKind = "file"
)

type FileNode struct {
	ID           uuid.UUID     `json:"id"`
	ProjectID    uuid.UUID     `json:"space_id"`
	ParentID     *uuid.UUID    `json:"parent_id"`
	Kind         FileNodeKind  `json:"kind"`
	Name         string        `json:"name"`
	MimeType     *string       `json:"mime_type"`
	SizeBytes    *int64        `json:"size_bytes"`
	StorageKey   *string       `json:"storage_key"`
	ChecksumSHA  *string       `json:"checksum_sha256"`
	CreatedBy    uuid.UUID     `json:"created_by"`
	CreatedAt    time.Time     `json:"created_at"`
	UpdatedAt    time.Time     `json:"updated_at"`
	DeletedAt    *time.Time    `json:"deleted_at"`
}

// ListAllFileNodes returns all non-deleted nodes in a project.
// Intended for building a client-side tree.
func (s *Store) ListAllFileNodes(ctx context.Context, projectID uuid.UUID) ([]FileNode, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT id, space_id, parent_id, kind, name, mime_type, size_bytes, storage_key,
		       checksum_sha256, created_by, created_at, updated_at, deleted_at
		FROM file_nodes
		WHERE space_id = $1
		  AND deleted_at IS NULL
		ORDER BY kind ASC, name ASC
	`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []FileNode
	for rows.Next() {
		var n FileNode
		if err := rows.Scan(
			&n.ID, &n.ProjectID, &n.ParentID, &n.Kind, &n.Name, &n.MimeType, &n.SizeBytes,
			&n.StorageKey, &n.ChecksumSHA, &n.CreatedBy, &n.CreatedAt, &n.UpdatedAt, &n.DeletedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

func (s *Store) ListFileNodes(ctx context.Context, projectID uuid.UUID, parentID *uuid.UUID, q string, includeDeleted bool) ([]FileNode, error) {
	// Simple name search. If parentID is nil, list root.
	// If q is non-empty, it searches either within parent scope (if parentID != nil) or whole project.
	rows, err := s.Pool.Query(ctx, `
		SELECT id, space_id, parent_id, kind, name, mime_type, size_bytes, storage_key,
		       checksum_sha256, created_by, created_at, updated_at, deleted_at
		FROM file_nodes
		WHERE space_id = $1
		  AND ( ($2::uuid IS NULL AND parent_id IS NULL) OR parent_id = $2 )
		  AND ($3 = '' OR name ILIKE '%' || $3 || '%')
		  AND ($4::bool OR deleted_at IS NULL)
		ORDER BY kind ASC, name ASC
	`, projectID, parentID, q, includeDeleted)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []FileNode
	for rows.Next() {
		var n FileNode
		if err := rows.Scan(
			&n.ID, &n.ProjectID, &n.ParentID, &n.Kind, &n.Name, &n.MimeType, &n.SizeBytes,
			&n.StorageKey, &n.ChecksumSHA, &n.CreatedBy, &n.CreatedAt, &n.UpdatedAt, &n.DeletedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

func (s *Store) SearchFileNodesInProject(ctx context.Context, projectID uuid.UUID, q string) ([]FileNode, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT id, space_id, parent_id, kind, name, mime_type, size_bytes, storage_key,
		       checksum_sha256, created_by, created_at, updated_at, deleted_at
		FROM file_nodes
		WHERE space_id = $1
		  AND deleted_at IS NULL
		  AND name ILIKE '%' || $2 || '%'
		ORDER BY kind ASC, name ASC
		LIMIT 200
	`, projectID, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []FileNode
	for rows.Next() {
		var n FileNode
		if err := rows.Scan(
			&n.ID, &n.ProjectID, &n.ParentID, &n.Kind, &n.Name, &n.MimeType, &n.SizeBytes,
			&n.StorageKey, &n.ChecksumSHA, &n.CreatedBy, &n.CreatedAt, &n.UpdatedAt, &n.DeletedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

func (s *Store) CreateFolderNode(ctx context.Context, projectID uuid.UUID, parentID *uuid.UUID, name string, createdBy uuid.UUID) (FileNode, error) {
	var n FileNode
	err := s.Pool.QueryRow(ctx, `
		INSERT INTO file_nodes (space_id, parent_id, kind, name, created_by)
		VALUES ($1, $2, 'folder', $3, $4)
		RETURNING id, space_id, parent_id, kind, name, mime_type, size_bytes, storage_key,
		          checksum_sha256, created_by, created_at, updated_at, deleted_at
	`, projectID, parentID, name, createdBy).Scan(
		&n.ID, &n.ProjectID, &n.ParentID, &n.Kind, &n.Name, &n.MimeType, &n.SizeBytes,
		&n.StorageKey, &n.ChecksumSHA, &n.CreatedBy, &n.CreatedAt, &n.UpdatedAt, &n.DeletedAt,
	)
	return n, err
}

func (s *Store) CreateFileNode(ctx context.Context, projectID uuid.UUID, parentID *uuid.UUID, name string, mimeType *string, sizeBytes *int64, storageKey string, createdBy uuid.UUID) (FileNode, error) {
	var n FileNode
	err := s.Pool.QueryRow(ctx, `
		INSERT INTO file_nodes (space_id, parent_id, kind, name, mime_type, size_bytes, storage_key, created_by)
		VALUES ($1, $2, 'file', $3, $4, $5, $6, $7)
		RETURNING id, space_id, parent_id, kind, name, mime_type, size_bytes, storage_key,
		          checksum_sha256, created_by, created_at, updated_at, deleted_at
	`, projectID, parentID, name, mimeType, sizeBytes, storageKey, createdBy).Scan(
		&n.ID, &n.ProjectID, &n.ParentID, &n.Kind, &n.Name, &n.MimeType, &n.SizeBytes,
		&n.StorageKey, &n.ChecksumSHA, &n.CreatedBy, &n.CreatedAt, &n.UpdatedAt, &n.DeletedAt,
	)
	return n, err
}

func (s *Store) RenameFileNode(ctx context.Context, projectID, nodeID uuid.UUID, name string) (bool, error) {
	tag, err := s.Pool.Exec(ctx, `
		UPDATE file_nodes SET name = $3, updated_at = now()
		WHERE id = $1 AND space_id = $2 AND deleted_at IS NULL
	`, nodeID, projectID, name)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

func (s *Store) MoveFileNode(ctx context.Context, projectID, nodeID uuid.UUID, parentID *uuid.UUID) (bool, error) {
	tag, err := s.Pool.Exec(ctx, `
		UPDATE file_nodes SET parent_id = $3, updated_at = now()
		WHERE id = $1 AND space_id = $2 AND deleted_at IS NULL
	`, nodeID, projectID, parentID)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

func (s *Store) SoftDeleteFileNode(ctx context.Context, projectID, nodeID uuid.UUID) (bool, error) {
	tag, err := s.Pool.Exec(ctx, `
		UPDATE file_nodes SET deleted_at = now(), updated_at = now()
		WHERE id = $1 AND space_id = $2 AND deleted_at IS NULL
	`, nodeID, projectID)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

func (s *Store) FileNodeByID(ctx context.Context, projectID, nodeID uuid.UUID) (FileNode, error) {
	var n FileNode
	err := s.Pool.QueryRow(ctx, `
		SELECT id, space_id, parent_id, kind, name, mime_type, size_bytes, storage_key,
		       checksum_sha256, created_by, created_at, updated_at, deleted_at
		FROM file_nodes
		WHERE id = $1 AND space_id = $2
	`, nodeID, projectID).Scan(
		&n.ID, &n.ProjectID, &n.ParentID, &n.Kind, &n.Name, &n.MimeType, &n.SizeBytes,
		&n.StorageKey, &n.ChecksumSHA, &n.CreatedBy, &n.CreatedAt, &n.UpdatedAt, &n.DeletedAt,
	)
	return n, err
}

const deliverablesFolderName = "Deliverables"

// EnsureDeliverablesFolder returns the space root folder named "Deliverables", creating it if missing.
func (s *Store) EnsureDeliverablesFolder(ctx context.Context, spaceID, createdBy uuid.UUID) (FileNode, error) {
	nodes, err := s.ListFileNodes(ctx, spaceID, nil, "", false)
	if err != nil {
		return FileNode{}, err
	}
	for _, n := range nodes {
		if n.Kind == FileNodeFolder && strings.EqualFold(n.Name, deliverablesFolderName) {
			return n, nil
		}
	}
	return s.CreateFolderNode(ctx, spaceID, nil, deliverablesFolderName, createdBy)
}

// FileIsUnderFolder returns true if nodeID (a file) is a descendant of ancestorFolderID within the space.
func (s *Store) FileIsUnderFolder(ctx context.Context, spaceID, nodeID, ancestorFolderID uuid.UUID) (bool, error) {
	n, err := s.FileNodeByID(ctx, spaceID, nodeID)
	if err != nil {
		return false, err
	}
	if n.Kind != FileNodeFile || n.DeletedAt != nil {
		return false, nil
	}
	cur := n
	for i := 0; i < 128; i++ {
		if cur.ParentID == nil {
			return false, nil
		}
		if *cur.ParentID == ancestorFolderID {
			return true, nil
		}
		cur, err = s.FileNodeByID(ctx, spaceID, *cur.ParentID)
		if err != nil {
			return false, err
		}
	}
	return false, errors.New("file tree depth exceeded")
}

func (s *Store) UpdateFileNodeSizeBytes(ctx context.Context, projectID, nodeID uuid.UUID, sizeBytes int64) error {
	tag, err := s.Pool.Exec(ctx, `
		UPDATE file_nodes SET size_bytes = $3, updated_at = now()
		WHERE id = $1 AND space_id = $2 AND kind = 'file' AND deleted_at IS NULL
	`, nodeID, projectID, sizeBytes)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

