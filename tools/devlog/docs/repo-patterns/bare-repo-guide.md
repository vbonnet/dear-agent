# Bare Repository + Worktrees Pattern

Comprehensive guide to organizing git repositories with bare repos and worktrees for efficient multi-branch workflows.

---

## Introduction

### What Problem Does This Solve?

Traditional git repositories (`.git` directory) allow only one branch to be checked out at a time. Switching branches:
- Disrupts running builds
- Requires stashing uncommitted changes
- Loses context when switching back
- Makes parallel testing difficult

**Bare repository + worktrees pattern** solves this by:
- Allowing multiple branches checked out simultaneously
- Each branch in its own directory (worktree)
- No branch switching needed
- Complete isolation between branches

### Multi-Branch Workflow Benefits

1. **Parallel Development**: Work on feature-a while building feature-b
2. **No Stashing**: Each worktree has its own working directory
3. **Build Isolation**: Compile different branches simultaneously
4. **Easy Comparison**: Have main and feature side-by-side
5. **Context Preservation**: Switch between tasks without losing state

---

## The Pattern

### Structure

```
repo/
├── .bare/                  # Bare repository (all git internals)
│   ├── HEAD
│   ├── config
│   ├── hooks/
│   ├── objects/
│   ├── refs/
│   └── worktrees/          # Worktree metadata
├── main/                   # Worktree for main branch
│   ├── src/
│   ├── tests/
│   └── README.md
├── feature-x/              # Worktree for feature-x branch
│   ├── src/
│   ├── tests/
│   └── README.md
└── hotfix-y/               # Worktree for hotfix-y branch
    ├── src/
    ├── tests/
    └── README.md
```

### Why `.bare/` Subdirectory?

**Community Standard** (2025): Most git worktree guides recommend `.bare/` subdirectory rather than bare repo at root.

**Benefits**:
- **Clear separation**: Git metadata isolated from working directories
- **No mixing**: Bare repo files (HEAD, config, etc.) don't clutter listings
- **Explicit**: Immediately obvious where git internals live
- **Predictable**: All worktrees are siblings at same level

**Alternative** (not recommended): Bare repo at root with worktrees as subdirectories
- Mixes git files with worktree directories in `ls` output
- Less clear what's git metadata vs working directory

---

## Benefits

### 1. Work on Multiple Branches Simultaneously

```bash
# Terminal 1: Work on feature
cd ~/repos/myapp/feature-oauth
npm run dev

# Terminal 2: Run tests on main
cd ~/repos/myapp/main
npm test

# Terminal 3: Build hotfix
cd ~/repos/myapp/hotfix-security
npm run build
```

All happening at once, no interference.

### 2. No Branch Switching Disruption

**Standard .git**:
```bash
# Working on feature
git checkout feature
# Make changes, start build
npm run build
# Need to check something on main
git checkout main  # ❌ Disrupts build, loses context
```

**Bare repo + worktrees**:
```bash
# Just open different directory
cd ~/repos/myapp/main  # ✅ Feature build continues
```

### 3. No Stashing Needed

**Standard .git**:
```bash
git checkout feature
# Make changes...
git checkout main  # ❌ Error: uncommitted changes
git stash
git checkout main
# Later...
git checkout feature
git stash pop  # Hope nothing conflicts
```

**Bare repo + worktrees**:
```bash
cd ~/repos/myapp/main  # ✅ Feature changes stay in feature/ worktree
```

### 4. Build Isolation

Each worktree has its own:
- `node_modules/` (or equivalent dependencies)
- `build/` or `dist/` directories
- Compilation artifacts
- Test databases/fixtures

**Result**: Build one branch while working on another.

### 5. Parallel Testing

```bash
# Test different branches simultaneously
cd ~/repos/myapp/main && npm test &
cd ~/repos/myapp/feature-a && npm test &
cd ~/repos/myapp/feature-b && npm test &
```

Compare results across branches without switching.

---

## When to Use

### Best For

✅ **Multi-branch development**: Regularly work on 2+ branches
✅ **Long-running features**: Features that take days/weeks
✅ **Parallel work**: Need to switch contexts frequently
✅ **Build-heavy projects**: Compiling takes significant time
✅ **Testing across branches**: Compare behavior between versions
✅ **Team workflows**: Multiple developers, many active branches

### Not Ideal For

❌ **Simple workflows**: Only work on one branch at a time
❌ **Quick fixes**: Single-commit changes, merge immediately
❌ **Read-only repos**: Archives, reference code
❌ **Disk-constrained systems**: Each worktree uses space

### Decision Criteria

**Use bare repo pattern if**:
- You answer "yes" to: "Do I regularly need multiple branches checked out?"
- Branch switching disrupts your workflow
- Build artifacts interfere with git operations

