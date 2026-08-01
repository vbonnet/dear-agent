package earslint

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfig_Compiles(t *testing.T) {
	if _, err := New(DefaultConfig()); err != nil {
		t.Fatalf("default config should compile: %v", err)
	}
	if len(DefaultConfig().Patterns) < 5 {
		t.Fatalf("expected at least the 5 EARS patterns, got %d", len(DefaultConfig().Patterns))
	}
}

func TestLoadConfig_OverridesPatterns(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".earslint.yml")
	body := `requirement_keyword: must
patterns:
  - name: ubiquitous-must
    regex: '(?i)^the\s+.+\s+must\s+.+'
    description: The <system> must <behavior>
`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.RequirementKeyword != "must" {
		t.Errorf("want keyword 'must', got %q", cfg.RequirementKeyword)
	}
	if len(cfg.Patterns) != 1 || cfg.Patterns[0].Name != "ubiquitous-must" {
		t.Errorf("unexpected patterns: %+v", cfg.Patterns)
	}
}

func TestLoadConfig_PartialFallsBackToDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".earslint.yml")
	// Only override the keyword; patterns should fall back to defaults.
	if err := os.WriteFile(path, []byte("requirement_keyword: shall\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if len(cfg.Patterns) != len(DefaultConfig().Patterns) {
		t.Errorf("expected default patterns when omitted, got %d", len(cfg.Patterns))
	}
}

func TestLoadConfig_Missing(t *testing.T) {
	if _, err := LoadConfig(filepath.Join(t.TempDir(), "nope.yml")); err == nil {
		t.Fatal("expected error for missing config")
	}
}

func TestLoadConfig_BadYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.yml")
	if err := os.WriteFile(path, []byte("patterns: [oops\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadConfig(path); err == nil {
		t.Fatal("expected error for malformed YAML")
	}
}
