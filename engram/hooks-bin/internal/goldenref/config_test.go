package goldenref

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfigFrom_ValidYAML(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	content := `version: 1
workspace_roots:
  - ~/src
  - ~/work/repos
worktree_base: ~/worktrees
`
	if err := os.WriteFile(cfgPath, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	cfg, err := LoadConfigFrom(cfgPath)
	if err != nil {
		t.Fatalf("LoadConfigFrom returned error: %v", err)
	}

	if cfg.Version != 1 {
		t.Errorf("Expected version 1, got %d", cfg.Version)
	}

	if len(cfg.WorkspaceRoots) != 2 {
		t.Errorf("Expected 2 workspace roots, got %d", len(cfg.WorkspaceRoots))
	}

	if len(cfg.ExpandedRoots()) != 2 {
		t.Errorf("Expected 2 expanded roots, got %d", len(cfg.ExpandedRoots()))
	}

	// Verify tilde expansion happened
	home, _ := os.UserHomeDir()
	for _, root := range cfg.ExpandedRoots() {
		if root[:1] == "~" {
			t.Errorf("Expanded root still contains ~: %s", root)
		}
		if !filepath.IsAbs(root) {
			t.Errorf("Expanded root is not absolute: %s", root)
		}
		cleaned := filepath.Clean(root)
		if !hasPrefix(cleaned, home) {
			t.Errorf("Expanded root doesn't start with home dir: %s", root)
		}
	}
}

func TestLoadConfigFrom_MissingFile(t *testing.T) {
	cfg, err := LoadConfigFrom("/nonexistent/path/config.yaml")
	if err != nil {
		t.Fatalf("Expected nil error for missing file, got: %v", err)
	}

	if cfg == nil {
		t.Fatal("Expected non-nil config for missing file")
	}

	if len(cfg.WorkspaceRoots) != 0 {
		t.Errorf("Expected empty workspace roots for missing file, got %d", len(cfg.WorkspaceRoots))
	}

	// Empty config should not match any path
	if cfg.IsWorkspaceRoot("/any/path") {
		t.Error("Empty config should not match any path")
	}
}

func TestLoadConfigFrom_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(cfgPath, []byte("{{invalid yaml"), 0644); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	cfg, err := LoadConfigFrom(cfgPath)
	if err != nil {
		t.Fatalf("Expected nil error for invalid YAML, got: %v", err)
	}

	if cfg == nil {
		t.Fatal("Expected non-nil config for invalid YAML")
	}

	if len(cfg.WorkspaceRoots) != 0 {
		t.Errorf("Expected empty workspace roots for invalid YAML, got %d", len(cfg.WorkspaceRoots))
	}
}

func TestLoadConfigFrom_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(cfgPath, []byte(""), 0644); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	cfg, err := LoadConfigFrom(cfgPath)
	if err != nil {
		t.Fatalf("Expected nil error for empty file, got: %v", err)
	}

	if len(cfg.WorkspaceRoots) != 0 {
		t.Errorf("Expected empty workspace roots, got %d", len(cfg.WorkspaceRoots))
	}
}

func TestIsWorkspaceRoot_DirectChild(t *testing.T) {
	cfg := configWithRoots("/workspace/src")

	if !cfg.IsWorkspaceRoot("/workspace/src/engram/main.go") {
		t.Error("Expected /workspace/src/engram/main.go to be under workspace root")
	}
}

func TestIsWorkspaceRoot_DeeplyNested(t *testing.T) {
	cfg := configWithRoots("/workspace/src")

	if !cfg.IsWorkspaceRoot("/workspace/src/engram/hooks/cmd/main.go") {
		t.Error("Expected deeply nested path to be under workspace root")
	}
}

func TestIsWorkspaceRoot_OutsideRoot(t *testing.T) {
	cfg := configWithRoots("/workspace/src")

	if cfg.IsWorkspaceRoot("/worktrees/feature/main.go") {
		t.Error("Expected /worktrees/feature/main.go to NOT be under workspace root")
	}
}

func TestIsWorkspaceRoot_HomeDirItself(t *testing.T) {
	cfg := configWithRoots("/tmp/test/src")

	if cfg.IsWorkspaceRoot("/tmp/test/file.txt") {
		t.Error("Expected home dir file to NOT be under workspace root")
	}
}

func TestIsWorkspaceRoot_TmpDir(t *testing.T) {
	cfg := configWithRoots("/tmp/test/src")

	if cfg.IsWorkspaceRoot("/tmp/test.txt") {
		t.Error("Expected /tmp path to NOT be under workspace root")
	}
}

func TestIsWorkspaceRoot_ExactRoot(t *testing.T) {
	cfg := configWithRoots("/workspace/src")

	// The root directory itself (with trailing content) should match
	if !cfg.IsWorkspaceRoot("/workspace/src/something") {
		t.Error("Expected path under root to match")
	}
}

func TestIsWorkspaceRoot_MultipleRoots(t *testing.T) {
	cfg := configWithRoots("/workspace/oss", "/workspace/work")

	if !cfg.IsWorkspaceRoot("/workspace/oss/repo/file.go") {
		t.Error("Expected /workspace/oss path to match first root")
	}

	if !cfg.IsWorkspaceRoot("/workspace/work/repo/file.go") {
		t.Error("Expected /workspace/work path to match second root")
	}

	if cfg.IsWorkspaceRoot("/workspace/personal/file.go") {
		t.Error("Expected /workspace/personal to NOT match any root")
	}
}

