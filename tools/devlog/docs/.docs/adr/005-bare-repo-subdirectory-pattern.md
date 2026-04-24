# ADR 005: Bare Repository Subdirectory Pattern (.bare/)

**Status**: Accepted
**Date**: 2025-12-19
**Deciders**: Devlog Maintainers
**Context**: Repository patterns documentation (bare repo + worktrees)

---

## Context

When documenting the bare repository + worktrees pattern for multi-branch workflows, a key decision arose: where should the bare repository live relative to worktrees?

**Options**:
1. Bare repo at root, worktrees as subdirectories
2. Bare repo in subdirectory (`.bare/`), worktrees as siblings
3. Bare repo separate location, worktrees in project directory

**Considerations**:
- Directory listing clarity (`ls` output)
- Community standards and conventions
- Git tooling compatibility
- Developer experience

**Research**:
- Surveyed git worktree guides (2025 best practices)
- Reviewed community recommendations
- Analyzed existing tooling patterns
- Tested developer workflow with each approach

---

## Decision

**Devlog will document bare repository in `.bare/` subdirectory with worktrees as siblings.**

**Recommended Structure**:
```
repo/
├── .bare/              # Bare repository (git internals)
│   ├── HEAD
│   ├── config
│   ├── objects/
│   ├── refs/
│   └── worktrees/
├── main/               # Worktree for main branch
├── feature-x/          # Worktree for feature-x branch
└── hotfix-y/           # Worktree for hotfix-y branch
```

**Not Recommended**:
```
repo/                   # Bare repository at root
├── HEAD                # Git files mixed with worktrees
├── config
├── objects/
├── refs/
├── worktrees/
├── main/               # Worktree (mixed in with git files)
└── feature-x/          # Worktree (mixed in with git files)
```

---

## Rationale

### Clear Separation of Concerns

**With `.bare/` Subdirectory**:
```bash
$ ls repo/
.bare/  main/  feature-x/  hotfix-y/
```

**Immediately Clear**:
- `.bare/` contains git internals
- Other directories are worktrees (working code)
- Clean separation visible in listings

**With Bare Repo at Root**:
```bash
$ ls repo/
HEAD  config  hooks/  objects/  refs/  worktrees/  main/  feature-x/  hotfix-y/
```

**Mixed and Confusing**:
- Git internals mixed with working directories
- Unclear which are worktrees vs. git files
- Cluttered directory listing

### Community Standard (2025)

**Research Findings**:
- Most git worktree guides (2025) recommend `.bare/` subdirectory
- Community consensus shifted from bare-at-root to bare-in-subdirectory
- Modern tutorials emphasize clean separation

**Sources Consulted**:
- Git worktree documentation updates
- Developer blog posts (2024-2025)
- Stack Overflow recommendations (recent)
- GitHub examples and templates

**Consensus**: `.bare/` subdirectory is emerging best practice.

### Developer Experience

**Benefits of `.bare/` Subdirectory**:

1. **Intuitive Structure**:
   - Developers immediately understand layout
   - Clear which directories to work in
   - Obvious where git internals live

2. **Tab Completion**:
   - `cd m<tab>` completes to `main/` (not `mkdir/`)
   - Fewer false completions from git internals
   - Faster navigation

3. **Tooling Compatibility**:
   - IDEs recognize worktree directories
   - File explorers show clean structure
   - Build tools work without confusion

4. **Mental Model**:
   - Repository contains worktrees
   - `.bare/` is "plumbing", worktrees are "porcelain"
   - Matches conceptual model of worktrees

### Reduced Cognitive Load

**With `.bare/` Subdirectory**:
```
repo/
├── .bare/          # "Don't touch, git internals"
├── main/           # "Work here for main branch"
└── feature/        # "Work here for feature branch"
```

**Mental Model**: Simple and clear.

**With Bare Repo at Root**:
```
repo/               # "Is this a worktree or bare repo?"
├── HEAD            # "What's this?"
├── config          # "Should I edit this?"
├── main/           # "Okay, this is a worktree"
└── feature/        # "This too, I guess?"
```

**Mental Model**: Confusing, requires git knowledge.

### Alignment with Git Conventions

**Git Conventions**:
- `.git/` contains internals (hidden directory)
- Working tree contains user files (visible)
- Separation between plumbing and porcelain

**`.bare/` Subdirectory**:
- Follows same principle (.bare/ analogous to .git/)
- Clear separation of internals from working files
- Consistent with git's design philosophy

**Bare at Root**:
- Reverses convention (internals visible, working trees nested)
- Different mental model from standard git
- More cognitive friction

---

## Consequences

### Positive

**Clarity**:
- Immediately obvious directory structure
- Clear separation of git internals from work directories
- Reduced confusion for new users

**Community Alignment**:
- Follows 2025 best practices
- Matches modern tutorials and guides
- Benefits from community learning

**Developer Experience**:
- Cleaner tab completion
- Better IDE recognition
- Intuitive navigation

**Consistency**:
- Analogous to `.git/` pattern
- Aligns with git's separation of concerns
- Predictable structure

**Tooling Compatibility**:
- IDEs work better with worktree siblings
- Build tools don't get confused
- File explorers show clean structure

### Negative

**Extra Directory Level**:
- One more level in git commands
- Must specify `.bare/` in git commands

**Migration Complexity**:
- Existing bare-at-root repos need migration
- Can't just rename .git to .bare
- Requires documented migration process

**Not Universal**:
- Some older guides show bare-at-root
- Users from older tutorials may be confused
- Need clear documentation of choice

