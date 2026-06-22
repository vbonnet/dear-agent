package cmd

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestResolveProjectRoot_DefaultsToWorkspaceWfDir verifies the project root
// defaults to ~/src/ws/{workspace}/wf/ — outside any git repo checkout, so it
// is neither inside the read-only golden tree nor matched by the wf/** forbidden
// temporal-artifact glob (ce-bq8y).
func TestResolveProjectRoot_DefaultsToWorkspaceWfDir(t *testing.T) {
	t.Setenv("HOME", "/home/test")
	t.Setenv("WAYFINDER_DIR", "")
	prev := projectDirFlag
	projectDirFlag = ""
	defer func() { projectDirFlag = prev }()

	got := resolveProjectRoot("oss")
	want := filepath.Join("/home/test", "src", "ws", "oss", "wf")
	if got != want {
		t.Fatalf("resolveProjectRoot(oss) = %q, want %q", got, want)
	}

	// Must not place wf/ inside the dear-agent golden checkout.
	if strings.Contains(got, filepath.Join("src", "dear-agent")) {
		t.Fatalf("project root %q must not be inside the dear-agent golden checkout", got)
	}
}

// TestResolveProjectRoot_FlagOverride verifies --project-dir wins over the
// default and the env var.
func TestResolveProjectRoot_FlagOverride(t *testing.T) {
	t.Setenv("HOME", "/home/test")
	t.Setenv("WAYFINDER_DIR", "/env/override")
	prev := projectDirFlag
	projectDirFlag = "/explicit/override"
	defer func() { projectDirFlag = prev }()

	if got := resolveProjectRoot("oss"); got != "/explicit/override" {
		t.Fatalf("resolveProjectRoot with flag = %q, want /explicit/override", got)
	}
}

// TestResolveProjectRoot_EnvOverride verifies WAYFINDER_DIR overrides the
// default when no flag is set.
func TestResolveProjectRoot_EnvOverride(t *testing.T) {
	t.Setenv("HOME", "/home/test")
	t.Setenv("WAYFINDER_DIR", "/env/override/wf")
	prev := projectDirFlag
	projectDirFlag = ""
	defer func() { projectDirFlag = prev }()

	if got := resolveProjectRoot("acme"); got != "/env/override/wf" {
		t.Fatalf("resolveProjectRoot with WAYFINDER_DIR = %q, want /env/override/wf", got)
	}
}
