# Workspace Pattern Decision Tree

Interactive guide to help you choose the correct workspace pattern.

---

## Purpose

**What**: Decision flowchart to guide pattern selection
**How to use**: Answer questions → get pattern recommendation → use template

---

## Quick Decision Flowchart

```
START: I need workspace patterns documentation

Question 1: Do you have confidentiality boundaries?
├─ YES → Multi-Workspace Pattern
│         Template: AGENTS-multi-workspace.md, README-multi-workspace.md
│         Example: {{DEVLOG_ROOT}}/ws/oss/ vs {{DEVLOG_ROOT}}/ws/acme/
│
└─ NO → Question 2: Is this nested within existing workspace?
        ├─ YES → Sub-Workspace Pattern
        │         Template: AGENTS-sub-workspace.md, README-sub-workspace.md
        │         Example: {{DEVLOG_ROOT}}/ws/acme/acme-app/
        │
        └─ NO → Question 3: Is this research vs product split?
                ├─ YES → Research-vs-Product Pattern
                │         Template: None (conceptual pattern)
                │         Example: {{DEVLOG_ROOT}}/ws/oss/ vs {{DEVLOG_ROOT}}/repos/engram/
                │
                └─ NO → Question 4: Multiple related projects?
                        ├─ YES → Mono-Repo Pattern
                        │         Template: AGENTS-mono-repo.md, README-mono-repo.md
                        │         Example: {{DEVLOG_ROOT}}/ws/oss/
                        │
                        └─ NO → No clear pattern (consult examples.md)
```

---

## Decision Path 1: Confidentiality Boundaries

**Question**: Do you have confidentiality boundaries (public vs private work, different teams, security policies)?

**If YES** → **Multi-Workspace Pattern**

**Details**:
- **Boundary types**: Public vs confidential, different teams, legal/compliance
- **Key indicator**: Content can't mix due to security, legal, or team reasons
- **Implementation**: Separate git repositories, different tracking policies

**Example**: oss/ (public) vs acme/ (confidential)
- oss/: Full git tracking, public content
- acme/: Metadata-only tracking, PII scrubbing, pre-commit hooks

**Next steps**:
1. Create separate workspace directories
2. Use [templates/AGENTS-multi-workspace.md](templates/AGENTS-multi-workspace.md)
3. Use [templates/README-multi-workspace.md](templates/README-multi-workspace.md)
4. Document boundaries in README.md
5. Set up security mechanisms if needed (pre-commit hooks, .gitignore)

