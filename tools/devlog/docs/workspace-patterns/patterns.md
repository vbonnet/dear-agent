# Workspace Patterns

Architectural reference for workspace organization patterns with detailed definitions.

---

## Introduction

**What are workspace patterns?**

Workspace patterns are architectural blueprints for organizing development workspaces. They define structure, boundaries, and invariants that prevent confusion about where content belongs.

**Why patterns matter:**

Without clear patterns, developers and AI agents create content in wrong locations, leading to:
- Directories at incorrect levels ({{DEVLOG_ROOT}}/ws/engram/ instead of recognizing oss/ = engram-research)
- Wayfinder projects in wrong workspaces
- Unclear workspace boundaries
- Identity confusion (is this research or product?)

**How to use this document:**

- **New workspace?** Read pattern definitions → Choose pattern → Use templates
- **Existing workspace?** Identify current pattern → Apply documentation
- **Confused about boundaries?** Review examples and anti-patterns

---

## Pattern 1: Mono-Repo

**Definition**: Single git repository containing multiple related projects that share a common workspace root.

### Structure

```
workspace/
├── README.md                  # Workspace identity and guidance
├── INDEX.md                   # Directory index (optional)
├── .git/                      # Single git repository
├── projects/                        # Wayfinder projects (all in one place)
├── research/                  # Research documents
├── docs/                      # Documentation
├── scripts/                   # Shared scripts and tools
└── [other shared content]/
```

**Key characteristics**:
- Single .git directory at root
- Multiple related projects in projects/ subdirectory
- Shared configuration and tooling at root level
- All content shares workspace root

### Boundaries

**Workspace root**: All content belongs to this workspace
**Project organization**: Related projects grouped logically
**Shared resources**: Configuration, scripts, tools available to all projects

### Invariants

- [ ] Single AGENTS.md at workspace root
- [ ] Single .git directory (not nested repos)
- [ ] All wayfinder projects in projects/ subdirectory (or documented elsewhere)
- [ ] README.md clarifies workspace identity
- [ ] Shared configuration lives at root

### When to Use

**Best for**:
- Multiple related projects (same domain, team, or purpose)
- Shared tools and configuration across projects
- Same team working on interconnected work
- Research repositories with multiple experiments

**Example scenarios**:
- Research workspace with multiple investigations
- Tool development with multiple related utilities
- Company projects sharing common infrastructure

### Example: oss/ (engram-research)

**Location**: {{DEVLOG_ROOT}}/ws/oss/

**Structure**:
```
oss/
├── README.md                  # Identity: "This is engram-research"
├── INDEX.md                   # Agent-optimized directory index
├── .git/                      # Single git repository
├── projects/                        # 119 wayfinder projects
├── research/                  # Research documents
├── pre-alpha-bonus/           # Pre-alpha enhancement tasks
├── debug-scripts/             # Debugging utilities
├── docs/                      # Documentation
└── [other directories]/
```

**Key decisions**:
- Why mono-repo? Multiple related projects (engram research + ai-tools development)
- What goes at root vs projects/? Root for shared content, projects/ for wayfinder projects only
- How is identity documented? README.md clarifies "oss = engram-research, NOT engram product"

**Documentation highlights**:
- README.md (lines 72-82): FAQ clarifies "Why is this called engram-research?"
- INDEX.md: Agent-optimized directory reference with one-line descriptions
- Clear guidance on "what belongs where"

**Lessons learned**:
- engram/ directory confusion: Initially created at {{DEVLOG_ROOT}}/ws/engram/ thinking it was for "engram work"
- Should have realized oss/ = engram-research (meta-work ON engram)
- Clear identity documentation prevents misplacements

### Anti-Patterns

❌ **Creating subdirectories that should be at root**
- Don't nest workspaces within mono-repo
- Example: Creating {{DEVLOG_ROOT}}/ws/oss/engram/ when engram work belongs at oss/ root

