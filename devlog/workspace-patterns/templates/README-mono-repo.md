# [Workspace Name]

Human-readable documentation for mono-repo pattern workspace.

---

## What is this repository?

[Brief description of workspace purpose - 2-3 sentences explaining what this workspace contains and why it exists]

**Important clarification**: [If there's potential confusion with similarly-named repos, clarify here. Example: "This repository is engram-research (research about engram), NOT the actual engram repository"]

---

## Pattern

This workspace follows the **Mono-Repo pattern**:
- Single git repository containing multiple related projects
- Shared tools, configuration, and resources at root level
- All projects organized in wf/ subdirectory (or documented elsewhere)
- Common workspace root for all content

**Benefits**:
- Easy sharing of tools and configuration
- Consistent development environment
- All related work in one place
- Simple navigation and discovery

---

## Directory Structure

```
[workspace]/
├── README.md                  # This file
├── AGENTS.md                  # AI agent guidance
├── .git/                      # Git repository
├── wf/                        # Wayfinder projects
├── research/                  # Research documents
├── docs/                      # Documentation
├── scripts/                   # Shared scripts and tools
└── [other directories]/
```

**Key directories**:
- **wf/**: All wayfinder projects for this workspace
- **research/**: Research documents and investigations
- **docs/**: Documentation and guides
- **scripts/**: Automation and utility scripts

---

## Workspace Boundaries

**What belongs here**:
- Related projects within same domain/purpose
- Wayfinder projects (in wf/ subdirectory)
- Shared configuration and tools
- Research documents related to workspace projects
- Documentation for workspace content

**What does NOT belong here**:
- Unrelated projects (create separate workspace)
- Confidential work if this is public (use Multi-Workspace pattern)
- Product code if this is research (use Research-vs-Product pattern)

---

## Getting Started

### For AI Agents

1. Read AGENTS.md for workspace-specific guidance
2. Check this README.md for context
3. Navigate to wf/ for wayfinder projects
4. Use workspace root for shared content

### For Developers

**Creating new wayfinder project**:
```bash
# Configure wayfinder to use wf/ directory
wayfinder-new [project-name]

# Project will be created at:
# ~/src/ws/[workspace]/wf/[project-name]/
```

**Adding shared tools/scripts**:
- Place in scripts/ directory at workspace root
- Update README.md if significant addition

**Adding research documents**:
- Place in research/ directory
- Use clear file naming (purpose-date.md)

---

## Related Workspaces

**Other workspaces** (if applicable):
- [List other workspaces and explain relationship]
- Example: ~/src/ws/[REDACTED_EMPLOYER]/ for confidential company work
- Example: ~/src/repos/[product]/ for actual product repository

---

## Related Documentation

**Pattern documentation**:
- Pattern details: ~/src/repos/ai-tools/base/devlog/workspace-patterns/patterns.md#mono-repo
- Real examples: ~/src/repos/ai-tools/base/devlog/workspace-patterns/examples.md#mono-repo
- Decision tree: ~/src/repos/ai-tools/base/devlog/workspace-patterns/decision-tree.md

---

**Workspace**: [workspace-name]
**Pattern**: Mono-Repo
**Created**: [date]
**Last updated**: [date]
