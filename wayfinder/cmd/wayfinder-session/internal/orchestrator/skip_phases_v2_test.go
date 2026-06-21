package orchestrator

import (
	"testing"

	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/status"
)

// liteSkipPhases mirrors ProfileLite (XS/S risk): DESIGN/SPEC/PLAN are skipped.
var liteSkipPhases = []string{status.PhaseV2Design, status.PhaseV2Spec, status.PhaseV2Plan}

// TestGetNextPhase_LiteProfileSkips verifies the orchestrator advances past the
// profile-skipped phases (DESIGN/SPEC/PLAN) so RESEARCH jumps straight to SETUP.
func TestGetNextPhase_LiteProfileSkips(t *testing.T) {
	testCases := []struct {
		current  string
		expected string
	}{
		{status.PhaseV2Charter, status.PhaseV2Problem},  // unaffected
		{status.PhaseV2Problem, status.PhaseV2Research}, // unaffected
		{status.PhaseV2Research, status.PhaseV2Setup},   // skips DESIGN/SPEC/PLAN
		{status.PhaseV2Setup, status.PhaseV2Build},      // unaffected
		{status.PhaseV2Build, status.PhaseV2Retro},      // unaffected
	}

	for _, tc := range testCases {
		t.Run(tc.current+"->"+tc.expected, func(t *testing.T) {
			st := createTestStatusV2()
			st.CurrentWaypoint = tc.current
			st.SkipPhases = liteSkipPhases

			orch := NewPhaseOrchestratorV2(st)

			next, err := orch.GetNextPhase()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if next != tc.expected {
				t.Errorf("expected %s, got %s", tc.expected, next)
			}
		})
	}
}

// TestGetNextPhase_FullProfileRunsAllPhases verifies that with no SkipPhases set
// (standard/deep profiles) every phase is still traversed one step at a time.
func TestGetNextPhase_FullProfileRunsAllPhases(t *testing.T) {
	testCases := []struct {
		current  string
		expected string
	}{
		{status.PhaseV2Research, status.PhaseV2Design}, // NOT skipped under full profile
		{status.PhaseV2Design, status.PhaseV2Spec},
		{status.PhaseV2Spec, status.PhaseV2Plan},
		{status.PhaseV2Plan, status.PhaseV2Setup},
	}

	for _, tc := range testCases {
		t.Run(tc.current+"->"+tc.expected, func(t *testing.T) {
			st := createTestStatusV2()
			st.CurrentWaypoint = tc.current
			st.SkipPhases = nil // full profile

			orch := NewPhaseOrchestratorV2(st)

			next, err := orch.GetNextPhase()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if next != tc.expected {
				t.Errorf("expected %s, got %s", tc.expected, next)
			}
		})
	}
}

// TestAdvancePhase_LiteProfileSkipsToSetup verifies the production advance path
// honors SkipPhases: a lite-profile RESEARCH advances directly to SETUP without
// being blocked by the phase-skipping guard, and DESIGN/SPEC/PLAN never become
// the current phase.
func TestAdvancePhase_LiteProfileSkipsToSetup(t *testing.T) {
	st := createTestStatusV2()
	st.CurrentWaypoint = status.PhaseV2Research
	st.SkipPhases = liteSkipPhases
	// RESEARCH exit criteria require its deliverable.
	markPhaseDeliverables(st, status.PhaseV2Research, []string{"D2-investigation.md"})

	orch := NewPhaseOrchestratorV2(st)

	next, err := orch.AdvancePhase()
	if err != nil {
		t.Fatalf("unexpected error advancing lite profile: %v", err)
	}
	if next != status.PhaseV2Setup {
		t.Fatalf("expected advance to SETUP (skipping DESIGN/SPEC/PLAN), got %s", next)
	}
	if st.CurrentWaypoint != status.PhaseV2Setup {
		t.Errorf("status not updated to SETUP, got %s", st.CurrentWaypoint)
	}

	// None of the skipped phases should have been recorded as a current/visited phase.
	for _, h := range st.WaypointHistory {
		if h.Name == status.PhaseV2Design || h.Name == status.PhaseV2Spec || h.Name == status.PhaseV2Plan {
			t.Errorf("skipped phase %s should not appear in waypoint history", h.Name)
		}
	}
}

// TestValidateTransition_LiteProfileAllowsSkip verifies the skip-aware transition
// validation: under ProfileLite, RESEARCH->SETUP is a legitimate advance, but the
// same jump is blocked under the full profile.
func TestValidateTransition_LiteProfileAllowsSkip(t *testing.T) {
	t.Run("lite profile allows RESEARCH->SETUP", func(t *testing.T) {
		st := createTestStatusV2()
		st.CurrentWaypoint = status.PhaseV2Research
		st.SkipPhases = liteSkipPhases

		orch := NewPhaseOrchestratorV2(st)
		if err := orch.validateTransition(status.PhaseV2Research, status.PhaseV2Setup); err != nil {
			t.Errorf("expected RESEARCH->SETUP allowed under lite profile, got: %v", err)
		}
	})

	t.Run("full profile blocks RESEARCH->SETUP", func(t *testing.T) {
		st := createTestStatusV2()
		st.CurrentWaypoint = status.PhaseV2Research
		st.SkipPhases = nil

		orch := NewPhaseOrchestratorV2(st)
		err := orch.validateTransition(status.PhaseV2Research, status.PhaseV2Setup)
		if err == nil {
			t.Error("expected RESEARCH->SETUP blocked under full profile, got nil")
		} else if !contains(err.Error(), "cannot skip phases") {
			t.Errorf("expected phase-skipping error, got: %v", err)
		}
	})

	t.Run("lite profile still blocks skipping a non-skipped phase", func(t *testing.T) {
		// SETUP is never skipped, so RESEARCH->BUILD must remain blocked even
		// when DESIGN/SPEC/PLAN are skipped.
		st := createTestStatusV2()
		st.CurrentWaypoint = status.PhaseV2Research
		st.SkipPhases = liteSkipPhases

		orch := NewPhaseOrchestratorV2(st)
		if err := orch.validateTransition(status.PhaseV2Research, status.PhaseV2Build); err == nil {
			t.Error("expected RESEARCH->BUILD blocked (SETUP not skipped), got nil")
		}
	})
}
