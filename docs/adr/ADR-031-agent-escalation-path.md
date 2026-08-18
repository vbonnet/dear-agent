# ADR-031: Audited exceptions and escalation

Status: Accepted (2026-06-15; amended 2026-07-17)

<!-- Last audited at: 2026-07-21 -->

## Context

A silent or generic bypass turns a safety gate into a suggestion and hides
whether the policy or tooling is wrong. Some operational wrappers still need a
narrow exceptional path when a known repository baseline prevents an otherwise
verified change from reaching review.

## Decision

- No wrapper exposes an unlogged `--emergency` escape hatch.
- An exceptional path, where one exists, is explicit, narrowly scoped, and
  recorded by the wrapper audit trail. Reason-bearing override APIs use the
  shared override guard.
- If no approved exception matches the condition, the agent uses `agm
  escalate`; it does not substitute a raw command that bypasses the wrapper.
- Repeated exception use is a defect signal and must become a tooling, policy,
  or test-isolation fix.

The implemented escalation entry point is:

```text
agm escalate ask --kind blocked-action --context "<why the normal path is unavailable>" "<what is needed>"
```

Inside an AGM-launched session the command attributes and routes the request
through the registered session and supervisor chain. Outside AGM, callers must
pass `--session <registered-session>`. If no registered session exists, the
agent asks the current user directly. Escalations are durable decision records;
they do not create or update Beads.

`safe-pr`, `safe-merge`, `internal/override`, and the escalation engine are the
source owners. ADR-032 defines supervisor routing.

## Alternatives

One universal force flag is easy to use and impossible to govern. A permanent
hard stop cannot distinguish a product decision from a broken local preflight.

## Consequences

Exceptional delivery is visible and reviewable, but may require a human or a
follow-up repair. Wrapper and override tests verify the allowed paths.
