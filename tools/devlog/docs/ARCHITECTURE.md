# Devlog Architecture

## Overview

Devlog is a documentation-only knowledge base library with no executable code. The architecture focuses on information organization, discoverability, and maintainability.

**Type**: Documentation Library
**Version**: 1.0
**Last Updated**: 2026-02-11

---

## Architectural Principles

### 1. Documentation Only, No Code

**Principle**: Devlog contains documentation, patterns, templates, and guides - no implementation code.

**Rationale**:
- Clear separation between "what to do" (devlog) and "how to automate it" (tools)
- Documentation remains stable even as tooling evolves
- Lower barrier to contribution (markdown editing vs. coding)
- Reduces maintenance burden and security surface

**Implications**:
- Migration scripts referenced but not stored in devlog
- Templates provide structure but require manual customization
- Guides describe processes but don't execute them
- Tools like `agm` and `engram` implement automation

### 2. Pattern-First Organization

**Principle**: Organize content around validated patterns, not arbitrary categories.

**Rationale**:
- Patterns represent proven solutions to recurring problems
- Pattern-based organization aids discoverability
- Reduces "where does this belong?" questions
- Enables pattern reuse across contexts

**Implications**:
- Each pattern gets dedicated documentation
- Patterns include structure, boundaries, invariants, examples
- New content added only after pattern validation
- Hypothetical patterns excluded until proven

### 3. Example-Driven Documentation

**Principle**: Every pattern includes real examples from production usage.

**Rationale**:
- Real examples are more credible than hypothetical scenarios
- Shows pattern application in context
- Reveals edge cases and lessons learned
- Provides validation that pattern actually works

**Implications**:
- Workspace examples from oss/, acme/, acme-app/
- Repository examples from 9 actual migrations
- Before/after comparisons included
- Lessons learned sections based on real problems

### 4. Multi-Audience Design

**Principle**: Serve both human developers and AI agents effectively.

**Rationale**:
- AI agents navigate using AGENTS.md files
- Humans prefer README.md and comprehensive guides
- Both audiences benefit from clear structure
- Cross-references aid both navigation styles

**Implications**:
- Dual template system (AGENTS.md for agents, README.md for humans)
- Clear heading hierarchy for AI parsing
- Navigation flowcharts for human guidance
- Quick reference tables for fast lookups

### 5. Progressive Disclosure

**Principle**: Provide quick navigation to detailed content without overwhelming users.

**Rationale**:
- Users arrive with varying levels of need
- Quick decisions need summary info
- Deep understanding requires comprehensive docs
- Navigation burden should be minimal

**Implications**:
- README.md files serve as navigation hubs
- Decision trees guide pattern selection quickly
- Detailed pattern docs available when needed
- Quick reference tables for common lookups

---

## System Structure

### Component Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                      DEVLOG ROOT                             │
│                  (Documentation Hub)                         │
│                                                              │
│  README.md ─────────┐                                       │
│  SPEC.md            │  Navigation & Overview               │
│  ARCHITECTURE.md    │  System Documentation                │
│  .docs/adr/         │  Decision Records                    │
│                     │                                       │
│  ┌──────────────────┼──────────────────────────────────┐  │
│  │                  │                                    │  │
│  │  SESSION ARTIFACT TRACKING                           │  │
│  │  (Single-File Component)                             │  │
│  │                                                       │  │
│  │  session-artifact-tracking.md                        │  │
│  │   ├─ Artifact categories                             │  │
│  │   ├─ Save location patterns                          │  │
│  │   ├─ Session end protocol                            │  │
│  │   └─ Implementation roadmap                          │  │
│  │                                                       │  │
│  └───────────────────────────────────────────────────────┘  │
│                                                              │
│  ┌──────────────────────────────────────────────────────┐  │
│  │                                                       │  │
│  │  WORKSPACE PATTERNS                                   │  │
│  │  (Multi-File Component)                               │  │
│  │                                                       │  │
│  │  workspace-patterns/                                  │  │
│  │   ├─ README.md ──────────┐  Navigation hub          │  │
│  │   │                       │                           │  │
│  │   ├─ patterns.md ◄────────┤  Pattern definitions     │  │
│  │   ├─ examples.md ◄────────┤  Real examples           │  │
│  │   ├─ decision-tree.md ◄───┤  Pattern selection       │  │
│  │   ├─ migration-guide.md ◄─┤  Existing workspaces     │  │
│  │   │                       │                           │  │
│  │   └─ templates/           │                           │  │
│  │       ├─ AGENTS-*.md ◄────┘  AI agent templates      │  │
│  │       └─ README-*.md         Human templates          │  │
│  │                                                       │  │
│  └───────────────────────────────────────────────────────┘  │
│                                                              │
│  ┌──────────────────────────────────────────────────────┐  │
│  │                                                       │  │
│  │  REPOSITORY PATTERNS                                  │  │
│  │  (Multi-File Component)                               │  │
│  │                                                       │  │
│  │  repo-patterns/                                       │  │
│  │   ├─ README.md ──────────┐  Navigation hub          │  │
│  │   │                       │                           │  │
│  │   ├─ bare-repo-guide.md ◄┤  Comprehensive guide      │  │
│  │   └─ examples.md ◄────────┘  Migration examples      │  │
│  │                                                       │  │
│  └───────────────────────────────────────────────────────┘  │
│                                                              │
└─────────────────────────────────────────────────────────────┘
```

### Component Relationships

```
User Entry Points
       ↓
    README.md ────────────────┐
       ↓                      │
    Decision Point            │
       ↓                      │
   ┌───┴───┬─────────┬────────┴────────┐
   ↓       ↓         ↓                 ↓
