# Workspace Patterns Documentation

**Purpose**: Prevent workspace boundary confusion and misplaced directories.

**Problem solved**: 17 workspace misplacements identified (4 directories at wrong level, 13 wayfinder projects in wrong locations).

---

## What Are Workspace Patterns?

Workspace patterns are architectural blueprints for organizing multi-project workspaces in {{DEVLOG_ROOT}}/ws/. Each pattern defines:

- **Structure**: Directory layout and organization
- **Boundaries**: What belongs vs doesn't belong
- **Invariants**: Rules that must hold true
- **Navigation**: How AI agents and humans find content

**4 patterns documented**:
1. **Mono-Repo**: Single repo, multiple related projects
2. **Research-vs-Product**: Separate repos for research vs product
3. **Multi-Workspace**: Independent workspaces with clear boundaries
4. **Sub-Workspace**: Workspace nested within parent

---

## Quick Start

### I'm Creating a New Workspace

**Start here**: [decision-tree.md](decision-tree.md)

Answer 4 questions to identify the right pattern, then use provided templates.

**Time**: 5-10 minutes

### I Have an Existing Workspace Without Documentation

**Start here**: [migration-guide.md](migration-guide.md)

Step-by-step guide to add AGENTS.md and README.md without reorganizing files.

**Time**: 15-30 minutes per workspace

### I Want to Understand a Pattern in Detail

**Start here**: [patterns.md](patterns.md)

Comprehensive pattern definitions with structure, boundaries, invariants, examples, and anti-patterns.

**Time**: 5-10 minutes per pattern

### I Want to See Real Examples

**Start here**: [examples.md](examples.md)

Real workspace walkthroughs (oss/, acme/, acme-app/) showing each pattern in practice.

**Time**: 10-15 minutes

---

## Documentation Files

### Core Documentation

#### [patterns.md](patterns.md)
**612 lines** | **Comprehensive reference**

Detailed definitions of all 4 workspace patterns.

**Use when**:
- Need to understand pattern deeply
- Architecting new workspace structure
- Evaluating pattern trade-offs

**Contents**:
- Pattern 1: Mono-Repo (single repo, multiple projects)
- Pattern 2: Research-vs-Product (research vs product separation)
- Pattern 3: Multi-Workspace (independent workspaces)
- Pattern 4: Sub-Workspace (nested workspaces)

Each pattern includes: Structure, Boundaries, Invariants, Examples, Anti-patterns.

#### [examples.md](examples.md)
**581 lines** | **Real-world walkthroughs**

Concrete examples from actual workspaces.

**Use when**:
- Want to see pattern in practice
- Learning by example
- Comparing your workspace to real ones

**Contents**:
- Example 1: Mono-Repo (oss/)
- Example 2: Multi-Workspace (oss/ vs acme/)
- Example 3: Sub-Workspace (acme/acme-app/)
- Example 4: Research-vs-Product (oss/ = engram-research)

Each example includes: Structure breakdown, Pattern application, Lessons learned, Anti-patterns avoided.

#### [decision-tree.md](decision-tree.md)
**223 lines** | **Pattern selection flowchart**

Interactive guide to choosing the right pattern.

**Use when**:
- Creating new workspace
- Unsure which pattern to use
- Need quick decision framework

**Contents**:
- 4 decision questions (confidentiality, nesting, research, projects)
- Mermaid flowchart visualization
- Pattern selection matrix
- Edge case handling

#### [migration-guide.md](migration-guide.md)
**477 lines** | **Existing workspace guide**

Step-by-step guide for documenting existing workspaces.

**Use when**:
- Workspace exists but lacks documentation
- Need to add AGENTS.md and README.md
- Want to clarify workspace identity

**Contents**:
- Step 1: Identify current pattern (audit checklist)
- Step 2: Choose template
- Step 3: Create AGENTS.md
- Step 4: Create/Update README.md
- Step 5: Validate documentation
- Common scenarios (oss/, acme/, acme-app/)
- Troubleshooting

### Templates

All templates in [templates/](templates/) directory.

#### AI Agent Guidance Templates (AGENTS.md)

| Pattern | Template | Lines |
|---------|----------|-------|
| Mono-Repo | [AGENTS-mono-repo.md](templates/AGENTS-mono-repo.md) | 73 |
| Multi-Workspace | [AGENTS-multi-workspace.md](templates/AGENTS-multi-workspace.md) | 78 |
| Sub-Workspace | [AGENTS-sub-workspace.md](templates/AGENTS-sub-workspace.md) | 75 |

**Research-vs-Product**: No template (clarify in README.md manually)

#### Human-Readable Documentation Templates (README.md)

| Pattern | Template | Lines |
|---------|----------|-------|
| Mono-Repo | [README-mono-repo.md](templates/README-mono-repo.md) | 95 |
| Multi-Workspace | [README-multi-workspace.md](templates/README-multi-workspace.md) | 107 |
| Sub-Workspace | [README-sub-workspace.md](templates/README-sub-workspace.md) | 98 |

**Research-vs-Product**: No template (use Mono-Repo or Multi-Workspace as base)

---

## Pattern Quick Reference

### When to Use Each Pattern

