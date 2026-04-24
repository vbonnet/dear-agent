# ADR 003: Dual Template System (AGENTS.md + README.md)

**Status**: Accepted
**Date**: 2025-12-13
**Deciders**: Devlog Maintainers
**Context**: Workspace patterns template design

---

## Context

Workspaces need documentation for navigation and identity, but there are two distinct audiences with different needs:

1. **AI Agents** (Claude Code, etc.): Need structured navigation to find content (projects, docs, scripts)
2. **Human Developers**: Need comprehensive documentation about workspace purpose, structure, and usage

The question arose: should workspaces have a single documentation file serving both audiences, or separate files optimized for each?

**Competing Priorities**:
- AI agents need concise, structured navigation
- Humans need comprehensive explanations and context
- Maintenance burden increases with multiple files
- Both audiences are critical for success

---

## Decision

**Workspaces will have dual documentation files: AGENTS.md for AI agents and README.md for humans.**

**AGENTS.md** (AI Agent Navigation):
- **Purpose**: Guide AI agents to find workspace content
- **Structure**: Highly structured, minimal prose
- **Content**: Directory locations, workspace boundaries, navigation aids
- **Format**: Bullet points, tables, clear sections
- **Length**: 70-80 lines (concise)

**README.md** (Human Documentation):
- **Purpose**: Explain workspace identity, purpose, and structure to humans
- **Structure**: Narrative format, comprehensive
- **Content**: Workspace purpose, structure overview, usage guidelines, contributor info
- **Format**: Sections with prose, examples, context
- **Length**: 95-110 lines (comprehensive)

**Both files**:
- Located at workspace root
- Customized from devlog templates
- Maintained as workspace evolves

---

## Rationale

### Different Information Needs

**AI Agents Need**:
- Where are wayfinder projects? (`projects/`)
- Where is documentation? (`docs/`)
- Where are scripts? (`scripts/`)
- What are workspace boundaries?
- Navigation shortcuts

**Humans Need**:
- What is this workspace for?
- Why does it exist?
- How is it organized?
- How do I contribute?
- What are the conventions?

**Attempting to serve both in one file** leads to:
- Too verbose for AI agents (cognitive load)
- Too terse for humans (missing context)
- Unclear primary audience
- Maintenance friction (updating for one audience affects other)

### Optimized Information Density

**AGENTS.md Example**:
```markdown
## Structure

```
workspace/
├── projects/       # Wayfinder projects
├── research/       # Research documents
├── docs/           # Documentation
└── scripts/        # Shared scripts
```

## Wayfinder Projects
Location: `projects/`
```

**Concise**, structured, easy to parse.

**README.md Example**:
```markdown
## What is this workspace?

This workspace contains research and development for the Engram
knowledge management system. It includes multiple related projects,
research investigations, and supporting documentation.

The workspace follows the Mono-Repo pattern, with all projects
sharing a common root and version control.
```

**Narrative**, contextual, human-friendly.

### Precedent in Software Development

**Established Pattern**:
- **README.md**: Universal convention for human documentation
- **Package metadata**: Structured machine-readable info (package.json, Cargo.toml, pyproject.toml)

Dual documentation (human + machine) is proven pattern:
- npm: README.md + package.json
- Rust: README.md + Cargo.toml
- Python: README.md + pyproject.toml

Devlog extends this to workspace navigation:
- README.md (human docs)
- AGENTS.md (AI navigation)

### Clear Separation of Concerns

**AGENTS.md**:
- Maintained for AI agent navigation effectiveness
- Updated when directory structure changes
- Optimized for parsing and discovery

**README.md**:
- Maintained for human understanding
- Updated when purpose or usage evolves
- Optimized for onboarding and collaboration

**No Conflict**: Different update triggers, different optimization goals.

### Avoids Compromise

**Single-File Attempts**:

**Too verbose** (human-optimized):
```markdown
# Workspace

This workspace is organized using the Mono-Repo pattern.
The Mono-Repo pattern is characterized by a single git
repository containing multiple related projects...

[AI agent scrolls past 200 lines of explanation to find: "projects are in projects/"]
```

