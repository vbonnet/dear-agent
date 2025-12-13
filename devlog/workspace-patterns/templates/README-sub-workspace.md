# [Sub-Workspace Name]

Human-readable documentation for sub-workspace pattern.

---

## What is this repository?

[Brief description of sub-workspace purpose - 2-3 sentences]

**Parent workspace**: ~/src/ws/[parent]/

**Relationship**: [Explain why this is nested under parent]
- Example: Product-specific area within company workspace
- Example: Team content within organization workspace

---

## Pattern

This workspace follows the **Sub-Workspace pattern**:
- Nested within parent workspace
- Documented in parent README.md
- Logical relationship to parent
- May have own tracking policy

**Why nested?**
- [Primary reason for nesting]
- [Relationship to parent workspace]

---

## Directory Structure

```
[parent]/                      # Parent workspace
├── README.md                  # Documents this sub-workspace
├── .gitignore                 # May exclude this sub-workspace
├── [parent content]/
└── [sub-workspace]/           # This sub-workspace
    ├── README.md              # This file (when sub-workspace grows)
    ├── [sub-workspace content]/
    └── [projects]/
```

**Integration with parent**:
- Parent README.md documents this sub-workspace
- Parent .gitignore may exclude this directory
- [Other integration points]

---

## Workspace Boundaries

**What belongs here**:
- Content specific to [sub-workspace purpose]
- Projects for [sub-workspace area]
- [Sub-workspace-specific examples]

**What does NOT belong here**:
- Parent workspace content → ~/src/ws/[parent]/
- Other sub-workspaces' content → ~/src/ws/[parent]/[other-sub]/
- Unrelated content

---

## Parent Workspace Integration

**Parent workspace**: ~/src/ws/[parent]/

**Documented in parent README.md**:
```markdown
## Directory Structure

- [sub-workspace]/ - [Purpose] (sub-workspace, [tracking policy])
```

**Parent .gitignore** (if sub-workspace not tracked):
```
# Sub-workspace
[sub-workspace]/
```

**When to check parent workspace**:
- Understanding overall structure
- Finding related parent content
- Security policies (if applicable)

---

## Wayfinder Projects

**Options for wayfinder projects**:

1. **Own wf/ directory**: ~/src/ws/[parent]/[sub-workspace]/wf/ (if many projects)
2. **Parent wf/ directory**: ~/src/ws/[parent]/wf/ (if few projects)

**Current approach**: [Document which option is used]

**Recommendation**: Start with parent wf/, create own wf/ when sub-workspace has 5+ projects.

---

## Getting Started

### For AI Agents

1. Read parent ~/src/ws/[parent]/AGENTS.md for overall context
2. Read ~/src/ws/[parent]/README.md for sub-workspace documentation
3. Read this README.md for sub-workspace-specific guidance
4. Understand integration with parent workspace

### For Developers

**Creating new wayfinder project**:
```bash
# Option 1: Parent wf/ directory
wayfinder-new [project-name]
# Creates: ~/src/ws/[parent]/wf/[project-name]/

# Option 2: Sub-workspace wf/ directory (if using own wf/)
wayfinder-new [project-name]
# Creates: ~/src/ws/[parent]/[sub-workspace]/wf/[project-name]/
```

**Working with parent workspace**:
- Parent content: ~/src/ws/[parent]/
- This sub-workspace: ~/src/ws/[parent]/[sub-workspace]/
- Keep purposes separate

---

## Growth Path

**Small sub-workspace** (current state if just starting):
- No sub-workspace README.md needed yet
- Parent README.md provides context
- Parent AGENTS.md provides guidance

**Growing sub-workspace** (when to add docs):
- Add this README.md when 5+ projects or significant content
- Add AGENTS.md for AI agent guidance
- Consider own wf/ directory if many projects

---

## Related Documentation

**Pattern documentation**:
- Pattern details: ~/src/repos/ai-tools/base/devlog/workspace-patterns/patterns.md#sub-workspace
- Real examples: ~/src/repos/ai-tools/base/devlog/workspace-patterns/examples.md#sub-workspace
- Decision tree: ~/src/repos/ai-tools/base/devlog/workspace-patterns/decision-tree.md

**Parent workspace**: ~/src/ws/[parent]/README.md

---

**Sub-Workspace**: [sub-workspace-name]
**Parent**: [parent-workspace-name]
**Pattern**: Sub-Workspace
**Created**: [date]
**Last updated**: [date]
