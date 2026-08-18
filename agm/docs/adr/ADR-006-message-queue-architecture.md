# ADR-006: Durable local message queue

Status: Accepted (2026-02-01; verified 2026-07-17)

## Context

Inter-session delivery must survive sender exit, recipient busy states, and
daemon restarts without requiring an external service.

## Decision

`agm/internal/messages.MessageQueue` stores ordered messages, attempts,
acknowledgements, and dead-letter state in SQLite with WAL and a bounded busy
timeout. The daemon selects queued entries, checks recipient readiness, delivers
through the harness boundary, and records retry or terminal outcomes.

Pending hook files from ADR-017 are a latency optimization for supported
harnesses, not a second authoritative queue.

## Alternatives

In-memory queues lose work on exit. Redis adds a service to a local-first tool.
Files alone make concurrent query, retry, and acknowledgement state harder to
coordinate.

## Consequences

SQLite is a single-host boundary and needs retention. Queue, acknowledgement,
and daemon tests verify ordering, retry, and failure behavior.
