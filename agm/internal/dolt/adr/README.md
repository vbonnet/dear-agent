# Architecture Decision Records (ADRs)

This directory contains Architecture Decision Records for the AGM Dolt Storage implementation.

## What is an ADR?

An Architecture Decision Record (ADR) captures an important architectural decision made along with its context and consequences.

## ADR Format

Each ADR follows this structure:
- **Status**: Proposed | Accepted | Deprecated | Superseded
- **Date**: When the decision was made
- **Context**: What is the issue we're addressing?
- **Decision**: What is the change we're proposing?
- **Rationale**: Why this decision over alternatives?
- **Consequences**: What becomes easier or harder as a result?

## Index

| ADR | Title | Status | Date |
|-----|-------|--------|------|
| [001](./001-dolt-over-sqlite.md) | Choose Dolt Over SQLite for AGM Storage | Accepted | 2026-03-07 |
| [002](./002-workspace-isolation-strategy.md) | Workspace Isolation via Separate Databases | Accepted | 2026-03-07 |
| [003](./003-embedded-migration-system.md) | Embedded Migration System with Checksum Validation | Accepted | 2026-03-07 |

## Decision Status

- **Proposed**: Under discussion, not yet implemented
- **Accepted**: Decision made and implementation in progress or complete
- **Deprecated**: No longer relevant or recommended
- **Superseded**: Replaced by a newer decision (link to new ADR)

## Key Decisions

### Storage Layer
- **ADR-001**: Chose Dolt over SQLite for Git-like version control and corruption prevention
- **ADR-002**: Separate databases per workspace for security and isolation
- **ADR-003**: Embedded migrations with SHA256 checksum validation

### Future ADRs

Potential future decisions to document:
- **ADR-004**: Message embedding strategy for semantic search
- **ADR-005**: Tool usage analytics schema design
- **ADR-006**: Cross-workspace query mechanisms (Corpus Callosum)
- **ADR-007**: Encryption at rest strategy (if needed for Acme Corp compliance)
- **ADR-008**: Backup and disaster recovery procedures

## References

- **Specification**: [../SPEC.md](../SPEC.md)
- **Architecture**: [../ARCHITECTURE.md](../ARCHITECTURE.md)
- **README**: [../README.md](../README.md)
- **ADR Template**: https://github.com/joelparkerhenderson/architecture-decision-record
