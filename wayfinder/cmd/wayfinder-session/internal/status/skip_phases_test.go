package status

import "testing"

// liteSkips is the phase set ProfileLite (XS/S risk) skips. Kept local to the test
// so the behavioral expectation is spelled out explicitly rather than imported.
var liteSkips = []string{WaypointV2Design, WaypointV2Spec, WaypointV2Plan}

// TestNextWaypoint_LiteSkipsDesignSpecPlan verifies that the V2 navigation layer
// advances past the phases a lite profile skips. After RESEARCH completes, the next
// phase for a lite task is SETUP — not DESIGN (ce-12pl / MVD T2.2).
func TestNextWaypoint_LiteSkipsDesignSpecPlan(t *testing.T) {
	s := &StatusV2{
		SchemaVersion:   SchemaVersion,
		CurrentWaypoint: WaypointV2Research,
		SkipPhases:      liteSkips,
		WaypointHistory: []WaypointHistory{
			{Name: WaypointV2Research, Status: WaypointStatusV2Completed},
		},
	}

	next, err := s.NextWaypoint()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if next != WaypointV2Setup {
		t.Errorf("lite profile: expected next waypoint %q after RESEARCH, got %q",
			WaypointV2Setup, next)
	}
}

// TestNextWaypoint_StandardKeepsDesign verifies the default (no skip) behavior is
// unchanged: after RESEARCH, the next phase is DESIGN.
func TestNextWaypoint_StandardKeepsDesign(t *testing.T) {
	s := &StatusV2{
		SchemaVersion:   SchemaVersion,
		CurrentWaypoint: WaypointV2Research,
		WaypointHistory: []WaypointHistory{
			{Name: WaypointV2Research, Status: WaypointStatusV2Completed},
		},
	}

	next, err := s.NextWaypoint()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if next != WaypointV2Design {
		t.Errorf("standard profile: expected next waypoint %q after RESEARCH, got %q",
			WaypointV2Design, next)
	}
}

func TestNextWaypointSkipRoadmapAdvancesFromPlanToBuild(t *testing.T) {
	s := &StatusV2{
		SchemaVersion:   SchemaVersion,
		CurrentWaypoint: WaypointV2Plan,
		SkipRoadmap:     true,
		WaypointHistory: []WaypointHistory{
			{Name: WaypointV2Plan, Status: WaypointStatusV2Completed},
		},
	}

	next, err := s.NextWaypoint()
	if err != nil {
		t.Fatal(err)
	}
	if next != WaypointV2Build {
		t.Fatalf("next waypoint = %q, want BUILD when SETUP is skipped", next)
	}
}

// TestNextWaypoint_LiteFullProgression walks an entire lite session and asserts the
// skipped phases never surface and the sequence terminates correctly at RETRO.
func TestNextWaypoint_LiteFullProgression(t *testing.T) {
	expectedSequence := []string{
		WaypointV2Charter,
		WaypointV2Problem,
		WaypointV2Research,
		// DESIGN, SPEC, PLAN skipped by the lite profile.
		WaypointV2Setup,
		WaypointV2Build,
		WaypointV2Retro,
	}

	s := &StatusV2{
		SchemaVersion:   SchemaVersion,
		CurrentWaypoint: "",
		SkipPhases:      liteSkips,
		WaypointHistory: []WaypointHistory{},
	}

	for i, expected := range expectedSequence {
		next, err := s.NextWaypoint()
		if err != nil {
			t.Fatalf("unexpected error at step %d: %v", i, err)
		}
		if next != expected {
			t.Errorf("step %d: expected %q, got %q", i, expected, next)
		}
		if s.IsPhaseSkipped(next) {
			t.Errorf("step %d: skipped phase %q should never be returned", i, next)
		}

		if i < len(expectedSequence)-1 {
			s.CurrentWaypoint = next
			s.UpdateWaypoint(next, WaypointStatusV2Completed, OutcomeSuccess)
		}
	}

	// RETRO is terminal even under the lite profile.
	s.CurrentWaypoint = WaypointV2Retro
	s.UpdateWaypoint(WaypointV2Retro, WaypointStatusV2Completed, OutcomeSuccess)
	if _, err := s.NextWaypoint(); err == nil {
		t.Error("expected error when advancing from RETRO, got none")
	}
}

// TestIsPhaseSkipped covers the membership helper directly.
func TestIsPhaseSkipped(t *testing.T) {
	s := &StatusV2{SkipPhases: liteSkips}

	for _, skipped := range liteSkips {
		if !s.IsPhaseSkipped(skipped) {
			t.Errorf("IsPhaseSkipped(%q) = false, want true", skipped)
		}
	}
	for _, kept := range []string{WaypointV2Charter, WaypointV2Research, WaypointV2Setup, WaypointV2Build, WaypointV2Retro} {
		if s.IsPhaseSkipped(kept) {
			t.Errorf("IsPhaseSkipped(%q) = true, want false", kept)
		}
	}

	// A nil/empty skip set (standard profile) skips nothing.
	empty := &StatusV2{}
	if empty.IsPhaseSkipped(WaypointV2Design) {
		t.Error("empty SkipPhases should skip nothing")
	}

	skipRoadmap := &StatusV2{SkipRoadmap: true}
	if !skipRoadmap.IsPhaseSkipped(WaypointV2Setup) {
		t.Error("SkipRoadmap should skip SETUP")
	}
	if skipRoadmap.IsPhaseSkipped(WaypointV2Build) {
		t.Error("SkipRoadmap must not skip BUILD")
	}
}
