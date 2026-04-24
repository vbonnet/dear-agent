package devlog

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
)

func TestSyncCommand_Exists(t *testing.T) {
	// Verify sync command was registered
	found := false
	for _, cmd := range rootCmd.Commands() {
		if cmd.Name() == "sync" {
			found = true
			break
		}
	}

	if !found {
		t.Error("sync command not registered with root command")
	}
}

func TestSyncCommand_Help(t *testing.T) {
	// Verify sync command has help text
	var syncCmd *cobra.Command
	for _, cmd := range rootCmd.Commands() {
		if cmd.Name() == "sync" {
			syncCmd = cmd
			break
		}
	}

	if syncCmd == nil {
		t.Fatal("sync command not found")
	}

	if syncCmd.Short == "" {
		t.Error("sync command missing short description")
	}

	if syncCmd.Long == "" {
		t.Error("sync command missing long description")
	}
}

func TestSyncCommand_NoConfig(t *testing.T) {
	// Test sync command fails gracefully when no config exists
	tmpDir := t.TempDir()
	t.Chdir(tmpDir)

	err := runSync(syncCmd, []string{})
	if err == nil {
		t.Error("runSync() should error when no config file exists")
	}
}

func TestSyncCommand_WithConfig_DryRun(t *testing.T) {
	// Test sync command in dry-run mode with valid config
	tmpDir := t.TempDir()
	t.Chdir(tmpDir)

	// Create minimal valid config
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

	// Enable dry-run mode
	originalDryRun := commonFlags.DryRun
	commonFlags.DryRun = true
	defer func() { commonFlags.DryRun = originalDryRun }()

	// Run sync in dry-run mode (should not fail even without git)
	err := runSync(syncCmd, []string{})

	// In dry-run mode, sync should complete successfully
	// (it won't actually try to clone, just report what it would do)
	if err != nil {
		t.Errorf("runSync() in dry-run mode error = %v, want nil", err)
	}
}
