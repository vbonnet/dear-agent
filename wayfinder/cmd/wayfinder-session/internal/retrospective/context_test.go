package retrospective

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/status"
)

func TestCaptureContext(t *testing.T) {
	// Create temp directory for testing
	tmpDir := t.TempDir()

	// Create mock status
	st := &status.Status{
		SessionID:    "test-session-123",
		CurrentPhase: "S7",
		Phases: []status.Phase{
			{Name: "W0", Status: status.PhaseStatusCompleted},
			{Name: "D1", Status: status.PhaseStatusCompleted},
			{Name: "S7", Status: status.PhaseStatusInProgress},
		},
	}

	// Create mock deliverables
	os.WriteFile(filepath.Join(tmpDir, "W0-PROJECT-CHARTER.md"), []byte("test"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "D1-problem-validation.md"), []byte("test"), 0644)

	// Capture context (git will fail in tmpDir, that's expected)
	snapshot := CaptureContext(tmpDir, st)

	// Validate phase context
	if snapshot.PhaseState.CurrentPhase != "S7" {
		t.Errorf("Expected CurrentPhase S7, got %s", snapshot.PhaseState.CurrentPhase)
	}
	if snapshot.PhaseState.SessionID != "test-session-123" {
		t.Errorf("Expected SessionID test-session-123, got %s", snapshot.PhaseState.SessionID)
	}
	if len(snapshot.PhaseState.CompletedPhases) != 2 {
		t.Errorf("Expected 2 completed phases, got %d", len(snapshot.PhaseState.CompletedPhases))
	}

	// Validate deliverables
	if len(snapshot.Deliverables) != 2 {
		t.Errorf("Expected 2 deliverables, got %d", len(snapshot.Deliverables))
	}

	// Git context will have error (not a git repo), that's acceptable
	if snapshot.Git.Error == "" {
		// If it succeeded, validate fields
		if snapshot.Git.Branch == "" {
			t.Errorf("Expected git branch, got empty")
		}
	}
}

func TestCapturePhaseContext(t *testing.T) {
	st := &status.Status{
		SessionID:    "test-123",
		CurrentPhase: "D2",
		Phases: []status.Phase{
			{Name: "W0", Status: status.PhaseStatusCompleted},
			{Name: "D1", Status: status.PhaseStatusCompleted},
			{Name: "D2", Status: status.PhaseStatusInProgress},
			{Name: "D3", Status: status.PhaseStatusPending},
		},
	}

	phaseCtx := capturePhaseContext(st)

	if phaseCtx.CurrentPhase != "D2" {
		t.Errorf("Expected CurrentPhase D2, got %s", phaseCtx.CurrentPhase)
	}
	if len(phaseCtx.CompletedPhases) != 2 {
		t.Errorf("Expected 2 completed phases, got %d", len(phaseCtx.CompletedPhases))
	}
	if phaseCtx.CompletedPhases[0] != "W0" {
		t.Errorf("Expected first completed phase W0, got %s", phaseCtx.CompletedPhases[0])
	}
}

func TestCaptureDeliverables(t *testing.T) {
	tmpDir := t.TempDir()

	// Create mock deliverables
	os.WriteFile(filepath.Join(tmpDir, "W0-charter.md"), []byte("test"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "D1-problem.md"), []byte("test"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "S6-design.md"), []byte("test"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "README.md"), []byte("test"), 0644) // Should be ignored

	deliverables, err := captureDeliverables(tmpDir)
	if err != nil {
		t.Fatalf("captureDeliverables failed: %v", err)
	}

	if len(deliverables) != 3 {
		t.Errorf("Expected 3 deliverables, got %d: %v", len(deliverables), deliverables)
	}
}
