package dolt

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/vbonnet/dear-agent/agm/internal/artifacts"
)

// ArtifactAdapter implements artifacts.Store using Dolt.
type ArtifactAdapter struct {
	adapter *Adapter
}

// Compile-time check that ArtifactAdapter implements artifacts.Store.
var _ artifacts.Store = (*ArtifactAdapter)(nil)

// NewArtifactAdapter creates an ArtifactAdapter backed by the given Dolt Adapter.
func NewArtifactAdapter(adapter *Adapter) *ArtifactAdapter {
	return &ArtifactAdapter{adapter: adapter}
}

// Store saves artifact metadata.
func (aa *ArtifactAdapter) Store(artifact *artifacts.Artifact) error {
	if artifact == nil {
		return fmt.Errorf("artifact cannot be nil")
	}
	if artifact.ID == "" {
		return fmt.Errorf("artifact id cannot be empty")
	}
	if artifact.SessionID == "" {
		return fmt.Errorf("artifact session_id cannot be empty")
	}

	if err := aa.adapter.ApplyMigrations(); err != nil {
		return fmt.Errorf("failed to apply migrations: %w", err)
	}

	var metadataJSON interface{}
	if artifact.Metadata != nil {
		b, err := json.Marshal(artifact.Metadata)
		if err != nil {
			return fmt.Errorf("failed to marshal artifact metadata: %w", err)
		}
		metadataJSON = string(b)
	}

	query := `
		INSERT INTO agm_artifacts (id, session_id, type, path, size, metadata, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`

	_, err := aa.adapter.conn.Exec(query, //nolint:noctx // TODO(context): plumb ctx through this layer
		artifact.ID,
		artifact.SessionID,
		artifact.Type,
		artifact.Path,
		artifact.Size,
		metadataJSON,
		artifact.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to insert artifact: %w", err)
	}

	return nil
}

// Get retrieves an artifact by ID.
func (aa *ArtifactAdapter) Get(id string) (*artifacts.Artifact, error) {
	if id == "" {
		return nil, fmt.Errorf("artifact id cannot be empty")
	}

	if err := aa.adapter.ApplyMigrations(); err != nil {
		return nil, fmt.Errorf("failed to apply migrations: %w", err)
	}

	query := `
		SELECT id, session_id, type, path, size, metadata, created_at
		FROM agm_artifacts
		WHERE id = ?
	`

	var artifact artifacts.Artifact
	var metadataJSON []byte
	var path sql.NullString
	var size sql.NullInt64

	err := aa.adapter.conn.QueryRow(query, id).Scan( //nolint:noctx // TODO(context): plumb ctx through this layer
		&artifact.ID,
		&artifact.SessionID,
		&artifact.Type,
		&path,
		&size,
		&metadataJSON,
		&artifact.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("artifact not found: %s", id)
		}
		return nil, fmt.Errorf("failed to get artifact: %w", err)
	}

	if path.Valid {
		artifact.Path = path.String
	}
	if size.Valid {
		artifact.Size = size.Int64
	}

	if len(metadataJSON) > 0 {
		if err := json.Unmarshal(metadataJSON, &artifact.Metadata); err != nil {
			return nil, fmt.Errorf("failed to unmarshal artifact metadata: %w", err)
		}
	}

	return &artifact, nil
}

// ListBySession retrieves all artifacts for a session.
func (aa *ArtifactAdapter) ListBySession(sessionID string) ([]*artifacts.Artifact, error) {
	if sessionID == "" {
		return nil, fmt.Errorf("session_id cannot be empty")
	}

	if err := aa.adapter.ApplyMigrations(); err != nil {
		return nil, fmt.Errorf("failed to apply migrations: %w", err)
	}

	query := `
		SELECT id, session_id, type, path, size, metadata, created_at
		FROM agm_artifacts
		WHERE session_id = ?
		ORDER BY created_at ASC
	`

	rows, err := aa.adapter.conn.Query(query, sessionID) //nolint:noctx // TODO(context): plumb ctx through this layer
	if err != nil {
		return nil, fmt.Errorf("failed to query artifacts: %w", err)
	}
	defer rows.Close()

	var results []*artifacts.Artifact
	for rows.Next() {
		var artifact artifacts.Artifact
		var metadataJSON []byte
		var path sql.NullString
		var size sql.NullInt64

		err := rows.Scan(
			&artifact.ID,
			&artifact.SessionID,
			&artifact.Type,
			&path,
			&size,
			&metadataJSON,
			&artifact.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan artifact: %w", err)
		}

		if path.Valid {
			artifact.Path = path.String
		}
		if size.Valid {
			artifact.Size = size.Int64
		}

		if len(metadataJSON) > 0 {
			if err := json.Unmarshal(metadataJSON, &artifact.Metadata); err != nil {
				return nil, fmt.Errorf("failed to unmarshal artifact metadata: %w", err)
			}
		}

		results = append(results, &artifact)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating artifact rows: %w", err)
	}

	return results, nil
}

// Delete removes an artifact by ID.
func (aa *ArtifactAdapter) Delete(id string) error {
	if id == "" {
		return fmt.Errorf("artifact id cannot be empty")
	}

	if err := aa.adapter.ApplyMigrations(); err != nil {
		return fmt.Errorf("failed to apply migrations: %w", err)
	}

	query := `DELETE FROM agm_artifacts WHERE id = ?`

	result, err := aa.adapter.conn.Exec(query, id) //nolint:noctx // TODO(context): plumb ctx through this layer
	if err != nil {
		return fmt.Errorf("failed to delete artifact: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("artifact not found: %s", id)
	}

	return nil
}