**Too terse** (AI-optimized):
```markdown
# Workspace

Structure: Mono-Repo
Projects: `projects/`
Research: `research/`
```

[Human wonders: "What is this workspace for? Why mono-repo? What are conventions?"]

**Dual files avoid compromise**: Each optimized for its audience.

---

## Consequences

### Positive

**Optimized Navigation**:
- AI agents find content quickly (AGENTS.md)
- Humans understand context deeply (README.md)
- No compromise on information density

**Clear Purpose**:
- AGENTS.md purpose: Navigation
- README.md purpose: Understanding
- No confusion about primary audience

**Maintenance Clarity**:
- Update AGENTS.md when structure changes
- Update README.md when purpose evolves
- Different triggers, reduced conflict

**Established Convention**:
- README.md universally recognized
- AGENTS.md follows machine-readable precedent
- Both familiar patterns

**Template Reusability**:
- 3 AGENTS.md templates for different patterns
- 3 README.md templates for different patterns
- Clear customization points

### Negative

**Dual Maintenance**:
- Two files to maintain per workspace
- Risk of becoming out of sync
- More effort than single file

**Potential Redundancy**:
- Some information (structure) in both files
- Directory layout may be duplicated
- Update coordination required

**Learning Curve**:
- Users must understand dual-file purpose
- Not obvious why both exist
- Requires documentation (ironic)

### Mitigation Strategies

**For Dual Maintenance**:
- Templates include common content
- Clear distinction of what goes in each
- Regular validation that both are current
- Update checklist for structure changes

**For Redundancy**:
- AGENTS.md: Structure only (what/where)
- README.md: Structure + context (what/where/why)
- Redundancy is intentional (different audience needs)

**For Learning Curve**:
- Devlog migration-guide explains dual-file purpose
- Templates include comments about what goes where
- README.md can reference AGENTS.md and vice versa

---

## Alternatives Considered

### Alternative 1: Single README.md for Both Audiences

**Approach**: One file with sections for both audiences

```markdown
# Workspace

## For AI Agents
[Structured navigation]

## Overview
[Human documentation]
```

**Pros**:
- Single file to maintain
- No synchronization issues
- One source of truth

**Cons**:
- Suboptimal for both audiences
- AI agents parse entire file for navigation
- Humans scroll past AI-specific sections
- Unclear primary audience
- Maintenance updates affect both audiences

**Rejected Because**: Compromise solution serves neither audience well.

### Alternative 2: README.md + .ai-config

**Approach**: README.md for humans, machine-readable config for AI

```markdown
# workspace/README.md
[Human documentation]
```

```yaml
# workspace/.ai-config.yaml
structure:
  projects: projects/
  research: research/
  docs: docs/
```

**Pros**:
- Clear separation
- Machine-readable for AI parsing
- Human-readable documentation separate

**Cons**:
- YAML/JSON not as readable as Markdown
- AI agents already parse Markdown well
- Extra format to maintain
- Less convention precedent

**Rejected Because**: Markdown works well for AI agents, no need for separate format.

### Alternative 3: README.md + Code Comments in Structure

**Approach**: README.md for humans, inline comments for AI

```markdown
# workspace/README.md
[Human documentation]
```

```
workspace/
├── projects/       # AI: Wayfinder projects here
├── research/       # AI: Research documents here
```

**Pros**:
- Single human-facing file
- AI gets navigation from filesystem itself
- No separate navigation file

**Cons**:
- Comments in filesystem are non-standard
- No single navigation reference for AI
- Harder to maintain conventions
- Directory comments not supported by all tools

**Rejected Because**: Non-standard approach, no central navigation reference.

### Alternative 4: Multiple Specialized Files

**Approach**: Separate file for each concern

```
workspace/
├── README.md              # Human overview
├── AGENTS.md              # AI navigation
├── STRUCTURE.md           # Directory structure
├── CONTRIBUTING.md        # Contribution guidelines
└── CONVENTIONS.md         # Workspace conventions
```

