# ADR 002: Hub-and-Spoke Navigation Structure

**Status**: Accepted
**Date**: 2025-12-13
**Deciders**: Devlog Maintainers
**Context**: Workspace patterns documentation organization

---

## Context

With multiple documentation files covering workspace patterns, repository patterns, and various guides, users need an effective way to navigate to relevant content. The question arose: how should documentation be organized for maximum discoverability?

**Challenges**:
- Users arrive with different goals (create workspace, migrate repo, understand pattern)
- Both humans and AI agents need navigation
- Content depth varies (quick reference vs. comprehensive guide)
- Cross-references needed between related topics

**Options Considered**:
1. Flat structure (all files at same level)
2. Deep hierarchy (nested categories)
3. Hub-and-spoke (central navigation hubs)
4. Index-based (separate index files)
5. Tag-based (metadata-driven discovery)

---

## Decision

**Devlog will use hub-and-spoke navigation structure with README.md files as central hubs.**

**Structure**:
```
devlog/
├── README.md                    # Root hub (system overview)
├── session-artifact-tracking.md # Single-file component
├── workspace-patterns/
│   ├── README.md                # Component hub
│   ├── patterns.md              # Spoke: pattern definitions
│   ├── examples.md              # Spoke: real examples
│   ├── decision-tree.md         # Spoke: pattern selection
│   ├── migration-guide.md       # Spoke: existing workspaces
│   └── templates/               # Spoke: reusable templates
└── repo-patterns/
    ├── README.md                # Component hub
    ├── bare-repo-guide.md       # Spoke: comprehensive guide
    └── examples.md              # Spoke: migration examples
```

**Navigation Flow**:
1. User enters at root README.md or component README.md
2. Hub guides to appropriate spoke based on user need
3. Spokes provide detailed content
4. Cross-references connect related spokes
5. User can always return to hub

---

## Rationale

### Clear Entry Points

**Every component has an obvious entry point** (README.md):
- Users know where to start
- Consistent across all components
- GitHub automatically displays README.md
- AI agents can find navigation hub reliably

**Example**:
```
Lost? → Go to README.md → Guided to right content
```

### Progressive Disclosure

**Hub provides quick overview, spokes provide depth**:

**At Hub Level**:
- What is this component?
- When to use it?
- Quick navigation to detailed docs

**At Spoke Level**:
- Comprehensive information
- Step-by-step guides
- Examples and troubleshooting

Users can stay at hub level for quick decisions or dive into spokes for depth.

### Scalability

**Hub-and-spoke scales well as content grows**:

**Current**: 3 components, 4-6 spokes per component
**Sustainable**: Up to ~10 spokes per component
**Scaling Strategy**: Add sub-hubs if component exceeds 10 spokes

**Example Growth**:
```
workspace-patterns/
├── README.md (hub)
├── basic-patterns/
│   ├── README.md (sub-hub)
│   ├── mono-repo.md
│   └── multi-workspace.md
└── advanced-patterns/
    ├── README.md (sub-hub)
    ├── hybrid.md
    └── custom.md
```

### Multi-Audience Support

**Hubs serve both humans and AI agents**:

**For Humans**:
- Quick navigation flowcharts
- "I want to..." scenario mapping
- Visual organization of content

**For AI Agents**:
- Clear file structure for navigation
- Standardized hub location (README.md)
- Cross-references for context

Both audiences benefit from centralized navigation.

### Reduced Navigation Burden

**Hub-and-spoke reduces clicks to content**:

**Flat Structure**: Browse all files, unclear which to read
**Deep Hierarchy**: Many clicks to reach content
**Hub-and-Spoke**: 2 clicks (hub → spoke)

**Navigation Path**:
```
Root README → Component README → Detailed Doc
(1 click)      (1 click)          (content)
```

### Maintainability

**Clear organization simplifies maintenance**:

**Adding Content**: Add spoke, update hub
**Reorganizing**: Move spokes, update hub references
**Deprecating**: Remove spoke, remove from hub

Hub is single point of navigation update.

---

## Consequences

### Positive

**Discoverability**:
- Users find content quickly
- Obvious entry points (README.md)
- Guided navigation reduces confusion

**Usability**:
- Progressive disclosure (overview → detail)
- Multi-audience support (humans + AI)
- Minimal clicks to content

**Scalability**:
- Supports growth to ~10 spokes per component
- Sub-hubs enable further scaling
- Clear expansion strategy

**Maintainability**:
- Single point for navigation updates (hub)
- Spokes can be updated independently
- Clear structure for contributors

**Consistency**:
- Predictable pattern across components
- README.md always the hub
- Uniform navigation experience

### Negative

**Hub Maintenance**:
- Hubs must be kept in sync with spokes
- Adding spoke requires hub update
- Hub can become outdated

**Indirection**:
- Always requires hub visit
- Can't go directly to spoke (unless you know path)
- Extra click compared to direct access

**Hub Size**:
- Hubs can grow large with many spokes
- May require sub-categorization
- Risk of overwhelming users

### Mitigation Strategies