Workspace Repository Session      Architecture
Patterns  Patterns   Artifacts    Docs (SPEC, etc.)
   ↓         ↓         ↓                ↓
Navigation Navigation Single       System
Hub        Hub        File         Docs
   ↓         ↓                          ↓
Detailed   Detailed                Decision
Docs       Docs                    Records (.docs/adr/)
   ↓         ↓
Templates  Examples
```

---

## Component Details

### 1. Root Documentation

**Purpose**: System-level navigation and architecture documentation

**Files**:
- `README.md`: Entry point, navigation to subsystems
- `SPEC.md`: Comprehensive specification (this document's companion)
- `ARCHITECTURE.md`: This file - system architecture
- `.docs/adr/`: Architecture decision records

**Design Decisions**:
- Minimal root clutter - most content in subsystem directories
- Root README provides quick overview and navigation
- Architecture docs separate from pattern docs
- ADRs capture key decisions with rationale

**Navigation Flow**:
```
README.md → User identifies need → Directed to component README
```

### 2. Workspace Patterns Component

**Purpose**: Comprehensive workspace organization documentation

**Structure**:
```
workspace-patterns/
├── README.md              # Navigation hub (337 lines)
├── patterns.md            # Pattern definitions (612 lines)
├── examples.md            # Real examples (581 lines)
├── decision-tree.md       # Pattern selection (223 lines)
├── migration-guide.md     # Existing workspaces (477 lines)
└── templates/
    ├── AGENTS-mono-repo.md              (73 lines)
    ├── AGENTS-multi-workspace.md        (78 lines)
    ├── AGENTS-sub-workspace.md          (75 lines)
    ├── README-mono-repo.md              (95 lines)
    ├── README-multi-workspace.md        (107 lines)
    └── README-sub-workspace.md          (98 lines)
```

**Component Architecture**:
```
       README.md (Hub)
            ↓
      User Question
            ↓
    ┌───────┼───────┬────────────┐
    ↓       ↓       ↓            ↓
  New?   Existing? Deep      Examples?
    ↓       ↓    Understanding?  ↓
decision- migration- ↓       examples.md
tree.md   guide.md patterns.md
    ↓       ↓       ↓            ↓
    └───────┴───────┴────────────┘
              ↓
         templates/
```

**Design Patterns**:
- **Hub-and-Spoke**: README.md central hub, specialized docs as spokes
- **Progressive Disclosure**: Quick decision tree → Detailed patterns → Templates
- **Dual Templates**: AGENTS.md for AI agents, README.md for humans
- **Reference Separation**: Patterns (what) separate from Templates (how)

**Information Flow**:
1. User enters via README.md
2. README guides to appropriate document based on need
3. Document provides detailed guidance
4. User obtains template or example
5. Cross-references available for deeper understanding

### 3. Repository Patterns Component

**Purpose**: Multi-branch workflow repository organization

**Structure**:
```
repo-patterns/
├── README.md              # Navigation hub (251 lines)
├── bare-repo-guide.md     # Comprehensive guide (~500 lines)
└── examples.md            # Migration examples (~300 lines)
```

**Component Architecture**:
```
       README.md (Hub)
            ↓
      User Question
            ↓
    ┌───────┴───────┐
    ↓               ↓
Multi-branch    Examples?
workflow?           ↓
    ↓         examples.md
bare-repo-        ↓
guide.md    (9 migrations)
    ↓
