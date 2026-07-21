package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// makeV2LiteStatusFile writes a V2 STATUS file for an XS/S task whose harness profile
// skips DESIGN/SPEC/PLAN, with RESEARCH already completed.
func makeV2LiteStatusFile(t *testing.T, dir string) {
	t.Helper()
	content := `---
schema_version: "2.0"
project_name: test
project_type: bugfix
risk_level: S
current_waypoint: RESEARCH
status: in-progress
created_at: 2026-01-01T00:00:00Z
updated_at: 2026-01-01T00:00:00Z
skip_phases:
  - DESIGN
  - SPEC
  - PLAN
waypoint_history:
  - name: CHARTER
    status: completed
    started_at: 2026-01-01T00:00:00Z
    completed_at: 2026-01-01T00:20:00Z
  - name: PROBLEM
    status: completed
    started_at: 2026-01-01T00:20:00Z
    completed_at: 2026-01-01T00:40:00Z
  - name: RESEARCH
    status: completed
    started_at: 2026-01-01T00:00:00Z
    completed_at: 2026-01-01T01:00:00Z
---
`
	if err := os.WriteFile(filepath.Join(dir, "WAYFINDER-STATUS.md"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// TestNextPhase_LiteProfileSkipsToSetup is the end-to-end proof that ProfileLite is
// wired into the production path: a persisted skip_phases set round-trips through YAML
// and `next-phase` advances from a completed RESEARCH straight to SETUP, skipping
// DESIGN/SPEC/PLAN (ce-12pl / MVD T2.2).
func TestNextPhase_LiteProfileSkipsToSetup(t *testing.T) {
	dir := t.TempDir()
	makeV2LiteStatusFile(t, dir)

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

	if output != "SETUP" {
		t.Errorf("lite profile: expected next phase SETUP after RESEARCH, got %q", output)
	}
}
