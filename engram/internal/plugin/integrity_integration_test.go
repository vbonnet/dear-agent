package plugin

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestLoader_IntegrityVerification_ValidPlugin(t *testing.T) {
	tmpDir := t.TempDir()

	// Create plugin directory
	pluginDir := filepath.Join(tmpDir, "test-plugin")
	if err := os.MkdirAll(pluginDir, 0755); err != nil {
		t.Fatalf("Failed to create plugin directory: %v", err)
	}

	// Create plugin executable
	execPath := filepath.Join(pluginDir, "tool")
	execContent := []byte("#!/bin/bash\necho 'hello'")
	if err := os.WriteFile(execPath, execContent, 0755); err != nil {
		t.Fatalf("Failed to create executable: %v", err)
	}

	// Generate integrity for the executable
	integrity, err := GenerateIntegrity(pluginDir, []string{"tool"})
	if err != nil {
		t.Fatalf("Failed to generate integrity: %v", err)
	}

	// Create manifest with integrity section
	manifest := Manifest{
		Name:        "test-plugin",
		Version:     "1.0.0",
		Description: "Test plugin with integrity",
		Pattern:     PatternTool,
		Commands: []Command{
			{
				Name:        "test",
				Description: "Test command",
				Executable:  "tool",
			},
		},
		Integrity: *integrity,
	}

	// Write manifest
	manifestData, err := yaml.Marshal(manifest)
	if err != nil {
		t.Fatalf("Failed to marshal manifest: %v", err)
	}

	manifestPath := filepath.Join(pluginDir, "plugin.yaml")
	if err := os.WriteFile(manifestPath, manifestData, 0644); err != nil {
		t.Fatalf("Failed to write manifest: %v", err)
	}

	// Load plugin (should succeed)
	loader := NewLoader([]string{tmpDir}, nil)
	plugins, err := loader.Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	if len(plugins) != 1 {
		t.Errorf("Load() returned %d plugins, want 1", len(plugins))
	}

	if plugins[0].Manifest.Name != "test-plugin" {
		t.Errorf("Loaded plugin name = %q, want %q", plugins[0].Manifest.Name, "test-plugin")
	}
}

func TestLoader_IntegrityVerification_TamperedPlugin(t *testing.T) {
	tmpDir := t.TempDir()

	// Create plugin directory
	pluginDir := filepath.Join(tmpDir, "test-plugin")
	if err := os.MkdirAll(pluginDir, 0755); err != nil {
		t.Fatalf("Failed to create plugin directory: %v", err)
	}

	// Create plugin executable
	execPath := filepath.Join(pluginDir, "tool")
	execContent := []byte("#!/bin/bash\necho 'hello'")
	if err := os.WriteFile(execPath, execContent, 0755); err != nil {
		t.Fatalf("Failed to create executable: %v", err)
	}

	// Generate integrity for the executable
	integrity, err := GenerateIntegrity(pluginDir, []string{"tool"})
	if err != nil {
		t.Fatalf("Failed to generate integrity: %v", err)
	}

	// Create manifest with integrity section
	manifest := Manifest{
		Name:        "test-plugin",
		Version:     "1.0.0",
		Description: "Test plugin with integrity",
		Pattern:     PatternTool,
		Commands: []Command{
			{
				Name:        "test",
				Description: "Test command",
				Executable:  "tool",
			},
		},
		Integrity: *integrity,
	}

	// Write manifest
	manifestData, err := yaml.Marshal(manifest)
	if err != nil {
		t.Fatalf("Failed to marshal manifest: %v", err)
	}

	manifestPath := filepath.Join(pluginDir, "plugin.yaml")
	if err := os.WriteFile(manifestPath, manifestData, 0644); err != nil {
		t.Fatalf("Failed to write manifest: %v", err)
	}

	// TAMPER with the executable (modify content)
	tamperedContent := []byte("#!/bin/bash\necho 'malicious code'")
	if err := os.WriteFile(execPath, tamperedContent, 0755); err != nil {
		t.Fatalf("Failed to tamper with executable: %v", err)
	}

	// Load plugin (should fail due to integrity check)
	loader := NewLoader([]string{tmpDir}, nil)
	plugins, err := loader.Load()

	// Should successfully load (loader continues past single plugin failures)
	// but the tampered plugin should not be in the list
	if err != nil {
		t.Fatalf("Load() failed unexpectedly: %v", err)
	}

	if len(plugins) != 0 {
		t.Errorf("Load() should skip tampered plugin, got %d plugins", len(plugins))
	}
}

func TestLoader_IntegrityVerification_NoIntegritySection(t *testing.T) {
	tmpDir := t.TempDir()

	// Create plugin directory
	pluginDir := filepath.Join(tmpDir, "legacy-plugin")
	if err := os.MkdirAll(pluginDir, 0755); err != nil {
		t.Fatalf("Failed to create plugin directory: %v", err)
	}

	// Create manifest WITHOUT integrity section (backwards compatibility)
	manifest := Manifest{
		Name:        "legacy-plugin",
		Version:     "1.0.0",
		Description: "Legacy plugin without integrity",
		Pattern:     PatternTool,
	}

	// Write manifest
	manifestData, err := yaml.Marshal(manifest)
	if err != nil {
		t.Fatalf("Failed to marshal manifest: %v", err)
	}

	manifestPath := filepath.Join(pluginDir, "plugin.yaml")
	if err := os.WriteFile(manifestPath, manifestData, 0644); err != nil {
		t.Fatalf("Failed to write manifest: %v", err)
	}

	// Load plugin (should succeed - backwards compatibility)
	loader := NewLoader([]string{tmpDir}, nil)
	plugins, err := loader.Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	if len(plugins) != 1 {
		t.Errorf("Load() returned %d plugins, want 1 (backwards compatibility)", len(plugins))
	}

	if plugins[0].Manifest.Name != "legacy-plugin" {
		t.Errorf("Loaded plugin name = %q, want %q", plugins[0].Manifest.Name, "legacy-plugin")
	}
}

func TestLoader_IntegrityVerification_MissingFile(t *testing.T) {
	tmpDir := t.TempDir()

	// Create plugin directory
	pluginDir := filepath.Join(tmpDir, "test-plugin")
	if err := os.MkdirAll(pluginDir, 0755); err != nil {
		t.Fatalf("Failed to create plugin directory: %v", err)
	}

	// Create manifest with integrity for a file that doesn't exist
	manifest := Manifest{
		Name:        "test-plugin",
		Version:     "1.0.0",
		Description: "Test plugin with missing file",
		Pattern:     PatternTool,
		Integrity: Integrity{
			Algorithm: "sha256",
			Files: map[string]string{
				"nonexistent.txt": "abc123",
			},
		},
	}

	// Write manifest
	manifestData, err := yaml.Marshal(manifest)
	if err != nil {
		t.Fatalf("Failed to marshal manifest: %v", err)
	}

	manifestPath := filepath.Join(pluginDir, "plugin.yaml")
	if err := os.WriteFile(manifestPath, manifestData, 0644); err != nil {
		t.Fatalf("Failed to write manifest: %v", err)
	}

	// Load plugin (should fail - file not found)
	loader := NewLoader([]string{tmpDir}, nil)
	plugins, err := loader.Load()
	if err != nil {
		t.Fatalf("Load() failed unexpectedly: %v", err)
	}

	// Plugin should be skipped
	if len(plugins) != 0 {
		t.Errorf("Load() should skip plugin with missing file, got %d plugins", len(plugins))
	}
}
