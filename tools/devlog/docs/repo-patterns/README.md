# Repository Patterns Documentation

**Purpose**: Guide developers in organizing individual git repositories for efficient multi-branch workflows.

**Problem solved**: Working on multiple branches simultaneously without branch switching, stashing, or build disruption.

---

## What Are Repository Patterns?

Repository patterns define how to structure individual git repositories in `{{DEVLOG_ROOT}}/repos/`. Unlike [workspace patterns](../workspace-patterns/) which organize multi-project workspaces in `{{DEVLOG_ROOT}}/ws/`, repository patterns focus on the internal structure of a single repository.

**Key pattern documented**: Bare Repository + Worktrees

---

## Quick Start

### I Want to Work on Multiple Branches Simultaneously

**Start here**: [bare-repo-guide.md](bare-repo-guide.md)

Learn how to use bare repositories with git worktrees to work on multiple branches at once without switching, stashing, or build disruption.

**Time**: 15-20 minutes to read, 5 minutes to set up

### I Have Real-World Migration Examples

**Start here**: [examples.md](examples.md)

See actual migrations from standard `.git` structure to bare repo pattern, with before/after comparisons and lessons learned.

**Time**: 10-15 minutes

### I Want to Know the Difference from Git-Worktrees Plugin

**Quick answer**:

- **Bare repo pattern** (this guide): Permanent repository structure in `{{DEVLOG_ROOT}}/repos/`
- **[Git-worktrees plugin](../../../engram/main/plugins/git-worktrees/)**: Temporary worktrees in `~/worktrees/` for isolated experiments

Both can coexist. Use bare repo as your permanent structure, create temporary worktrees via plugin for multi-agent workflows.

---

## Documentation Files

### Core Documentation

#### [bare-repo-guide.md](bare-repo-guide.md)
**~500 lines** | **Comprehensive reference**

Complete guide to bare repository + worktrees pattern.

**Use when**:
- Setting up new repository
- Migrating existing repository
- Learning git worktree workflows
- Troubleshooting worktree issues

**Contents**:
- What is the bare repo pattern and why use it
- Directory structure and benefits
- Setup for new and existing repositories
- Common operations (create, list, remove worktrees)
- Integration with git-worktrees plugin
- Edge cases (IDE support, builds, dependencies)
- Troubleshooting guide
- Migration conceptual approach

#### [examples.md](examples.md)
**~300 lines** | **Real-world migrations**

Actual migration examples from 9 repositories.

**Use when**:
- Want to see pattern in practice
- Planning your own migration
- Comparing your setup to working examples
- Understanding different migration scenarios

**Contents**:
- engram: Rename migration (base/ → .bare/)
- acme-app: Convert migration (base/.git → .bare/, 3 worktrees)
- acme-infra: Transform migration (standard .git → .bare/)
- grouper, aegis, beads: Additional examples
- Before/after comparisons
- Lessons learned

---

## Repository Pattern Quick Reference

### When to Use Bare Repo Pattern

| Use When | Don't Use When |
|----------|----------------|
| Work on multiple branches simultaneously | Only work on one branch at a time |
| Long-running feature branches | Quick single-commit fixes |
| Need to test across branches | Simple linear workflow |
| Build artifacts interfere with switching | No build/compile step |
| Team uses multi-branch workflow | Solo developer, simple projects |

### Bare Repo Pattern Structure

```
repo/
├── .bare/              # Bare repository (all git internals)
│   ├── HEAD
│   ├── config
│   ├── objects/
│   ├── refs/
│   └── worktrees/
├── main/               # Primary worktree (main/master branch)
├── feature-x/          # Worktree for feature-x branch
└── hotfix-y/           # Worktree for hotfix-y branch
```

### Common Operations

