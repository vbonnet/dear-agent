package devlog

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
)

func TestStatusCommand_Exists(t *testing.T) {
	// Verify status command was registered
	found := false
	for _, cmd := range rootCmd.Commands() {
		if cmd.Name() == "status" {
			found = true
			break
		}
	}

	if !found {
		t.Error("status command not registered with root command")
	}
}

func TestStatusCommand_Help(t *testing.T) {
	// Verify status command has help text
	var statusCmd *cobra.Command
	for _, cmd := range rootCmd.Commands() {
		if cmd.Name() == "status" {
			statusCmd = cmd
			break
		}
	}

	if statusCmd == nil {
		t.Fatal("status command not found")
	}

	if statusCmd.Short == "" {
		t.Error("status command missing short description")
	}

	if statusCmd.Long == "" {
		t.Error("status command missing long description")
	}
}

func TestStatusCommand_NoConfig(t *testing.T) {
	// Test status command fails gracefully when no config exists
	tmpDir := t.TempDir()
	t.Chdir(tmpDir)

	err := runStatus(statusCmd, []string{})
	if err == nil {
		t.Error("runStatus() should error when no config file exists")
	}
}

func TestStatusCommand_WithConfig_NoRepos(t *testing.T) {
	// Test status command with config but no cloned repos
	tmpDir := t.TempDir()
	t.Chdir(tmpDir)

	// Create minimal valid config
	configDir := filepath.Join(tmpDir, ".devlog")
	if err := os.Mkdir(configDir, 0755); err != nil {
		t.Fatal(err)
	}

	configYAML := `name: test-workspace
description: Test workspace for status command
repos:
  - name: test-repo
    url: https://github.com/test/repo.git
    type: bare
    worktrees:
      - name: main
        branch: main
`
	configPath := filepath.Join(configDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(configYAML), 0644); err != nil {
		t.Fatal(err)
	}

	// Run status (should succeed even though repos don't exist)
	err := runStatus(statusCmd, []string{})
	if err != nil {
		t.Errorf("runStatus() error = %v, want nil", err)
	}
}

func TestStatusCommand_WithBareRepo(t *testing.T) {
	// Test status command with a fake bare repo structure
	tmpDir := t.TempDir()
	t.Chdir(tmpDir)

	// Create config
	configDir := filepath.Join(tmpDir, ".devlog")
	if err := os.Mkdir(configDir, 0755); err != nil {
		t.Fatal(err)
	}

	configYAML := `name: test-workspace
repos:
  - name: test-repo
    url: https://github.com/test/repo.git
    type: bare
    worktrees:
      - name: main
        branch: main
`
	configPath := filepath.Join(configDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(configYAML), 0644); err != nil {
		t.Fatal(err)
	}

	// Create fake bare repo structure (HEAD, objects/, refs/)
	repoPath := filepath.Join(tmpDir, "test-repo")
	if err := os.MkdirAll(repoPath, 0755); err != nil {
		t.Fatal(err)
	}

	headPath := filepath.Join(repoPath, "HEAD")
	if err := os.WriteFile(headPath, []byte("ref: refs/heads/main\n"), 0644); err != nil {
		t.Fatal(err)
	}

	objectsPath := filepath.Join(repoPath, "objects")
	if err := os.Mkdir(objectsPath, 0755); err != nil {
		t.Fatal(err)
	}

	refsPath := filepath.Join(repoPath, "refs")
	if err := os.Mkdir(refsPath, 0755); err != nil {
		t.Fatal(err)
	}

	// Run status (repo exists but no worktrees, so ListWorktrees will fail)
	// This tests error handling when git commands fail
	err := runStatus(statusCmd, []string{})

	// Should succeed even if ListWorktrees fails
	// (status shows repos that exist, doesn't require git to work perfectly)
	if err != nil {
		t.Errorf("runStatus() error = %v, want nil (should handle git errors gracefully)", err)
	}
}

func TestStatusCommand_MinimalWorkspace(t *testing.T) {
	// Test status with minimal workspace (one repo, no worktrees)
	tmpDir := t.TempDir()
	t.Chdir(tmpDir)

	// Create minimal config with one repo, no worktrees
	configDir := filepath.Join(tmpDir, ".devlog")
	if err := os.Mkdir(configDir, 0755); err != nil {
		t.Fatal(err)
	}

	configYAML := `name: minimal-workspace
description: Workspace with one repo, no worktrees
repos:
  - name: test-repo
    url: https://github.com/test/repo.git
    type: bare
`
	configPath := filepath.Join(configDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(configYAML), 0644); err != nil {
		t.Fatal(err)
	}

	// Run status (should succeed with 1 repo, 0 worktrees)
	err := runStatus(statusCmd, []string{})
	if err != nil {
		t.Errorf("runStatus() error = %v, want nil for minimal workspace", err)
	}
}
