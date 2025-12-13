# Workspace: [WORKSPACE_NAME]

AI agent guidance for mono-repo pattern workspaces.

---

## What is this workspace?

This is a **mono-repo workspace** containing multiple related projects within a single git repository.

**Pattern**: Mono-Repo (single repository, multiple projects)

**Key characteristics**:
- Single .git directory at root
- Multiple related projects in wf/ subdirectory
- Shared tools, configuration, and resources at root level
- All content shares common workspace root

---

## Workspace Structure

**Root**: ~/src/ws/[workspace]/
**Projects**: ~/src/ws/[workspace]/wf/
**Pattern**: All projects share common workspace root

**Directory organization**:
```
[workspace]/
├── README.md          # Workspace identity
├── AGENTS.md          # This file
├── .git/              # Single git repository
├── wf/                # All wayfinder projects
├── research/          # Research documents
├── docs/              # Documentation
├── scripts/           # Shared scripts
└── [other content]/
```

---

## What belongs here

✅ **Content that belongs in this workspace**:
- Related projects within same domain
- Wayfinder projects (all go in wf/ subdirectory)
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

**Location**: ~/src/ws/[workspace]/wf/

**All wayfinder projects for this workspace live in the wf/ subdirectory.**

**Usage**:
```bash
# Create new wayfinder project (configure to use wf/ directory)
wayfinder-new [project-name]

# Project will be created at:
# ~/src/ws/[workspace]/wf/[project-name]/
```

**Organization**:
- Each project in wf/ is a separate wayfinder session
- wf/ subdirectory keeps projects organized
- Shared workspace content stays at root level

---

## Related Workspaces

**Other workspaces** (if applicable):
- [List other workspaces and explain relationship]
- [Example: ~/src/ws/[REDACTED_EMPLOYER]/ for confidential work]

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
Root: ~/src/ws/my-research/
Projects: ~/src/ws/my-research/wf/
```

---

## Reference

**Pattern documentation**: ~/src/repos/ai-tools/base/devlog/workspace-patterns/patterns.md#pattern-1-mono-repo

**More information**:
- Pattern details: [patterns.md](../patterns.md#pattern-1-mono-repo)
- Real examples: [examples.md](../examples.md#example-1-mono-repo-oss)
- Migration guide: [migration-guide.md](../migration-guide.md)

---

**Template**: AGENTS-mono-repo.md
**Pattern**: Mono-Repo
**Last updated**: 2025-12-13
