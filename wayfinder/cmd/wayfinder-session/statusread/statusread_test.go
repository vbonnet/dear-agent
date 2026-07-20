package statusread

import (
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
	contents := `---
schema_version: "2.0"
project_name: reader-test
project_type: feature
risk_level: S
current_waypoint: BUILD
status: in-progress
created_at: 2026-07-20T00:00:00Z
updated_at: 2026-07-20T00:00:00Z
beads:
  - ce-reader
---
`
	if err := os.WriteFile(filepath.Join(dir, "WAYFINDER-STATUS.md"), []byte(contents), 0o600); err != nil {
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
}
