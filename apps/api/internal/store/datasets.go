package store

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type DatasetFormat string

const (
	DatasetFormatParquet DatasetFormat = "parquet"
	DatasetFormatCSV     DatasetFormat = "csv"
)

type SpaceDataset struct {
	ID                uuid.UUID       `json:"id"`
	OrganizationID    uuid.UUID       `json:"organization_id"`
	SpaceID           uuid.UUID       `json:"space_id"`
	Name              string          `json:"name"`
	Format            DatasetFormat   `json:"format"`
	StorageKey        string          `json:"storage_key"`
	SizeBytes         *int64          `json:"size_bytes"`
	SchemaJSON        json.RawMessage `json:"schema_json"`
	RowCountEstimate  *int64          `json:"row_count_estimate"`
	CreatedBy         uuid.UUID       `json:"created_by"`
	CreatedAt         time.Time       `json:"created_at"`
	UpdatedAt         time.Time       `json:"updated_at"`
}

func (s *Store) CreateDatasetPending(ctx context.Context, orgID, spaceID uuid.UUID, name string, format DatasetFormat, storageKey string, createdBy uuid.UUID) (SpaceDataset, error) {
	var d SpaceDataset
	var schemaBytes []byte
	err := s.Pool.QueryRow(ctx, `
		INSERT INTO space_datasets (organization_id, space_id, name, format, storage_key, created_by)
		VALUES ($1, $2, $3, $4::dataset_format, $5, $6)
		RETURNING id, organization_id, space_id, name, format::text, storage_key, size_bytes, schema_json, row_count_estimate,
		          created_by, created_at, updated_at
	`, orgID, spaceID, name, string(format), storageKey, createdBy).Scan(
		&d.ID, &d.OrganizationID, &d.SpaceID, &d.Name, &d.Format, &d.StorageKey, &d.SizeBytes, &schemaBytes, &d.RowCountEstimate,
		&d.CreatedBy, &d.CreatedAt, &d.UpdatedAt,
	)
	if err != nil {
		return d, err
	}
	if len(schemaBytes) > 0 {
		d.SchemaJSON = append(json.RawMessage(nil), schemaBytes...)
	}
	return d, nil
}

func (s *Store) CompleteDataset(ctx context.Context, spaceID, datasetID uuid.UUID, sizeBytes int64, schemaJSON json.RawMessage, rowCount *int64) (SpaceDataset, error) {
	var d SpaceDataset
	var schemaBytes []byte
	err := s.Pool.QueryRow(ctx, `
		UPDATE space_datasets
		SET size_bytes = $3, schema_json = $4::jsonb, row_count_estimate = $5, updated_at = now()
		WHERE id = $1 AND space_id = $2
		RETURNING id, organization_id, space_id, name, format::text, storage_key, size_bytes, schema_json, row_count_estimate,
		          created_by, created_at, updated_at
	`, datasetID, spaceID, sizeBytes, schemaJSON, rowCount).Scan(
		&d.ID, &d.OrganizationID, &d.SpaceID, &d.Name, &d.Format, &d.StorageKey, &d.SizeBytes, &schemaBytes, &d.RowCountEstimate,
		&d.CreatedBy, &d.CreatedAt, &d.UpdatedAt,
	)
	if err != nil {
		return d, err
	}
	if len(schemaBytes) > 0 {
		d.SchemaJSON = append(json.RawMessage(nil), schemaBytes...)
	}
	return d, nil
}

func (s *Store) DatasetByID(ctx context.Context, spaceID, datasetID uuid.UUID) (SpaceDataset, error) {
	var d SpaceDataset
	var schemaBytes []byte
	err := s.Pool.QueryRow(ctx, `
		SELECT id, organization_id, space_id, name, format::text, storage_key, size_bytes, schema_json, row_count_estimate,
		       created_by, created_at, updated_at
		FROM space_datasets
		WHERE id = $1 AND space_id = $2
	`, datasetID, spaceID).Scan(
		&d.ID, &d.OrganizationID, &d.SpaceID, &d.Name, &d.Format, &d.StorageKey, &d.SizeBytes, &schemaBytes, &d.RowCountEstimate,
		&d.CreatedBy, &d.CreatedAt, &d.UpdatedAt,
	)
	if err != nil {
		return d, err
	}
	if len(schemaBytes) > 0 {
		d.SchemaJSON = append(json.RawMessage(nil), schemaBytes...)
	}
	return d, nil
}

func (s *Store) ListDatasets(ctx context.Context, spaceID uuid.UUID) ([]SpaceDataset, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT id, organization_id, space_id, name, format::text, storage_key, size_bytes, schema_json, row_count_estimate,
		       created_by, created_at, updated_at
		FROM space_datasets
		WHERE space_id = $1
		ORDER BY name ASC
	`, spaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SpaceDataset
	for rows.Next() {
		var d SpaceDataset
		var schemaBytes []byte
		if err := rows.Scan(
			&d.ID, &d.OrganizationID, &d.SpaceID, &d.Name, &d.Format, &d.StorageKey, &d.SizeBytes, &schemaBytes, &d.RowCountEstimate,
			&d.CreatedBy, &d.CreatedAt, &d.UpdatedAt,
		); err != nil {
			return nil, err
		}
		if len(schemaBytes) > 0 {
			d.SchemaJSON = append(json.RawMessage(nil), schemaBytes...)
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (s *Store) DeleteDataset(ctx context.Context, spaceID, datasetID uuid.UUID) error {
	ct, err := s.Pool.Exec(ctx, `DELETE FROM space_datasets WHERE id = $1 AND space_id = $2`, datasetID, spaceID)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (s *Store) LogDatasetQueryInvocation(ctx context.Context, orgID, spaceID, datasetID, userID uuid.UUID, kind string, requestJSON json.RawMessage, rowCount, durationMs int) error {
	_, err := s.Pool.Exec(ctx, `
		INSERT INTO dataset_query_invocations (organization_id, space_id, dataset_id, user_id, kind, request_json, row_count_returned, duration_ms)
		VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7, $8)
	`, orgID, spaceID, datasetID, userID, kind, requestJSON, rowCount, durationMs)
	return err
}
