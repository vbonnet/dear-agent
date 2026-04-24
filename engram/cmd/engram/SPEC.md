# Engram CLI - Technical Specification

## Overview

The Engram CLI is a command-line interface for managing AI agent learning and memory persistence. It enables AI coding agents (Claude Code, Cursor, Windsurf) to learn from experience through memory traces (engrams) stored as markdown files.

**Repository**: `github.com/vbonnet/engram`
**Package**: `core/cmd/engram`
**Language**: Go
**Framework**: Cobra CLI
**Version**: v0.1.0-prototype

## Core Capabilities

### 1. Workspace Management

#### `engram init`
Initializes the Engram workspace with required directory structure.

**Creates**:
- `~/.engram/` - Workspace directory
- `~/.engram/user/` - User directory for custom engrams
- `~/.engram/core` - Symlink to engram repository
- `~/.engram/logs/` - Logs directory
- `~/.engram/cache/` - Cache directory
- `~/.engram/user/config.yaml` - User configuration file

**Detection Strategy**:
1. Development build: Navigate up from binary to find repo root
2. Common paths: `./engram`, `~/engram`, `~/go/src/github.com/vbonnet/engram`
3. System paths: `~/.local/share/engram`, `/usr/local/share/engram`, `/opt/engram`

**Idempotent**: Safe to run multiple times.

#### `engram doctor`
Comprehensive health checks for Engram infrastructure.

**Checks**:
- Core infrastructure (workspace, config, logs)
- Dependencies (yq, jq, python3)
- Hooks (configuration and scripts)
- File permissions
- Plugin health (if plugins implement health-check.sh)

**Flags**:
- `--auto-fix` - Apply safe auto-fixes (Tier 1 operations)
- `--quiet` - Only show issues, silent if healthy
- `--json` - Output JSON for automation
- `--no-cache` - Force fresh checks, bypass cache
- `--no-color` - Disable color output

**Exit Codes**:
- 0 - All checks passed (healthy)
- 1 - Warnings present (degraded)
- 2 - Errors present (critical)

### 2. Engram Retrieval

#### `engram retrieve [query]`
AI-powered search using ecphory (3-tier retrieval system).

**Retrieval Tiers**:
1. **Fast filter**: Index-based filtering by tags/type
2. **API ranking**: Claude AI ranks candidates by relevance
3. **Budget**: Returns top results within token budget

**Query Syntax**:
```bash
# Positional argument
engram retrieve "How do I handle errors in Go?"

# Flag syntax
engram retrieve --query "Python async patterns"
engram retrieve -q "Testing patterns"
```

**Flags**:
- `-p, --path` - Path to engrams directory (default: "engrams")
- `--tag` - Filter by tag before ranking
- `-t, --type` - Filter by type (pattern, strategy, howto, principle)
- `-n, --limit` - Maximum results (default: 10, range: 1-100)
- `--no-api` - Skip API ranking (fast index-based filter only)
- `--auto` - Auto-detect context from git repo or directory
- `--format` - Output format: table or json

**Output Formats**:
- **Table**: Human-readable with title, type, tags, relevance score, preview
- **JSON**: Machine-readable with query, candidates, metadata

**Security**:
- Path traversal validation
- Safe path checking against allowed directories
- Query length validation (max defined in cli.MaxQueryLength)

### 3. Index Management

#### `engram index rebuild`
Rebuild engram indexes for fast filtering.

**Tiers**: `user`, `team`, `company`, `core`, `all` (default: all)

**Flags**:
- `--tier` - Tier to rebuild (user|team|company|core|all)
- `--incremental` - Incremental rebuild (future feature)
- `--verify` - Verify index after rebuild

**Process**:
1. Scans tier directory recursively for `.ai.md` files
2. Extracts frontmatter metadata
3. Builds in-memory index
4. Reports engram count and duration

#### `engram index verify`
Verify index health across all tiers.