❌ **Mixing unrelated projects**
- Don't combine public and confidential work in same repo
- Don't mix different teams' projects without clear rationale

❌ **Multiple .git directories (nested repos)**
- Indicates separate workspaces, not mono-repo
- Use Multi-Workspace pattern instead

### Templates

- **AGENTS.md**: [templates/AGENTS-mono-repo.md](templates/AGENTS-mono-repo.md)
- **README.md**: [templates/README-mono-repo.md](templates/README-mono-repo.md)

---

## Pattern 2: Research-vs-Product

**Definition**: Separate repositories for research (work ABOUT a product) versus the actual product code.

### Structure

**Research repository**:
```
research/
├── README.md                  # Research identity
├── .git/                      # Separate git repository
├── experiments/               # Experiments and prototypes
├── analysis/                  # Analysis documents
├── docs/                      # Documentation ABOUT product
└── prototypes/                # Proof-of-concept code
```

**Product repository**:
```
product/
├── README.md                  # Product identity
├── .git/                      # Separate git repository
├── src/                       # Actual product source code
├── tests/                     # Product tests
└── docs/                      # Product documentation
```

**Key characteristics**:
- Two separate git repositories
- Research repo contains meta-work (work ON the product)
- Product repo contains actual product implementation
- Research references product, not vice versa

### Boundaries

**Research = meta-work**: Experiments, analysis, documentation ABOUT product
**Product = actual work**: Core product code, tested features, stable implementation

**Reference direction**: Research → Product (one-way)

### Invariants

- [ ] Separate git repositories (not subdirectories)
- [ ] Research repo doesn't contain production code
- [ ] Product repo doesn't contain experiments or draft research
- [ ] Clear README.md in each explaining relationship
- [ ] Different lifecycles (research is exploratory, product is stable)

### When to Use

**Best for**:
- Product code needs to be separate from research work
- Different lifecycles (research vs stable product)
- Need to keep experiments out of production repository
- Clear separation between "work ON X" vs "X itself"

**Example scenarios**:
- Academic research + implementation
- R&D work + production system
- Prototyping + stable product

### Example: oss/ vs {{DEVLOG_ROOT}}/repos/engram/

**Research repository**: {{DEVLOG_ROOT}}/ws/oss/ (engram-research)
**Product repository**: {{DEVLOG_ROOT}}/repos/engram/ (engram core product)

**Separation rationale**:
- oss/ = work ON engram (research, analysis, ai-tools development)
- engram/ = engram itself (core product code, tested features)
- Different lifecycles: oss/ is exploratory, engram/ is stable

**What goes where**:

**Research (oss/)**:
- Prototypes and experiments
- Analysis and investigation documents
- Documentation ABOUT engram development
- AI-tools research and development

**Product (engram/)**:
- Core engram product code
- Tested, production-ready features
- Product documentation (user guides, API docs)

**Reference direction**: oss/ references engram/ (one-way)

**Key confusion point**:
- oss/ **IS** engram-research (not a separate "research thing")
- Need to clarify in README.md to prevent confusion
- Identity documentation critical

**Lessons learned**:
- Initially created engram/ at {{DEVLOG_ROOT}}/ws/engram/ (thought it was for "engram work")
- Should have recognized oss/ = engram-research
- README.md identity clarification prevents misplacements

### Anti-Patterns

❌ **Mixing research and product in same repository**
- Don't put experiments in product repo
- Don't put stable product code in research repo

❌ **Research repository becoming "everything else"**
- Avoid dumping ground for "not product" content
- Maintain clear identity and purpose

❌ **Product repo referencing research repo**
- Keep dependency direction clean: research → product only
- Product should be self-contained

### Templates

**No specific template** - This is a conceptual relationship pattern, not a workspace structure pattern.

Use Mono-Repo or Multi-Workspace patterns for each repository's structure.

---

## Pattern 3: Multi-Workspace

**Definition**: Multiple independent workspaces with clear boundaries, typically based on confidentiality or team separation.

