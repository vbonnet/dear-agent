# Architecture Decision Records

**Component:** devlog-cli
**Version:** 0.1.0
**Last Updated:** 2026-02-11

---

## Overview

This document consolidates all Architecture Decision Records (ADRs) for devlog-cli. The devlog-cli tool is a command-line application for managing development workspaces using bare git repositories with multiple worktrees.

---

## ADR Index

### Active ADRs

**ADR-001: Bare Repositories Over Standard Repositories**
- **Status:** Accepted
- **Date:** 2025-XX-XX (estimated from initial project)
- **Summary:** Use git bare repositories exclusively for worktree management

**ADR-002: YAML for Configuration Format**
- **Status:** Accepted
- **Date:** 2025-XX-XX
- **Summary:** Use YAML for declarative workspace configuration instead of TOML, JSON, or code-based DSL

**ADR-003: Shell Out to Git CLI**
- **Status:** Accepted
- **Date:** 2025-XX-XX
- **Summary:** Execute git commands via exec.Command instead of using go-git library

**ADR-004: Additive Configuration Merging**
- **Status:** Accepted
- **Date:** 2025-XX-XX
- **Summary:** Local config extends base config additively, cannot remove team resources

**ADR-005: Idempotent Operations by Design**
- **Status:** Accepted
- **Date:** 2025-XX-XX
- **Summary:** All commands are safe to run multiple times without side effects

**ADR-006: Security-First Configuration Validation**
- **Status:** Accepted
- **Date:** 2025-XX-XX
- **Summary:** Comprehensive input validation prevents path traversal, injection, and DoS attacks

**ADR-007: Cobra CLI Framework**
- **Status:** Accepted
- **Date:** 2025-XX-XX
- **Summary:** Use spf13/cobra for CLI structure, commands, and flags

**ADR-008: Interface-Based Git Abstraction**
- **Status:** Accepted
- **Date:** 2025-XX-XX
- **Summary:** Define Repository interface to enable testing and potential alternative implementations

---

## Quick Reference

### Decision Summary Table

| Decision | Chosen Option | Alternative Considered | Status | ADR |
|----------|---------------|------------------------|--------|-----|
| Repository Type | Bare repos | Standard repos | Accepted | ADR-001 |
| Config Format | YAML | TOML, JSON, DSL | Accepted | ADR-002 |
| Git Integration | Shell exec | go-git library | Accepted | ADR-003 |
| Config Merging | Additive | Full override, deep merge | Accepted | ADR-004 |
| Operation Safety | Idempotent | Destructive | Accepted | ADR-005 |
| Security | Validation-first | Trust user input | Accepted | ADR-006 |
| CLI Framework | Cobra | Flag, cli, custom | Accepted | ADR-007 |
| Git Abstraction | Repository interface | Direct git calls | Accepted | ADR-008 |

---

## Decision Categories

### Architecture Patterns

---

## ADR-001: Bare Repositories Over Standard Repositories

**Status:** ✅ Accepted
**Date:** 2025-XX-XX
**Deciders:** Core team

### Context

Git worktrees allow parallel development on multiple branches without switching. Standard repositories have a primary working directory plus attached worktrees, while bare repositories have no primary working directory (all worktrees are equal).

**Options considered:**
1. **Bare repositories** - Clone with `--bare`, all worktrees equal
2. **Standard repositories** - Traditional clone, one worktree is "main"
3. **Hybrid approach** - Support both bare and standard

### Decision

Implement devlog-cli for **bare repositories exclusively** (type: bare).

### Rationale

**Why Bare Repositories:**
- **No privileged worktree:** All worktrees are equal (no confusion about which is "main")
- **Cleaner semantics:** Repository directory contains git metadata only, worktrees are subdirectories
- **Better worktree management:** git worktree add works naturally with bare repos
- **Industry standard:** Bare repos recommended for worktree workflows in git documentation
- **Simpler path logic:** workspace/repo/ (bare metadata), workspace/repo/worktree/ (working directory)

**Why not Standard Repositories:**
- **Confusing layout:** Primary working directory at workspace/repo/, worktrees elsewhere
- **Inconsistent semantics:** One worktree is "special" (the primary checkout)
- **Path confusion:** Where do worktrees go? (must be outside primary working directory)
- **Less common:** Standard repos with worktrees less documented, less familiar

**Why not Hybrid:**
- **Complexity:** Supporting both doubles validation logic, testing surface
- **User confusion:** "Should I use bare or standard?" adds decision burden
- **Recommendation clarity:** Clear guidance is better than flexibility

