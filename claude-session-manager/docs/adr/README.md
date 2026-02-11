# Architecture Decision Records (ADRs)

This directory contains Architecture Decision Records (ADRs) for AGM (AI/Agent Gateway Manager).

## What are ADRs?

ADRs document significant architectural decisions made during the development of AGM. Each ADR captures:
- **Context**: Why the decision was needed
- **Decision**: What was chosen
- **Alternatives**: What was considered and rejected
- **Consequences**: Trade-offs and impacts

## ADR Format

Each ADR follows this structure:
- **Status**: Accepted | Deprecated | Superseded
- **Date**: When the decision was made
- **Deciders**: Who made the decision
- **Context**: Background and problem statement
- **Decision**: Chosen approach
- **Alternatives Considered**: What else was evaluated
- **Consequences**: Positive, negative, and neutral impacts
- **Implementation**: Technical details
- **Validation**: How the decision is verified

---

## ADR Index

### Core Architecture

**[ADR-001: Multi-Agent Architecture](ADR-001-multi-agent-architecture.md)**
- **Status**: Accepted (2026-01-15)
- **Summary**: Implement multi-agent support via Agent Adapter pattern with command translation layer
- **Key Decision**: Agent interface with per-agent adapters vs monolithic or multi-binary approaches
- **Impact**: Extensible architecture supporting Claude, Gemini, GPT, and future agents

**[ADR-002: Command Translation Layer](ADR-002-command-translation-layer.md)**
- **Status**: Accepted (2026-01-18)
- **Summary**: Abstract agent-specific commands into unified interface with graceful degradation
- **Key Decision**: Strategy pattern for command translation vs no abstraction or best-effort emulation
- **Impact**: Consistent UX across agents, honest about feature support

**[ADR-004: Tmux Integration Strategy](ADR-004-tmux-integration-strategy.md)**
- **Status**: Accepted (2026-01-10)
- **Summary**: Tight integration with tmux as required dependency using control mode
- **Key Decision**: Tmux as foundation vs terminal-based, custom daemon, or loose integration
- **Impact**: Rock-solid session persistence, battle-tested technology

**[ADR-005: Manifest Versioning Strategy](ADR-005-manifest-versioning-strategy.md)**
- **Status**: Accepted (2026-01-16)
- **Summary**: "Read Old, Write New" versioning with lazy migration
- **Key Decision**: Lazy migration vs immediate migration or dual write
- **Impact**: Backward compatible, safe migration path, rollback capability

---

### User Experience

**[ADR-003: Environment Validation Philosophy](ADR-003-environment-validation-philosophy.md)**
- **Status**: Accepted (2026-01-19)
- **Summary**: "Validate, Don't Manage" - AGM validates environment and provides guidance but doesn't manage it
- **Key Decision**: Validation and guidance vs full environment management or no validation
- **Impact**: Clear errors, security (no secrets stored), compatibility with existing tools

---

### Data & Storage

**Related to ADR-005**: Manifest versioning and schema evolution

---

### Infrastructure

**Related to ADR-004**: Tmux integration and session persistence

---

## ADR Status Definitions

- **Proposed**: ADR is under discussion, not yet accepted
- **Accepted**: ADR has been approved and is being/has been implemented
- **Deprecated**: ADR is no longer applicable (but kept for historical context)
- **Superseded**: ADR has been replaced by a newer ADR (link to successor)

---

## ADR Timeline

```
2026-01-10: ADR-004 - Tmux Integration Strategy
2026-01-15: ADR-001 - Multi-Agent Architecture
2026-01-16: ADR-005 - Manifest Versioning Strategy
2026-01-18: ADR-002 - Command Translation Layer
2026-01-19: ADR-003 - Environment Validation Philosophy
```

---

## Related Documentation

### Product Documentation
- **[AGM Specification](../AGM-SPEC.md)**: Complete product specification
- **[Architecture Overview](../ARCHITECTURE.md)**: System architecture documentation
- **[Command Reference](../AGM-COMMAND-REFERENCE.md)**: Complete CLI reference

