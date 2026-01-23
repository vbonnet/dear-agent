package launcher

import (
	"fmt"
	"log"

	"github.com/[REDACTED_EMPLOYER]-src/ai-tools/autonomous-swarm/pkg/taskqueue"
)

// Orchestrator manages dependency-aware bead launching.
// It ensures beads are launched in topological order, respecting dependencies.
type Orchestrator struct {
	coordinator *taskqueue.Coordinator
}

// NewOrchestrator creates a new launch orchestrator.
func NewOrchestrator(coordinator *taskqueue.Coordinator) *Orchestrator {
	return &Orchestrator{
		coordinator: coordinator,
	}
}

// LaunchReady launches all ready beads in dependency order.
// It builds a dependency graph, performs topological sort, and launches beads sequentially.
// Returns error if circular dependency detected or coordinator operations fail.
func (o *Orchestrator) LaunchReady() error {
	// Load current queue state
	queue := o.coordinator.GetQueue()
	if queue == nil {
		return fmt.Errorf("failed to get queue from coordinator")
	}

	// If no ready beads, nothing to do
	if len(queue.Ready) == 0 {
		log.Printf("No beads ready for launch")
		return nil
	}

	// Build dependency graph from ready beads
	graph, err := BuildGraph(queue.Ready)
	if err != nil {
		return fmt.Errorf("failed to build dependency graph: %w", err)
	}

	// Perform topological sort to get launch order
	launchOrder, err := graph.TopologicalSort()
	if err != nil {
		// Circular dependency detected
		return fmt.Errorf("cannot launch beads: %w", err)
	}

	log.Printf("Launch order determined: %v", launchOrder)

	// Launch beads in order
	var failedBeads []string
	for _, beadID := range launchOrder {
		if err := o.launchBead(beadID); err != nil {
			log.Printf("Warning: Failed to launch bead %s: %v", beadID, err)
			failedBeads = append(failedBeads, beadID)
			// Continue launching other beads (partial failure allowed)
		}
	}

	// Report partial failures if any
	if len(failedBeads) > 0 {
		log.Printf("Warning: %d beads failed to launch: %v", len(failedBeads), failedBeads)
		// Return error if all beads failed, otherwise continue
		if len(failedBeads) == len(launchOrder) {
			return fmt.Errorf("all beads failed to launch")
		}
	}

	return nil
}

// launchBead attempts to claim and launch a single bead.
// Returns error if claim fails or launch fails.
func (o *Orchestrator) launchBead(beadID string) error {
	// Generate session name for this bead
	sessionName := fmt.Sprintf("agent-%s", beadID)

	// Attempt to claim bead (validates dependencies)
	if err := o.coordinator.Claim(beadID, sessionName); err != nil {
		return fmt.Errorf("failed to claim bead: %w", err)
	}

	// Save queue state after claim
	if err := o.coordinator.Save(); err != nil {
		return fmt.Errorf("failed to save queue after claim: %w", err)
	}

	log.Printf("Successfully claimed bead %s for session %s", beadID, sessionName)

	// TODO: Actually spawn agent process here
	// For now, this is a placeholder - actual agent spawning would happen in integration
	// Example:
	// cmd := exec.Command("swarm-executor", "--bead-id", beadID, "--session", sessionName)
	// if err := cmd.Start(); err != nil {
	//     return fmt.Errorf("failed to spawn agent: %w", err)
	// }

	log.Printf("Agent launched for bead %s (placeholder - actual spawn TBD)", beadID)

	return nil
}
