package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// scaffold builds a repo root (with a manifest + sources) and a host home, and
// returns the common flags pointing at them. The manifest declares one required
// plist (with a token) and one optional unbuilt hook.
func scaffold(t *testing.T) (repo, home string) {
	t.Helper()
	repo = t.TempDir()
	home = t.TempDir()

	mustWrite(t, filepath.Join(repo, "deploy/launchd/x.plist"), "home=__HOME__\n")
	manifest := `artifacts:
  - name: x.plist
    source: deploy/launchd/x.plist
    deployed: ~/Library/LaunchAgents/x.plist
    mode: "0644"
    tokens:
      __HOME__: ${HOME}
  - name: hook
    source: bin/hook
    deployed: ~/.config/hooks/hook
    mode: "0755"
    optional: true
    remediation: make build-write-guards
`
	mustWrite(t, filepath.Join(repo, "deploy/manifest.yaml"), manifest)
	return repo, home
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
}

// invoke runs the CLI with the standard --repo-root/--home wiring appended.
func invoke(t *testing.T, repo, home string, args ...string) (int, string, string) {
	t.Helper()
	full := append(args, "--repo-root", repo, "--home", home)
	var out, errb bytes.Buffer
	code := run(full, &out, &errb)
	return code, out.String(), errb.String()
}

func TestList(t *testing.T) {
	repo, home := scaffold(t)
	code, out, errs := invoke(t, repo, home, "list")
	if code != 0 {
		t.Fatalf("exit = %d, stderr=%s", code, errs)
	}
	if !strings.Contains(out, "x.plist") || !strings.Contains(out, "hook") {
		t.Fatalf("list missing artifacts: %s", out)
	}
	if !strings.Contains(out, "2 artifact(s)") {
		t.Fatalf("list count wrong: %s", out)
	}
}

func TestStatus_MissingThenSynced(t *testing.T) {
	repo, home := scaffold(t)

	// Before any sync: required plist missing => exit 2; optional hook skipped.
	code, out, _ := invoke(t, repo, home, "status")
	if code != 2 {
		t.Fatalf("status exit = %d, want 2 (drift); out=%s", code, out)
	}
	if !strings.Contains(out, "MISSING") || !strings.Contains(out, "OUT OF SYNC") {
		t.Fatalf("status output unexpected: %s", out)
	}

	// Sync, then status should be clean (exit 0).
	if code, out, errs := invoke(t, repo, home, "sync"); code != 0 {
		t.Fatalf("sync exit = %d, out=%s err=%s", code, out, errs)
	}
	code, out, _ = invoke(t, repo, home, "status")
	if code != 0 {
		t.Fatalf("post-sync status exit = %d, want 0; out=%s", code, out)
	}
	if !strings.Contains(out, "In sync") {
		t.Fatalf("expected in-sync message: %s", out)
	}
}

func TestSync_DeploysAndRendersToken(t *testing.T) {
	repo, home := scaffold(t)
	code, out, errs := invoke(t, repo, home, "sync")
	if code != 0 {
		t.Fatalf("sync exit = %d, err=%s", code, errs)
	}
	if !strings.Contains(out, "installed") {
		t.Fatalf("sync output: %s", out)
	}
	deployed := filepath.Join(home, "Library/LaunchAgents/x.plist")
	got, err := os.ReadFile(deployed)
	if err != nil {
		t.Fatalf("read deployed: %v", err)
	}
	want := "home=" + home + "\n"
	if string(got) != want {
		t.Fatalf("rendered = %q, want %q", got, want)
	}

	// Second sync is idempotent: everything unchanged.
	_, out2, _ := invoke(t, repo, home, "sync")
	if !strings.Contains(out2, "unchanged") || strings.Contains(out2, "1 changed") {
		t.Fatalf("second sync not idempotent: %s", out2)
	}
}

func TestSync_SkipsOptionalUnbuilt(t *testing.T) {
	repo, home := scaffold(t)
	_, out, _ := invoke(t, repo, home, "sync", "hook")
	if !strings.Contains(out, "skipped") {
		t.Fatalf("expected optional hook skipped: %s", out)
	}
}

func TestInstall_ForcesRewrite(t *testing.T) {
	repo, home := scaffold(t)
	if code, _, errs := invoke(t, repo, home, "sync", "x.plist"); code != 0 {
		t.Fatalf("sync: %s", errs)
	}
	// install of an already-synced artifact must report a change (force rewrite),
	// where a second sync would report unchanged.
	_, out, _ := invoke(t, repo, home, "install", "x.plist")
	if !strings.Contains(out, "1 changed") {
		t.Fatalf("install did not force rewrite: %s", out)
	}
}

func TestSync_DryRunWritesNothing(t *testing.T) {
	repo, home := scaffold(t)
	code, out, _ := invoke(t, repo, home, "sync", "--dry-run")
	if code != 0 {
		t.Fatalf("dry-run exit = %d", code)
	}
	if !strings.Contains(out, "dry-run") || !strings.Contains(out, "install") {
		t.Fatalf("dry-run output: %s", out)
	}
	if _, err := os.Stat(filepath.Join(home, "Library/LaunchAgents/x.plist")); !os.IsNotExist(err) {
		t.Fatal("dry-run must not write the deployed file")
	}
}

func TestStatus_JSON(t *testing.T) {
	repo, home := scaffold(t)
	code, out, _ := invoke(t, repo, home, "status", "--json")
	if code != 2 { // plist still missing
		t.Fatalf("status --json exit = %d, want 2", code)
	}
	var results []map[string]any
	if err := json.Unmarshal([]byte(out), &results); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	if len(results) != 2 {
		t.Fatalf("got %d results, want 2", len(results))
	}
}

func TestUnknownSubcommandAndNoArgs(t *testing.T) {
	var out, errb bytes.Buffer
	if code := run(nil, &out, &errb); code != 1 {
		t.Fatalf("no args exit = %d, want 1", code)
	}
	out.Reset()
	errb.Reset()
	if code := run([]string{"frobnicate"}, &out, &errb); code != 1 {
		t.Fatalf("unknown subcommand exit = %d, want 1", code)
	}
	if !strings.Contains(errb.String(), "unknown subcommand") {
		t.Fatalf("expected unknown-subcommand error: %s", errb.String())
	}
}

func TestUnknownArtifactErrors(t *testing.T) {
	repo, home := scaffold(t)
	code, _, errs := invoke(t, repo, home, "sync", "does-not-exist")
	if code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if !strings.Contains(errs, "unknown artifact") {
		t.Fatalf("expected unknown-artifact error: %s", errs)
	}
}