**See**: [patterns.md#pattern-3-multi-workspace](patterns.md#pattern-3-multi-workspace), [examples.md#example-2-multi-workspace](examples.md#example-2-multi-workspace-oss--acme)

---

## Decision Path 2: Nested Workspace

**Question**: Is this nested within an existing workspace (product within company, team within org)?

**If YES** → **Sub-Workspace Pattern**

**Details**:
- **Nesting rationale**: Product-specific area, different tracking policy, logical subdivision
- **Key indicator**: Belongs under parent workspace but has distinct purpose
- **Implementation**: Nested directory, parent README.md documents it, may have own tracking

**Example**: acme/acme-app/ (Quantum product within Acme Corp workspace)
- Parent: acme/ (company workspace)
- Sub-workspace: acme-app/ (Quantum product)
- .gitignore excludes acme-app/ from parent git
- Parent README.md documents acme-app/

**Next steps**:
1. Create subdirectory under parent workspace
2. Update parent README.md to document sub-workspace
3. Update parent .gitignore if needed
4. When sub-workspace grows: Add acme-app/README.md and acme-app/AGENTS.md
5. Use [templates/AGENTS-sub-workspace.md](templates/AGENTS-sub-workspace.md)
6. Use [templates/README-sub-workspace.md](templates/README-sub-workspace.md)

**See**: [patterns.md#pattern-4-sub-workspace](patterns.md#pattern-4-sub-workspace), [examples.md#example-3-sub-workspace](examples.md#example-3-sub-workspace-acmeacme-app)

---

## Decision Path 3: Research vs Product Split

**Question**: Is this research vs product split (work ABOUT product vs product itself)?

**If YES** → **Research-vs-Product Pattern**

**Details**:
- **Key distinction**: Research = meta-work (work ON product), Product = actual implementation
- **Different lifecycles**: Research is exploratory, product is stable
- **Reference direction**: Research → Product (one-way)

**Example**: oss/ (engram-research) vs {{DEVLOG_ROOT}}/repos/engram/ (engram product)
- oss/: Research, experiments, analysis, prototypes
- engram/: Core product code, tested features
- oss/ references engram/, not vice versa

**Next steps**:
1. Create separate repositories (research and product)
2. Document relationship in both README.md files
3. Use Mono-Repo or Multi-Workspace pattern for each repo's internal structure
4. **No specific template** (conceptual pattern)

**See**: [patterns.md#pattern-2-research-vs-product](patterns.md#pattern-2-research-vs-product), [examples.md#example-4-research-vs-product](examples.md#example-4-research-vs-product-oss-vs-engram-repo)

---

## Decision Path 4: Multiple Related Projects

**Question**: Do you have multiple related projects (same domain, team, or purpose)?

**If YES** → **Mono-Repo Pattern**

**Details**:
- **Key characteristic**: Single repo, multiple projects, shared context
- **Shared resources**: Configuration, tools, scripts available to all projects
- **Organization**: projects/ subdirectory for wayfinder projects

**Example**: oss/ (engram-research with 119 projects)
- Single .git directory
- projects/ contains all wayfinder projects
- Shared tools, research, documentation at root

**Next steps**:
1. Create workspace directory with .git
2. Create projects/ subdirectory for wayfinder projects
3. Use [templates/AGENTS-mono-repo.md](templates/AGENTS-mono-repo.md)
4. Use [templates/README-mono-repo.md](templates/README-mono-repo.md)
5. Add README.md to clarify workspace identity
6. Add INDEX.md for directory navigation (optional)

**See**: [patterns.md#pattern-1-mono-repo](patterns.md#pattern-1-mono-repo), [examples.md#example-1-mono-repo-oss](examples.md#example-1-mono-repo-oss)

---

## Edge Cases

### None of these patterns fit

**If no pattern matches your situation**:

1. **Review examples**: See [examples.md](examples.md) for real workspace walkthroughs
2. **Combine patterns**: Some patterns can combine (Multi-Workspace + Sub-Workspace)
3. **Start simple**: Use Mono-Repo as default, refine later
4. **Consult patterns.md**: See [Pattern Relationships](patterns.md#pattern-relationships) section

### Multiple patterns apply

**If more than one pattern fits**:

**Compatible combinations**:
- Multi-Workspace + Sub-Workspace: ✅ (acme/ workspace with acme-app/ sub-workspace)
- Mono-Repo + Multi-Workspace: ✅ (oss/ is mono-repo, acme/ is separate workspace)
- Research-vs-Product + Mono-Repo: ✅ (research repo uses mono-repo internally)

**Choose primary pattern** based on main characteristic:
- Confidentiality boundary? → Multi-Workspace (primary)
- Nesting? → Sub-Workspace (within Multi-Workspace)
- Multiple projects? → Mono-Repo (within each workspace)

**See**: [patterns.md#pattern-relationships](patterns.md#pattern-relationships)

### Unsure about workspace identity

**If you're unclear which pattern**:

1. **Ask clarifying questions**:
   - Is this work confidential? (Multi-Workspace)
   - Is this nested under parent? (Sub-Workspace)
   - Is this research vs product? (Research-vs-Product)
   - Multiple projects? (Mono-Repo)

2. **Audit current structure**: See [migration-guide.md](migration-guide.md)

3. **Start with templates**: Pick closest match, adjust as needed

4. **Experiment**: Create test workspace, validate with AI agent

---

## Quick Reference Table

| Pattern | Key Question | Example | Templates |
|---------|--------------|---------|-----------|
| **Mono-Repo** | Multiple related projects? | {{DEVLOG_ROOT}}/ws/oss/ | AGENTS-mono-repo.md<br/>README-mono-repo.md |
| **Research-vs-Product** | Research vs product split? | oss/ vs engram/ | None (conceptual) |
| **Multi-Workspace** | Confidentiality boundaries? | oss/ vs acme/ | AGENTS-multi-workspace.md<br/>README-multi-workspace.md |
| **Sub-Workspace** | Nested within existing? | acme/acme-app/ | AGENTS-sub-workspace.md<br/>README-sub-workspace.md |

---

## Next Steps

**After choosing pattern**:

1. **Read pattern details**: See [patterns.md](patterns.md) for architectural definitions
2. **Review examples**: See [examples.md](examples.md) for real workspace walkthroughs
3. **Use templates**: See [templates/](templates/) for AGENTS.md and README.md
4. **Validate**: Test with AI agent, verify clarity

**For existing workspaces**: See [migration-guide.md](migration-guide.md)

---

**Last updated**: 2025-12-13
**Part of**: {{DEVLOG_ROOT}}/repos/ai-tools/main/devlog/workspace-patterns/
