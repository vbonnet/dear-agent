# ADR-030: Shared resume operation

Status: Accepted (2026-07-27)

## Context

Session resume was implemented as a broad CLI transaction that combined human
identifier resolution, tmux ownership, harness-specific launch and readiness,
metadata compare-and-swap, prompt uncertainty, activity updates, terminal
presentation, and interactive attachment. Other production entry points called
back into that command-layer owner, and lifecycle tests depended on a large
callback facade rather than one public operation contract.

## Decision

`internal/ops.ResumeSession` owns the complete non-interactive resume
transaction. It accepts one stable session ID, acquires the shared lifecycle
lock before reading mutable state, and returns typed health, ownership, launch,
prompt, and warning facts. Harness-neutral ordering stays in the operation;
native tmux behavior is exposed through narrow optional capabilities on the
session adapter. The operation owns exact-identity rollback and metadata
compensation.

CLI code may resolve human identifiers, validate and read prompt files, render
read-only events, and attach to the returned tmux target after the operation
releases its lock. Observer callbacks cannot authorize or alter lifecycle
steps.

## Alternatives

Keeping the transaction in `cmd/agm` preserves direct access to terminal UI but
makes the command package the reusable service boundary. Moving the transaction
into `internal/session` would couple durable storage mutation to tmux mechanics.
A callback-rich runtime seam makes individual phases easy to stub but exposes
the transaction's implementation as its test interface.

## Consequences

All production resume entry points share one transaction, stable lock boundary,
and typed result. Operation tests exercise lifecycle state transitions through
the public contract; tmux adapter tests exercise native readiness policy; CLI
tests are limited to input, presentation, and post-operation attachment.
Adding a harness requires explicit resume capabilities and policy in the shared
owner instead of another command-layer switch.
