# ADR-003: Fall back locally when API credentials are absent

Status: Accepted

## Context

Remote ranking improves some semantic queries but requires an Anthropic API
key. Once API ranking is configured, construction and ranking failures are
operational errors rather than evidence that a local result is equivalent.

## Decision

The service attempts configured API ranking only after it has local candidates.
When API ranking is requested but `ANTHROPIC_API_KEY` is absent, it returns the
local ranking. When the key is present, provider construction or ranking
failures remain errors; the service does not silently downgrade them.

## Consequences

- Retrieval remains available when API ranking has not been configured.
- Provider outages and quota failures remain visible to configured callers.
- Callers must not treat local fallback as a recovery path for provider errors.

## Evidence

- retrieval service API-key fallback tests
- `../../retrieval.go`
