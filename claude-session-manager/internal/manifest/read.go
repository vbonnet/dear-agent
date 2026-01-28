package manifest

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Read reads and parses a manifest file
// Automatically migrates v1 manifests to v2 on first read
func Read(path string) (*Manifest, error) {
	// Detect version
	version, err := detectVersion(path)
	if err != nil {
		return nil, fmt.Errorf("failed to detect manifest version: %w", err)
	}

	// Migrate if needed (migration acquires its own lock)
	if version == "1.0" {
		if err := MigrateV1ToV2(path); err != nil {
			return nil, fmt.Errorf("migration failed: %w", err)
		}
	}

	// Read file (now guaranteed to be v2)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read manifest: %w", err)
	}

	// Parse YAML
	var m Manifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("failed to parse manifest: %w", err)
	}

	// Validate (v2 validation)
	if err := m.Validate(); err != nil {
		return nil, err
	}

	return &m, nil
}

// List returns all manifests in directory, including archived sessions
// Scans both main directory and .archive-old-format/ subdirectory
func List(dir string) ([]*Manifest, error) {
	var allManifests []*Manifest

	// Scan main directory
	mainManifests, err := scanDirectory(dir)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	allManifests = append(allManifests, mainManifests...)

	// Scan archive directory (if it exists)
	archiveDir := filepath.Join(dir, ".archive-old-format")
	if _, err := os.Stat(archiveDir); err == nil {
		archiveManifests, err := scanDirectory(archiveDir)
		if err == nil {
			allManifests = append(allManifests, archiveManifests...)
		}
		// Silently ignore archive directory scan errors
	}

	return allManifests, nil
}

// scanDirectory scans a single directory for session manifests
// Uses filepath.Glob to find all manifest.yaml files in immediate subdirectories
func scanDirectory(dir string) ([]*Manifest, error) {
	// Find all manifest.yaml files in immediate subdirectories
	pattern := filepath.Join(dir, "*/manifest.yaml")
	manifestPaths, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("failed to find manifests: %w", err)
	}

	var manifests []*Manifest
	for _, manifestPath := range manifestPaths {
		m, err := Read(manifestPath)
		if err != nil {
			// Skip invalid manifests, continue processing
			continue
		}

		manifests = append(manifests, m)
	}

	return manifests, nil
}

// ReadAll is an alias for List
func ReadAll(dir string) ([]*Manifest, error) {
	return List(dir)
}
