# Launcher Package

The launcher package provides dependency-aware bead launching for the autonomous swarm executor.

## Overview

This package ensures beads are launched in the correct order based on their dependencies, preventing agents from blocking on missing dependencies.

## Key Components

### DependencyGraph (`graph.go`)

Represents a directed acyclic graph (DAG) of bead dependencies:
- `BuildGraph(beads []Bead)` - Constructs dependency graph from beads
- `TopologicalSort()` - Returns beads in dependency order using Kahn's algorithm
- Detects circular dependencies and returns descriptive errors

### Orchestrator (`orchestrator.go`)

Manages dependency-aware bead launching:
- `LaunchReady()` - Launches all ready beads in topological order
- Validates dependencies via coordinator.Claim() before launching
- Handles partial failures gracefully (continues launching remaining beads)

## Algorithm

Uses **Kahn's Topological Sort** algorithm:
1. Build in-degree map (count dependencies per bead)
2. Initialize queue with zero-dependency beads
3. Process queue: remove bead, reduce dependents' in-degree
4. Cycle detected if sorted length < total beads

**Time Complexity**: O(V + E) where V = beads, E = dependencies

## Usage

```go
import "github.com/[REDACTED_EMPLOYER]-src/ai-tools/autonomous-swarm/pkg/launcher"

// Create coordinator
coord := taskqueue.NewCoordinator(queuePath)
coord.Load()

// Create orchestrator
orch := launcher.NewOrchestrator(coord)

// Launch all ready beads in dependency order
if err := orch.LaunchReady(); err != nil {
    log.Fatalf("Launch failed: %v", err)
}
```

## Error Handling

- **Circular Dependency**: Returns error with bead IDs involved in cycle
- **Claim Failure**: Logs warning, skips bead, continues with others
- **All Beads Failed**: Returns error if no beads launched successfully

## Testing

Run tests:
```bash
go test ./pkg/launcher/
```

Test coverage:
- Graph construction (empty, single, linear, parallel, diamond)
- Topological sort (all patterns including cycles)
- Orchestrator integration (empty queue, linear deps, cycles)

## Future Enhancements

- Parallel launch for independent beads (goroutines)
- Retry logic for failed launches
- Dynamic priority adjustment based on dependency depth
