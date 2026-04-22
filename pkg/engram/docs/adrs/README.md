# Architecture Decision Records (ADRs)

This directory contains Architecture Decision Records (ADRs) for the engram package.

## What are ADRs?

ADRs document key architectural decisions made during the design and implementation of the engram package. Each ADR captures:

- **Context**: What problem were we trying to solve?
- **Decision**: What did we decide to do?
- **Consequences**: What are the positive and negative impacts?
- **Alternatives**: What other options did we consider and why were they rejected?

ADRs provide historical context for why the code is structured the way it is, helping future developers understand design rationale.

## ADR Index

### ADR-001: YAML Frontmatter Format
**Status**: Accepted
**Date**: 2026-02-11 (Backfilled)

Chose YAML frontmatter delimited by `---` lines for engram metadata storage. Enables human-readable, machine-parseable files compatible with static site generators and version control systems.

**Key Decision**: Use `---\n` delimiters with YAML content for metadata, followed by markdown content.

**Rationale**: YAML is more readable than JSON, widely supported in markdown ecosystems (Jekyll, Hugo), and version-control friendly.

### ADR-002: Backward Compatibility via Defaults
**Status**: Accepted
**Date**: 2026-02-11 (Backfilled)

Apply default values for missing metadata fields during parsing (not file rewriting). Enables legacy engrams to work without modification while supporting new memory strength tracking features.

**Key Decision**:
- `encoding_strength`: Default to 1.0 (neutral)
- `retrieval_count`: Default to 0 (never retrieved)
- `created_at`: Default to file mtime (legacy) or current time (new)
- `last_accessed`: Default to zero (never accessed)

**Rationale**: Non-invasive (files never modified), zero breaking changes, gradual migration path.

### ADR-003: Memory Strength Tracking Fields
**Status**: Accepted
**Date**: 2026-02-11 (Backfilled)

Add four metadata fields to support advanced retrieval features: `encoding_strength` (quality), `retrieval_count` (usage), `created_at` (age), and `last_accessed` (recency).

**Key Decision**: Track intrinsic quality, usage patterns, and temporal data to enable quality-based ranking, usage-based ranking, temporal decay, and active forgetting.

**Rationale**: Enables data-driven retrieval improvements inspired by human memory systems (encoding strength, retrieval frequency, temporal decay).

## ADR Format

Each ADR follows this structure:

1. **Title**: ADR-XXX: [Decision Name]
2. **Status**: Accepted / Proposed / Deprecated / Superseded
3. **Date**: YYYY-MM-DD (when decision was made or backfilled)
4. **Deciders**: Who made the decision
5. **Context**: Problem statement, constraints, requirements
6. **Decision**: What was decided and key design elements
7. **Consequences**: Positive and negative impacts
8. **Alternatives Considered**: Other options and why rejected
9. **Implementation Notes**: Code examples, pseudo-code
10. **Related Decisions**: Links to related ADRs
11. **References**: External resources, papers, standards
12. **Revision History**: Changes to the ADR over time

## Creating New ADRs

When making a new architectural decision:

1. Copy the template from `docs/adr/000-template.md`
2. Number sequentially (ADR-004, ADR-005, etc.)
3. Fill in all sections (don't skip Alternatives or Consequences)
4. Link to related ADRs
5. Commit with descriptive message: `docs: Add ADR-XXX for [decision]`

## Reading ADRs

**For new contributors**:
- Start with ADR-001 (foundational format decision)
- Read ADR-002 and ADR-003 to understand metadata handling
- Refer to specific ADRs when modifying related code

**For debugging**:
- If code behavior seems unexpected, check ADRs for design rationale
- Understand trade-offs before proposing changes

**For refactoring**:
- Review ADRs to understand why current design was chosen
- Update or supersede ADRs if design changes

## ADR Status Definitions

- **Proposed**: Decision under discussion, not yet implemented
- **Accepted**: Decision made and implemented in code
- **Deprecated**: Decision no longer recommended, but code may still exist
- **Superseded**: Replaced by newer ADR (link to replacement)

## Backfilled ADRs

ADRs 001-003 were backfilled on 2026-02-11 to document existing implementation decisions. These ADRs describe the current state of the code, not proposals for future changes.

---

**Maintained by**: Engram Core Team
**Questions**: See SPEC.md for requirements, ARCHITECTURE.md for design details
