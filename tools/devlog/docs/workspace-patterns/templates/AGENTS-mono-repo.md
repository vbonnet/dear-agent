# Workspace: [WORKSPACE_NAME]

AI agent guidance for mono-repo pattern workspaces.

---

## What is this workspace?

This is a **mono-repo workspace** containing multiple related projects within a single git repository.

**Pattern**: Mono-Repo (single repository, multiple projects)

**Key characteristics**:
- Single .git directory at root
- Multiple related projects in projects/ subdirectory
- Shared tools, configuration, and resources at root level
- All content shares common workspace root

---

## Workspace Structure

**Root**: {{DEVLOG_ROOT}}/ws/[workspace]/
**Projects**: {{DEVLOG_ROOT}}/ws/[workspace]/projects/
**Pattern**: All projects share common workspace root

**Directory organization**:
```
[workspace]/
├── README.md          # Workspace identity
├── AGENTS.md          # This file
├── .git/              # Single git repository
├── projects/                # All wayfinder projects
├── research/          # Research documents
├── docs/              # Documentation
├── scripts/           # Shared scripts
└── [other content]/
```

---

## What belongs here

✅ **Content that belongs in this workspace**:
- Related projects within same domain
- Wayfinder projects (all go in projects/ subdirectory)
- Shared configuration files
- Common tools and scripts
- Research documents related to workspace projects
- Documentation for workspace content

**Examples**:
- Multiple experiments in same research area
- Tools for same product/domain
- Related prototypes and implementations

---

## What does NOT belong here

❌ **Content that should go elsewhere**:
- Unrelated projects (create separate workspace)
- Confidential work mixing with public (use Multi-Workspace pattern)
- Product code if this is research workspace (use Research-vs-Product pattern)
- Nested workspaces (use Sub-Workspace pattern instead)

---

## Wayfinder Projects

**Location**: {{DEVLOG_ROOT}}/ws/[workspace]/projects/

**All wayfinder projects for this workspace live in the projects/ subdirectory.**

**Usage**:
```bash
# Create new wayfinder project (configure to use projects/ directory)
wayfinder-new [project-name]

# Project will be created at:
# {{DEVLOG_ROOT}}/ws/[workspace]/projects/[project-name]/
```

**Organization**:
- Each project in projects/ is a separate wayfinder session
- projects/ subdirectory keeps projects organized
- Shared workspace content stays at root level

---

## Related Workspaces

**Other workspaces** (if applicable):
- [List other workspaces and explain relationship]
- [Example: {{DEVLOG_ROOT}}/ws/acme/ for confidential work]

**Workspace boundaries**:
- This workspace: [workspace-specific content]
- Other workspaces: [their content]

---

## Example Values

Replace these placeholders when using this template:

- `[WORKSPACE_NAME]` → "my-research" (display name)
- `[workspace]` → "my-research" (directory name, lowercase)

**Example result**:
```
Workspace: my-research
Root: {{DEVLOG_ROOT}}/ws/my-research/
Projects: {{DEVLOG_ROOT}}/ws/my-research/projects/
```

---

## Reference

**Pattern documentation**: {{DEVLOG_ROOT}}/repos/ai-tools/main/devlog/workspace-patterns/patterns.md#pattern-1-mono-repo

**More information**:
- Pattern details: [patterns.md](../patterns.md#pattern-1-mono-repo)
- Real examples: [examples.md](../examples.md#example-1-mono-repo-oss)
- Migration guide: [migration-guide.md](../migration-guide.md)

---

**Template**: AGENTS-mono-repo.md
**Pattern**: Mono-Repo
**Last updated**: 2025-12-13
