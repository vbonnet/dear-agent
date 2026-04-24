# Devlog Specification

## Overview

**Name**: Devlog
**Type**: Knowledge Base / Documentation Library
**Purpose**: Capture and share AI-assisted development best practices, patterns, and templates
**Location**: `{{DEVLOG_ROOT}}/repos/ai-tools/main/devlog/`
**Version**: 1.0
**Last Updated**: 2026-02-11

---

## Problem Statement

AI-assisted development teams lose valuable learnings when:
- Session artifacts remain in `/tmp` and are lost after sessions end
- Successful patterns are not documented for reuse
- Workspace organization lacks clear boundaries, leading to misplaced content
- Repository structures don't support multi-branch workflows
- No centralized location exists for development best practices

**Impact**: Teams repeatedly solve the same problems, misplace content, and can't compound their learnings over time.

---

## Solution

Devlog provides a centralized knowledge base that:

1. **Documents Patterns**: Captures proven patterns for workspace and repository organization
2. **Provides Templates**: Offers reusable templates for common development scenarios
3. **Shares Best Practices**: Guides for session artifact tracking, debugging, research, and more
4. **Enables Discovery**: Organizes knowledge for easy navigation by humans and AI agents
5. **Supports Learning**: Allows teams to compound knowledge across sessions

---

## Scope

### In Scope

**Current (v1.0)**:
- Workspace organization patterns (Mono-Repo, Multi-Workspace, Sub-Workspace, Research-vs-Product)
- Repository structure patterns (Bare repo + worktrees)
- Session artifact tracking guidelines
- AGENTS.md and README.md templates for workspace documentation
- Migration guides for existing workspaces and repositories
- Real-world examples from production usage

**Planned (Future)**:
- Multi-persona review templates
- Validation methodology patterns
- Gap analysis patterns
- Common debugging patterns
- Debug script templates
- Research tier classification
- Archival criteria and batch processing

### Out of Scope

- Implementation code (covered by specific tools like `agm`, `engram`)
- Project-specific documentation (belongs in individual project repos)
- Automated tooling (separate from documentation)
- Session management functionality (covered by `agm`)
- Agent knowledge management (covered by `engram`)

---

## Core Components

### 1. Workspace Patterns

**Purpose**: Prevent workspace boundary confusion and misplaced directories

**Contents**:
- 4 documented patterns: Mono-Repo, Research-vs-Product, Multi-Workspace, Sub-Workspace
- Decision tree for pattern selection
- Migration guide for existing workspaces
- 6 templates (3 AGENTS.md, 3 README.md)
- Real-world examples from oss/, acme/, acme-app/ workspaces

**Files**: 13 files, ~3300 lines
- `workspace-patterns/README.md` - Navigation hub
- `workspace-patterns/patterns.md` - Pattern definitions
- `workspace-patterns/examples.md` - Real-world walkthroughs
- `workspace-patterns/decision-tree.md` - Pattern selection guide
- `workspace-patterns/migration-guide.md` - Document existing workspaces
- `workspace-patterns/templates/` - AGENTS.md and README.md templates

**Problem Solved**: 17 workspace misplacements identified in oss-workspace-audit

### 2. Repository Patterns

**Purpose**: Guide multi-branch workflow organization using bare repositories and worktrees

**Contents**:
- Bare repository + worktrees pattern (recommended default)
- Complete setup guide for new and existing repositories
- 9 real migration examples
- Integration with git-worktrees plugin for temporary isolation

**Files**: 3 files, ~800 lines
- `repo-patterns/README.md` - Navigation hub
- `repo-patterns/bare-repo-guide.md` - Comprehensive pattern guide
- `repo-patterns/examples.md` - Real migration examples

**Problem Solved**: Branch switching disrupts builds, requires stashing, loses context

### 3. Session Artifact Tracking

**Purpose**: Preserve valuable session artifacts instead of losing them in `/tmp`

**Contents**:
- Artifact categorization (retrospectives, metrics, tools, closures, snapshots)
- Save location patterns
- Session end protocol for AI assistants
- Implementation roadmap

**Files**: 1 file, ~124 lines
- `session-artifact-tracking.md` - Complete tracking guide

**Problem Solved**: Valuable learnings, metrics, and scripts lost when `/tmp` is cleared

---

## Target Users

### Primary Users