**For Hub Maintenance**:
- Include "last updated" dates
- Review hubs when adding/removing spokes
- Automated link checking (future enhancement)

**For Indirection**:
- Provide quick reference tables in hubs
- Enable direct spoke access for experienced users
- Cross-references allow spoke-to-spoke navigation

**For Hub Size**:
- Use quick reference tables for scanning
- Add navigation flowcharts for visual guidance
- Split into sub-hubs if exceeds 500 lines

---

## Alternatives Considered

### Alternative 1: Flat Structure

**Approach**: All files at same level

```
devlog/
├── workspace-patterns.md
├── workspace-examples.md
├── workspace-templates.md
├── repo-patterns.md
├── repo-examples.md
└── session-artifacts.md
```

**Pros**:
- Simple directory listing
- No indirection
- Direct file access

**Cons**:
- Doesn't scale beyond ~10 files
- Unclear which file to read first
- No guided navigation
- File names become unwieldy

**Rejected Because**: Doesn't scale, unclear navigation, poor discoverability.

### Alternative 2: Deep Hierarchy

**Approach**: Nested categories

```
devlog/
├── workspace/
│   ├── patterns/
│   │   ├── mono-repo.md
│   │   ├── multi-workspace.md
│   │   └── sub-workspace.md
│   ├── examples/
│   │   └── real-world.md
│   └── templates/
│       └── agents-readme.md
└── repository/
    ├── patterns/
    │   └── bare-repo.md
    └── examples/
        └── migrations.md
```

**Pros**:
- Very organized
- Clear categorization
- Infinite nesting possible

**Cons**:
- Many clicks to reach content
- Unclear entry point
- Hard to discover content
- Over-engineering for current size

**Rejected Because**: Too many clicks, poor discoverability, over-complex.

### Alternative 3: Index-Based

**Approach**: Separate index files for navigation

```
devlog/
├── INDEX.md                    # Master index
├── workspace-patterns.md
├── workspace-examples.md
├── workspace-templates.md
├── workspace-index.md          # Component index
├── repo-patterns.md
└── repo-examples.md
```

**Pros**:
- Flexible navigation
- Can have multiple indexes
- Easy to maintain indexes

**Cons**:
- Duplicate navigation information
- Indexes can become stale
- Not obvious which index to use
- Extra files to maintain

**Rejected Because**: Duplicate effort, unclear which index to use, maintenance burden.

### Alternative 4: Tag-Based

**Approach**: Metadata-driven discovery

```markdown
---
tags: [workspace, pattern, mono-repo]
audience: [developer, ai-agent]
level: [beginner]
---
# Workspace Patterns
```

**Pros**:
- Flexible discovery
- Multiple navigation paths
- Searchable metadata

**Cons**:
- Requires tooling for tag search
- Not human-friendly without tools
- Metadata can become stale
- No guided navigation

**Rejected Because**: Requires tooling (conflicts with documentation-only), not human-friendly.

---

## Implementation Guidelines

### Hub Requirements

**Every component hub (README.md) must include**:

1. **Quick Overview**: What is this component?
2. **Quick Start**: "I want to..." scenario mapping
3. **Navigation**: Clear paths to each spoke
4. **Quick Reference**: Tables for fast lookup
5. **File Relationships**: How spokes connect

**Template**:
```markdown
# Component Name

**Purpose**: Brief description

## Quick Start

### I Want to Create New X
Start here: [file.md](file.md)

### I Want to Migrate Existing Y
Start here: [other.md](other.md)

## Documentation Files

### [spoke1.md](spoke1.md)
Brief description, when to use

### [spoke2.md](spoke2.md)
Brief description, when to use

## Navigation Flowchart
(visual guide)
```

### Spoke Requirements

**Every spoke must include**:

1. **Purpose**: What problem does this solve?
2. **Audience**: Who should read this?
3. **Prerequisites**: What to read first (if any)
4. **Content**: Comprehensive information
5. **Cross-References**: Links to related spokes

### Cross-Reference Strategy

**Spokes can reference each other without going through hub**:

```markdown
See also: [examples.md](examples.md) for real-world usage
Related: [decision-tree.md](decision-tree.md) for pattern selection
```

This enables spoke-to-spoke navigation for experienced users while maintaining hub as primary entry point.

---

## Related Decisions

**ADR 001**: Documentation-only library (no navigation tooling)
**ADR 003**: Dual template system (AGENTS.md for agents, README.md for humans)
**ADR 004**: Real examples required for all patterns

---

## References

**Similar Approaches**:
- Rust documentation (module-based hubs)
- Django documentation (guide-based navigation)
- AWS Well-Architected Framework (pillar-based organization)

**Documentation Patterns**:
- [Divio Documentation System](https://documentation.divio.com/) (tutorials, how-to, reference, explanation)
- [Google Developer Documentation Style Guide](https://developers.google.com/style) (navigation principles)

---

## Review History

**2025-12-13**: Initial decision (workspace patterns used hub-and-spoke)
**2025-12-19**: Validated (repository patterns adopted same structure)
**2026-02-11**: Documented in ADR (backfill documentation)

**Next Review**: 2026-05-11
