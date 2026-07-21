package validator

import (
	"testing"

	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/status"
)

// liteSkips is the phase set a lite (XS/S) harness profile persists on the session.
var liteSkips = []string{status.PhaseV2Design, status.PhaseV2Spec, status.PhaseV2Plan}

// TestCanStartPhase_LiteSkipsToSetup verifies that under a lite profile, SETUP can be
// started directly after RESEARCH completes — the validator walks back over the
// skipped DESIGN/SPEC/PLAN phases to find RESEARCH as the gating phase (ce-12pl).
func TestCanStartPhase_LiteSkipsToSetup(t *testing.T) {
	st := &status.StatusV2{
		SchemaVersion:   status.SchemaVersion,
		CurrentWaypoint: status.PhaseV2Research,
		SkipPhases:      liteSkips,
		WaypointHistory: []status.WaypointHistory{
			{Name: status.PhaseV2Research, Status: status.WaypointStatusV2Completed},
		},
	}

	v := NewValidator(st)
	if err := v.CanStartPhase(status.PhaseV2Setup, "/tmp/test-project"); err != nil {
		t.Errorf("lite profile: expected SETUP to be startable after RESEARCH, got error: %v", err)
	}
}

// TestCanStartPhase_LiteStillGatesOnResearch verifies the skip does not remove the
// gate entirely: if the nearest non-skipped previous phase (RESEARCH) is incomplete,
// SETUP is still blocked.
func TestCanStartPhase_LiteStillGatesOnResearch(t *testing.T) {
	st := &status.StatusV2{
		SchemaVersion:   status.SchemaVersion,
		CurrentWaypoint: status.PhaseV2Research,
		SkipPhases:      liteSkips,
		WaypointHistory: []status.WaypointHistory{
			{Name: status.PhaseV2Research, Status: status.WaypointStatusV2InProgress},
		},
	}

	v := NewValidator(st)
	err := v.CanStartPhase(status.PhaseV2Setup, "/tmp/test-project")
	if err == nil {
		t.Fatal("lite profile: expected SETUP to be blocked while RESEARCH is incomplete")
	}
	if !contains(err.Error(), "previous phase RESEARCH is not completed") {
		t.Errorf("expected error to name RESEARCH as the gate, got: %v", err)
	}
}

func TestCanStartPhaseRejectsConfiguredSkippedPhase(t *testing.T) {
	tests := []struct {
		name  string
		phase string
		st    *status.StatusV2
	}{
		{
			name:  "profile skipped phase",
			phase: status.PhaseV2Design,
			st:    &status.StatusV2{SkipPhases: liteSkips},
		},
		{
			name:  "roadmap skipped phase",
			phase: status.PhaseV2Setup,
			st:    &status.StatusV2{SkipRoadmap: true},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := NewValidator(test.st).CanStartPhase(test.phase, t.TempDir())
			if err == nil || !contains(err.Error(), "phase is configured to be skipped") {
				t.Fatalf("CanStartPhase(%s) error = %v, want configured-skip rejection", test.phase, err)
			}
		})
	}
}

// TestCanStartPhase_StandardStillGatesDesign verifies the default (no skip) behavior
// is unchanged: starting SETUP requires PLAN, not RESEARCH.
func TestCanStartPhase_StandardStillGatesDesign(t *testing.T) {
	st := &status.StatusV2{
		SchemaVersion:   status.SchemaVersion,
		CurrentWaypoint: status.PhaseV2Research,
		WaypointHistory: []status.WaypointHistory{
			{Name: status.PhaseV2Research, Status: status.WaypointStatusV2Completed},
		},
	}

	v := NewValidator(st)
	err := v.CanStartPhase(status.PhaseV2Setup, "/tmp/test-project")
	if err == nil {
		t.Fatal("standard profile: expected SETUP to be blocked when PLAN is not completed")
	}
	if !contains(err.Error(), "previous phase PLAN is not completed") {
		t.Errorf("expected error to name PLAN as the gate, got: %v", err)
	}
}

// TestCanStartPhase_SkipRoadmapPreserved verifies the pre-existing skip_roadmap
// behavior survives the generalized previous-phase resolution: with skip_roadmap on,
// BUILD can start once PLAN completes (SETUP skipped).
func TestCanStartPhase_SkipRoadmapPreserved(t *testing.T) {
	st := &status.StatusV2{
		SchemaVersion:   status.SchemaVersion,
		CurrentWaypoint: status.PhaseV2Plan,
		SkipRoadmap:     true,
		WaypointHistory: []status.WaypointHistory{
			{Name: status.PhaseV2Plan, Status: status.WaypointStatusV2Completed},
		},
	}

	v := NewValidator(st)
	if err := v.CanStartPhase(status.PhaseV2Build, "/tmp/test-project"); err != nil {
		t.Errorf("skip_roadmap: expected BUILD to be startable after PLAN, got error: %v", err)
	}
}
