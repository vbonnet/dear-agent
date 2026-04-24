# Engram CLI - Architecture

## System Overview

The Engram CLI is a Cobra-based command-line application that provides AI agents with memory persistence, learning, and retrieval capabilities. It follows a modular architecture with clear separation of concerns.

```
┌─────────────────────────────────────────────────────────────┐
│                         CLI Layer                            │
│  (Cobra Commands, Flag Parsing, Input Validation)           │
└───────────────────────────┬─────────────────────────────────┘
                            │
┌───────────────────────────┼─────────────────────────────────┐
│                      Internal CLI                            │
│  (Errors, Output, Progress, Validation, Security)            │
└───────────────────────────┬─────────────────────────────────┘
                            │
┌───────────────────────────┴─────────────────────────────────┐
│                     Core Services                            │
│  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐       │
│  │ Retrieval│ │  Health  │ │  Config  │ │  Plugin  │       │
│  │ Service  │ │  Check   │ │  Loader  │ │  Loader  │       │
│  └──────────┘ └──────────┘ └──────────┘ └──────────┘       │
└───────────────────────────┬─────────────────────────────────┘
                            │
┌───────────────────────────┴─────────────────────────────────┐
│                     Storage Layer                            │
│  ┌──────────┐ ┌──────────┐ ┌──────────┐                     │
│  │ Ecphory  │ │  Memory  │ │  Index   │                     │
│  │  Index   │ │ Provider │ │  Cache   │                     │
│  └──────────┘ └──────────┘ └──────────┘                     │
└─────────────────────────────────────────────────────────────┘
```

## Directory Structure

```
core/cmd/engram/
├── main.go                      # Entry point
├── cmd/                         # Command implementations
│   ├── root.go                  # Root command & version
│   ├── init.go                  # Workspace initialization
│   ├── doctor.go                # Health checks
│   ├── retrieve.go              # Engram retrieval
│   ├── index.go                 # Index management
│   ├── memory.go                # Memory commands
│   ├── memory_store.go
│   ├── memory_retrieve.go
│   ├── memory_update.go
│   ├── memory_delete.go
│   ├── plugin.go                # Plugin management
│   ├── tokens.go                # Token estimation
│   ├── tokens_estimate.go
│   ├── analytics.go             # Analytics commands
│   ├── analytics_list.go
│   ├── analytics_show.go
│   ├── analytics_summary.go
│   ├── analytics_persona.go
│   ├── analytics_ecphory.go
│   ├── analytics_wayfinder.go
│   ├── config.go                # Config commands
│   ├── config_show.go
│   ├── guidance.go              # Guidance retrieval
│   ├── hash.go                  # Hashing utilities
│   ├── slashcmd.go              # Slash command support
│   ├── subagent.go              # Subagent management
│   └── *_test.go                # Tests
├── internal/                    # Internal packages
│   ├── cli/                     # CLI utilities
│   │   ├── errors.go            # Error types
│   │   ├── output.go            # Output formatting
│   │   ├── progress.go          # Progress indicators
│   │   ├── validation.go        # Input validation
│   │   ├── validation_doc.go    # Validation documentation
│   │   ├── security.go          # Security utilities
│   │   └── *_test.go
│   └── validation/              # Validation logic
│       ├── validator.go
│       └── validator_test.go
├── SPEC.md                      # Technical specification
├── ARCHITECTURE.md              # This file
└── engram                       # Compiled binary
```

## Core Components

### 1. CLI Layer (`cmd/`)

#### Root Command (`root.go`)
- Entry point for all commands
- Version information display
- Persistent pre-run hooks (prints header)
- Shell completion generation

**Key Features**:
- Version info from ldflags: `version`, `commit`, `date`, `builtBy`
- Auto-completion for bash, zsh, fish, powershell
- Consistent header output: `engram {version} ({executable})`

#### Command Organization
Commands are organized by domain:
- **Infrastructure**: init, doctor, config
- **Retrieval**: retrieve, index, guidance
- **Memory**: memory store/retrieve/update/delete
- **Plugins**: plugin list
- **Analytics**: analytics list/show/summary/persona/ecphory/wayfinder
- **Utilities**: tokens estimate, hash, completion

