package config

import (
	"os"
	"path/filepath"
	"testing"

	"errors"
	deverrors "github.com/vbonnet/dear-agent/tools/devlog/internal/errors"
)

func TestLoadMerged_Success(t *testing.T) {
	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, ".devlog")
	if err := os.Mkdir(configDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create base config
	baseYAML := `name: test-workspace
repos:
  - name: repo1
    url: https://github.com/test/repo1.git
    worktrees:
      - name: main
        branch: main
`
	baseConfigPath := filepath.Join(configDir, "config.yaml")
	if err := os.WriteFile(baseConfigPath, []byte(baseYAML), 0600); err != nil {
		t.Fatal(err)
	}

	// Create local config
	localYAML := `repos:
  - name: repo1
    worktrees:
      - name: feature
        branch: feature-branch
`
	localConfigPath := filepath.Join(configDir, "config.local.yaml")
	if err := os.WriteFile(localConfigPath, []byte(localYAML), 0600); err != nil {
		t.Fatal(err)
	}

	// Load merged config
	cfg, err := LoadMerged(tmpDir)
	if err != nil {
		t.Fatalf("LoadMerged() error = %v, want nil", err)
	}

	// Verify merging worked
	if len(cfg.Repos) != 1 {
		t.Fatalf("len(Repos) = %d, want 1", len(cfg.Repos))
	}

	repo := cfg.Repos[0]
	if len(repo.Worktrees) != 2 {
		t.Fatalf("len(Worktrees) = %d, want 2 (merged)", len(repo.Worktrees))
	}
}

func TestLoadMerged_NoLocalConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, ".devlog")
	if err := os.Mkdir(configDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create only base config
	baseYAML := `name: test-workspace
repos:
  - name: repo1
    url: https://github.com/test/repo1.git
`
	baseConfigPath := filepath.Join(configDir, "config.yaml")
	if err := os.WriteFile(baseConfigPath, []byte(baseYAML), 0600); err != nil {
		t.Fatal(err)
	}

	// Load (should work without local config)
	cfg, err := LoadMerged(tmpDir)
	if err != nil {
		t.Fatalf("LoadMerged() error = %v, want nil", err)
	}

	if cfg.Name != "test-workspace" {
		t.Errorf("Name = %q, want %q", cfg.Name, "test-workspace")
	}
}

func TestLoadMerged_InvalidLocalConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, ".devlog")
	if err := os.Mkdir(configDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create base config
	baseYAML := `name: test
repos:
  - name: repo1
    url: https://github.com/test/repo.git
`
	baseConfigPath := filepath.Join(configDir, "config.yaml")
	if err := os.WriteFile(baseConfigPath, []byte(baseYAML), 0600); err != nil {
		t.Fatal(err)
	}

	// Create invalid local config
	localYAML := `invalid: yaml: structure
  - bad
`
	localConfigPath := filepath.Join(configDir, "config.local.yaml")
	if err := os.WriteFile(localConfigPath, []byte(localYAML), 0600); err != nil {
		t.Fatal(err)
	}

	// Load should fail
	_, err := LoadMerged(tmpDir)
	if err == nil {
		t.Fatal("LoadMerged() error = nil, want error")
	}

	if !errors.Is(err, deverrors.ErrConfigInvalid) {
		t.Errorf("error = %v, want ErrConfigInvalid", err)
	}
}

func TestFindConfigDir_CurrentDir(t *testing.T) {
	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, ".devlog")
	if err := os.Mkdir(configDir, 0755); err != nil {
		t.Fatal(err)
	}

	configYAML := `name: test
repos: []
`
	configPath := filepath.Join(configDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(configYAML), 0600); err != nil {
		t.Fatal(err)
	}

	found, err := findConfigDir(tmpDir)
	if err != nil {
		t.Fatalf("findConfigDir() error = %v, want nil", err)
	}

	if found != configDir {
		t.Errorf("findConfigDir() = %q, want %q", found, configDir)
	}
}

func TestFindConfigDir_ParentDir(t *testing.T) {
	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, ".devlog")
	subDir := filepath.Join(tmpDir, "subdir", "deep")

	// Create config in root
	if err := os.Mkdir(configDir, 0755); err != nil {
		t.Fatal(err)
	}

	configYAML := `name: test
repos: []
`
	configPath := filepath.Join(configDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(configYAML), 0600); err != nil {
		t.Fatal(err)
	}

	// Create subdirectories
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Search from subdirectory should find parent config
	found, err := findConfigDir(subDir)
	if err != nil {
		t.Fatalf("findConfigDir() error = %v, want nil", err)
	}

	if found != configDir {
		t.Errorf("findConfigDir() = %q, want %q", found, configDir)
	}
}

func TestFindConfigDir_NotFound(t *testing.T) {
	tmpDir := t.TempDir()

	_, err := findConfigDir(tmpDir)
	if err == nil {
		t.Fatal("findConfigDir() error = nil, want error")
	}

	if !errors.Is(err, deverrors.ErrConfigNotFound) {
		t.Errorf("error = %v, want ErrConfigNotFound", err)
	}
}
