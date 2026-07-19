# ADR-001: Use bounded capture-pane polling for prompt detection

Status: Accepted

## Context

AGM observes tmux sessions it does not exclusively own. Tmux control mode
provides a stream but changes attachment semantics and complicates recovery when
AGM or the observed process restarts.

## Decision

Prompt detection reads a bounded tail with `tmux capture-pane -p` and applies
the shared detector. Callers poll at their owned cadence and treat missing or
ended sessions as explicit state, not empty output.

Session-level tmux commands use the `=` exact-target prefix to prevent a short
name such as `astrocyte` from silently matching `astrocyte-improvements`.
Tmux 3.4 rejects that prefix for pane-level commands, so those commands use a
plain session name only after `HasSession` performs the exact session check.

| Target class | Commands | Target rule |
|---|---|---|
| Session | `has-session`, `kill-session`, `list-sessions`, `list-clients` | `=name` through `FormatSessionTarget` |
| Pane | `send-keys`, `capture-pane` | plain `name`, after exact session validation |

## Consequences

- Observation is stateless and recoverable across AGM restarts.
- Exact session checks prevent prefix collisions without breaking tmux 3.4
  pane targeting.
- Detection latency is bounded by the caller poll interval.
- Output parsing remains heuristic and must fail closed for permission actions.

## Evidence

- `capture.go` and `capture_test.go`
- `prompt_detector.go`, `prompt.go`, and their tests
- `pane_monitor.go` and `output_watcher.go`