### Mitigation Strategies

**For Extra Directory Level**:
- Provide command examples in documentation
- Show how to use `-C` flag: `git -C .bare/ worktree list`
- Include in migration guide templates

**For Migration Complexity**:
- Document migration process clearly
- Provide step-by-step guide in bare-repo-guide.md
- Include troubleshooting section

**For Confusion from Older Guides**:
- Explain rationale in documentation
- Reference community standard shift
- Provide "why .bare/ subdirectory" section

---

## Implementation in Devlog

### Documentation Approach

**In bare-repo-guide.md**:

```markdown
### Structure

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

### Why `.bare/` Subdirectory?

**Community Standard** (2025): Most git worktree guides recommend
`.bare/` subdirectory rather than bare repo at root.

**Benefits**:
- Clear separation: Git metadata isolated from working directories
- No mixing: Bare repo files don't clutter listings
- Explicit: Immediately obvious where git internals live
- Predictable: All worktrees are siblings at same level
```

**Emphasis**: Document as recommended pattern, explain rationale.

### Migration Examples

**All 9 Migration Examples**:
- engram: Used `.bare/` subdirectory
- acme-app: Migrated to `.bare/` subdirectory
- acme-infra: Migrated to `.bare/` subdirectory
- grouper, aegis, ai-tools, beads, velvet, vpaste: All `.bare/` subdirectory

**Consistency**: 100% of real examples use `.bare/` subdirectory.

### Command Examples

**Creating Worktree**:
```bash
git -C ~/repos/myrepo/.bare worktree add ~/repos/myrepo/feature-x feature-x
```

**Listing Worktrees**:
```bash
git -C ~/repos/myrepo/.bare worktree list
```

**Removing Worktree**:
```bash
git -C ~/repos/myrepo/.bare worktree remove ~/repos/myrepo/feature-x
```

**Pattern**: Always use `-C .bare/` flag in documentation examples.

---

## Alternatives Considered

### Alternative 1: Bare Repo at Root

**Structure**:
```
repo/                   # Bare repository
├── HEAD
├── config
├── objects/
├── refs/
├── worktrees/
├── main/               # Worktree
└── feature/            # Worktree
```

**Pros**:
- One less directory level
- Simpler git commands (no `-C` flag needed)
- Some older guides use this approach

**Cons**:
- Git files mixed with worktrees in `ls` output
- Unclear which are worktrees vs git internals
- Cluttered directory structure
- Counter to emerging community standard

**Rejected Because**: Poor directory listing clarity, against community standard.

### Alternative 2: Bare Repo in Separate Location

**Structure**:
```
~/git-bare/myrepo.git   # Bare repository
~/repos/myrepo/
├── main/               # Worktree
└── feature/            # Worktree
```

**Pros**:
- Complete separation of bare repo and worktrees
- Very clean worktree directory
- Bare repos centralized

**Cons**:
- Non-obvious relationship between bare repo and worktrees
- Harder to find bare repo for given worktree
- Path management complexity
- Extra coordination required

**Rejected Because**: Reduces discoverability, adds path complexity.

### Alternative 3: `.git/` Directory (Fake Bare Repo)

**Structure**:
```
repo/
├── .git/               # Bare repository (misleading name)
├── main/               # Worktree
└── feature/            # Worktree
```

**Pros**:
- Familiar `.git/` name
- Hidden directory (dotfile)
- Similar to standard git

**Cons**:
- Misleading: `.git/` typically means standard repo, not bare
- Confusing when worktree contains `.git` file pointing to bare
- Breaks assumption that `.git/` is at repo root

**Rejected Because**: Misleading naming, breaks git conventions.

---

## Migration Path from Bare-at-Root

**For Existing Bare-at-Root Repos**:

Not documented in devlog (existing repos predating this decision), but migration would be:

1. Create `.bare/` subdirectory
2. Move git internals (HEAD, config, objects/, refs/, etc.) into `.bare/`
3. Update worktree metadata
4. Test worktree operations

**Decision**: Don't document this migration (not expected use case).

**Rationale**:
- Bare-at-root not documented pattern in devlog
- Users following devlog will use `.bare/` subdirectory from start
- Migration from bare-at-root is edge case

---

## Related Decisions

**ADR 001**: Documentation-only library (no migration script for bare-at-root)
**ADR 002**: Hub-and-spoke navigation (bare-repo-guide.md documents this pattern)
**ADR 004**: Real examples required (9 examples validate `.bare/` subdirectory)

---

## References

**Community Standards**:
- [Git Worktree Documentation](https://git-scm.com/docs/git-worktree)
- Modern Git Worktree Guides (2024-2025)
- Stack Overflow Best Practices (2025)

**Similar Patterns**:
- `.git/` for standard repositories (hidden internals)
- `node_modules/` for dependencies (separate from source)
- `.venv/` for Python virtual environments (separate from code)

**Migration Examples**:
- 9 repositories migrated to `.bare/` subdirectory
- Zero repositories using bare-at-root
- 100% adoption of `.bare/` pattern

---

## Review History

**2025-12-19**: Initial decision (repository patterns documentation)
**2025-12-19**: Validated (9 migrations all used `.bare/` subdirectory)
**2026-02-11**: Documented in ADR (backfill documentation)

**Next Review**: 2026-05-11

**Success Metrics**:
- 9/9 migrations used `.bare/` subdirectory (100%)
- Developer feedback positive on clarity
- No confusion reported about structure
- Community alignment confirmed