### 2. Internal CLI Layer (`internal/cli/`)

#### Error Handling (`errors.go`)
Custom error types for user-friendly error messages:

```go
type EngramError struct {
    Symbol      string   // Visual indicator (✗, !, etc.)
    Message     string   // User-facing message
    Cause       error    // Underlying error
    Suggestions []string // Actionable suggestions
}
```

**Error Factories**:
- `InvalidInputError(field, value, constraint string) error`
- `ConfigNotFoundError(path string, cause error) error`
- `PluginLoadError(path string, cause error) error`

#### Output Formatting (`output.go`)
Consistent output across all commands:

**Functions**:
- `PrintSuccess(msg string)` - Green checkmark + message
- `PrintError(msg string)` - Red X + message
- `PrintWarning(msg string)` - Yellow ! + message
- `PrintInfo(msg string)` - Blue info icon + message
- `DisableColor()` - Disable ANSI colors

**Icons**:
- Success: ✓ (green)
- Error: ✗ (red)
- Warning: ! (yellow)
- Info: ℹ (blue)

#### Progress Indicators (`progress.go`)
Spinner for long-running operations:

```go
type ProgressIndicator struct {
    message string
    spinner *spinner.Spinner
}

// Methods
NewProgress(message string) *ProgressIndicator
Start()
Update(message string)
Stop()
Complete(message string)
Fail(message string)
```

#### Validation (`validation.go`)
Input validation utilities:

**Validators**:
- `ValidateNonEmpty(field, value string) error`
- `ValidateMaxLength(field, value string, max int) error`
- `ValidateRangeInt(field, value, min, max int) error`
- `ValidateOutputFormat(format string, allowed ...string) error`
- `ValidateTier(tier string) error`
- `ValidateSafePath(field, path string, allowedPaths []string) error`
- `ValidateSafeEnvExpansion(field, path string, allowedPaths []string) error`

**Security Features**:
- Path traversal prevention
- Environment variable injection prevention
- Whitelist-based path validation
- Input length limits

#### Security (`security.go`)
Security-focused utilities:

**Functions**:
- `GetAllowedPaths() ([]string, error)` - Returns allowed filesystem paths
- `IsSafePath(path string, allowedPaths []string) bool` - Validates path safety
- `SanitizeInput(input string) string` - Removes dangerous characters

### 3. Core Services

#### Retrieval Service (`pkg/retrieval`)
AI-powered engram retrieval using ecphory.

**3-Tier System**:
1. **Fast Filter**: Index-based filtering by tags/type
   - In-memory index scan
   - Tag matching
   - Type filtering
2. **API Ranking**: Claude AI relevance scoring
   - Semantic similarity
   - Context-aware ranking
   - Fallback to index-based if API unavailable
3. **Budget**: Token budget management
   - Limits results to fit within token budget
   - Prioritizes highest-ranked results

**Types**:
```go
type SearchOptions struct {
    EngramPath string
    Query      string
    SessionID  string
    Transcript string
    Tags       []string
    Type       string
    Limit      int
    UseAPI     bool
}

type SearchResult struct {
    Path    string
    Engram  *ecphory.Engram
    Score   float64
}
```

#### Health Check System (`internal/health`)
Comprehensive health monitoring.

**Components**:
- `HealthChecker` - Runs all health checks
- `Tier1Fixer` - Applies safe auto-fixes
- `Formatter` - Formats check results (default, quiet, JSON)
- `Logger` - Logs health check results to JSONL

**Check Types**:
- Infrastructure: workspace, config, logs exist
- Dependencies: yq, jq, python3 available
- Hooks: hook scripts exist and are executable
- Permissions: directories are writable
- Plugins: plugin health-check.sh scripts

**Cache**:
- Results cached to `~/.engram/cache/health-check.json`
- TTL-based cache invalidation
- `--no-cache` flag bypasses cache

