# devlog-cli Specification

**Version:** 0.1.0
**Status:** Production-ready
**Type:** CLI Tool
**Last Updated:** 2026-02-11

---

## Executive Summary

devlog-cli is a command-line tool for managing development workspaces using bare git repositories with multiple worktrees. It automates the setup, synchronization, and status checking of multi-repository, multi-worktree development environments across machines.

**Core Value Proposition:**
- Automates bare repository cloning and worktree creation across machines
- Enables parallel feature development with isolated worktrees per branch
- Declarative workspace configuration with team and local override support
- Idempotent sync operations safe to run repeatedly
- Multi-machine workspace synchronization via shared config files

---

## Problem Statement

### Current Pain Points

**Manual Worktree Management Complexity:**
- Setting up multiple worktrees manually is tedious (git clone --bare, git worktree add for each branch)
- No standardized way to replicate workspace structure across machines (laptop, desktop, CI)
- Manual tracking of which repositories and worktrees exist versus desired state
- No mechanism to share team workspace structure (onboarding requires manual setup)
- Worktree management differs from traditional git workflows (unfamiliar to new users)

**Impact:**
- 30-60 minutes to set up workspace on new machine (manual cloning and worktree creation)
- Inconsistent workspace layouts across team members
- Difficult onboarding for developers new to git worktrees
- No way to version control workspace structure itself
- Risk of manually creating worktrees incorrectly (wrong paths, wrong branches)

### Target Use Cases

**Primary:**
- Solo developer with multi-machine setup (laptop + desktop)
- Team sharing workspace structure for consistent development environment
- Parallel feature development (work on feature-a while reviewing feature-b)
- Maintaining multiple release branches simultaneously (v1.x, v2.x, main)

**Secondary:**
- Disaster recovery (recreate workspace from config backup)
- CI/CD environment setup (clone repos and create worktrees for testing)
- Code review workflows (keep main clean, work in separate worktrees)
- Dotfiles management (commit .devlog/config.yaml to dotfiles repo)

**Non-goals:**
- Git operations beyond clone and worktree (use git directly for fetch, pull, push, commit)
- Worktree cleanup or removal (use `git worktree remove` manually)
- Branch switching within worktrees (use `git checkout` in worktree directory)
- Remote synchronization (fetching changes, pulling updates)

---

## Solution Overview

### Architecture

```
devlog-cli/
├── cmd/devlog/          # CLI commands
│   ├── root.go          # Root command and global flags
│   ├── init.go          # Initialize workspace
│   ├── sync.go          # Clone repos and create worktrees
│   └── status.go        # Show workspace status
├── internal/
│   ├── config/          # Configuration loading and merging
│   │   ├── config.go    # Config types and validation
│   │   ├── loader.go    # Config discovery and loading
│   │   └── merge.go     # Config merging logic
│   ├── git/             # Git operations
│   │   ├── git.go       # Repository interface
│   │   └── local.go     # Local git implementation
│   ├── workspace/       # Workspace management
│   │   └── workspace.go # Workspace loading and path resolution
│   ├── output/          # CLI output formatting
│   │   └── output.go    # Writer interface and implementations
│   └── errors/          # Error types
│       └── errors.go    # Structured errors with context
└── main.go              # Entry point
```

### Component Classification

**Type:** CLI Tool (executable command-line application)

**Technology Stack:**
- Language: Go 1.25
- CLI Framework: cobra v1.8.1
- Config Format: YAML (gopkg.in/yaml.v3 v3.0.1)
- Git Integration: Shell commands via exec.Command

---

## API Specification

### Commands

#### 1. devlog init

**Purpose:** Initialize a new devlog workspace with example configuration

**Syntax:**
```bash
devlog init [workspace-name] [--force]
```

**Arguments:**
- `workspace-name` (optional): Name for the workspace (defaults to current directory name)

**Flags:**
- `--force`: Overwrite existing configuration

**Behavior:**
1. Create `.devlog/` directory
2. Generate `.devlog/config.yaml` with example repository configuration
3. Create `.devlog/config.local.yaml.example` template
4. Create `.devlog/README.md` with quick start guide
5. Create `.devlog/.gitignore` to ignore local config

**Output:**
```
✓ Created .devlog directory
✓ Created .devlog/config.yaml
✓ Created .devlog/config.local.yaml.example
✓ Created .devlog/README.md
✓ Created .devlog/.gitignore
✓ Initialized devlog workspace: my-workspace

Next steps:
  1. Edit .devlog/config.yaml to configure your repositories
  2. Run 'devlog sync' to clone repos and create worktrees
  3. (Optional) Create .devlog/config.local.yaml for personal overrides
```

