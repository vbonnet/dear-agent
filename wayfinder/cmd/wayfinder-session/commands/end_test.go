package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	wayfinderstatus "github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/status"
)

func makeV2StatusFileWithCreatedAt(t *testing.T, dir string, createdAt time.Time, completedWaypoints int) {
	t.Helper()
	var sb strings.Builder
	waypointNames := wayfinderstatus.AllWaypointsV2Schema()
	for _, waypointName := range waypointNames[:completedWaypoints] {
		sb.WriteString("\n  - name: " + waypointName + "\n    status: completed\n    started_at: " +
			createdAt.UTC().Format(time.RFC3339) + "\n    completed_at: " +
			createdAt.Add(time.Minute).UTC().Format(time.RFC3339) + "\n")
	}
	waypoints := sb.String()
	currentWaypoint := wayfinderstatus.WaypointV2Charter
	if completedWaypoints > 0 {
		currentWaypoint = waypointNames[completedWaypoints-1]
	}
	content := `---
schema_version: "2.0"
project_name: test-project
project_type: feature
risk_level: S
current_waypoint: ` + currentWaypoint + `
status: in-progress
lifecycle_state: working
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
	makeV2StatusFileWithCreatedAt(t, dir, createdAt, len(wayfinderstatus.AllWaypointsV2Schema()))

	if err := runEndV2(dir, "completed", ""); err != nil {
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
	if !strings.Contains(content, "lifecycle_state: completed") {
		t.Errorf("expected completed lifecycle in output, got:\n%s", content)
	}
}

func TestRunEndV2_RejectsIncompleteWorkflow(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	createdAt := time.Now().Add(-30 * time.Minute)
	makeV2StatusFileWithCreatedAt(t, dir, createdAt, 1)

	err := runEndV2(dir, "completed", "")
	if err == nil || !strings.Contains(err.Error(), "required Wayfinder phases are incomplete") {
		t.Fatalf("runEndV2 incomplete workflow error = %v, want completion guard", err)
	}

	data, readErr := os.ReadFile(filepath.Join(dir, "WAYFINDER-STATUS.md"))
	if readErr != nil {
		t.Fatal(readErr)
	}
	if strings.Contains(string(data), "lifecycle_state: completed") {
		t.Fatalf("incomplete workflow was marked completed:\n%s", data)
	}
}

func TestValidateSessionCompletionHonorsConfiguredSkips(t *testing.T) {
	now := time.Now()
	st := &wayfinderstatus.StatusV2{
		SkipPhases:  []string{wayfinderstatus.WaypointV2Design, wayfinderstatus.WaypointV2Spec, wayfinderstatus.WaypointV2Plan},
		SkipRoadmap: true,
	}
	for _, waypointName := range wayfinderstatus.AllWaypointsV2Schema() {
		if st.IsPhaseSkipped(waypointName) {
			continue
		}
		st.WaypointHistory = append(st.WaypointHistory, wayfinderstatus.WaypointHistory{
			Name:        waypointName,
			Status:      wayfinderstatus.WaypointStatusV2Completed,
			StartedAt:   now,
			CompletedAt: &now,
		})
	}

	if err := wayfinderstatus.ValidateSessionCompletion(st); err != nil {
		t.Fatalf("ValidateSessionCompletion() rejected configured skips: %v", err)
	}
	st.WaypointHistory = st.WaypointHistory[:len(st.WaypointHistory)-1]
	if err := wayfinderstatus.ValidateSessionCompletion(st); err == nil || !strings.Contains(err.Error(), wayfinderstatus.WaypointV2Retro) {
		t.Fatalf("ValidateSessionCompletion() error = %v, want missing RETRO", err)
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

	if err := runEndV2(dir, "completed", ""); err == nil || !strings.Contains(err.Error(), "created_at is required") {
		t.Fatalf("runEndV2 with zero created_at error = %v, want validation failure", err)
	}
}

func TestRunEndV2_BlockedRequiresAndPersistsReason(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	createdAt := time.Now().Add(-30 * time.Minute)
	makeV2StatusFileWithCreatedAt(t, dir, createdAt, 1)

	for _, reason := range []string{"", "  \t\n  "} {
		if err := runEndV2(dir, "blocked", reason); err == nil || !strings.Contains(err.Error(), "requires --reason") {
			t.Fatalf("runEndV2 with blocked reason %q error = %v, want required reason", reason, err)
		}
	}
	if err := runEndV2(dir, "blocked", "  waiting for reviewer  "); err != nil {
		t.Fatalf("runEndV2 blocked: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "WAYFINDER-STATUS.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "blocked_reason: waiting for reviewer") {
		t.Fatalf("blocked reason missing from status:\n%s", data)
	}
	if strings.Contains(string(data), "lifecycle_state:") {
		t.Fatalf("generic blocked end retained a conflicting lifecycle state:\n%s", data)
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
