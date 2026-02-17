package executor

import (
	"testing"

	"github.com/vbonnet/ai-tools/autonomous-swarm/pkg/taskqueue"
)

func TestNewIterationManager(t *testing.T) {
	manager := NewIterationManager(3)

	if manager == nil {
		t.Fatal("NewIterationManager should not return nil")
	}

	if manager.maxIterations != 3 {
		t.Errorf("maxIterations: got %d, want 3", manager.maxIterations)
	}
}

func TestCheckIterationLimit(t *testing.T) {
	tests := []struct {
		name          string
		maxIterations int
		currentCount  int
		wantError     bool
		errorType     ErrorType
	}{
		{"under limit", 3, 0, false, 0},
		{"at limit-1", 3, 2, false, 0},
		{"at limit", 3, 3, true, ErrorEscalation},
		{"over limit", 3, 4, true, ErrorEscalation},
		{"max 1", 1, 1, true, ErrorEscalation},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager := NewIterationManager(tt.maxIterations)
			bead := &taskqueue.Bead{
				ID: "test-bead",
				Metadata: taskqueue.BeadMetadata{
					Iterations: tt.currentCount,
				},
			}

			err := manager.CheckIterationLimit(bead)

			if tt.wantError && err == nil {
				t.Errorf("expected error but got nil")
			}

			if !tt.wantError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			if tt.wantError && err != nil {
				execErr, ok := err.(*ExecutionError)
				if !ok {
					t.Errorf("expected *ExecutionError, got %T", err)
				} else if execErr.Type != tt.errorType {
					t.Errorf("error type: got %v, want %v", execErr.Type, tt.errorType)
				}
			}
		})
	}
}

func TestIncrementIteration(t *testing.T) {
	manager := NewIterationManager(3)
	bead := &taskqueue.Bead{
		ID: "test-bead",
		Metadata: taskqueue.BeadMetadata{
			Iterations: 0,
		},
	}

	manager.IncrementIteration(bead)
	if bead.Metadata.Iterations != 1 {
		t.Errorf("after increment: got %d, want 1", bead.Metadata.Iterations)
	}

	manager.IncrementIteration(bead)
	if bead.Metadata.Iterations != 2 {
		t.Errorf("after second increment: got %d, want 2", bead.Metadata.Iterations)
	}
}
