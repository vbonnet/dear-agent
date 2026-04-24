# Devlog CLI - Architecture Documentation

## Overview

Devlog CLI is a workspace management tool for developers working with multiple git repositories using bare repos and worktrees. It provides a declarative configuration approach to synchronize development environments across machines.

## Architecture Diagrams

### C4 Component Diagram

![C4 Component Diagram](c4-component-devlog.d2)

**Source**: [c4-component-devlog.d2](c4-component-devlog.d2)

The component diagram shows the internal architecture of the Devlog CLI application at C4 Level 3, including:

- **CLI Commands Layer**: Command handlers (init, sync, status) built on Cobra framework
- **Core Business Logic**: WorkspaceManager, ConfigLoader, ConfigMerger, ConfigValidator
- **Git Operations Layer**: Repository interface and LocalRepository implementation
- **Infrastructure**: OutputWriter for formatted CLI output, ErrorHandler for structured errors
- **Domain Model**: Config, Workspace, and WorktreeInfo data structures

## Key Components

### CLI Commands Layer

#### RootCommand
- Entry point for the CLI application
- Manages persistent flags (verbose, dry-run, config)
- Coordinates command execution

#### InitCommand
- Bootstraps a new devlog workspace
- Creates `.devlog/` directory structure
- Generates initial `config.yaml` template

#### SyncCommand
- Clones missing bare repositories
- Creates configured worktrees
- Idempotent operations (safe to run multiple times)
- Supports dry-run mode

#### StatusCommand
- Shows workspace state
- Reports repo clone status
- Lists worktrees and current branches
- Identifies missing resources

### Core Business Logic

#### WorkspaceManager
- Discovers workspace root by searching for `.devlog/` directory
- Manages workspace lifecycle
- Provides path resolution utilities (GetRepoPath, GetWorktreePath)
- Delegates to ConfigLoader for configuration

#### ConfigLoader
- Searches up directory tree to find `.devlog/config.yaml`
- Loads both base config (committed) and local config (git-ignored)
- Enforces size limits (1MB max) to prevent YAML bomb attacks
- Delegates to ConfigMerger and ConfigValidator

#### ConfigMerger
- Implements additive merge strategy
- Local config extends base config (never removes team repos)
- Deduplicates repos and worktrees
- Preserves team-wide settings while allowing local customization

#### ConfigValidator
- Security validations:
  - Path traversal prevention (checks for `..`, `/`, `\`, null bytes)
  - Git URL validation (HTTPS and SSH only)
  - Duplicate detection
- Semantic validations:
  - Required fields (name, repos, URLs)
  - Valid repo types (bare/standard)
  - Valid worktree branch names

### Git Operations Layer

#### Repository Interface
Defines the contract for git operations:
- `Clone(url, path)` - Clone bare repository
- `CreateWorktree(name, branch)` - Create new worktree
- `ListWorktrees()` - List all worktrees
- `GetCurrentBranch(worktree)` - Get current branch
- `Exists()` - Check if repo exists

#### LocalRepository
Implementation using local git CLI:
- Executes git commands via `exec.Command` with argument separation (security)
- Parses porcelain output (`git worktree list --porcelain`)
- Checks bare repo structure (HEAD, objects/, refs/)
- Handles worktree creation with automatic branch tracking

### Infrastructure & Support

#### OutputWriter
- Formatted CLI output with visual indicators (✓, ✗, →)
- Message types: Success, Error, Info, Progress
- Table rendering for structured data
- Verbose mode control

#### ErrorHandler
- Structured error type: `DevlogError` with operation and path context
- Sentinel errors: `ErrConfigNotFound`, `ErrConfigInvalid`, `ErrGitFailed`
- Error unwrapping for Go 1.13+ `errors.Is()` and `errors.As()`
- Exit code mapping

### Domain Model

#### Config
```go
type Config struct {
    Name        string
    Description string
    Owner       string
    Repos       []Repo
}

type Repo struct {
    Name      string
    URL       string
    Type      RepoType  // "bare" or "standard"
    Worktrees []Worktree
}

type Worktree struct {
    Name      string
    Branch    string
    Protected bool
}
```

#### Workspace
```go
type Workspace struct {
    Config *Config
    Root   string  // Directory containing .devlog/
}
```

## Data Flow

### Sync Command Flow
1. User executes `devlog sync`
2. RootCommand parses flags and invokes SyncCommand
3. SyncCommand → WorkspaceManager.LoadWorkspace()
4. WorkspaceManager → ConfigLoader.LoadMerged()
5. ConfigLoader searches for `.devlog/`, reads YAML files
6. ConfigLoader → ConfigMerger.Merge(base, local)
7. ConfigLoader → ConfigValidator.Validate()
8. SyncCommand iterates repos from config
9. For each repo: LocalRepository.Clone() if missing
10. For each worktree: LocalRepository.CreateWorktree() if missing
11. OutputWriter reports progress to user

### Status Command Flow
1. User executes `devlog status`
2. StatusCommand → WorkspaceManager.LoadWorkspace()
3. StatusCommand iterates repos from config
4. For each repo: LocalRepository.Exists()
5. For each repo: LocalRepository.ListWorktrees()
6. Compare configured vs actual worktrees
7. OutputWriter displays status table

## Security Features

### Configuration Security
- **File size limits**: 1MB maximum to prevent YAML bomb DoS
- **Path traversal prevention**: Validates worktree names (no `..`, `/`, `\`)
- **URL validation**: Only allows HTTPS and SSH git URLs
- **Input validation**: Rejects null bytes, absolute paths

### Git Command Security
- **Argument separation**: Uses `exec.Command(cmd, arg1, arg2)` not string concatenation
- **No shell injection**: Direct binary execution, no shell interpretation
- **URL sanitization**: Validates URLs before passing to git

### Error Handling
- **Structured errors**: Operation and path context for debugging
- **No sensitive data leakage**: Error messages don't expose credentials
- **Exit codes**: Proper exit codes for scripting and automation

## Dependencies

### External Libraries
- **github.com/spf13/cobra** v1.8.1 - CLI framework
- **gopkg.in/yaml.v3** v3.0.1 - YAML parser (CVE-2022-28948 fix included)

### External Systems
- **Git CLI** - Requires git 2.5+ for worktree support
- **File System** - Stores `.devlog/` configs and manages repos

## Design Principles

1. **Idempotency**: Sync operations are safe to run multiple times
2. **Additive Merging**: Local configs extend team configs, never remove
3. **Security First**: Path validation, URL validation, size limits
4. **Fail-Fast Validation**: Validate config before executing git operations
5. **Structured Output**: Consistent, parseable CLI output
6. **Error Context**: Rich error messages with operation and path tracking

## Testing Strategy

Each component has corresponding `*_test.go` files:
- **Unit tests**: ConfigLoader, ConfigMerger, ConfigValidator, LocalRepository
- **Integration tests**: Command tests with mock workspaces
- **Security tests**: Path traversal, URL validation, size limits

## Future Enhancements

Potential areas for expansion:
- Remote sync (push/pull coordination)
- Worktree cleanup (remove stale worktrees)
- Config templates and scaffolding
- Interactive mode for guided setup
- Parallel operations for faster sync