### Consequences

**Positive:**
- ✅ Clear mental model: repo directory = bare metadata, subdirectories = worktrees
- ✅ All worktrees equal (no "main" vs "others" confusion)
- ✅ Follows git best practices for worktree workflows
- ✅ Simpler implementation (one code path)
- ✅ Easier to explain to users

**Negative:**
- ❌ Bare repos unfamiliar to git beginners (no working directory in repo root)
- ❌ Cannot `cd repo && git status` (must cd into worktree)
- ❌ Existing standard repos require re-cloning as bare

**Mitigations:**
- Document bare repo concept clearly in README
- Provide examples showing bare repo layout
- Include helper commands (devlog status) to inspect state
- Consider future `devlog migrate` to convert standard → bare (out of scope v0.1)

**Risks:**
- Low: Users can learn bare repos quickly, mental model is simpler than standard+worktrees

### Implementation Notes

**Repository Structure:**
```
workspace/
├── .devlog/
│   └── config.yaml
├── my-repo/           # Bare repository (git metadata)
│   ├── HEAD
│   ├── config
│   ├── objects/
│   ├── refs/
│   ├── main/          # Worktree 1 (working directory)
│   │   ├── .git       # Link to bare repo
│   │   └── ...        # Source files
│   └── feature/       # Worktree 2 (working directory)
│       ├── .git
│       └── ...
```

**Clone Command:**
```bash
git clone --bare https://github.com/user/repo.git workspace/repo
```

**Config Type:**
```yaml
repos:
  - name: my-repo
    url: https://github.com/user/repo.git
    type: bare  # Only supported type in v0.1
```

---

## ADR-002: YAML for Configuration Format

**Status:** ✅ Accepted
**Date:** 2025-XX-XX
**Deciders:** Core team

### Context

Workspace configuration must be:
- **Human-readable** - Easy to write and edit manually
- **Machine-parsable** - Reliable parsing in Go
- **Git-diffable** - Changes should produce readable diffs
- **Hierarchical** - Support nested structures (repos, worktrees)

**Options considered:**
1. **YAML** - Indentation-based, widely used in DevOps (Kubernetes, Docker Compose)
2. **TOML** - INI-like syntax, gaining popularity (Cargo, Pipenv)
3. **JSON** - JavaScript-based, strict syntax
4. **HCL** - HashiCorp Configuration Language (Terraform)
5. **Go code** - Programmatic DSL (e.g., Pulumi style)

### Decision

Use **YAML (gopkg.in/yaml.v3 v3.0.1+)** for configuration files.

### Rationale

**Why YAML:**
- **Familiarity:** Widely used in DevOps (Kubernetes, GitHub Actions, Docker Compose)
- **Readability:** Indentation-based, minimal syntax noise
- **Git-diffable:** Changes produce clean diffs (add/remove lines)
- **Comments:** Supports `# comments` for documentation
- **Hierarchy:** Natural nested structures
- **Ecosystem:** Well-supported in Go (yaml.v3)

**Why not TOML:**
- Less familiar to DevOps users
- Nested structures more verbose (repeated section headers)
- Smaller ecosystem in Go

**Why not JSON:**
- No comments (cannot document config inline)
- Trailing commas forbidden (annoying to edit)
- More syntax noise (quotes, braces)
- Less human-friendly

**Why not HCL:**
- Terraform-specific knowledge required
- Less common outside HashiCorp ecosystem
- Overkill for simple configuration

**Why not Go Code:**
- Requires recompilation for config changes
- Barrier to non-programmers (ops, designers)
- Security risk (arbitrary code execution)

### Consequences

**Positive:**
- ✅ Familiar to DevOps engineers (Kubernetes, CI/CD configs)
- ✅ Readable diffs when committed to git
- ✅ Inline comments for documentation
- ✅ Minimal syntax overhead
- ✅ Well-tested parsing library (yaml.v3)

**Negative:**
- ❌ Indentation-sensitive (whitespace errors possible)
- ❌ YAML bomb DoS risk (requires size limits)
- ❌ Type ambiguity (strings vs numbers, true vs "true")

**Mitigations:**
- Validate config after parsing (strict type checks)
- Limit file size to 1MB (prevent YAML bomb DoS)
- Use gopkg.in/yaml.v3 v3.0.1+ (CVE-2022-28948 fix)
- Provide example configs with clear formatting

**Risks:**
- Low: YAML parsing well-understood, libraries mature

