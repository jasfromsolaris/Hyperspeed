package store

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type TaskDeliverableFile struct {
	TaskID      uuid.UUID `json:"task_id"`
	FileNodeID  uuid.UUID `json:"file_node_id"`
	AddedBy     uuid.UUID `json:"added_by"`
	CreatedAt   time.Time `json:"created_at"`
	FileName    string    `json:"file_name"`
	MimeType    *string   `json:"mime_type,omitempty"`
	SizeBytes   *int64    `json:"size_bytes,omitempty"`
}

func (s *Store) ListTaskDeliverableFiles(ctx context.Context, spaceID, taskID uuid.UUID) ([]TaskDeliverableFile, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT d.task_id, d.file_node_id, d.added_by, d.created_at,
		       fn.name, fn.mime_type, fn.size_bytes
		FROM task_deliverable_files d
		JOIN file_nodes fn ON fn.id = d.file_node_id AND fn.space_id = $1
		WHERE d.task_id = $2 AND fn.deleted_at IS NULL
		ORDER BY d.created_at ASC
	`, spaceID, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TaskDeliverableFile
	for rows.Next() {
		var r TaskDeliverableFile
		if err := rows.Scan(&r.TaskID, &r.FileNodeID, &r.AddedBy, &r.CreatedAt, &r.FileName, &r.MimeType, &r.SizeBytes); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) LinkTaskDeliverableFile(ctx context.Context, spaceID, taskID, fileNodeID, addedBy uuid.UUID) error {
	_, err := s.Pool.Exec(ctx, `
		INSERT INTO task_deliverable_files (task_id, file_node_id, added_by)
		VALUES ($1, $2, $3)
		ON CONFLICT (task_id, file_node_id) DO NOTHING
	`, taskID, fileNodeID, addedBy)
	return err
}

func (s *Store) UnlinkTaskDeliverableFile(ctx context.Context, taskID, fileNodeID uuid.UUID) (bool, error) {
	tag, err := s.Pool.Exec(ctx, `
		DELETE FROM task_deliverable_files WHERE task_id = $1 AND file_node_id = $2
	`, taskID, fileNodeID)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}
