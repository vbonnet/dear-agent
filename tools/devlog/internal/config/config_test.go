package config

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	deverrors "github.com/vbonnet/dear-agent/tools/devlog/internal/errors"
)

func TestLoad_ValidConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	configYAML := `name: test-workspace
owner: testuser
repos:
  - name: test-repo
    url: https://github.com/test/repo.git
    type: bare
    worktrees:
      - name: main
        branch: main
`
	if err := os.WriteFile(configPath, []byte(configYAML), 0600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}

	if cfg.Name != "test-workspace" {
		t.Errorf("Name = %q, want %q", cfg.Name, "test-workspace")
	}
	if cfg.Owner != "testuser" {
		t.Errorf("Owner = %q, want %q", cfg.Owner, "testuser")
	}
	if len(cfg.Repos) != 1 {
		t.Fatalf("len(Repos) = %d, want 1", len(cfg.Repos))
	}
	if cfg.Repos[0].Name != "test-repo" {
		t.Errorf("Repos[0].Name = %q, want %q", cfg.Repos[0].Name, "test-repo")
	}
}

func TestLoad_FileNotFound(t *testing.T) {
	_, err := Load("/nonexistent/config.yaml")
	if err == nil {
		t.Fatal("Load() error = nil, want error")
	}

	var devErr *deverrors.DevlogError
	if !errors.As(err, &devErr) {
		t.Fatalf("error type = %T, want *DevlogError", err)
	}
	if !errors.Is(err, deverrors.ErrConfigNotFound) {
		t.Errorf("error = %v, want ErrConfigNotFound", err)
	}
}

func TestLoad_InvalidYAML(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	invalidYAML := `name: test
invalid yaml structure
  - bad indent
`
	if err := os.WriteFile(configPath, []byte(invalidYAML), 0600); err != nil {
		t.Fatal(err)
	}

	_, err := Load(configPath)
	if err == nil {
		t.Fatal("Load() error = nil, want error")
	}

	if !errors.Is(err, deverrors.ErrConfigInvalid) {
		t.Errorf("error = %v, want ErrConfigInvalid", err)
	}
}

func TestConfig_Validate_MissingName(t *testing.T) {
	cfg := &Config{
		Repos: []Repo{{Name: "test", URL: "https://github.com/test/repo.git"}},
	}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("Validate() error = nil, want error")
	}
	if !errors.Is(err, deverrors.ErrConfigInvalid) {
		t.Errorf("error = %v, want ErrConfigInvalid", err)
	}
}

func TestConfig_Validate_NoRepos(t *testing.T) {
	cfg := &Config{
		Name:  "test",
		Repos: []Repo{},
	}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("Validate() error = nil, want error")
	}
	if !errors.Is(err, deverrors.ErrConfigInvalid) {
		t.Errorf("error = %v, want ErrConfigInvalid", err)
	}
}

func TestConfig_Validate_DuplicateRepos(t *testing.T) {
	cfg := &Config{
		Name: "test",
		Repos: []Repo{
			{Name: "repo1", URL: "https://github.com/test/repo1.git"},
			{Name: "repo1", URL: "https://github.com/test/repo2.git"},
		},
	}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("Validate() error = nil, want error")
	}
	if !errors.Is(err, deverrors.ErrConfigInvalid) {
		t.Errorf("error = %v, want ErrConfigInvalid", err)
	}
}

func TestConfig_Validate_Valid(t *testing.T) {
	cfg := &Config{
		Name: "test",
		Repos: []Repo{
			{
				Name: "repo1",
				URL:  "https://github.com/test/repo1.git",
				Worktrees: []Worktree{
					{Name: "main", Branch: "main"},
				},
			},
		},
	}

	if err := cfg.Validate(); err != nil {
		t.Errorf("Validate() error = %v, want nil", err)
	}
}

func TestRepo_Validate_MissingName(t *testing.T) {
	repo := &Repo{
		URL: "https://github.com/test/repo.git",
	}

	err := repo.Validate()
	if err == nil {
		t.Fatal("Validate() error = nil, want error")
	}
	if !errors.Is(err, deverrors.ErrConfigInvalid) {
		t.Errorf("error = %v, want ErrConfigInvalid", err)
	}
}