### Implementation Notes

**Example Config:**
```yaml
name: my-workspace
description: Development workspace for parallel feature development

repos:
  - name: backend
    url: https://github.com/team/backend.git
    type: bare
    worktrees:
      - name: main
        branch: main
      - name: feature-auth
        branch: feature/auth-system
```

**Security Protections:**
```go
// config/config.go:64-84
const MaxConfigSize = 1 * 1024 * 1024 // 1MB

func Load(path string) (*Config, error) {
    // Check file size before reading (prevent YAML bomb)
    info, err := os.Stat(path)
    if err != nil {
        return nil, err
    }
    if info.Size() > MaxConfigSize {
        return nil, fmt.Errorf("config too large: %d bytes", info.Size())
    }

    // Read and parse
    data, err := os.ReadFile(path)
    if err != nil {
        return nil, err
    }

    var cfg Config
    if err := yaml.Unmarshal(data, &cfg); err != nil {
        return nil, err
    }

    return &cfg, nil
}
```

**YAML Library Version:**
- **gopkg.in/yaml.v3 v3.0.1+** - Fixes CVE-2022-28948 (DoS via malicious YAML)
- Do NOT downgrade below v3.0.1

---

## ADR-003: Shell Out to Git CLI

**Status:** ✅ Accepted
**Date:** 2025-XX-XX
**Deciders:** Core team

### Context

Devlog-cli needs to execute git operations (clone, worktree add, worktree list). Two approaches:
1. **Shell out to git CLI** - Execute `git` commands via os/exec
2. **Use go-git library** - Pure Go git implementation

**Requirements:**
- Reliable git worktree support (create, list)
- Bare repository handling
- Cross-platform compatibility (Linux, macOS, Windows)
- Minimal maintenance burden

**Options considered:**
1. **exec.Command("git", ...)** - Shell out to system git
2. **go-git/go-git** - Pure Go git library
3. **libgit2 bindings** - C library with Go wrapper

### Decision

Use **exec.Command to shell out to git CLI** for all git operations.

### Rationale

**Why Shell Out to Git CLI:**
- **Stability:** Git CLI is battle-tested, stable, ubiquitous
- **Feature completeness:** All git features available (worktree, bare repos, etc.)
- **Zero learning curve:** Standard git commands, no library API to learn
- **Easy debugging:** Can see exact git commands executed (verbose mode)
- **No version lock-in:** Uses whatever git version user has installed
- **Worktree support:** Full support for git worktree commands

**Why not go-git:**
- **Incomplete worktree support:** go-git v5 worktree implementation experimental
- **Bare repo limitations:** Bare repo handling less mature than git CLI
- **Maintenance burden:** Must track go-git updates, API changes
- **Performance:** Pure Go slower than C implementation for large repos
- **Debugging harder:** Library bugs harder to diagnose than CLI errors

**Why not libgit2:**
- **CGo dependency:** Requires C compiler, complicates cross-compilation
- **Platform differences:** libgit2 behavior varies across platforms
- **Complexity:** Wrapping C library adds maintenance burden
- **Worktree support:** Less mature than git CLI

### Consequences

**Positive:**
- ✅ Reliable git operations (battle-tested CLI)
- ✅ Full worktree support (no experimental features)
- ✅ Easy debugging (verbose mode shows exact git commands)
- ✅ Zero maintenance for git implementation
- ✅ Cross-platform (git available everywhere developers work)

**Negative:**
- ❌ Requires git installed (not bundled with devlog-cli)
- ❌ Subprocess overhead (~10-50ms per git command)
- ❌ No cross-compilation benefit (git must be on PATH)
- ❌ Version compatibility risk (different git versions may behave differently)

**Mitigations:**
- Document git requirement clearly (README, error messages)
- Subprocess overhead acceptable for workspace setup operations (not hot path)
- Test with multiple git versions (2.30+)
- Provide helpful error messages when git not found

**Risks:**
- Low: Git CLI extremely stable, available everywhere
- Minimal version compatibility issues (worktree added in git 2.5, 2015)

### Implementation Notes

**Security: Prevent Command Injection**
```go
// WRONG: Vulnerable to injection
cmd := exec.Command("sh", "-c", "git clone " + url + " " + path)

// CORRECT: Separate arguments prevent injection
cmd := exec.Command("git", "clone", "--bare", url, path)
```

