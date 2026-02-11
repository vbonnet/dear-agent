# Architecture Decision Records (ADRs)

This directory contains Architecture Decision Records for the swarm-executor component.

## About ADRs

An Architecture Decision Record (ADR) captures an important architectural decision made
along with its context and consequences. ADRs help document why decisions were made and
provide historical context for future maintainers.

## Format

Each ADR follows a standard format:
- **Status**: Proposed, Accepted, Deprecated, Superseded
- **Context**: The issue motivating this decision
- **Decision**: The change being proposed or implemented
- **Consequences**: The resulting context after applying the decision
- **Alternatives Considered**: Other options that were evaluated
- **References**: Related documents and resources

## Index

### Component Design

| ADR | Title | Status | Date |
|-----|-------|--------|------|
| [ADR-001](ADR-001-exit-code-design.md) | Three-Tier Exit Code Design | Accepted | 2026-02-11 |
| [ADR-002](ADR-002-telemetry-file-location.md) | Telemetry File Location Strategy | Accepted | 2026-02-11 |
| [ADR-003](ADR-003-flag-validation-order.md) | Flag Validation Order and Help Accessibility | Accepted | 2026-02-11 |

### Summary

**ADR-001: Exit Code Design**
- Decision: Use 3-tier exit codes (0=success, 1=error, 2=escalation)
- Rationale: Enable automation to detect human intervention needs
- Impact: Launcher scripts can handle escalations differently from errors

**ADR-002: Telemetry File Location**
- Decision: Colocate all telemetry files with queue file
- Rationale: Simplified discovery, cleanup, and zero-config operation
- Impact: All execution artifacts in queue directory

**ADR-003: Flag Validation Order**
- Decision: Process --version/--help before validating required flags
- Rationale: Help should always be accessible regardless of flag validity
- Impact: Users can get help even with invalid flags

## Creating New ADRs

When creating a new ADR:

1. **Use the next sequential number**: ADR-004, ADR-005, etc.
2. **Follow the naming convention**: `ADR-XXX-short-title.md`
3. **Include all standard sections**: Status, Context, Decision, Consequences, Alternatives
4. **Update this index**: Add entry to the table above
5. **Reference related ADRs**: Link to decisions that influenced or are influenced by this one

### Template

```markdown
# ADR-XXX: Title

## Status

**Proposed** | **Accepted** | **Deprecated** | **Superseded**

## Context

[Describe the issue motivating this decision and any context that influences the decision]

## Decision

[Describe the decision and its implementation]

## Consequences

### Positive
[List benefits of this decision]

### Negative
[List drawbacks and trade-offs]

## Alternatives Considered

### Alternative 1: Name
[Describe alternative approach and why it was rejected]

## Implementation Notes

[Technical details about implementing this decision]

## Related Decisions

[Links to related ADRs]

## References

[External resources, documentation, papers]

## Revision History

| Version | Date | Changes | Author |
|---------|------|---------|--------|
| 1.0 | YYYY-MM-DD | Initial decision record | Author Name |
```

## Decision Process

ADRs are created when:
- Making significant architectural choices
- Changing existing architectural decisions
- Resolving design debates with multiple viable options
- Documenting important trade-offs

ADRs are NOT needed for:
- Minor implementation details
- Obvious choices with no alternatives
- Temporary workarounds
- Bug fixes without architectural impact

## Status Definitions

- **Proposed**: Decision under discussion, not yet implemented
- **Accepted**: Decision approved and implemented
- **Deprecated**: Decision no longer recommended but still in use
- **Superseded**: Decision replaced by a newer ADR (link to replacement)

## Superseding ADRs

When superseding an ADR:
1. Update old ADR status to "Superseded by ADR-XXX"
2. Create new ADR with "Supersedes ADR-XXX"
3. Explain why the original decision changed
4. Document migration path if applicable

Example:
```markdown
# ADR-001: Old Design

## Status

**Superseded** by [ADR-005: New Design](ADR-005-new-design.md)
```

## Related Documentation

- [Component Specification](../../SPEC.md) - Requirements and interface
- [Component Architecture](../../ARCHITECTURE.md) - Overall design
- [System ADRs](../../../../docs/adr/) - System-wide decisions

## Revision History

| Version | Date | Changes | Author |
|---------|------|---------|--------|
| 1.0 | 2026-02-11 | Initial ADR index | Backfill Documentation |