**Checks**:
- Index file exists
- Index is not corrupted
- Index is up-to-date with files
- Statistics (count, last updated)

### 4. Memory Management

#### `engram memory`
Pluggable storage backends for AI agent memory.

**Four Tiers**:
1. **Working Context** - Active session data
2. **Session History** - Recent session logs
3. **Long-term Memory** - Persistent patterns and learnings
4. **Artifacts** - Generated files and outputs

**Subcommands**:
- `store` - Store a new memory entry
- `retrieve` - Retrieve memories matching a query
- `update` - Update an existing memory entry
- `delete` - Delete a memory entry

**Configuration Priority**:
1. `--provider` flag
2. `ENGRAM_MEMORY_PROVIDER` env
3. Default: "simple"

**Config Path Priority**:
1. `--config` flag
2. `ENGRAM_MEMORY_CONFIG` env
3. Default: `~/.engram/memory.yaml`

**Note (v0.1.0)**: The `--config` flag specifies storage directory path, not a config file. Full YAML config file support planned for v0.2.0.

### 5. Plugin System

#### `engram plugin list`
List all loaded plugins from configured paths.

**Displays**:
- Plugin name, pattern (guidance/tool/connector), description
- Version
- Path
- Commands
- EventBus subscriptions
- Permissions (verbose mode)

**Flags**:
- `-v, --verbose` - Show full details including permissions

**Plugin Patterns**:
- **Guidance**: Provides AI agent instructions
- **Tool**: Executable commands
- **Connector**: External service integrations

**Permissions**:
- Filesystem access paths
- Network access endpoints
- Allowed commands

### 6. Token Estimation

#### `engram tokens estimate`
Estimate token counts for engram files.

**Tokenization Methods**:
- `char/4` - Simple heuristic (character count / 4)
- `tiktoken` - OpenAI's cl100k_base encoding (if available)
- `simple` - Word-based tokenizer (if available)

**Use Case**: Optimize engram files to stay within token limits.

### 7. Analytics

#### `engram analytics`
Analyze Wayfinder session metrics from telemetry data.

**Subcommands**:
- `list` - List all Wayfinder sessions
- `show` - Show detailed session timeline
- `summary` - Show aggregate statistics
- `persona` - Persona-specific analytics
- `ecphory` - Ecphory retrieval analytics
- `wayfinder` - Wayfinder session analytics

**Metrics**:
- Session durations
- Phase breakdowns
- AI time vs. wait time
- Estimated costs

**Telemetry Path Priority**:
1. `--telemetry-path` flag
2. `ENGRAM_TELEMETRY_PATH` env
3. Default: `~/.claude/telemetry.jsonl`

### 8. Documentation Backfill

#### `engram backfill`
Generate missing documentation for existing projects using hybrid analysis (codebase scanning + LLM synthesis).

**Subcommands**:
- `backfill-spec` - Generate SPEC.md from codebase analysis
- `backfill-architecture` - Generate ARCHITECTURE.md from codebase analysis
- `backfill-adrs` - Generate ADR files from git history and code

**Flags**:
- `--project-dir` - Project directory to analyze (default: ".")

**Requirements**:
- Python 3.9+ installed
- Documentation skills available in the git history
- ANTHROPIC_API_KEY environment variable

**Note**: Python skill implementations are in development. CLI integration is ready.

### 9. Documentation Review

#### `engram review`
Validate documentation quality using LLM-as-judge + Multi-Persona review.

**Subcommands**:
- `review-spec` - Validate SPEC.md quality
- `review-architecture` - Validate ARCHITECTURE.md quality
- `review-adr` - Validate ADR file quality

**Flags**:
- `--file` - Path to documentation file (required)

**Quality Thresholds**:
- Score ≥8/10 = PASS
- Score 6-7/10 = WARN (improvements recommended)
- Score <6/10 = FAIL (blocking, requires fixes)

**Requirements**:
- Python 3.9+ installed
- Review skills available in the git history
- ANTHROPIC_API_KEY environment variable

