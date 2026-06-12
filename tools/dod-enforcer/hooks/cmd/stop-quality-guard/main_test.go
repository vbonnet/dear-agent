package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/vbonnet/dear-agent/pkg/stophook"
)

func TestGrepCount_NonGitDir_ReturnsZero(t *testing.T) {
	// A directory without a git repo causes git grep to fail; grepCount returns 0.
	n := grepCount(t.TempDir(), "TODO")
	if n != 0 {
		t.Errorf("grepCount on non-git dir = %d, want 0", n)
	}
}

func TestCheckDocs_NoFiles_NoBlock(t *testing.T) {
	dir := t.TempDir()
	result := &stophook.Result{HookName: "test"}
	checkDocs(result, dir)
	// Empty dir has no README or SPEC — should warn, not block (exit 1).
	if result.ExitCode() == 1 {
		t.Error("checkDocs on empty dir should not block (exit 1)")
	}
}

func TestCheckDocs_WithReadme_Passes(t *testing.T) {
	dir := t.TempDir()
	// Write a fresh README
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	result := &stophook.Result{HookName: "test"}
	checkDocs(result, dir)
	// Fresh README should not produce a blocking result.
	if result.ExitCode() == 1 {
		t.Error("checkDocs with fresh README should not block")
	}
}

func TestCheckCodeMarkers_EmptyDir_NoBlock(t *testing.T) {
	result := &stophook.Result{HookName: "test"}
	checkCodeMarkers(result, t.TempDir())
	if result.ExitCode() == 1 {
		t.Error("checkCodeMarkers on empty dir should not block")
	}
}
