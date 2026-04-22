package enforcement

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadPatterns(t *testing.T) {
	// Write a temp YAML file
	dir := t.TempDir()
	yamlContent := `
version: "1.0"
updated: "2026-03-28"
purpose: "test patterns"
used_by: ["test"]
patterns:
  - id: test-pattern
    order: 0
    re2_regex: '\bcd\s+'
    regex: '\bcd\s+'
    pattern_name: "cd command"
    remediation: "Use absolute paths"
    reason: "Using cd command"
    alternative: "Use -C flag"
    examples:
      - "cd /repo"
    severity: high
    tier2_validation: true
`
	path := filepath.Join(dir, "test-patterns.yaml")
	if err := os.WriteFile(path, []byte(yamlContent), 0600); err != nil {
		t.Fatal(err)
	}

	db, err := LoadPatterns(path)
	if err != nil {
		t.Fatalf("LoadPatterns failed: %v", err)
	}

	if len(db.Patterns) != 1 {
		t.Fatalf("expected 1 pattern, got %d", len(db.Patterns))
	}
	if db.Patterns[0].ID != "test-pattern" {
		t.Errorf("expected id 'test-pattern', got %q", db.Patterns[0].ID)
	}
	if db.Version != "1.0" {
		t.Errorf("expected version '1.0', got %q", db.Version)
	}
}

func TestLoadPatternsFileNotFound(t *testing.T) {
	_, err := LoadPatterns("/nonexistent/file.yaml")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func TestLoadPatternsEmptyPatterns(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.yaml")
	if err := os.WriteFile(path, []byte("patterns: []\n"), 0600); err != nil {
		t.Fatal(err)
	}
	_, err := LoadPatterns(path)
	if err == nil {
		t.Fatal("expected error for empty patterns")
	}
}

func TestGetPattern(t *testing.T) {
	db := &PatternDatabase{
		Patterns: []Pattern{
			{ID: "alpha", Reason: "reason-a"},
			{ID: "beta", Reason: "reason-b"},
		},
	}

	p := db.GetPattern("beta")
	if p == nil {
		t.Fatal("expected to find pattern 'beta'")
	}
	if p.Reason != "reason-b" {
		t.Errorf("expected reason 'reason-b', got %q", p.Reason)
	}

	if db.GetPattern("missing") != nil {
		t.Error("expected nil for missing pattern")
	}
}

func TestFilterBySeverity(t *testing.T) {
	db := &PatternDatabase{
		Patterns: []Pattern{
			{ID: "a", Severity: "high"},
			{ID: "b", Severity: "low"},
			{ID: "c", Severity: "high"},
		},
	}

	high := db.FilterBySeverity("high")
	if len(high) != 2 {
		t.Errorf("expected 2 high-severity patterns, got %d", len(high))
	}
}

func TestFilterByTier(t *testing.T) {
	db := &PatternDatabase{
		Patterns: []Pattern{
			{ID: "a", Tier2Validation: true, Tier3Rejection: false},
			{ID: "b", Tier2Validation: false, Tier3Rejection: true},
			{ID: "c", Tier2Validation: true, Tier3Rejection: true},
		},
	}

	tier2 := db.FilterByTier("tier2")
	if len(tier2) != 2 {
		t.Errorf("expected 2 tier2 patterns, got %d", len(tier2))
	}

	tier3 := db.FilterByTier("tier3")
	if len(tier3) != 2 {
		t.Errorf("expected 2 tier3 patterns, got %d", len(tier3))
	}
}

func TestActivePatterns(t *testing.T) {
	db := &PatternDatabase{
		Patterns: []Pattern{
			{ID: "active", RE2Regex: `\bcd\s+`},
			{ID: "relaxed", RE2Regex: `\bfoo\s+`, Relaxed: true},
			{ID: "consolidated", RE2Regex: `\bbar\s+`, ConsolidatedInto: "active"},
			{ID: "no-re2", Regex: `\bbaz\s+`},
		},
	}

	active := db.ActivePatterns()
	if len(active) != 1 {
		t.Fatalf("expected 1 active pattern, got %d", len(active))
	}
	if active[0].ID != "active" {
		t.Errorf("expected active pattern id 'active', got %q", active[0].ID)
	}
}