**Example Implementation:**
```go
// git/local.go:28-37
func (r *LocalRepository) Clone(url, path string) error {
    // exec.Command with separate args prevents injection
    cmd := exec.Command("git", "clone", "--bare", url, path)

    output, err := cmd.CombinedOutput()
    if err != nil {
        return wrapGitError("clone", path, err, output)
    }

    return nil
}
```

**Error Handling:**
```go
// Capture both stdout and stderr for debugging
output, err := cmd.CombinedOutput()
if err != nil {
    return fmt.Errorf("git clone failed: %w (output: %s)",
        err, strings.TrimSpace(string(output)))
}
```

**Minimum Git Version:**
- Git 2.5+ (worktree command added)
- Recommend Git 2.30+ (latest stable features)

---

## ADR-004: Additive Configuration Merging

**Status:** ✅ Accepted
**Date:** 2025-XX-XX
**Deciders:** Core team

### Context

Devlog supports two config files:
- **config.yaml** - Team-wide configuration (committed to git)
- **config.local.yaml** - Local overrides (git-ignored)

Need to define merge semantics when both files exist.

**Requirements:**
- Team config defines shared repositories
- Local config adds personal repositories or worktrees
- Local config cannot break team setup

**Options considered:**
1. **Additive merge** - Local adds to base, cannot remove
2. **Full override** - Local replaces base entirely
3. **Deep merge** - Recursively merge all fields
4. **Explicit override keys** - local.override_repos vs local.add_repos

### Decision

Use **additive merge strategy** - local config extends base config, cannot remove base resources.

### Rationale

**Why Additive Merge:**
- **Safety:** Cannot accidentally remove team repositories or worktrees
- **Simplicity:** Easy to understand (local adds, never removes)
- **Common use case:** Personal worktrees on top of team structure
- **Predictable:** Merged result is base + local (no surprises)

**Why not Full Override:**
- **Dangerous:** Local config could accidentally remove team repositories
- **Confusing:** Must duplicate team config in local to keep it
- **Error-prone:** One mistake in local config breaks entire workspace

**Why not Deep Merge:**
- **Complex:** Recursive merging rules hard to predict
- **Conflicts:** How to handle conflicting values? (last-write-wins? error?)
- **Debugging:** Merged result hard to reason about

**Why not Explicit Override:**
- **Verbose:** Requires separate sections (override_repos, add_repos)
- **Confusing:** Users must know which section to use
- **Overkill:** Additive merge handles 99% of use cases

### Consequences

**Positive:**
- ✅ Safe: Cannot break team configuration
- ✅ Simple: Local adds, never removes
- ✅ Predictable: Merged result = base + local
- ✅ Common workflow: Add personal worktrees to team repos

**Negative:**
- ❌ Cannot remove team repos via local config (must edit base)
- ❌ Cannot override worktree branches (base branch wins on duplicate)
- ❌ Duplicate worktree names ignored (base wins, local ignored)

**Mitigations:**
- Document merge semantics clearly in README
- Provide examples showing additive merge behavior
- Future: Add `devlog validate` to check for ignored local config entries

**Risks:**
- Low: Additive merge is intuitive, matches user expectations

### Implementation Notes

**Merge Algorithm (config/merge.go:10-87):**
```
1. Metadata: local overrides base
   - name: local.name if set, else base.name
   - description: local.description if set, else base.description

2. Repositories: additive
   - Start with all base repos
   - For repos in both: add local worktrees to base worktrees (skip duplicates)
   - For repos only in local: add entire repo

3. Worktrees: additive within repo
   - Base worktrees preserved
   - Local worktrees added (skip if name exists in base)
```

**Example:**
```yaml
# config.yaml (base)
repos:
  - name: backend
    url: https://github.com/team/backend.git
    worktrees:
      - name: main
        branch: main
      - name: staging
        branch: staging

# config.local.yaml (local)
repos:
  - name: backend
    worktrees:
      - name: my-feature
        branch: feature/my-work
  - name: personal-project
    url: https://github.com/me/project.git
    worktrees:
      - name: main
        branch: main

# Merged result
repos:
  - name: backend
    url: https://github.com/team/backend.git  # From base
    worktrees:
      - name: main        # From base
        branch: main
      - name: staging     # From base
        branch: staging
      - name: my-feature  # From local (added)
        branch: feature/my-work
  - name: personal-project  # From local (new repo)
    url: https://github.com/me/project.git
    worktrees:
      - name: main
        branch: main
```

