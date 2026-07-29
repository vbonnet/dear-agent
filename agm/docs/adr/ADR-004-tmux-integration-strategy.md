# ADR-004: Tmux as the local CLI session runtime

Status: Accepted (2026-01-10; amended 2026-07-17)

## Context

Interactive coding harnesses need a terminal that survives AGM process exit and
can be inspected, attached, and controlled by other local processes. Raw PTYs
would require AGM to rebuild a terminal multiplexer and persistence layer.

## Decision

Tmux is the required local runtime for CLI harness sessions. AGM uses exact
session identifiers and explicit tmux commands for create, send, capture,
attach, and stop. Harness adapters interpret captured output and capabilities;
tmux itself is not treated as the source of durable lifecycle truth.

## Alternatives

Owning PTYs and a daemon duplicates mature multiplexing behavior. Detached
subprocesses lack a reliable interactive attach surface. Loose shell scripts
cannot enforce exact session targeting.

## Consequences

Local operation depends on tmux and inherits its platform constraints. Durable
session records and hooks remain separate from pane state. `session.RealTmux`
is the one production adapter; tests under `agm/internal/tmux`,
`agm/internal/session`, shared operations, and harness adapters verify the
boundary.