func TestRepo_Validate_MissingURL(t *testing.T) {
	repo := &Repo{
		Name: "test",
	}

	err := repo.Validate()
	if err == nil {
		t.Fatal("Validate() error = nil, want error")
	}
	if !errors.Is(err, deverrors.ErrConfigInvalid) {
		t.Errorf("error = %v, want ErrConfigInvalid", err)
	}
}

func TestRepo_Validate_InvalidURL(t *testing.T) {
	tests := []struct {
		name string
		url  string
	}{
		{"file protocol", "file:///path/to/repo"},
		{"http protocol", "http://github.com/test/repo.git"},
		{"relative path", "../relative/path"},
		{"absolute path", "/absolute/path"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &Repo{
				Name: "test",
				URL:  tt.url,
			}

			err := repo.Validate()
			if err == nil {
				t.Fatalf("Validate() error = nil for URL %q, want error", tt.url)
			}
			if !errors.Is(err, deverrors.ErrConfigInvalid) {
				t.Errorf("error = %v, want ErrConfigInvalid", err)
			}
		})
	}
}

func TestRepo_Validate_ValidURLs(t *testing.T) {
	tests := []struct {
		name string
		url  string
	}{
		{"https", "https://github.com/test/repo.git"},
		{"ssh", "git@github.com:test/repo.git"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &Repo{
				Name: "test",
				URL:  tt.url,
			}

			if err := repo.Validate(); err != nil {
				t.Errorf("Validate() error = %v for URL %q, want nil", err, tt.url)
			}
		})
	}
}

func TestRepo_Validate_InvalidType(t *testing.T) {
	repo := &Repo{
		Name: "test",
		URL:  "https://github.com/test/repo.git",
		Type: "invalid",
	}

	err := repo.Validate()
	if err == nil {
		t.Fatal("Validate() error = nil, want error")
	}
	if !errors.Is(err, deverrors.ErrConfigInvalid) {
		t.Errorf("error = %v, want ErrConfigInvalid", err)
	}
}

func TestRepo_Validate_DuplicateWorktrees(t *testing.T) {
	repo := &Repo{
		Name: "test",
		URL:  "https://github.com/test/repo.git",
		Worktrees: []Worktree{
			{Name: "wt1", Branch: "main"},
			{Name: "wt1", Branch: "feature"},
		},
	}

	err := repo.Validate()
	if err == nil {
		t.Fatal("Validate() error = nil, want error")
	}
	if !errors.Is(err, deverrors.ErrConfigInvalid) {
		t.Errorf("error = %v, want ErrConfigInvalid", err)
	}
}

func TestWorktree_Validate_MissingName(t *testing.T) {
	wt := &Worktree{
		Branch: "main",
	}

	err := wt.Validate()
	if err == nil {
		t.Fatal("Validate() error = nil, want error")
	}
}

func TestWorktree_Validate_MissingBranch(t *testing.T) {
	wt := &Worktree{
		Name: "test",
	}

	err := wt.Validate()
	if err == nil {
		t.Fatal("Validate() error = nil, want error")
	}
}

func TestWorktree_Validate_PathTraversal(t *testing.T) {
	tests := []struct {
		name     string
		wtName   string
		wantErr  bool
		errMatch string
	}{
		{"double dot", "../etc/passwd", true, "path traversal"},
		{"forward slash", "path/to/wt", true, "path traversal"},
		{"valid name", "feature-branch", false, ""},
		{"valid with dash", "my-worktree", false, ""},
		{"valid with underscore", "my_worktree", false, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wt := &Worktree{
				Name:   tt.wtName,
				Branch: "main",
			}

			err := wt.Validate()
			if tt.wantErr {
				if err == nil {
					t.Fatalf("Validate() error = nil for name %q, want error", tt.wtName)
				}
			} else {
				if err != nil {
					t.Errorf("Validate() error = %v for name %q, want nil", err, tt.wtName)
				}
			}
		})
	}
}
