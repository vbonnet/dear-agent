# Monitoring package decisions

Status: Accepted

## Context

Sub-agent progress is not proven by one signal. Event delivery can be missed,
files can predate a task, commits may be absent for valid work, and output text
is heuristic.

## Decisions

1. **Multi-signal result.** Validation combines repository, file, test, event,
   and stub signals under an explicit caller-supplied configuration.
2. **Fallback observation.** When event data is absent, validators may inspect
   the worktree or Git directly. A fallback is reported as evidence, not treated
   as an event that definitely occurred.
3. **Best-effort collection.** File watching, Git hooks, and output parsing
   improve observability but do not become an authority for permission or merge
   decisions.
4. **Structured event log.** Monitoring events use the shared EventBus shape so
   producers and validators do not depend on one another.

## Consequences

- A missing optional signal need not make all monitoring unusable.
- Scores depend on the selected task profile and are not universal quality
  measurements.
- Safety-critical completion still requires the owning verifier.

## Evidence

- `validator.go` and `validator_test.go`
- `agent_monitor.go`, `file_watcher.go`, and `git_hooks.go`
