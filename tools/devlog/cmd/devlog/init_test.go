package devlog

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
)

func TestInitCommand_Exists(t *testing.T) {
	// Verify init command was registered
	found := false
	for _, cmd := range rootCmd.Commands() {
		if cmd.Name() == "init" {
			found = true
			break
		}
	}

	if !found {
		t.Error("init command not registered with root command")
	}
}

func TestInitCommand_Help(t *testing.T) {
	// Verify init command has help text
	var initCmd *cobra.Command
	for _, cmd := range rootCmd.Commands() {
		if cmd.Name() == "init" {
			initCmd = cmd
			break
		}
	}

	if initCmd == nil {
		t.Fatal("init command not found")
	}

	if initCmd.Short == "" {
		t.Error("init command missing short description")
	}

	if initCmd.Long == "" {
		t.Error("init command missing long description")
	}
}

func TestInitCommand_CreatesFiles(t *testing.T) {
	tmpDir := t.TempDir()
	t.Chdir(tmpDir)

	// Run init command
	err := runInit(initCmd, []string{"test-workspace"})
	if err != nil {
		t.Fatalf("runInit() error = %v, want nil", err)
	}

	// Verify .devlog directory was created
	devlogPath := filepath.Join(tmpDir, ".devlog")
	if _, err := os.Stat(devlogPath); os.IsNotExist(err) {
		t.Error(".devlog directory was not created")
	}

	// Verify config.yaml was created
	configPath := filepath.Join(devlogPath, "config.yaml")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Error(".devlog/config.yaml was not created")
	}

	// Verify config.local.yaml.example was created
	localExamplePath := filepath.Join(devlogPath, "config.local.yaml.example")
	if _, err := os.Stat(localExamplePath); os.IsNotExist(err) {
		t.Error(".devlog/config.local.yaml.example was not created")
	}

	// Verify README.md was created
	readmePath := filepath.Join(devlogPath, "README.md")
	if _, err := os.Stat(readmePath); os.IsNotExist(err) {
		t.Error(".devlog/README.md was not created")
	}

	// Verify .gitignore was created
	gitignorePath := filepath.Join(devlogPath, ".gitignore")
	if _, err := os.Stat(gitignorePath); os.IsNotExist(err) {
		t.Error(".devlog/.gitignore was not created")
	}
}

func TestInitCommand_ConfigContent(t *testing.T) {
	tmpDir := t.TempDir()
	t.Chdir(tmpDir)

	// Run init command with workspace name
	err := runInit(initCmd, []string{"my-test-workspace"})
	if err != nil {
		t.Fatalf("runInit() error = %v, want nil", err)
	}

	// Read config.yaml
	configPath := filepath.Join(tmpDir, ".devlog", "config.yaml")
	content, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("Failed to read config.yaml: %v", err)
	}

	configStr := string(content)

	// Verify workspace name is in config
	if !contains(configStr, "my-test-workspace") {
		t.Error("config.yaml should contain workspace name 'my-test-workspace'")
	}

	// Verify example repo is in config
	if !contains(configStr, "example-repo") {
		t.Error("config.yaml should contain example repository")
	}

	// Verify header comment is present
	if !contains(configStr, "# devlog workspace configuration") {
		t.Error("config.yaml should contain header comment")
	}
}

func TestInitCommand_DefaultWorkspaceName(t *testing.T) {
	tmpDir := t.TempDir()
	t.Chdir(tmpDir)

	// Run init without workspace name (should use directory name)
	err := runInit(initCmd, []string{})
	if err != nil {
		t.Fatalf("runInit() error = %v, want nil", err)
	}

	// Read config.yaml
	configPath := filepath.Join(tmpDir, ".devlog", "config.yaml")
	content, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("Failed to read config.yaml: %v", err)
	}

	// Should use tmpDir basename as workspace name
	expectedName := filepath.Base(tmpDir)
	if !contains(string(content), expectedName) {
		t.Errorf("config.yaml should contain default workspace name '%s'", expectedName)
	}
}

func TestInitCommand_AlreadyInitialized(t *testing.T) {
	tmpDir := t.TempDir()
	t.Chdir(tmpDir)

	// Run init first time
	err := runInit(initCmd, []string{"test"})
	if err != nil {
		t.Fatalf("First runInit() error = %v, want nil", err)
	}

	// Run init second time without --force
	err = runInit(initCmd, []string{"test"})
	if err == nil {
		t.Error("runInit() should error when workspace already initialized")
	}
}

func TestInitCommand_ForceOverwrite(t *testing.T) {
	tmpDir := t.TempDir()
	t.Chdir(tmpDir)

	// Run init first time
	err := runInit(initCmd, []string{"test"})
	if err != nil {
		t.Fatalf("First runInit() error = %v, want nil", err)
	}

	// Enable force flag
	originalForce := initForce
	initForce = true
	defer func() { initForce = originalForce }()

	// Run init second time with --force
	err = runInit(initCmd, []string{"test-overwrite"})
	if err != nil {
		t.Errorf("runInit() with --force error = %v, want nil", err)
	}

	// Verify config was overwritten with new name
	configPath := filepath.Join(tmpDir, ".devlog", "config.yaml")
	content, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("Failed to read config.yaml: %v", err)
	}

	if !contains(string(content), "test-overwrite") {
		t.Error("config.yaml should be overwritten with new workspace name")
	}
}

func TestCreateExampleConfig(t *testing.T) {
	cfg := createExampleConfig("test-workspace")

	if cfg.Name != "test-workspace" {
		t.Errorf("config.Name = %s, want test-workspace", cfg.Name)
	}

	if len(cfg.Repos) == 0 {
		t.Error("config should have at least one example repository")
	}

	if len(cfg.Repos) > 0 {
		repo := cfg.Repos[0]
		if repo.Name == "" {
			t.Error("example repo should have a name")
		}
		if repo.URL == "" {
			t.Error("example repo should have a URL")
		}
		if len(repo.Worktrees) == 0 {
			t.Error("example repo should have worktrees")
		}
	}
}

// Helper function to check if string contains substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && containsAt(s, substr))
}

func containsAt(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
