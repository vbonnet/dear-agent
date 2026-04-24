# ADR 004: Real Examples Required for All Patterns

**Status**: Accepted
**Date**: 2025-12-13
**Deciders**: Devlog Maintainers
**Context**: Documentation quality and credibility standards

---

## Context

Devlog documents patterns for AI-assisted development, including workspace organization, repository structure, and development practices. The question arose: should patterns be documented based on theoretical design, or should they require validation through real-world usage?

**Competing Considerations**:
- **Speed**: Theoretical patterns can be documented immediately
- **Quality**: Real examples provide credibility and reveal edge cases
- **Completeness**: Waiting for real usage may delay documentation
- **Accuracy**: Hypothetical examples may not reflect actual usage

**Pattern Sources**:
- Theoretical designs (not yet implemented)
- Pilot implementations (limited usage)
- Production usage (validated in real workspaces)

---

## Decision

**All patterns in devlog must be backed by real examples from production usage before documentation.**

**Requirements**:
- At least one real example from actual usage
- Example must include before/after state (for migrations)
- Lessons learned captured from real experience
- Edge cases documented from actual problems encountered

**Not Acceptable**:
- Hypothetical examples ("imagine a workspace with...")
- Theoretical patterns ("you could organize it like...")
- Untested designs ("this should work for...")
- Synthetic examples created for documentation only

**Documentation Process**:
1. Pattern emerges from real usage
2. Pattern validated in at least one production scenario
3. Example documented with real paths, structure, decisions
4. Pattern generalized from example(s)
5. Template created from validated pattern
6. Additional examples added as pattern is reused

---

## Rationale

### Credibility and Trust

**Real Examples Provide**:
- Proof that pattern actually works
- Credibility with users ("this has been done")
- Confidence in applying pattern
- Trust in documentation accuracy

**Hypothetical Examples**:
- Questionable whether pattern works
- Users unsure if they'll hit undocumented issues
- Reduced trust in documentation
- Perception of "vaporware" documentation

**User Perspective**:
> "I'd rather follow a pattern that's been proven in real usage than experiment with a theoretical design."

### Edge Cases and Reality Checks

**Real Usage Reveals**:
- Edge cases not obvious in theory
- Platform-specific issues
- Tool incompatibilities
- Human factors and usability issues

**Example from Workspace Patterns**:
- **Theory**: "Workspaces should have AGENTS.md for navigation"
- **Reality**: Need to distinguish Mono-Repo vs. Multi-Workspace patterns because boundaries differ
- **Edge Case**: Research-vs-Product is meta-pattern, not structure pattern
- **Lesson**: 4 patterns needed, not just "workspace documentation"

Without real usage, these nuances would not have been discovered.

### Lessons Learned Capture

**Real Examples Enable**:
- "What we tried" sections
- "What worked well" findings
- "What we'd do differently" reflections
- Concrete troubleshooting guidance

**Example from Repository Patterns**:
```markdown
### Lessons Learned (acme-app migration)

**What worked well**:
- Starting with clean working directory
- Creating main worktree first

**What didn't work**:
- Attempting migration with uncommitted changes
- Creating worktrees before bare repo setup

**Troubleshooting**:
- If migration fails, restore from backup
- Verify .bare/config before removing old .git
```

This richness only comes from real usage.

### Prevents Premature Standardization

**Risk of Theoretical Patterns**:
- Standardize on design that doesn't work in practice
- Users adopt broken pattern
- Need to deprecate and replace
- Loss of credibility

**Real Usage Validates**:
- Pattern works in actual environment
- Edge cases identified and handled
- Refinements made before documentation
- Users receive working pattern

### Documentation Quality Over Speed

**Trade-off**:
- **Slower**: Must wait for real usage before documenting
- **Higher Quality**: Documentation reflects reality
- **More Valuable**: Users get proven patterns
- **Lower Rework**: Less likely to need major revisions

**Decision**: Quality over speed.

**Rationale**: Better to have 3 validated patterns than 10 theoretical patterns.

---

## Consequences

### Positive

**High Credibility**:
- Users trust documented patterns
- Real examples provide proof
- Lessons learned add value

