package executor

import (
	"fmt"

	"github.com/vbonnet/ai-tools/autonomous-swarm/pkg/taskqueue"
)

// IterationManager tracks iteration counts and enforces limits
type IterationManager struct {
	maxIterations int
}

// NewIterationManager creates a new iteration manager
func NewIterationManager(maxIterations int) *IterationManager {
	return &IterationManager{
		maxIterations: maxIterations,
	}
}

// CheckIterationLimit checks if a bead has exceeded iteration limits
func (m *IterationManager) CheckIterationLimit(bead *taskqueue.Bead) error {
	if bead.Metadata.Iterations >= m.maxIterations {
		return NewEscalationError(
			bead.ID,
			bead.Metadata.Iterations,
			fmt.Sprintf("max iterations exceeded (%d/%d)", bead.Metadata.Iterations, m.maxIterations),
			nil,
		)
	}
	return nil
}

// IncrementIteration increments the iteration count for a bead
func (m *IterationManager) IncrementIteration(bead *taskqueue.Bead) {
	bead.Metadata.Iterations++
}
