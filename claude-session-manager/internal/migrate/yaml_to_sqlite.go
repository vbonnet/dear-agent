package migrate

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/vbonnet/ai-tools/claude-session-manager/internal/db"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/manifest"
)

// MigrationResult tracks statistics from a migration run
type MigrationResult struct {
	TotalFiles      int
	SuccessCount    int
	ErrorCount      int
	SkippedCount    int
	SessionsMigrated int
	Errors          []MigrationError
}

// MigrationError captures details about a failed migration
type MigrationError struct {
	FilePath string
	Error    error
}

// ReadYAMLManifests scans a directory for *.yaml files and parses them into Manifest structs
func ReadYAMLManifests(dir string) ([]*manifest.Manifest, error) {
	if dir == "" {
		return nil, fmt.Errorf("directory cannot be empty")
	}

	// Expand home directory if needed
	if strings.HasPrefix(dir, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to get home directory: %w", err)
		}
		dir = filepath.Join(home, dir[2:])
	}

	// Check if directory exists
	info, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			// Directory doesn't exist - return empty list instead of error
			return []*manifest.Manifest{}, nil
		}
		return nil, fmt.Errorf("failed to stat directory: %w", err)
	}

	if !info.IsDir() {
		return nil, fmt.Errorf("path is not a directory: %s", dir)
	}

	// Find all .yaml files
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory: %w", err)
	}

	var manifests []*manifest.Manifest
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if !strings.HasSuffix(name, ".yaml") && !strings.HasSuffix(name, ".yml") {
			continue
		}

		filePath := filepath.Join(dir, name)
		m, err := readYAMLManifest(filePath)
		if err != nil {
			// Log error but continue with other files
			fmt.Fprintf(os.Stderr, "Warning: failed to read %s: %v\n", filePath, err)
			continue
		}

		manifests = append(manifests, m)
	}

	return manifests, nil
}

// readYAMLManifest reads and parses a single YAML manifest file
func readYAMLManifest(filePath string) (*manifest.Manifest, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	var m manifest.Manifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}

	// Validate required fields
	if m.SessionID == "" {
		return nil, fmt.Errorf("session_id is required")
	}

	if m.Name == "" {
		return nil, fmt.Errorf("name is required")
	}

	// Set defaults for optional fields
	if m.SchemaVersion == "" {
		m.SchemaVersion = "2.0"
	}

	if m.Lifecycle == "" {
		m.Lifecycle = "" // Empty string means active/stopped (not archived)
	}

	return &m, nil
}

// MigrateManifest imports a single manifest into the SQLite database
func MigrateManifest(database *db.DB, m *manifest.Manifest) error {
	if database == nil {
		return fmt.Errorf("database cannot be nil")
	}

	if m == nil {
		return fmt.Errorf("manifest cannot be nil")
	}

	// Check if session already exists
	existing, err := database.GetSession(m.SessionID)
	if err == nil && existing != nil {
		return fmt.Errorf("session already exists: %s", m.SessionID)
	}

	// Insert the session
	if err := database.CreateSession(m); err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}

	return nil
}

// MigrateAll imports all manifests into the SQLite database
func MigrateAll(database *db.DB, manifests []*manifest.Manifest, dryRun bool) (*MigrationResult, error) {
	if database == nil {
		return nil, fmt.Errorf("database cannot be nil")
	}

	result := &MigrationResult{
		TotalFiles: len(manifests),
	}

	for _, m := range manifests {
		if dryRun {
			// In dry-run mode, just validate and count
			if err := validateManifest(m); err != nil {
				result.ErrorCount++
				result.Errors = append(result.Errors, MigrationError{
					FilePath: m.SessionID,
					Error:    fmt.Errorf("validation failed: %w", err),
				})
				continue
			}
			result.SuccessCount++
			result.SessionsMigrated++
			continue
		}

		// Check if already exists
		existing, err := database.GetSession(m.SessionID)
		if err == nil && existing != nil {
			result.SkippedCount++
			continue
		}

		// Attempt migration
		if err := MigrateManifest(database, m); err != nil {
			result.ErrorCount++
			result.Errors = append(result.Errors, MigrationError{
				FilePath: m.SessionID,
				Error:    err,
			})
			continue
		}

		result.SuccessCount++
		result.SessionsMigrated++
	}

	return result, nil
}

// validateManifest checks that a manifest has all required fields
func validateManifest(m *manifest.Manifest) error {
	if m == nil {
		return fmt.Errorf("manifest cannot be nil")
	}

	if m.SessionID == "" {
		return fmt.Errorf("session_id is required")
	}

	if m.Name == "" {
		return fmt.Errorf("name is required")
	}

	if m.CreatedAt.IsZero() {
		return fmt.Errorf("created_at is required")
	}

	if m.UpdatedAt.IsZero() {
		return fmt.Errorf("updated_at is required")
	}

	return nil
}
