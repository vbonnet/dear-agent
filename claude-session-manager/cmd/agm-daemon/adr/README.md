# Architecture Decision Records (ADRs)

This directory contains Architecture Decision Records for the AGM Daemon.

## What are ADRs?

Architecture Decision Records (ADRs) document significant architectural decisions made during the development of this project. Each ADR captures:
- The context and problem being solved
- The decision made
- The rationale behind the decision
- The consequences of the decision

## ADR Index

| ADR | Title | Status | Date |
|-----|-------|--------|------|
| [001](./001-http-api-choice.md) | HTTP API for State Exposure | Accepted | 2026-02-11 |
| [002](./002-polling-strategy.md) | Polling-Based State Detection | Accepted | 2026-02-11 |
| [003](./003-dual-interface-design.md) | Dual Interface (HTTP + File) | Accepted | 2026-02-11 |
| [004](./004-state-detection-patterns.md) | Visual Pattern-Based State Detection | Accepted | 2026-02-11 |

## ADR Summary

### [001: HTTP API for State Exposure](./001-http-api-choice.md)

**Decision**: Expose session states via HTTP API on port 8765 instead of CLI commands or direct file reading.

**Key Rationale**:
- RESTful interface for programmatic access
- Language-agnostic integration
- Real-time query capability
- Standard JSON response format

### [002: Polling-Based State Detection](./002-polling-strategy.md)

**Decision**: Use timer-based polling (2s interval) instead of event-driven monitoring.

**Key Rationale**:
- Simple implementation (no tmux hooks)
- Reliable state detection
- Configurable overhead vs freshness tradeoff
- Predictable resource usage

### [003: Dual Interface Design](./003-dual-interface-design.md)

**Decision**: Provide both HTTP API and file-based status updates instead of single interface.

**Key Rationale**:
- HTTP for programmatic access
- Files for shell scripts and tmux integration
- Redundancy for reliability
- Different use cases require different interfaces

### [004: Visual Pattern-Based State Detection](./004-state-detection-patterns.md)

**Decision**: Use regex patterns on terminal output instead of tmux state variables or process inspection.

**Key Rationale**:
- Accurate state detection (what user sees)
- Works with any tmux session
- No tmux modifications required
- Handles all Claude Code states

## Decision Process

### When to Create an ADR

Create an ADR for:
- Architectural choices with long-term impact
- Trade-offs between multiple viable options
- Decisions that affect external interfaces (API, file format)
- Choices that future developers may question

### ADR Template

Each ADR follows this structure:
1. **Status**: Proposed, Accepted, Deprecated, Superseded
2. **Context**: Problem being solved, requirements, constraints
3. **Decision**: What was decided
4. **Rationale**: Why this decision was made
5. **Consequences**: Positive, negative, and neutral impacts
6. **References**: Links to relevant documentation

### ADR Lifecycle

- **Proposed**: Under discussion
- **Accepted**: Decision implemented
- **Deprecated**: No longer recommended
- **Superseded**: Replaced by newer ADR

## Contributing

When making significant architectural changes:

1. **Propose**: Create draft ADR with "Proposed" status
2. **Discuss**: Review with team/maintainers
3. **Decide**: Update status to "Accepted" or reject
4. **Implement**: Code the decision
5. **Document**: Update README if needed

## References

- ADR best practices: https://github.com/joelparkerhenderson/architecture-decision-record
- Lightweight ADRs: https://www.thoughtworks.com/radar/techniques/lightweight-architecture-decision-records
- When to use ADRs: https://cognitect.com/blog/2011/11/15/documenting-architecture-decisions
