# ADR-024: A2A supervisor protocol

Status: Accepted (2026-05-26; verified 2026-07-17)

## Context

Tmux keystrokes and pane scraping are useful harness controls but are not a
typed agent-to-supervisor protocol. Questions, artifacts, task state, and
cross-process handoff need one interoperable contract with a harness-neutral
fallback.

## Decision

Use A2A-compatible messages, task states, artifacts, and agent cards at the
agent/supervisor seam. `pkg/a2a` owns the root protocol abstractions;
`agm/internal/a2a` owns AGM adapters and persistence. AGM may still use tmux to
control a local harness, but protocol state is not inferred solely from terminal
text. Human input is represented as an input-required task state rather than a
blocking harness-specific question call.

## Alternatives

A proprietary JSON-RPC schema would duplicate a public interoperability model.
Tmux-only control cannot represent structured artifacts. Requiring Claude-only
channels would exclude Codex, AGY, and OpenCode harnesses.

## Consequences

The system carries protocol and adapter complexity, and not every harness
supports every extension. Core task semantics remain harness-neutral; richer
harness integrations are optional. Tests under `pkg/a2a` and
`agm/internal/a2a` verify the contract.
