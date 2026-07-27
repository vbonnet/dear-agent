# ADR-032: One earned local runtime owner

Status: Accepted (2026-07-27)

## Context

AGM accumulated two generalized runtime systems around one concrete tmux
process. The legacy path wrapped `session.RealTmux` as a backend and immediately
adapted it back to `session.TmuxInterface`. A second manager backend was
constructed beside it and injected into shared operations, but production
selected the exact tmux path first. The manager path therefore had no active
caller and implemented weaker capture-then-unpinned-send readiness semantics.

The legacy process backend and newer Docker backend had package-local tests but
no production command. Documentation also advertised a Temporal backend that
did not exist.

## Decision

`session.RealTmux` is AGM's one production local runtime adapter. The CLI
constructs it once and injects it directly through `session.TmuxInterface`.
Shared operations discover focused capabilities for checked termination,
strict existence, liveness, startup readiness, input readiness, atomic input
delivery, and exact-pane send.

The legacy backend adapter/registry, parallel manager registry and
implementations, and runtime-selection setting are removed. Pure API sessions
retain their distinct provider transaction and do not require a general local
runtime facade.

A future alternative runtime must arrive with a production caller, define the
smallest consumer-owned seam that caller needs, and prove equivalent safety for
overlapping behavior. Speculative registries and broad lifecycle/message/state
interfaces are not retained as placeholders.

## Alternatives

Keeping only the legacy backend would preserve the tmux-to-backend-to-tmux
round trip and capability forwarding. Standardizing on the manager backend
would replace the proven atomic exact-pane path with weaker semantics and
require a behavior rewrite. Calling raw tmux functions everywhere would remove
the deterministic session-owned test seam.

## Consequences

Production has one runtime construction and one semantic owner. New tmux safety
capabilities are proved directly on `RealTmux` instead of forwarded through
intermediate types. Dormant process, Docker, and nonexistent Temporal choices
are no longer advertised.

AGM remains intentionally dependent on tmux for local interactive harnesses.
Adding a real alternative now requires an explicit architecture change and
caller evidence instead of registry-only code.
