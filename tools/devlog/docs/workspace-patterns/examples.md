# Workspace Pattern Examples

Real workspace walkthroughs showing each pattern in action.

---

## Introduction

**Purpose**: Learn from real examples of workspace patterns

**How to use this document**:
1. Find your pattern in the table below
2. Read the corresponding example
3. See how real workspaces implement the pattern
4. Apply lessons learned to your workspace

**All examples are real workspaces** - not synthetic examples. Paths and structures are accurate as of 2025-12-13.

---

## Quick Reference

| Pattern | Example | Location | Key Feature |
|---------|---------|----------|-------------|
| Mono-Repo | oss/ (engram-research) | {{DEVLOG_ROOT}}/ws/oss/ | 119 projects in projects/ |
| Multi-Workspace | oss/ + acme/ | {{DEVLOG_ROOT}}/ws/oss/ and {{DEVLOG_ROOT}}/ws/acme/ | Confidentiality boundary |
| Sub-Workspace | acme/acme-app/ | {{DEVLOG_ROOT}}/ws/acme/acme-app/ | Nested product area |
| Research-vs-Product | oss/ vs engram repo | {{DEVLOG_ROOT}}/ws/oss/ vs {{DEVLOG_ROOT}}/repos/engram/ | Different lifecycles |

---

## Example 1: Mono-Repo (oss/)

**Pattern**: Mono-Repo
**Location**: {{DEVLOG_ROOT}}/ws/oss/
**Purpose**: engram-research (work ON engram + ai-tools)

### Structure Breakdown

```
oss/                           # Workspace root
├── README.md                  # Identity: "This is engram-research"
├── INDEX.md                   # Agent-optimized directory index
├── .git/                      # Single git repository
├── .beads/                    # Task beads (issues, enhancements)
├── .claude/                   # Claude Code configuration
│
├── projects/                        # 119 wayfinder projects
│   ├── oss-workspace-audit/
│   ├── workspace-patterns-documentation/
│   └── [117 other projects]/
│
├── research/                  # Research documents
│   ├── archive/
│   ├── TIER1-RESEARCH-*.md
│   └── TIER2-RESEARCH-*.md
│
├── pre-alpha-bonus/           # Pre-alpha enhancement tasks
│   ├── PA-001/
│   ├── PA-002/
│   └── [other tasks]/
│
├── debug-scripts/             # Debugging utilities
├── docs/                      # Documentation and guides
├── agentic-design-patterns/   # AI agent design patterns
├── archived/                  # Historical/archived content
├── case-studies/              # Case study documentation
├── retrospectives/            # Project retrospectives
├── scripts/                   # Automation scripts
└── session-manifests/         # Session metadata
```

### Key Decisions

**Why mono-repo?**
- Multiple related projects (engram research + ai-tools development)
- Shared context (all work related to engram ecosystem)
- Single team/person working across all projects
- Common tools and infrastructure

