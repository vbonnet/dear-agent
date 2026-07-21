# ADR-002: Resolve models through role and tool configuration

Status: Accepted

## Context

Different tools and roles have different model needs. Hard-coding every choice
inside a command makes changes require releases and prevents ordered fallback.

## Decision

Configuration may select a model for a tool or role and provides explicit
fallback candidates. The router resolves model identifiers to provider
families, constructs providers lazily, and tries candidates in configured
order. A literal model requested by a workflow bypasses role lookup but still
uses provider resolution and validation.

Default values are compatibility fallbacks, not claims about the globally best
or current model.

## Consequences

- Model policy changes independently of tool logic.
- Invalid or unavailable candidates can fall through to a configured
  alternative.
- The chosen provider family, configured model ID, and role (when applicable)
  are normalized into response metadata by the router.

## Evidence

- `../config/`
- `../router/router.go` and `../router/router_test.go`
