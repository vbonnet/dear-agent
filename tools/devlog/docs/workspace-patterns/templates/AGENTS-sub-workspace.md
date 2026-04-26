# Workspace: [WORKSPACE_NAME]

AI agent guidance for sub-workspace pattern.

---

## What is this workspace?

This is a **sub-workspace** nested within a parent workspace, with distinct purpose but logical relationship to parent.

**Pattern**: Sub-Workspace (nested under parent workspace)

**Parent workspace**: {{DEVLOG_ROOT}}/ws/[parent]/

**Key characteristics**:
- Nested under parent workspace
- Documented in parent README.md
- May have own tracking policy
- Logical relationship to parent (product within company, team within org)

---

## Workspace Structure

**Root**: {{DEVLOG_ROOT}}/ws/[parent]/[workspace]/
**Parent**: {{DEVLOG_ROOT}}/ws/[parent]/
**Pattern**: Nested sub-workspace with parent integration

**Directory organization**:
```
[parent]/                      # Parent workspace
├── README.md                  # Documents this sub-workspace
├── .gitignore                 # May exclude sub-workspace
├── [parent content]/
└── [workspace]/               # This sub-workspace
    ├── [sub-workspace content]/
    └── [may have own README.md when it grows]
```

---

## What belongs here

✅ **Content that belongs in this sub-workspace**:
- Content specific to this product/area
- Projects for this sub-workspace
- [Sub-workspace-specific content]

**Purpose**: [Explain sub-workspace purpose]
- Example: Product development within company workspace
- Example: Team-specific content within organization workspace

---

## What does NOT belong here

❌ **Content that should go elsewhere**:
- Parent workspace content (goes in parent root)
- Other sub-workspaces' content
- Content unrelated to this sub-workspace's purpose

**Parent workspace content**: {{DEVLOG_ROOT}}/ws/[parent]/

---

## Wayfinder Projects

**Location**: {{DEVLOG_ROOT}}/ws/[parent]/[workspace]/projects/ OR {{DEVLOG_ROOT}}/ws/[parent]/projects/

**Options**:
1. **Own projects/ directory**: {{DEVLOG_ROOT}}/ws/[parent]/[workspace]/projects/ (if many projects)
2. **Parent projects/ directory**: {{DEVLOG_ROOT}}/ws/[parent]/projects/ (if few projects)

**Recommendation**: Start with parent projects/, add own projects/ when it grows.

---

## Parent Integration

**Parent workspace documents this sub-workspace**:

**Parent README.md** should include:
- Sub-workspace purpose and location
- .gitignore policy (if sub-workspace not tracked)
- Integration with parent

**Example** (from parent README.md):
```markdown
## Directory Structure

- [workspace]/ - [Purpose] (sub-workspace, [tracking policy])
```

**Parent .gitignore** (if sub-workspace not tracked):
```
# Sub-workspace
[workspace]/
```

---

## When to Add Sub-Workspace Documentation

**Small sub-workspace** (current state):
- No [workspace]/README.md needed yet
- Parent README.md provides context
- Parent AGENTS.md or this file provides guidance

**Growing sub-workspace**:
- Add [workspace]/README.md when sub-workspace has 5+ projects or significant content
- Add [workspace]/AGENTS.md for AI agent guidance
- Keep parent README.md updated with sub-workspace status

---

## Example Values

Replace these placeholders when using this template:

- `[WORKSPACE_NAME]` → "acme-app" (sub-workspace display name)
- `[workspace]` → "acme-app" (sub-workspace directory name)
- `[parent]` → "acme" (parent workspace name)

**Example result**:
```
Workspace: acme-app
Root: {{DEVLOG_ROOT}}/ws/acme/acme-app/
Parent: {{DEVLOG_ROOT}}/ws/acme/
```

---

## Reference

**Pattern documentation**: {{DEVLOG_ROOT}}/repos/ai-tools/main/devlog/workspace-patterns/patterns.md#pattern-4-sub-workspace

**More information**:
- Pattern details: [patterns.md](../patterns.md#pattern-4-sub-workspace)
- Real examples: [examples.md](../examples.md#example-3-sub-workspace-acmeacme-app)
- Migration guide: [migration-guide.md](../migration-guide.md)

---

**Template**: AGENTS-sub-workspace.md
**Pattern**: Sub-Workspace
**Last updated**: 2025-12-13