**Accurate Documentation**:
- Reflects actual usage, not theory
- Edge cases documented from real problems
- Troubleshooting from real issues

**Reduced Rework**:
- Patterns validated before documentation
- Less likely to need major revisions
- Deprecations rare

**Valuable Lessons**:
- Real experience captured
- Users benefit from others' learning
- Compound knowledge over time

**Clear Quality Bar**:
- Contributors know validation is required
- No hypothetical pattern submissions
- Documentation standards clear

### Negative

**Slower Documentation**:
- Must wait for real usage
- Can't document theoretical patterns
- Delays pattern availability

**Missing Coverage**:
- Some useful patterns may not yet be used
- Documentation lags behind community practices
- Users may need undocumented patterns

**Higher Bar for Contribution**:
- Can't contribute theoretical patterns
- Requires real implementation first
- Reduces potential contributions

### Mitigation Strategies

**For Slower Documentation**:
- Accept slower pace as quality trade-off
- Use "Planned" section in README for future patterns
- Encourage experimentation but wait for validation before documenting

**For Missing Coverage**:
- Monitor for repeated questions about undocumented scenarios
- Fast-track documentation when pattern used in production
- Clearly communicate what's not yet documented

**For Higher Contribution Bar**:
- Accept higher bar as maintaining quality
- Provide guidance on validation process
- Celebrate validated contributions

---

## Validation Requirements

### Minimum Requirements for Pattern Documentation

**Before Documenting Pattern**:

1. **At Least One Real Example**:
   - Implemented in production workspace/repository
   - Used for actual work (not demo only)
   - Survived at least one session/workflow

