# Architecture Decision Records - Index

This directory contains Architecture Decision Records (ADRs) for the Engram CLI project. ADRs document significant architectural decisions, their context, rationale, and consequences.

## What is an ADR?

An Architecture Decision Record (ADR) captures an important architectural decision made along with its context and consequences. ADRs help teams understand:
- Why decisions were made
- What alternatives were considered
- What the trade-offs are
- How to implement the decision

## ADR Format

Each ADR follows this structure:
- **Status**: Accepted | Deprecated | Superseded
- **Date**: Decision date
- **Context**: Problem and requirements
- **Decision**: What was decided
- **Rationale**: Why this decision
- **Alternatives Considered**: Other options evaluated
- **Consequences**: Positive and negative impacts
- **Implementation Notes**: How to apply this decision
- **Related Decisions**: Links to related ADRs

## Active ADRs

### Core Architecture

- **[ADR-001: Cobra CLI Framework](./ADR-001-cobra-cli-framework.md)**
  - Use Cobra for command-line interface
  - Provides command hierarchy, flags, help, completion
  - Status: Accepted
  - Date: 2024-01-15

- **[ADR-005: Hierarchical Workspace Structure](./ADR-005-hierarchical-workspace-structure.md)**
  - Four-tier workspace: user, team, company, core
  - Separation of concerns with override capability
  - Status: Accepted
  - Date: 2024-02-05

### User Experience

- **[ADR-002: Structured Error Handling](./ADR-002-structured-error-handling.md)**
  - Custom error types with actionable suggestions
  - User-friendly error messages with visual feedback
  - Status: Accepted
  - Date: 2024-01-20

- **[ADR-003: Output Formatting Standards](./ADR-003-output-formatting-standards.md)**
  - Standardized output with icons, colors, JSON mode
  - Accessibility with --no-color, quiet mode
  - Status: Accepted
  - Date: 2024-01-22

### Retrieval & Search

- **[ADR-004: Three-Tier Retrieval System](./ADR-004-three-tier-retrieval-system.md)**
  - Fast filter, API ranking, token budget
  - Ecphory retrieval system architecture
  - Status: Accepted
  - Date: 2024-02-01

### Security

- **[ADR-006: Security-First Input Validation](./ADR-006-security-first-input-validation.md)**
  - Whitelist-based path validation
  - Defense against path traversal, injection attacks
  - Status: Accepted
  - Date: 2024-02-10

### Extensibility

- **[ADR-007: Pluggable Memory Providers](./ADR-007-pluggable-memory-providers.md)**
  - Provider interface for storage backends
  - Simple, SQLite, Postgres, Redis providers
  - Status: Accepted
  - Date: 2024-02-15

- **[ADR-008: Plugin Architecture Patterns](./ADR-008-plugin-architecture-patterns.md)**
  - Three plugin patterns: Guidance, Tool, Connector
  - Permission model and EventBus
  - Status: Accepted
  - Date: 2024-02-20

## ADR Lifecycle

### Status Definitions

- **Proposed**: Under discussion, not yet implemented
- **Accepted**: Decision made, implementation in progress or complete
- **Deprecated**: No longer recommended, but not yet replaced
- **Superseded**: Replaced by another ADR (link to successor)

### When to Create an ADR

Create an ADR when:
- Making a significant architectural decision
- Choosing between multiple approaches
- Establishing a pattern or standard
- Making a decision with long-term impact
- Reversing or updating a previous decision

### When NOT to Create an ADR

Don't create an ADR for:
- Implementation details
- Minor bug fixes
- Temporary solutions
- Obvious decisions with no alternatives

## Reading Guide

### For New Contributors

Start here to understand the system:
1. [ADR-001: Cobra CLI Framework](./ADR-001-cobra-cli-framework.md) - How commands work
2. [ADR-005: Hierarchical Workspace](./ADR-005-hierarchical-workspace-structure.md) - Directory structure
3. [ADR-004: Retrieval System](./ADR-004-three-tier-retrieval-system.md) - How search works

### For Security Reviewers

Security-focused decisions:
1. [ADR-006: Input Validation](./ADR-006-security-first-input-validation.md) - Security model
2. [ADR-008: Plugin Architecture](./ADR-008-plugin-architecture-patterns.md) - Plugin security
3. [ADR-007: Memory Providers](./ADR-007-pluggable-memory-providers.md) - Data security

### For UX Designers

User experience decisions:
1. [ADR-002: Error Handling](./ADR-002-structured-error-handling.md) - Error messages
2. [ADR-003: Output Formatting](./ADR-003-output-formatting-standards.md) - Visual design
3. [ADR-004: Retrieval System](./ADR-004-three-tier-retrieval-system.md) - Search UX

### For System Architects

Architecture decisions:
1. [ADR-005: Workspace Structure](./ADR-005-hierarchical-workspace-structure.md) - Tiers
2. [ADR-007: Memory Providers](./ADR-007-pluggable-memory-providers.md) - Storage
3. [ADR-008: Plugins](./ADR-008-plugin-architecture-patterns.md) - Extensibility

## Decision Dependencies

```
ADR-001 (Cobra)
    ├── ADR-002 (Errors)
    ├── ADR-003 (Output)
    └── ADR-006 (Validation)

ADR-004 (Retrieval)
    ├── ADR-005 (Workspace)
    └── ADR-006 (Validation)

ADR-005 (Workspace)
    └── ADR-008 (Plugins)

ADR-007 (Memory)
    ├── ADR-006 (Validation)
    └── ADR-005 (Workspace)

ADR-008 (Plugins)
    ├── ADR-005 (Workspace)
    └── ADR-006 (Validation)
```

## Proposing an ADR

1. Copy ADR template (create one based on existing ADRs)
2. Fill in sections (Context, Decision, Rationale, etc.)
3. Set status to "Proposed"
4. Open PR for discussion
5. Address feedback
6. Update status to "Accepted" when merged

## Updating an ADR

ADRs are immutable once accepted. To update:
1. Create a new ADR superseding the old one
2. Update old ADR status to "Superseded by ADR-XXX"
3. Link new ADR to old one in "Related Decisions"

## References

- [Architecture Decision Records (ADR)](https://adr.github.io/) - ADR methodology
- [Documenting Architecture Decisions](https://cognitect.com/blog/2011/11/15/documenting-architecture-decisions) - Original article by Michael Nygard
- [ADR Tools](https://github.com/npryce/adr-tools) - Command-line tools for ADRs

## See Also

- [SPEC.md](./SPEC.md) - Technical specification
- [ARCHITECTURE.md](./ARCHITECTURE.md) - System architecture overview
- [README.md](../../README.md) - Project README
