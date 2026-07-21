package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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

func makeLegacyStatusFile(t *testing.T, dir string) {
	t.Helper()
	content := "---\nschema_version: \"1.0\"\nsession_id: legacy\ncurrent_phase: PROBLEM\n---\n"
	if err := os.WriteFile(filepath.Join(dir, "WAYFINDER-STATUS.md"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// TestNextPhase_V2StatusReturnsWaypoint verifies that a V2 STATUS file
// with current_waypoint=CHARTER returns CHARTER (correct V2 behaviour).
func TestNextPhase_V2StatusReturnsWaypoint(t *testing.T) {
	dir := t.TempDir()
	makeV2StatusFile(t, dir, "CHARTER")

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	oldStdout := os.Stdout
	os.Stdout = w

	cmdErr := runNextPhaseInDir(dir)

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

	err := runNextPhaseInDir(dir)
	if err == nil {
		t.Fatal("expected error for missing STATUS file, got nil")
	}
	if !strings.Contains(err.Error(), "STATUS") {
		t.Errorf("error should mention STATUS file, got: %v", err)
	}
}

func TestNextPhase_RejectsUnsupportedSchema(t *testing.T) {
	dir := t.TempDir()
	makeLegacyStatusFile(t, dir)
	err := runNextPhaseInDir(dir)
	if err == nil || !strings.Contains(err.Error(), "unsupported schema_version") {
		t.Fatalf("runNextPhase schema error = %v, want unsupported schema guidance", err)
	}
}
