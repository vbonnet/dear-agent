# Architecture Decision Records (ADRs)

This directory contains Architecture Decision Records for the Devlog library, documenting key architectural decisions, their context, rationale, and consequences.

---

## What are ADRs?

Architecture Decision Records capture important architectural decisions made during the project, including:
- **Context**: What situation led to the decision
- **Decision**: What was decided
- **Rationale**: Why this decision was made
- **Consequences**: What are the positive and negative outcomes
- **Alternatives**: What other options were considered and why they were rejected

ADRs provide historical context for future maintainers and contributors, explaining not just what the architecture is, but why it is that way.

---

## ADR Index

### ADR 001: Documentation-Only Library
**Status**: Accepted | **Date**: 2025-12-13

**Decision**: Devlog will be a documentation-only library containing no executable code.

**Key Points**:
- Clear separation between "what to do" (devlog) and "how to automate it" (tools)
- Reduced maintenance burden (no code testing, dependencies, security updates)
- Lower barrier to contribution (markdown editing vs. coding)
- Clear differentiation from `agm` and `engram`

**Consequences**:
- ✅ Stable documentation, simpler maintenance, easier contributions
- ❌ Manual effort required, no automation provided
- ✅ Mitigation: Detailed step-by-step guides, templates, tool references

[Read Full ADR →](001-documentation-only-library.md)

---

### ADR 002: Hub-and-Spoke Navigation Structure
**Status**: Accepted | **Date**: 2025-12-13

**Decision**: Devlog will use hub-and-spoke navigation with README.md files as central hubs.

**Key Points**:
- Every component has README.md as navigation hub
- Detailed docs ("spokes") linked from hub
- Progressive disclosure (overview → detail)
- Scales to ~10 spokes per component

**Structure**:
```
component/
├── README.md           # Hub (navigation, overview)
├── detailed-doc-1.md   # Spoke (comprehensive info)
├── detailed-doc-2.md   # Spoke
└── templates/          # Spoke (reusable content)
```

**Consequences**:
- ✅ Clear entry points, progressive disclosure, scales well
- ❌ Hub maintenance required, extra click for direct access
- ✅ Mitigation: Quick reference tables, cross-references, sub-hubs if needed

[Read Full ADR →](002-hub-and-spoke-navigation.md)

---

### ADR 003: Dual Template System (AGENTS.md + README.md)
**Status**: Accepted | **Date**: 2025-12-13

**Decision**: Workspaces will have dual documentation files: AGENTS.md for AI agents and README.md for humans.

**Key Points**:
- **AGENTS.md**: Concise, structured navigation for AI agents (70-80 lines)
- **README.md**: Comprehensive documentation for humans (95-110 lines)
- Different information needs require different formats
- Follows precedent of README.md + machine-readable config (package.json, Cargo.toml)

**Rationale**:
- AI agents need: "Where are projects?" "What are boundaries?"
- Humans need: "What is this for?" "Why does it exist?" "How do I contribute?"
- Single file would be suboptimal for both audiences

**Consequences**:
- ✅ Optimized for each audience, clear purpose, established convention
- ❌ Dual maintenance, potential redundancy
- ✅ Mitigation: Templates include common content, clear update guidance

[Read Full ADR →](003-dual-template-system.md)

---

### ADR 004: Real Examples Required for All Patterns
**Status**: Accepted | **Date**: 2025-12-13

**Decision**: All patterns must be backed by real examples from production usage before documentation.

**Key Points**:
- At least one real example required before documenting pattern
- Examples must include before/after, lessons learned, edge cases
- No hypothetical or theoretical patterns
- Documentation process: usage → validation → documentation → generalization

**Rationale**:
- Real examples provide credibility and trust
- Real usage reveals edge cases and reality checks
- Lessons learned only come from actual experience
- Prevents premature standardization on broken patterns

**Results**:
- Workspace patterns: 4 patterns, 4 real examples
- Repository patterns: 1 pattern, 9 real migrations
- Zero deprecated patterns due to not working

**Consequences**:
- ✅ High credibility, accurate docs, valuable lessons, reduced rework
- ❌ Slower documentation, missing coverage for unused patterns
- ✅ Mitigation: Accept quality over speed, fast-track validated patterns

[Read Full ADR →](004-real-examples-required.md)

---

### ADR 005: Bare Repository Subdirectory Pattern (.bare/)
**Status**: Accepted | **Date**: 2025-12-19

**Decision**: Bare repository will be in `.bare/` subdirectory with worktrees as siblings.

**Structure**:
```
repo/
├── .bare/              # Bare repository (git internals)
├── main/               # Worktree for main branch
├── feature-x/          # Worktree for feature-x branch
└── hotfix-y/           # Worktree for hotfix-y branch
```

**Key Points**:
- Clear separation of git internals from working directories
- Aligns with 2025 community standard
- Cleaner directory listings (`ls` shows worktrees, not git files)
- Better developer experience (intuitive, tab completion)

**Rationale**:
- Alternative (bare at root) mixes git files with worktrees in listings
- Community consensus shifted to `.bare/` subdirectory
- Follows git convention of hidden internals (like `.git/`)

**Validation**: 9/9 repository migrations used `.bare/` subdirectory (100% adoption)

**Consequences**:
- ✅ Clarity, community alignment, better DX, consistency
- ❌ Extra directory level in commands, migration complexity
- ✅ Mitigation: Command examples in docs, clear migration guide

[Read Full ADR →](005-bare-repo-subdirectory-pattern.md)

---

## ADR Status

**Accepted**: Decision is approved and implemented
**Proposed**: Decision is suggested but not yet implemented
**Deprecated**: Decision has been superseded by another ADR
**Superseded**: Decision replaced by newer ADR (link provided)

---

## Creating New ADRs

When making significant architectural decisions:

1. **Use ADR Template**:
   ```markdown
   # ADR XXX: [Title]

   **Status**: [Proposed|Accepted|Deprecated|Superseded]
   **Date**: YYYY-MM-DD
   **Deciders**: [Who made this decision]
   **Context**: [What triggered this decision]

   ## Context
   [Situation and problem statement]

   ## Decision
   [What was decided]

   ## Rationale
   [Why this decision was made]

   ## Consequences
   ### Positive
   ### Negative
   ### Mitigation Strategies

   ## Alternatives Considered
   [What else was considered and why rejected]

   ## Related Decisions
   [Links to related ADRs]
   ```

2. **Number Sequentially**: Use next available number (e.g., 006)

3. **Update This Index**: Add entry with summary

4. **Link from Documentation**: Reference ADR in relevant docs

---

## ADR Principles

**When to Create an ADR**:
- Architectural decisions with long-term impact
- Trade-offs between competing alternatives
- Decisions that need historical context
- Choices that future contributors might question

**When NOT to Create an ADR**:
- Implementation details
- Minor documentation changes
- Obvious decisions with no alternatives
- Temporary decisions

**ADR Quality**:
- Clear problem statement
- Explicit decision
- Well-reasoned rationale
- Honest consequences (positive and negative)
- Documented alternatives with rejection rationale

---

## Related Documentation

**Devlog Specification**: `../SPEC.md` - What devlog is and what problems it solves
**Devlog Architecture**: `../ARCHITECTURE.md` - How devlog is structured and organized

**Relationship**:
- **SPEC.md**: Product specification (requirements, features, success metrics)
- **ARCHITECTURE.md**: System design (components, patterns, quality mechanisms)
- **ADRs**: Decision records (why architecture is the way it is)

---

**Last Updated**: 2026-02-11
**ADR Count**: 5
**Next ADR Number**: 006