(Setup, usage,
troubleshooting)
```

**Design Patterns**:
- **Single Pattern Focus**: Only bare repo + worktrees documented (validated pattern)
- **Guide + Examples**: Comprehensive guide complemented by real examples
- **Integration Documentation**: Clear relationship to git-worktrees plugin
- **Before/After**: Migration examples show transformation

**Simplicity Rationale**:
- Only one validated pattern (bare repo + worktrees)
- Standard `.git` structure doesn't need documentation
- Future patterns added only after validation

### 4. Session Artifact Tracking Component

**Purpose**: Preserve valuable session artifacts

**Structure**:
```
session-artifact-tracking.md (124 lines, single file)
├── Quick reference table
├── Detailed guidelines (7 artifact types)
├── Session end protocol
└── Implementation roadmap
```

**Component Architecture**:
```
session-artifact-tracking.md
         ↓
   Single reference
         ↓
    ┌────┼────┬─────┬──────┬────────┐
    ↓    ↓    ↓     ↓      ↓        ↓
  Retro- Metrics Tools Project Task
  spectives     Closures Snapshots
```

**Design Patterns**:
- **Single File**: All guidance in one location (not complex enough for multi-file)
- **Table-First**: Quick reference table at top for fast lookups
- **Pattern Matching**: Artifact patterns for automated categorization
- **Protocol-Based**: Clear session end protocol for AI assistants

**Simplicity Rationale**:
- Single concern (artifact preservation)
- No sub-categories requiring separate files
- Reference-style documentation fits single file

---

## Information Architecture

### Navigation Strategy

**Three-Level Navigation**:

1. **Level 1: Entry (README.md)**
   - User arrives at devlog/README.md
   - Quick overview of what devlog provides
   - Navigation to subsystems

2. **Level 2: Component Hub (component/README.md)**
   - Component-specific navigation
   - Quick start scenarios
   - Guidance to appropriate detailed docs

3. **Level 3: Detailed Documentation**
   - Comprehensive pattern definitions
   - Step-by-step guides
   - Examples and troubleshooting

**Cross-Cutting Navigation**:
- Quick reference tables in hub docs
- Decision trees for pattern selection
- Flowcharts for navigation guidance
- Cross-references between related docs

### Content Organization Patterns

**Pattern Documentation Structure**:
```
Pattern Definition
├── Introduction (what problem solved)
├── Structure (directory layout)
├── Boundaries (what belongs)
├── Invariants (must hold true)
├── When to Use (scenarios)
├── Examples (real usage)
└── Anti-patterns (what to avoid)
```

**Guide Documentation Structure**:
```
Guide
├── Introduction (purpose, problem)
├── Quick Start (fastest path)
├── Detailed Steps (comprehensive)
├── Examples (real scenarios)
├── Troubleshooting (common issues)
└── References (related docs)
```

**Template Structure**:
```
Template
├── Header (instructions for customization)
├── Placeholders ({{VARIABLE}} format)
├── Example Values (commented or in separate section)
└── Validation Checklist (ensure completeness)
```

---

## Design Decisions

### File Organization

**Decision**: Hub-and-spoke with README.md navigation hubs

**Rationale**:
- Reduces navigation burden (central hub per component)
- Scales well as content grows
- Clear entry point for each subsystem
- Enables progressive disclosure

**Alternatives Considered**:
- Flat structure: Doesn't scale, unclear navigation
- Deep hierarchy: Too many clicks to reach content
- Index-based: Requires maintenance of separate index

### Template Strategy

**Decision**: Dual templates (AGENTS.md for AI, README.md for humans)

**Rationale**:
- AI agents need structured navigation (AGENTS.md)
- Humans prefer comprehensive documentation (README.md)
- Different information density and format needs
- Both audiences critical for success

**Alternatives Considered**:
- Single template: Doesn't serve either audience well
- AI-only templates: Excludes human team members
- Human-only templates: AI agents can't navigate workspace

### Pattern Validation

**Decision**: Only document validated patterns from real usage

**Rationale**:
- Credibility requires real examples
- Untested patterns may not work in practice
- Reduces speculative documentation
- Ensures quality over quantity

**Alternatives Considered**:
- Hypothetical patterns: Less credible, may not work
- Community-submitted patterns: Quality control challenges
- Exhaustive pattern catalog: Maintenance burden

### Content Format

**Decision**: Markdown with ~100 character line width

**Rationale**:
- Markdown is universally readable
- ~100 chars works well in terminal
- Easy to version control and diff
- AI agents parse markdown effectively

**Alternatives Considered**:
- HTML: Requires build step, less terminal-friendly
- Plain text: Lacks structure for navigation
- Wiki format: Requires separate platform

---

## Quality Mechanisms

### Documentation Quality

**Mechanisms**:
1. **Real Examples**: Every pattern backed by production usage
2. **Cross-References**: Links between related documentation verified
3. **Navigation Validation**: Flowcharts tested against user scenarios
4. **Template Testing**: Templates applied to real workspaces

**Validation Process**:
- New pattern requires ≥1 real example before documentation
- Examples include before/after and lessons learned
- Templates customized and deployed to validate usability
- Navigation paths tested with fresh users

### Consistency

**Mechanisms**:
1. **Structural Consistency**: Similar docs use same structure
2. **Naming Consistency**: Files named with clear conventions
3. **Format Consistency**: Tables, code blocks, headings standardized
4. **Placeholder Consistency**: `{{VARIABLE}}` format throughout

**Enforcement**:
- Pattern docs follow template structure
- Guide docs follow guide structure
- Templates use consistent placeholder format
- README.md files use consistent sections

### Maintainability

**Mechanisms**:
1. **Modular Structure**: Components can be updated independently
2. **Version Information**: Last updated dates in major files
3. **Clear Ownership**: Maintainer info in project context sections
4. **Contribution Guidelines**: Process for adding patterns

**Sustainability**:
- New patterns added only after validation
- Quarterly review cycle for existing patterns
- Lessons learned captured and integrated
- Deprecated patterns marked clearly

---

## Integration Architecture

### Tool Integration

**Claude Session Manager**:
```
Claude Session Manager (Tool)
         ↓
   References Devlog
         ↓
