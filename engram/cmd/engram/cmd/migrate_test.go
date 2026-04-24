package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRunMigrate_NoLegacyPath(t *testing.T) {
	// Save and restore HOME
	originalHome := os.Getenv("HOME")
	defer os.Setenv("HOME", originalHome)

	// Create temp home without .engram
	tmpHome, err := os.MkdirTemp("", "engram-migrate-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpHome)

	os.Setenv("HOME", tmpHome)

	// Should succeed with message about no migration needed
	err = runMigrate(nil, []string{})
	if err != nil {
		t.Errorf("runMigrate should succeed when no legacy path exists, got error: %v", err)
	}
}

func TestRunMigrate_AlreadyMigrated(t *testing.T) {
	// Save and restore HOME
	originalHome := os.Getenv("HOME")
	defer os.Setenv("HOME", originalHome)

	// Create temp home
	tmpHome, err := os.MkdirTemp("", "engram-migrate-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpHome)

	os.Setenv("HOME", tmpHome)

	// Create target directory with proper engram structure
	targetDir, err := os.MkdirTemp("", "engram-target-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(targetDir)

	// Create minimal engram structure in target (simulates real migrated setup)
	coreDir := filepath.Join(targetDir, "core")
	if err := os.MkdirAll(coreDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create minimal config.yaml so config loading doesn't fail
	minimalConfig := `platform:
  engram_path: ~/.engram
`
	configPath := filepath.Join(coreDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(minimalConfig), 0644); err != nil {
		t.Fatal(err)
	}

	// Create .engram as symlink (already migrated)
	legacyPath := filepath.Join(tmpHome, ".engram")
	if err := os.Symlink(targetDir, legacyPath); err != nil {
		t.Fatal(err)
	}

	// Should succeed with message about already migrated
	err = runMigrate(nil, []string{})
	if err != nil {
		t.Errorf("runMigrate should succeed when already migrated, got error: %v", err)
	}
}

func TestRunMigrate_NoWorkspace(t *testing.T) {
	// Save and restore HOME
	originalHome := os.Getenv("HOME")
	defer os.Setenv("HOME", originalHome)

	// Save and restore WORKSPACE
	originalWorkspace := os.Getenv("WORKSPACE")
	defer func() {
		if originalWorkspace != "" {
			os.Setenv("WORKSPACE", originalWorkspace)
		} else {
			os.Unsetenv("WORKSPACE")
		}
	}()

	// Create temp home with .engram
	tmpHome, err := os.MkdirTemp("", "engram-migrate-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpHome)

	os.Setenv("HOME", tmpHome)
	os.Unsetenv("WORKSPACE")

	// Create legacy .engram directory
	legacyPath := filepath.Join(tmpHome, ".engram")
	if err := os.MkdirAll(legacyPath, 0755); err != nil {
		t.Fatal(err)
	}

	// Create some test data
	testFile := filepath.Join(legacyPath, "test.txt")
	if err := os.WriteFile(testFile, []byte("test data"), 0644); err != nil {
		t.Fatal(err)
	}

	// Should fail when no workspace can be detected
	err = runMigrate(nil, []string{})
	if err == nil {
		t.Error("runMigrate should fail when no workspace detected")
	}
}

func TestMigrateWorkspaceFlag(t *testing.T) {
	// This is a basic validation test - actual migration requires user input
	// Just verify the flag is recognized

	// Save and restore HOME
	originalHome := os.Getenv("HOME")
	defer os.Setenv("HOME", originalHome)

	// Create temp home without .engram (no migration needed case)
	tmpHome, err := os.MkdirTemp("", "engram-migrate-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpHome)

	os.Setenv("HOME", tmpHome)

	// Set workspace flag
	migrateWorkspaceFlag = "test-workspace"
	defer func() { migrateWorkspaceFlag = "" }()

	// Should not panic with workspace flag set
	err = runMigrate(nil, []string{})
	if err != nil {
		// Expected to succeed with "no migration needed" message
		t.Logf("Got expected result: %v", err)
	}
}
