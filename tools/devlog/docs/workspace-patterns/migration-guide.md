# Migration Guide: Applying Patterns to Existing Workspaces

**Purpose**: Add workspace patterns documentation to existing workspaces without reorganizing files.

**Not covered here**: File reorganization, creating new workspaces (see decision-tree.md for new workspaces).

---

## Overview

This guide helps you document existing workspaces using the workspace patterns framework. The goal is to add AGENTS.md and README.md (or update existing files) to clarify workspace identity, boundaries, and navigation.

**What this does**:
- Identifies which pattern your workspace follows
- Provides template to copy and customize
- Validates documentation completeness

**What this does NOT do**:
- Move files around (that's reorganization, separate decision)
- Create new workspaces
- Change git structure

---

## Step 1: Identify Current Pattern

Audit your workspace to determine which of the 4 patterns it follows.

### Audit Checklist

Run these commands to understand your workspace structure:

```bash
# Check workspace root
ls -la {{DEVLOG_ROOT}}/ws/[workspace]/

# Check for wayfinder projects
ls {{DEVLOG_ROOT}}/ws/[workspace]/projects/ 2>/dev/null || echo "No projects/ directory"

# Check for git repository
ls -d {{DEVLOG_ROOT}}/ws/[workspace]/.git/ 2>/dev/null || echo "No .git"

# Check if nested within another workspace
ls -la {{DEVLOG_ROOT}}/ws/[workspace]/../ | grep -E '(README.md|AGENTS.md|\.git)'

# Count projects
find {{DEVLOG_ROOT}}/ws/[workspace]/projects/ -mindepth 1 -maxdepth 1 -type d 2>/dev/null | wc -l
```

### Pattern Identification Questions

Answer these questions to identify your pattern:

#### Question 1: Confidentiality Boundaries?

**Is this workspace separated from others due to confidentiality, team, or security policies?**

- ✅ YES → **Multi-Workspace pattern** (Pattern 3)
  - Examples: acme/ (confidential company work), oss/ (public work)
  - Check: Do you have other workspaces at {{DEVLOG_ROOT}}/ws/[other]/ with different access policies?

- ❌ NO → Continue to Question 2

#### Question 2: Nested Within Another Workspace?

**Is this workspace nested within a parent workspace?**

Check:
```bash
# Look for parent workspace
ls -la {{DEVLOG_ROOT}}/ws/[parent]/ | grep -E '(README.md|AGENTS.md|\.git)'
ls -la {{DEVLOG_ROOT}}/ws/[parent]/.gitignore | grep [workspace]
```

- ✅ YES → **Sub-Workspace pattern** (Pattern 4)
  - Examples: acme-app/ (nested in acme/)
  - Check: Parent README.md should document this sub-workspace
  - Check: Parent .gitignore may exclude this directory

- ❌ NO → Continue to Question 3

#### Question 3: Research vs Product Separation?

**Is this workspace for research ABOUT a product (not the product itself)?**

- ✅ YES → **Research-vs-Product pattern** (Pattern 2)
  - Examples: oss/ (engram-research, work ON engram), NOT engram itself
  - Check: Actual product repository at {{DEVLOG_ROOT}}/repos/[product]/
  - Check: This workspace has research/, projects/, docs/
  - **Note**: No template for this pattern (clarify in README.md manually)

- ❌ NO → Continue to Question 4

#### Question 4: Single Repository, Multiple Projects?

**Does this workspace contain multiple related projects in a single git repository?**

- ✅ YES → **Mono-Repo pattern** (Pattern 1)
  - Examples: oss/ (if not research-focused)
  - Check: Single .git/ at root
  - Check: Multiple projects in projects/ or other subdirectories
  - Check: Shared tools, configuration at root

- ❌ NO → **Clarification needed**
  - If workspace has single project only, consider if workspace pattern is needed
  - Workspace patterns best apply to workspaces with multiple projects

### Pattern Decision Matrix

| Pattern | Confidentiality? | Nested? | Research? | Multiple Projects? |
|---------|------------------|---------|-----------|-------------------|
| Multi-Workspace | ✅ | ❌ | Either | Either |
| Sub-Workspace | Either | ✅ | Either | Either |
| Research-vs-Product | Either | ❌ | ✅ | Usually |
| Mono-Repo | ❌ | ❌ | ❌ | ✅ |

**Tie-breaker**: If workspace matches multiple patterns:
- Confidentiality boundaries override other patterns → Multi-Workspace
- Nested structure is next priority → Sub-Workspace
- Research focus distinguishes from product → Research-vs-Product
- Default to Mono-Repo if none apply

---

## Step 2: Choose Template

Map your identified pattern to the appropriate template.

### Pattern → Template Mapping

| Pattern | AGENTS.md Template | README.md Template |
|---------|-------------------|--------------------|
| Mono-Repo | templates/AGENTS-mono-repo.md | templates/README-mono-repo.md |
| Research-vs-Product | **No template** (clarify manually) | **No template** (clarify manually) |
| Multi-Workspace | templates/AGENTS-multi-workspace.md | templates/README-multi-workspace.md |
| Sub-Workspace | templates/AGENTS-sub-workspace.md | templates/README-sub-workspace.md |

### Research-vs-Product Note

**Why no template?**
- Research-vs-Product is a meta-pattern (explains relationship between TWO repositories)
- Actual workspace uses Mono-Repo or Multi-Workspace pattern
- Document research/product relationship in README.md manually

**Example** (oss/ workspace):
```markdown
## What is this repository?

This repository is **engram-research** (research ABOUT engram), not the actual
engram product repository.

**Product repository**: {{DEVLOG_ROOT}}/repos/engram/base/ (engram itself)
**This repository**: Research, experiments, and tools development for engram

**Pattern**: Mono-Repo (research workspace with multiple projects)
```

---

## Step 3: Create AGENTS.md

Copy the template and customize for your workspace.

### Workflow

```bash
# 1. Copy template to workspace
cp {{DEVLOG_ROOT}}/repos/ai-tools/main/devlog/workspace-patterns/templates/AGENTS-[pattern].md \
   {{DEVLOG_ROOT}}/ws/[workspace]/AGENTS.md

# 2. Edit file to fill placeholders
# (Use editor of choice)
```

### Placeholder Replacement

Replace these placeholders (case-sensitive):

**Display names** (capitalized):
- `[WORKSPACE_NAME]` → Human-readable name (e.g., "Engram Research", "Acme Work")
- `[Other Workspace Name]` → Related workspace display names

**Paths** (lowercase):
- `[workspace]` → Directory name (e.g., "oss", "acme", "acme-app")
- `[parent]` → Parent workspace directory (for Sub-Workspace only)
- `[other-workspace]` → Other workspace directories

**Content-specific**:
- `[Purpose]` → Workspace purpose (1-2 sentences)
- `[Boundary reason]` → Why separate from other workspaces
- `[Sub-workspace-specific content]` → What belongs in sub-workspace

### Example: Mono-Repo Template Customization

**Before** (template/AGENTS-mono-repo.md):
```markdown
# Workspace: [WORKSPACE_NAME]

**Root**: {{DEVLOG_ROOT}}/ws/[workspace]/
**Projects**: {{DEVLOG_ROOT}}/ws/[workspace]/projects/

**Purpose**: [Explain workspace purpose]
```

**After** ({{DEVLOG_ROOT}}/ws/oss/AGENTS.md):
```markdown
# Workspace: Engram Research

**Root**: {{DEVLOG_ROOT}}/ws/oss/
**Projects**: {{DEVLOG_ROOT}}/ws/oss/projects/

**Purpose**: Research about engram project, tool development, and experiments.
Not the actual engram product (that's {{DEVLOG_ROOT}}/repos/engram/base/).
```

### Customization Checklist

After filling placeholders:

- [ ] All `[UPPERCASE]` placeholders replaced with display names
- [ ] All `[lowercase]` placeholders replaced with directory names
- [ ] Purpose section clearly states workspace identity
- [ ] Boundaries section explains what belongs vs doesn't belong
- [ ] Wayfinder projects location documented (projects/ directory path)
- [ ] Related workspaces listed (for Multi-Workspace pattern)
- [ ] Parent integration documented (for Sub-Workspace pattern)

---

## Step 4: Create/Update README.md

Choose approach based on whether README.md already exists.

### Option A: New README.md (Workspace Has None)

**Workflow**:
```bash
# 1. Copy template
cp {{DEVLOG_ROOT}}/repos/ai-tools/main/devlog/workspace-patterns/templates/README-[pattern].md \
   {{DEVLOG_ROOT}}/ws/[workspace]/README.md

# 2. Customize (same placeholder replacement as AGENTS.md)
```

**Customization points**:
- Replace all placeholders (see Step 3)
- Update directory structure diagram to match actual workspace
- Document security policies (if Multi-Workspace with confidentiality)
- Add Getting Started instructions specific to workspace

### Option B: Update Existing README.md

**Workflow**:
1. Read existing README.md
2. Identify missing sections from template
3. Add pattern clarification at top
4. Merge template sections into existing content

**Recommended additions**:

#### Add Pattern Identity (Top of File)

```markdown
# [Workspace Name]

## What is this repository?

[Existing description - keep this]

**Pattern**: [Pattern Name]
- [Key characteristic 1]
- [Key characteristic 2]

**Why this pattern?**: [Explain boundary/separation reason if applicable]
```

#### Add Workspace Boundaries Section

If missing, add after "What is this repository?":

```markdown
## Workspace Boundaries

**What belongs here**:
- [List what content belongs in this workspace]

**What does NOT belong here**:
- [List what should go elsewhere]
```

#### Add Wayfinder Projects Section

If workspace uses wayfinder:

```markdown
## Wayfinder Projects

**Location**: {{DEVLOG_ROOT}}/ws/[workspace]/projects/

**Creating new project**:
```bash
wayfinder-new [project-name]
# Creates: {{DEVLOG_ROOT}}/ws/[workspace]/projects/[project-name]/
```
```

#### Add Security Section (Multi-Workspace Only)

If workspace has confidentiality requirements:

```markdown
## Security and Confidentiality

**Tracking Policy**: [Full tracking | Metadata-only]

**Pre-Commit Hooks**: [If applicable]
```bash
./scripts/install-hooks.sh
```

**PII Scrubbing Guidelines**: [If applicable]
- [ ] No individual names
- [ ] No email addresses
- [ ] No system hostnames
```

### README.md vs AGENTS.md Content Split

**AGENTS.md** (AI agent guidance):
- Workspace structure and boundaries
- What belongs vs doesn't belong
- Wayfinder projects location
- Pattern-specific guidance
- Concise, action-oriented

**README.md** (Human-readable documentation):
- Full workspace identity and purpose
- Directory structure with explanations
- Getting Started guides
- Security policies (if applicable)
- Related documentation links
- More comprehensive, explanatory

---

## Step 5: Validate Documentation

Ensure documentation is complete and accurate.

### Validation Checklist

#### AGENTS.md Validation

- [ ] File exists at workspace root: {{DEVLOG_ROOT}}/ws/[workspace]/AGENTS.md
- [ ] No placeholder text remaining (`[UPPERCASE]` or `[lowercase]`)
- [ ] Workspace name in header matches actual name
- [ ] Root path matches actual workspace location
- [ ] Wayfinder projects path documented correctly
- [ ] "What belongs here" section is specific (not generic)
- [ ] "What does NOT belong here" section references actual other locations
- [ ] Pattern-specific sections complete:
  - Multi-Workspace: Related workspaces listed with boundaries
  - Sub-Workspace: Parent integration documented
  - Mono-Repo: Shared content areas documented

#### README.md Validation

- [ ] File exists at workspace root: {{DEVLOG_ROOT}}/ws/[workspace]/README.md
- [ ] Pattern identified in "What is this repository?" section
- [ ] Directory structure diagram matches actual workspace
- [ ] Workspace Boundaries section explains what belongs/doesn't belong
- [ ] Security section present (if Multi-Workspace with confidentiality)
- [ ] Getting Started section provides actionable instructions
- [ ] Cross-references to pattern documentation correct

#### Cross-Reference Validation

Test that links work:

- [ ] Pattern documentation: {{DEVLOG_ROOT}}/repos/ai-tools/main/devlog/workspace-patterns/patterns.md#[pattern]
- [ ] Examples: {{DEVLOG_ROOT}}/repos/ai-tools/main/devlog/workspace-patterns/examples.md
- [ ] Decision tree: {{DEVLOG_ROOT}}/repos/ai-tools/main/devlog/workspace-patterns/decision-tree.md
- [ ] Templates: {{DEVLOG_ROOT}}/repos/ai-tools/main/devlog/workspace-patterns/templates/

### Test with AI Agent

**Validation test**: Ask AI agent to describe workspace without providing context.

Open new Claude Code session with working directory: `{{DEVLOG_ROOT}}/ws/[workspace]/`

Then ask:
"What is this workspace? What belongs here? Where do wayfinder projects go?"

**Expected result**:
- Agent reads AGENTS.md automatically
- Correctly identifies workspace pattern
- Accurately describes boundaries
- Points to correct wayfinder projects location

**If agent gives wrong answers**:
- AGENTS.md may have incorrect information
- Placeholders not replaced
- Boundaries not specific enough

---

## Common Scenarios

### Scenario 1: oss/ Workspace (Mono-Repo)

**Current state**:
- Directory: {{DEVLOG_ROOT}}/ws/oss/
- Contains: 119 wayfinder projects in projects/, research/, docs/
- Purpose: engram-research (work ON engram)
- No AGENTS.md or README.md

**Steps**:
1. Identify pattern: Mono-Repo (single repo, multiple projects)
2. Copy templates:
   ```bash
   cp {{DEVLOG_ROOT}}/repos/ai-tools/main/devlog/workspace-patterns/templates/AGENTS-mono-repo.md \
      {{DEVLOG_ROOT}}/ws/oss/AGENTS.md
   cp {{DEVLOG_ROOT}}/repos/ai-tools/main/devlog/workspace-patterns/templates/README-mono-repo.md \
      {{DEVLOG_ROOT}}/ws/oss/README.md
   ```
3. Customize:
   - `[WORKSPACE_NAME]` → "Engram Research"
   - `[workspace]` → "oss"
   - Purpose: "Research about engram project, NOT the product itself"
4. Add clarification:
   ```markdown
   ## Important Clarification

   This repository is **engram-research** (research ABOUT engram), NOT the
   actual engram product repository.

   - **Product repo**: {{DEVLOG_ROOT}}/repos/engram/base/
   - **Research repo**: {{DEVLOG_ROOT}}/ws/oss/ (this workspace)
   ```
5. Validate: Check wayfinder projects location ({{DEVLOG_ROOT}}/ws/oss/projects/)

### Scenario 2: acme/ Workspace (Multi-Workspace)

**Current state**:
- Directory: {{DEVLOG_ROOT}}/ws/acme/
- Contains: Confidential company work, acme-app/ sub-workspace
- Purpose: Separate from public oss/ workspace
- Has README.md (needs pattern clarification)

**Steps**:
1. Identify pattern: Multi-Workspace (confidentiality boundary)
2. Create AGENTS.md:
   ```bash
   cp {{DEVLOG_ROOT}}/repos/ai-tools/main/devlog/workspace-patterns/templates/AGENTS-multi-workspace.md \
      {{DEVLOG_ROOT}}/ws/acme/AGENTS.md
   ```
3. Update existing README.md:
   - Add pattern identity at top:
     ```markdown
     **Pattern**: Multi-Workspace (confidentiality boundary)

     **Boundary**: Confidential company work (separate from public oss/ workspace)
     ```
   - Add security section:
     ```markdown
     ## Security and Confidentiality

     **Tracking Policy**: Metadata-only (session-manifests/ tracked, content excluded)
     **Pre-Commit Hooks**: Installed via {{DEVLOG_ROOT}}/ws/acme/scripts/install-hooks.sh
     **PII Scrubbing**: Required before committing session manifests
     ```
4. Document sub-workspace:
   ```markdown
   ## Directory Structure

   - acme-app/ - Product-specific work (sub-workspace, metadata-only tracking)
   ```
5. Validate: Check related workspaces (oss/) documented

### Scenario 3: acme-app/ Sub-Workspace (Sub-Workspace)

**Current state**:
- Directory: {{DEVLOG_ROOT}}/ws/acme/acme-app/
- Parent: {{DEVLOG_ROOT}}/ws/acme/
- Purpose: Product-specific work within acme/
- No AGENTS.md or README.md yet

**Steps**:
1. Identify pattern: Sub-Workspace (nested in acme/)
2. Create AGENTS.md:
   ```bash
   cp {{DEVLOG_ROOT}}/repos/ai-tools/main/devlog/workspace-patterns/templates/AGENTS-sub-workspace.md \
      {{DEVLOG_ROOT}}/ws/acme/acme-app/AGENTS.md
   ```
3. Customize:
   - `[WORKSPACE_NAME]` → "acme-app"
   - `[workspace]` → "acme-app"
   - `[parent]` → "acme"
   - Purpose: "Product-specific work for acme-app product"
4. Document parent integration:
   - Check {{DEVLOG_ROOT}}/ws/acme/README.md mentions acme-app/
   - Check {{DEVLOG_ROOT}}/ws/acme/.gitignore excludes acme-app/
5. Decide wayfinder location:
   - Few projects → Use parent projects/: {{DEVLOG_ROOT}}/ws/acme/projects/
   - Many projects → Create own: {{DEVLOG_ROOT}}/ws/acme/acme-app/projects/
6. Validate: Ensure parent README.md documents this sub-workspace

### Scenario 4: Workspace with Existing AGENTS.md (Update Only)

**Current state**:
- Has AGENTS.md but doesn't follow pattern format
- Need to align with workspace patterns framework

**Steps**:
1. Backup existing:
   ```bash
   cp {{DEVLOG_ROOT}}/ws/[workspace]/AGENTS.md {{DEVLOG_ROOT}}/ws/[workspace]/AGENTS.md.backup
   ```
2. Identify pattern (Step 1)
3. Read template to see recommended structure
4. Merge content:
   - Keep existing workspace-specific guidance
   - Add pattern identity header
   - Add "What belongs here" / "What does NOT belong here" sections
   - Add wayfinder projects location
   - Add pattern-specific sections (parent integration, related workspaces, etc.)
5. Validate merged content
6. Keep backup for reference

---

## When Reorganization Is Needed

**This guide is for documentation-only changes.** Sometimes workspaces need file reorganization.

### Signs Reorganization Needed

**Directory at wrong level**:
```bash
# Wrong: Workspace nested when it shouldn't be
{{DEVLOG_ROOT}}/ws/parent/child/  # child should be {{DEVLOG_ROOT}}/ws/child/

# Wrong: Workspace at top level when it should be nested
{{DEVLOG_ROOT}}/ws/workspace/  # should be {{DEVLOG_ROOT}}/ws/parent/workspace/
```

**Wayfinder projects in wrong location**:
```bash
# Wrong: Projects scattered outside projects/
{{DEVLOG_ROOT}}/ws/workspace/project1/
{{DEVLOG_ROOT}}/ws/workspace/project2/
{{DEVLOG_ROOT}}/ws/workspace/project3/

# Right: Projects in projects/ subdirectory
{{DEVLOG_ROOT}}/ws/workspace/projects/project1/
{{DEVLOG_ROOT}}/ws/workspace/projects/project2/
```

**Mixed content across workspaces**:
```bash
# Wrong: Confidential content in public workspace
{{DEVLOG_ROOT}}/ws/oss/confidential-project/  # Should be {{DEVLOG_ROOT}}/ws/company/

# Wrong: Public content in confidential workspace
{{DEVLOG_ROOT}}/ws/company/open-source-project/  # Should be {{DEVLOG_ROOT}}/ws/oss/
```

### Reorganization Approach

**If reorganization needed**:
1. Document current state first (use this guide)
2. File a bead for reorganization task
3. Plan moves carefully (git history, dependencies)
4. Update documentation after moves
5. Test that nothing broke

**Reorganization is separate from documentation.** Don't try to do both at once.

---

## Troubleshooting

### Problem: Can't Identify Pattern

**Symptoms**:
- Workspace matches multiple patterns
- Pattern decision questions don't narrow it down

**Solution**:
1. Check confidentiality first (overrides other patterns)
2. Check nesting second (Sub-Workspace if nested)
3. If still unclear, ask:
   - What is workspace separated from? (Multi-Workspace)
   - Is this research vs product? (Research-vs-Product)
   - Default to Mono-Repo if none apply

**Example**:
- Workspace has confidential content AND is research about product
- Confidentiality overrides → Multi-Workspace pattern
- Mention research aspect in README.md clarification

### Problem: Template Doesn't Fit

**Symptoms**:
- Template sections don't apply to workspace
- Workspace has unique characteristics not covered

**Solution**:
1. Start with closest-matching template
2. Remove inapplicable sections
3. Add custom sections for unique characteristics
4. Keep pattern identity clear in header
5. Document deviations in README.md

**Example**: Research-vs-Product has no template because it's a meta-pattern. Use Mono-Repo or Multi-Workspace template as base, add research clarification manually.

### Problem: Wayfinder Projects in Multiple Locations

**Symptoms**:
- Some projects in projects/, some scattered elsewhere
- Unclear which location to document

**Solution**:
1. Document current state honestly in AGENTS.md:
   ```markdown
   ## Wayfinder Projects

   **Primary location**: {{DEVLOG_ROOT}}/ws/[workspace]/projects/
   **Legacy projects**: [List other locations]

   **New projects**: Use projects/ directory going forward.
   ```
2. Optionally file bead for consolidation later
3. Don't move files during documentation phase

### Problem: Multiple Workspaces with Same Purpose

**Symptoms**:
- Two workspaces seem to have same content/purpose
- Unclear which workspace to use

**Solution**:
1. Document both as-is (don't merge during documentation)
2. In README.md, clarify distinction:
   ```markdown
   ## Related Workspaces

   **Other workspace**: {{DEVLOG_ROOT}}/ws/[other]/
   **Distinction**: [Explain difference - team, timeline, scope, etc.]
   ```
3. File bead if consolidation makes sense
4. Keep workspaces separate until consolidation plan ready

### Problem: Parent Workspace Doesn't Document Sub-Workspace

**Symptoms** (Sub-Workspace pattern):
- Parent README.md doesn't mention this sub-workspace
- Unclear if sub-workspace is supposed to be nested

**Solution**:
1. Create sub-workspace AGENTS.md anyway
2. Update parent README.md to document sub-workspace:
   ```markdown
   ## Directory Structure

   - [sub-workspace]/ - [Purpose] (sub-workspace, [tracking policy])
   ```
3. Add to parent .gitignore if sub-workspace not tracked
4. Validate parent integration documented correctly

---

## Next Steps

After completing documentation for existing workspace:

1. **Validate** (Step 5) - Ensure everything works
2. **Test with AI agent** - Start new session, verify agent understands workspace
3. **Update related workspaces** - Document relationships in other workspace READMEs
4. **File beads for improvements** - If reorganization or consolidation needed
5. **Share pattern learnings** - Update examples.md if workspace demonstrates new edge case

---

## Reference

**Pattern documentation**: {{DEVLOG_ROOT}}/repos/ai-tools/main/devlog/workspace-patterns/patterns.md

**More information**:
- Pattern details: [patterns.md](patterns.md)
- Real examples: [examples.md](examples.md)
- Decision tree (for new workspaces): [decision-tree.md](decision-tree.md)
- Templates: [templates/](templates/)

---

**Last updated**: 2025-12-13
**Version**: 1.0
