package executor

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/vbonnet/ai-tools/autonomous-swarm/pkg/agm"
	"github.com/vbonnet/ai-tools/autonomous-swarm/pkg/taskqueue"
)

func TestNewHarness(t *testing.T) {
	coord := taskqueue.NewCoordinator("test.yaml")
	orch := csm.NewOrchestrator()
	prompter := csm.NewPrompter()

	harness := NewHarness(coord, orch, prompter, 3)

	if harness == nil {
		t.Fatal("NewHarness should not return nil")
	}

	if harness.maxIterations != 3 {
		t.Errorf("maxIterations: got %d, want 3", harness.maxIterations)
	}
}

func TestExecuteBead_Validation(t *testing.T) {
	// This is a placeholder test - full integration testing requires AGM
	coord := taskqueue.NewCoordinator("nonexistent.yaml")
	orch := csm.NewOrchestrator()
	prompter := csm.NewPrompter()

	harness := NewHarness(coord, orch, prompter, 3)

	// Execute will fail because queue doesn't exist, but tests struct is wired correctly
	err := harness.ExecuteBead("bead-1", "test-session")
	if err == nil {
		t.Error("ExecuteBead should fail with nonexistent queue")
	}
}

func TestExecuteBead_ClaimFailure(t *testing.T) {
	// Create empty task queue file
	tmpDir := t.TempDir()
	queueFile := filepath.Join(tmpDir, "queue.yaml")

	queueData := `schema_version: "1.0.0"
last_updated: 2024-01-01T00:00:00Z
ready: []
in_progress: []
blocked: []
completed: []
`
	if err := os.WriteFile(queueFile, []byte(queueData), 0644); err != nil {
		t.Fatalf("failed to create test queue: %v", err)
	}

	coord := taskqueue.NewCoordinator(queueFile)
	if err := coord.Load(); err != nil {
		t.Fatalf("failed to load test queue: %v", err)
	}

	orch := csm.NewOrchestrator()
	prompter := csm.NewPrompter()
	harness := NewHarness(coord, orch, prompter, 3)

	// Try to claim bead that doesn't exist
	err := harness.ExecuteBead("nonexistent-bead", "test-session")
	if err == nil {
		t.Error("ExecuteBead should fail when bead doesn't exist in ready section")
	}
}

func TestExecuteBead_FieldValidation(t *testing.T) {
	tests := []struct {
		name        string
		beadID      string
		sessionName string
		wantError   bool
	}{
		{"empty bead ID", "", "session", true},
		{"empty session name", "bead-1", "", true},
		{"valid inputs but no queue", "bead-1", "session", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			coord := taskqueue.NewCoordinator("nonexistent.yaml")
			orch := csm.NewOrchestrator()
			prompter := csm.NewPrompter()
			harness := NewHarness(coord, orch, prompter, 3)

			err := harness.ExecuteBead(tt.beadID, tt.sessionName)

			if tt.wantError && err == nil {
				t.Error("expected error but got nil")
			}

			if !tt.wantError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}
