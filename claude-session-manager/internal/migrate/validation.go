package migrate

import (
	"fmt"
	"reflect"

	"github.com/vbonnet/ai-tools/claude-session-manager/internal/manifest"
)

// ValidateMigration compares a YAML-sourced manifest with its SQLite counterpart
// to ensure all fields migrated correctly
func ValidateMigration(yamlManifest *manifest.Manifest, sqliteManifest *manifest.Manifest) error {
	if yamlManifest == nil {
		return fmt.Errorf("yaml manifest cannot be nil")
	}

	if sqliteManifest == nil {
		return fmt.Errorf("sqlite manifest cannot be nil")
	}

	// Compare primary fields
	if yamlManifest.SessionID != sqliteManifest.SessionID {
		return fmt.Errorf("session_id mismatch: yaml=%s, sqlite=%s",
			yamlManifest.SessionID, sqliteManifest.SessionID)
	}

	if yamlManifest.Name != sqliteManifest.Name {
		return fmt.Errorf("name mismatch: yaml=%s, sqlite=%s",
			yamlManifest.Name, sqliteManifest.Name)
	}

	if yamlManifest.SchemaVersion != sqliteManifest.SchemaVersion {
		return fmt.Errorf("schema_version mismatch: yaml=%s, sqlite=%s",
			yamlManifest.SchemaVersion, sqliteManifest.SchemaVersion)
	}

	// Compare timestamps (use Unix() for reliable comparison, ignoring nanoseconds)
	if yamlManifest.CreatedAt.Unix() != sqliteManifest.CreatedAt.Unix() {
		return fmt.Errorf("created_at mismatch: yaml=%s, sqlite=%s",
			yamlManifest.CreatedAt, sqliteManifest.CreatedAt)
	}

	if yamlManifest.UpdatedAt.Unix() != sqliteManifest.UpdatedAt.Unix() {
		return fmt.Errorf("updated_at mismatch: yaml=%s, sqlite=%s",
			yamlManifest.UpdatedAt, sqliteManifest.UpdatedAt)
	}

	// Compare lifecycle
	if yamlManifest.Lifecycle != sqliteManifest.Lifecycle {
		return fmt.Errorf("lifecycle mismatch: yaml=%s, sqlite=%s",
			yamlManifest.Lifecycle, sqliteManifest.Lifecycle)
	}

	// Compare agent
	if yamlManifest.Agent != sqliteManifest.Agent {
		return fmt.Errorf("agent mismatch: yaml=%s, sqlite=%s",
			yamlManifest.Agent, sqliteManifest.Agent)
	}

	// Compare context
	if err := validateContext(yamlManifest.Context, sqliteManifest.Context); err != nil {
		return fmt.Errorf("context mismatch: %w", err)
	}

	// Compare Claude metadata
	if yamlManifest.Claude.UUID != sqliteManifest.Claude.UUID {
		return fmt.Errorf("claude.uuid mismatch: yaml=%s, sqlite=%s",
			yamlManifest.Claude.UUID, sqliteManifest.Claude.UUID)
	}

	// Compare Tmux metadata
	if yamlManifest.Tmux.SessionName != sqliteManifest.Tmux.SessionName {
		return fmt.Errorf("tmux.session_name mismatch: yaml=%s, sqlite=%s",
			yamlManifest.Tmux.SessionName, sqliteManifest.Tmux.SessionName)
	}

	// Compare Engram metadata
	if err := validateEngramMetadata(yamlManifest.EngramMetadata, sqliteManifest.EngramMetadata); err != nil {
		return fmt.Errorf("engram_metadata mismatch: %w", err)
	}

	return nil
}

// validateContext compares Context structs
func validateContext(yaml manifest.Context, sqlite manifest.Context) error {
	if yaml.Project != sqlite.Project {
		return fmt.Errorf("project mismatch: yaml=%s, sqlite=%s", yaml.Project, sqlite.Project)
	}

	if yaml.Purpose != sqlite.Purpose {
		return fmt.Errorf("purpose mismatch: yaml=%s, sqlite=%s", yaml.Purpose, sqlite.Purpose)
	}

	if yaml.Notes != sqlite.Notes {
		return fmt.Errorf("notes mismatch: yaml=%s, sqlite=%s", yaml.Notes, sqlite.Notes)
	}

	// Compare tags (handle nil vs empty slice)
	if !equalStringSlices(yaml.Tags, sqlite.Tags) {
		return fmt.Errorf("tags mismatch: yaml=%v, sqlite=%v", yaml.Tags, sqlite.Tags)
	}

	return nil
}

// validateEngramMetadata compares EngramMetadata structs
func validateEngramMetadata(yaml *manifest.EngramMetadata, sqlite *manifest.EngramMetadata) error {
	// Both nil is OK
	if yaml == nil && sqlite == nil {
		return nil
	}

	// One nil, one not nil is a mismatch
	if (yaml == nil) != (sqlite == nil) {
		return fmt.Errorf("nil mismatch: yaml=%v, sqlite=%v", yaml == nil, sqlite == nil)
	}

	// Both non-nil - compare fields
	if yaml.Enabled != sqlite.Enabled {
		return fmt.Errorf("enabled mismatch: yaml=%v, sqlite=%v", yaml.Enabled, sqlite.Enabled)
	}

	if yaml.Query != sqlite.Query {
		return fmt.Errorf("query mismatch: yaml=%s, sqlite=%s", yaml.Query, sqlite.Query)
	}

	if yaml.Count != sqlite.Count {
		return fmt.Errorf("count mismatch: yaml=%d, sqlite=%d", yaml.Count, sqlite.Count)
	}

	if !equalStringSlices(yaml.EngramIDs, sqlite.EngramIDs) {
		return fmt.Errorf("engram_ids mismatch: yaml=%v, sqlite=%v", yaml.EngramIDs, sqlite.EngramIDs)
	}

	// Compare LoadedAt (allow for slight timestamp differences due to serialization)
	if !yaml.LoadedAt.IsZero() || !sqlite.LoadedAt.IsZero() {
		if yaml.LoadedAt.Unix() != sqlite.LoadedAt.Unix() {
			return fmt.Errorf("loaded_at mismatch: yaml=%s, sqlite=%s",
				yaml.LoadedAt, sqlite.LoadedAt)
		}
	}

	return nil
}

// equalStringSlices compares two string slices for equality
// Treats nil and empty slice as equivalent
func equalStringSlices(a, b []string) bool {
	// Normalize nil to empty slice
	if a == nil {
		a = []string{}
	}
	if b == nil {
		b = []string{}
	}

	return reflect.DeepEqual(a, b)
}
