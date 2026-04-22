package common

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// Helper to create a temporary git repo for testing
func setupGitRepo(t *testing.T) (string, func()) {
	t.Helper()

	tmpDir := t.TempDir()
	t.Chdir(tmpDir)

	cmd := exec.Command("git", "init")
	cmd.Dir = tmpDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to init git repo: %v", err)
	}

	gitConfig := exec.Command("git", "config", "user.name", "Test User")
	gitConfig.Dir = tmpDir
	gitConfig.Run()
	gitEmail := exec.Command("git", "config", "user.email", "test@example.com")
	gitEmail.Dir = tmpDir
	gitEmail.Run()

	cleanup := func() {}

	return tmpDir, cleanup
}

func TestScenarioFileCount(t *testing.T) {
	tests := []struct {
		scenario string
		want     int
	}{
		{"empty", 0},
		{"small", 10},
		{"medium", 50},
		{"unknown", 10}, // defaults to small
	}

	for _, tt := range tests {
		t.Run(tt.scenario, func(t *testing.T) {
			got := ScenarioFileCount(tt.scenario)
			if got != tt.want {
				t.Errorf("ScenarioFileCount(%q) = %d, want %d", tt.scenario, got, tt.want)
			}
		})
	}
}

func TestStageTestFiles_Empty(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	_, cleanup := setupGitRepo(t)
	defer cleanup()

	// Stage empty scenario (0 files)
	if err := StageTestFiles("empty"); err != nil {
		t.Fatalf("StageTestFiles(empty) failed: %v", err)
	}

	// Verify no directory created
	if _, err := os.Stat(stagingDir); err == nil {
		t.Errorf("staging directory should not exist for empty scenario")
	}
}

func TestStageTestFiles_Small(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	_, cleanup := setupGitRepo(t)
	defer cleanup()

	// Stage small scenario (10 files)
	if err := StageTestFiles("small"); err != nil {
		t.Fatalf("StageTestFiles(small) failed: %v", err)
	}
	defer UnstageTestFiles()

	// Verify directory exists
	if _, err := os.Stat(stagingDir); os.IsNotExist(err) {
		t.Errorf("staging directory should exist")
	}

	// Verify file count
	files, err := filepath.Glob(filepath.Join(stagingDir, "*.txt"))
	if err != nil {
		t.Fatalf("failed to glob files: %v", err)
	}
	if len(files) != 10 {
		t.Errorf("expected 10 files, got %d", len(files))
	}
}

func TestUnstageTestFiles(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	_, cleanup := setupGitRepo(t)
	defer cleanup()

	// Stage files
	if err := StageTestFiles("small"); err != nil {
		t.Fatalf("StageTestFiles failed: %v", err)
	}

	// Unstage
	if err := UnstageTestFiles(); err != nil {
		t.Fatalf("UnstageTestFiles failed: %v", err)
	}

	// Verify directory removed
	if _, err := os.Stat(stagingDir); err == nil {
		t.Errorf("staging directory should be removed")
	}
}