### 10. Configuration Management

#### `engram config`
Manage Engram configuration using 4-tier hierarchy (Core → Company → Team → User).

**Subcommands**:
- `show` - Display effective configuration with sources

**Flags**:
- `--json` - JSON output format

**Configuration Priority**:
1. Core - Embedded defaults (~/.engram/core/config.yaml)
2. Company - Enterprise settings (/etc/engram/config.yaml)
3. Team - Project settings (.engram/config.yaml)
4. User - Personal settings (~/.engram/user/config.yaml)

Environment variables and CLI flags take precedence over all configuration files.

### 11. Telemetry Management

#### `engram telemetry`
Manage telemetry configuration and view current settings.

**Subcommands**:
- `status` - Show whether telemetry is enabled/disabled
- `enable` - Enable telemetry collection
- `disable` - Disable telemetry collection (GDPR opt-out)
- `show` - Show detailed telemetry configuration
- `loaded` - Show engrams loaded in current session (last 30 minutes)

**Privacy**:
- Data stored locally in ~/.engram/telemetry/ by default
- No code content, file paths, or personal data collected
- Can be disabled anytime (unless enforced by organization)

**Loaded Engrams Flags**:
- `--format` - Output format: table or json (default: "table")
- `--detailed` - Show additional fields (load_when, timestamp)

### 12. Guidance Search

#### `engram guidance`
Search and manage guidance files (*.ai.md) in engram library.

**Subcommands**:
- `search [query]` - Search guidance files by keyword

**Search Flags**:
- `-p, --path` - Path to engrams directory (default: auto-detect)
- `--domain` - Filter by domain (e.g., go, python, hipaa)
- `-t, --type` - Filter by type (pattern, workflow, reference)
- `--tag` - Filter by tag
- `-n, --limit` - Maximum number of results (1-100, default: 10)
- `--format` - Output format: table, json, paths (default: "table")

**Search Behavior**:
Matches query against frontmatter metadata (title, description, tags, domain).

### 13. Slash Command Utilities

#### `engram slashcmd`
Utilities for working with enhanced slash commands.

**Subcommands**:
- `parse COMMAND_FILE` - Parse and display slash command metadata
- `autocomplete COMMAND_FILE PARAM_NAME` - Get autocomplete values for a parameter

**Security**: Path validation prevents path traversal attacks. Only allows access to ~/.claude/commands directory.

### 14. Sub-Agent Management

#### `engram subagent`
Manage Claude Code sub-agents for interactive workflows.

**Subcommands**:
- `writer` - Invoke engram-writer sub-agent for pattern creation
- `reviewer <engram-file>` - Invoke engram-reviewer sub-agent for quality analysis
- `wayfinder <phase>` - Invoke wayfinder-phase sub-agent for SDLC phase execution

**Wayfinder Phases**:
D1, D2, D3, D4, S4, S5, S6 (CRITICAL), S7, S8, S9, S10 (CRITICAL), S11

**Note**: Sub-agents require Claude Code. On other platforms, fallback guidance is provided.

### 15. File Hashing

#### `engram hash <file>`
Calculate SHA-256 hash of a file.

**Output Format**: `sha256:{hex_hash}`

**Features**:
- Tilde (~) expansion for home directory paths
- Outputs hash to stdout (no newline) for easy piping
- Returns error if file cannot be read or does not exist

**Use Case**: Used by Wayfinder to calculate phase engram hashes for methodology freshness validation.

### 16. Shell Completion

#### `engram completion [shell]`
Generate auto-completion scripts for bash, zsh, fish, powershell.

**Installation**:
```bash
# Bash
source <(engram completion bash)
echo 'source <(engram completion bash)' >> ~/.bashrc

# Zsh
source <(engram completion zsh)
echo 'source <(engram completion zsh)' >> ~/.zshrc

# Fish
engram completion fish > ~/.config/fish/completions/engram.fish

# PowerShell
engram completion powershell | Out-String | Invoke-Expression
```

