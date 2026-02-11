# Architecture Decision Records (ADRs)

This directory contains Architecture Decision Records (ADRs) for Autonomous Swarm. ADRs document significant architectural decisions, their context, alternatives considered, and consequences.

## What is an ADR?

An Architecture Decision Record captures an important architectural decision made along with its context and consequences. ADRs help future maintainers understand why the system is built the way it is.

## ADR Format

Each ADR follows this structure:
- **Status**: Accepted, Superseded, Deprecated
- **Context**: The problem or situation requiring a decision
- **Decision**: The architectural choice made
- **Consequences**: Positive and negative outcomes of the decision
- **Alternatives**: Other approaches considered and why they were rejected

## Index of ADRs

### Core Architecture

| ADR | Title | Status | Date |
|-----|-------|--------|------|
| [ADR-001](ADR-001-dependency-graph.md) | Dependency Graph with Topological Sort | Accepted | 2026-02-11 |
| [ADR-002](ADR-002-atomic-writes.md) | Atomic Queue Writes with Temp-Rename Pattern | Accepted | 2026-02-11 |
| [ADR-003](ADR-003-escalation-model.md) | Three-Tier Error Classification and Escalation Model | Accepted | 2026-02-11 |

### Quick Reference

#### ADR-001: Dependency Graph with Topological Sort
**Problem**: How to execute beads in dependency order and prevent circular dependencies?
**Solution**: Use Kahn's algorithm for topological sorting with O(V+E) complexity
**Key Benefit**: Enables parallel execution while guaranteeing dependency correctness

#### ADR-002: Atomic Queue Writes with Temp-Rename Pattern
**Problem**: How to prevent queue corruption on process crash or disk full?
**Solution**: Write to temp file, then atomically rename to target
**Key Benefit**: Queue state always consistent, even on crash (crash-only design)

#### ADR-003: Three-Tier Error Classification and Escalation Model
**Problem**: How to distinguish retry-able errors from those requiring human intervention?
**Solution**: Classify errors as Recoverable, Escalation, or Fatal
**Key Benefit**: Autonomous retry on transient failures, escalate when stuck

## Decision-Making Process

### When to Create an ADR

Create an ADR when making decisions about:
- Core algorithms and data structures
- External integrations and dependencies
- Error handling strategies
- Performance trade-offs
- Security or reliability mechanisms
- File formats or protocols

### ADR Lifecycle

1. **Proposed**: Draft ADR written, seeking feedback
2. **Accepted**: Decision approved and implemented
3. **Superseded**: Replaced by newer ADR (link to successor)
4. **Deprecated**: Decision reversed (explain why)

### Template

Use this template for new ADRs:

```markdown
# ADR-XXX: [Title]

## Status
[Proposed | Accepted | Superseded | Deprecated]

## Context
[Describe the problem or situation requiring a decision]

## Decision
[Describe the architectural choice made]

## Consequences
### Positive
[List benefits]

### Negative
[List drawbacks]

### Trade-offs
[Describe what you're optimizing for]

## Rejected Alternatives
### Alternative 1: [Name]
**Approach**: [Description]
**Rejected Because**: [Reasons]

## References
[Links to code, docs, external resources]

## Revision History
| Version | Date | Changes | Author |
|---------|------|---------|--------|
| 1.0.0 | YYYY-MM-DD | Initial decision | [Name] |
```

## Contributing

When proposing a new ADR:
1. Copy the template above
2. Number it sequentially (ADR-004, ADR-005, ...)
3. Submit as pull request for review
4. Update this README index after acceptance

## References

- [SPEC.md](../../SPEC.md) - System specification
- [ARCHITECTURE.md](../../ARCHITECTURE.md) - Detailed architecture
- [README.md](../../README.md) - User documentation

## External Resources

- [ADR GitHub Organization](https://adr.github.io/) - ADR best practices
- [Documenting Architecture Decisions](https://cognitect.com/blog/2011/11/15/documenting-architecture-decisions) - Michael Nygard's original article