**Duplicate Handling:**
```yaml
# config.yaml
repos:
  - name: backend
    worktrees:
      - name: main
        branch: main

# config.local.yaml
repos:
  - name: backend
    worktrees:
      - name: main      # Duplicate name
        branch: develop # Different branch

# Merged result
repos:
  - name: backend
    worktrees:
      - name: main
        branch: main  # Base wins, local ignored
```

---

## ADR-005: Idempotent Operations by Design

**Status:** ✅ Accepted
**Date:** 2025-XX-XX
**Deciders:** Core team

### Context

Devlog commands may be run multiple times:
- `devlog sync` after editing config
- `devlog sync` on multiple machines with same config
- `devlog status` for health checks
- `devlog init` accidentally re-run

**Requirements:**
- Safe to run commands repeatedly
- Predictable behavior (same input → same output)
- No data loss or corruption

**Options considered:**
1. **Idempotent by design** - Check state, skip existing resources
2. **Destructive operations** - Always recreate (delete + create)
3. **Interactive prompts** - Ask user before destructive actions
4. **Force flags** - Require --force for destructive operations

### Decision

Make **all operations idempotent by design** - check existing state and skip operations for resources that already exist.

### Rationale

**Why Idempotent:**
- **Safety:** No data loss from accidental re-runs
- **Predictable:** Same config + same workspace state → no changes
- **Automation-friendly:** Safe in scripts, CI/CD, cron jobs
- **Multi-machine:** Same config can sync across machines without conflicts
- **Declarative semantics:** Config describes desired state, sync achieves it

**Why not Destructive:**
- **Data loss risk:** Deleting and recreating loses local work
- **Slow:** Full recreation unnecessary when resources exist
- **Scary:** Users afraid to run commands

**Why not Interactive Prompts:**
- **Automation-hostile:** Cannot use in scripts, CI/CD
- **Annoying:** Interrupts workflow for confirmation
- **Inconsistent:** Behavior changes based on TTY

**Why not Force Flags:**
- **Default dangerous:** Without --force, what is default? (error? skip?)
- **Complexity:** Need --force and non-force paths
- **User burden:** Must remember when to use --force

### Consequences

**Positive:**
- ✅ Safe: Re-running sync never loses data
- ✅ Fast: Skips existing resources (no unnecessary work)
- ✅ Automation-friendly: Safe in scripts, no prompts
- ✅ Declarative: Config = desired state, sync converges to it
- ✅ Multi-machine: Sync on laptop, desktop, CI → same result

**Negative:**
- ❌ Cannot force-recreate repos/worktrees via sync (must delete manually)
- ❌ Stale resources not cleaned up (manual removal required)
- ❌ Cannot detect "drift" (worktree on wrong branch)

**Mitigations:**
- Document idempotent behavior clearly
- Provide `devlog status` to detect drift (branch mismatches)
- Future: `devlog clean` to remove unconfigured resources (out of scope v0.1)
- Future: `devlog reset` to force-recreate resources (out of scope v0.1)

**Risks:**
- Low: Idempotent operations are safer than destructive alternatives

### Implementation Notes

**Sync Command (cmd/devlog/sync.go:28-133):**
```go
// For each repo:
if !gitRepo.Exists() {
    // Clone only if not exists
    git.Clone(url, path)
} else {
    // Skip clone, repo already exists
    out.Progress("Repository already exists, skipping")
}

// For each worktree:
existingWorktrees := git.ListWorktrees()
if !exists(worktree, existingWorktrees) {
    // Create only if not exists
    git.CreateWorktree(name, branch)
} else {
    // Skip creation, worktree already exists
    out.Progress("Worktree already exists, skipping")
}
```

**Init Command (cmd/devlog/init.go:60-63):**
```go
// Check if workspace already initialized
if fileExists(".devlog/config.yaml") && !force {
    return fmt.Errorf("workspace already initialized (use --force to overwrite)")
}
```

**Status Command:**
- Read-only, no side effects
- Inherently idempotent

---

## ADR-006: Security-First Configuration Validation

**Status:** ✅ Accepted
**Date:** 2025-XX-XX
**Deciders:** Core team

### Context

Devlog parses YAML configuration and executes git commands based on user input. Security risks:
- **Path traversal:** Malicious worktree names (`../../etc/passwd`)
- **Command injection:** Malicious URLs in git clone (`https://evil.com; rm -rf /`)
- **YAML bomb DoS:** Malicious YAML with exponential expansion (crashes parser)
- **Arbitrary file access:** Reading sensitive files via config path