#### Config Loader (`internal/config`)
Hierarchical configuration management.

**Loading Strategy**:
1. Load core config: `~/.engram/core/config.yaml`
2. Load user config: `~/.engram/user/config.yaml`
3. Merge configs (user overrides core)
4. Apply environment variables

**Config Structure**:
```yaml
plugins:
  paths:
    - ~/.engram/plugins
  disabled: []

ecphory:
  max_results: 10
  enable_api_ranking: true

telemetry:
  enabled: false
```

#### Plugin Loader (`internal/plugin`)
Dynamic plugin discovery and loading.

**Plugin Manifest**:
```yaml
name: plugin-name
version: 1.0.0
pattern: guidance|tool|connector
description: Plugin description

commands:
  - name: command-name
    script: ./scripts/command.sh

eventbus:
  subscribe:
    - event-name

permissions:
  filesystem:
    - ~/.engram/user
  network:
    - api.example.com
  commands:
    - git
```

**Loading Process**:
1. Scan plugin paths from config
2. Read manifest.yaml from each plugin
3. Validate plugin structure
4. Load enabled plugins (skip disabled)
5. Register commands and event subscriptions

### 4. Storage Layer

#### Ecphory Index (`pkg/ecphory`)
In-memory index for fast engram filtering.

**Index Structure**:
- Map of file path to engram metadata
- Frontmatter extraction: title, tags, type, agents
- Recursive directory scanning for `.ai.md` files

**Operations**:
- `Build(path string) error` - Build index from directory
- `All() []*Engram` - Get all engrams
- `FilterByTag(tag string) []*Engram` - Filter by tag
- `FilterByType(type string) []*Engram` - Filter by type

#### Memory Provider (`pkg/memory`)
Pluggable memory storage backends.

**Providers**:
- `simple` - File-based storage (default)
- Future: sqlite, postgres, redis

**Interface**:
```go
type Provider interface {
    Store(namespace, memoryID string, content []byte) error
    Retrieve(namespace string, query Query) ([]Memory, error)
    Update(namespace, memoryID string, update Update) error
    Delete(namespace, memoryID string) error
}
```

**Four-Tier Memory**:
1. Working Context - Active session data
2. Session History - Recent session logs
3. Long-term Memory - Persistent patterns
4. Artifacts - Generated files

#### Index Cache
Persistent index storage for fast startup.

**Cache Files**:
- `~/.engram/cache/index-user.json`
- `~/.engram/cache/index-team.json`
- `~/.engram/cache/index-company.json`
- `~/.engram/cache/index-core.json`

**Cache Strategy**:
- Build index on first use
- Save to cache file
- Load from cache on subsequent use
- Invalidate on file changes (future)

## Data Flow

### Retrieve Command Flow

```
1. User Input
   └─> engram retrieve "query" --tag python --limit 5
       │
2. Validation
   └─> ValidateRangeInt(limit, 1, 100)
   └─> ValidateOutputFormat(format)
   └─> ValidateSafePath(path)
       │
3. Query Processing
   └─> getRetrieveQuery(args) -> "query"
   └─> buildSearchOptions() -> SearchOptions
       │
4. Retrieval Service
   └─> Service.Search(ctx, opts)
       │
       ├─> Tier 1: Fast Filter
       │   └─> Index.FilterByTag("python")
       │   └─> Index.FilterByType()
       │       │
       ├─> Tier 2: API Ranking
       │   └─> Claude API ranks candidates
       │   └─> Fallback to index if API fails
       │       │
       └─> Tier 3: Budget
           └─> Limit results to top N
               │
5. Output Formatting
   └─> outputTable() or outputJSON()
       │
6. Display
   └─> Print results to stdout
```

### Doctor Command Flow

