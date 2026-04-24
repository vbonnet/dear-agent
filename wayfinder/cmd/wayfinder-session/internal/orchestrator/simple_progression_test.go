package orchestrator

import (
	"testing"

	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/status"
)

// TestSimplePhaseProgression tests W0->D1->D2 workflow
func TestSimplePhaseProgression(t *testing.T) {
	st := createTestStatusV2()
	st.CurrentWaypoint = status.PhaseV2Charter
	addCompletedPhase(st, status.PhaseV2Charter)
	markPhaseDeliverables(st, status.PhaseV2Charter, []string{"W0-intake.md"})

	orch := NewPhaseOrchestratorV2(st)

	// W0 -> D1
	next, err := orch.AdvancePhase()
	if err != nil {
		t.Fatalf("W0->D1 failed: %v", err)
	}
	if next != status.PhaseV2Problem {
		t.Errorf("expected D1, got %s", next)
	}

	// Mark D1 deliverables
	markPhaseDeliverables(st, status.PhaseV2Problem, []string{"D1-discovery.md"})

	// D1 -> D2
	next, err = orch.AdvancePhase()
	if err != nil {
		t.Fatalf("D1->D2 failed: %v", err)
	}
	if next != status.PhaseV2Research {
		t.Errorf("expected D2, got %s", next)
	}

	if st.CurrentWaypoint != status.PhaseV2Research {
		t.Errorf("current phase should be D2, got %s", st.CurrentWaypoint)
	}
}