**Stick with standard .git if**:
- Linear workflow (main → feature → merge → repeat)
- Rarely switch branches
- Disk space is very limited

---

## Setup

### New Repository

```bash
# Create repository directory
mkdir ~/repos/myrepo
cd ~/repos/myrepo

# Initialize bare repository in .bare/
git init --bare .bare

# Configure remote fetch (required for bare repos)
git -C .bare config remote.origin.fetch '+refs/heads/*:refs/remotes/origin/*'

# Add remote
git -C .bare remote add origin git@github.com:user/repo.git

# Fetch from remote
git -C .bare fetch

# Create main worktree
git -C .bare worktree add ~/repos/myrepo/main main
```

**Result**:
```
myrepo/
├── .bare/    # Bare repository
└── main/     # Main branch worktree
```

### Existing Repository (Standard .git → .bare/)

**⚠️ Safety First**: Backup before migration

```bash
# Backup
cp -r ~/repos/myrepo ~/repos/myrepo.backup

# Clone .git as bare repository
git clone --bare ~/repos/myrepo/.git ~/repos/myrepo/.bare

# Configure remote fetch
git -C ~/repos/myrepo/.bare config remote.origin.fetch '+refs/heads/*:refs/remotes/origin/*'

# Get current branch
cd ~/repos/myrepo
CURRENT_BRANCH=$(git rev-parse --abbrev-ref HEAD)

# Remove old .git
rm -rf ~/repos/myrepo/.git

# Move files to temporary location
mkdir ~/repos/myrepo/.migration-temp
mv ~/repos/myrepo/* ~/repos/myrepo/.migration-temp/  # Excludes .bare

# Create worktree for current branch
git -C ~/repos/myrepo/.bare worktree add ~/repos/myrepo/main $CURRENT_BRANCH

# Move files back
mv ~/repos/myrepo/.migration-temp/* ~/repos/myrepo/main/
rmdir ~/repos/myrepo/.migration-temp
```

**Result**:
```
myrepo/
├── .bare/    # New bare repository
└── main/     # Working files moved here
```

### Existing Repository (With Worktrees: base/ → .bare/)

If you already have a bare repo in `base/` directory:

```bash
# Simple rename
cd ~/repos/myrepo
mv base .bare
```

**Done!** Worktrees continue working automatically.

---

## Common Operations

### Create Worktree for New Branch

```bash
# Create and checkout new branch in new worktree
git -C ~/repos/myrepo/.bare worktree add ~/repos/myrepo/feature-x feature-x
```

**Result**: New directory `~/repos/myrepo/feature-x/` with new branch checked out.

### Create Worktree for Existing Branch

```bash
# Checkout existing branch in new worktree
git -C ~/repos/myrepo/.bare worktree add ~/repos/myrepo/hotfix-y hotfix-y
```

### Create Worktree from Remote Branch

```bash
# Fetch first
git -C ~/repos/myrepo/.bare fetch

# Create worktree tracking remote branch
git -C ~/repos/myrepo/.bare worktree add ~/repos/myrepo/feature-z origin/feature-z
```

### List All Worktrees

```bash
git -C ~/repos/myrepo/.bare worktree list
```

**Output**:
```
~/repos/myrepo/.bare     (bare)
~/repos/myrepo/main      abc123 [main]
~/repos/myrepo/feature-x def456 [feature-x]
```

### Remove Worktree

```bash
# Remove worktree
git -C ~/repos/myrepo/.bare worktree remove ~/repos/myrepo/feature-x

# Optional: Delete branch
git -C ~/repos/myrepo/.bare branch -d feature-x
```

**Note**: Directory must be clean (no uncommitted changes).

### Force Remove Worktree

```bash
# If worktree has uncommitted changes or is corrupted
git -C ~/repos/myrepo/.bare worktree remove --force ~/repos/myrepo/feature-x
```

### Cleanup Stale Worktrees

