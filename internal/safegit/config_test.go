package safegit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseConfig_Full(t *testing.T) {
	data := []byte(`
expected_reviewers:
  - login: "gemini-code-assist[bot]"
    timeout_minutes: 30
    require_newer_than_push: true
flaky_checks:
  - name: "TestFullLifecycle"
    max_retries: 1
`)
	cfg, err := ParseConfig(data)
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}
	if len(cfg.ExpectedReviewers) != 1 {
		t.Fatalf("want 1 reviewer, got %d", len(cfg.ExpectedReviewers))
	}
	r := cfg.ExpectedReviewers[0]
	if r.Login != "gemini-code-assist[bot]" {
		t.Errorf("login = %q", r.Login)
	}
	if r.TimeoutMinutes != 30 {
		t.Errorf("timeout = %d", r.TimeoutMinutes)
	}
	if !r.RequireNewerThanPush {
		t.Error("require_newer_than_push should be true")
	}
	if len(cfg.FlakyChecks) != 1 || cfg.FlakyChecks[0].Name != "TestFullLifecycle" || cfg.FlakyChecks[0].MaxRetries != 1 {
		t.Errorf("flaky checks = %+v", cfg.FlakyChecks)
	}
}

func TestParseConfig_Empty(t *testing.T) {
	cfg, err := ParseConfig([]byte(""))
	if err != nil {
		t.Fatalf("empty config should parse, got: %v", err)
	}
	if !cfg.IsEmpty() {
		t.Error("empty document should yield an empty config")
	}
}

func TestParseConfig_UnknownFieldRejected(t *testing.T) {
	// A typo in a top-level key must fail loudly, not silently disable a gate.
	data := []byte("expected_reviewer:\n  - login: bot\n")
	_, err := ParseConfig(data)
	if err == nil {
		t.Fatal("expected error for unknown field, got nil")
	}
}

func TestParseConfig_ReviewerMissingLogin(t *testing.T) {
	data := []byte("expected_reviewers:\n  - timeout_minutes: 10\n")
	_, err := ParseConfig(data)
	if err == nil || !strings.Contains(err.Error(), "login") {
		t.Fatalf("expected missing-login error, got: %v", err)
	}
}

func TestParseConfig_FlakyMissingName(t *testing.T) {
	data := []byte("flaky_checks:\n  - max_retries: 1\n")
	_, err := ParseConfig(data)
	if err == nil || !strings.Contains(err.Error(), "name") {
		t.Fatalf("expected missing-name error, got: %v", err)
	}
}

func TestExpectedReviewer_TimeoutDefault(t *testing.T) {
	if got := (ExpectedReviewer{}).Timeout(); got != DefaultReviewerTimeoutMinutes {
		t.Errorf("default timeout = %d, want %d", got, DefaultReviewerTimeoutMinutes)
	}
	if got := (ExpectedReviewer{TimeoutMinutes: 5}).Timeout(); got != 5 {
		t.Errorf("explicit timeout = %d, want 5", got)
	}
}

func TestLoadConfig_AbsentRepoRoot_SkipsSilently(t *testing.T) {
	// No .safe-merge.yml at the given dir and no explicit path → (nil, nil).
	dir := t.TempDir()
	cfg, err := LoadConfig(dir, "")
	if err != nil {
		t.Fatalf("missing repo-root config should be silent, got: %v", err)
	}
	if cfg != nil {
		t.Errorf("expected nil config, got %+v", cfg)
	}
}

func TestLoadConfig_RepoRootFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ConfigFileName)
	if err := os.WriteFile(path, []byte("flaky_checks:\n  - name: Foo\n    max_retries: 2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(dir, "")
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg == nil || len(cfg.FlakyChecks) != 1 || cfg.FlakyChecks[0].MaxRetries != 2 {
		t.Fatalf("unexpected config: %+v", cfg)
	}
}

func TestLoadConfig_ExplicitPathMissing_IsError(t *testing.T) {
	// An explicit --config that points at nothing is a mistake, not a skip.
	_, err := LoadConfig(t.TempDir(), filepath.Join(t.TempDir(), "nope.yml"))
	if err == nil {
		t.Fatal("explicit missing config path should error")
	}
}

func TestLoadConfig_ExplicitPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "custom.yml")
	if err := os.WriteFile(path, []byte("expected_reviewers:\n  - login: bot\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig("/does/not/matter", path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg == nil || len(cfg.ExpectedReviewers) != 1 || cfg.ExpectedReviewers[0].Login != "bot" {
		t.Fatalf("unexpected config: %+v", cfg)
	}
}

func TestConfig_IsEmpty(t *testing.T) {
	if !(&Config{}).IsEmpty() {
		t.Error("zero config should be empty")
	}
	var nilCfg *Config
	if !nilCfg.IsEmpty() {
		t.Error("nil config should be empty")
	}
	if (&Config{FlakyChecks: []FlakyCheck{{Name: "x"}}}).IsEmpty() {
		t.Error("config with a flaky check is not empty")
	}
}