**Requirements:**
- Prevent path traversal attacks
- Prevent command injection
- Prevent DoS via malicious config
- Validate all user inputs

**Options considered:**
1. **Validation-first** - Validate all inputs before use
2. **Trust user** - Assume config is safe (user-controlled)
3. **Sandboxing** - Run git commands in restricted environment
4. **Manual review** - Require human approval before operations

### Decision

Implement **comprehensive validation-first approach** with multiple layers of security checks.

### Rationale

**Why Validation-First:**
- **Defense in depth:** Multiple validation layers (parsing, semantic, security)
- **Early failure:** Reject invalid config before any operations
- **Clear errors:** Explain why config is invalid, how to fix
- **Composable:** Validation logic reusable across commands

**Why not Trust User:**
- **Shared configs:** Config may be from untrusted source (public repos)
- **Typos:** Accidental mistakes can cause damage
- **Attack surface:** Malicious actor could commit bad config to team repo

**Why not Sandboxing:**
- **Complex:** OS-level sandboxing (containers, VMs) overkill
- **Platform-specific:** Different sandboxing on Linux, macOS, Windows
- **Overhead:** Adds latency to every git operation

**Why not Manual Review:**
- **Slow:** Human approval blocks automation
- **Inconsistent:** Different reviewers may miss different issues
- **Doesn't scale:** Review burden grows with users

### Consequences

