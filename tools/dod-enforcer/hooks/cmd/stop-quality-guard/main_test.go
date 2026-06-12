package main

import (
	"os"
	"testing"

	"github.com/vbonnet/dear-agent/pkg/stophook"
)

func TestGrepCount_NonGitDir(t *testing.T) {
	t.Parallel()
	// A temp dir that is not a git repo should return 0 (error path).
	dir := t.TempDir()
	got := grepCount(dir, "TODO")
	if got != 0 {
		t.Errorf("grepCount(non-git) = %d, want 0", got)
	}
}

func TestCheckCodeMarkers_EmptyDir(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	r := &stophook.Result{HookName: "test"}
	checkCodeMarkers(r, dir)
	if r.ExitCode() != 0 {
		t.Errorf("checkCodeMarkers(empty dir) exit = %d, want 0", r.ExitCode())
	}
}

func TestCheckDocs_EmptyDir(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	r := &stophook.Result{HookName: "test"}
	checkDocs(r, dir)
	// Empty dir has no source files: no SPEC.md warning expected.
	// README.md is missing → warn. Either way, no blocking checks.
	if r.ExitCode() != 0 {
		t.Errorf("checkDocs(empty dir) exit = %d, want 0", r.ExitCode())
	}
}

func TestCheckDocs_WithReadme(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(dir+"/README.md", []byte("# Test\n"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	r := &stophook.Result{HookName: "test"}
	checkDocs(r, dir)
	if r.ExitCode() != 0 {
		t.Errorf("checkDocs(with README) exit = %d, want 0", r.ExitCode())
	}
}