| Pattern | Use When | Don't Use When |
|---------|----------|----------------|
| **Mono-Repo** | Multiple related projects, shared tools | Confidentiality boundaries exist |
| **Research-vs-Product** | Research ABOUT product, not product itself | Working on actual product code |
| **Multi-Workspace** | Confidentiality/team/security boundaries | No clear separation reason |
| **Sub-Workspace** | Logical nesting (product in company, team in org) | No parent relationship |

### Pattern Selection Priority

If workspace matches multiple patterns:

1. **Confidentiality boundaries** → Multi-Workspace (overrides all)
2. **Nested structure** → Sub-Workspace (second priority)
3. **Research focus** → Research-vs-Product (distinguishes from product)
4. **Default** → Mono-Repo (multiple related projects)

### Common Confusion Points

**Q: Is oss/ Mono-Repo or Research-vs-Product?**

A: **Both**. oss/ is Mono-Repo (structure) and demonstrates Research-vs-Product (relationship to engram product). Document both aspects:
- Pattern: Mono-Repo (single repo, multiple projects)
- Clarification: This is engram-research, NOT engram product ({{DEVLOG_ROOT}}/repos/engram/base/)

**Q: Is acme/acme-app/ Sub-Workspace or Multi-Workspace?**

A: **Sub-Workspace**. It's nested within acme/. The fact that acme/ is Multi-Workspace (separate from oss/ due to confidentiality) doesn't change acme-app/'s pattern.

**Q: Do I need templates for Research-vs-Product?**

A: **No**. Research-vs-Product is a meta-pattern (explains relationship between two repos). The research workspace itself uses Mono-Repo or Multi-Workspace pattern. Clarify the research/product relationship manually in README.md.

**Q: Can workspace follow multiple patterns?**

A: **Yes, in different dimensions**:
- Structure pattern (Mono-Repo, Sub-Workspace)
- Relationship pattern (Research-vs-Product)
- Boundary pattern (Multi-Workspace)

Document all applicable patterns in workspace documentation.

---

## When to Use This Guide

### Use this guide when:

✅ **Creating new workspace** → decision-tree.md
✅ **Documenting existing workspace** → migration-guide.md
✅ **Unclear what belongs in workspace** → patterns.md
✅ **Confused by directory placement** → examples.md
✅ **AI agent can't find content** → Add AGENTS.md via migration-guide.md
✅ **Multiple workspaces with unclear boundaries** → Multi-Workspace pattern
✅ **Wayfinder projects scattered** → Document projects/ location in AGENTS.md

### Don't use this guide when:

❌ **Single project only** → Workspace patterns apply to multi-project workspaces
❌ **Working in {{DEVLOG_ROOT}}/repos/** → Repo patterns (not workspace patterns)
❌ **File reorganization needed** → migration-guide.md documents first, reorganize later
❌ **Workspace is temporary** → No need for documentation

---

## Navigation Flowchart

```
START
  ↓
Do you have a workspace?
  ↓                    ↓
 YES                  NO
  ↓                    ↓
  ↓              decision-tree.md
  ↓              (create new)
  ↓
Does it have AGENTS.md/README.md?
  ↓                    ↓
 YES                  NO
  ↓                    ↓
  ↓              migration-guide.md
  ↓              (document existing)
  ↓
Want to understand pattern deeply?
  ↓                    ↓
 YES                  NO
  ↓                    ↓
patterns.md       examples.md
(detailed)        (real examples)
```

---

## File Relationships

```
README.md (you are here)
├── decision-tree.md ────→ For new workspaces
│   └── Leads to templates/
│
├── migration-guide.md ──→ For existing workspaces
│   └── Leads to templates/
│
├── patterns.md ─────────→ Pattern definitions
│   └── Referenced by examples.md
│
├── examples.md ─────────→ Real-world walkthroughs
│   └── References patterns.md
│
└── templates/ ──────────→ AGENTS.md and README.md templates
    ├── AGENTS-mono-repo.md
    ├── AGENTS-multi-workspace.md
    ├── AGENTS-sub-workspace.md
    ├── README-mono-repo.md
    ├── README-multi-workspace.md
    └── README-sub-workspace.md
```

---

## Contributing

### Adding New Patterns

If you discover a new workspace pattern:

1. Document structure, boundaries, invariants
2. Add to patterns.md
3. Create real example in examples.md
4. Add to decision-tree.md flowchart
5. Create templates (AGENTS.md and README.md)
6. Update this README.md

### Improving Existing Patterns

If pattern definitions need clarification:

1. Update patterns.md with clarification
2. Add edge case example to examples.md
3. Update migration-guide.md troubleshooting if needed

### Reporting Issues

File beads for:
- Pattern confusion (unclear definitions)
- Missing scenarios (templates don't fit)
- Documentation gaps (questions not answered)
- Reorganization needs (files in wrong place)

---

## Project Context

**Created**: 2025-12-13
**Wayfinder project**: {{DEVLOG_ROOT}}/ws/oss/projects/workspace-patterns-documentation/
**Bead source**: Bead 3 (filed 2025-12-11)
**Problem**: 17 workspace misplacements identified in oss-workspace-audit

**Deliverables**:
- 4 core documentation files (patterns, examples, decision-tree, migration-guide)
- 6 templates (3 AGENTS.md, 3 README.md)
- This README.md (navigation hub)

**Total**: 13 files, ~3300 lines of documentation

---

**Last updated**: 2025-12-13
**Version**: 1.0
**Maintainer**: See wayfinder project S11 retrospective