```bash
# Create worktree for new branch
git -C ~/repos/myrepo/.bare worktree add ~/repos/myrepo/feature-x feature-x

# List all worktrees
git -C ~/repos/myrepo/.bare worktree list

# Remove worktree
git -C ~/repos/myrepo/.bare worktree remove ~/repos/myrepo/feature-x

# Cleanup stale worktrees
git -C ~/repos/myrepo/.bare worktree prune
```

---

## Relationship to Other Documentation

### Workspace Patterns vs Repository Patterns

**[Workspace Patterns](../workspace-patterns/)** - How to organize `{{DEVLOG_ROOT}}/ws/` with multiple projects
- Mono-Repo, Multi-Workspace, Sub-Workspace patterns
- Where to put wayfinder projects, research, docs
- AGENTS.md and README.md templates

**Repository Patterns** (this guide) - How to structure individual git repositories
- Bare repo + worktrees for multi-branch workflows
- Internal repository organization
- Git workflow optimization

**Both work together**: Use workspace patterns for `{{DEVLOG_ROOT}}/ws/` organization, repository patterns for `{{DEVLOG_ROOT}}/repos/` structure.

### Git-Worktrees Plugin vs Bare Repo Pattern

**[Git-Worktrees Plugin](../../../engram/main/plugins/git-worktrees/)** - Temporary worktrees for isolated work
- Create temporary worktrees in `~/worktrees/`
- Multi-agent workflow isolation
- Experimental branches
- Short-lived feature work

**Bare Repo Pattern** (this guide) - Permanent repository structure
- All work done in worktrees
- Replaces standard `.git` structure
- Long-term multi-branch workflow
- Primary development pattern

**Complementary**: Use bare repo as your permanent structure, create additional temporary worktrees via plugin when needed.

---

## When to Use This Guide

### Use this guide when:

✅ **Working on multiple branches** → bare-repo-guide.md
✅ **Branch switching disrupts workflow** → bare-repo-guide.md
✅ **Build artifacts interfere with git operations** → bare-repo-guide.md
✅ **Planning repository migration** → examples.md
✅ **Setting up new repository** → bare-repo-guide.md setup section
✅ **Confused about worktree vs workspace** → This README relationship sections

### Don't use this guide when:

❌ **Organizing {{DEVLOG_ROOT}}/ws/ workspace** → Use [workspace-patterns](../workspace-patterns/) instead
❌ **Need temporary isolated worktrees** → Use [git-worktrees plugin](../../../engram/main/plugins/git-worktrees/)
❌ **Simple single-branch workflow** → Standard `.git` structure is fine
❌ **Repository is read-only/archive** → No need for worktrees

---

## Navigation Flowchart

```
START
  ↓
Do you work on multiple branches simultaneously?
  ↓                           ↓
 YES                         NO
  ↓                           ↓
bare-repo-guide.md      Standard .git is fine
(multi-branch workflow)
  ↓
Want to see real examples?
  ↓                           ↓
 YES                         NO
  ↓                           ↓
examples.md             Start using pattern
(migration examples)
```

---

## Contributing

### Improving Documentation

If you discover issues or have suggestions:

1. Update bare-repo-guide.md with clarifications
2. Add real examples to examples.md
3. Update this README.md with new use cases

### Reporting Issues

File beads for:
- Unclear documentation
- Missing edge cases
- Broken cross-references
- New patterns to document

---

## Project Context

**Created**: 2025-12-19
**Wayfinder project**: `{{DEVLOG_ROOT}}/ws/oss/projects/devlog-bare-repo-docs/`
**Wayfinder session**: df6bf13c-1745-4958-b53f-d96573a0240c
**Based on**: Successful migration of 9 repositories to bare repo pattern

**Repositories migrated**:
- engram, acme-app, acme-infra, grouper
- aegis, ai-tools, beads, velvet, vpaste

**Migration script**: `{{DEVLOG_ROOT}}/ws/oss/projects/bare-repo-migration/migrate-to-bare.sh` (not included in devlog per user request)

---

**Last updated**: 2025-12-19
**Version**: 1.0
**Maintainer**: See wayfinder project retrospective