Applies Workspace Patterns
         ↓
  Creates AGENTS.md from Templates
```

**Relationship**: Session manager automates what devlog documents manually

**Engram**:
```
Engram (Product)
         ↓
  Documentation References Devlog
         ↓
Applies Patterns for Own Workspace
         ↓
  Plugin Docs Cross-Reference Devlog
```

**Relationship**: Engram is specific product, devlog is general practices

**Git-Worktrees Plugin**:
```
Git-Worktrees Plugin (Temporary)
         ↓
  Documented Separately from Bare Repo Pattern
         ↓
Both Can Coexist (Complementary)
         ↓
  Bare Repo = Permanent, Plugin = Temporary
```

**Relationship**: Different use cases, both valuable, clear distinction

### Workspace Integration

**AGENTS.md in Workspaces**:
```
Workspace Root
├── AGENTS.md  ◄─── Generated from devlog template
│    ├── Structure section
│    ├── Boundary section
│    └── Navigation section
└── README.md  ◄─── Generated from devlog template
     ├── Identity section
     ├── Structure section
     └── Purpose section
```

**Data Flow**:
1. Developer selects pattern via decision tree
2. Obtains appropriate template from devlog
3. Customizes template for specific workspace
4. Deploys AGENTS.md and README.md to workspace
5. AI agents and humans use deployed docs for navigation

---

## Scalability Considerations

### Content Growth

**Current State**: 3 components, ~4200 lines documentation

**Growth Pattern**:
- New patterns added after validation (quarterly)
- Each pattern adds ~1000 lines (pattern + examples + templates)
- Hub-and-spoke scales to ~10 patterns per component

**Scaling Triggers**:
- If component exceeds 10 patterns → Consider component split
- If navigation hub exceeds 500 lines → Add sub-hubs
- If template count exceeds 10 → Add template categories

**Future Components** (not yet implemented):
- Methodologies component (review, validation, gap analysis)
- Debugging component (patterns, scripts, troubleshooting)
- Research component (tier classification, sources, synthesis)
- Archiving component (criteria, restoration, batch processing)

### Navigation Scalability

**Current**: 3-level navigation (root → component → detail)

**Scaling Strategy**:
- Add sub-components if detail level grows beyond 10 docs
- Use category-based navigation in hubs
- Maintain quick reference tables for fast access
- Cross-references for related content

**Limit**: Hub-and-spoke effective to ~50 documents per component

---

## Technology Choices

### Documentation Format: Markdown

**Choice**: GitHub Flavored Markdown

**Reasons**:
- Universal readability (no build required)
- Version control friendly (line-based diffs)
- AI agent parseable (clear structure)
- Terminal displayable (cat, less, bat)
- GitHub rendering (if published)

**Constraints**:
- No interactive elements
- Limited table complexity
- ~100 char line width for terminal

### Placeholder Format: {{VARIABLE}}

**Choice**: Double curly braces for template placeholders

**Reasons**:
- Visually distinct from regular text
- Easy to find/replace programmatically
- Conventional in template systems
- Unlikely to appear in regular markdown

**Example**:
```markdown
# {{WORKSPACE_NAME}}

