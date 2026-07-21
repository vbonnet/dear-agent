package validator

import (
	"testing"

	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/status"
)

func TestCanonicalValidatorBlocksBuildWithoutDeliverable(t *testing.T) {
	t.Parallel()

	v := NewValidator(&status.StatusV2{
		SchemaVersion:   status.SchemaVersion,
		CurrentWaypoint: status.PhaseV2Build,
		WaypointHistory: []status.WaypointHistory{
			{Name: status.PhaseV2Build, Status: status.PhaseStatusInProgress},
		},
	})

	err := v.CanCompletePhase(status.PhaseV2Build, t.TempDir(), "")
	if err == nil {
		t.Fatal("expected BUILD completion to fail without a deliverable")
	}
	if !contains(err.Error(), "no deliverable file found") {
		t.Fatalf("expected missing-deliverable error, got %v", err)
	}
}

func TestCanonicalValidatorBlocksDesignWithoutResearchEvidence(t *testing.T) {
	t.Parallel()

	v := NewValidator(&status.StatusV2{
		SchemaVersion:   status.SchemaVersion,
		CurrentWaypoint: status.PhaseV2Research,
		WaypointHistory: []status.WaypointHistory{
			{Name: status.PhaseV2Research, Status: status.PhaseStatusCompleted},
		},
	})

	err := v.CanStartPhase(status.PhaseV2Design, t.TempDir())
	if err == nil {
		t.Fatal("expected DESIGN start to fail without RESEARCH evidence")
	}
	if !contains(err.Error(), "RESEARCH-existing-solutions.md does not exist") {
		t.Fatalf("expected missing-research error, got %v", err)
	}
}
