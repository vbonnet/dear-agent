package commands

import (
	"testing"

	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/status"
)

func TestRemainingPhaseStatusHonorsConfiguredSkips(t *testing.T) {
	currentStatus := &status.StatusV2{
		SkipPhases:  []string{status.WaypointV2Design, status.WaypointV2Spec, status.WaypointV2Plan},
		SkipRoadmap: true,
	}

	for _, phase := range []string{status.WaypointV2Design, status.WaypointV2Spec, status.WaypointV2Plan, status.WaypointV2Setup} {
		if got := remainingPhaseStatus(currentStatus, phase); got != "(skipped)" {
			t.Errorf("remainingPhaseStatus(%s) = %q, want %q", phase, got, "(skipped)")
		}
	}
	if got := remainingPhaseStatus(currentStatus, status.WaypointV2Build); got != "(pending)" {
		t.Errorf("remainingPhaseStatus(BUILD) = %q, want %q", got, "(pending)")
	}
}