**AI Agents (Claude Code, etc.)**:
- Navigate workspace structures using AGENTS.md
- Follow documented patterns when creating content
- Preserve session artifacts in correct locations
- Apply templates for common scenarios

**Developers Using AI-Assisted Development**:
- Organize workspaces with clear boundaries
- Structure repositories for multi-branch workflows
- Document workspace identity for team collaboration
- Preserve learnings across sessions

### Secondary Users

**Team Leads**:
- Establish workspace organization standards
- Review and approve workspace patterns
- Guide team on best practices

**DevOps/Infrastructure**:
- Understand workspace structure for tooling integration
- Implement artifact archival automation

---

## Key Features

### 1. Pattern-Based Organization

**Capability**: Multiple documented patterns for different scenarios
**Value**: Choose right pattern for specific needs rather than one-size-fits-all

**Patterns**:
- **Mono-Repo**: Single repo, multiple related projects
- **Multi-Workspace**: Independent workspaces with confidentiality boundaries
- **Sub-Workspace**: Workspace nested within parent
- **Research-vs-Product**: Separate research from product code
- **Bare Repo + Worktrees**: Multi-branch workflow support

### 2. Templates and Examples

**Capability**: Reusable templates with real-world examples
**Value**: Faster workspace setup, consistent documentation

**Templates**:
- AGENTS.md for AI agent navigation (3 variants)
- README.md for human documentation (3 variants)
- Customizable for specific workspace needs

**Examples**:
- 3 workspace pattern examples (oss/, acme/, acme-app/)
- 9 repository migration examples
- Before/after comparisons with lessons learned

### 3. Decision Support

**Capability**: Guided decision-making for pattern selection
**Value**: Reduce uncertainty, avoid common mistakes

**Guides**:
- Decision tree with 4 key questions
- Pattern selection matrix
- Edge case handling
- Anti-patterns to avoid

### 4. Migration Support

**Capability**: Document existing workspaces without reorganization
**Value**: Low-friction adoption, preserve existing structure

**Approach**:
- Identify current pattern via audit checklist
- Add documentation without moving files
- Validate against pattern invariants
- Troubleshooting for common scenarios

### 5. Artifact Preservation

**Capability**: Systematic approach to preserving valuable session artifacts
**Value**: Compound learning, track quality trends, tool discovery

**Coverage**:
- 7 artifact types with save patterns
- Session end protocol
- Integration with existing project structures

---

## User Workflows

### Workflow 1: Create New Workspace

**Actor**: Developer setting up new workspace

**Steps**:
1. Read `workspace-patterns/decision-tree.md`
2. Answer 4 decision questions
3. Identify recommended pattern
4. Choose appropriate AGENTS.md template from `workspace-patterns/templates/`
5. Choose appropriate README.md template from `workspace-patterns/templates/`
6. Customize templates for specific workspace
7. Create workspace structure following pattern

**Duration**: 5-10 minutes for decision, 10-15 minutes for setup

### Workflow 2: Document Existing Workspace

**Actor**: Developer with undocumented workspace

**Steps**:
1. Read `workspace-patterns/migration-guide.md`
2. Audit current workspace structure using checklist
3. Identify matching pattern
4. Create AGENTS.md using appropriate template
5. Create/update README.md using appropriate template
6. Validate documentation against pattern invariants
7. Commit documentation to workspace

**Duration**: 15-30 minutes per workspace

### Workflow 3: Migrate Repository to Bare Repo Pattern

**Actor**: Developer wanting multi-branch workflow

**Steps**:
1. Read `repo-patterns/bare-repo-guide.md`
2. Review migration examples in `repo-patterns/examples.md`
3. Back up existing repository
4. Follow migration steps from guide
5. Create initial worktrees for active branches
6. Update development workflow to use worktrees
7. Validate migration

**Duration**: 20-30 minutes for migration, immediate workflow benefits

### Workflow 4: Preserve Session Artifacts

**Actor**: AI assistant at end of session

**Steps**:
1. Scan `/tmp` for valuable artifacts
2. Match artifacts against patterns in `session-artifact-tracking.md`
3. Categorize found artifacts by type
4. Prompt user with suggested destinations
5. Execute moves with user confirmation
6. Update session notes with artifact locations

**Duration**: 2-5 minutes at session end

### Workflow 5: Learn Pattern in Detail

**Actor**: Developer or AI agent needing deep understanding

