# ADR-007: Hooks as high-confidence state signals

Status: Accepted (2026-02-02; amended 2026-07-17)

## Context

Terminal text is ambiguous: the same prompt-looking output can be history,
current input, or a harness-specific status line. Harness hooks can report
lifecycle events at their actual boundary, but not every harness exposes the
same hook surface.

## Decision

Use harness hooks as high-confidence state signals where supported. Persist
their events through AGM state/storage modules and combine them with bounded
runtime health checks. Harnesses without equivalent hooks use explicit adapter
fallbacks; Claude-specific hooks are not the universal contract.

## Alternatives

Pane polling alone is portable but heuristic. Hook-only detection would make
unsupported harnesses invisible and cannot detect a dead process that emits no
final event.

## Consequences

State confidence varies by harness and must be represented honestly. Hook,
monitor, and adapter tests own the evidence.
