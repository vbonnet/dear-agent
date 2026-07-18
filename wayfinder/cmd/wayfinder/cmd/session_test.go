package cmd

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestRootCommandUsesCanonicalSessionSurface(t *testing.T) {
	var names []string
	for _, command := range rootCmd.Commands() {
		names = append(names, command.Name())
	}
	if !slices.Contains(names, "session") {
		t.Fatalf("root commands = %v, want session", names)
	}
	for _, retired := range []string{"start", "autopilot", "features", "abort"} {
		if slices.Contains(names, retired) {
			t.Errorf("root commands expose retired V1 executor %q: %v", retired, names)
		}
	}
}

func TestSessionCommandRegistersCanonicalOperations(t *testing.T) {
	var names []string
	for _, command := range sessionCmd.Commands() {
		names = append(names, command.Name())
	}
	for _, want := range []string{"start", "status", "next-phase", "start-phase", "complete-phase", "end", "task", "rewind-to", "set-lifecycle-state", "coord"} {
		if !slices.Contains(names, want) {
			t.Errorf("session commands = %v, missing %q", names, want)
		}
	}
}

func writeStatus(t *testing.T, dir string) {
	t.Helper()
	content := "---\nschema_version: \"2.0\"\nproject_name: test\nproject_type: feature\nrisk_level: M\ncurrent_waypoint: CHARTER\nstatus: planning\ncreated_at: 2026-07-17T00:00:00Z\nupdated_at: 2026-07-17T00:00:00Z\nwaypoint_history: []\n---\n"
	if err := os.WriteFile(filepath.Join(dir, "WAYFINDER-STATUS.md"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// TestDiscoverProjectDir_SingleProject verifies auto-discovery when exactly one
// project exists under wf/.
func TestDiscoverProjectDir_SingleProject(t *testing.T) {
	root := t.TempDir()
	projDir := filepath.Join(root, "wf", "my-project")
	if err := os.MkdirAll(projDir, 0o700); err != nil {
		t.Fatal(err)
	}
	writeStatus(t, projDir)

	got, err := discoverProjectDir(root)
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}
	if got != projDir {
		t.Errorf("expected %q, got %q", projDir, got)
	}
}

// TestDiscoverProjectDir_MultipleProjects verifies an error when more than one
// project is present (user must use -C).
func TestDiscoverProjectDir_MultipleProjects(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"proj-a", "proj-b"} {
		dir := filepath.Join(root, "wf", name)
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		writeStatus(t, dir)
	}

	_, err := discoverProjectDir(root)
	if err == nil {
		t.Fatal("expected error for multiple projects, got nil")
	}
	if !strings.Contains(err.Error(), "-C") {
		t.Errorf("error should mention -C flag, got: %v", err)
	}
}

// TestDiscoverProjectDir_NoWfDir verifies an error when wf/ does not exist.
func TestDiscoverProjectDir_NoWfDir(t *testing.T) {
	root := t.TempDir() // no wf/ subdirectory

	_, err := discoverProjectDir(root)
	if err == nil {
		t.Fatal("expected error when wf/ is absent, got nil")
	}
}

// TestDiscoverProjectDir_EmptyWfDir verifies an error when wf/ exists but has
// no project directories with STATUS files.
func TestDiscoverProjectDir_EmptyWfDir(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "wf"), 0o700); err != nil {
		t.Fatal(err)
	}

	_, err := discoverProjectDir(root)
	if err == nil {
		t.Fatal("expected error for empty wf/, got nil")
	}
}
