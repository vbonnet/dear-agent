package earslint

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfigOverridesPatterns(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".earslint.yml")
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

func TestLoadConfigPartialFallsBackToDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".earslint.yml")
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

func TestLoadConfigMissing(t *testing.T) {
	if _, err := LoadConfig(filepath.Join(t.TempDir(), "nope.yml")); err == nil {
		t.Fatal("expected error for missing config")
	}
}

func TestLoadConfigBadYAML(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.yml")
	if err := os.WriteFile(path, []byte("patterns: [oops\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadConfig(path); err == nil {
		t.Fatal("expected error for malformed YAML")
	}
}
