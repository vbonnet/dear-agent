// Package testutil provides test helpers for platform package testing.
// This file is part of B4.2 (Platform Tests) sub-project.
package testutil

import (
	"os"
	"path/filepath"
	"testing"
)

// SetupTestConfig creates a minimal test configuration directory
func SetupTestConfig(t *testing.T, tmpdir string) {
	t.Helper()

	// Create config directory structure (.engram/core/)
	configDir := filepath.Join(tmpdir, ".engram", "core")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}

	// Create minimal config file
	configContent := `platform:
  agent: "test-agent"
  engram_path: ""
  token_budget: 1000
telemetry:
  enabled: false
  path: ""
plugins:
  paths: []
  disabled: []
`
	configPath := filepath.Join(configDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	// Set HOME to tmpdir for config discovery
	t.Setenv("HOME", tmpdir)
}

// SetupTestEngramPath creates a test engram directory structure
func SetupTestEngramPath(t *testing.T, tmpdir string) string {
	t.Helper()

	engramPath := filepath.Join(tmpdir, "test-engrams")
	if err := os.MkdirAll(engramPath, 0755); err != nil {
		t.Fatalf("failed to create engram path: %v", err)
	}

	// Create sample engram file
	sampleEngram := filepath.Join(engramPath, "sample.md")
	content := `# Sample Engram

This is a test engram for platform testing.
`
	if err := os.WriteFile(sampleEngram, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write sample engram: %v", err)
	}

	return engramPath
}
