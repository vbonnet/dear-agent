package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func trustedProjects(t *testing.T, home string) map[string]map[string]any {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(home, ".claude.json"))
	if err != nil {
		t.Fatalf("read seeded config: %v", err)
	}
	var config struct {
		Projects map[string]map[string]any `json:"projects"`
	}
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatalf("parse seeded config: %v", err)
	}
	return config.Projects
}

// The create path used to report trust as pre-configured without configuring
// anything, which is what let the dialog reach an unattended spawn while the
// dialog monitor was being skipped.
func TestSeedWorkspaceTrustReportsRealSeeding(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	workDir := filepath.Join(home, "workspace")
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		t.Fatal(err)
	}

	if !seedWorkspaceTrust("claude-code", workDir) {
		t.Fatal("seedWorkspaceTrust() = false for a workspace it could seed")
	}
	resolved, err := filepath.EvalSymlinks(workDir)
	if err != nil {
		t.Fatal(err)
	}
	entry, ok := trustedProjects(t, home)[resolved]
	if !ok {
		t.Fatalf("no trust entry for %q", resolved)
	}
	if entry["hasTrustDialogAccepted"] != true {
		t.Errorf("hasTrustDialogAccepted = %v, want true", entry["hasTrustDialogAccepted"])
	}
}

// Reporting trust as pre-configured when it is not is what silences the dialog
// monitor, so a failed seed has to report false and let the monitor run.
func TestSeedWorkspaceTrustReportsFalseWhenItCannotSeed(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if seedWorkspaceTrust("claude-code", filepath.Join(home, "does-not-exist")) {
		t.Error("seedWorkspaceTrust() = true for a workspace it could not seed")
	}
}

// hasTrustDialogAccepted is Claude Code's flag; other harnesses have their own
// trust surfaces and must not be reported as pre-configured by it.
func TestSeedWorkspaceTrustSkipsOtherHarnesses(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	workDir := filepath.Join(home, "workspace")
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if seedWorkspaceTrust("codex-cli", workDir) {
		t.Error("seedWorkspaceTrust() = true for a non-Claude harness")
	}
	if _, err := os.Stat(filepath.Join(home, ".claude.json")); !os.IsNotExist(err) {
		t.Error("seeding a non-Claude harness touched the Claude config")
	}
}

// A sandboxed session runs under the sandbox's "merged" symlink, but Claude
// keys trust by resolved path. Seeding the symlink is the original bug.
func TestSeedWorkspaceTrustSeedsResolvedSandboxPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	sandbox := filepath.Join(home, ".agm", "sandboxes", "session")
	upper := filepath.Join(sandbox, "upper", "repo0")
	if err := os.MkdirAll(upper, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(sandbox, "upper"), filepath.Join(sandbox, "merged")); err != nil {
		t.Fatal(err)
	}

	if !seedWorkspaceTrust("claude-code", filepath.Join(sandbox, "merged", "repo0")) {
		t.Fatal("seedWorkspaceTrust() = false for a sandbox workspace")
	}
	resolvedUpper, err := filepath.EvalSymlinks(upper)
	if err != nil {
		t.Fatal(err)
	}
	projects := trustedProjects(t, home)
	if _, ok := projects[resolvedUpper]; !ok {
		t.Errorf("resolved sandbox path %q not trusted; trusted: %v", resolvedUpper, projects)
	}
}
