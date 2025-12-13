# Workspace: [WORKSPACE_NAME]

AI agent guidance for multi-workspace pattern.

---

## What is this workspace?

This is one of **multiple independent workspaces** with clear boundaries (typically confidentiality, team, or security boundaries).

**Pattern**: Multi-Workspace (independent workspace with clear boundaries)

**Key characteristics**:
- Separate git repository from other workspaces
- Minimal cross-references to other workspaces
- Clear boundary reason (confidentiality, team, security)
- Independent lifecycle and purpose

---

## Workspace Structure

**Root**: ~/src/ws/[workspace]/
**Projects**: ~/src/ws/[workspace]/wf/
**Pattern**: Independent workspace, separate from others

**Directory organization**:
```
[workspace]/
├── README.md          # Workspace identity and boundaries
├── AGENTS.md          # This file
├── .git/              # Separate git repository
├── wf/                # Wayfinder projects for this workspace
└── [workspace-specific content]/
```

---

## What belongs here

✅ **Content that belongs in this workspace**:
- Content specific to this workspace's purpose
- Projects within this workspace's boundary
- Documentation for this workspace's content
- Configuration specific to this workspace

**Boundary reason**: [Explain why this workspace is separate]
- Example: Confidentiality (public vs private work)
- Example: Team separation (different organizations)
- Example: Security policies (different compliance requirements)

---

## What does NOT belong here

❌ **Content that should go elsewhere**:
- Content from other workspaces (respect boundaries)
- Cross-workspace projects (keep in appropriate workspace)
- Shared infrastructure (consider separate shared workspace)

**Other workspaces and their content**:
- [List other workspaces and what belongs there]

---

## Wayfinder Projects

**Location**: ~/src/ws/[workspace]/wf/

**All wayfinder projects for THIS workspace live in wf/ subdirectory.**

**Usage**:
```bash
# Create wayfinder project in THIS workspace
wayfinder-new [project-name]

# Project created at:
# ~/src/ws/[workspace]/wf/[project-name]/
```

**Important**: Do not mix projects from different workspaces.

---

## Related Workspaces

**Other workspaces in ~/src/ws/**:
- **[Other workspace name]**: [Purpose and boundary]
  - Location: ~/src/ws/[other-workspace]/
  - Boundary: [Explain separation reason]

**Cross-workspace rules**:
- Minimal cross-references (keep workspaces independent)
- Document any cross-workspace dependencies in README.md
- Respect confidentiality and security boundaries

---

## Security and Confidentiality

**If this workspace has security requirements**:

- [ ] Document tracking policy (full vs metadata-only)
- [ ] Set up pre-commit hooks if needed
- [ ] Configure .gitignore for confidential content
- [ ] Document PII scrubbing guidelines (if applicable)

**Example** (for confidential workspaces):
```
Tracking Policy: Metadata-Only
- Only session-manifests/ tracked in git
- Content excluded via .gitignore
- Pre-commit hooks prevent accidents
```

---

## Example Values

Replace these placeholders when using this template:

- `[WORKSPACE_NAME]` → "company-work" (display name)
- `[workspace]` → "company-work" (directory name)
- `[Other workspace name]` → "open-source" (related workspace)
- `[other-workspace]` → "oss" (related workspace directory)

**Example result**:
```
Workspace: company-work
Root: ~/src/ws/company-work/
Other workspaces: ~/src/ws/oss/ (open-source work)
Boundary: Confidentiality (company vs public)
```

---

## Reference

**Pattern documentation**: ~/src/repos/ai-tools/base/devlog/workspace-patterns/patterns.md#pattern-3-multi-workspace

**More information**:
- Pattern details: [patterns.md](../patterns.md#pattern-3-multi-workspace)
- Real examples: [examples.md](../examples.md#example-2-multi-workspace-oss--[REDACTED_EMPLOYER])
- Migration guide: [migration-guide.md](../migration-guide.md)

---

**Template**: AGENTS-multi-workspace.md
**Pattern**: Multi-Workspace
**Last updated**: 2025-12-13