func TestIsWorkspaceRoot_EmptyConfig(t *testing.T) {
	cfg := &Config{}

	if cfg.IsWorkspaceRoot("/any/path") {
		t.Error("Empty config should not match any path")
	}
}

func TestIsWorkspaceRoot_SimilarPrefix(t *testing.T) {
	cfg := configWithRoots("/tmp/test/src")

	// /tmp/test/src-backup should NOT match /tmp/test/src/
	if cfg.IsWorkspaceRoot("/tmp/test/src-backup/file.go") {
		t.Error("Similar prefix should NOT match (src-backup vs src)")
	}
}

func TestMatchedRoot(t *testing.T) {
	cfg := configWithRoots("/workspace/oss", "/workspace/work")

	matched := cfg.MatchedRoot("/workspace/oss/engram/main.go")
	if matched != "/workspace/oss" {
		t.Errorf("Expected matched root '/workspace/oss', got '%s'", matched)
	}

	matched = cfg.MatchedRoot("/workspace/work/project/file.go")
	if matched != "/workspace/work" {
		t.Errorf("Expected matched root '/workspace/work', got '%s'", matched)
	}

	matched = cfg.MatchedRoot("/tmp/file.txt")
	if matched != "" {
		t.Errorf("Expected empty matched root for /tmp, got '%s'", matched)
	}
}

func TestExpandHome(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("Cannot determine home directory")
	}

	tests := []struct {
		input    string
		expected string
	}{
		{"~/src", filepath.Join(home, "src")},
		{"~/src/engram", filepath.Join(home, "src/engram")},
		{"/absolute/path", "/absolute/path"},
		{"relative/path", "relative/path"},
		{"~", home},
	}

	for _, tt := range tests {
		result := expandHome(tt.input)
		if result != tt.expected {
			t.Errorf("expandHome(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

// configWithRoots creates a Config with pre-expanded absolute roots
// (bypasses tilde expansion for test determinism).
func configWithRoots(roots ...string) *Config {
	cfg := &Config{
		Version:        1,
		WorkspaceRoots: roots,
		expandedRoots:  make([]string, len(roots)),
	}
	for i, root := range roots {
		expanded := root
		if !filepath.IsAbs(expanded) {
			expanded = expandHome(expanded)
		}
		if expanded[len(expanded)-1] != filepath.Separator {
			expanded += string(filepath.Separator)
		}
		cfg.expandedRoots[i] = expanded
	}
	return cfg
}

// hasPrefix checks if path starts with prefix (helper for tests).
func hasPrefix(path, prefix string) bool {
	return len(path) >= len(prefix) && path[:len(prefix)] == prefix
}

func TestLoadConfigFrom_SessionIsolation(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	content := `version: 2
workspace_roots:
  - ~/src
worktree_base: ~/worktrees
session_isolation:
  enabled: true
  auto_provision: true
  branch_prefix: "session-"
  cleanup_on_end: true
  max_age_days: 7
`
	if err := os.WriteFile(cfgPath, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	cfg, err := LoadConfigFrom(cfgPath)
	if err != nil {
		t.Fatalf("LoadConfigFrom returned error: %v", err)
	}

	if cfg.Version != 2 {
		t.Errorf("Expected version 2, got %d", cfg.Version)
	}

	if cfg.SessionIsolation == nil {
		t.Fatal("Expected session_isolation to be populated")
	}

	if !cfg.SessionIsolation.Enabled {
		t.Error("Expected session_isolation.enabled to be true")
	}

	if !cfg.SessionIsolation.AutoProvision {
		t.Error("Expected session_isolation.auto_provision to be true")
	}

	if cfg.SessionIsolation.BranchPrefix != "session-" {
		t.Errorf("Expected branch_prefix 'session-', got %q", cfg.SessionIsolation.BranchPrefix)
	}

	if !cfg.SessionIsolation.CleanupOnEnd {
		t.Error("Expected cleanup_on_end to be true")
	}

	if cfg.SessionIsolation.MaxAgeDays != 7 {
		t.Errorf("Expected max_age_days 7, got %d", cfg.SessionIsolation.MaxAgeDays)
	}
}

func TestLoadConfigFrom_NoSessionIsolation(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	content := `version: 1
workspace_roots:
  - ~/src
worktree_base: ~/worktrees
`
	if err := os.WriteFile(cfgPath, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	cfg, err := LoadConfigFrom(cfgPath)
	if err != nil {
		t.Fatalf("LoadConfigFrom returned error: %v", err)
	}

	// session_isolation should be nil when not present in config
	if cfg.SessionIsolation != nil {
		t.Error("Expected session_isolation to be nil when not in config")
	}
}

func TestLoadConfigFrom_SessionIsolationDisabled(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	content := `version: 2
workspace_roots:
  - ~/src
worktree_base: ~/worktrees
session_isolation:
  enabled: false
  auto_provision: false
`
	if err := os.WriteFile(cfgPath, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	cfg, err := LoadConfigFrom(cfgPath)
	if err != nil {
		t.Fatalf("LoadConfigFrom returned error: %v", err)
	}

	if cfg.SessionIsolation == nil {
		t.Fatal("Expected session_isolation to be present")
	}

	if cfg.SessionIsolation.Enabled {
		t.Error("Expected session_isolation.enabled to be false")
	}

	if cfg.SessionIsolation.AutoProvision {
		t.Error("Expected session_isolation.auto_provision to be false")
	}
}