**Positive:**
- ✅ Secure against path traversal (no `../`, `/`, `\` in worktree names)
- ✅ Secure against command injection (exec.Command with separate args)
- ✅ Secure against YAML bombs (1MB file size limit)
- ✅ Clear error messages (explains validation failures)
- ✅ Defense in depth (multiple validation layers)

**Negative:**
- ❌ Stricter than necessary (some valid edge cases rejected)
- ❌ Validation complexity (more code to maintain, test)
- ❌ Error message noise (multiple validation errors at once)

**Mitigations:**
- Document validation rules clearly (README, error messages)
- Provide examples of valid configuration
- Future: `devlog validate` command to check config without executing

**Risks:**
- Low: Over-validation better than under-validation

### Implementation Notes

**Layer 1: File Size Limit (YAML Bomb Prevention)**
```go
// config/config.go:64-84
const MaxConfigSize = 1 * 1024 * 1024 // 1MB

func Load(path string) (*Config, error) {
    info, err := os.Stat(path)
    if err != nil {
        return nil, err
    }

    if info.Size() > MaxConfigSize {
        return nil, fmt.Errorf("config too large: %d bytes", info.Size())
    }

    // Proceed to parse...
}
```

**Layer 2: Path Traversal Prevention**
```go
// config/config.go:168-196
func (w *Worktree) Validate() error {
    // No path separators
    if strings.ContainsAny(w.Name, "/\\") {
        return ErrConfigInvalid
    }

    // No parent directory references
    if strings.Contains(w.Name, "..") {
        return ErrConfigInvalid
    }

    // No null bytes
    if strings.Contains(w.Name, "\x00") {
        return ErrConfigInvalid
    }

    // No absolute paths
    if filepath.IsAbs(w.Name) {
        return ErrConfigInvalid
    }

    // Must be clean filename (no directory components)
    if w.Name != filepath.Base(filepath.Clean(w.Name)) {
        return ErrConfigInvalid
    }

    return nil
}
```

**Layer 3: URL Validation (Command Injection Prevention)**
```go
// config/config.go:214-245
func validateGitURL(gitURL string) error {
    // Length check
    if len(gitURL) > MaxGitURLLength {
        return ErrConfigInvalid
    }

    // HTTPS URLs
    if strings.HasPrefix(gitURL, "https://") {
        parsed, err := url.Parse(gitURL)
        if err != nil || parsed.Host == "" {
            return ErrConfigInvalid
        }
        return nil
    }

    // SSH URLs (git@hostname:path)
    if strings.HasPrefix(gitURL, "git@") {
        // Strict regex validation
        if !sshGitURLPattern.MatchString(gitURL) {
            return ErrConfigInvalid
        }
        return nil
    }

    // Reject all other schemes (file://, git://, etc.)
    return ErrConfigInvalid
}
```

**Layer 4: Command Injection Prevention**
```go
// git/local.go:28-37
func (r *LocalRepository) Clone(url, path string) error {
    // SECURITY: exec.Command with separate arguments prevents injection
    // URL and path are validated before reaching this function

    cmd := exec.Command("git", "clone", "--bare", url, path)
    // NOT: exec.Command("sh", "-c", "git clone " + url + " " + path)

    output, err := cmd.CombinedOutput()
    if err != nil {
        return wrapGitError("clone", path, err, output)
    }

    return nil
}
```

**Security Checklist:**
- ✅ Max file size: 1MB (prevent YAML bomb)
- ✅ Path traversal: No `../`, `/`, `\`, absolute paths
- ✅ Null bytes: Rejected in worktree names
- ✅ URL schemes: Only https:// and git@ allowed
- ✅ URL length: Max 2000 chars
- ✅ Command injection: exec.Command with separate args
- ✅ YAML library: gopkg.in/yaml.v3 v3.0.1+ (CVE-2022-28948 fix)

---

## ADR-007: Cobra CLI Framework

**Status:** ✅ Accepted
**Date:** 2025-XX-XX
**Deciders:** Core team

### Context

Devlog-cli needs command-line interface with:
- Multiple subcommands (init, sync, status)
- Global flags (--config, --verbose, --dry-run)
- Help text generation
- Flag parsing and validation

**Options considered:**
1. **spf13/cobra** - Comprehensive CLI framework (used by kubectl, gh, hugo)
2. **urfave/cli** - Simpler CLI framework
3. **stdlib flag** - Go standard library flag package
4. **Custom implementation** - Roll our own CLI parser

### Decision

Use **spf13/cobra v1.8.1** for CLI structure, commands, and flags.

### Rationale

**Why Cobra:**
- **Industry standard:** Used by kubectl, GitHub CLI, Hugo, Docker CLI
- **Comprehensive:** Commands, subcommands, flags, help text, completion
- **Well-documented:** Extensive documentation, many examples
- **Mature:** Battle-tested in production CLIs
- **Conventions:** Follows CLI best practices (help, version, completion)

**Why not urfave/cli:**
- Less comprehensive (no built-in completion, help less polished)
- Smaller ecosystem (fewer examples, less community)
- Different conventions (not as familiar to Kubernetes users)

**Why not stdlib flag:**
- No subcommand support (must implement manually)
- No help text generation
- No flag completion
- More boilerplate for multi-command CLIs

**Why not Custom:**
- **Reinventing wheel:** CLI parsing is solved problem
- **Maintenance burden:** Must maintain parser, help generation, completion
- **Missing features:** Would take months to reach cobra feature parity

### Consequences

**Positive:**
- ✅ Familiar to Kubernetes/Docker users (cobra conventions)
- ✅ Comprehensive help text (`devlog --help`, `devlog sync --help`)
- ✅ Shell completion support (bash, zsh, fish)
- ✅ Flag validation and type safety
- ✅ Subcommand organization (init, sync, status)
- ✅ Well-documented, many examples

**Negative:**
- ❌ Dependency on external library (cobra + pflag)
- ❌ Learning curve for contributors (cobra API)
- ❌ Slightly verbose boilerplate

**Mitigations:**
- Cobra is stable, widely-used dependency (low risk)
- Document cobra patterns in CONTRIBUTING.md
- Keep command implementations simple (minimal cobra API surface)

**Risks:**
- Low: Cobra extremely stable, unlikely to break compatibility

### Implementation Notes

**Root Command (cmd/devlog/root.go):**
```go
var rootCmd = &cobra.Command{
    Use:   "devlog",
    Short: "Manage devlog workspaces with bare repos and worktrees",
    Long: `devlog is a CLI tool for managing development workspaces...`,
}

func Execute() error {
    return rootCmd.Execute()
}

func init() {
    // Global flags
    rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file")
    rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "verbose output")
    rootCmd.PersistentFlags().BoolVar(&dryRun, "dry-run", false, "show what would happen")
}
```

**Subcommand (cmd/devlog/sync.go):**
```go
var syncCmd = &cobra.Command{
    Use:   "sync",
    Short: "Clone repositories and create worktrees from config",
    Long: `Sync reads the workspace configuration and:
  1. Clones missing bare repositories
  2. Creates configured worktrees
  3. Checks out specified branches`,
    RunE: runSync,
}

func runSync(cmd *cobra.Command, args []string) error {
    // Implementation here
    return nil
}