## Architecture Patterns

### Command Structure
- **Root Command**: `engram` - Main entry point with version info
- **Subcommands**: Organized by domain (init, doctor, retrieve, index, memory, plugin, tokens, analytics, backfill, review, config, telemetry, guidance, slashcmd, subagent, hash, completion)
- **Flags**: Global persistent flags and command-specific flags
- **Validation**: Input validation via `internal/cli` package

### Error Handling
- **Structured Errors**: Custom error types in `internal/cli/errors.go`
- **User-Friendly**: Contextual suggestions for common mistakes
- **Exit Codes**: Consistent exit code patterns

### Output Formatting
- **Progress Indicators**: Spinner for long-running operations
- **Success/Warning/Error Icons**: Visual feedback
- **Multiple Formats**: table, json for machine consumption
- **Color Support**: Optional color output with `--no-color`

### Security
- **Path Validation**: Prevent path traversal attacks
- **Safe Environment Variables**: Validate env expansion
- **Allowed Paths**: Whitelist-based path access
- **Input Sanitization**: Query length limits, format validation

### Configuration
- **Hierarchical Loading**: User config overrides core config
- **Environment Variables**: Prefix: `ENGRAM_*`
- **Config File**: `~/.engram/user/config.yaml`
- **Default Config**: Generated on `engram init`

## Dependencies

### Core Libraries
- `github.com/spf13/cobra` - CLI framework
- `github.com/google/uuid` - UUID generation
- `github.com/vbonnet/engram/core/pkg/*` - Internal packages

### Internal Packages
- `internal/cli` - CLI utilities (errors, output, validation, security)
- `internal/health` - Health check system
- `internal/config` - Configuration management
- `internal/plugin` - Plugin system
- `internal/context` - Context detection
- `pkg/retrieval` - Ecphory retrieval system
- `pkg/ecphory` - Index management

## Environment Variables

| Variable | Purpose | Default |
|----------|---------|---------|
| `ENGRAM_HOME` | Workspace directory | `~/.engram` |
| `ENGRAM_MEMORY_PROVIDER` | Memory storage provider | `simple` |
| `ENGRAM_MEMORY_CONFIG` | Memory config path | `~/.engram/memory.yaml` |
| `ENGRAM_TELEMETRY_PATH` | Telemetry file path | `~/.claude/telemetry.jsonl` |
| `HOME` | User home directory | System default |

## File Structure

```
~/.engram/
├── user/              # User-specific engrams
│   └── config.yaml    # User configuration
├── core -> /path/to/engram  # Symlink to repository
├── logs/              # Health check logs
├── cache/             # Index cache
├── team/              # Team engrams (optional)
├── company/           # Company engrams (optional)
└── memory.yaml        # Memory storage config
```

## Version Information

The CLI reports version information from build-time ldflags:
- `version` - Version tag (e.g., "v0.1.0-prototype")
- `commit` - Git commit SHA
- `date` - Build date
- `builtBy` - Builder (e.g., "goreleaser", "manual")
- Go version, OS, architecture

## Future Enhancements

### Planned Features
- Memory config YAML support (v0.2.0)
- Incremental index rebuild
- Plugin enable/disable commands
- Plugin info command
- Extended analytics dashboards
- Multi-language tokenization support

### Extensibility
- Plugin architecture for custom workflows
- EventBus for plugin communication
- Pluggable memory backends
- Custom retrieval strategies

## Testing

### Test Coverage
- Unit tests: `*_test.go` files
- Integration tests: `*_integration_test.go` files
- Command tests: `cmd/*_test.go`
- Validation tests: `internal/cli/validation_test.go`
- Security tests: `internal/cli/security_test.go`

### Test Strategy
- Mock external dependencies
- Test error paths
- Validate security constraints
- Test CLI flag parsing
- Test output formatting
