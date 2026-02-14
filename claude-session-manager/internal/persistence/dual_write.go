package persistence

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/vbonnet/ai-tools/claude-session-manager/internal/db"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/fileutil"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/manifest"
)

// DualWriter handles dual-write pattern: SQLite (source of truth) + YAML (cache)
// Write pattern: SQLite first, then YAML (fail fast if SQLite fails)
// Read pattern: Always read from SQLite (YAML is ignored for reads)
type DualWriter struct {
	db      *db.DB
	yamlDir string
}

// NewDualWriter creates a new DualWriter instance
func NewDualWriter(database *db.DB, yamlDir string) *DualWriter {
	return &DualWriter{
		db:      database,
		yamlDir: yamlDir,
	}
}

// CreateSession writes a new session to both SQLite and YAML
// SQLite write fails: Return error immediately (don't write YAML)
// YAML write fails after SQLite success: Log warning but continue (SQLite is source of truth)
func (dw *DualWriter) CreateSession(m *manifest.Manifest) error {
	if m == nil {
		return fmt.Errorf("manifest cannot be nil")
	}

	// Step 1: Write to SQLite first (source of truth)
	if err := dw.db.CreateSession(m); err != nil {
		return fmt.Errorf("failed to create session in SQLite: %w", err)
	}

	// Step 2: Write to YAML (cache for backward compatibility)
	// If this fails, we log a warning but don't fail the operation
	// since SQLite is the source of truth
	yamlPath := dw.getYAMLPath(m.SessionID)
	if err := dw.WriteYAML(m, yamlPath); err != nil {
		log.Printf("WARNING: Failed to write YAML cache for session %s: %v (SQLite write succeeded)", m.SessionID, err)
		// Don't return error - SQLite write succeeded, that's what matters
	}

	return nil
}

// UpdateSession updates a session in both SQLite and YAML
// SQLite write fails: Return error immediately (don't write YAML)
// YAML write fails after SQLite success: Log warning but continue (SQLite is source of truth)
func (dw *DualWriter) UpdateSession(m *manifest.Manifest) error {
	if m == nil {
		return fmt.Errorf("manifest cannot be nil")
	}

	// Step 1: Update in SQLite first (source of truth)
	if err := dw.db.UpdateSession(m); err != nil {
		return fmt.Errorf("failed to update session in SQLite: %w", err)
	}

	// Step 2: Update YAML cache
	// If this fails, we log a warning but don't fail the operation
	yamlPath := dw.getYAMLPath(m.SessionID)
	if err := dw.WriteYAML(m, yamlPath); err != nil {
		log.Printf("WARNING: Failed to update YAML cache for session %s: %v (SQLite update succeeded)", m.SessionID, err)
		// Don't return error - SQLite update succeeded, that's what matters
	}

	return nil
}

// DeleteSession deletes a session from both SQLite and YAML
// SQLite delete fails: Return error immediately (don't delete YAML)
// YAML delete fails after SQLite success: Log warning but continue (SQLite is source of truth)
func (dw *DualWriter) DeleteSession(sessionID string) error {
	if sessionID == "" {
		return fmt.Errorf("session_id cannot be empty")
	}

	// Step 1: Delete from SQLite first (source of truth)
	if err := dw.db.DeleteSession(sessionID); err != nil {
		return fmt.Errorf("failed to delete session from SQLite: %w", err)
	}

	// Step 2: Delete YAML cache
	// If this fails, we log a warning but don't fail the operation
	yamlPath := dw.getYAMLPath(sessionID)
	if err := os.RemoveAll(filepath.Dir(yamlPath)); err != nil {
		log.Printf("WARNING: Failed to delete YAML cache for session %s: %v (SQLite delete succeeded)", sessionID, err)
		// Don't return error - SQLite delete succeeded, that's what matters
	}

	return nil
}

// GetSession reads a session from SQLite ONLY (YAML is ignored)
// SQLite is always the source of truth for reads
func (dw *DualWriter) GetSession(sessionID string) (*manifest.Manifest, error) {
	if sessionID == "" {
		return nil, fmt.Errorf("session_id cannot be empty")
	}

	// Read from SQLite only - YAML is just a cache, not used for reads
	return dw.db.GetSession(sessionID)
}

// ListSessions returns all sessions from SQLite ONLY (YAML is ignored)
// SQLite is always the source of truth for reads
func (dw *DualWriter) ListSessions(filter *db.SessionFilter) ([]*manifest.Manifest, error) {
	// Read from SQLite only - YAML is just a cache, not used for reads
	return dw.db.ListSessions(filter)
}

// WriteYAML writes a manifest to a YAML file using atomic write
// This ensures the YAML file is never in a partially written state
func (dw *DualWriter) WriteYAML(m *manifest.Manifest, path string) error {
	if m == nil {
		return fmt.Errorf("manifest cannot be nil")
	}

	// Validate manifest before writing
	if err := m.Validate(); err != nil {
		return fmt.Errorf("invalid manifest: %w", err)
	}

	// Marshal to YAML
	data, err := yaml.Marshal(m)
	if err != nil {
		return fmt.Errorf("failed to marshal manifest to YAML: %w", err)
	}

	// Use atomic write to ensure consistency
	// This writes to a temp file and then atomically renames it
	if err := fileutil.AtomicWrite(path, data, 0600); err != nil {
		return fmt.Errorf("failed to write YAML file: %w", err)
	}

	return nil
}

// ReadYAML reads a manifest from a YAML file
// This is primarily used by the sync tool to rebuild the YAML cache
func (dw *DualWriter) ReadYAML(path string) (*manifest.Manifest, error) {
	// Read file
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read YAML file: %w", err)
	}

	// Parse YAML
	var m manifest.Manifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}

	// Validate
	if err := m.Validate(); err != nil {
		return nil, fmt.Errorf("invalid manifest in YAML file: %w", err)
	}

	return &m, nil
}

// SyncYAMLFromSQLite rebuilds the YAML cache from SQLite database
// This is used when YAML files are corrupted or out of sync
// It reads all sessions from SQLite and writes them to YAML files
func (dw *DualWriter) SyncYAMLFromSQLite() (int, error) {
	// Get all sessions from SQLite (source of truth)
	sessions, err := dw.db.ListSessions(nil)
	if err != nil {
		return 0, fmt.Errorf("failed to list sessions from SQLite: %w", err)
	}

	// Write each session to YAML
	syncCount := 0
	for _, session := range sessions {
		yamlPath := dw.getYAMLPath(session.SessionID)
		if err := dw.WriteYAML(session, yamlPath); err != nil {
			log.Printf("WARNING: Failed to sync YAML for session %s: %v", session.SessionID, err)
			// Continue with other sessions
			continue
		}
		syncCount++
	}

	return syncCount, nil
}

// getYAMLPath returns the YAML file path for a session
// Format: ~/.agm/sessions/{session-id}/manifest.yaml
func (dw *DualWriter) getYAMLPath(sessionID string) string {
	return filepath.Join(dw.yamlDir, sessionID, "manifest.yaml")
}
