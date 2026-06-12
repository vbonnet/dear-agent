package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// makeV1StatusFile writes a minimal V1 WAYFINDER-STATUS.md into dir.
func makeV1StatusFile(t *testing.T, dir string, currentPhase string) {
	t.Helper()
	content := `---
schema_version: "1.0"
session_id: test-session
project_path: /test/project
status: in_progress
current_phase: ` + currentPhase + `
phases: []
---

# Wayfinder Session
`
	if err := os.WriteFile(filepath.Join(dir, "WAYFINDER-STATUS.md"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// makeV2StatusFile writes a minimal V2 WAYFINDER-STATUS.md into dir.
func makeV2StatusFile(t *testing.T, dir string, currentWaypoint string) {
	t.Helper()
	content := `---
schema_version: "2.0"
project_name: test
project_type: feature
risk_level: S
current_waypoint: ` + currentWaypoint + `
status: in-progress
created_at: 2026-01-01T00:00:00Z
updated_at: 2026-01-01T00:00:00Z
waypoint_history: []
---
`
	if err := os.WriteFile(filepath.Join(dir, "WAYFINDER-STATUS.md"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// TestNextPhase_V1StatusReturnsCorrectPhase verifies that a V1 STATUS file
// with no completed phases returns the first V1 phase (W0), not "CHARTER".
// This was the ce-l8o4 bug: ParseV2FromDir always returned CHARTER for V1 files.
func TestNextPhase_V1StatusReturnsCorrectPhase(t *testing.T) {
	dir := t.TempDir()
	makeV1StatusFile(t, dir, "") // empty current_phase → first phase

	// Set project directory and capture output by running the logic directly.
	// We test the schema-version branch without invoking cobra end-to-end.
	SetProjectDirectory(dir)
	defer func() { projectDirectory = "" }()

	// runNextPhase writes to stdout; redirect via a pipe.
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	oldStdout := os.Stdout
	os.Stdout = w

	cmdErr := runNextPhase(nil, nil)

	w.Close()
	os.Stdout = oldStdout

	if cmdErr != nil {
		t.Fatalf("runNextPhase returned error: %v", cmdErr)
	}

	buf := make([]byte, 64)
	n, _ := r.Read(buf)
	output := strings.TrimSpace(string(buf[:n]))

	// V1 first phase is "W0", not "CHARTER" (which would indicate V2 was used).
	if output == "CHARTER" {
		t.Errorf("got CHARTER — V2 parser was used on a V1 STATUS file (ce-l8o4 regression)")
	}
	// V1 AllPhases() starts with "W0"
	if output != "W0" {
		t.Errorf("expected W0 as first V1 phase, got %q", output)
	}
}

// TestNextPhase_V2StatusReturnsWaypoint verifies that a V2 STATUS file
// with current_waypoint=CHARTER returns CHARTER (correct V2 behaviour).
func TestNextPhase_V2StatusReturnsWaypoint(t *testing.T) {
	dir := t.TempDir()
	makeV2StatusFile(t, dir, "CHARTER")

	SetProjectDirectory(dir)
	defer func() { projectDirectory = "" }()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	oldStdout := os.Stdout
	os.Stdout = w

	cmdErr := runNextPhase(nil, nil)

	w.Close()
	os.Stdout = oldStdout

	if cmdErr != nil {
		t.Fatalf("runNextPhase returned error: %v", cmdErr)
	}

	buf := make([]byte, 64)
	n, _ := r.Read(buf)
	output := strings.TrimSpace(string(buf[:n]))

	if output != "CHARTER" {
		t.Errorf("expected CHARTER for V2 STATUS at CHARTER waypoint, got %q", output)
	}
}

// TestNextPhase_MissingStatusFileErrors verifies a helpful error when no STATUS found.
func TestNextPhase_MissingStatusFileErrors(t *testing.T) {
	dir := t.TempDir() // no STATUS file written

	SetProjectDirectory(dir)
	defer func() { projectDirectory = "" }()

	err := runNextPhase(nil, nil)
	if err == nil {
		t.Fatal("expected error for missing STATUS file, got nil")
	}
	if !strings.Contains(err.Error(), "STATUS") {
		t.Errorf("error should mention STATUS file, got: %v", err)
	}
}
