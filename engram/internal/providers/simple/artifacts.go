// Package simple provides simple-related functionality.
package simple

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/vbonnet/dear-agent/engram/internal/consolidation"
	"github.com/vbonnet/dear-agent/internal/telemetry"
)

// Artifact Operations

// StoreArtifact stores binary data as a file.
//
// File path: {storagePath}/_artifacts/{artifactID}
// Artifacts are stored in a separate directory from memories.
func (p *SimpleFileProvider) StoreArtifact(ctx context.Context, artifactID string, data []byte) error {
	start := time.Now()
	var err error
	success := false

	// Emit telemetry on function exit
	defer func() {
		if recorder := consolidation.GetTelemetryRecorder(ctx); recorder != nil {
			eventData := map[string]interface{}{
				"provider":    "simple",
				"artifact_id": artifactID,
				"size_bytes":  len(data),
				"latency_ms":  time.Since(start).Milliseconds(),
				"success":     success,
			}
			if err != nil {
				eventData["error_msg"] = err.Error()
			}
			level := telemetry.LevelInfo
			if err != nil {
				level = telemetry.LevelError
			}
			_ = recorder.Record(consolidation.EventArtifactStored, "", level, eventData)
		}
	}()

	// 1. Validate artifact ID (basic check)
	if artifactID == "" || artifactID == "." || artifactID == ".." {
		err = fmt.Errorf("store artifact: invalid artifact ID")
		return err
	}

	// 2. Construct file path
	artifactPath := filepath.Join(p.storagePath, "_artifacts", artifactID)

	// 3. Create parent directory
	if err = os.MkdirAll(filepath.Dir(artifactPath), 0o700); err != nil {
		return fmt.Errorf("store artifact: create directory: %w", err)
	}

	// 4. Write binary data
	if err = os.WriteFile(artifactPath, data, 0o600); err != nil {
		return fmt.Errorf("store artifact: write file: %w", err)
	}

	success = true
	return nil
}

// GetArtifact retrieves binary data by ID.
//
// Returns ErrNotFound if artifact doesn't exist.
func (p *SimpleFileProvider) GetArtifact(ctx context.Context, artifactID string) ([]byte, error) {
	start := time.Now()
	var err error
	success := false
	var dataSize int

	// Emit telemetry on function exit
	defer func() {
		if recorder := consolidation.GetTelemetryRecorder(ctx); recorder != nil {
			eventData := map[string]interface{}{
				"provider":    "simple",
				"artifact_id": artifactID,
				"size_bytes":  dataSize,
				"latency_ms":  time.Since(start).Milliseconds(),
				"success":     success,
			}
			if err != nil {
				eventData["error_msg"] = err.Error()
			}
			level := telemetry.LevelInfo
			if err != nil {
				level = telemetry.LevelError
			}
			_ = recorder.Record(consolidation.EventArtifactFetched, "", level, eventData)
		}
	}()

	// 1. Validate artifact ID
	if artifactID == "" || artifactID == "." || artifactID == ".." {
		err = fmt.Errorf("get artifact: invalid artifact ID")
		return nil, err
	}

	// 2. Construct file path
	artifactPath := filepath.Join(p.storagePath, "_artifacts", artifactID)

	// 3. Read file
	data, readErr := os.ReadFile(artifactPath)
	if os.IsNotExist(readErr) {
		err = consolidation.ErrNotFound
		return nil, fmt.Errorf("get artifact: %w", err)
	}
	if readErr != nil {
		err = readErr
		return nil, fmt.Errorf("get artifact: read file: %w", err)
	}

	success = true
	dataSize = len(data)
	return data, nil
}

// DeleteArtifact removes an artifact.
//
// Returns ErrNotFound if artifact doesn't exist.
func (p *SimpleFileProvider) DeleteArtifact(ctx context.Context, artifactID string) error {
	start := time.Now()
	var err error
	success := false

	// Emit telemetry on function exit
	defer func() {
		if recorder := consolidation.GetTelemetryRecorder(ctx); recorder != nil {
			eventData := map[string]interface{}{
				"provider":    "simple",
				"artifact_id": artifactID,
				"latency_ms":  time.Since(start).Milliseconds(),
				"success":     success,
			}
			if err != nil {
				eventData["error_msg"] = err.Error()
			}
			level := telemetry.LevelInfo
			if err != nil {
				level = telemetry.LevelError
			}
			_ = recorder.Record(consolidation.EventArtifactDeleted, "", level, eventData)
		}
	}()

	// 1. Validate artifact ID
	if artifactID == "" || artifactID == "." || artifactID == ".." {
		err = fmt.Errorf("delete artifact: invalid artifact ID")
		return err
	}

	// 2. Construct file path
	artifactPath := filepath.Join(p.storagePath, "_artifacts", artifactID)

	// 3. Check file exists
	if _, statErr := os.Stat(artifactPath); os.IsNotExist(statErr) {
		err = consolidation.ErrNotFound
		return fmt.Errorf("delete artifact: %w", err)
	}

	// 4. Delete file
	if err = os.Remove(artifactPath); err != nil {
		return fmt.Errorf("delete artifact: %w", err)
	}

	success = true
	return nil
}