**What goes at root vs projects/?**
- **projects/**: Wayfinder projects only (SDLC-driven work)
- **Root**: Shared content, research, tools, documentation
- **Clear separation**: Makes it easy to find wayfinder projects

**How is identity documented?**
- README.md clarifies: "This is engram-research, NOT engram product"
- Critical distinction: Prevents confusion with {{DEVLOG_ROOT}}/repos/engram/
- FAQ section addresses "Why is this called engram-research?"

### Documentation Highlights

**README.md** (92 lines):
```markdown
# OSS Workspace (engram-research)

This is the **engram-research** repository containing open-source project work.

## What is this repository?

**engram-research** contains:
- **Engram project**: Research and development work ON the engram project
- **AI tools**: AI-powered development tools and research
- **Related open-source projects**: Supporting tools and experiments

**Important clarification**: This repository is **engram-research**
(research about engram + ai-tools development), NOT the actual **engram**
repository (which exists separately as the core engram product).
```

**INDEX.md** (80 lines):
- Agent-optimized directory reference
- One-line descriptions for each directory
- Usage notes for AI agents and developers
- Directory count: ~30 top-level directories

### Lessons Learned

**engram/ directory confusion**:
- ❌ Initially created {{DEVLOG_ROOT}}/ws/engram/ directory
- Thought it was for "engram work" (misunderstood workspace identity)
- ✅ Should have realized oss/ = engram-research (the workspace already existed)
- **Fix**: Clear README.md identity prevents this confusion

**Clear identity prevents misplacements**:
- README.md FAQ section critical
- "What is this repository?" explanation needed
- Explicit statement: "This is X, NOT Y"

**INDEX.md helps navigation**:
- Agent-optimized format
- Quick reference for directory purposes
- Reduces "where does this go?" questions

### Template Used

Would use:
- [templates/AGENTS-mono-repo.md](templates/AGENTS-mono-repo.md)
- [templates/README-mono-repo.md](templates/README-mono-repo.md)

---

## Example 2: Multi-Workspace (oss/ + acme/)

**Pattern**: Multi-Workspace
**Location**: {{DEVLOG_ROOT}}/ws/oss/ and {{DEVLOG_ROOT}}/ws/acme/
**Purpose**: Separate open-source from confidential work

### Structure Breakdown

**Parent directory**:
```
{{DEVLOG_ROOT}}/ws/
├── AGENTS.md                  # Workspace root guidance (optional)
├── oss/                       # Open-source workspace
└── acme/                    # Confidential workspace
```

**oss/ (public workspace)**:
```
oss/
├── README.md                  # Open-source identity
├── .git/                      # Full git tracking
├── projects/                        # Public wayfinder projects
├── research/                  # Public research
└── [all content tracked]/
```

**acme/ (confidential workspace)**:
```
acme/
├── README.md                  # Security policy, workflow (255 lines)
├── .git/                      # Metadata-only tracking
├── .githooks/                 # Pre-commit safety hooks
│   └── pre-commit
├── .gitignore                 # Excludes work/, projects/, acme-app/
├── .workstream-manifest.json  # Tracking policy definition
│
├── session-manifests/         # ✅ TRACKED (PII-scrubbed)
├── scripts/                   # ✅ TRACKED (validation, install-hooks)
│
├── acme-app/                      # ❌ NOT TRACKED (sub-workspace)
├── projects/                        # ❌ NOT TRACKED (wayfinder projects)
└── work/                      # ❌ NOT TRACKED (confidential content)
```

### Boundary Maintenance

**Why separate workspaces?**
- **Confidentiality boundary**: Public (oss/) vs internal (acme/)
- **Different security policies**: Acme Corp has strict PII protection
- **Different tracking policies**: oss/ full tracking, acme/ metadata-only
- **Legal/compliance**: Acme Corp work must not leak confidential info

**How enforced?**

1. **Separate git repositories**: No accidental cross-commits
2. **Pre-commit hooks** (acme/):
   - Location: .githooks/pre-commit
   - Prevents commits outside session-manifests/
   - Scans for PII patterns
   - Installation: scripts/install-hooks.sh

3. **Metadata-only tracking** (acme/):
   - Only session-manifests/ tracked
   - .gitignore excludes work/, projects/, acme-app/
   - PII scrubbing before commit

**Cross-references**: Minimal, documented when needed in README.md

### Security Pattern (acme/)

**Pre-commit hook checks**:
```bash
# From acme/.githooks/pre-commit
# Only session-manifests/*.md can be committed
# Scans for:
# - Email addresses
# - Names
# - System hostnames
# - API keys
# - Customer/client names
```

**PII scrubbing guidelines** (from acme/README.md:118-144):

✅ **GOOD examples**:
- "Configured OAuth for internal MCP server"
- "Analyzed healthcare data pipeline performance"
- "Fixed authentication bug in admin portal"

❌ **BAD examples**:
- "Configured mcp.acme.health for John's team"
- "Analyzed patient records for Dr. Smith's COVID study"
- "Fixed bug in acme-prod-01.internal.acme.com"

**Validation script**:
```bash
# acme/scripts/validate-manifest.sh
# Checks for:
# - Required fields (date, workstreams, status, objective)
# - PII patterns
# - Confidential information markers
```

### Documentation Highlights

**oss/README.md** (92 lines):
- Open-source identity
- Related workspaces section (lines 84-87) mentions acme/

**acme/README.md** (255 lines):
- Comprehensive security policy
- PII scrubbing guidelines (40+ lines)
- Pre-commit hook documentation
- Workflow for new sessions
- Troubleshooting section

**Cross-workspace documentation**:
- Each README.md references the other
- Explains relationship and boundaries
- "Related Workspaces" section in both

### Lessons Learned

**acme-app/ initially at wrong level**:
- ❌ Initially created {{DEVLOG_ROOT}}/ws/acme-app/ (separate workspace)
- Thought it was independent from acme/
- ✅ Should be acme/acme-app/ (Quantum is Acme Corp product, use sub-workspace pattern)
- **Fix**: Parent workspace README.md documents sub-workspaces

**Clear security documentation prevents accidents**:
- Pre-commit hooks critical for confidentiality
- PII scrubbing guidelines prevent leaks
- Validation scripts catch mistakes before commit

**Metadata-only tracking works well**:
- Session manifests provide history
- Content stays private
- .gitignore + pre-commit hooks enforce policy

### Template Used

For each workspace:
- [templates/AGENTS-multi-workspace.md](templates/AGENTS-multi-workspace.md)
- [templates/README-multi-workspace.md](templates/README-multi-workspace.md)

---

## Example 3: Sub-Workspace (acme/acme-app/)

**Pattern**: Sub-Workspace
**Location**: {{DEVLOG_ROOT}}/ws/acme/acme-app/
**Purpose**: Quantum product development within Acme Corp workspace

### Structure Breakdown

**Parent workspace**:
```
acme/                        # Parent workspace
├── README.md                  # Documents acme-app/ sub-workspace
├── .gitignore                 # Excludes acme-app/ from tracking
├── .workstream-manifest.json  # Tracking policy
│
├── session-manifests/         # ✅ TRACKED
├── scripts/                   # ✅ TRACKED
│
├── acme-app/                      # ❌ NOT TRACKED (sub-workspace)
│   └── mcp-wizard-beta-polish/
│
├── projects/                        # ❌ NOT TRACKED
└── work/                      # ❌ NOT TRACKED
```

**Sub-workspace**:
```
acme-app/
└── mcp-wizard-beta-polish/    # Quantum wayfinder project
```

### Integration with Parent

**How acme/README.md documents acme-app/**:

```markdown
## Directory Structure

```
{{DEVLOG_ROOT}}/ws/acme/
├── acme-app/                      # ❌ NOT TRACKED (Quantum product sub-workspace)
└── work/                      # ❌ NOT TRACKED (confidential content)
```

**Sub-workspaces:**
- **acme-app/** - Quantum product development (distinct sub-workspace, not tracked in acme git)
- **work/** - General Acme Corp projects and session work
```

**Tracking policy** (.gitignore):
```gitignore
# Sub-workspaces
acme-app/

# Work content
work/
projects/
```

**Integration points**:
- Parent README.md (lines 40-48): Lists acme-app/ as sub-workspace
- .gitignore excludes acme-app/ from tracking
- No separate README.md yet (small, doesn't need it)

### Key Decisions

**Why sub-workspace vs separate workspace?**
- Quantum is Acme Corp product → logical grouping
- Shared security policies with parent
- Convenient to keep related work together
- Different tracking policy justified (acme-app/ has own repo)

**How documented?**
- Parent README.md explains acme-app/ exists
- .gitignore policy clear
- Future: acme-app/README.md when it grows

**Tracking policy**:
- acme-app/ excluded from acme git
- Quantum has own git tracking (separate repo)
- Keeps acme/ metadata-only

### Current State

**Size**: Small (1 wayfinder project currently)

**Documentation**:
- No acme-app/README.md yet (not needed when small)
- Parent acme/README.md provides context
- .gitignore policy clear

**Future growth**:
- When acme-app/ grows: Add acme-app/README.md
- acme-app/AGENTS.md would help agents understand acme-app-specific content
- For now, parent documentation sufficient

### Lessons Learned

**Initially created at wrong level**:
- ❌ Created {{DEVLOG_ROOT}}/ws/acme-app/ (thought it was separate workspace)
- Didn't recognize Quantum as Acme Corp product
- ✅ Should be acme/acme-app/ (sub-workspace pattern)
- **Fix**: Parent workspace README.md documents sub-workspaces

**Parent documentation prevents confusion**:
- acme/README.md mentions acme-app/
- .gitignore policy explicit
- Relationship clear

**Start simple, add docs when needed**:
- No acme-app/README.md yet (small sub-workspace)
- Parent documentation sufficient
- Add sub-workspace docs when it grows

### Template Used

When acme-app/ grows:
- [templates/AGENTS-sub-workspace.md](templates/AGENTS-sub-workspace.md)
- [templates/README-sub-workspace.md](templates/README-sub-workspace.md)

Currently: Parent workspace documents sub-workspace

---

## Example 4: Research-vs-Product (oss/ vs engram repo)

**Pattern**: Research-vs-Product
**Location**: {{DEVLOG_ROOT}}/ws/oss/ (research) vs {{DEVLOG_ROOT}}/repos/engram/ (product)
**Purpose**: Separate work ABOUT engram from engram itself

### Separation Rationale

**oss/ = engram-research** (work ON engram):
- Research documents
- Experiments and prototypes
- Analysis and investigations
- AI-tools development
- Documentation ABOUT engram development

**{{DEVLOG_ROOT}}/repos/engram/ = engram product**:
- Core engram product code
- Tested, production-ready features
- Product documentation (API docs, user guides)
- Stable implementation

**Different lifecycles**:
- Research (oss/): Exploratory, experimental, draft
- Product (engram/): Stable, tested, production-ready

### What Goes Where

**Research (oss/)**:
```
oss/
├── projects/                        # Research wayfinder projects
│   ├── engram-repository-restructuring/
│   ├── alpha-launch-2025/
│   └── [research projects]/
│
├── research/                  # Research documents
│   ├── TIER1-RESEARCH-*.md
│   ├── TIER2-RESEARCH-*.md
│   └── ECPHORY-ARCHITECTURE-*.md
│
├── pre-alpha-bonus/           # Pre-alpha enhancement work
├── agentic-design-patterns/   # Design pattern research
└── [meta-work on engram]/
```

**Product (engram/)**:
```
engram/
├── core/                      # Core engram product
│   ├── cmd/engram/
│   ├── internal/
│   └── pkg/
│
├── plugins/                   # Engram plugins
│   ├── wayfinder/
│   ├── beads-connector/
│   └── [other plugins]/
│
├── docs/                      # Product documentation
└── tests/                     # Product tests
```

**Reference direction**: oss/ → engram/ (one-way)
- Research references product code
- Product doesn't reference research

### Key Confusion Point

**oss/ IS engram-research**:
- Not a separate "research thing"
- The actual engram-research repository
- Contains work ON engram + ai-tools

**Common mistake**:
- Thinking oss/ is generic open-source work
- Not realizing oss/ = engram-research
- Creating separate engram/ workspace

**Clarity needed**:
- README.md must state: "This is engram-research"
- Explain relationship to engram product repo
- FAQ section critical

### Documentation Requirements

**oss/README.md** (lines 1-13):
```markdown
# OSS Workspace (engram-research)

This is the **engram-research** repository containing open-source project work.

**Important clarification**: This repository is **engram-research**
(research about engram + ai-tools development), NOT the actual **engram**
repository (which exists separately as the core engram product).
```

**oss/README.md** (lines 72-82):
```markdown
**Q: Why is this called engram-research and not just engram?**

A: The actual **engram** repository contains the core engram product code.
This **engram-research** repository contains research, development work,
and ai-tools - essentially the meta-work ON engram plus related tooling.
```

### Lessons Learned

**engram/ directory confusion**:
- ❌ Initially created {{DEVLOG_ROOT}}/ws/engram/ directory
- Thought: "I need a place for engram work"
- Didn't realize oss/ = engram-research already
- ✅ Should have consulted oss/README.md first

**Identity documentation critical**:
- "What is this repository?" section prevents confusion
- FAQ answers common questions
- Explicit "This is X, NOT Y" needed

**Research-vs-Product needs clear boundaries**:
- Document what goes in research vs product
- Reference direction matters (research → product only)
- Different lifecycles justify separation

### Template Used

**No specific template** - This is a conceptual relationship pattern.

Each repository uses appropriate structure pattern:
- oss/: Uses Mono-Repo pattern internally
- engram/: Uses standard product repo structure

---

## Comparison Summary

| Pattern | Example | Key Characteristic | Boundary Type | When to Use |
|---------|---------|-------------------|---------------|-------------|
| Mono-Repo | oss/ | 119 projects in projects/ | Shared workspace root | Related projects, shared tools |
| Multi-Workspace | oss/ + acme/ | Separate git repos | Confidentiality | Public vs private work |
| Sub-Workspace | acme/acme-app/ | Nested under parent | Logical subdivision | Product within company |
| Research-vs-Product | oss/ vs engram/ | Different lifecycles | Meta-work vs actual work | Research vs product code |

### Which Example for Your Situation?

**Multiple related projects?** → See Example 1 (Mono-Repo)

**Confidentiality boundary?** → See Example 2 (Multi-Workspace)

**Nested product area?** → See Example 3 (Sub-Workspace)

**Research vs product split?** → See Example 4 (Research-vs-Product)

**Combination of patterns?** → See Examples 2 + 3 (Multi-Workspace with Sub-Workspace)

---

## Next Steps

**After reviewing examples**:
1. **Choose your pattern**: See [decision-tree.md](decision-tree.md) for guidance
2. **Read pattern details**: See [patterns.md](patterns.md) for architectural definitions
3. **Use templates**: See [templates/](templates/) for AGENTS.md and README.md templates
4. **Apply to existing workspace**: See [migration-guide.md](migration-guide.md)

---

**Last updated**: 2025-12-13
**Part of**: {{DEVLOG_ROOT}}/repos/ai-tools/main/devlog/workspace-patterns/
