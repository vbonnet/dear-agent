# AI Tools Devlog

Knowledge base and best practices for AI-assisted development workflows.

---

## What is Devlog?

Devlog contains documentation, guides, templates, and patterns for effective AI-assisted development. Think of it as a "development blog" that captures learnings, best practices, and reusable patterns.

## What Does Devlog Cover?

Devlog documents two aspects of development workspace organization:

1. **Workspace Patterns** ([workspace-patterns/](workspace-patterns/))
   - How to organize `{{DEVLOG_ROOT}}/ws/` with multiple projects
   - Mono-Repo, Multi-Workspace, Sub-Workspace patterns
   - AGENTS.md and README.md templates

2. **Repository Patterns** ([repo-patterns/](repo-patterns/))
   - How to structure individual git repositories
   - Bare repository + worktrees for multi-branch workflows
   - Migration guides and real examples

---

## Contents

### Documentation

- **[SPEC.md](SPEC.md)** - Complete specification: what devlog is, what problems it solves, and how it works
- **[ARCHITECTURE.md](ARCHITECTURE.md)** - System architecture: components, navigation, design patterns, and quality mechanisms
- **[.docs/adr/](.docs/adr/)** - Architecture Decision Records: why devlog is designed the way it is

### Guides

- **[session-artifact-tracking.md](session-artifact-tracking.md)** - How to preserve valuable session artifacts instead of losing them in /tmp

### Workspace Patterns

- **[workspace-patterns/](workspace-patterns/)** - Workspace organization patterns, templates, and examples
  - Documentation of common workspace patterns (mono-repo, multi-workspace, etc.)
  - Reusable AGENTS.md and README.md templates
  - Real-world examples from oss/, acme/, acme-app/

### Repository Patterns

- **[repo-patterns/](repo-patterns/)** - Repository structure patterns for multi-branch workflows
  - Bare repository + git worktrees pattern (recommended default)
  - Complete setup guide for new and existing repositories
  - Real migration examples from 9 successfully migrated repos
  - Integration with git-worktrees plugin for temporary isolation

---

## Future Additions (Planned)

As devlog matures, we plan to add:

### Methodologies
- **Multi-persona review templates** - How to run quality reviews with multiple perspective personas
- **Validation methodology** - Three-tier validation patterns (manual + automated + review)
- **Gap analysis patterns** - How to identify missing high-value content

### Debugging
- **Common debugging patterns** - When to debug, how to debug effectively
- **Debug script templates** - How to write reusable debug scripts
- **Troubleshooting guides** - Common issues and solutions

### Research
- **Tier classification** - How to classify research by priority (TIER1/TIER2/TIER3)
- **Trusted sources** - Curated list of authoritative research sources
- **Synthesis patterns** - How to synthesize findings from multiple sources

### Archiving
- **Archival criteria** - When to archive vs delete content
- **Restoration criteria** - When to restore archived content
- **Batch processing** - How to batch-archive engrams

**Note**: These will be extracted from successful sessions after workspace-patterns is complete.

---

## Purpose

Devlog exists to:
1. **Capture learnings** from AI-assisted development sessions
2. **Document patterns** that work well across projects
3. **Provide templates** for common setups (workspaces, documentation, etc.)
4. **Share best practices** for AI-agent collaboration

---

## Relationship to Other Tools

**agm**: CLI tool for managing Claude Code sessions
- Devlog: Documentation and patterns (what to do)
- agm: Tooling (how to do it)

**engram**: AI agent knowledge management system
- Devlog: General ai-tools best practices
- Engram: Specific implementation and product

---

## Contributing

When you discover a useful pattern or practice:
1. Document it in devlog/
2. Add examples from real usage
3. Create templates if applicable
4. Update this README with new content

---

**Last updated**: 2026-02-11 (Backfill documentation added: SPEC.md, ARCHITECTURE.md, ADRs)
**Location**: {{DEVLOG_ROOT}}/repos/ai-tools/main/devlog/
