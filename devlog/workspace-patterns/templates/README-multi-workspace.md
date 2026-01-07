# [Workspace Name]

Human-readable documentation for multi-workspace pattern.

---

## What is this repository?

[Brief description of workspace purpose and boundary - 2-3 sentences]

**Boundary**: [Explain why this workspace is separate from others]
- Example: Confidentiality (public vs private work)
- Example: Team separation (different organizations)
- Example: Security policies (different compliance requirements)

---

## Pattern

This workspace follows the **Multi-Workspace pattern**:
- Independent workspace with clear boundaries
- Separate git repository from other workspaces
- Minimal cross-references to other workspaces
- Own lifecycle, team, or security policy

**Why separate?**
- [Primary reason for workspace separation]
- [Secondary reasons if applicable]

---

## Directory Structure

```
[workspace]/
├── README.md                  # This file (workspace identity and boundaries)
├── AGENTS.md                  # AI agent guidance
├── .git/                      # Separate git repository
├── projects/                        # Wayfinder projects for THIS workspace
└── [workspace-specific content]/
```

**Boundary enforcement**:
- Separate git repository
- [If applicable: Pre-commit hooks, .gitignore rules]
- [If applicable: Security mechanisms]

---

## Workspace Boundaries

**What belongs here**:
- Content specific to [this workspace's purpose]
- Projects within [this workspace's domain/team]
- [Workspace-specific examples]

**What does NOT belong here**:
- Content from [other workspace names]
- [Specific examples of what goes elsewhere]

**Boundary rules**:
- [Any specific rules for maintaining separation]
- [Cross-workspace interaction policies]

---

## Related Workspaces

**Other workspaces in {{DEVLOG_ROOT}}/ws/**:

### [Other Workspace Name]
- **Location**: {{DEVLOG_ROOT}}/ws/[other-workspace]/
- **Purpose**: [Other workspace purpose]
- **Boundary**: [Why separate from this workspace]

### [Another Workspace Name] (if applicable)
- **Location**: {{DEVLOG_ROOT}}/ws/[another-workspace]/
- **Purpose**: [Purpose]
- **Boundary**: [Reason for separation]

**Cross-workspace guidelines**:
- Minimize cross-references
- Document dependencies when needed
- Respect confidentiality boundaries

---

## Security and Confidentiality (if applicable)

**If this workspace has security requirements**:

### Tracking Policy
- **Full tracking**: All content tracked in git
- **Metadata-only**: Only session-manifests/ tracked (content excluded)

### Pre-Commit Hooks (if applicable)
```bash
# Install pre-commit hooks
./scripts/install-hooks.sh
```

**What hooks check**:
- [List validation checks]
- [PII scrubbing if applicable]
- [File path restrictions]

### PII Scrubbing Guidelines (if applicable)

**Before committing session manifests**:
- [ ] No individual names (use roles: "engineer", "manager")
- [ ] No email addresses
- [ ] No system hostnames
- [ ] No API keys or credentials
- [ ] Generic descriptions only

**Example transformations**:
- ❌ "Fixed bug in server-01.company.com" → ✅ "Fixed bug in backend server"
- ❌ "Analyzed data for Client ABC" → ✅ "Analyzed client data"

---

## Getting Started

### For AI Agents

1. Read AGENTS.md for workspace-specific guidance
2. Understand workspace boundaries
3. Respect cross-workspace separation
4. Use projects/ for wayfinder projects in THIS workspace only

### For Developers

**Creating new wayfinder project**:
```bash
# Create in THIS workspace only
wayfinder-new [project-name]

# Project created at:
# {{DEVLOG_ROOT}}/ws/[workspace]/projects/[project-name]/
```

**Working with other workspaces**:
- Keep work separated by workspace
- Document cross-workspace dependencies if needed
- Respect confidentiality boundaries

---

## Related Documentation

**Pattern documentation**:
- Pattern details: {{DEVLOG_ROOT}}/repos/ai-tools/main/devlog/workspace-patterns/patterns.md#multi-workspace
- Real examples: {{DEVLOG_ROOT}}/repos/ai-tools/main/devlog/workspace-patterns/examples.md#multi-workspace
- Decision tree: {{DEVLOG_ROOT}}/repos/ai-tools/main/devlog/workspace-patterns/decision-tree.md

---

**Workspace**: [workspace-name]
**Pattern**: Multi-Workspace
**Boundary**: [confidentiality/team/security]
**Created**: [date]
**Last updated**: [date]
