package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func makeV1StatusFileWithPhases(t *testing.T, dir string, startedAt time.Time) {
	t.Helper()
	content := `---
schema_version: "1.0"
session_id: test-session-abc
project_path: /test/project
started_at: ` + startedAt.UTC().Format(time.RFC3339) + `
status: in_progress
current_phase: D3
phases:
  - name: D1
    status: completed
  - name: D2
    status: completed
  - name: D3
    status: in_progress
---

# Wayfinder Session
`
	if err := os.WriteFile(filepath.Join(dir, "WAYFINDER-STATUS.md"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func makeV2StatusFileWithCreatedAt(t *testing.T, dir string, createdAt time.Time, completedWaypoints int) {
	t.Helper()
	var sb strings.Builder
	for range completedWaypoints {
		sb.WriteString("\n  - name: CHARTER\n    status: completed\n    started_at: " +
			createdAt.UTC().Format(time.RFC3339) + "\n")
	}
	waypoints := sb.String()
	content := `---
schema_version: "2.0"
project_name: test-project
project_type: feature
risk_level: S
current_waypoint: PROBLEM
status: in-progress
created_at: ` + createdAt.UTC().Format(time.RFC3339) + `
updated_at: ` + createdAt.UTC().Format(time.RFC3339) + `
waypoint_history:` + waypoints + `
---
`
	if err := os.WriteFile(filepath.Join(dir, "WAYFINDER-STATUS.md"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestRunEndV1_UpdatesStatusFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	startedAt := time.Now().Add(-2 * time.Hour)
	makeV1StatusFileWithPhases(t, dir, startedAt)

	if err := runEndV1(dir, "completed"); err != nil {
		t.Fatalf("runEndV1: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "WAYFINDER-STATUS.md"))
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !strings.Contains(content, "status: completed") {
		t.Errorf("expected status: completed in output, got:\n%s", content)
	}
	if !strings.Contains(content, "ended_at:") {
		t.Errorf("expected ended_at in output, got:\n%s", content)
	}
}

func TestRunEndV1_CountsCompletedPhases(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	makeV1StatusFileWithPhases(t, dir, time.Now().Add(-1*time.Hour))

	// runEndV1 should not error and should count 2 completed phases (D1, D2)
	if err := runEndV1(dir, "completed"); err != nil {
		t.Fatalf("runEndV1: %v", err)
	}
}

func TestRunEndV1_InvalidStatus(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	makeV1StatusFileWithPhases(t, dir, time.Now())

	if err := runEndV1(dir, "bogus"); err == nil {
		t.Error("expected error for invalid status, got nil")
	}
}

func TestRunEndV2_UpdatesStatusFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	createdAt := time.Now().Add(-30 * time.Minute)
	makeV2StatusFileWithCreatedAt(t, dir, createdAt, 1)

	if err := runEndV2(dir, "completed"); err != nil {
		t.Fatalf("runEndV2: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "WAYFINDER-STATUS.md"))
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !strings.Contains(content, "status: completed") {
		t.Errorf("expected status: completed in output, got:\n%s", content)
	}
}

func TestRunEndV2_ZeroCreatedAt(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	// Write a V2 file with zero/missing created_at — should not panic or produce huge duration.
	content := `---
schema_version: "2.0"
project_name: zero-date-test
project_type: feature
risk_level: S
current_waypoint: PROBLEM
status: in-progress
created_at: 0001-01-01T00:00:00Z
updated_at: 0001-01-01T00:00:00Z
---
`
	if err := os.WriteFile(filepath.Join(dir, "WAYFINDER-STATUS.md"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	// Should not error; zero CreatedAt falls back to now so duration is ~0.
	if err := runEndV2(dir, "completed"); err != nil {
		t.Fatalf("runEndV2 with zero created_at: %v", err)
	}
}

func TestFormatDuration(t *testing.T) {
	t.Parallel()
	cases := []struct {
		d    time.Duration
		want string
	}{
		{30 * time.Second, "30s"},
		{90 * time.Second, "1m"},
		{90 * time.Minute, "1h 30m"},
		{2 * time.Hour, "2h"},
		{2*time.Hour + 15*time.Minute, "2h 15m"},
	}
	for _, tc := range cases {
		got := formatDuration(tc.d)
		if got != tc.want {
			t.Errorf("formatDuration(%v) = %q, want %q", tc.d, got, tc.want)
		}
	}
}
