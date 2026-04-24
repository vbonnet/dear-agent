# ADR 001: Documentation-Only Library

**Status**: Accepted
**Date**: 2025-12-13
**Deciders**: Devlog Maintainers
**Context**: Initial devlog architecture design

---

## Context

Devlog needs to capture AI-assisted development best practices, patterns, and templates. The question arose: should devlog include executable code (scripts, automation, tools) or focus solely on documentation?

Several options were considered:
1. Documentation only (patterns, guides, templates)
2. Documentation + migration scripts
3. Documentation + full automation toolkit
4. Hybrid approach with optional tooling

**Related Decisions**:
- Relationship to `agm` (implements session management)
- Relationship to `engram` (implements knowledge management)
- Ownership and maintenance model

---

## Decision

**Devlog will be a documentation-only library containing no executable code.**

Devlog provides:
- Pattern definitions
- Migration guides
- Templates (AGENTS.md, README.md)
- Examples from real usage
- Best practices documentation

Devlog does NOT provide:
- Migration scripts
- Automation tools
- CLI utilities
- Programmatic interfaces

---

## Rationale

### Separation of Concerns

**Documentation vs. Implementation**:
- **Documentation** (devlog): "What to do" and "why"
- **Implementation** (tools): "How to automate it"

This separation provides:
- **Stability**: Documentation changes less frequently than code
- **Clarity**: Clear boundary between patterns and automation
- **Flexibility**: Multiple tools can implement same pattern

**Example**:
- Devlog documents bare repository pattern
- Users can implement manually or use migration script
- Script can live in separate tools repository
- Pattern documentation remains stable

### Reduced Maintenance Burden

**Code Requires**:
- Testing infrastructure
- Dependency management
- Security updates
- Bug fixes
- Platform compatibility
- Version management

**Documentation Requires**:
- Content accuracy
- Example validation
- Cross-reference updates
- Periodic review

Documentation-only significantly reduces maintenance overhead.

### Lower Barrier to Contribution

**Contributing Code**:
- Requires programming skills
- Needs test coverage
- Must handle edge cases
- Security considerations
- Code review process

**Contributing Documentation**:
- Markdown editing skills
- Real usage example
- Pattern validation
- Content review

Documentation-only lowers contribution barrier, encouraging community input.

### Clearer Value Proposition

**Devlog Position**:
- "Library of best practices for AI-assisted development"
- Clear, focused purpose
- Easy to explain and understand

**With Code**:
- "Framework/toolkit/tool for..."
- Overlaps with existing tools
- Unclear differentiation

Documentation-only creates clear differentiation from tools like `agm` and `engram`.

### Tool Independence

**Documentation-Only Benefits**:
- Works with any tooling
- Users choose their own automation approach
- Multiple tools can reference same patterns
- Not tied to specific implementation

**Example**:
- Workspace patterns documented once
- `agm` can automate workspace setup
- Custom scripts can implement migrations
- Manual following of patterns also valid

---

## Consequences

### Positive

**Stability**:
- Documentation changes infrequently
- No breaking changes from code updates
- Patterns remain valid longer

**Maintainability**:
- Lower maintenance burden
- No security vulnerabilities in code
- Simpler review process

**Accessibility**:
- Easier to contribute
- No programming required
- Markdown skills sufficient

**Clarity**:
- Clear purpose and scope
- Obvious differentiation from tools
- Focused value proposition

**Flexibility**:
- Users can automate or follow manually
- Multiple implementation approaches
- Not locked into specific tooling

### Negative

**Manual Effort Required**:
- Users must implement patterns manually
- Migration requires manual steps or separate scripts
- No one-click automation

**Duplication Risk**:
- Multiple tools may implement same automation
- No canonical implementation provided
- Potential inconsistency across tools

**Slower Adoption**:
- Higher friction than automated tools
- Requires user effort to apply patterns
- Less "magical" than automated solutions

### Mitigation Strategies

**For Manual Effort**:
- Provide detailed step-by-step guides
- Include complete examples with commands
- Reference external automation tools where they exist
- Templates reduce customization effort

**For Duplication Risk**:
- Document patterns clearly and completely
- Provide reference implementations (links to tools)
- Encourage tool developers to reference devlog
- Clear specification reduces interpretation variance

**For Adoption**:
- Make documentation highly accessible
- Provide quick-start paths
- Show value through real examples
- Reference success stories

---

## Alternatives Considered

### Alternative 1: Documentation + Migration Scripts

**Approach**: Include migration scripts alongside documentation

**Pros**:
- One-click migrations
- Canonical implementation
- Lower user friction

**Cons**:
- Testing burden
- Platform compatibility
- Security surface
- Maintenance overhead
- Unclear boundary with tools

**Rejected Because**: Maintenance burden outweighs convenience, overlaps with tools.

### Alternative 2: Full Automation Toolkit

**Approach**: Build complete automation around patterns

**Pros**:
- Comprehensive solution
- Lowest user friction
- Consistent implementation

**Cons**:
- Massive scope increase
- Overlaps with `agm`
- High maintenance burden
- Requires programming expertise
- Longer time to initial value

**Rejected Because**: Out of scope, overlaps with existing tools, unsustainable.

### Alternative 3: Hybrid with Optional Tooling

**Approach**: Documentation core + optional scripts in separate directory

**Pros**:
- Flexibility (use or ignore scripts)
- Documentation remains stable
- Some automation available

**Cons**:
- Mixed responsibilities
- Unclear what's maintained
- Scripts become out of sync with docs
- Partial maintenance burden

**Rejected Because**: Adds complexity without clear benefit, maintenance still required.

---

## Related Decisions

**ADR 002**: Hub-and-spoke navigation structure
**ADR 003**: Dual template system (AGENTS.md + README.md)
**ADR 004**: Real examples required for all patterns

---

## References

**Successful Documentation-Only Projects**:
- [C4 Model](https://c4model.com/) - Architecture documentation
- [ADR Tools](https://adr.github.io/) - Decision record patterns
- [Architectural Patterns](https://martinfowler.com/) - Software patterns

**Related Implementations**:
- `agm`: Session management automation
- `engram`: Knowledge management system
- Git worktrees migration scripts (separate from devlog)

---

## Review History

**2025-12-13**: Initial decision (workspace patterns created)
**2025-12-19**: Validated (repository patterns added without code)
**2026-02-11**: Documented in ADR (backfill documentation)

**Next Review**: 2026-05-11
