package statusread

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseFromDirRejectsIncompleteCanonicalStatus(t *testing.T) {
	dir := t.TempDir()
	contents := `---
schema_version: "2.0"
status: completed
current_waypoint: RETRO
---
`
	if err := os.WriteFile(filepath.Join(dir, "WAYFINDER-STATUS.md"), []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := ParseFromDir(dir)
	if err == nil || !strings.Contains(err.Error(), "project_name is required") {
		t.Fatalf("ParseFromDir() error = %v, want missing project_name", err)
	}
}

func TestParseFromDirReturnsValidatedConsumerFields(t *testing.T) {
	dir := t.TempDir()
	contents := canonicalStatusBytes("reader-test", "BUILD", "beads:\n  - ce-reader\n")
	if err := os.WriteFile(filepath.Join(dir, "WAYFINDER-STATUS.md"), contents, 0o600); err != nil {
		t.Fatal(err)
	}

	summary, err := ParseFromDir(dir)
	if err != nil {
		t.Fatalf("ParseFromDir() error = %v", err)
	}
	if summary.ProjectName != "reader-test" || summary.Status != "in-progress" || summary.CurrentWaypoint != "BUILD" {
		t.Fatalf("ParseFromDir() summary = %+v", summary)
	}
	if len(summary.Beads) != 1 || summary.Beads[0] != "ce-reader" {
		t.Fatalf("ParseFromDir() beads = %v", summary.Beads)
	}
	if summary.UpdatedAt.IsZero() {
		t.Fatal("ParseFromDir() omitted canonical updated_at")
	}
}

func TestParseValidatesExactStatusBytes(t *testing.T) {
	contents := canonicalStatusBytes("byte-reader", "SETUP", "")
	summary, err := Parse(contents)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if summary.ProjectName != "byte-reader" || summary.CurrentWaypoint != "SETUP" {
		t.Fatalf("Parse() summary = %+v", summary)
	}

	invalid := []byte(strings.Replace(string(contents), "risk_level: S", "risk_level: impossible", 1))
	if _, err := Parse(invalid); err == nil || !strings.Contains(err.Error(), "invalid risk_level") {
		t.Fatalf("Parse() error = %v, want invalid risk_level", err)
	}
}

func canonicalStatusBytes(projectName, currentWaypoint, extraFields string) []byte {
	var history strings.Builder
	for _, waypoint := range []string{"CHARTER", "PROBLEM", "RESEARCH", "DESIGN", "SPEC", "PLAN", "SETUP", "BUILD", "RETRO"} {
		if waypoint == currentWaypoint {
			break
		}
		fmt.Fprintf(&history, "  - {name: %s, status: completed, started_at: 2026-07-20T00:00:00Z, completed_at: 2026-07-20T00:01:00Z}\n", waypoint)
	}
	return fmt.Appendf(nil, `---
schema_version: "2.0"
project_name: %s
project_type: feature
risk_level: S
current_waypoint: %s
status: in-progress
created_at: 2026-07-20T00:00:00Z
updated_at: 2026-07-20T00:00:00Z
%swaypoint_history:
%s---
`, projectName, currentWaypoint, extraFields, history.String())
}

func TestParseDerivesProgressFromCanonicalHistory(t *testing.T) {
	contents := []byte(`---
schema_version: "2.0"
project_name: progress-reader
project_type: feature
risk_level: S
current_waypoint: BUILD
status: in-progress
created_at: 2026-07-20T00:00:00Z
updated_at: 2026-07-20T00:00:00Z
waypoint_history:
  - name: CHARTER
    status: completed
    started_at: 2026-07-20T00:00:00Z
    completed_at: 2026-07-20T00:01:00Z
  - name: PROBLEM
    status: completed
    started_at: 2026-07-20T00:01:00Z
    completed_at: 2026-07-20T00:02:00Z
  - name: RESEARCH
    status: completed
    started_at: 2026-07-20T00:02:00Z
    completed_at: 2026-07-20T00:03:00Z
  - name: DESIGN
    status: completed
    started_at: 2026-07-20T00:03:00Z
    completed_at: 2026-07-20T00:04:00Z
  - name: SPEC
    status: completed
    started_at: 2026-07-20T00:04:00Z
    completed_at: 2026-07-20T00:05:00Z
  - name: PLAN
    status: completed
    started_at: 2026-07-20T00:05:00Z
    completed_at: 2026-07-20T00:06:00Z
  - name: SETUP
    status: completed
    started_at: 2026-07-20T00:06:00Z
    completed_at: 2026-07-20T00:07:00Z
---
`)
	summary, err := Parse(contents)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if summary.Progress != 77 {
		t.Fatalf("Parse() progress = %d, want 77", summary.Progress)
	}
}

func TestParseCountsConfiguredSkippedPhasesInProgress(t *testing.T) {
	contents := []byte(`---
schema_version: "2.0"
project_name: lite-progress-reader
project_type: feature
risk_level: XS
current_waypoint: BUILD
status: in-progress
created_at: 2026-07-20T00:00:00Z
updated_at: 2026-07-20T00:00:00Z
skip_roadmap: true
skip_phases:
  - DESIGN
  - SPEC
  - PLAN
waypoint_history:
  - name: CHARTER
    status: completed
    started_at: 2026-07-20T00:00:00Z
    completed_at: 2026-07-20T00:01:00Z
  - name: PROBLEM
    status: completed
    started_at: 2026-07-20T00:01:00Z
    completed_at: 2026-07-20T00:02:00Z
  - name: RESEARCH
    status: completed
    started_at: 2026-07-20T00:02:00Z
    completed_at: 2026-07-20T00:03:00Z
---
`)
	summary, err := Parse(contents)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if summary.Progress != 77 {
		t.Fatalf("Parse() progress = %d, want 77", summary.Progress)
	}
}
