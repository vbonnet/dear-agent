# ADR-028: Dependency-aware integration test selection

Status: Accepted (2026-05-26; verified 2026-07-17)

## Context

Tagged integration suites are too expensive to run indiscriminately, while
path-only CI filters miss impacts through shared Go dependencies. Leaving them
out of CI makes their signal optional.

## Decision

`cmd/test-affected` combines the git change set with the Go import graph to
select test-bearing packages whose transitive dependencies changed. CI runs the
selected packages with the requested build tags. Changes to dependency metadata,
the selector, or other global inputs fail safe to the full tagged suite.

Empty selection is a successful no-op, not a reason to run unrelated tests.

## Alternatives

Running every tagged package on every PR wastes capacity and amplifies host
flakes. Workflow `paths` filters are fast but not dependency-aware. Maintaining
a hand-written package map would drift.

## Consequences

Selection depends on accurate Go package metadata and an available base ref.
The command explains forced-full decisions and is covered by unit tests; the CI
workflow owns end-to-end invocation.
