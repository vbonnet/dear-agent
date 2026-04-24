# devlog-cli Architecture

**Version:** 0.1.0
**Last Updated:** 2026-03-18
**Type:** CLI Tool

---

## Table of Contents

1. [Overview](#overview)
2. [System Context](#system-context)
3. [Component Architecture](#component-architecture)
4. [Command Flow](#command-flow)
5. [Configuration System](#configuration-system)
6. [Git Integration](#git-integration)
7. [Error Handling](#error-handling)
8. [Data Flow](#data-flow)
9. [Extension Points](#extension-points)
10. [Dependencies](#dependencies)

---

## Overview

### Purpose

devlog-cli is a command-line tool that automates the management of development workspaces using bare git repositories with multiple worktrees. It provides declarative configuration, idempotent synchronization, and status checking for multi-repository, multi-worktree environments.

### Architecture Goals

1. **Idempotent Operations** - Safe to run commands multiple times without side effects
2. **Declarative Configuration** - Config describes desired state, tool achieves it
3. **Security First** - Validate all inputs, prevent injection and traversal attacks
4. **Testability** - Interface-based design enables comprehensive testing
5. **Composability** - Small, focused packages with clear responsibilities
6. **Git-Native** - Use standard git commands, no custom git implementation

### Key Design Decisions

**Decision 1: Bare repositories over standard repositories**
- **Rationale:** Git worktrees work best with bare repos (no primary working directory)
- **Tradeoff:** Unfamiliar to users vs better worktree management
- **Alternative considered:** Standard repos (confusing which worktree is "main")

**Decision 2: YAML for configuration**
- **Rationale:** Human-readable, git-diffable, standard in DevOps tools
- **Tradeoff:** Parsing complexity vs readability
- **Alternative considered:** TOML (less common), JSON (harder to read/edit)

**Decision 3: Shell out to git CLI**
- **Rationale:** Git CLI is stable, well-tested, available everywhere
- **Tradeoff:** Subprocess overhead vs maintaining git library bindings
- **Alternative considered:** go-git (complex, incomplete worktree support)

**Decision 4: Additive config merging**
- **Rationale:** Local config extends team config, cannot accidentally break team setup
- **Tradeoff:** Cannot remove team repos via local config vs safety
- **Alternative considered:** Full override (dangerous), deep merge (complex)

---

## System Context

### Ecosystem Position

```
┌──────────────────────────────────────────────────────────────┐
│                   Development Environment                     │
│                                                               │
│  ┌────────────┐     ┌────────────┐     ┌────────────┐      │
│  │   Laptop   │     │  Desktop   │     │  CI/CD     │      │
│  │            │     │            │     │  Server    │      │
│  │ devlog-cli │     │ devlog-cli │     │ devlog-cli │      │
│  └─────┬──────┘     └─────┬──────┘     └─────┬──────┘      │
│        │                  │                  │              │
│        │   shared via     │   shared via     │              │
│        └──────────┬───────┴──────────────────┘              │
│                   │                                          │
│        ┌──────────▼──────────┐                              │
│        │  .devlog/config.yaml │  (committed to git)         │
│        │  .devlog/config.local.yaml │  (local, git-ignored)│
│        └──────────┬──────────┘                              │
│                   │                                          │
└───────────────────┼──────────────────────────────────────────┘
                    │
     ┌──────────────▼──────────────┐
     │      Git CLI (system)        │
     │  - git clone --bare          │
     │  - git worktree add          │
     │  - git worktree list         │
     └──────────────┬───────────────┘
                    │
     ┌──────────────▼──────────────┐
     │    Filesystem                │
     │  workspace/                  │
     │  ├── .devlog/                │
     │  │   └── config.yaml         │
     │  ├── repo1/                  │
     │  │   ├── (bare repo files)   │
     │  │   ├── main/               │
     │  │   └── feature/            │
     │  └── repo2/                  │
     │      └── ...                 │
     └──────────────────────────────┘
```

### User Workflows

**Workflow 1: Initial Setup**
```
Developer → devlog init → .devlog/config.yaml created
         → edit config.yaml (add repos)
         → devlog sync → repos cloned, worktrees created
```

**Workflow 2: Multi-Machine Sync**
```
Machine 1 → devlog init → edit config → git commit config.yaml
Machine 2 → git clone dotfiles → devlog sync → identical workspace
```

**Workflow 3: Status Check**
```
Developer → devlog status → see which repos/worktrees exist
         → devlog sync (if missing resources)
```

---

## Component Architecture

### Package Structure

```
devlog-cli/
├── main.go                    # Entry point (11 lines)
├── cmd/devlog/                # CLI commands (463 lines)
│   ├── root.go                # Root command, global flags (63 lines)
│   ├── init.go                # Initialize workspace (245 lines)
│   ├── sync.go                # Sync repos and worktrees (138 lines)
│   └── status.go              # Show workspace status (145 lines)
├── internal/
│   ├── config/                # Configuration system (569 lines)
│   │   ├── config.go          # Types, validation, loading (246 lines)
│   │   ├── loader.go          # Config discovery and merging (89 lines)
│   │   └── merge.go           # Additive merge logic (88 lines)
│   ├── git/                   # Git operations (213 lines)
│   │   ├── git.go             # Repository interface (39 lines)
│   │   └── local.go           # Git CLI implementation (174 lines)
│   ├── workspace/             # Workspace management (53 lines)
│   │   └── workspace.go       # Workspace loading, path resolution (53 lines)
│   ├── output/                # Output formatting (106 lines)
│   │   └── output.go          # Writer interface, stdout impl (106 lines)
│   └── errors/                # Error types (92 lines)
│       └── errors.go          # Structured errors, exit codes (92 lines)
└── go.mod                     # Module definition

Total: ~1,400 lines of production code (excluding tests)
Test Coverage: 83%
```

### Component Diagram

![C4 Component Diagram](diagrams/rendered/c4-component-devlog.svg)

**Diagram:** [diagrams/c4-component-devlog.d2](diagrams/c4-component-devlog.d2)

The component diagram shows the internal architecture of devlog-cli organized into layers:

**CLI Commands Layer:**
- **Root Command**: Cobra-based command router with global flags (--verbose, --dry-run, --config)
- **Init Command**: Workspace initialization, generates config files and directory structure
- **Sync Command**: Idempotent repository cloning and worktree creation from config
- **Status Command**: Display workspace state, compare config vs actual repos/worktrees

**Core Components Layer:**
- **Workspace Manager**: Discovers workspace root, loads config, resolves repo/worktree paths
- **Config System**: Configuration loading, merging, and validation with three sub-components:
  - **Config Loader**: Discovers .devlog/config.yaml by walking up directory tree
  - **Config Merger**: Additively merges base config with local overrides
  - **Config Validator**: Validates config structure and security (path traversal, URL schemes)

**Git Integration Layer:**
- **Repository Interface**: Defines operations for bare repositories and worktrees
- **Local Repository**: Implements interface via git CLI commands (clone --bare, worktree add/list)

**Support Components:**
- **Output Formatter**: Structured output with success/error/info/progress messages
- **Error Handler**: Wraps errors with operation context and path information

**External Systems:**
- **Git CLI**: System git installation for all repository operations
- **Filesystem**: Stores workspace config (.devlog/), bare repos, and worktrees

**Key Data Flows:**
1. Commands → Workspace Manager → Config System → Load & merge YAML configs
2. Sync Command → Git Local → Git CLI → Clone repos, create worktrees on filesystem
3. Status Command → Git Local → Git CLI → List worktrees, compare with config
4. All operations → Error Handler → Wrap with context, Output Formatter → Display results

---

## Command Flow

### init Command Flow

```
User: devlog init my-workspace [--force]
  │
  ├─> Parse flags (force)
  ├─> Determine workspace name (arg or cwd basename)
  ├─> Check if .devlog/config.yaml exists
  │    ├─> exists && !force → Error: already initialized
  │    └─> exists && force → Continue
  ├─> Create .devlog/ directory (os.MkdirAll)
  ├─> Generate example Config struct
  │    ├─> workspace name
  │    ├─> example repository (example-repo)
  │    └─> example worktrees (main, feature)
  ├─> Marshal to YAML (yaml.Marshal)
  ├─> Write .devlog/config.yaml (with header comment)
  ├─> Write .devlog/config.local.yaml.example (template)
  ├─> Write .devlog/README.md (quick start guide)
  ├─> Write .devlog/.gitignore (ignore config.local.yaml)
  └─> Output success + next steps
```

**Key Functions:**
- `runInit()` (init.go:39-214)
- `createExampleConfig()` (init.go:217-239)
- `output.NewStdoutWriter()` (output.go:38-40)

---

### sync Command Flow

```
User: devlog sync [--dry-run] [--verbose]
  │
  ├─> Load workspace (workspace.LoadWorkspace)
  │    ├─> Find .devlog/config.yaml (walk up tree)
  │    ├─> Load base config (config.Load)
  │    ├─> Load local config if exists (config.Load)
  │    ├─> Merge configs (config.Merge)
  │    └─> Validate merged config (config.Validate)
  │
  ├─> For each repo in config:
  │    ├─> Get repo path (workspace.GetRepoPath)
  │    ├─> Create git.Repository (git.NewLocalRepository)
  │    ├─> Check if repo exists (gitRepo.Exists)
  │    │    ├─> Not exists → Clone bare repo
  │    │    │    ├─> DRY RUN: log would clone
  │    │    │    └─> REAL: exec "git clone --bare <url> <path>"
  │    │    └─> Exists → Skip clone
  │    │
  │    └─> For each worktree in repo:
  │         ├─> List existing worktrees (gitRepo.ListWorktrees)
  │         │    └─> exec "git worktree list --porcelain"
  │         ├─> Check if worktree exists
  │         │    ├─> Not exists → Create worktree
  │         │    │    ├─> DRY RUN: log would create
  │         │    │    └─> REAL: exec "git worktree add <name> <branch>"
  │         │    └─> Exists → Skip creation
  │         └─> Continue to next worktree
  │
  └─> Print summary (repos cloned, worktrees created)
```

**Key Functions:**
- `runSync()` (sync.go:28-133)
- `workspace.LoadWorkspace()` (workspace.go:20-39)
- `git.Clone()` (local.go:28-37)
- `git.CreateWorktree()` (local.go:42-57)

---

### status Command Flow

```
User: devlog status [--verbose]
  │
  ├─> Load workspace (workspace.LoadWorkspace)
  │
  ├─> Print workspace metadata (name, description)
  │
  ├─> For each repo in config:
  │    ├─> Get repo path (workspace.GetRepoPath)
  │    ├─> Check if repo exists (gitRepo.Exists)
  │    │    ├─> Exists → Print "✓ repo-name (cloned)"
  │    │    └─> Not exists → Print "✗ repo-name (not cloned)"
  │    │
  │    ├─> If repo exists:
  │    │    ├─> List actual worktrees (gitRepo.ListWorktrees)
  │    │    ├─> Create map of actual worktrees (by name)
  │    │    ├─> For each configured worktree:
  │    │    │    ├─> Check if in actual map
  │    │    │    │    ├─> Exists → Print "✓ name → branch"
  │    │    │    │    │    └─> Warn if branch mismatch
  │    │    │    │    └─> Not exists → Print "✗ name (not created)"
  │    │    │
  │    │    └─> For each actual worktree not in config:
  │    │         └─> Print "→ name (not in config) → branch"
  │    │
  │    └─> If repo not exists:
  │         └─> For each configured worktree:
  │              └─> Print "- name → branch (pending)"
  │
  └─> Print summary (repos configured/cloned, worktrees configured/created)
```

**Key Functions:**
- `runStatus()` (status.go:27-140)
- `git.ListWorktrees()` (local.go:61-74)
- `git.GetCurrentBranch()` (local.go:79-95)

---

## Configuration System

### Configuration Loading Pipeline

```
1. Discovery Phase (loader.go:findConfigDir)
   ├─> Start from current directory
   ├─> Check for .devlog/config.yaml
   ├─> Not found? Walk up to parent directory
   ├─> Repeat until found or filesystem root
   └─> Error if not found: ErrConfigNotFound

2. Base Config Load (config.go:Load)
   ├─> Check file size (<1MB, prevent YAML bombs)
   ├─> Read file (os.ReadFile)
   ├─> Parse YAML (yaml.Unmarshal)
   └─> Return Config struct

3. Local Config Load (loader.go:LoadMerged)
   ├─> Try load config.local.yaml
   ├─> If not exists → OK, use nil
   ├─> If exists but invalid → Error
   └─> Return local Config or nil

4. Merge Phase (merge.go:Merge)
   ├─> Override metadata (name, description) if local has values
   ├─> For each base repo:
   │    ├─> Copy base worktrees
   │    └─> Add local worktrees (skip duplicates)
   └─> Add repos from local not in base

5. Validation Phase (config.go:Validate)
   ├─> Check required fields (name, repos)
   ├─> Validate each repo (name, URL, type)
   ├─> Validate each worktree (name, branch)
   ├─> Security checks (path traversal, URL schemes)
   └─> Return error if any validation fails
```

### Configuration Validation Rules

**Workspace Level:**
```go
// config.go:101-125
func (c *Config) Validate() error {
    // name required
    if c.Name == "" {
        return ErrConfigInvalid
    }

    // at least one repo required
    if len(c.Repos) == 0 {
        return ErrConfigInvalid
    }

    // repos must have unique names
    seen := make(map[string]bool)
    for _, repo := range c.Repos {
        if seen[repo.Name] {
            return ErrConfigInvalid // duplicate
        }
        seen[repo.Name] = true

        // validate repo
        if err := repo.Validate(); err != nil {
            return err
        }
    }

    return nil
}
```

**Repository Level:**
```go
// config.go:128-165
func (r *Repo) Validate() error {
    // name and URL required
    if r.Name == "" || r.URL == "" {
        return ErrConfigInvalid
    }

    // validate git URL
    if err := validateGitURL(r.URL); err != nil {
        return err
    }

    // type must be "bare" or "standard" if specified
    if r.Type != "" && r.Type != RepoTypeBare && r.Type != RepoTypeStandard {
        return ErrConfigInvalid
    }

    // worktrees must have unique names
    seen := make(map[string]bool)
    for _, wt := range r.Worktrees {
        if seen[wt.Name] {
            return ErrConfigInvalid // duplicate
        }
        seen[wt.Name] = true

        // validate worktree
        if err := wt.Validate(); err != nil {
            return err
        }
    }

    return nil
}
```

**Worktree Level (Security Critical):**
```go
// config.go:168-196
func (w *Worktree) Validate() error {
    // name and branch required
    if w.Name == "" || w.Branch == "" {
        return ErrConfigInvalid
    }

    // SECURITY: Path traversal prevention
    if strings.ContainsAny(w.Name, "/\\") {
        return ErrConfigInvalid // no path separators
    }
    if strings.Contains(w.Name, "..") {
        return ErrConfigInvalid // no parent directory
    }
    if strings.Contains(w.Name, "\x00") {
        return ErrConfigInvalid // no null bytes
    }
    if filepath.IsAbs(w.Name) {
        return ErrConfigInvalid // no absolute paths
    }

    // Ensure clean filename (no directory components)
    if w.Name != filepath.Base(filepath.Clean(w.Name)) {
        return ErrConfigInvalid
    }

    return nil
}
```

**Git URL Validation:**
```go
// config.go:214-245
func validateGitURL(gitURL string) error {
    // Length check (max 2000 chars)
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
        // Regex: git@[valid-hostname]:[valid-path]
        // Prevents malformed hostnames, validates structure
        if !sshGitURLPattern.MatchString(gitURL) {
            return ErrConfigInvalid
        }
        return nil
    }

    // Reject all other schemes (file://, git://, etc.)
    return ErrConfigInvalid
}
```

### Configuration Merging Logic

**Additive Merge Strategy:**
```go
// merge.go:10-87
func Merge(base, local *Config) *Config {
    // Handle nil cases
    if base == nil { return local }
    if local == nil { return base }

    result := &Config{}

    // Metadata: local overrides base
    result.Name = base.Name
    if local.Name != "" {
        result.Name = local.Name
    }
    // (same for description, owner)

    // Repos: additive merge
    baseRepos := makeRepoMap(base.Repos)
    localRepos := makeRepoMap(local.Repos)

    // Process base repos
    for _, baseRepo := range base.Repos {
        merged := Repo{
            Name: baseRepo.Name,
            URL:  baseRepo.URL,
            Type: baseRepo.Type,
        }

        // Copy base worktrees
        merged.Worktrees = append(merged.Worktrees, baseRepo.Worktrees...)

        // Add local worktrees if repo exists in local
        if localRepo := localRepos[baseRepo.Name]; localRepo != nil {
            existingWorktrees := makeWorktreeSet(merged.Worktrees)
            for _, localWt := range localRepo.Worktrees {
                if !existingWorktrees[localWt.Name] {
                    merged.Worktrees = append(merged.Worktrees, localWt)
                }
            }
        }

        result.Repos = append(result.Repos, merged)
    }

    // Add repos from local not in base
    for _, localRepo := range local.Repos {
        if baseRepos[localRepo.Name] == nil {
            result.Repos = append(result.Repos, localRepo)
        }
    }

    return result
}
```

**Merge Behavior Examples:**
```yaml
# Base config.yaml
repos:
  - name: repo1
    worktrees:
      - name: main
        branch: main
      - name: develop
        branch: develop

# Local config.local.yaml
repos:
  - name: repo1
    worktrees:
      - name: my-feature  # Added
        branch: feature/mine
  - name: repo2           # New repo
    worktrees:
      - name: main
        branch: main

# Merged result
repos:
  - name: repo1
    worktrees:
      - name: main        # From base
      - name: develop     # From base
      - name: my-feature  # From local
  - name: repo2           # From local
    worktrees:
      - name: main
```

---

## Git Integration

### Repository Interface

```go
// git/git.go:9-30
type Repository interface {
    // Clone creates a bare repository
    Clone(url, path string) error

    // CreateWorktree creates a worktree with branch
    CreateWorktree(name, branch string) error

    // ListWorktrees returns all worktrees
    ListWorktrees() ([]WorktreeInfo, error)

    // GetCurrentBranch returns worktree's current branch
    GetCurrentBranch(worktree string) (string, error)

    // Exists checks if repository exists
    Exists() bool
}

type WorktreeInfo struct {
    Name   string // Worktree directory name
    Path   string // Full path to worktree
    Branch string // Current branch
    Commit string // Current commit SHA
}
```

### Git CLI Implementation

**Clone Operation:**
```go
// git/local.go:28-37
func (r *LocalRepository) Clone(url, path string) error {
    // SECURITY: Use exec.Command with separate arguments
    // This prevents shell injection via URL or path
    cmd := exec.Command("git", "clone", "--bare", url, path)

    output, err := cmd.CombinedOutput()
    if err != nil {
        return wrapGitError("clone", path, err, output)
    }

    return nil
}
```

**Create Worktree:**
```go
// git/local.go:42-57
func (r *LocalRepository) CreateWorktree(name, branch string) error {
    // Validate repo exists
    if !r.Exists() {
        return ErrGitFailed
    }

    worktreePath := filepath.Join(r.Path, name)

    // SECURITY: Separate arguments prevent injection
    cmd := exec.Command("git", "-C", r.Path, "worktree", "add", worktreePath, branch)

    output, err := cmd.CombinedOutput()
    if err != nil {
        return wrapGitError("create worktree", worktreePath, err, output)
    }

    return nil
}
```

**List Worktrees:**
```go
// git/local.go:61-74
func (r *LocalRepository) ListWorktrees() ([]WorktreeInfo, error) {
    // Use --porcelain for machine-readable output
    cmd := exec.Command("git", "-C", r.Path, "worktree", "list", "--porcelain")

    output, err := cmd.CombinedOutput()
    if err != nil {
        return nil, wrapGitError("list worktrees", r.Path, err, output)
    }

    // Parse porcelain output
    return parseWorktreeList(string(output))
}
```

**Worktree List Parsing:**
```
Porcelain Format:
  worktree /path/to/repo/main
  HEAD abcdef1234567890
  branch refs/heads/main

  worktree /path/to/repo/feature
  HEAD 1234567890abcdef
  branch refs/heads/feature-branch

Parsing Logic (local.go:113-173):
  1. Split output by blank lines (worktree separator)
  2. For each worktree block:
     - Parse "worktree <path>" → extract path, basename = name
     - Parse "HEAD <commit>" → extract commit SHA
     - Parse "branch refs/heads/<branch>" → extract branch name
  3. Handle detached HEAD (no branch line)
  4. Return []WorktreeInfo
```

**Repository Existence Check:**
```go
// git/local.go:99-111
func (r *LocalRepository) Exists() bool {
    // Bare repos have: HEAD, objects/, refs/
    // (no .git directory like standard repos)

    headPath := filepath.Join(r.Path, "HEAD")
    objectsPath := filepath.Join(r.Path, "objects")
    refsPath := filepath.Join(r.Path, "refs")

    _, err1 := os.Stat(headPath)
    info2, err2 := os.Stat(objectsPath)
    info3, err3 := os.Stat(refsPath)

    return err1 == nil &&
           err2 == nil && info2.IsDir() &&
           err3 == nil && info3.IsDir()
}
```

---

## Error Handling

### Error Type Hierarchy

```
error (interface)
  └─> DevlogError (struct)
       ├─> Op: string (operation name)
       ├─> Path: string (file/directory path)
       └─> Err: error (wrapped error)

Sentinel Errors (for errors.Is):
  - ErrConfigNotFound
  - ErrConfigInvalid
  - ErrRepoNotFound
  - ErrGitFailed
```

### Error Wrapping

```go
// errors/errors.go:58-79
func WrapPath(op, path string, err error) error {
    if err == nil {
        return nil
    }
    return &DevlogError{
        Op:   op,
        Path: path,
        Err:  err,
    }
}

// Usage examples:
config.Load():
    return WrapPath("load config", path, ErrConfigNotFound)

git.Clone():
    return WrapPath("clone repository", path, ErrGitFailed)

workspace.LoadWorkspace():
    return Wrap("load workspace", err)
```

### Error Unwrapping

```go
// errors/errors.go:38-40
func (e *DevlogError) Unwrap() error {
    return e.Err
}

// Usage with errors.Is:
err := config.Load("missing.yaml")
if errors.Is(err, errors.ErrConfigNotFound) {
    // Handle config not found
}

// Usage with errors.As:
var devErr *errors.DevlogError
if errors.As(err, &devErr) {
    fmt.Printf("Operation: %s, Path: %s\n", devErr.Op, devErr.Path)
}
```

### Exit Code Mapping

```go
// main.go:11-28
func main() {
    if err := devlog.Execute(); err != nil {
        fmt.Fprintln(os.Stderr, err)

        var devErr *deverrors.DevlogError
        if deverrors.As(err, &devErr) {
            if deverrors.Is(err, deverrors.ErrConfigNotFound) ||
               deverrors.Is(err, deverrors.ErrConfigInvalid) {
                os.Exit(deverrors.ExitConfigError)  // Exit 2
            }
            if deverrors.Is(err, deverrors.ErrGitFailed) {
                os.Exit(deverrors.ExitGitError)     // Exit 3
            }
        }

        os.Exit(deverrors.ExitGeneralError)  // Exit 1
    }
    // Exit 0 (success)
}
```

---

## Data Flow

### Sync Command Data Flow

```
┌─────────────────────────────────────────────────────────┐
│ 1. Configuration Loading                                 │
│                                                          │
│  .devlog/config.yaml ──┐                                │
│                        ├─> config.LoadMerged()          │
│  .devlog/config.local  ─┘         │                     │
│  .yaml (optional)                 │                     │
│                                   ▼                      │
│                          ┌────────────────┐             │
│                          │ Merged Config  │             │
│                          │ - Repos[]      │             │
│                          │ - Worktrees[]  │             │
│                          └────────┬───────┘             │
└──────────────────────────────────┼─────────────────────┘
                                   │
┌──────────────────────────────────▼─────────────────────┐
│ 2. Repository Processing                                │
│                                                          │
│  For each repo in config:                               │
│    │                                                     │
│    ├─> Get repo path: workspace.Root + repo.Name        │
│    │                                                     │
│    ├─> Check exists: git.LocalRepository.Exists()       │
│    │    ├─> Not exists → git.Clone(url, path)           │
│    │    │    └─> exec "git clone --bare ..."            │
│    │    └─> Exists → skip                               │
│    │                                                     │
└────┼─────────────────────────────────────────────────────┘
     │
┌────▼─────────────────────────────────────────────────────┐
│ 3. Worktree Processing                                   │
│                                                          │
│  For each worktree in repo:                             │
│    │                                                     │
│    ├─> List existing: git.ListWorktrees()               │
│    │    └─> exec "git worktree list --porcelain"        │
│    │        └─> parse output → []WorktreeInfo           │
│    │                                                     │
│    ├─> Check if worktree exists in list                 │
│    │    ├─> Not exists → git.CreateWorktree(name, br)   │
│    │    │    └─> exec "git worktree add ..."            │
│    │    └─> Exists → skip                               │
│    │                                                     │
└────┼─────────────────────────────────────────────────────┘
     │
┌────▼─────────────────────────────────────────────────────┐
│ 4. Output Summary                                        │
│                                                          │
│  Print summary:                                          │
│    - Repositories cloned: N                              │
│    - Repositories skipped: M                             │
│    - Worktrees created: X                                │
│    - Worktrees skipped: Y                                │
│                                                          │
└──────────────────────────────────────────────────────────┘
```

---

## Extension Points

### Adding New Commands

**Steps to add a new command:**
1. Create `cmd/devlog/newcommand.go`
2. Define `cobra.Command` struct
3. Implement `RunE` function
4. Register in `init()` function: `rootCmd.AddCommand(newCmd)`

**Example:**
```go
// cmd/devlog/update.go
var updateCmd = &cobra.Command{
    Use:   "update",
    Short: "Pull latest changes in all worktrees",
    RunE:  runUpdate,
}

func runUpdate(cmd *cobra.Command, args []string) error {
    ws, err := workspace.LoadWorkspace(".")
    if err != nil {
        return err
    }

    // Implementation here
    return nil
}

func init() {
    rootCmd.AddCommand(updateCmd)
}
```

### Adding New Git Operations

**Steps to extend git.Repository interface:**
1. Add method to `git/git.go` interface
2. Implement method in `git/local.go`
3. Add tests in `git/local_test.go`

**Example:**
```go
// git/git.go
type Repository interface {
    // Existing methods...

    // RemoveWorktree removes a worktree
    RemoveWorktree(name string) error
}

// git/local.go
func (r *LocalRepository) RemoveWorktree(name string) error {
    worktreePath := filepath.Join(r.Path, name)
    cmd := exec.Command("git", "-C", r.Path, "worktree", "remove", worktreePath)
    output, err := cmd.CombinedOutput()
    if err != nil {
        return wrapGitError("remove worktree", worktreePath, err, output)
    }
    return nil
}
```

### Adding New Output Formats

**Steps to add new output writer:**
1. Implement `output.Writer` interface
2. Create factory function
3. Use in commands

**Example:**
```go
// internal/output/json.go
type JSONWriter struct {
    writer io.Writer
}

func NewJSONWriter(w io.Writer) *JSONWriter {
    return &JSONWriter{writer: w}
}

func (w *JSONWriter) Success(msg string) {
    json.NewEncoder(w.writer).Encode(map[string]string{
        "level": "success",
        "message": msg,
    })
}
// ... implement other methods

// Usage in command:
var jsonOutput bool
if jsonOutput {
    out = output.NewJSONWriter(os.Stdout)
} else {
    out = output.NewStdoutWriter(IsVerbose())
}
```

---

## Dependencies

### Dependency Graph

```
main.go
  └─> cmd/devlog.Execute()
       ├─> cmd/devlog/init.go
       ├─> cmd/devlog/sync.go
       │    ├─> workspace.LoadWorkspace()
       │    │    └─> config.LoadMerged()
       │    │         ├─> config.Load() (YAML parsing)
       │    │         └─> config.Merge()
       │    ├─> git.NewLocalRepository()
       │    │    ├─> git.Clone() → exec.Command("git", "clone", "--bare")
       │    │    ├─> git.ListWorktrees() → exec.Command("git", "worktree", "list")
       │    │    └─> git.CreateWorktree() → exec.Command("git", "worktree", "add")
       │    └─> output.NewStdoutWriter()
       └─> cmd/devlog/status.go
            ├─> workspace.LoadWorkspace()
            ├─> git.NewLocalRepository()
            └─> output.NewStdoutWriter()
```

### External Dependency Details

**github.com/spf13/cobra v1.8.1**
- Purpose: CLI framework (command parsing, flags, help)
- Used in: All cmd/devlog/*.go files
- Why chosen: Industry standard, comprehensive feature set
- License: Apache 2.0

**gopkg.in/yaml.v3 v3.0.1**
- Purpose: YAML parsing and marshaling
- Used in: config/config.go, cmd/devlog/init.go
- Why chosen: v3.0.1+ fixes CVE-2022-28948 (DoS via YAML bomb)
- Security: Max file size check (1MB) prevents YAML bombs
- License: MIT + Apache 2.0

**Go standard library:**
- os, os/exec: Filesystem operations, git CLI execution
- path/filepath: Cross-platform path handling
- fmt, strings: String formatting and manipulation
- errors: Error handling and wrapping
- testing: Unit tests

### Zero Git Library Dependency

**Decision: Shell out to git CLI instead of go-git**

**Rationale:**
- Git CLI is stable, ubiquitous, well-tested
- Git worktree support in go-git is incomplete
- Subprocess overhead negligible for workspace setup operations
- Easier to debug (can see exact git commands executed)

**Tradeoffs:**
- Requires git CLI installed (acceptable for development tool)
- Subprocess overhead (~10-50ms per git command)
- No cross-compilation benefit (git must be on PATH)

---

## Performance Characteristics

### Initialization (devlog init)

**Operations:**
- Create .devlog directory: <1ms
- Generate YAML config: <1ms
- Write 4 files (config, example, README, .gitignore): <10ms

**Total time:** <15ms

---

### Sync (devlog sync)

**Cold start (nothing exists):**
- Config loading: ~5-10ms
- Clone 1 bare repo: ~2-10 seconds (network dependent)
- Create 1 worktree: ~100-500ms (disk I/O)

**For 3 repos, 6 worktrees:**
- Config loading: ~10ms
- Clone 3 repos: ~6-30 seconds (parallel not implemented)
- Create 6 worktrees: ~600-3000ms
- **Total: ~7-35 seconds (network dependent)**

**Warm start (all exists, idempotent check):**
- Config loading: ~10ms
- Repo exists checks: ~3ms per repo (stat calls)
- Worktree list: ~50ms per repo (git worktree list)
- **Total: ~200ms for 3 repos**

---

### Status (devlog status)

**Operations:**
- Config loading: ~10ms
- Repo exists checks: ~3ms per repo
- List worktrees: ~50ms per repo
- Output formatting: ~5ms

**For 3 repos:**
- **Total: ~175ms**

---

## Debugging and Troubleshooting

### Verbose Mode

**Enable:** `devlog sync --verbose`

**Output:**
```
Verbose mode enabled
Config file: .devlog/config.yaml
Dry run: false
→ Cloning example-repo...
→ Processing worktrees for example-repo...
→   Creating worktree main...
→   Creating worktree feature...
```

### Common Issues

**Issue 1: Config not found**
```
Error: find config directory failed for .: config file not found: searched from . up to root
```
**Solution:** Run `devlog init` to create .devlog/config.yaml

**Issue 2: Git clone failed**
```
Error: clone repository failed for repo-name: git clone failed: fatal: repository not found
```
**Solution:** Check repository URL in config.yaml (must be valid, accessible)

**Issue 3: Worktree creation failed**
```
Error: create worktree failed for repo/feature: git worktree add failed: fatal: branch already exists
```
**Solution:** Worktree with same name already exists, check `git worktree list`

---

**Document Status:** Complete
**Last Reviewed:** 2026-02-11
**Next Review:** Quarterly or when adding major features