Location: {{WORKSPACE_PATH}}
```

### Path Format: Tilde Notation

**Choice**: Use `~` instead of `~/` and `{{DEVLOG_ROOT}}/` for variable root

**Reasons**:
- Shorter and more readable
- Shell expansion friendly
- Environment-independent documentation
- Conventional in Unix/Linux docs

**Example**:
```markdown
Location: ./
Root: {{DEVLOG_ROOT}}/repos/engram/
```

---

## Security and Privacy

### No Sensitive Data

**Principle**: Devlog contains no credentials, tokens, or sensitive information

**Enforcement**:
- Examples use placeholder values
- Real paths anonymized where necessary
- No API keys or tokens in examples
- Code snippets sanitized

### Public Shareable

**Design**: All devlog content designed for public sharing

**Implications**:
- Can be open-sourced without redaction
- Templates safe to share across teams
- Examples don't reveal confidential information
- Patterns applicable to any organization

---

## Testing and Validation

### Documentation Testing

**Validation Methods**:
1. **Navigation Testing**: Verify all cross-references work
2. **Template Testing**: Apply templates to real workspaces
3. **Example Validation**: Ensure examples reflect real state
4. **Decision Tree Testing**: Walk through scenarios

**Test Cases**:
- New workspace creation using decision tree → template application
- Existing workspace documentation using migration guide
- Repository migration using bare-repo-guide
- Artifact preservation using session-artifact-tracking

### User Acceptance

**Validation Criteria**:
- Time to find relevant doc < 2 minutes
- Template customization < 10 minutes
- Workspace setup following pattern < 25 minutes
- Pattern understanding < 30 minutes

**Feedback Loops**:
- Lessons learned captured in examples
- Edge cases added to troubleshooting
- Unclear sections revised based on questions
- New patterns added based on validated usage

---

## Version Management

### Versioning Strategy

**Approach**: Semantic versioning at component level

**Version Format**: `MAJOR.MINOR.PATCH`
- **MAJOR**: Incompatible pattern changes (rare)
- **MINOR**: New patterns, significant additions
- **PATCH**: Clarifications, examples, fixes

**Current Versions**:
- Devlog overall: v1.0
- Workspace patterns: v1.0
- Repository patterns: v1.0
- Session artifact tracking: v1.0

### Update Process

**Minor Updates** (clarifications, examples):
1. Update content
2. Update "Last updated" date
3. Add note to relevant CHANGELOG (if exists)
4. No version bump required

**Major Updates** (new patterns):
1. Validate pattern in real usage
2. Create pattern documentation
3. Add examples and templates
4. Update component README
5. Bump MINOR version
6. Update root README

---

## Future Architecture Evolution

### Planned Enhancements

**Phase 2: Methodologies Component**
- Multi-persona review templates
- Validation methodology patterns
- Gap analysis patterns
- Estimated: +1500 lines, 6 files

**Phase 3: Debugging Component**
- Common debugging patterns catalog
- Debug script templates
- Troubleshooting decision trees
- Estimated: +1200 lines, 5 files

**Phase 4: Research Component**
- Research tier classification
- Trusted sources curation
- Synthesis patterns
- Estimated: +1000 lines, 4 files

**Phase 5: Archiving Component**
- Archival criteria framework
- Restoration process
- Batch processing patterns
- Estimated: +800 lines, 3 files

### Architectural Principles Maintained

**As devlog grows**:
- Hub-and-spoke navigation per component
- Documentation only (no code)
- Real examples required
- Pattern validation before inclusion
- Dual audience (AI + human)
- ~100 char line width
- Markdown format

---

## Conclusion

Devlog's architecture prioritizes:
1. **Discoverability**: Hub-and-spoke navigation with progressive disclosure
2. **Usability**: Dual templates for AI agents and humans
3. **Quality**: Real examples, validated patterns, comprehensive guides
4. **Maintainability**: Modular structure, clear ownership, version management
5. **Scalability**: Component-based organization supporting growth

The documentation-only approach keeps devlog focused on "what to do" while enabling tools to automate "how to do it", creating a sustainable knowledge base for AI-assisted development practices.

---

**Document Version**: 1.0
**Last Updated**: 2026-02-11
**Next Review**: 2026-05-11
