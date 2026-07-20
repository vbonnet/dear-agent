package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func makeV2StatusFileWithCreatedAt(t *testing.T, dir string, createdAt time.Time, completedWaypoints int) {
	t.Helper()
	var sb strings.Builder
	for range completedWaypoints {
		sb.WriteString("\n  - name: CHARTER\n    status: completed\n    started_at: " +
			createdAt.UTC().Format(time.RFC3339) + "\n    completed_at: " +
			createdAt.Add(time.Minute).UTC().Format(time.RFC3339) + "\n")
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

func TestRunEndV2_RejectsZeroCreatedAt(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	// Write malformed canonical state with zero timestamps.
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

	if err := runEndV2(dir, "completed"); err == nil || !strings.Contains(err.Error(), "created_at is required") {
		t.Fatalf("runEndV2 with zero created_at error = %v, want validation failure", err)
	}
}

func TestRunEnd_RejectsLegacyStatus(t *testing.T) {
	dir := t.TempDir()
	makeLegacyStatusFile(t, dir)

	if err := runEndInDir(dir, "completed"); err == nil {
		t.Fatal("runEnd accepted legacy status without explicit migration")
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
