package commands

import (
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/status"
)

func TestResetForRewindResetsTargetAndLaterPhases(t *testing.T) {
	now := time.Now()
	outcome := status.OutcomeSuccess
	st := &status.StatusV2{
		WaypointHistory: []status.WaypointHistory{
			{Name: status.WaypointV2Research, Status: status.PhaseStatusV2Completed, StartedAt: now, CompletedAt: &now, Outcome: &outcome},
			{Name: status.WaypointV2Design, Status: status.PhaseStatusV2Completed, StartedAt: now, CompletedAt: &now, Outcome: &outcome},
			{Name: status.WaypointV2Spec, Status: status.PhaseStatusV2Completed, StartedAt: now, CompletedAt: &now, Outcome: &outcome},
		},
		Roadmap: &status.Roadmap{Phases: []status.RoadmapPhase{
			{ID: status.WaypointV2Research, Status: status.PhaseStatusV2Completed, StartedAt: &now, CompletedAt: &now},
			{ID: status.WaypointV2Design, Status: status.PhaseStatusV2Completed, StartedAt: &now, CompletedAt: &now},
			{ID: status.WaypointV2Spec, Status: status.PhaseStatusV2Completed, StartedAt: &now, CompletedAt: &now},
		}},
	}
	phases := status.AllPhasesV2Schema()
	resetForRewind(st, phases, 3)

	if got := st.WaypointHistory[0].Status; got != status.PhaseStatusV2Completed {
		t.Fatalf("earlier phase status = %q, want completed", got)
	}
	if got := len(st.WaypointHistory); got != 1 {
		t.Fatalf("waypoint history length = %d, want only the earlier completed entry", got)
	}
	for _, phase := range st.Roadmap.Phases[1:] {
		if phase.Status != status.PhaseStatusV2Pending || phase.StartedAt != nil || phase.CompletedAt != nil {
			t.Errorf("rewound roadmap phase = %+v, want clean pending state", phase)
		}
	}
	st.UpdatePhase(status.WaypointV2Design, status.PhaseStatusV2InProgress, "")
	restarted := st.GetPhaseHistory(status.WaypointV2Design)
	if restarted == nil || restarted.StartedAt.IsZero() {
		t.Fatalf("restarted target history = %+v, want a valid start timestamp", restarted)
	}
}

func TestResetLifecycleForRewindReopensTerminalStatus(t *testing.T) {
	completedAt := time.Now().Add(-time.Hour)
	rewoundAt := time.Now()
	st := &status.StatusV2{
		Status:         status.StatusV2Completed,
		LifecycleState: status.LifecycleCompleted,
		CompletionDate: &completedAt,
		BlockedReason:  "stale block",
		BlockedOn:      "stale-dependency",
		ErrorMessage:   "stale error",
		InputNeeded:    "stale input",
	}

	resetLifecycleForRewind(st, rewoundAt)

	if st.Status != status.StatusV2InProgress || st.LifecycleState != status.LifecycleWorking {
		t.Fatalf("rewound lifecycle = %q/%q, want in-progress/working", st.Status, st.LifecycleState)
	}
	if st.CompletionDate != nil || st.BlockedReason != "" || st.BlockedOn != "" || st.ErrorMessage != "" || st.InputNeeded != "" {
		t.Fatalf("rewound lifecycle retained terminal metadata: %+v", st)
	}
	if !st.UpdatedAt.Equal(rewoundAt) {
		t.Fatalf("updated_at = %s, want %s", st.UpdatedAt, rewoundAt)
	}
}

func TestValidateRewindTargetRejectsConfiguredSkip(t *testing.T) {
	now := time.Now()
	st := &status.StatusV2{
		SkipPhases: []string{status.WaypointV2Design},
		WaypointHistory: []status.WaypointHistory{
			{Name: status.WaypointV2Design, Status: status.PhaseStatusV2Skipped, StartedAt: now},
		},
	}

	err := validateRewindTarget(st, status.WaypointV2Design)
	if err == nil || err.Error() != "cannot rewind to phase DESIGN: phase is configured to be skipped" {
		t.Fatalf("validateRewindTarget() error = %v, want configured-skip rejection", err)
	}
}
