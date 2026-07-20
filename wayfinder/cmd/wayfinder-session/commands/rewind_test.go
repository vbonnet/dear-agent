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
	for _, phase := range st.WaypointHistory[1:] {
		if phase.Status != status.PhaseStatusV2Pending || !phase.StartedAt.IsZero() || phase.CompletedAt != nil || phase.Outcome != nil {
			t.Errorf("rewound waypoint = %+v, want clean pending state", phase)
		}
	}
	for _, phase := range st.Roadmap.Phases[1:] {
		if phase.Status != status.PhaseStatusV2Pending || phase.StartedAt != nil || phase.CompletedAt != nil {
			t.Errorf("rewound roadmap phase = %+v, want clean pending state", phase)
		}
	}
}