**Steps**:
1. Navigate to relevant README.md (workspace or repo patterns)
2. Read pattern definition in `patterns.md` or `bare-repo-guide.md`
3. Review real examples in `examples.md`
4. Study anti-patterns and edge cases
5. Apply pattern to specific situation

**Duration**: 15-30 minutes for deep understanding

---

## Quality Attributes

### Usability

**Requirement**: Easy navigation for humans and AI agents
**Measure**: Time to find relevant documentation < 2 minutes
**Implementation**:
- Clear README.md navigation hubs
- Cross-references between related docs
- Decision trees for guidance
- Quick reference tables

### Maintainability

**Requirement**: Easy to update as patterns evolve
**Measure**: Add new pattern in < 1 hour
**Implementation**:
- Modular file structure
- Consistent documentation format
- Clear contributing guidelines
- Template-based approach

### Completeness

**Requirement**: Cover common scenarios comprehensively
**Measure**: < 10% of users need undocumented pattern
**Implementation**:
- 4 workspace patterns covering main scenarios
- Migration paths for existing setups
- Edge case documentation
- Troubleshooting sections

### Accuracy

**Requirement**: Documentation reflects real usage
**Measure**: Based on actual migrations and usage
**Implementation**:
- Examples from 9 real repository migrations
- Workspace examples from production usage
- Lessons learned from actual problems
- Validated against 17 identified misplacements

### Discoverability

**Requirement**: Users can find relevant content quickly
**Measure**: Navigation flowcharts guide to right doc
**Implementation**:
- Navigation flowcharts in each README
- Pattern quick reference tables
- Decision trees for selection
- Clear file relationships

---

## Technical Requirements

### File Organization

**Structure**:
```
devlog/
├── README.md                       # Root navigation hub
├── SPEC.md                         # This specification
├── ARCHITECTURE.md                 # System architecture
├── .docs/adr/                      # Architecture decision records
├── session-artifact-tracking.md   # Artifact preservation guide
├── workspace-patterns/             # Workspace organization patterns
│   ├── README.md                   # Navigation hub
│   ├── patterns.md                 # Pattern definitions
│   ├── examples.md                 # Real examples
│   ├── decision-tree.md            # Pattern selection
│   ├── migration-guide.md          # Existing workspace guide
│   └── templates/                  # AGENTS.md and README.md templates
└── repo-patterns/                  # Repository structure patterns
    ├── README.md                   # Navigation hub
    ├── bare-repo-guide.md          # Comprehensive guide
    └── examples.md                 # Migration examples
```

### File Format

- **Markdown**: All documentation in Markdown format
- **Line Width**: ~100 characters for terminal readability
- **Headings**: Clear hierarchy with `#`, `##`, `###`
- **Code Blocks**: Fenced code blocks with language identifiers
- **Links**: Relative links between files in same documentation set

### Content Standards

- **Examples**: Real examples from production usage, not hypothetical
- **Templates**: Customizable with `{{PLACEHOLDER}}` markers
- **Cross-references**: Links to related documentation
- **Version info**: Last updated date in each major file
- **Metadata**: Location, version, maintainer info where applicable

### Accessibility

- **AI Agent Readable**: Clear structure for AI parsing
- **Human Readable**: Scannable with headers, tables, flowcharts
- **Terminal Friendly**: ~100 char width, no wide tables
- **Clickable Paths**: Absolute paths using `~` notation

---

## Success Metrics

### Adoption

- **Target**: 80% of new workspaces use documented patterns
- **Measure**: Presence of AGENTS.md following templates
- **Timeline**: 3 months after v1.0 release

### Problem Reduction

- **Target**: < 5 workspace misplacements per year
- **Measure**: Audit findings in annual workspace reviews
- **Baseline**: 17 misplacements identified in 2025-12 audit

### Knowledge Reuse

- **Target**: 50% reduction in time to set up new workspace
- **Measure**: Time from decision to working workspace
- **Baseline**: ~45 minutes (estimated pre-devlog)
- **Goal**: < 25 minutes with templates

### Artifact Preservation

- **Target**: 80% of valuable artifacts preserved
- **Measure**: Retrospective review of session outputs
- **Timeline**: 6 months after artifact tracking adoption

### Pattern Coverage

