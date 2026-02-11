# ADR-001: Dependency Graph with Topological Sort

## Status

Accepted

## Context

Autonomous Swarm needs to execute beads (autonomous tasks) that may have dependencies on other beads. Executing a bead before its dependencies are complete would result in:

1. Agent failures due to missing prerequisites
2. Wasted computational resources on retries
3. Unpredictable execution order
4. Potential data corruption or inconsistent state

We need a mechanism to:
- Represent bead dependencies as a directed acyclic graph (DAG)
- Detect circular dependencies that would cause deadlock
- Determine a valid execution order that respects dependencies
- Support parallel execution of independent beads

## Decision

We will implement dependency management using a **Directed Acyclic Graph (DAG) with Kahn's algorithm for topological sorting**.

### Key Design Elements

#### 1. Graph Representation

**Data Structure** (`pkg/launcher/graph.go`):
```go
type DependencyGraph struct {
    inDegree  map[string]int      // beadID -> count of dependencies
    adjacency map[string][]string // beadID -> list of dependents
    beads     map[string]taskqueue.Bead // beadID -> bead data
}
```

**Rationale**:
- `inDegree` enables O(1) dependency count tracking
- `adjacency` supports efficient dependent lookup
- Combined structure optimized for Kahn's algorithm

#### 2. Topological Sort Algorithm

**Algorithm**: Kahn's Algorithm
- **Time Complexity**: O(V + E) where V = beads, E = dependencies
- **Space Complexity**: O(V + E) for graph storage

**Process**:
1. Initialize queue with zero-degree beads (no dependencies)
2. Process queue: remove bead, reduce dependent in-degrees
3. Add newly-zero-degree beads to queue
4. If sorted count < total beads, circular dependency exists

**Rationale**:
- Kahn's algorithm provides both sorting AND cycle detection in one pass
- O(V + E) complexity is optimal for DAG sorting
- Simple implementation with clear failure modes

#### 3. Dependency Validation

**At Claim Time** (`pkg/taskqueue/coordinator.go`):
```go
// Validate all dependencies are completed before claiming
if len(bead.DependsOn) > 0 {
    completed := make(map[string]bool)
    for _, b := range c.queue.Completed {
        completed[b.ID] = true
    }

    for _, depID := range bead.DependsOn {
        if !completed[depID] {
            return fmt.Errorf("missing dependencies %v", missingDeps)
        }
    }
}
```

**Rationale**:
- Fail-fast: catch dependency issues before execution starts
- Prevents wasted resources on doomed executions
- Explicit error messages for debugging

#### 4. Automatic Unblocking

**On Completion** (`pkg/taskqueue/coordinator.go`):
```go
func (c *Coordinator) unblockDependents(completedBeadID string) {
    // Check each blocked bead
    for _, bead := range c.queue.Blocked {
        allDepsCompleted := true
        for _, depID := range bead.DependsOn {
            if !completed[depID] {
                allDepsCompleted = false
                break
            }
        }

        if allDepsCompleted {
            c.queue.Ready = append(c.queue.Ready, bead)
        }
    }
}
```

**Rationale**:
- Automatic unblocking reduces manual intervention
- Keeps system running autonomously
- Embedded in completion flow for consistency

### Rejected Alternatives

#### Alternative 1: Simple Sequential Execution

**Approach**: Execute beads in YAML file order without dependency checking

**Rejected Because**:
- No parallelization opportunity
- User must manually order beads (error-prone)
- Fails on out-of-order execution attempts

#### Alternative 2: External DAG Scheduler (Apache Airflow, Temporal)

**Approach**: Integrate existing workflow orchestration system

**Rejected Because**:
- Heavy dependency (Python runtime, database)
- Over-engineered for simple dependency chains
- Breaks "file-based state" design principle
- Adds deployment complexity

#### Alternative 3: Dynamic Dependency Resolution at Runtime

**Approach**: No upfront sorting; resolve dependencies dynamically during execution

**Rejected Because**:
- Complex retry logic when dependencies not ready
- Potential for live-lock scenarios
- No cycle detection until runtime
- More difficult to reason about execution plan

## Consequences

### Positive

1. **Correctness**: Dependencies always respected via claim-time validation
2. **Efficiency**: O(V + E) sorting enables hundreds of beads
3. **Safety**: Cycle detection prevents deadlock scenarios
4. **Parallelization**: Topological sort identifies independent beads for parallel launch
5. **Observability**: Clear execution order in logs

### Negative

1. **Complexity**: Requires DAG algorithm understanding for maintenance
2. **Validation Overhead**: O(d) dependency check per claim (acceptable for v1)
3. **Memory**: O(V + E) graph storage (negligible for < 1000 beads)

### Trade-offs

- **Simplicity vs Safety**: Added graph complexity buys correctness guarantees
- **Upfront Cost vs Runtime Safety**: Graph construction cost pays off with fail-fast validation
- **Flexibility vs Consistency**: Strict DAG enforcement prevents flexible but error-prone patterns

## Implementation Notes

### Graph Construction (`BuildGraph`)

```go
// Two-pass algorithm:
// 1. Initialize all beads with in-degree 0
// 2. Build adjacency and update in-degrees

for _, bead := range beads {
    for _, depID := range bead.DependsOn {
        graph.adjacency[depID] = append(graph.adjacency[depID], bead.ID)
        graph.inDegree[bead.ID]++
    }
}
```

**Note**: Gracefully handles dependencies in completed queue (skips validation)

### Cycle Detection

```go
if len(sorted) < len(g.beads) {
    cycleBeads := findUnsorted(sorted, g.beads)
    return fmt.Errorf("circular dependency: %s", cycleBeads)
}
```

**User Experience**: Clear error message identifies problematic beads

### Future Enhancements

1. **Parallel Execution**: Launch all zero-degree beads simultaneously
2. **Dependency Visualization**: Generate DOT graph for debugging
3. **Soft Dependencies**: Continue on optional dependency failure
4. **Dynamic Dependencies**: Beads declare dependencies at runtime

## References

- **Kahn's Algorithm**: Kahn, A. B. (1962). "Topological sorting of large networks"
- **Implementation**: `pkg/launcher/graph.go`
- **Tests**: `pkg/launcher/graph_test.go`
- **Related**: [ADR-002: Atomic Writes](ADR-002-atomic-writes.md)

## Revision History

| Version | Date | Changes | Author |
|---------|------|---------|--------|
| 1.0.0 | 2026-02-11 | Initial decision record | Backfill Documentation |