```
1. User Input
   └─> engram doctor --auto-fix
       │
2. Setup
   └─> HealthChecker.New(ctx)
   └─> Progress.Start("Running health checks...")
       │
3. Health Checks
   └─> checker.RunAllChecks()
       │
       ├─> InfrastructureCheck (workspace, config, logs)
       ├─> DependencyCheck (yq, jq, python3)
       ├─> HookCheck (hook scripts)
       ├─> PermissionCheck (writability)
       └─> PluginCheck (plugin health)
           │
4. Cache
   └─> WriteCache(results)
       │
5. Auto-Fix (if --auto-fix)
   └─> Tier1Fixer.PreviewFixes(results)
   └─> User confirmation
   └─> Tier1Fixer.ApplyFixes(fixes)
   └─> Re-run checks
       │
6. Output
   └─> Formatter.FormatDefault() or FormatJSON() or FormatQuiet()
       │
7. Logging
   └─> Logger.AppendToLog(results)
       │
8. Exit
   └─> os.Exit(exitCode)
```

### Init Command Flow

```
1. User Input
   └─> engram init
       │
2. Home Directory
   └─> os.Getenv("HOME")
   └─> workspaceDir = $HOME/.engram
       │
3. Create Directories
   └─> os.MkdirAll(~/.engram)
   └─> os.MkdirAll(~/.engram/user)
   └─> os.MkdirAll(~/.engram/logs)
   └─> os.MkdirAll(~/.engram/cache)
       │
4. Detect Repository
   └─> detectEngramRepo()
       │
       ├─> Strategy 1: From binary path
       │   └─> os.Executable() -> navigate up
       │       │
       ├─> Strategy 2: Common paths
       │   └─> Check ./engram
       │   └─> Check ~/engram
       │   └─> Check ~/go/src/github.com/vbonnet/engram
       │       │
       └─> Strategy 3: System paths
           └─> Check ~/.local/share/engram
           └─> Check /usr/local/share/engram
               │
5. Create Core Symlink
   └─> os.Symlink(repoPath, ~/.engram/core)
       │
6. Create User Config
   └─> os.WriteFile(~/.engram/user/config.yaml, defaultConfig)
       │
7. Success Message
   └─> Print next steps
```

## Security Model

### Path Validation
All filesystem paths are validated against an allowlist:

**Allowed Paths**:
- `~/.engram/` - Engram workspace
- `~/` - User home directory (restricted operations)
- `/tmp/engram-*` - Test paths

**Validation Steps**:
1. Resolve symlinks: `filepath.EvalSymlinks(path)`
2. Get absolute path: `filepath.Abs(path)`
3. Check against allowlist
4. Reject if outside allowed paths

### Input Sanitization
- Query length limits
- Format validation (table|json)
- Tier validation (user|team|company|core|all)
- Environment variable expansion validation

### Plugin Security
- Permission manifest (filesystem, network, commands)
- Sandboxed execution (future)
- Disabled plugin list in config

## Testing Strategy

### Unit Tests
- Command flag parsing
- Input validation
- Error handling
- Output formatting

### Integration Tests
- End-to-end command execution
- File system operations
- Health check system
- Memory storage

### Security Tests
- Path traversal attempts
- Environment variable injection
- Input length attacks
- Format injection

## Performance Considerations

### Index Performance
- In-memory index for fast lookups
- Lazy loading (build on first use)
- Cache persistence for fast startup
- Incremental rebuild (future)

### Retrieval Performance
- Fast filter tier for quick results
- API ranking only when needed (`--no-api` to skip)
- Token budget to limit result size
- Parallel index scanning (future)

### Health Check Performance
- Cache to avoid repeated checks
- Parallel check execution (future)
- Incremental fixes only

## Extensibility

### Adding New Commands
1. Create `cmd/newcommand.go`
2. Define cobra.Command struct
3. Implement RunE function
4. Add to root command in init()

### Adding New Validators
1. Add function to `internal/cli/validation.go`
2. Return custom error with suggestions
3. Document in `validation_doc.go`

### Adding New Providers
1. Implement Provider interface
2. Register in provider factory
3. Add configuration schema
4. Document in SPEC.md

### Adding New Plugins
1. Create plugin directory
2. Add manifest.yaml
3. Implement scripts
4. Add to plugin paths in config