### Design Documents
- **[Command Translation Design](../COMMAND-TRANSLATION-DESIGN.md)**: Detailed command translation implementation
- **[Environment Management Spec](../agm-environment-management-spec.md)**: Environment validation specification
- **[BDD Catalog](../BDD-CATALOG.md)**: Behavior-driven development scenarios

---

## ADR Dependencies

```
ADR-001 (Multi-Agent Architecture)
  ├─> ADR-002 (Command Translation Layer)
  ├─> ADR-003 (Environment Validation)
  └─> ADR-005 (Manifest Versioning)

ADR-004 (Tmux Integration)
  └─> ADR-005 (Manifest Versioning)
```

**Dependency Explanation**:
- **ADR-001** is foundational (multi-agent support enables all other features)
- **ADR-002** depends on ADR-001 (command translation requires agent abstraction)
- **ADR-003** depends on ADR-001 (environment validation per agent)
- **ADR-005** depends on ADR-001 (v3 manifest includes `agent` field)
- **ADR-004** is independent (tmux integration predates multi-agent support)

---

## How to Propose a New ADR

1. **Copy Template**:
   ```bash
   cp ADR-TEMPLATE.md docs/adr/ADR-XXX-your-decision.md
   ```

2. **Fill Out Sections**:
   - Status: "Proposed"
   - Context: Why is this decision needed?
   - Decision: What are you proposing?
   - Alternatives: What else did you consider?
   - Consequences: What are the trade-offs?

3. **Submit PR**:
   - Create PR with ADR
   - Tag relevant reviewers
   - Discuss in PR comments

4. **Acceptance**:
   - Once approved, change status to "Accepted"
   - Add to ADR Index above
   - Update related documentation

---

## ADR Review Checklist

When reviewing an ADR, check:

- [ ] **Context** clearly explains the problem
- [ ] **Decision** is specific and actionable
- [ ] **Alternatives** includes at least 2 other options
- [ ] **Consequences** covers positive, negative, and neutral
- [ ] **Validation** describes how to verify the decision
- [ ] **Related Decisions** links to relevant ADRs
- [ ] **Status** is accurate (Proposed, Accepted, etc.)
- [ ] **Date** is included
- [ ] **Deciders** are named

---

## ADR Maintenance

### When to Create an ADR

Create an ADR for decisions that:
- ✅ Have significant architectural impact
- ✅ Affect multiple components
- ✅ Are hard to reverse
- ✅ Have multiple viable alternatives
- ✅ Involve trade-offs

Don't create an ADR for:
- ❌ Implementation details (code structure, naming)
- ❌ Obvious decisions (no alternatives)
- ❌ Temporary decisions (will be revisited soon)
- ❌ Minor features (low impact)

### When to Deprecate an ADR

Deprecate an ADR when:
- Technology evolves and decision is no longer relevant
- Better approach discovered through experience
- Business requirements change

**Process**:
1. Change status to "Deprecated"
2. Add deprecation note explaining why
3. Link to superseding ADR (if applicable)
4. Keep ADR in repo (historical context)

### When to Supersede an ADR

Supersede an ADR when:
- New ADR makes opposite decision
- New ADR refines/extends decision

**Process**:
1. Change old ADR status to "Superseded by ADR-XXX"
2. Create new ADR with status "Supersedes ADR-YYY"
3. Explain in both ADRs why decision changed

---

## Examples of Good ADRs

**Best ADRs in this repo**:
1. **ADR-001**: Clear alternatives, detailed implementation, good consequences
2. **ADR-003**: Multi-persona review, clear philosophy, actionable mitigations
3. **ADR-005**: Backward compatibility strategy, rollback plan, future-proof

**What makes them good**:
- Context is clear (anyone can understand the problem)
- Alternatives are detailed (not just "we considered X")
- Consequences are honest (includes negatives)
- Validation is concrete (BDD scenarios, metrics)

---

## References

- **ADR Concept**: https://adr.github.io/
- **Michael Nygard's ADR Template**: https://github.com/joelparkerhenderson/architecture-decision-record
- **When to Use ADRs**: https://cognitect.com/blog/2011/11/15/documenting-architecture-decisions

---

**Maintained by**: Foundation Engineering
**Last Updated**: 2026-02-11
**AGM Version**: 3.0+