**Exit Codes:**
- 0: Success
- 1: General error
- 2: Config error (workspace already initialized without --force)

---

#### 2. devlog sync

**Purpose:** Clone repositories and create worktrees from configuration

**Syntax:**
```bash
devlog sync [--dry-run] [--verbose]
```

**Flags:**
- `--dry-run`: Show what would happen without making changes
- `--verbose`: Show detailed progress information
- `--config <path>`: Use alternate config file (default: `.devlog/config.yaml`)

**Behavior:**
1. Load and merge config.yaml + config.local.yaml
2. For each configured repository:
   - Clone bare repository if it doesn't exist (skip if exists)
   - List existing worktrees
   - Create missing worktrees (skip if exist)
3. Print summary of actions taken

**Idempotency:** Safe to run multiple times - skips existing repos and worktrees

**Output (normal mode):**
```
→ Cloning example-repo...
✓ Cloned example-repo
→ Processing worktrees for example-repo...
→   Creating worktree main...
✓   Created worktree main on branch main
→   Creating worktree feature...
✓   Created worktree feature on branch feature-branch

Sync Summary:
  Repositories cloned: 1
  Repositories skipped (already exist): 0
  Worktrees created: 2
  Worktrees skipped (already exist): 0
✓ Sync complete!
```

**Output (dry-run mode):**
```
DRY RUN MODE - No changes will be made
→ Would clone example-repo from https://github.com/user/repo.git
→   Would create worktree main on branch main
→   Would create worktree feature on branch feature-branch
DRY RUN COMPLETE - No changes were made
```

**Exit Codes:**
- 0: Success
- 1: General error
- 2: Config not found or invalid
- 3: Git operation failed

---

#### 3. devlog status

**Purpose:** Show current workspace state (repos cloned, worktrees created, branches)

**Syntax:**
```bash
devlog status [--verbose]
```

**Flags:**
- `--verbose`: Show detailed output
- `--config <path>`: Use alternate config file

**Behavior:**
1. Load workspace configuration
2. For each configured repository:
   - Check if repository exists
   - List actual worktrees (via `git worktree list`)
   - Compare configured vs actual worktrees
   - Show current branch for each worktree
3. Warn if worktree is on different branch than configured
4. Show unconfigured worktrees (created manually but not in config)
5. Print summary statistics

**Output:**
```
Workspace: my-workspace
Description: Development workspace with multiple repositories and worktrees

✓ example-repo (cloned)
  ✓ main → main
  ✓ feature → feature-branch
  → debug (not in config) → debug-branch

✗ other-repo (not cloned)
  - worktree1 → branch1 (pending)

Summary:
  Repositories: 2 configured, 1 cloned
  Worktrees: 3 configured, 2 created

Run 'devlog sync' to create missing repos and worktrees
```

**Exit Codes:**
- 0: Success
- 1: General error
- 2: Config not found or invalid

---

### Configuration Format

#### config.yaml Structure

**Location:** `.devlog/config.yaml` (committed to git)

**Format:**
```yaml
name: workspace-name
description: Optional workspace description
owner: Optional owner name

repos:
  - name: repo-directory-name    # Directory where repo will be cloned
    url: https://github.com/user/repo.git  # Git remote URL
    type: bare                     # "bare" or "standard" (bare recommended)
    worktrees:
      - name: main                 # Worktree directory name
        branch: main               # Git branch to checkout
        protected: false           # Optional: prevent deletion (future)
      - name: feature
        branch: feature-branch
```