If you deleted worktree directory manually (don't do this!), cleanup metadata:

```bash
git -C ~/repos/myrepo/.bare worktree prune
```

### Move Worktree

```bash
# Can't move directly, must recreate
git -C ~/repos/myrepo/.bare worktree remove ~/repos/myrepo/old-location
git -C ~/repos/myrepo/.bare worktree add ~/repos/myrepo/new-location branch-name
```

---

## Integration with Git-Worktrees Plugin

### Two Different Patterns

**Bare Repo Pattern** (this guide):
- **Purpose**: Permanent repository structure
- **Location**: `{{DEVLOG_ROOT}}/repos/{repo}/.bare/` + `{{DEVLOG_ROOT}}/repos/{repo}/{branch}/`
- **Use case**: All your regular development work
- **Lifetime**: Permanent (worktrees last weeks/months)

**[Git-Worktrees Plugin](../../../engram/main/plugins/git-worktrees/)**:
- **Purpose**: Temporary isolated worktrees
- **Location**: `~/worktrees/{project}-{context}`
- **Use case**: Multi-agent workflows, experiments, isolated fixes
- **Lifetime**: Temporary (hours/days, cleanup after use)

### Using Both Together

**Example workflow**:

```bash
# Your permanent structure (bare repo pattern)
repos/myapp/
├── .bare/
├── main/      # Primary development
└── feature/   # Long-running feature

# Temporary worktree for quick fix (git-worktrees plugin)
worktrees/myapp-hotfix-security/  # Created, used, removed
```

**When to use each**:
- **Bare repo worktrees**: Regular branches you work on repeatedly
- **Plugin temporary worktrees**: One-off fixes, multi-agent isolation, experiments

**Both use same bare repo**:
```bash
# Plugin can create temporary worktrees from your bare repo
git -C ~/repos/myapp/.bare worktree add ~/worktrees/myapp-experiment experiment
```

### Cross-Reference

For temporary worktree workflows, see [Git-Worktrees Plugin Guide](../../../engram/main/plugins/git-worktrees/quick-reference.ai.md).

---

## Edge Cases

### IDE Support

#### VS Code
**Status**: ✅ Full support (July 2025+)

**Setup**: Open each worktree as separate VS Code window
```bash
code ~/repos/myapp/main
code ~/repos/myapp/feature-x
```

**Git integration**: Works per-worktree automatically.

#### JetBrains IDEs (IntelliJ, WebStorm, etc.)
**Status**: ✅ Supported

**Setup**: Each worktree is a separate project
- Open `~/repos/myapp/main` in one window
- Open `~/repos/myapp/feature-x` in another window

#### Vim/Neovim
**Status**: ✅ Fully supported

Just `cd` to worktree directory and open files normally.

### Build Artifacts

**Issue**: Each worktree creates own `build/`, `dist/`, etc.

**Solutions**:

1. **Separate builds** (recommended):
   - Let each worktree have its own build artifacts
   - Pros: Complete isolation, parallel builds
   - Cons: Uses more disk space

2. **Shared build directory**:
   - Configure build tool to use shared cache
   - Pros: Saves space
   - Cons: Can conflict if building different branches simultaneously

**Examples**:
```bash
# Maven: Shared local repository
<localRepository>~/.m2/repository</localRepository>

# Gradle: Shared daemon and cache
gradle.user.home=~/.gradle

# npm: Shared cache
npm config set cache ~/.npm
```

### Dependencies (node_modules, vendor/, etc.)

**Issue**: Should each worktree have its own dependencies?

**Recommendation**: **Yes, separate dependencies per worktree**

**Why**:
- Different branches may have different dependency versions
- Avoids conflicts
- Each worktree is self-contained

**Disk space concern**:
- Use package manager caching (npm cache, Bundler cache, etc.)
- Shared cache saves space while keeping dependencies isolated

**Anti-pattern**: Symlinking dependencies
```bash
# ❌ Don't do this
ln -s ~/repos/myapp/main/node_modules ~/repos/myapp/feature/node_modules
```
Causes version conflicts when branches diverge.

### Git Hooks

**Behavior**: Hooks in `.bare/hooks/` apply to **all** worktrees

**Example**:
```bash
# pre-commit hook in .bare/hooks/pre-commit
#!/bin/bash
npm test
```

Runs for commits in any worktree.

**This is usually desired**:
- Consistent validation across all branches
- No need to duplicate hooks

**If you need per-worktree hooks**:
- Check `$GIT_DIR` environment variable in hook script
- Conditionally run based on worktree

### Large Repositories

**Concern**: Does N worktrees = N times the disk space?

**Answer**: No, git is efficient

**How it works**:
- `.bare/objects/` stores all git objects (shared)
- Each worktree only stores working files
- Disk usage ≈ `.bare size` + `N × working tree size`

**Example**:
```
.bare/: 500 MB (all git history)
main/: 100 MB (working files)
feature-x/: 105 MB (working files + 5MB new files)
Total: ~705 MB

vs. standard .git approach:
.git/: 500 MB + working files: 100 MB = 600 MB
```

Only ~100 MB overhead for additional worktree.

---

## Troubleshooting

### "fatal: 'branch-name' is already checked out"

**Cause**: Trying to create worktree for branch already checked out elsewhere

**Solution**: List worktrees to find where branch is checked out
```bash
git -C ~/repos/myrepo/.bare worktree list
```

Remove existing worktree or use different branch name.

### "Cannot create worktree"

**Possible causes**:
1. Directory already exists
2. Branch doesn't exist
3. Insufficient permissions

**Debug**:
```bash
# Check if directory exists
ls ~/repos/myrepo/feature-x

# Check if branch exists
git -C ~/repos/myrepo/.bare branch -a | grep feature-x

# Check permissions
ls -ld ~/repos/myrepo/
```

### Stale Worktree Metadata

**Symptom**: `git worktree list` shows worktree that no longer exists

**Cause**: Worktree directory deleted manually (without `git worktree remove`)

**Solution**:
```bash
# Cleanup stale worktree records
git -C ~/repos/myrepo/.bare worktree prune
```

### Fetch Doesn't Get Remote Branches

**Symptom**: `git fetch` in bare repo doesn't create remote-tracking branches

**Cause**: Bare repos need explicit fetch configuration

**Solution**:
```bash
git -C ~/repos/myrepo/.bare config remote.origin.fetch '+refs/heads/*:refs/remotes/origin/*'
git -C ~/repos/myrepo/.bare fetch
```

### Can't Push from Worktree

**Symptom**: Push fails from worktree directory

**Cause**: Git commands in worktree need to know where bare repo is (usually automatic)

**Solution**: Use git normally from within worktree
```bash
cd ~/repos/myrepo/feature-x
git push origin feature-x
```

Should work automatically. If not, check `.git` file in worktree points to correct bare repo.

### Disk Space Issues

**If running low on space**:

1. **Remove unused worktrees**:
   ```bash
   git -C ~/repos/myrepo/.bare worktree remove ~/repos/myrepo/old-feature
   ```

2. **Clean build artifacts**:
   ```bash
   rm -rf ~/repos/myrepo/*/node_modules
   rm -rf ~/repos/myrepo/*/build
   ```

3. **Garbage collect**:
   ```bash
   git -C ~/repos/myrepo/.bare gc --aggressive
   ```

---

## Migration Conceptual Approach

**Note**: Migration script exists at `{{DEVLOG_ROOT}}/ws/oss/projects/bare-repo-migration/migrate-to-bare.sh` but is not included in devlog per user request. Below describes the approach conceptually.

### Three Migration Scenarios

#### 1. Rename (base/ bare repo → .bare/)

**When**: You already have a bare repo in `base/` directory

**Steps**:
```bash
cd ~/repos/myrepo
mv base .bare
```

**That's it!** Existing worktrees continue working.

#### 2. Convert (base/.git standard → .bare/)

**When**: You have standard repo in `base/` with worktrees

**Steps**:
1. Clone `base/.git` as bare to `.bare/`
2. Configure remote fetch
3. Remove `base/` directory
4. Recreate `base/` as worktree
5. Update other worktrees to point to new bare repo

#### 3. Transform (standard .git → .bare/)

**When**: You have standard `.git` repository structure

**Steps**:
1. Backup repository
2. Clone `.git` as bare to `.bare/`
3. Configure remote fetch
4. Move working files to temporary location
5. Remove `.git` directory
6. Create worktree for current branch
7. Move files back into worktree

### Safety Checklist

Before migrating:

- [ ] Commit or stash all changes
- [ ] Push commits to remote
- [ ] Create backup (`cp -r repo repo.backup`)
- [ ] Verify backup is complete
- [ ] Test migration on small repo first

### Post-Migration Verification

After migrating:

```bash
# List worktrees (should show all expected branches)
git -C ~/repos/myrepo/.bare worktree list

# Verify git operations work
cd ~/repos/myrepo/main
git status
git log
git fetch

# Verify you can create new worktree
git -C ~/repos/myrepo/.bare worktree add ~/repos/myrepo/test-branch test-branch
git -C ~/repos/myrepo/.bare worktree remove ~/repos/myrepo/test-branch
```

### Rollback Plan

If migration fails:

```bash
# Remove new structure
rm -rf ~/repos/myrepo

# Restore backup
mv ~/repos/myrepo.backup ~/repos/myrepo
```

---

## Related Documentation

- **[Repository Patterns Overview](README.md)** - Navigation hub for repo patterns
- **[Real Migration Examples](examples.md)** - Before/after from 9 actual repos
- **[Git-Worktrees Plugin](../../../engram/main/plugins/git-worktrees/)** - Temporary worktrees for multi-agent workflows
- **[Workspace Patterns](../workspace-patterns/)** - Organizing {{DEVLOG_ROOT}}/ws/ with multiple projects

---

## Community Resources

- [Git worktrees for fun and profit · Bug Repellent](https://blog.safia.rocks/2025/09/03/git-worktrees/)
- [Sliced bread: git-worktree and bare repo – Andreas Schneider](https://blog.cryptomilk.org/2023/02/10/sliced-bread-git-worktree-and-bare-repo/)
- [How I use git worktrees | Nick Nisi](https://nicknisi.com/posts/git-worktrees/)
- [Multiply your branches in a Git Worktree](https://sylhare.github.io/2025/10/24/Git-worktree.html)
- [Git Worktree Official Documentation](https://git-scm.com/docs/git-worktree)

---

**Last updated**: 2025-12-19
**Version**: 1.0