- **Target**: < 10% of workspaces require undocumented pattern
- **Measure**: Percentage of workspaces using documented patterns
- **Current**: 100% coverage for oss/, acme/, acme-app/ workspaces

---

## Integration Points

### Related Tools

**agm**:
- **Relationship**: Devlog provides patterns, session-manager provides tooling
- **Integration**: Session-manager could reference devlog for workspace setup
- **Future**: Automate artifact archival using session-artifact-tracking.md

**engram**:
- **Relationship**: Devlog for general practices, engram for specific product
- **Integration**: Engram documentation references devlog patterns
- **Future**: Engram plugins could validate workspace patterns

**git-worktrees plugin**:
- **Relationship**: Temporary worktrees vs. permanent bare repo pattern
- **Integration**: Both can coexist - bare repo as base, plugin for temporary isolation
- **Documentation**: Clear distinction in repo-patterns/README.md

### Workspace Integration

**AGENTS.md**:
- Located at workspace root
- Guides AI agents on workspace structure
- Points to projects/, research/, docs/ locations
- Generated from devlog templates

**README.md**:
- Located at workspace root
- Human-readable workspace documentation
- Clarifies workspace identity and purpose
- Generated from devlog templates

---

## Constraints and Assumptions

### Constraints

**Documentation Only**:
- Devlog contains no executable code
- Implementation belongs in separate tools
- Migration scripts not included in devlog library

**Pattern Stability**:
- Patterns must be validated before documentation
- Real usage required before pattern inclusion
- No hypothetical or untested patterns

**File Size**:
- Individual files < 1000 lines preferred
- Break large content into focused documents
- Maintain terminal readability

### Assumptions

**User Environment**:
- Users work in `{{DEVLOG_ROOT}}/` directory structure
- Git available for repository management
- Terminal-based workflow with AI assistance

**AI Agent Capabilities**:
- Can read and parse Markdown documentation
- Can navigate file structures using AGENTS.md
- Can apply templates with placeholder substitution

**Workspace Structure**:
- `{{DEVLOG_ROOT}}/ws/` for workspaces
- `{{DEVLOG_ROOT}}/repos/` for repositories
- Standard directory naming conventions

---

## Future Directions

### Planned Additions

**Methodologies** (Phase 2):
- Multi-persona review templates
- Three-tier validation patterns (manual + automated + review)
- Gap analysis patterns for identifying missing content

**Debugging** (Phase 3):
- Common debugging pattern catalog
- Debug script templates with usage examples
- Troubleshooting decision trees

**Research** (Phase 4):
- Research tier classification (TIER1/TIER2/TIER3)
- Trusted sources curation
- Synthesis patterns for multi-source findings

**Archiving** (Phase 5):
- Archival criteria decision framework
- Restoration criteria and process
- Batch processing patterns for engrams

### Extraction Strategy

**Approach**: Extract patterns from successful sessions after workspace-patterns is complete
**Process**:
1. Identify successful pattern usage in real sessions
2. Document pattern with examples
3. Create templates if applicable
4. Validate across multiple use cases
5. Add to devlog with version update

**Timeline**: New patterns added quarterly based on validated usage

---

## Version History

**v1.0** (2026-02-11):
- Initial specification created as backfill documentation
- Workspace patterns fully documented (4 patterns, 6 templates)
- Repository patterns documented (bare repo + worktrees)
- Session artifact tracking guidelines published

**Pre-v1.0** (2025-12):
- Workspace patterns created (2025-12-13)
- Repository patterns created (2025-12-19)
- Session artifact tracking documented
- 9 successful repository migrations completed

---

## References

### Internal Documentation

- Workspace Patterns: `workspace-patterns/README.md`
- Repository Patterns: `repo-patterns/README.md`
- Session Artifact Tracking: `session-artifact-tracking.md`

### Source Projects

- Workspace patterns project: `{{DEVLOG_ROOT}}/ws/oss/projects/workspace-patterns-documentation/`
- Bare repo docs project: `{{DEVLOG_ROOT}}/ws/oss/projects/devlog-bare-repo-docs/`
- OSS workspace audit: Identified 17 misplacements (2025-12)

### Related Tools

- agm: Session management CLI
- engram: AI agent knowledge management system
- git-worktrees plugin: Temporary worktree isolation

---

**Document Owner**: Devlog Maintainers
**Review Cycle**: Quarterly or when new patterns added
**Next Review**: 2026-05-11
