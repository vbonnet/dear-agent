# Ecphory ranking decisions

Status: Accepted

## Context

Ranking providers have different credentials and availability. Ecphory also
needs a deterministic path for local development, tests, and degraded
operation.

## Decisions

1. **Narrow provider boundary.** Ranking depends on the package provider
   interface; construction and credential detection stay in the factory.
2. **Factory precedence.** `AutoDetect` follows the hard-coded precedence in
   `factory.go` and `detection.go`; the stored `Config.Ecphory.Ranking.Provider`
   and `Fallback` fields do not currently alter that selection. Callers that
   require an explicit provider use `GetProvider`.
3. **Local baseline.** The local provider uses token-set similarity as a
   deterministic, dependency-free baseline.
4. **No credential extraction.** Provider adapters consume supported
   credentials and do not extract OAuth material from another harness.

## Consequences

- Tests can use the local implementation without network access.
- Provider addition does not change Ecphory's retrieval boundary.
- Auto-detection order and explicit `GetProvider` selection are behavior and
  therefore covered by tests.

## Evidence

- `provider.go`, `factory.go`, `detection.go`, and tests
- `local.go`