### Structure

```
{{DEVLOG_ROOT}}/ws/
├── workspace-a/               # Independent workspace
│   ├── README.md              # Workspace A identity
│   ├── .git/                  # Separate git repository
│   └── [workspace-a content]/
│
└── workspace-b/               # Independent workspace
    ├── README.md              # Workspace B identity
    ├── .git/                  # Separate git repository
    └── [workspace-b content]/
```

**Key characteristics**:
- Each workspace is self-contained
- Separate git repositories
- Minimal cross-references between workspaces
- Independent purposes and boundaries

### Boundaries

**Boundary types**:
- **Confidentiality**: Public vs confidential work
- **Team**: Different teams or organizations
- **Security**: Different security policies

**Boundary enforcement**:
- Separate git repositories
- Different tracking policies (full vs metadata-only)
- Security mechanisms (pre-commit hooks, PII scrubbing)

### Invariants

- [ ] Each workspace has own AGENTS.md and README.md
- [ ] Each workspace has own .git repository
- [ ] Cross-workspace content is rare and documented when needed
- [ ] Boundaries clearly explained in README.md
- [ ] Independent lifecycles and purposes

### When to Use

**Best for**:
- Confidentiality boundaries (public vs private work)
- Different teams or organizations
- Different security policies
- Legal or compliance requirements

**Example scenarios**:
- Open-source vs company work
- Multiple client projects
- Public research vs confidential implementation

### Example: oss/ vs acme/

**Workspace A**: {{DEVLOG_ROOT}}/ws/oss/ (open-source work)
**Workspace B**: {{DEVLOG_ROOT}}/ws/acme/ (confidential company work)

**Structure comparison**:

**oss/ (public)**:
```
oss/
├── README.md                  # Open-source identity
├── .git/                      # Full git tracking
├── projects/                        # Public wayfinder projects
└── [public content]/
```

**acme/ (confidential)**:
```
acme/
├── README.md                  # Security policy, workflow
├── .git/                      # Metadata-only tracking
├── .githooks/                 # Pre-commit safety hooks
├── .gitignore                 # Excludes work/, projects/, acme-app/
├── session-manifests/         # ✅ TRACKED (PII-scrubbed)
├── scripts/                   # ✅ TRACKED (validation tools)
└── [confidential content]/    # ❌ NOT TRACKED
```

**Boundary maintenance**:

**Why separate?**
- Confidentiality boundary (public vs internal)
- Different security policies
- Different tracking policies

**How enforced?**
- Acme Corp pre-commit hooks prevent content commits
- Only session-manifests/ tracked (PII-scrubbed)
- .gitignore excludes confidential content

**Cross-references**: Minimal, documented in README.md when needed

**Security mechanisms** (acme/):

