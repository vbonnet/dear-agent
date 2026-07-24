# AGM Circuit Breaker Specification

## BDD Traceability

- Feature: `agm/test/bdd/features/legacy_spec_bdd_linkage_guardrails.feature`

<!-- Last audited at: 2026-07-03 -->

## Purpose

`agm/internal/circuitbreaker` provides resource-health gates used before AGM
starts or continues work. Platform-specific readers must report actionable
machine state without false positives that would block healthy agent sessions.

## EARS Requirements

**CBRK-01** When AGM checks memory availability on macOS, the system shall use the platform `memory_pressure -Q` signal instead of raw inactive-page arithmetic.

**CBRK-02** When the macOS memory probe is invoked, the system shall bound the subprocess with a timeout so a stuck probe cannot hang session admission.

**CBRK-03** When `memory_pressure -Q` reports a system-wide free-memory percentage, the system shall parse and return that percentage as a numeric value.

**CBRK-04** When `memory_pressure -Q` fails, times out, or omits the free-percentage line, the system shall return an explicit error instead of inventing a memory value.

**CBRK-05** When the default memory reader is requested on macOS, the system shall return the native memory-pressure reader for circuit-breaker decisions.

**CBRK-06** When the spawn timer contains a future timestamp written by the resource governor, AGM shall refuse the spawn and identify the condition as a governor pause with the earliest possible admission boundary including the required spawn safety interval, while stating that the governor may extend the hold and other gates must also pass.

**CBRK-07** When the spawn timer contains a recent timestamp in the past, AGM shall identify the condition as a recent spawn with the minimum interval and remaining wait.

**CBRK-08** When the worker counter enumerates sessions on the shared AGM tmux socket, the system shall count only sessions recognisable as workers — those whose name carries the `worker-` prefix, or that the AGM session DB records as active and tagged `role:worker` — so test fixtures, supervisor panes, and orphan sessions cannot consume worker slots.

**CBRK-09** When the AGM session DB cannot be read while classifying an unprefixed tmux session, the system shall fall back to prefix-only classification rather than counting the session as a worker, so an unreadable database cannot deadlock dispatch.

**CBRK-10** When the known-worker lookup is invoked, the system shall bound it with a timeout so a locked or overloaded session database cannot hang session admission.

**CBRK-11** When a worker's record name differs from its tmux session name, the system shall recognise the session under either name, so a resumed or imported worker still counts against the cap.