**Pros**:
- Highly specialized files
- Clear separation of concerns
- Each file optimized for purpose

**Cons**:
- Too many files
- Fragmentation of information
- Unclear where to find basic info
- High maintenance burden

**Rejected Because**: Over-engineering, too fragmented, maintenance burden.

---

## Implementation Guidelines

### AGENTS.md Template Structure

**Required Sections**:

1. **Identity**: Workspace name and pattern
2. **Structure**: Directory layout with brief descriptions
3. **Wayfinder Projects**: Location of projects
4. **Key Locations**: Where to find docs, scripts, etc.
5. **Boundaries**: What belongs vs. doesn't belong
6. **Navigation**: Quick reference for AI agents

**Format**:
- Bullet points and tables
- Code blocks for directory structure
- Minimal prose (1 sentence per section)
- 70-80 lines total

**Example**:
```markdown
# {{WORKSPACE_NAME}} - AI Agent Navigation

## Pattern
Mono-Repo (single repository, multiple related projects)

## Structure
```
workspace/
├── projects/       # Wayfinder projects
├── research/       # Research documents
├── docs/           # Documentation
```

## Key Locations
- **Wayfinder projects**: `projects/`
- **Documentation**: `docs/`
```

### README.md Template Structure

**Required Sections**:

1. **Identity**: What is this workspace?
2. **Purpose**: Why does it exist?
3. **Structure**: How is it organized?
4. **Usage**: How to work in this workspace?
5. **Contributing**: How to contribute?
6. **Related**: Links to related workspaces/repos

**Format**:
- Prose with headings
- Examples and context
- Narrative explanations
- 95-110 lines total

**Example**:
```markdown
# {{WORKSPACE_NAME}}

## What is this workspace?

This workspace contains [description]. It follows the
Mono-Repo pattern, with multiple related projects sharing
a common repository.

## Purpose

[Why this workspace exists, what problems it solves]

## Structure

```
workspace/
├── projects/       # Wayfinder projects for [purpose]
├── research/       # Research documents for [topic]
├── docs/           # Documentation for [audience]
```

## Usage

[How to work in this workspace, conventions, guidelines]
```

### Customization Process

**Template Customization**:
1. Copy template from devlog
2. Replace `{{PLACEHOLDERS}}` with actual values
3. Add workspace-specific sections if needed
4. Remove inapplicable sections
5. Review for accuracy
6. Deploy to workspace root

**Common Customizations**:
- Add submodule information
- Include build/test commands
- Add CI/CD status badges
- Include team contacts
- Reference external documentation

---

## Related Decisions

**ADR 001**: Documentation-only library (templates, not automation)
**ADR 002**: Hub-and-spoke navigation (README.md as hubs)
**ADR 004**: Real examples required for all patterns

---

## References

**Similar Dual-Documentation Patterns**:
- npm: README.md + package.json
- Rust: README.md + Cargo.toml
- Python: README.md + pyproject.toml
- Maven: README.md + pom.xml

**AI Agent Navigation Examples**:
- GitHub Actions: README.md + workflow YAML
- CI/CD: README.md + pipeline config
- Documentation: README.md + mkdocs.yml

**Workspace Documentation Precedents**:
- Monorepo tools (Nx, Turborepo): Human docs + workspace config
- Development environments: README + IDE config

---

## Review History

**2025-12-13**: Initial decision (workspace patterns templates created)
**2025-12-13**: Validated (3 AGENTS.md + 3 README.md templates created)
**2025-12-19**: Applied to oss/, acme/, acme-app/ workspaces successfully
**2026-02-11**: Documented in ADR (backfill documentation)

**Next Review**: 2026-05-11

**Success Metrics**:
- All 3 workspaces (oss/, acme/, acme-app/) have both files
- AI agent navigation reported as effective
- Human onboarding time reduced
- No reported confusion about dual files
