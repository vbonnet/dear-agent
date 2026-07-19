# ADR-001: Use bounded capture-pane polling for prompt detection

Status: Accepted

## Context

AGM observes tmux sessions it does not exclusively own. Tmux control mode
provides a stream but changes attachment semantics and complicates recovery when
AGM or the observed process restarts.

## Decision

Prompt detection reads a bounded tail with `tmux capture-pane -p` and applies
the shared detector. Callers poll at their owned cadence and treat missing or
ended sessions as explicit state, not empty output. Session-level tmux commands
use exact targets.

## Consequences

- Observation is stateless and recoverable across AGM restarts.
- Detection latency is bounded by the caller poll interval.
- Output parsing remains heuristic and must fail closed for permission actions.

## Evidence

- `monitor.go`, `output.go`, and tmux package tests
- `../monitor/` prompt detectors