**Validation Rules:**
- `name` (workspace): Required, non-empty string
- `repos`: At least one repository required
- `repos[].name`: Required, unique, no path separators
- `repos[].url`: Required, valid git URL (https:// or git@)
- `repos[].type`: Optional, must be "bare" or "standard" if specified
- `repos[].worktrees[].name`: Required, unique within repo, no path traversal
- `repos[].worktrees[].branch`: Required, non-empty string

**Security Protections:**
- Max file size: 1MB (prevents YAML bomb attacks)
- URL validation: Only https:// and git@ schemes allowed
- Path traversal prevention: No `..`, `/`, `\`, absolute paths in worktree names
- Null byte detection: Reject names with \x00

---

#### config.local.yaml Structure

**Location:** `.devlog/config.local.yaml` (git-ignored)

**Purpose:** Local machine-specific overrides (personal worktrees, local repos)

**Merging Behavior:**
- Local config extends base config additively
- Add new repositories from local config
- Add new worktrees to existing repositories
- Override workspace metadata (name, description, owner)
- Cannot remove repos or worktrees from base config

**Example:**
```yaml
# Add local-only repository
repos:
  - name: my-local-repo
    url: git@github.com:me/private-repo.git
    type: bare
    worktrees:
      - name: main
        branch: main

# Add personal worktree to existing repo
repos:
  - name: example-repo  # Must match name in config.yaml
    worktrees:
      - name: my-feature
        branch: feature/personal-experiment
```

---

## Functional Requirements

### FR-1: Workspace Initialization

**Requirement:** Users can initialize a devlog workspace in any directory

**Acceptance Criteria:**
- `devlog init` creates `.devlog/` directory with config files
- Generated config.yaml includes example repository and worktrees
- config.local.yaml.example provides template for local overrides
- README.md explains configuration and next steps
- .gitignore prevents committing config.local.yaml
- --force flag allows overwriting existing configuration
- Workspace name defaults to current directory name
- Error if workspace already initialized without --force

**Verification:** Test with `devlog init test-workspace` in empty directory

---

### FR-2: Repository Cloning

**Requirement:** Users can clone bare repositories from config

**Acceptance Criteria:**
- `devlog sync` clones bare repositories (git clone --bare)
- Cloning is idempotent (skips if repository already exists)
- Supports https:// and git@ URL formats
- Validates URLs before attempting clone
- Reports errors if clone fails (invalid URL, network error)
- --dry-run shows what would be cloned without executing
- Cloned repos stored in workspace root (same directory as .devlog/)

**Verification:** Test with valid and invalid repository URLs

---

### FR-3: Worktree Creation

**Requirement:** Users can create git worktrees from config

**Acceptance Criteria:**
- `devlog sync` creates worktrees for each configured branch
- Worktree creation is idempotent (skips if worktree exists)
- Worktrees created as subdirectories of repository (repo/worktree-name/)
- Creates branch tracking origin/branch if branch doesn't exist locally
- Reports errors if worktree creation fails
- Skips worktree creation if repository doesn't exist
- Handles multiple worktrees per repository

**Verification:** Test with multiple worktrees in config, run sync twice

---

### FR-4: Workspace Status Checking

**Requirement:** Users can view current workspace state

**Acceptance Criteria:**
- `devlog status` shows which repos are cloned vs configured
- Shows which worktrees exist vs configured
- Displays current branch for each worktree
- Warns if worktree is on different branch than configured
- Lists unconfigured worktrees (created manually)
- Provides summary statistics (repos cloned, worktrees created)
- Suggests running `devlog sync` if resources are missing

**Verification:** Test with partial workspace (some repos cloned, some not)

---

### FR-5: Configuration Merging

**Requirement:** Local config extends base config additively

**Acceptance Criteria:**
- config.local.yaml adds new repositories to base config
- config.local.yaml adds new worktrees to existing repositories
- Duplicate worktrees in local config are ignored (base wins)
- Local config cannot remove base repos or worktrees
- Workspace metadata (name, description) can be overridden by local
- Missing local config is not an error (uses base config only)
- Merged config is validated after merging

**Verification:** Test with base + local config, verify additive merge

---

### FR-6: Configuration Discovery

**Requirement:** Commands find config by walking up directory tree

**Acceptance Criteria:**
- Commands search for .devlog/config.yaml starting from current directory
- Walk up directory tree until .devlog/ is found or filesystem root
- Error if .devlog/config.yaml not found after searching
- Commands work from any subdirectory within workspace
- --config flag overrides automatic discovery

**Verification:** Test running commands from workspace root, repo dir, worktree dir

---

### FR-7: Error Handling and Exit Codes

**Requirement:** Errors are reported clearly with appropriate exit codes

**Acceptance Criteria:**
- Config errors exit with code 2
- Git errors exit with code 3
- General errors exit with code 1
- Success exits with code 0
- Error messages include operation context and path
- Errors support unwrapping (errors.Is, errors.As)
- Structured error types for programmatic handling

**Verification:** Test various error scenarios, check exit codes

---

## Non-Functional Requirements

### NFR-1: Idempotency

**Requirement:** All operations are safe to run multiple times

**Acceptance Criteria:**
- `devlog sync` skips existing repos and worktrees
- `devlog init` errors if workspace exists (unless --force)
- `devlog status` is read-only (no side effects)
- Running sync multiple times produces same result
- No data loss or corruption from repeated execution

**Performance Target:** Sync with all resources existing completes in <5 seconds

---

### NFR-2: Configuration Security

**Requirement:** Config parsing is secure against malicious input

**Acceptance Criteria:**
- YAML bomb protection (max file size 1MB)
- Path traversal prevention (no ../, /, \ in worktree names)
- URL validation (only https:// and git@ schemes)
- Null byte detection in strings
- No arbitrary command execution (git commands use exec.Command with args)
- Safe error messages (no sensitive data leakage)

**Security Standards:** Uses gopkg.in/yaml.v3 v3.0.1+ (CVE-2022-28948 fixed)

---

### NFR-3: Usability

**Requirement:** Tool is easy to learn and use

**Acceptance Criteria:**
- Clear help text for all commands (--help)
- Informative error messages with next steps
- Example configuration generated by `devlog init`
- README.md in .devlog/ explains configuration
- Status command shows what to do next
- Dry-run mode for preview before execution
- Verbose mode for debugging

**Documentation:** README.md, inline help, example config

---

### NFR-4: Cross-Platform Compatibility

**Requirement:** Works on Linux, macOS, Windows

**Acceptance Criteria:**
- Uses filepath.Join for path construction
- Git invocation works across platforms (uses exec.Command)
- YAML parsing platform-agnostic
- File permissions set appropriately (0755 dirs, 0644 files)
- Path separators handled correctly

**Tested Platforms:** Linux (primary), macOS, Windows (git bash)

---

### NFR-5: Maintainability

**Requirement:** Code is well-structured and testable

**Acceptance Criteria:**
- Clear separation of concerns (cmd, internal packages)
- Interface-based design (Repository, Writer)
- Unit tests for core logic (config, git, workspace)
- Test coverage >80%
- Godoc comments on public types and functions
- Linting passes (golangci-lint)

**Code Quality:** 83% test coverage, zero linter warnings

---

## Dependencies

### External Dependencies

**Runtime:**
- Go 1.25+ (compiler and runtime)
- Git CLI (for clone and worktree operations)

**Build:**
- github.com/spf13/cobra v1.8.1 (CLI framework)
- gopkg.in/yaml.v3 v3.0.1 (YAML parsing, CVE-2022-28948 fix)

**Indirect:**
- github.com/inconshreveable/mousetrap v1.1.0 (cobra dependency)
- github.com/spf13/pflag v1.0.5 (cobra dependency)

**Zero External Dependencies for Git:** Uses exec.Command to invoke system git

---

## Success Criteria

### Primary Metrics

- **Workspace setup time:** <5 minutes (vs 30-60 manual)
- **Onboarding friction:** New developer can clone and sync in single command
- **Multi-machine sync:** Config committed to git enables instant workspace replication
- **Reliability:** 100% idempotent operations (no data loss from re-running commands)

### Secondary Metrics

- **Test coverage:** ≥80% (current: 83%)
- **User adoption:** Used for ≥3 real-world projects
- **Error handling:** Zero panics, all errors contextual
- **Documentation:** README + inline help sufficient for onboarding

### Validation Metrics

- **Dogfooding:** Used by devlog-cli developers for self-development
- **Multi-machine testing:** Works on Linux, macOS, Windows
- **Team sharing:** Config can be shared via git and merged successfully

---

## Out of Scope

**NOT Included:**
- Git operations beyond clone and worktree add (use git directly for fetch, pull, push, commit, merge)
- Worktree removal or cleanup (use `git worktree remove` manually)
- Branch switching within worktrees (use `git checkout` in worktree directory)
- Remote synchronization (fetching changes, pulling updates)
- Standard (non-bare) repository support (bare repos recommended for worktrees)
- GUI or web interface (CLI only)
- Integration with IDE/editor (users configure tools themselves)
- Automated testing within worktrees (use existing CI/CD tools)

**Future Considerations:**
- `devlog clean` - Remove worktrees not in config
- `devlog update` - Pull latest changes in all worktrees
- `devlog switch` - Switch branch in worktree
- Shell completion (bash, zsh, fish)
- Interactive init with prompts
- Config validation command (lint config files)

---

## Design Principles

1. **Idempotent by default** - All operations safe to run multiple times
2. **Declarative configuration** - Config describes desired state, sync achieves it
3. **Git-native operations** - Uses standard git commands, no custom git logic
4. **Minimal dependencies** - Only essential libraries (cobra, yaml)
5. **Security first** - Validate all inputs, prevent injection attacks
6. **Composable commands** - Each command does one thing well
7. **Fail-fast validation** - Validate config before executing operations
8. **Clear error messages** - Errors explain what went wrong and how to fix

---

**Document Status:** Complete
**Last Reviewed:** 2026-02-11
**Next Review:** Quarterly or when adding major features
