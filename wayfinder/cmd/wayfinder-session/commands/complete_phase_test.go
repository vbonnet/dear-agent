package commands

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/history"
	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/status"
)

func TestRunCompletePhaseLeavesLegacyHistoryUntouchedWhenTrackerSetupFails(t *testing.T) {
	projectDir := t.TempDir()
	previousProjectDir := projectDirectory
	previousOutcome := phaseOutcome
	projectDirectory = projectDir
	phaseOutcome = status.OutcomeSuccess
	t.Cleanup(func() {
		projectDirectory = previousProjectDir
		phaseOutcome = previousOutcome
	})

	st := status.NewStatusV2("complete-phase-guard", "service", "low")
	st.UpdatePhase(status.WaypointV2Charter, status.PhaseStatusInProgress, "")
	if err := status.WriteV2ToDir(st, projectDir); err != nil {
		t.Fatalf("seed status file: %v", err)
	}

	legacyPath := filepath.Join(projectDir, history.LegacyHistoryFilename)
	if err := os.WriteFile(legacyPath, []byte("{\"event\":\"seed\"}\n"), 0o600); err != nil {
		t.Fatalf("seed legacy history: %v", err)
	}

	notDirectory := filepath.Join(projectDir, "not-a-directory")
	if err := os.WriteFile(notDirectory, []byte("block mkdir"), 0o600); err != nil {
		t.Fatalf("seed telemetry path blocker: %v", err)
	}
	t.Setenv("ENGRAM_TELEMETRY_PATH", filepath.Join(notDirectory, "telemetry.jsonl"))

	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	if err := runCompletePhase(cmd, []string{status.WaypointV2Charter}); err == nil {
		t.Fatal("runCompletePhase succeeded despite invalid telemetry path")
	}

	if _, err := os.Stat(legacyPath); err != nil {
		t.Fatalf("failed completion migrated the legacy history file: %v", err)
	}
	if _, err := os.Stat(filepath.Join(projectDir, history.HistoryFilename)); !os.IsNotExist(err) {
		t.Fatalf("failed completion created %s (stat err: %v)", history.HistoryFilename, err)
	}
}

// TestRunCompletePhaseAdvancesSessionPointer pins that a successful completion
// moves the session forward. Before ce-2sgej, complete-phase updated
// waypoint_history and exited 0 while current_waypoint stayed parked on the
// phase that had just finished and session status stayed "planning" for the
// life of the session, so every read of WAYFINDER-STATUS.md reported a session
// that had never moved.
func TestRunCompletePhaseAdvancesSessionPointer(t *testing.T) {
	projectDir := t.TempDir()
	previousProjectDir := projectDirectory
	previousOutcome := phaseOutcome
	projectDirectory = projectDir
	phaseOutcome = status.OutcomeSuccess
	t.Cleanup(func() {
		projectDirectory = previousProjectDir
		phaseOutcome = previousOutcome
	})

	st := status.NewStatusV2("complete-phase-advance", "feature", "M")
	st.UpdatePhase(status.WaypointV2Charter, status.PhaseStatusInProgress, "")
	st.SetCurrentWaypoint(status.WaypointV2Charter)
	if err := status.WriteV2ToDir(st, projectDir); err != nil {
		t.Fatalf("seed status file: %v", err)
	}

	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	if err := runCompletePhase(cmd, []string{status.WaypointV2Charter}); err != nil {
		t.Fatalf("runCompletePhase: %v", err)
	}

	got, err := status.ParseV2FromDir(projectDir)
	if err != nil {
		t.Fatalf("re-read status: %v", err)
	}
	if want := status.WaypointV2Problem; got.GetCurrentWaypoint() != want {
		t.Errorf("current_waypoint = %q after completing CHARTER, want %q", got.GetCurrentWaypoint(), want)
	}
	if got.Status != status.StatusV2InProgress {
		t.Errorf("session status = %q after first completion, want %q", got.Status, status.StatusV2InProgress)
	}
	if phase := got.GetPhaseStatus(status.WaypointV2Charter); phase != status.PhaseStatusCompleted {
		t.Errorf("CHARTER status = %q, want %q", phase, status.PhaseStatusCompleted)
	}
	if !got.UpdatedAt.After(st.UpdatedAt) {
		t.Errorf("updated_at = %s did not move past the seeded %s", got.UpdatedAt, st.UpdatedAt)
	}
}

// TestAdvanceWaypoint pins the forward-pointer rule directly. Driving these
// cases through runCompletePhase would mean fabricating engram-hashed
// deliverable frontmatter for each phase, which measures the deliverable gate
// rather than the advance.
func TestAdvanceWaypoint(t *testing.T) {
	tests := map[string]struct {
		skip      []string
		completed string
		current   string
		want      string
	}{
		"next phase": {
			completed: status.WaypointV2Charter,
			current:   status.WaypointV2Charter,
			want:      status.WaypointV2Problem,
		},
		"steps over profile-skipped phases": {
			skip:      []string{status.WaypointV2Design, status.WaypointV2Spec, status.WaypointV2Plan},
			completed: status.WaypointV2Research,
			current:   status.WaypointV2Research,
			want:      status.WaypointV2Setup,
		},
		"holds on the final waypoint": {
			completed: status.WaypointV2Retro,
			current:   status.WaypointV2Retro,
			want:      status.WaypointV2Retro,
		},
		"leaves the pointer alone when completing an earlier phase": {
			completed: status.WaypointV2Charter,
			current:   status.WaypointV2Research,
			want:      status.WaypointV2Research,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			st := status.NewStatusV2("advance", "feature", "M")
			st.SkipPhases = tc.skip
			st.SetCurrentWaypoint(tc.current)
			st.UpdatePhase(tc.completed, status.PhaseStatusCompleted, status.OutcomeSuccess)

			got, err := advanceWaypoint(st, tc.completed)
			if err != nil {
				t.Fatalf("advanceWaypoint: %v", err)
			}
			if got != tc.want {
				t.Errorf("advanceWaypoint returned %q, want %q", got, tc.want)
			}
			if st.GetCurrentWaypoint() != tc.want {
				t.Errorf("current_waypoint = %q, want %q", st.GetCurrentWaypoint(), tc.want)
			}
		})
	}
}

// TestAdvanceWaypointRejectsUnknownWaypoint verifies an unroutable session
// fails loud instead of silently reporting a completed phase.
func TestAdvanceWaypointRejectsUnknownWaypoint(t *testing.T) {
	st := status.NewStatusV2("advance-bogus", "feature", "M")
	st.CurrentWaypoint = "NOT-A-WAYPOINT"
	st.UpdatePhase("NOT-A-WAYPOINT", status.PhaseStatusCompleted, status.OutcomeSuccess)

	if _, err := advanceWaypoint(st, "NOT-A-WAYPOINT"); err == nil {
		t.Fatal("advanceWaypoint accepted an unknown current waypoint")
	}
}
