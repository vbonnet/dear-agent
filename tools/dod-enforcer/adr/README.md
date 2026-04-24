# Architecture Decision Records (ADRs)

This directory contains Architecture Decision Records for the Bead DoD package.

## ADR Index

| ADR | Title | Status |
|-----|-------|--------|
| [ADR-001](ADR-001-yaml-specification.md) | Use YAML for DoD Specification | Accepted |
| [ADR-002](ADR-002-validation-never-errors.md) | Validation Returns Results, Not Errors | Accepted |
| [ADR-003](ADR-003-sequential-execution.md) | Sequential Check Execution | Accepted |
| [ADR-004](ADR-004-shell-based-commands.md) | Shell-Based Command Execution | Accepted |
| [ADR-005](ADR-005-hardcoded-timeouts.md) | Hardcoded Timeout Values | Accepted |

## How to Use ADRs

Each ADR documents a significant architectural decision made during the development of the Bead DoD package. They provide:

- **Context**: Why the decision was needed
- **Decision**: What was decided
- **Consequences**: Implications of the decision
- **Alternatives**: Options that were considered but not chosen

ADRs are immutable once accepted. If a decision needs to be changed, create a new ADR that supersedes the old one.