func init() {
    rootCmd.AddCommand(syncCmd)
}
```

---

## ADR-008: Interface-Based Git Abstraction

**Status:** ✅ Accepted
**Date:** 2025-XX-XX
**Deciders:** Core team

### Context

Devlog needs to execute git operations. Two approaches:
1. **Direct git calls** - Call exec.Command("git", ...) directly in commands
2. **Interface abstraction** - Define Repository interface, implement with git CLI

**Requirements:**
- Testable (mock git operations in tests)
- Maintainable (isolate git logic from command logic)
- Potential for alternative implementations (future: libgit2, go-git)

**Options considered:**
1. **Repository interface** - Define interface, implement with LocalRepository
2. **Direct git calls** - No abstraction, call git directly in commands
3. **Test wrapper** - Only abstract for testing, not production

### Decision

Define **Repository interface** in `internal/git/git.go`, implement with `LocalRepository` (git CLI).

### Rationale

**Why Repository Interface:**
- **Testable:** Mock Repository in unit tests (no git CLI required)
- **Separation of concerns:** Commands use Repository interface, don't know about git CLI
- **Future-proof:** Can swap implementations (libgit2, go-git) without changing commands
- **Clean API:** Repository interface documents git operations clearly

**Why not Direct Git Calls:**
- **Hard to test:** Must execute real git commands in tests (slow, brittle)
- **Coupling:** Command logic tightly coupled to git CLI implementation
- **Inflexible:** Cannot swap git implementation without rewriting commands

**Why not Test Wrapper:**
- **Inconsistent:** Different code paths for testing vs production
- **Leaky abstraction:** Production code still couples to git CLI
- **Limited benefit:** Abstraction is cheap, why not use everywhere?

### Consequences

**Positive:**
- ✅ Testable: Mock Repository in unit tests
- ✅ Clean separation: Commands → Repository interface → LocalRepository → git CLI
- ✅ Documented API: Repository interface is contract
- ✅ Future-proof: Can add alternative implementations

**Negative:**
- ❌ Indirection: One extra layer (interface → implementation)
- ❌ Slight verbosity: Define interface + implementation

**Mitigations:**
- Indirection is minimal (single function call)
- Interface documents git operations clearly (good for onboarding)

**Risks:**
- Low: Interface pattern is standard Go practice

### Implementation Notes

**Interface Definition (git/git.go:9-30):**
```go
type Repository interface {
    Clone(url, path string) error
    CreateWorktree(name, branch string) error
    ListWorktrees() ([]WorktreeInfo, error)
    GetCurrentBranch(worktree string) (string, error)
    Exists() bool
}

type WorktreeInfo struct {
    Name   string
    Path   string
    Branch string
    Commit string
}
```

**Implementation (git/local.go:14-24):**
```go
type LocalRepository struct {
    Path string // Path to bare repository
}

func NewLocalRepository(path string) *LocalRepository {
    return &LocalRepository{Path: path}
}

// Implements Repository interface
func (r *LocalRepository) Clone(url, path string) error { ... }
func (r *LocalRepository) CreateWorktree(name, branch string) error { ... }
func (r *LocalRepository) ListWorktrees() ([]WorktreeInfo, error) { ... }
func (r *LocalRepository) GetCurrentBranch(worktree string) (string, error) { ... }
func (r *LocalRepository) Exists() bool { ... }
```

**Usage in Commands (cmd/devlog/sync.go:50-67):**
```go
for _, repo := range ws.Config.Repos {
    repoPath := ws.GetRepoPath(&repo)
    gitRepo := git.NewLocalRepository(repoPath)  // Interface type

    if !gitRepo.Exists() {
        gitRepo.Clone(repo.URL, repoPath)
    }

    for _, wt := range repo.Worktrees {
        gitRepo.CreateWorktree(wt.Name, wt.Branch)
    }
}
```

**Mock for Testing (git/mock.go, future):**
```go
type MockRepository struct {
    CloneCalled bool
    CloneError error
    // ... other fields
}

func (m *MockRepository) Clone(url, path string) error {
    m.CloneCalled = true
    return m.CloneError
}

// Test usage:
mockRepo := &MockRepository{CloneError: nil}
// Pass mockRepo to command logic, verify calls
```

---

## Future ADR Topics

**Topics to document in future ADRs:**
- ADR-009: `devlog clean` command design (remove unconfigured resources)
- ADR-010: `devlog update` command design (pull latest changes in worktrees)
- ADR-011: Shell completion implementation approach
- ADR-012: Config validation command (`devlog validate`)
- ADR-013: Migration from standard repos to bare repos
- ADR-014: Support for non-bare repositories (if needed)

---

**Document Status:** Complete
**Last Reviewed:** 2026-02-11
**Next Review:** Quarterly or when adding major features
