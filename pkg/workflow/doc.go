// Package workflow implements an Archon-style YAML DAG engine for declarative
// orchestration. Workflows are directed acyclic graphs of typed Nodes (ai,
// bash, gate, loop) that the Runner executes in topological order while
// honoring dependencies, loop-until conditions, and gate signals.
//
// The engine provides three optional extension points:
//   - Node-level retry policies (RetryPolicy on Node)
//   - Parallel-per-iteration loop mode (Parallel: true on LoopNode)
//   - Crash-recovery persistence (State interface + FileState implementation)
package workflow