2. **Documentation of Example**:
   - Before state (if migration)
   - After state (current structure)
   - Decision rationale (why this pattern)
   - Lessons learned (what worked/didn't)

3. **Edge Case Identification**:
   - Document any issues encountered
   - Troubleshooting for common problems
   - Limitations or constraints discovered

4. **Generalization**:
   - Pattern abstracted from example
   - Applicable to other scenarios
   - Clear boundaries of applicability

### Example Validation Checklist

**For Workspace Patterns**:
- [ ] Workspace exists in production
- [ ] AGENTS.md and README.md deployed
- [ ] AI agent successfully navigates workspace
- [ ] Humans successfully onboard to workspace
- [ ] Pattern used for >1 week
- [ ] No major issues discovered
- [ ] Lessons learned documented

**For Repository Patterns**:
- [ ] Repository migrated in production
- [ ] Pattern used for actual development
- [ ] Multi-branch workflow validated
- [ ] No regression in functionality
- [ ] Migration repeatable (>1 repo)
- [ ] Lessons learned documented

### Adding Additional Examples

**Process**:
1. New usage of existing pattern
2. Document as additional example
3. Capture any new lessons learned
4. Add to examples.md
5. Update pattern if new insights emerge

**Value**:
- Reinforces pattern validity
- Shows pattern versatility
- Captures diverse lessons learned

---

## Alternatives Considered

### Alternative 1: Document Theoretical Patterns

**Approach**: Document patterns based on design, before implementation

**Pros**:
- Fast documentation
- Comprehensive coverage
- Can document future patterns

**Cons**:
- Unknown if patterns work
- No real lessons learned
- Reduced credibility
- Potential rework when reality differs

**Rejected Because**: Credibility and accuracy more valuable than speed.

### Alternative 2: Require Multiple Examples

**Approach**: Require 3+ real examples before documenting pattern

**Pros**:
- Very high confidence in pattern
- Diverse lessons learned
- Proven versatility

**Cons**:
- Very slow documentation
- High barrier to entry
- May never document some patterns
- Excessive validation

**Rejected Because**: One real example sufficient for validation, more can be added later.

### Alternative 3: Pilot Then Document

**Approach**: Pilot pattern in test workspace, then document

**Pros**:
- Faster than production validation
- Still provides real example
- Lower risk

**Cons**:
- Pilot may not reveal production issues
- Test workspaces don't capture real usage patterns
- Less credible than production examples

**Rejected Because**: Production usage provides more valuable lessons than pilots.

### Alternative 4: Community Contributions Without Validation

**Approach**: Accept pattern documentation from community without validation requirement

**Pros**:
- Fast growth of documentation
- Community-driven
- Diverse patterns

**Cons**:
- Quality inconsistency
- Hypothetical patterns likely
- Difficult to maintain quality bar
- Credibility suffers

**Rejected Because**: Quality and credibility are core values for devlog.

---

## Implementation Examples

### Workspace Patterns (Validated)

**Pattern 1: Mono-Repo**
- **Example**: oss/ workspace (engram-research)
- **Real Usage**: 119 wayfinder projects, active development
- **Lessons Learned**: Need INDEX.md for directory listing, projects/ for all wayfinders

**Pattern 2: Multi-Workspace**
- **Example**: oss/ vs. acme/ separation
- **Real Usage**: Confidentiality boundary enforced
- **Lessons Learned**: Can't reference across boundary, separate session-managers

**Pattern 3: Sub-Workspace**
- **Example**: acme/acme-app/ nested in acme/
- **Real Usage**: Product-within-company structure
- **Lessons Learned**: Parent AGENTS.md must acknowledge sub-workspace

**Pattern 4: Research-vs-Product**
- **Example**: oss/ (engram-research) vs. ~/repos/engram/base/ (product)
- **Real Usage**: Clear separation prevents product contamination in research
- **Lessons Learned**: Meta-pattern, clarify in README not AGENTS.md

**Total Examples**: 4 patterns, 4 real examples, 17 misplacements prevented

### Repository Patterns (Validated)

**Pattern: Bare Repo + Worktrees**
- **Examples**: 9 repositories migrated (engram, acme-app, acme-infra, grouper, aegis, ai-tools, beads, velvet, vpaste)
- **Real Usage**: Multi-branch workflows in production
- **Lessons Learned**:
  - Start with clean working directory
  - Create main worktree first
  - Use .bare/ subdirectory (community standard)
  - Integration with git-worktrees plugin works well

**Total Examples**: 1 pattern, 9 real migrations

---

## Future Patterns

### Planned (Not Yet Documented - Awaiting Validation)

**Methodologies**:
- Multi-persona review: Needs 1 complete review example
- Validation methodology: Needs 1 three-tier validation example
- Gap analysis: Needs 1 complete gap analysis example

**Debugging**:
- Debug patterns: Needs catalog of 5+ real debug scenarios
- Debug scripts: Needs 3+ reusable script examples
- Troubleshooting: Needs decision tree validated on real issues

**Research**:
- Tier classification: Needs 10+ classified research items
- Trusted sources: Needs curated list with usage validation
- Synthesis patterns: Needs 3+ multi-source synthesis examples

**Archiving**:
- Archival criteria: Needs 20+ archival decisions
- Restoration: Needs 3+ restoration examples
- Batch processing: Needs 1 batch archival session

These will be documented only after validation in real usage.

---

## Related Decisions

**ADR 001**: Documentation-only library (real examples don't require code)
**ADR 002**: Hub-and-spoke navigation (examples integrated into navigation)
**ADR 003**: Dual template system (templates derived from real examples)

---

## References

**Similar Quality Requirements**:
- [C4 Model](https://c4model.com/): Real examples for every pattern
- [Design Patterns (Gang of Four)](https://en.wikipedia.org/wiki/Design_Patterns): Patterns from real software
- [Enterprise Integration Patterns](https://www.enterpriseintegrationpatterns.com/): Real integration scenarios

**Quality Over Speed**:
- Agile: "Working software over comprehensive documentation" (but when documented, must reflect working software)
- Scientific Method: Observation before theory
- Engineering: Prototype before specification

---

## Review History

**2025-12-13**: Initial decision (workspace patterns required real examples)
**2025-12-19**: Validated (repository patterns included 9 real migrations)
**2026-02-11**: Documented in ADR (backfill documentation)

**Next Review**: 2026-05-11

**Success Metrics**:
- 100% of documented patterns have real examples (achieved)
- 0 deprecated patterns due to not working (achieved)
- User trust high (qualitative feedback positive)
