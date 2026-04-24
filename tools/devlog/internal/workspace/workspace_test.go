package workspace

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/vbonnet/dear-agent/tools/devlog/internal/config"
)

func TestGetRepoPath(t *testing.T) {
	ws := &Workspace{
		Root: "~/workspace",
	}

	repo := &config.Repo{
		Name: "test-repo",
	}

	path := ws.GetRepoPath(repo)
	expected := "~/workspace/test-repo"

	if path != expected {
		t.Errorf("GetRepoPath() = %s, want %s", path, expected)
	}
}

func TestGetWorktreePath(t *testing.T) {
	ws := &Workspace{
		Root: "~/workspace",
	}

	repo := &config.Repo{
		Name: "test-repo",
	}

	worktree := &config.Worktree{
		Name:   "feature",
		Branch: "feature-branch",
	}

	path := ws.GetWorktreePath(repo, worktree)
	expected := "~/workspace/test-repo/feature"

	if path != expected {
		t.Errorf("GetWorktreePath() = %s, want %s", path, expected)
	}
}

func TestLoadWorkspace(t *testing.T) {
	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, ".devlog")
	if err := os.Mkdir(configDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create minimal config
	configYAML := `name: test-workspace
repos:
  - name: repo1
    url: https://github.com/test/repo1.git
`
	configPath := filepath.Join(configDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(configYAML), 0600); err != nil {
		t.Fatal(err)
	}

	ws, err := LoadWorkspace(tmpDir)
	if err != nil {
		t.Fatalf("LoadWorkspace() error = %v, want nil", err)
	}

	if ws.Config.Name != "test-workspace" {
		t.Errorf("Config.Name = %s, want test-workspace", ws.Config.Name)
	}

	if len(ws.Config.Repos) != 1 {
		t.Errorf("len(Config.Repos) = %d, want 1", len(ws.Config.Repos))
	}

	// Root should be absolute path
	if !filepath.IsAbs(ws.Root) {
		t.Errorf("Root should be absolute path, got: %s", ws.Root)
	}
}

func TestLoadWorkspace_NoConfig(t *testing.T) {
	tmpDir := t.TempDir()

	_, err := LoadWorkspace(tmpDir)
	if err == nil {
		t.Fatal("LoadWorkspace() error = nil, want error for missing config")
	}
}
