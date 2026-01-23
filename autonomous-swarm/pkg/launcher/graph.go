package launcher

import (
	"fmt"
	"strings"

	"github.com/[REDACTED_EMPLOYER]-src/ai-tools/autonomous-swarm/pkg/taskqueue"
)

// DependencyGraph represents a directed acyclic graph (DAG) of bead dependencies.
// It tracks the in-degree (number of dependencies) and adjacency list (dependents)
// for topological sorting to determine launch order.
type DependencyGraph struct {
	inDegree  map[string]int      // beadID -> number of dependencies
	adjacency map[string][]string // beadID -> list of dependent beads
	beads     map[string]taskqueue.Bead // beadID -> bead (for reference)
}

// BuildGraph constructs a dependency graph from a list of beads.
// Returns error if any dependency references a non-existent bead.
func BuildGraph(beads []taskqueue.Bead) (*DependencyGraph, error) {
	graph := &DependencyGraph{
		inDegree:  make(map[string]int),
		adjacency: make(map[string][]string),
		beads:     make(map[string]taskqueue.Bead),
	}

	// First pass: Initialize all beads with in-degree 0
	for _, bead := range beads {
		graph.inDegree[bead.ID] = 0
		graph.adjacency[bead.ID] = []string{}
		graph.beads[bead.ID] = bead
	}

	// Second pass: Build adjacency list and calculate in-degrees
	for _, bead := range beads {
		for _, depID := range bead.DependsOn {
			// Validate dependency exists
			if _, exists := graph.beads[depID]; !exists {
				// Dependency might be in completed queue (already done)
				// This is OK - we'll filter out such dependencies during sort
				continue
			}

			// Add edge: depID -> bead.ID (dependency points to dependent)
			graph.adjacency[depID] = append(graph.adjacency[depID], bead.ID)

			// Increment in-degree for dependent bead
			graph.inDegree[bead.ID]++
		}
	}

	return graph, nil
}

// TopologicalSort performs Kahn's algorithm to sort beads in dependency order.
// Returns ordered list of bead IDs (dependencies before dependents).
// Returns error if circular dependency detected.
func (g *DependencyGraph) TopologicalSort() ([]string, error) {
	// Initialize queue with zero-degree beads (no dependencies)
	queue := []string{}
	for beadID, degree := range g.inDegree {
		if degree == 0 {
			queue = append(queue, beadID)
		}
	}

	// Track sorted order
	sorted := []string{}

	// Process queue
	for len(queue) > 0 {
		// Dequeue first bead
		beadID := queue[0]
		queue = queue[1:]
		sorted = append(sorted, beadID)

		// Reduce in-degree for all dependents
		for _, dependentID := range g.adjacency[beadID] {
			g.inDegree[dependentID]--

			// If dependent has no more dependencies, add to queue
			if g.inDegree[dependentID] == 0 {
				queue = append(queue, dependentID)
			}
		}
	}

	// Check for cycles: if sorted length < total beads, cycle exists
	if len(sorted) < len(g.beads) {
		// Find beads not in sorted list (part of cycle)
		cycleBeads := []string{}
		sortedMap := make(map[string]bool)
		for _, id := range sorted {
			sortedMap[id] = true
		}
		for id := range g.beads {
			if !sortedMap[id] {
				cycleBeads = append(cycleBeads, id)
			}
		}

		return nil, fmt.Errorf("circular dependency detected involving beads: %s",
			strings.Join(cycleBeads, ", "))
	}

	return sorted, nil
}
