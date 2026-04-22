# Architecture Decision Records (ADRs)

This directory contains Architecture Decision Records (ADRs) for the retrieval package,
documenting key architectural decisions and their rationale.

## What is an ADR?

An Architecture Decision Record captures an important architectural decision made during
the development of this package, including the context, decision, and consequences.

## ADR Index

### Core Architecture

- **[ADR-001: Per-Search Index Building](./ADR-001-per-search-index-building.md)**
  - **Status**: Accepted
  - **Decision**: Build ecphory.Index per Search call, not at Service initialization
  - **Rationale**: Supports dynamic EngramPath, avoids cache invalidation complexity
  - **Trade-offs**: Higher per-search cost vs flexibility and simplicity

- **[ADR-002: Tracking Integration in Service Layer](./ADR-002-tracking-integration-in-service-layer.md)**
  - **Status**: Accepted
  - **Decision**: Service owns tracking.Tracker, records access after parse
  - **Rationale**: Natural service boundary, automatic tracking, simple API
  - **Trade-offs**: Service handles non-search concern vs consumer manual tracking

- **[ADR-003: API Ranking Fallback Strategy](./ADR-003-api-ranking-fallback-strategy.md)**
  - **Status**: Accepted
  - **Decision**: Silent fallback on missing API key, error on ranking failures
  - **Rationale**: Graceful degradation for expected config, strict on real errors
  - **Trade-offs**: Silent fallback vs fail-hard on all API issues

## ADR Lifecycle

**Statuses**:
- **Proposed**: Under discussion
- **Accepted**: Approved and implemented
- **Deprecated**: No longer recommended (replaced by newer ADR)
- **Superseded**: Replaced by specific ADR (reference provided)

## Template

New ADRs should follow this structure:

```markdown
# ADR-XXX: Title

**Status**: Proposed | Accepted | Deprecated | Superseded
**Date**: YYYY-MM-DD
**Deciders**: Team/Individual
**Context**: When decision was needed

---

## Context
What is the issue that we're seeing that is motivating this decision?

## Decision
What is the change that we're proposing and/or doing?

## Consequences
What becomes easier or harder to do because of this change?

### Positive
Benefits of this decision

### Negative
Drawbacks and trade-offs

## Alternatives Considered
What other options did we evaluate?

## References
Links to related documents, code, or discussions
```

## Related Documentation

- **[SPEC.md](../../SPEC.md)**: Package specification (vision, goals, API reference)
- **[ARCHITECTURE.md](../../ARCHITECTURE.md)**: Detailed architecture (components, data flow)
- **[retrieval.go](../../retrieval.go)**: Implementation code

## Questions?

For questions about these ADRs or to propose new ones, see the main package documentation
or consult the engram core team.
