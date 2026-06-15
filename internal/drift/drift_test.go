package drift

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeFile is a tiny helper that creates a file with content under dir.
func writeFile(t *testing.T, dir, rel, content string) string {
	t.Helper()
	p := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", p, err)
	}
	return p
}

func TestCheck_Match(t *testing.T) {
	repo := t.TempDir()
	host := t.TempDir()
	writeFile(t, repo, "src/hook", "line1\nline2\n")
	writeFile(t, host, "deployed/hook", "line1\nline2\n")

	cfg := Config{Targets: []Target{{
		Name:     "hook",
		Deployed: filepath.Join(host, "deployed/hook"),
		Source:   "src/hook",
	}}}
	rep, err := Check(context.Background(), cfg, Options{RepoRoot: repo})
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if rep.HasDrift() {
		t.Fatalf("expected no drift, got %+v", rep.Summary)
	}
	if rep.Summary.OK != 1 {
		t.Fatalf("expected 1 ok, got %d", rep.Summary.OK)
	}
	if rep.Targets[0].DeployedSHA256 != rep.Targets[0].SourceSHA256 {
		t.Fatalf("matching files should have equal hashes")
	}
}

func TestCheck_Drift(t *testing.T) {
	repo := t.TempDir()
	host := t.TempDir()
	writeFile(t, repo, "src/hook", "line1\nNEW LINE\nline2\n")
	deployed := writeFile(t, host, "hook", "line1\nline2\n")

	cfg := Config{Targets: []Target{{
		Name:        "hook",
		Deployed:    deployed,
		Source:      "src/hook",
		Remediation: "agm admin install-hooks",
	}}}
	rep, err := Check(context.Background(), cfg, Options{RepoRoot: repo})
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if !rep.HasDrift() {
		t.Fatalf("expected drift")
	}
	tgt := rep.Targets[0]
	if tgt.Status != StatusDrift {
		t.Fatalf("status = %q, want drift", tgt.Status)
	}
	if tgt.Remediation != "agm admin install-hooks" {
		t.Fatalf("drift result should carry remediation, got %q", tgt.Remediation)
	}
	if !strings.Contains(tgt.Diff, "NEW LINE") {
		t.Fatalf("diff should mention the added line, got:\n%s", tgt.Diff)
	}
}

func TestCheck_Tokens(t *testing.T) {
	repo := t.TempDir()
	host := t.TempDir()
	t.Setenv("HOME", "/Users/test")
	// Source is templated; deployed is the rendered form. With token
	// substitution they must compare equal.
	writeFile(t, repo, "plist", "Home=__USER_HOME__\nBin=__USER_HOME__/go/bin/agm\n")
	deployed := writeFile(t, host, "plist", "Home=/Users/test\nBin=/Users/test/go/bin/agm\n")

	cfg := Config{Targets: []Target{{
		Name:     "plist",
		Deployed: deployed,
		Source:   "plist",
		Tokens:   map[string]string{"__USER_HOME__": "${HOME}"},
	}}}
	rep, err := Check(context.Background(), cfg, Options{RepoRoot: repo, Home: "/Users/test"})
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if rep.HasDrift() {
		t.Fatalf("token-substituted source should match rendered deploy; got drift\n%+v", rep.Targets[0])
	}
}

func TestCheck_MissingDeployed_Required(t *testing.T) {
	repo := t.TempDir()
	writeFile(t, repo, "src/hook", "x\n")
	cfg := Config{Targets: []Target{{
		Name:        "hook",
		Deployed:    filepath.Join(t.TempDir(), "does-not-exist"),
		Source:      "src/hook",
		Remediation: "agm admin install-hooks",
	}}}
	rep, err := Check(context.Background(), cfg, Options{RepoRoot: repo})
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if !rep.HasDrift() {
		t.Fatalf("a required artifact that was never installed is drift")
	}
	if rep.Summary.Missing != 1 {
		t.Fatalf("expected 1 missing, got %d", rep.Summary.Missing)
	}
}

func TestCheck_MissingDeployed_Optional(t *testing.T) {
	repo := t.TempDir()
	writeFile(t, repo, "src/plist", "x\n")
	cfg := Config{Targets: []Target{{
		Name:     "plist",
		Deployed: filepath.Join(t.TempDir(), "does-not-exist"),
		Source:   "src/plist",
		Optional: true,
	}}}
	rep, err := Check(context.Background(), cfg, Options{RepoRoot: repo})
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if rep.HasDrift() {
		t.Fatalf("an optional uninstalled artifact is not drift")
	}
	if rep.Summary.Skipped != 1 {
		t.Fatalf("expected 1 skipped, got %d", rep.Summary.Skipped)
	}
}

func TestCheck_MissingSource(t *testing.T) {
	repo := t.TempDir()
	host := t.TempDir()
	deployed := writeFile(t, host, "hook", "x\n")
	cfg := Config{Targets: []Target{{
		Name:     "hook",
		Deployed: deployed,
		Source:   "src/not-there",
	}}}
	rep, err := Check(context.Background(), cfg, Options{RepoRoot: repo})
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if rep.Summary.Error != 1 {
		t.Fatalf("missing source is a config error, got summary %+v", rep.Summary)
	}
	if rep.HasDrift() {
		t.Fatalf("a missing source is not host drift")
	}
}

func TestCheck_RequiresRepoRoot(t *testing.T) {
	_, err := Check(context.Background(), Config{Targets: []Target{{Name: "x", Deployed: "/tmp/x", Source: "y"}}}, Options{})
	if err == nil {
		t.Fatalf("expected error when RepoRoot is empty")
	}
}

func TestExpandPath(t *testing.T) {
	got := expandPath("~/foo/bar", "/home/me")
	if got != "/home/me/foo/bar" {
		t.Fatalf("expandPath ~ = %q", got)
	}
	if expandPath("~", "/home/me") != "/home/me" {
		t.Fatalf("bare ~ should expand to home")
	}
}

func TestParseConfig(t *testing.T) {
	cfg, err := ParseConfig([]byte("targets:\n  - name: h\n    deployed: ~/h\n    source: src/h\n"))
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}
	if len(cfg.Targets) != 1 || cfg.Targets[0].Name != "h" {
		t.Fatalf("unexpected config: %+v", cfg)
	}
}

func TestParseConfig_RejectsUnknownField(t *testing.T) {
	_, err := ParseConfig([]byte("targets:\n  - name: h\n    deployed: ~/h\n    source: src/h\n    typo: oops\n"))
	if err == nil {
		t.Fatalf("expected strict parse to reject unknown field")
	}
}

func TestParseConfig_RejectsEmpty(t *testing.T) {
	if _, err := ParseConfig([]byte("targets: []\n")); err == nil {
		t.Fatalf("expected error for empty target list")
	}
}
