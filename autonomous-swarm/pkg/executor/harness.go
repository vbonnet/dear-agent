package executor

import (
	"fmt"

	"github.com/vbonnet/ai-tools/autonomous-swarm/pkg/agm"
	"github.com/vbonnet/ai-tools/autonomous-swarm/pkg/taskqueue"
)

// Harness manages agent execution lifecycle
type Harness struct {
	coordinator   *taskqueue.Coordinator
	orchestrator  *csm.Orchestrator
	prompter      *csm.Prompter
	maxIterations int
}

// NewHarness creates a new execution harness
func NewHarness(coordinator *taskqueue.Coordinator, orchestrator *csm.Orchestrator, prompter *csm.Prompter, maxIterations int) *Harness {
	return &Harness{
		coordinator:   coordinator,
		orchestrator:  orchestrator,
		prompter:      prompter,
		maxIterations: maxIterations,
	}
}

// ExecuteBead executes a single bead through the full lifecycle
func (h *Harness) ExecuteBead(beadID string, sessionName string) error {
	// 1. Claim bead from queue
	if err := h.coordinator.Claim(beadID, sessionName); err != nil {
		return fmt.Errorf("failed to claim bead %s: %w", beadID, err)
	}

	// Save queue state
	if err := h.coordinator.Save(); err != nil {
		return fmt.Errorf("failed to save queue after claim: %w", err)
	}

	// 2. Create AGM session
	if err := h.orchestrator.Create(sessionName); err != nil {
		return fmt.Errorf("failed to create session %s: %w", sessionName, err)
	}

	// 3. Wait for session to be ready
	if err := h.orchestrator.WaitForSession(sessionName, 5*1000000000); err != nil {
		_ = h.orchestrator.Archive(sessionName)
		return fmt.Errorf("session %s not ready: %w", sessionName, err)
	}

	// 4. Inject bead prompt
	if err := h.prompter.InjectPrompt(sessionName, beadID, "Execute bead"); err != nil {
		_ = h.orchestrator.Archive(sessionName)
		return fmt.Errorf("failed to inject prompt: %w", err)
	}

	// 5. Monitor session health (placeholder - Phase 0B enhancement)
	// TODO: Implement heartbeat monitoring

	// 6. Extract results (session UUID)
	uuid, err := h.orchestrator.Extract(sessionName)
	if err != nil {
		_ = h.orchestrator.Archive(sessionName)
		return fmt.Errorf("failed to extract session UUID: %w", err)
	}

	// 7. Archive session
	if err := h.orchestrator.Archive(sessionName); err != nil {
		return fmt.Errorf("failed to archive session: %w", err)
	}

	// 8. Validate results (placeholder - Phase 5)
	// TODO: Call validation engine
	_ = uuid

	// 9. Complete bead
	if err := h.coordinator.Complete(beadID); err != nil {
		return fmt.Errorf("failed to complete bead: %w", err)
	}

	// Save final queue state
	if err := h.coordinator.Save(); err != nil {
		return fmt.Errorf("failed to save queue after complete: %w", err)
	}

	return nil
}