1. **Pre-commit hooks**:
   - Prevents accidental commits of confidential content
   - Only session-manifests/*.md can be committed
   - Scans for PII (emails, names, etc.)
   - Installed via scripts/install-hooks.sh

2. **PII scrubbing guidelines**:
   - No names, emails, hostnames, API keys
   - Use roles ("engineer", "manager")
   - Generic placeholders ("Client A", "server-A")
   - Validation script checks for PII patterns

**Documentation highlights**:
- oss/README.md: Clarifies open-source identity
- acme/README.md (255 lines): Comprehensive security policy, workflow, PII scrubbing guidelines
- Both READMEs document workspace boundaries

**Lessons learned**:
- acme-app/ initially created at wrong level ({{DEVLOG_ROOT}}/ws/acme-app/ instead of acme/acme-app/)
- Clear security documentation prevents accidents
- Pre-commit hooks critical for confidentiality enforcement

### Anti-Patterns

❌ **Creating workspace for minor separation**
- Don't create separate workspace for slight differences
- Use subdirectories or projects instead

❌ **Frequent cross-workspace dependencies**
- Indicates workspaces aren't truly independent
- Consider consolidating into mono-repo

❌ **Unclear boundaries**
- Document why workspaces are separate
- Explain confidentiality or team boundaries in README.md

### Templates

- **AGENTS.md**: [templates/AGENTS-multi-workspace.md](templates/AGENTS-multi-workspace.md)
- **README.md**: [templates/README-multi-workspace.md](templates/README-multi-workspace.md)

---

## Pattern 4: Sub-Workspace

**Definition**: Workspace nested within a parent workspace, with distinct purpose but logical relationship to parent.

### Structure

```
parent-workspace/
├── README.md                  # Documents sub-workspace
├── .git/                      # Parent git repository
├── .gitignore                 # May exclude sub-workspace
├── [parent content]/
└── sub-workspace/             # Nested sub-workspace
    ├── [sub-workspace content]/
    └── [may have own README.md when it grows]
```

**Key characteristics**:
- Nested under parent workspace
- May have own git repo or tracking policy
- Parent README.md documents sub-workspace
- Logical relationship (product within company, team within org)

### Boundaries

**Nesting rationale**: Product-specific area, team-specific content, logical subdivision

**Integration**: Parent workspace documents sub-workspace in README.md and .gitignore

### Invariants

- [ ] Parent AGENTS.md or README.md mentions sub-workspace
- [ ] Sub-workspace purpose is clear
- [ ] Integration with parent is documented
- [ ] Nesting is shallow (avoid deep hierarchies)

### When to Use

**Best for**:
- Product area within company workspace
- Different tracking policy (sub-workspace not tracked in parent git)
- Logical subdivision that doesn't warrant separate workspace
- Team-specific content within larger organization workspace

**Example scenarios**:
- Product development within company workspace
- Team space within multi-team workspace
- Project-specific area within department workspace

### Example: acme/acme-app/

**Parent workspace**: {{DEVLOG_ROOT}}/ws/acme/ (Acme company work)
**Sub-workspace**: {{DEVLOG_ROOT}}/ws/acme/acme-app/ (Quantum product)

**Structure**:
```
acme/
├── README.md                  # Documents acme-app/ sub-workspace
├── .gitignore                 # Excludes acme-app/ from tracking
├── .workstream-manifest.json  # Tracking policy
├── session-manifests/         # ✅ TRACKED
├── scripts/                   # ✅ TRACKED
├── acme-app/                      # ❌ NOT TRACKED (sub-workspace)
│   └── mcp-wizard-beta-polish/
└── work/                      # ❌ NOT TRACKED
```

**Integration with parent**:

**How acme/README.md documents acme-app/**:
- Line 40: "acme-app/ - ❌ NOT TRACKED (Quantum product sub-workspace)"
- Line 47: "acme-app/ - Quantum product development (distinct sub-workspace)"
- .gitignore policy inherited from parent

**Why nested?**
- Quantum is Acme Corp product → logical grouping
- Different tracking policy (acme-app/ not tracked in acme git)
- Product-specific area within company workspace

**Why not separate workspace?**
- Logical relationship (Quantum is Acme Corp product)
- Shared security policies
- Convenient grouping under company workspace

**Current state**:
- Contains wayfinder projects (mcp-wizard-beta-polish/)
- No README.md yet (small, doesn't need it)
- Parent README.md provides integration documentation

**Lessons learned**:
- Initially created at wrong level ({{DEVLOG_ROOT}}/ws/acme-app/)
- Should be acme/acme-app/ (product within company workspace)
- Parent documentation prevents confusion

### Anti-Patterns

❌ **Deep nesting (more than 2 levels)**
- Avoid parent/child/grandchild hierarchies
- Keep structure flat

❌ **Unclear sub-workspace purpose**
- Document why nested vs separate workspace
- Explain in parent README.md

❌ **Missing parent documentation**
- Always document sub-workspace in parent README.md
- Explain integration, tracking policy, purpose

### Templates

- **AGENTS.md**: [templates/AGENTS-sub-workspace.md](templates/AGENTS-sub-workspace.md)
- **README.md**: [templates/README-sub-workspace.md](templates/README-sub-workspace.md)

---

## Pattern Relationships

**Can patterns combine?**

Yes, patterns can combine in specific ways:

### Compatible Combinations

**Mono-Repo + Multi-Workspace**:
- ✅ Example: oss/ is mono-repo, acme/ is separate workspace
- Use case: Multiple independent workspaces, each using mono-repo internally

**Multi-Workspace + Sub-Workspace**:
- ✅ Example: acme/ has acme-app/ sub-workspace
- Use case: Multiple workspaces, some with nested product areas

**Research-vs-Product + Mono-Repo**:
- ✅ Example: oss/ (research, mono-repo) vs engram/ (product)
- Use case: Research workspace is mono-repo, product is separate

### Pattern Conflicts

**Mono-Repo vs Multi-Workspace** (for same workspace):
- ❌ Choose one per workspace
- Mono-repo = single workspace, Multi-workspace = multiple independent
- Can't be both simultaneously

### Decision Matrix

| Scenario | Pattern(s) | Rationale |
|----------|-----------|-----------|
| Multiple related projects, same team | Mono-Repo | Shared context and tools |
| Public vs confidential work | Multi-Workspace | Confidentiality boundary |
| Research vs product split | Research-vs-Product + Mono-Repo (each) | Different lifecycles |
| Product within company workspace | Multi-Workspace + Sub-Workspace | Logical nesting |
| Team area within org workspace | Sub-Workspace | Logical subdivision |

### Real-World Combinations

**Example 1**: oss/ (mono-repo) + acme/ (multi-workspace) + acme-app/ (sub-workspace)
- oss/: Mono-repo for engram-research
- acme/: Separate workspace for confidential work
- acme-app/: Sub-workspace within acme/ for Quantum product

**Example 2**: Research mono-repo + Product repo
- Research workspace: Mono-repo with experiments, analysis
- Product workspace: Separate repo with stable code
- Pattern: Research-vs-Product + Mono-Repo

---

## Choosing a Pattern

**Quick flowchart reference**: See [decision-tree.md](decision-tree.md) for interactive guide

**Questions to ask**:
1. Do you have confidentiality boundaries? → Multi-Workspace
2. Is this nested within existing workspace? → Sub-Workspace
3. Is this research vs product split? → Research-vs-Product
4. Multiple related projects? → Mono-Repo

### Edge Cases

**None of these fit?**
- Review [examples.md](examples.md) for real workspace walkthroughs
- Consider hybrid approach (combining patterns)
- Ask for guidance or file issue

**Multiple patterns apply?**
- See Pattern Relationships section above
- Patterns can combine (e.g., Multi-Workspace + Sub-Workspace)
- Choose primary pattern based on main characteristic

**Unsure?**
- Start with [decision-tree.md](decision-tree.md) for guided selection
- Review [examples.md](examples.md) for real-world examples
- Use [migration-guide.md](migration-guide.md) for existing workspaces

---

## Summary Table

| Pattern | Key Characteristic | Example | When to Use |
|---------|-------------------|---------|-------------|
| Mono-Repo | Single repo, multiple projects | oss/ | Related projects, shared tools |
| Research-vs-Product | Separate repos for research vs product | oss/ vs engram/ | Different lifecycles |
| Multi-Workspace | Independent workspaces | oss/ vs acme/ | Confidentiality, team boundaries |
| Sub-Workspace | Nested workspace | acme/acme-app/ | Product area within workspace |

---

**For more information**:
- **Pattern selection**: [decision-tree.md](decision-tree.md)
- **Real examples**: [examples.md](examples.md)
- **Existing workspaces**: [migration-guide.md](migration-guide.md)
- **Templates**: [templates/](templates/)

---

**Last updated**: 2025-12-13
**Part of**: {{DEVLOG_ROOT}}/repos/ai-tools/main/devlog/workspace-patterns/
