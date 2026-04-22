# AGM Architecture Overview

Complete architectural documentation for the AI/Agent Gateway Manager (AGM).

**Version**: 3.2
**Last Updated**: 2026-02-20

---

## Table of Contents

- [System Overview](#system-overview)
- [C4 Component Diagram](#c4-component-diagram)
- [Core Architecture](#core-architecture)
- [Component Structure](#component-structure)
- [Data Flow](#data-flow)
- [Storage Architecture](#storage-architecture)
  - [Directory Structure](#directory-structure)
  - [Manifest Versioning](#manifest-versioning)
  - [Git Auto-Commit](#git-auto-commit)
- [Multi-Agent System](#multi-agent-system)
- [Multi-Session Coordination](#multi-session-coordination)
- [Session Lifecycle](#session-lifecycle)
- [Session Initialization](#session-initialization)
- [Command Translation Layer](#command-translation-layer)
- [Security Model](#security-model)
- [Performance Considerations](#performance-considerations)

---

## System Overview

### What is AGM?

AGM (AI/Agent Gateway Manager) is a sophisticated session management system that provides unified access to multiple AI agents (Claude, Gemini, Codex/OpenAI, OpenCode) through a consistent command-line interface. It evolved from Agent Session Manager (AGM) to support multi-agent workflows.

### Design Principles

1. **Multi-agent abstraction** - Single interface for multiple AI providers
2. **Session persistence** - Long-lived conversations across terminal sessions
3. **Backward compatibility** - AGM sessions migrate seamlessly
4. **Tmux integration** - Leverages tmux for session management
5. **Zero-downtime** - Sessions persist across reboots and network interruptions
6. **Explicit configuration** - No hidden state, manifest-driven design

### High-Level Architecture

```
┌─────────────────────────────────────────────────────────────────────┐
│                        AGM CLI                                       │
│  (Unified command interface for all agents)                          │
└────────┬────────────────────────────────┬────────────────────────────┘
         │                                 │
    ┌────▼─────┐                     ┌────▼─────┐      ┌──────────────┐
    │  Command │                     │ Session  │      │ Coordination │
    │Translator│                     │ Manager  │◄─────│   Daemon     │
    └────┬─────┘                     └────┬─────┘      └──────┬───────┘
         │                                 │                   │
    ┌────▼──────────────────┐         ┌───▼──────┐      ┌─────▼──────┐
    │   Agent Adapters      │         │ Manifest │      │  Message   │
    │ ┌──────┬──────┬──────┬────────┐│         │  Store   │      │   Queue    │
    │ │Claude│Gemini│Codex │OpenCode││         └──────────┘      │ (SQLite)   │
    │ └──┬──┴───┬──┴───┬──┘ │                           └────────────┘
    └────│──────│──────│────┘
         │      │      │
    ┌────▼──────▼──────▼────┐
    │   Tmux Integration    │
    │  (Session multiplexer)│
    └───────────────────────┘
```

---

## C4 Component Diagram

### Internal Architecture (Level 3)

The AGM (Agent Gateway Manager) system is visualized using a C4 Component diagram showing the internal architecture:

**Diagrams**:
- [C4 Component Diagram (SVG)](../diagrams/rendered/c4-component-agm.svg)
- [C4 Component Diagram (PNG)](../diagrams/rendered/c4-component-agm.png)
- [D2 Source](../diagrams/c4-component-agm.d2)

The diagram shows:

- **Command Layer**: CLI interface with session, admin, and workflow commands
- **Business Logic Layer**: SessionManager, AgentRouter, ManifestStore, UUIDDetector
- **Agent Abstraction Layer**: CommandTranslator with adapters for Claude, Gemini, Codex, OpenCode
- **Integration Layer**: TmuxController, MCPServerManager, ProcessMonitor, LockManager
- **Storage Layer**: ManifestRepository (v2/v3), HistoryManager, BackupManager, MigrationEngine
- **Coordination Layer**: CoordinationDaemon, MessageQueue (SQLite)

**Key architectural features**:
- Multi-agent support through adapter pattern
- Tmux-based session persistence
- Manifest-driven configuration (v2 with v3 migration path)
- Message queue for multi-session coordination
- Graceful degradation for unsupported agent features

---

## Core Architecture

### Component Layers

AGM is organized into distinct functional layers:

#### 1. CLI Layer (`cmd/csm/`)
- Command parsing (Cobra framework)
- Flag handling and validation
- User interaction (Huh TUI library)
- Error presentation
- Output formatting (table, JSON, simple)

#### 2. Business Logic Layer (`internal/`)
- Session lifecycle management
- Agent routing and selection
- UUID detection and association
- Backup and restore operations
- Health checking and diagnostics

#### 3. Agent Abstraction Layer (`internal/agent/`, `internal/command/`)
- CommandTranslator interface
- Agent-specific adapters (Claude, Gemini, Codex/OpenAI, OpenCode)
- Command normalization
- Graceful degradation for unsupported features

#### 4. Integration Layer (`internal/tmux/`, `internal/mcp/`)
- Tmux control mode integration
- Socket management
- Process monitoring
- Lock management

#### 5. Storage Layer (`internal/manifest/`, `internal/history/`)
- Manifest schema (v2, v3)
- Conversation history
- Backup management
- Migration utilities

---

## Component Structure

### Internal Packages

```
internal/
├── agent/              # Agent abstraction and adapters
│   ├── interface.go    # Agent interface definition
│   ├── claude_adapter.go   # Claude-specific implementation
│   ├── gemini_cli_adapter.go  # Gemini CLI-specific implementation
│   ├── openai_adapter.go   # OpenAI/Codex API-based implementation
│   └── opencode_adapter.go # OpenCode SSE-based implementation
│
├── command/            # Command translation layer
│   ├── translator.go   # CommandTranslator interface
│   ├── claude.go       # Claude command translator
│   └── gemini.go       # Gemini command translator
│
├── session/            # Session management
│   ├── manager.go      # Session CRUD operations
│   ├── status.go       # Status computation (active/stopped/archived)
│   └── lifecycle.go    # Lifecycle events
│
├── manifest/           # Manifest schema and operations
│   ├── manifest.go     # Manifest v2 schema
│   ├── v3.go           # Manifest v3 schema (future)
│   ├── reader.go       # Manifest reading
│   └── writer.go       # Manifest writing
│
├── tmux/               # Tmux integration
│   ├── tmux.go         # Core tmux operations
│   ├── init_sequence.go    # Session initialization orchestration
│   ├── control.go      # Control mode (-C) for programmatic control
│   ├── output_watcher.go   # Output stream monitoring
│   ├── prompt_detector.go  # Claude prompt detection via capture-pane
│   ├── socket.go       # Unix socket management
│   ├── lock.go         # Global tmux lock
│   └── send_command.go # Send literal commands to tmux sessions
│
├── history/            # Conversation history
│   ├── parser.go       # Parse ~/.claude/history.jsonl
│   └── search.go       # Semantic search (Vertex AI)
│
├── detection/          # UUID auto-detection
│   ├── detector.go     # Hybrid detection algorithm
│   └── confidence.go   # Confidence scoring
│
├── ui/                 # Interactive TUI components
│   ├── picker.go       # Session picker
│   ├── forms.go        # Multi-step forms
│   └── cleanup.go      # Multi-select cleanup
│
├── fuzzy/              # Fuzzy matching (Levenshtein)
│   └── match.go        # Distance calculation, similarity threshold
│
├── git/                # Git auto-commit for manifest changes
│   └── git.go          # Auto-commit manifest files to git repos
│
├── lock/               # Locking primitives
│   └── filelock.go     # File-based locking with timeout
│
├── backup/             # Backup and restore
│   ├── backup.go       # Create numbered backups
│   └── restore.go      # Restore from backup
│
├── messages/           # Message queue and acknowledgments (Phase 2)
│   ├── queue.go        # SQLite message queue implementation
│   ├── ack.go          # Acknowledgment protocol manager
│   ├── logger.go       # JSONL message logging
│   ├── id.go           # Message ID generation
│   └── rate_limit.go   # Token bucket rate limiter
│
├── daemon/             # AGM daemon (Phase 2)
│   ├── daemon.go       # Main daemon logic (polling, delivery)
│   ├── config.go       # Daemon configuration
│   └── state.go        # Session state detection
│
├── workflow/           # Workflow automation
│   └── deep_research.go    # Deep research workflow (Gemini)
│
└── gateway/            # Gateway layer (experimental)
    └── router.go       # Route requests to appropriate agent
```

### Key Abstractions

#### Agent Interface

```go
type Agent interface {
    // Start agent CLI session
    Start(ctx context.Context, sessionID string, opts *StartOptions) error

    // Check if agent is available (API keys configured)
    IsAvailable() bool

    // Get agent metadata
    GetMetadata() *AgentMetadata

    // Translate commands (see CommandTranslator)
    GetTranslator() command.Translator
}
```

#### CommandTranslator Interface

```go
type Translator interface {
    // Rename session/conversation
    RenameSession(ctx context.Context, sessionID, newName string) error

    // Set working directory context
    SetDirectory(ctx context.Context, sessionID, dirPath string) error

    // Run initialization hook (agent-specific)
    RunHook(ctx context.Context, sessionID, hookType string) error
}
```

#### Manifest Schema (v2)

```yaml
version: "2.0"
session_id: "a1b2c3d4-e5f6-7890-abcd-ef1234567890"  # UUID format
tmux_session_name: "my-coding-session"
agent: "claude"  # claude, gemini, gpt
lifecycle: "active"  # active, stopped, archived
context:
  project: "~/projects/myapp"
  tags: ["feature", "backend"]
metadata:
  created_at: "2026-02-04T10:00:00Z"
  updated_at: "2026-02-04T14:30:00Z"
claude:
  uuid: "abc-def-123"  # Agent-specific UUID
  version: "0.7.1"
```

---

## Data Flow

### Session Creation Flow

```
1. User: agm new --harness gemini-cli research-task
        │
2. CLI: Parse command, validate flags
        │
3. Session Manager: Create manifest
        │
4. Tmux: Create new tmux session
        │
5. Agent Adapter: Start Gemini CLI
        │
6. UUID Detector: Monitor history.jsonl
        │
7. Manifest Writer: Update manifest with UUID
        │
8. CLI: Attach user to tmux session
```

### Session Resume Flow

```
1. User: agm resume research-task
        │
2. Session Manager: Load manifest
        │
3. Validator: Check session health
        │ (manifest valid, tmux session exists)
        │
4. Tmux: Attach to existing tmux session
        │
5. Agent: Already running, context restored
```

### Bulk Resume Flow (resume-all)

```
1. User: agm sessions resume-all [--workspace-filter alpha]
        │
2. Session Manager: List all manifests
        │
3. Filter Pipeline:
        │ → filterNonArchived() - Remove archived sessions (unless --include-archived)
        │ → filterByWorkspace() - Apply workspace filter (if specified)
        │
4. Batch Status Computation:
        │ → session.ComputeStatusBatch(manifests, tmuxClient)
        │ → Returns map[session_name]status ("active", "stopped", "archived")
        │
5. Filter Stopped Sessions:
        │ → Only process sessions with status == "stopped"
        │
6. Preview Display:
        │ → Show count: "Found 15 stopped sessions to resume"
        │ → If --dry-run, exit here
        │
7. Sequential Resume (with progress indicators):
        │ FOR EACH stopped session:
        │   ├─→ checkSessionHealth() - Validate worktree, manifest
        │   ├─→ resumeSession(detached=true) - Resume without attaching
        │   ├─→ Collect errors (if any)
        │   ├─→ time.Sleep(500ms) - Prevent tmux overload
        │   └─→ Update progress bar
        │
8. Orchestrator Coordination:
        │ FOR EACH successfully resumed session:
        │   └─→ writeResumeTimestamp(sessionID)
        │       → Creates .agm/resume-timestamp file
        │       → Timestamp used by orchestrator v2 for restart detection
        │       → See ADR-010 for integration details
        │
9. Summary Report:
        │ → Success count: 12
        │ → Failure count: 3
        │ → Error details for failed sessions
        │
10. Exit:
    → Exit code 0 if all succeeded
    → Exit code 1 if any failures (unless --continue-on-error)
```

**Key Features**:
- **Batch Processing**: Uses `ComputeStatusBatch()` for efficient status checking (single tmux call)
- **Sequential Resume**: 500ms delays between resumes prevent tmux server overload
- **Progress Feedback**: Charmbracelet Bubbles spinner + progress bar for long operations
- **Error Isolation**: One failure doesn't stop remaining resumes (default behavior)
- **Orchestrator Integration**: Writes timestamp files for orchestrator v2 coordination (ADR-010)

**Performance Characteristics**:
- 20 sessions: ~12 seconds (500ms per session + overhead)
- 50 sessions: ~30 seconds
- Batch status computation: O(1) tmux calls (vs O(n) for individual checks)

**References**:
- Implementation: `cmd/agm/resume_all.go`
- Architecture Decision: `docs/adr/ADR-010-orchestrator-resume-detection.md`
- Boot Automation: `systemd/agm-resume-boot.service`

### Command Translation Flow

```
1. User (in session): /rename new-session-name
        │
2. Tmux: Capture command
        │
3. CommandTranslator: Translate to agent-specific format
        │
4. Claude: Send slash command via tmux
   OR
   Gemini: Call API (UpdateConversationTitle)
        │
5. Manifest Writer: Update manifest with new name
```

---

## Storage Architecture

### Directory Structure

```
~/sessions/                          # Unified session storage (v3)
├── my-coding-session/
│   ├── manifest.yaml                # Session manifest (v2 or v3)
│   ├── .backups/                    # Numbered manifest backups
│   │   ├── manifest.1               # Most recent backup
│   │   ├── manifest.2
│   │   └── manifest.3
│   └── conversations/               # Conversation history (optional)
│       └── history.jsonl
│
~/.claude/                           # Claude-specific storage
├── history.jsonl                    # Global conversation history
└── session-env/                     # Session environment cache
    └── <uuid>/                      # Per-session cache
        └── manifest.json

~/.config/agm/config.yaml            # User configuration
~/.agm/                              # AGM coordination infrastructure (Phase 2)
├── queue.db                         # SQLite message queue (WAL mode)
├── queue.db-shm                     # Shared memory file (WAL)
├── queue.db-wal                     # Write-ahead log (WAL)
├── daemon.pid                       # Daemon process ID (lock file)
├── logs/
│   ├── daemon/                      # Daemon logs
│   │   └── daemon.log
│   └── messages/                    # Message logs (JSONL per day)
│       ├── 2026-02-01.jsonl
│       ├── 2026-02-02.jsonl
│       └── 2026-02-03.jsonl
└── backups/                         # Automated backups
    └── YYYYMMDD-HHMMSS/
        ├── queue.db
        └── sessions.tar.gz
```

### Manifest Versioning

**v2 (Current)**:
- Session UUID
- Agent field
- Lifecycle field (active, stopped, archived)
- Context (project, tags)
- Agent-specific metadata

**v3 (Future)**:
- Unified storage structure
- Multi-conversation support
- Workflow metadata
- Rich tagging system

**Migration**: AGM reads v2 manifests and migrates on first write.

### Git Auto-Commit

AGM automatically commits manifest changes to git repositories when sessions are created or modified.

**Behavior**:
- Automatically detects if the sessions directory is within a git repository
- Commits only the specific manifest file that changed (not other staged/unstaged files)
- Works correctly even if the repository has unrelated staged or unstaged files
- Gracefully handles non-git directories (no-op, no error)
- Uses descriptive commit messages: `agm: <operation> session '<name>'`

**Supported Operations**:
- `create`: New session creation
- `archive`: Session archival
- `unarchive`: Session restoration
- `associate`: UUID association
- `resume`: Session resume (updates timestamp)
- `sync`: Manifest sync from tmux
- `create-child`: Child session creation

**Implementation**:
- `internal/git/git.go`: Core git integration
- Walks up directory tree to find git root
- Uses `git commit --only` to commit only the specified file
- Non-invasive: returns silently if not in a git repository

**Error Handling**:
- Git commit failures are logged as warnings but don't fail the operation
- Manifest is always written successfully before attempting commit
- Missing git binary or git errors are handled gracefully

**Example commit history**:
```
agm: create session 'my-coding-session'
agm: associate session 'my-coding-session'
agm: archive session 'old-project'
agm: unarchive session 'old-project'
```

### Dolt Database Storage (Migration in Progress)

**Status**: Phases 1-2 Complete (Dual-Write Mode) - 2026-03-14
**Migration Plan**: ``
**Documentation**: `docs/YAML-TO-DOLT-MIGRATION-PHASES-1-2.md`

AGM is migrating from filesystem-based YAML manifests to Dolt database storage for improved query performance, atomic operations, and version history.

#### Current State: Dual-Write Pattern

During the migration (Phases 1-2 complete, Phases 3-6 pending), AGM uses a dual-write pattern:

**Storage Operations by Command**:

| Command | YAML | Dolt | Status |
|---------|------|------|--------|
| `agm session new` | Write | Write | ✅ Dual-write (Phase 1) |
| `agm session list` | - | Read | ✅ Dolt-only |
| `agm session archive` | Write | Read+Write | ✅ Dual-write |
| `agm session resume` | - | Read | ✅ Dolt-only (Phase 6) |
| `agm session kill` | - | Read | ✅ Dolt-only (Phase 6) |
| Tab-completion | - | Read | ✅ Dolt-only (Phase 6) |

**Dolt Database Location**:
```
~/src/ws/<workspace>/.dolt/dolt-db/
├── .dolt/                    # Dolt repository metadata
├── agm_sessions/             # Main sessions table
└── dolt_schemas/             # Schema history
```

**Connection**:
- Protocol: MySQL-compatible (port 3307)
- Workspace isolation: `WORKSPACE` environment variable
- Adapter: `internal/dolt/adapter.go`

**Schema** (`agm_sessions` table):
```sql
CREATE TABLE agm_sessions (
    id VARCHAR(255) PRIMARY KEY,           -- Session ID (UUID)
    created_at TIMESTAMP,
    updated_at TIMESTAMP,
    status VARCHAR(50),                    -- Lifecycle: active, stopped, archived
    workspace VARCHAR(100),                -- Workspace name
    name VARCHAR(255),                     -- Human-readable session name
    agent VARCHAR(50),                     -- Agent: claude, gemini, gpt
    context_project TEXT,                  -- Project directory
    context_purpose TEXT,                  -- Session purpose
    context_tags JSON,                     -- Tags array
    context_notes TEXT,                    -- Session notes
    claude_uuid VARCHAR(255),              -- Claude-specific UUID
    tmux_session_name VARCHAR(255),        -- Tmux session identifier
    metadata JSON,                         -- Additional metadata
    INDEX idx_workspace (workspace),
    INDEX idx_status (status),
    INDEX idx_tmux_name (tmux_session_name)
);
```

#### Migration Timeline

**✅ Phase 1** (Complete): Emergency fix - `agm session new` writes to Dolt
**✅ Phase 2** (Complete): Bulk migration tool - `agm migrate migrate-yaml-to-dolt`
**🚧 Phase 3** (Next): Command layer migration - resume, kill, tab-completion
**🚧 Phase 4**: Internal modules migration
**🚧 Phase 5**: Test suite migration
**🚧 Phase 6**: YAML code deletion (Dolt-only)

#### Advantages

1. **Query Performance**: SQL-based filtering and sorting (workspace, status, tags)
2. **Atomic Operations**: ACID transactions for session updates
3. **Version History**: Git-like versioning of session data
4. **Workspace Isolation**: Database-level separation of workspaces
5. **Concurrent Access**: Multiple processes can safely query sessions

#### Backward Compatibility

During migration, YAML manifests are maintained for rollback safety:
- YAML writes continue (dual-write)
- Dolt is source of truth (reads prioritize Dolt)
- Rollback possible by reverting code changes
- No data loss if Dolt server unavailable (graceful degradation)

**Final State** (Post-Phase 6): Dolt-only, YAML code removed

---

## Multi-Agent System

### Agent Registry

AGM maintains a registry of available agents:

```go
type AgentRegistry struct {
    agents map[string]Agent
}

func (r *AgentRegistry) Register(name string, agent Agent)
func (r *AgentRegistry) Get(name string) (Agent, error)
func (r *AgentRegistry) List() []AgentInfo
```

### Agent Selection

**Default harness**: Claude Code (if `--harness` flag not specified)

**Selection order**:
1. Explicit `--harness` flag
2. AGENTS.md configuration (future)
3. Manifest `agent` field (for resume)
4. Default (claude)

### Agent Routing (AGENTS.md)

**Status**: Infrastructure complete, integration pending

**Concept**: Auto-select agent based on session name keywords:

```yaml
# ~/projects/.claude/AGENTS.md
default_agent: claude
preferences:
  - keywords: [research, papers, analysis]
    agent: gemini
  - keywords: [code, debug, refactor]
    agent: claude
  - keywords: [brainstorm, ideas, creative]
    agent: gpt
```

**Implementation**:
- YAML parsing: `internal/agents/parser.go`
- Keyword matching: `internal/agents/matcher.go`
- Multi-path detection: Search up directory tree for AGENTS.md

---

## Agent Interface Specification

**Version**: 2.0 (Updated 2026-03-11)
**Status**: Production (4 agents with full feature parity)
**Agents**: Claude CLI, Gemini CLI, Codex API, OpenCode API
**Phase 2 Complete**: Enhanced command execution across all agents

### Overview

AGM's multi-agent architecture is built on a unified Agent interface that all harness implementations must satisfy. This enables consistent session management, command translation, and state detection across different AI providers while allowing harness-specific optimizations.

### Core Agent Interface

All agent adapters implement the following 11 required methods:

```go
type Agent interface {
    // Session Lifecycle
    CreateSession(ctx SessionContext) (SessionID, error)
    ResumeSession(sessionID SessionID) error
    TerminateSession(sessionID SessionID) error

    // Session State
    GetSessionStatus(sessionID SessionID) (SessionStatus, error)

    // Communication
    SendMessage(sessionID SessionID, message string) error
    GetHistory(sessionID SessionID) ([]Message, error)

    // Data Exchange
    ExportConversation(sessionID SessionID, format ExportFormat) ([]byte, error)
    ImportConversation(sessionID SessionID, data []byte, format ImportFormat) error

    // Capabilities
    Capabilities() AgentCapabilities

    // Commands
    ExecuteCommand(sessionID SessionID, command Command) error

    // Metadata
    Name() string
    Version() string
}
```

### Harness Implementations

AGM supports four production-ready agent harnesses:

#### 1. Claude Adapter (`claude_adapter.go`)

**Provider**: Anthropic Claude Code CLI
**Architecture**: Local CLI binary with tmux integration
**Session Model**: UUID-based resume with conversation history

**Key Characteristics**:
- **Session Resume**: `claude --resume <uuid>` (native UUID support)
- **State Detection**: Hook-based + tmux pane scraping (fallback)
- **Working Directory**: `claude --add-dir <path>` (native support)
- **Persistence**: `~/.claude/history.jsonl` (automatic)
- **Multi-session**: Yes (multiple UUIDs tracked)

**Unique Features**:
- Directory authorization via `~/.claude/CLAUDE.md` association
- Slash commands (`/commit`, `/review-spec`, etc.)
- Hook system for lifecycle events
- MCP server integration
- Conversation compaction (auto-managed)

**State Detection Strategy**:
```go
// Primary: Hook-based state updates
// Fallback: Tmux pane scraping for prompt detection
func (a *ClaudeAdapter) detectState(sessionID string) State {
    // Check manifest state field (updated by hooks)
    if state := a.manifest.GetState(); state != "" {
        return state
    }

    // Fallback: Scrape tmux for "▌" prompt indicator
    return a.tmuxScraper.DetectPrompt(sessionID)
}
```

#### 2. Gemini CLI Adapter (`gemini_cli_adapter.go`)

**Provider**: Google Gemini CLI (CLI-based, similar to Claude Code)
**Architecture**: CLI-based with tmux integration and local state
**Session Model**: Local history with UUID-based resume

**Key Characteristics**:
- **Session Resume**: UUID-based via `--resume {uuid}` flag (with "latest" fallback)
- **State Detection**: Tmux scraping (same pattern as Claude)
- **Working Directory**: Native CLI support via `--include-directories` flag
- **Persistence**: Local session metadata + Gemini's native session storage
- **Multi-session**: Yes (multiple tmux sessions with unique UUIDs)

**Unique Features**:
- UUID extraction from `--list-sessions` output
- Directory pre-authorization (no interactive prompts)
- Graceful fallback to "latest" session if UUID unavailable
- Integration with Gemini's native session management
- Hook system support (SessionStart, SessionEnd, BeforeAgent, AfterAgent)

**Phase 2 Enhancements (2026-03-11)**:
- ✅ Full command execution parity with Claude
- ✅ CommandSetDir: Directory changes via tmux + metadata tracking
- ✅ CommandClearHistory: History file management
- ✅ CommandSetSystemPrompt: System prompt in session metadata
- ✅ Comprehensive BDD test coverage across all 4 agents

**UUID-Based Resume Pattern**:
```go
// Resume with UUID or fallback to latest
func (a *GeminiCLIAdapter) ResumeSession(sessionID SessionID) error {
    metadata, _ := a.sessionStore.Get(sessionID)

    var resumeCmd string
    if metadata.UUID != "" {
        // Resume specific session by UUID
        resumeCmd = fmt.Sprintf("cd '%s' && gemini --resume %s && exit",
            metadata.WorkingDir, metadata.UUID)
    } else {
        // Fallback to latest session
        resumeCmd = fmt.Sprintf("cd '%s' && gemini --resume latest && exit",
            metadata.WorkingDir)
    }

    return tmux.SendCommand(metadata.TmuxName, resumeCmd)
}
```

**Directory Authorization Pattern**:
```go
// Pre-authorize directories to avoid interactive prompts
func (a *GeminiCLIAdapter) CreateSession(ctx SessionContext) (SessionID, error) {
    geminiCmd := fmt.Sprintf("gemini --include-directories '%s'",
        ctx.WorkingDirectory)

    // Add additional authorized directories
    for _, dir := range ctx.AuthorizedDirs {
        if dir != ctx.WorkingDirectory {
            geminiCmd += fmt.Sprintf(" --include-directories '%s'", dir)
        }
    }

    geminiCmd += " && exit"
    // ... execute command in tmux
}
```

**State Detection Strategy**:
```go
// Tmux-based detection (same as Claude)
func (a *GeminiCLIAdapter) detectState(sessionID string) State {
    metadata, _ := a.sessionStore.Get(sessionID)

    // Check if tmux session exists
    exists, _ := tmux.HasSession(metadata.TmuxName)
    if !exists {
        return StateTerminated
    }

    // Check if Gemini CLI is running
    running, _ := tmux.IsProcessRunning(metadata.TmuxName, "gemini")
    if running {
        return StatusActive
    }

    return StatusTerminated
}
```

**Enhanced Command Execution (Phase 2)**:
```go
// ExecuteCommand implements full command translation
func (a *GeminiCLIAdapter) ExecuteCommand(cmd Command) error {
    sessionID := SessionID(cmd.Params["session_id"].(string))
    metadata, _ := a.sessionStore.Get(sessionID)

    switch cmd.Type {
    case CommandSetDir:
        // 1. Send cd to tmux (immediate effect on CLI)
        newPath := cmd.Params["path"].(string)
        tmux.SendCommand(metadata.TmuxName, fmt.Sprintf("cd %s\r", newPath))

        // 2. Update metadata (persisted state tracking)
        metadata.WorkingDir = newPath
        return a.sessionStore.Set(sessionID, metadata)

    case CommandClearHistory:
        // Remove history file
        historyPath := filepath.Join(homeDir, ".gemini", "sessions",
            metadata.TmuxName, "history.jsonl")
        return os.Remove(historyPath)

    case CommandSetSystemPrompt:
        // Store in metadata (injected on resume)
        metadata.SystemPrompt = cmd.Params["prompt"].(string)
        return a.sessionStore.Set(sessionID, metadata)
    }
}
```

#### 3. Codex Adapter (`codex_adapter.go`)

**Provider**: OpenAI API (Codex models)
**Architecture**: API-based with OpenAI SDK
**Session Model**: API conversation history

**Key Characteristics**:
- **Session Resume**: API-based (conversation_id tracking)
- **State Detection**: API polling
- **Working Directory**: Manual context injection
- **Persistence**: Server-side (OpenAI maintains history)
- **Multi-session**: Yes (multiple conversation IDs)

**Unique Features**:
- Code completion specialized models
- Function calling support
- Fine-tuned model support
- Temperature/top-p control
- Max tokens configuration

**State Detection Strategy**:
```go
// API-based polling (similar to Gemini)
func (a *CodexAdapter) detectState(sessionID string) State {
    // Query OpenAI API for conversation status
    completion, err := a.client.GetCompletion(sessionID)
    if err != nil {
        return StateOffline
    }

    return completion.State  // DONE, WORKING, etc.
}
```

#### 4. OpenCode Adapter (`opencode_adapter.go`)

**Provider**: OpenCode server (mock/reference implementation)
**Architecture**: Client-server with SSE (Server-Sent Events)
**Session Model**: Server-managed state with SSE monitoring

**Key Characteristics**:
- **Session Resume**: `opencode attach` (no UUID, server maintains session)
- **State Detection**: SSE events (real-time push notifications)
- **Working Directory**: `opencode attach -C <path>` (native support)
- **Persistence**: Server-side (client stateless)
- **Multi-session**: Yes (via server endpoint routing)

**Unique Features**:
- Real-time state updates via SSE (no polling)
- Server health check endpoint (`/health`)
- Production-ready monitoring infrastructure
- Stateless client design
- Concurrent session support

**State Detection Strategy**:
```go
// SSE-based event stream
func (a *OpenCodeAdapter) detectState(sessionID string) State {
    // Subscribe to SSE events from OpenCode server
    // Events pushed in real-time (no polling required)
    event := <-a.sseMonitor.EventStream(sessionID)

    switch event.Type {
    case "state_change":
        return event.State
    case "error":
        return StateOffline
    default:
        return StateWorking
    }
}
```

**SSE Integration**:
- Monitor implementation: `internal/monitor/opencode/`
- Event schema: `{"type": "state_change", "state": "DONE", "sessionID": "..."}`
- Coverage: 88.1% (production-ready)

### Session Management Patterns

Different harnesses employ different session management strategies:

#### UUID-Based Resume (Claude)

```go
// Claude: Native UUID support
session, _ := agent.CreateSession(ctx)
// Returns: SessionID{UUID: "abc-123", TmuxName: "my-session"}

// Later:
agent.ResumeSession("abc-123")  // Uses: claude --resume abc-123
```

**Benefits**: Native conversation restoration, full history replay
**Limitations**: Requires local history.jsonl file

#### API-Based Resume (Gemini, Codex)

```go
// Gemini/Codex: API conversation tracking
session, _ := agent.CreateSession(ctx)
// Returns: SessionID{ConversationID: "conv-456", TmuxName: "my-session"}

// Later:
agent.ResumeSession("conv-456")  // API call to fetch history
```

**Benefits**: Server-side persistence, cross-device access
**Limitations**: Network dependency, no offline resume

#### Server-Managed State (OpenCode)

```go
// OpenCode: Server maintains all state
session, _ := agent.CreateSession(ctx)
// Returns: SessionID{ServerSession: "sess-789", TmuxName: "my-session"}

// Later:
agent.ResumeSession("sess-789")  // opencode attach (server routing)
```

**Benefits**: Stateless client, real-time monitoring, scalable
**Limitations**: Requires running server

### State Detection Strategies

| Strategy | Harnesses | Latency | Reliability | Overhead |
|----------|-----------|---------|-------------|----------|
| **Hook-based** | Claude | <100ms | 95% | Low (hooks run async) |
| **Tmux scraping** | Claude (fallback) | ~200ms | 80% | Medium (regex parsing) |
| **API polling** | Gemini, Codex | 500ms-2s | 90% | High (network calls) |
| **SSE push** | OpenCode | <50ms | 98% | Low (event-driven) |

**Best Practices**:
- Use hooks when available (fastest, most reliable)
- Fall back to tmux scraping for legacy support
- Prefer SSE over polling for real-time requirements
- Cache state for 10-30s to reduce overhead

### Harness Comparison Matrix

| Feature | Claude | Gemini | Codex | OpenCode |
|---------|--------|--------|-------|----------|
| **Session Resume** | UUID-based | API-based | API-based | Server-based |
| **State Detection** | Hooks + Scraping | API poll | API poll | SSE push |
| **Working Directory** | Native | Manual | Manual | Native |
| **Offline Mode** | Yes | No | No | No |
| **Multi-session** | Yes | Yes | Yes | Yes |
| **Slash Commands** | Yes | No | No | No |
| **Function Calling** | Yes | Yes | Yes | No |
| **Conversation Export** | JSONL | API format | API format | Server format |
| **Parity Score** | 100% (baseline) | ~92% | ~93% | ~95% |

### Harness-Specific Differences

#### Directory Authorization

**Claude**: Native support via `--add-dir`, requires `.claude/CLAUDE.md` association
```bash
claude --add-dir ~/projects/myapp --resume abc-123
```

**Gemini/Codex**: Manual context injection (copy file contents to conversation)
```bash
# AGM abstracts this via adapter:
agent.SendMessage(sessionID, "I'm working in " + workingDir + ". Files: ...")
```

**OpenCode**: Native support via `-C` flag
```bash
opencode attach -C ~/projects/myapp
```

#### Error Handling

**Claude**: Detailed error messages, exit codes, stderr parsing
**Gemini/Codex**: API error responses (HTTP status codes, JSON error bodies)
**OpenCode**: HTTP status codes + SSE error events

#### Configuration

**Claude**: `~/.claude/config.yaml`, environment variables (`ANTHROPIC_API_KEY`)
**Gemini**: Environment variables (`GOOGLE_API_KEY`, `GEMINI_MODEL`)
**Codex**: Environment variables (`OPENAI_API_KEY`, `OPENAI_MODEL`)
**OpenCode**: Environment variables (`OPENCODE_SERVER_URL`, defaults to `http://localhost:4096`)

### Adding New Harnesses

To add a new agent harness to AGM:

1. **Implement Agent Interface** (`internal/agent/<harness>_adapter.go`)
   - All 11 required methods
   - Harness-specific configuration struct
   - Error handling for all edge cases

2. **Register in Factory** (`internal/agent/factory.go`)
   ```go
   agentRegistry["newharness"] = NewNewHarnessAdapter
   ```

3. **Add Validation** (`internal/agent/validate.go`)
   ```go
   knownAgents = append(knownAgents, "newharness")
   validateNewHarnessAvailable() // Check API keys, server health, etc.
   ```

4. **Update Manifest Schema** (`internal/manifest/manifest.go`)
   ```go
   type Manifest struct {
       // ... existing fields
       NewHarness *NewHarnessMetadata `yaml:"newharness,omitempty"`
   }
   ```

5. **CLI Integration** (`cmd/agm/new.go`, `cmd/agm/resume.go`)
   - Add to interactive agent selector
   - Add initialization case
   - Add resume case

6. **Comprehensive Testing**
   - Unit tests (15+ tests, >80% coverage)
   - Integration parity tests (add to ALL test files)
   - BDD scenarios (add to Examples tables)
   - Contract tests (verify interface compliance)

**Automation**: Use `test/scripts/add_harness_to_parity_tests.py` to automatically add new harness to all 50+ parity test scenarios.

### Design Decisions

**Why a unified interface?**
- Consistent UX across harnesses (same commands work everywhere)
- Easy to add new providers (plug-and-play architecture)
- Testable (parity tests enforce consistency)
- Future-proof (new capabilities added via interface methods)

**Why allow harness-specific differences?**
- Each provider has unique strengths (Claude's hooks, Gemini's API, OpenCode's SSE)
- Forcing uniformity would limit capabilities
- Clear documentation makes differences discoverable
- Adapter pattern encapsulates complexity

**Why not abstract away all differences?**
- Users benefit from provider-specific features
- Abstraction layer would be leaky (providers evolve independently)
- Transparency > false abstraction (users should know which harness they're using)

**Related ADRs**:
- ADR-001: Multi-Agent Architecture
- ADR-002: Command Translation Layer
- ADR-003: State Detection Strategies (proposed)

---

## Multi-Session Coordination

**Status**: Implemented in Phase 2 (v3.2)

### Overview

Multi-session coordination enables AGM sessions to communicate and collaborate asynchronously. This feature supports distributed workflows where multiple AI agents work together on complex tasks.

### Architecture Components

#### 1. Message Queue (`~/.agm/queue.db`)

**Technology**: SQLite with WAL mode
**Purpose**: Persistent message storage with FIFO ordering

**Schema**:
```sql
CREATE TABLE message_queue (
    message_id TEXT PRIMARY KEY,
    from_session TEXT NOT NULL,
    to_session TEXT NOT NULL,
    message TEXT NOT NULL,
    priority INTEGER NOT NULL DEFAULT 1,
    status TEXT NOT NULL DEFAULT 'pending',
    attempt_count INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMP NOT NULL,
    delivered_at TIMESTAMP,
    ack_required INTEGER NOT NULL DEFAULT 1,
    ack_received INTEGER NOT NULL DEFAULT 0,
    ack_timeout TIMESTAMP
);
```

**Performance Characteristics**:
- Enqueue latency: <10ms
- Dequeue latency: <50ms
- Max throughput: 200 messages/minute
- Queue capacity: 10,000 messages (configurable)

#### 2. AGM Daemon (`agm-daemon`)

**Purpose**: Background service for state-aware message delivery

**Core Functions**:
```go
type Daemon struct {
    queue      *messages.MessageQueue
    ackManager *messages.AckManager
    pollInterval time.Duration  // Default: 30s
}

func (d *Daemon) deliverPending() error {
    entries, _ := d.queue.GetAllPending()
    for _, entry := range entries {
        state, _ := session.DetectState(entry.To)
        switch state {
        case StateDone:
            d.deliverMessage(entry)
        case StateWorking, StateCompacting:
            // Defer delivery
            continue
        case StateOffline:
            d.retryLater(entry)
        }
    }
}
```

**Deployment Modes**:
- **Foreground**: `agm daemon start --foreground` (debugging)
- **Background**: `agm daemon start` (production)
- **Systemd**: Managed via systemd service
- **Launchd**: Managed via launchd (macOS)

#### 3. Session State Detection

**State Enum**:
```go
const (
    StateDone      = "DONE"       // Idle, awaiting input
    StateWorking       = "WORKING"    // Processing a request
    StateCompacting = "COMPACTING" // Database maintenance
    StateOffline    = "OFFLINE"    // Session not running
)
```

**Detection Strategy**:
1. Read session manifest: `~/.agm/sessions/{name}/manifest.json`
2. Check `state` field (updated by hooks)
3. Validate tmux session exists
4. Cache state for 10s (configurable)

**Hook-Based Updates**:
- `~/.claude/hooks/session-start/agm-state-ready`: Set DONE on startup
- `~/.claude/hooks/message-complete/agm-state-ready`: Set DONE after response
- `~/.claude/hooks/compact-start/agm-state-compacting`: Set COMPACTING
- `~/.claude/hooks/compact-complete/agm-state-ready`: Set DONE after compact

#### 4. Acknowledgment Protocol

**Purpose**: Confirm message delivery and processing

**Protocol Flow**:
```
Sender:
  1. agm send target "message" --ack
  2. Message enqueued with ack_required=1
  3. Wait for ack (optional, 60s timeout)

Daemon:
  4. Detect target session is DONE
  5. Deliver message via tmux send-keys
  6. Update session state to WORKING
  7. Mark message as delivered
  8. Send acknowledgment signal

Receiver:
  9. Process message
  10. (Hook sets state back to DONE)
```

**Implementation**:
```go
type AckManager struct {
    pendingAcks map[string]chan error  // messageID → ack channel
    mu          sync.RWMutex
}

func (am *AckManager) WaitForAck(msgID string, timeout time.Duration) error {
    ackCh := make(chan error, 1)
    am.pendingAcks[msgID] = ackCh
    select {
    case err := <-ackCh:
        return err
    case <-time.After(timeout):
        return ErrAckTimeout
    }
}
```

### Message Delivery Pipeline

**Full Delivery Flow**:
```
1. User runs: agm send session-b "task"
   ↓
2. CLI validates session-b exists
   ↓
3. Message enqueued to SQLite queue
   ↓
4. Daemon polls queue (every 30s)
   ↓
5. Daemon detects session-b state = DONE
   ↓
6. Daemon sends message via tmux:
   tmux send-keys -t session-b "From: session-a\n\ntask" C-m
   ↓
7. Daemon updates session-b manifest: state = WORKING
   ↓
8. Daemon marks message as delivered in queue
   ↓
9. Daemon sends acknowledgment (if requested)
   ↓
10. Session-b processes message
   ↓
11. Hook sets session-b state = DONE (after completion)
```

### Retry Logic

**Exponential Backoff**:
```
Attempt 1: 5s delay
Attempt 2: 10s delay
Attempt 3: 20s delay
Attempt 4+: Mark PERMANENTLY_FAILED
```

**Error Handling**:
- **Transient errors**: Retry with backoff (network, tmux)
- **Permanent errors**: Move to dead letter queue (session deleted)
- **Timeout errors**: Requeue after 60s

### Performance Characteristics

| Metric | Value | Notes |
|--------|-------|-------|
| **Delivery Latency** | 300-500ms | End-to-end for DONE session |
| **Poll Overhead** | 50-100ms | Per 30s cycle |
| **Throughput** | 100 msg/min | Sustained rate (single daemon) |
| **Memory Usage** | ~10MB | Daemon baseline |
| **CPU Usage** | <1% | Idle daemon |
| **Disk I/O** | ~10KB/s | Steady state |

### Scalability Limits

- **Max messages in queue**: 10,000 (configurable)
- **Max concurrent sessions**: No limit (filesystem-bound)
- **Max message size**: 64KB (tmux buffer limit)
- **Delivery rate**: 200 msg/min (theoretical max with 30s poll)

### Integration Points

**CLI Integration**:
```bash
# Send message
agm send target-session "message content"

# Query queue status
agm daemon status --queue

# Clean old messages
agm daemon clean --older-than 7d
```

**Daemon Management**:
```bash
# Start daemon
agm daemon start

# Stop daemon
agm daemon stop

# Check health
agm daemon health
```

**Session State Management**:
```bash
# Get session state
agm session get-state my-session

# Force state (admin only)
agm admin set-state my-session DONE
```

### Coordination Patterns

See [Multi-Session Guide](MULTI_SESSION_GUIDE.md) for detailed usage patterns:

1. **Parallel Research**: Distribute tasks across multiple sessions
2. **Sequential Pipeline**: Chain tasks with dependencies
3. **Broadcast**: Send same message to multiple sessions
4. **Request-Response**: Session A asks Session B for data
5. **Error Recovery**: Detect and retry failed deliveries

### Future Enhancements

**Planned Features (v3.3+)**:

1. **Priority Queue**: High-priority messages delivered first
2. **Dynamic Poll Interval**: Adjust based on queue size
3. **Concurrent Delivery**: Parallel message delivery
4. **Message Filtering**: Subscribe to message topics
5. **Remote Queue**: Networked queue for multi-machine coordination
6. **End-to-End Encryption**: Secure message content at rest

---

## Multi-Agent State Monitoring

**Status**: Implemented in Phase 1 (v4.0)

### Overview

AGM supports multiple agent harnesses (Claude Code, Gemini CLI, OpenCode, Cortex) through a hybrid integration architecture. Each agent has different state detection capabilities, unified through AGM's EventBus as the canonical integration layer.

### Architecture

```
                    ┌─────────────────────────────────────┐
                    │         AGM EventBus Hub            │
                    │  (Canonical integration layer)      │
                    └──────────────┬──────────────────────┘
                                   │
                    ┌──────────────┼──────────────────────┐
                    │              │                      │
         ┌──────────▼────┐  ┌─────▼──────┐   ┌─────────▼────────┐
         │ OpenCode SSE  │  │  Gemini    │   │    Claude        │
         │   Adapter     │  │    Tmux    │   │     Tmux         │
         └───────────────┘  └────────────┘   └──────────────────┘
              │                   │                    │
              │              ┌────▼────────────────────▼────┐
              │              │   Astrocyte Python Daemon    │
              │              │   (tmux capture-pane)        │
              │              └──────────────┬───────────────┘
              │                             │
         ┌────▼──────┐               incidents.jsonl
         │ OpenCode  │                      │
         │  Server   │               ┌──────▼──────┐
         │ (SSE)     │               │  Astrocyte  │
         └───────────┘               │  Go Watcher │
                                     └─────────────┘
```

**Key Principle**: All state detection mechanisms publish to EventBus. Consumers (notifications, status, Temporal) subscribe once and receive events from all agents.

### Agent-Specific Strategies

#### OpenCode: SSE Adapter

**Detection Method**: Server-Sent Events from OpenCode server

**Architecture**:
- OpenCode runs as client-server: `opencode serve` (headless) + `opencode attach` (TUI)
- SSE endpoint: `GET /event` streams real-time events
- Events: `permission.asked`, `tool.execute.before`, `tool.execute.after`, `session.created`

**Implementation**: `internal/monitor/opencode/`
- SSE client with auto-reconnect and exponential backoff
- Event parser mapping OpenCode events to AGM states
- EventBus publisher for state transitions
- Session lifecycle manager for server start/stop detection

**State Mapping**:
- `permission.asked` → `AWAITING_PERMISSION`
- `tool.execute.before` → `WORKING`
- `tool.execute.after` → `IDLE`
- `session.created` → `DONE`
- `session.closed` → `TERMINATED`

**Fallback**: If SSE connection fails, Astrocyte can monitor via tmux as backup

#### Claude Code: Astrocyte Tmux (Current)

**Detection Method**: Tmux screen scraping via Astrocyte Python

**Architecture**:
- Astrocyte runs `tmux capture-pane` every 60 seconds
- Pattern matching for stuck states (mustering timeout, zero-token, cursor frozen)
- Writes incidents to `~/.agm/astrocyte/incidents.jsonl`
- Go watcher polls incidents and publishes to EventBus

**Why Keep Scraping**:
- Proven 0% failure rate in production (per ADR-0001)
- Detects emergent behaviors (stuck states, crashes) that hooks don't cover
- Resilient to Claude Code version changes

**Optional Enhancement**: HTTP webhooks (PreToolUse hook) for real-time permission prompts (deferred to Phase 6)

#### Gemini CLI: Astrocyte Tmux

**Detection Method**: Same as Claude Code (tmux scraping)

**Why Keep Scraping**:
- Headless mode (`--output-format json`) disables interactive TUI
- BeforeTool hook requires complex stdin/stdout proxy script
- Scraping is simpler and proven reliable

**Optional Enhancement**: BeforeTool hook (deferred to Phase 4)

#### Cortex: To Be Determined

**Strategy**: Evaluate integration options when work begins (check for SSE/WebSocket/hooks, build adapter accordingly)

### EventBus Integration

#### Event Schema

All adapters publish to EventBus using standard schema:

```go
type SessionStateChangeEvent struct {
    SessionID   string                 `json:"session_id"`
    State       string                 `json:"state"`
    Timestamp   int64                  `json:"timestamp"`
    Source      string                 `json:"source"`     // "opencode-sse", "astrocyte"
    Agent       string                 `json:"agent"`      // "opencode", "claude", "gemini"
    Metadata    map[string]interface{} `json:"metadata,omitempty"`
}
```

**Event Types**:
- `EventSessionStateChange`: State transitions (IDLE → WORKING)
- `EventSessionCreated`: New session initialized
- `EventSessionTerminated`: Session ended
- `EventSessionEscalated`: Stuck state detected by Astrocyte

#### State File Format (v4)

When state change event published, EventBus subscriber writes:

**Path**: `~/.agm/sessions/{session-id}/state`

**Format**: `{STATE} {TIMESTAMP}`

**Example**: `WORKING 1709654321`

**States**: `DONE`, `IDLE`, `WORKING`, `AWAITING_PERMISSION`, `STUCK`, `TERMINATED`

### Adapter Pattern

All adapters implement:

```go
type Adapter interface {
    Start(ctx context.Context) error  // Begin monitoring
    Stop(ctx context.Context) error   // Graceful shutdown
    Health() HealthStatus             // Diagnostics
    Name() string                     // Identifier
}
```

**Lifecycle**:
1. Daemon reads config to determine enabled adapters
2. Daemon creates adapter instances
3. Daemon calls `Start()` on each adapter
4. Adapters begin monitoring and publishing to EventBus
5. On shutdown: Daemon calls `Stop()` with context timeout

### Astrocyte Agent Detection

Astrocyte Python skips sessions monitored by native adapters:

**Implementation**:
1. Read `~/.agm/sessions/{id}/manifest.json`
2. Check `agent` field: `"claude"`, `"gemini"`, `"opencode"`, `"cortex"`
3. If `agent == "opencode"` and OpenCode adapter enabled: Skip monitoring
4. Otherwise: Continue tmux scraping

**Configuration Override**:

```yaml
astrocyte:
  force_scraping: false  # If true, ignore agent type and scrape everything
  skip_agents:
    - "opencode"         # Skip these agent types (default)
```

### Configuration

```yaml
# Multi-agent adapters
adapters:
  opencode:
    enabled: true
    server_url: "http://localhost:4096"
    reconnect:
      initial_delay: 1s
      max_delay: 30s
      multiplier: 2
    fallback_to_tmux: true

  claude_hooks:
    enabled: false  # Phase 3 (optional)
    listen_addr: "127.0.0.1:14321"

  gemini_hooks:
    enabled: false  # Phase 4 (optional, deferred)

# Astrocyte Python settings
astrocyte:
  enabled: true
  scan_interval: 60  # seconds
  force_scraping: false
  skip_agents:
    - "opencode"
```

### Observability

**Metrics**:
```
agm_adapter_connected{name="opencode-sse"} 1
agm_adapter_events_total{name="opencode-sse", event_type="permission.asked"} 42
agm_adapter_errors_total{name="opencode-sse", error_type="connection_failed"} 3
agm_eventbus_queue_depth 23
```

**Logging**:
```
[INFO] OpenCode SSE adapter started (server: http://localhost:4096)
[INFO] Session my-session: IDLE → WORKING (source: opencode-sse)
[WARN] SSE connection lost, reconnecting in 2s...
[INFO] Skipping session my-session (agent: opencode, reason: SSE adapter enabled)
```

**agm status**:
```bash
$ agm status

Sessions: 3 active

Adapters:
  opencode-sse: ✓ Connected (last event: 2s ago)
  claude-hooks: ✗ Disabled
  gemini-hooks: ✗ Disabled

Astrocyte:
  ✓ Running (monitoring 2 sessions, skipping 1)

EventBus:
  ✓ Running (queue depth: 5/1000)
```

### Related Documentation

- **MULTI-AGENT-INTEGRATION-SPEC.md**: Complete integration specification
- **ADR-009**: EventBus as integration layer decision rationale
- **internal/monitor/opencode/ARCHITECTURE.md**: OpenCode SSE adapter design
- **ADR-007**: Hook-based state detection pattern

---

## Session Lifecycle

### State Machine

```
         ┌──────────┐
         │ Creating │
         └────┬─────┘
              │
         ┌────▼────┐
         │ Active  │ ◄────┐
         └────┬────┘      │
              │           │
         ┌────▼────┐      │
         │ Stopped │ ─────┘
         └────┬────┘      (Resume)
              │
         ┌────▼────┐
         │Archived │
         └────┬────┘
              │
         ┌────▼────┐
         │ Deleted │
         └─────────┘
```

### State Transitions

- **Creating → Active**: Session created successfully
- **Active → Stopped**: User detaches or exits tmux
- **Stopped → Active**: User resumes session
- **Stopped → Archived**: User archives via `agm archive`
- **Archived → Stopped**: User restores via `agm unarchive`
- **Archived → Deleted**: Manual deletion (no CLI command yet)

### Status Computation

Status is **computed**, not stored (except for `archived`):

```go
func ComputeStatus(manifest *Manifest) string {
    if manifest.Lifecycle == "archived" {
        return "archived"
    }
    if tmux.HasSession(manifest.TmuxSessionName) {
        return "active"
    }
    return "stopped"
}
```

---

## Session Initialization

### InitSequence Component

**Purpose**: Automate the initialization of Claude sessions by sending `/rename` and `/agm:agm-assoc` skill commands without user intervention.

**Location**: `internal/tmux/init_sequence.go`

### Architecture

**Key Design Decisions**:
1. Uses capture-pane polling (not control mode) - see [ADR-0001](adr/0001-init-sequence-capture-pane.md)
2. Implements timing delays to prevent command queueing
3. Avoids lock conflicts by calling tmux commands directly

**Component Structure**:

```go
type InitSequence struct {
    SessionName    string
    PromptVerified bool  // Skip redundant WaitForClaudePrompt calls if true
}

func (seq *InitSequence) Run() error {
    // Step 1: Wait for Claude prompt (if not already verified by caller)
    if !seq.PromptVerified {
        if err := WaitForClaudePrompt(seq.SessionName, 30*time.Second); err != nil {
            return err
        }
    }

    // Step 2: Send /rename command
    if err := seq.sendRename(); err != nil {
        return err
    }

    // Step 3: Send /agm:agm-assoc command
    if err := seq.sendAssociation(); err != nil {
        return err
    }

    return nil
}
```

### Timing Constraints

**Critical timing requirements** (prevents command queueing bug):

1. **SendCommandLiteral delay**: 500ms between text and Enter
   - Prevents both commands queuing on same input line
   - Ensures tmux processes text before Enter key

2. **Post-rename wait**: 5 seconds after `/rename` completes
   - Ensures first command fully executes before second starts
   - Total minimum duration: ≥6 seconds

**Implementation**:

```go
func SendCommandLiteral(sessionName, command string) error {
    socketPath := GetSocketPath()

    // Send command text with -l flag (literal interpretation)
    cmdText := exec.Command("tmux", "-S", socketPath, "send-keys", "-t", sessionName, "-l", command)
    if err := cmdText.Run(); err != nil {
        return err
    }

    time.Sleep(500 * time.Millisecond)  // Critical delay

    // Send Enter separately
    cmdEnter := exec.Command("tmux", "-S", socketPath, "send-keys", "-t", sessionName, "C-m")
    if err := cmdEnter.Run(); err != nil {
        return err
    }

    return nil
}
```

### Lock Management

**Important**: InitSequence.Run() does NOT use withTmuxLock() wrapper.

**Rationale**:
- SendCommandLiteral calls `exec.Command` directly (not SendCommand)
- SendCommand acquires tmux lock via withTmuxLock()
- Double-locking causes "tmux lock already held by this process" error

**Design Pattern**:

```go
// ❌ WRONG - causes double-lock error
func (seq *InitSequence) Run() error {
    return withTmuxLock(func() error {
        // SendCommandLiteral internally calls SendCommand
        // SendCommand also calls withTmuxLock()
        return seq.sendRename()  // ERROR: double-lock
    })
}

// ✅ CORRECT - no lock wrapper, direct tmux commands
func (seq *InitSequence) Run() error {
    // Each SendCommandLiteral uses exec.Command directly
    // No lock conflicts
    return seq.sendRename()  // OK
}
```

### Error Handling

**Trust Prompt Handling**:
- `init_sequence.go` auto-confirms trust dialogs for sandbox directories by checking capture-pane output for trust dialog signatures ("Enter to confirm", "I trust this folder") and sending Enter
- After confirming, waits 3s for Claude to load, then re-waits for the real Claude prompt before sending /rename
- Continues after "❯" prompt detected

**Timeout Handling** (30 seconds):
- Warning displayed to user
- Session remains attached (not killed)
- User can manually run `/rename` and `/agm:agm-assoc`

**Network Interruption**:
- Retries with exponential backoff
- Uses ready-file signal for completion detection

### Ready-File Signal

**Path**: `~/.agm/claude-ready-<session-name>`

**Purpose**: File-based signal that Claude CLI is ready and initialization is complete.

**Integration**:
- Created by Claude SessionStart hook (`~/.claude/hooks/session-start/agm-ready-signal`)
- AGM waits for this file before sending commands
- Replaces fragile text-parsing-based prompt detection

### Test Coverage

**Regression Tests**: `internal/tmux/init_sequence_regression_test.go`
- TestSendCommandLiteral_DoesNotUseSendCommand (no double-lock)
- TestSendCommandLiteral_Timing (500ms delays)
- TestInitSequence_NoDoubleLock (no lock errors)
- TestSendCommandLiteral_UsesLiteralFlag (correct tmux flags)
- TestInitSequence_DetachedMode (detached mode works)

**BDD Scenarios**: `test/bdd/features/session_initialization.feature`
- Commands execute on separate lines
- Sufficient delay between commands
- Detached sessions initialize automatically

**Documentation**: `docs/testing/INIT_SEQUENCE_TEST_COVERAGE.md`

### Performance Characteristics

**Typical execution**:
- Wait for prompt: 2-10 seconds (depends on Claude startup)
- Send /rename: 500ms (command + delay)
- Wait after /rename: 5 seconds
- Send /agm:agm-assoc: 500ms (command + delay)
- **Total**: ~8-16 seconds

**Overhead acceptable** because:
- Initialization is infrequent (once per session creation)
- Reliability more important than speed
- Timing delays prevent critical bugs

---

## Command Translation Layer

### Purpose

Provide unified command interface across agents with different capabilities.

### Supported Commands (Phase 2 Complete)

| Command | Claude CLI | Gemini CLI | Codex API | OpenCode API |
|---------|-----------|-----------|-----------|-------------|
| CommandRename | ✅ (slash command) | ✅ (/chat save) | ✅ (API) | ✅ (API) |
| CommandSetDir | ✅ (cd via tmux) | ✅ (cd via tmux) | ✅ (metadata) | ✅ (metadata) |
| CommandRunHook | ✅ (tmux send) | ✅ (hook framework) | ✅ (limited) | ✅ (limited) |
| CommandClearHistory | ✅ (file removal) | ✅ (file removal) | ✅ (API) | ✅ (API) |
| CommandSetSystemPrompt | ✅ (metadata) | ✅ (metadata) | ✅ (API param) | ✅ (API param) |
| CommandAuthorize | ✅ (CLAUDE.md) | ✅ (--include-directories) | N/A | N/A |

**Phase 2 Achievement:** Full command parity across CLI agents (Claude & Gemini)

### Graceful Degradation

```go
err := translator.RenameSession(ctx, sessionID, "new-name")
if errors.Is(err, command.ErrNotSupported) {
    // Fallback: Update manifest only
    manifest.TmuxSessionName = "new-name"
    return manifestWriter.Write(manifest)
}
```

### Implementation Strategies

**Claude CLI**:
- Commands sent via tmux (`tmux send-keys`)
- Slash commands: `/rename`, `/agm-assoc`
- Working directory: `cd` command via tmux
- History: File removal from `~/.claude/history.jsonl`
- System prompt: Stored in session metadata
- Synchronous execution

**Gemini CLI** (Phase 2):
- Commands sent via tmux (`tmux send-keys`)
- Session rename: `/chat save {name}` + metadata update
- Working directory: `cd` command via tmux + metadata update
- History: File removal from `~/.gemini/sessions/{id}/history.jsonl`
- System prompt: Stored in session metadata
- Synchronous execution

**Codex/OpenCode (API)**:
- Commands sent via OpenAI API
- Session management: API conversation tracking
- Working directory: Context injection
- History: API-based clearing
- System prompt: API parameter
- Asynchronous execution

**Command Translation Pattern** (Gemini CLI Example):
```go
case CommandSetDir:
    // 1. Send cd to tmux session
    tmux.SendCommand(metadata.TmuxName, fmt.Sprintf("cd %s\r", newPath))

    // 2. Update AGM metadata (dual tracking)
    metadata.WorkingDir = newPath
    a.sessionStore.Set(sessionID, metadata)
```

---

## Security Model

### API Key Management

- **Storage**: Environment variables (not in config files)
- **Validation**: Checked before agent start
- **Rotation**: User responsibility (update env vars)

**Required environment variables**:
```bash
ANTHROPIC_API_KEY=sk-ant-...    # Claude (CLI-based)
GEMINI_API_KEY=AIza...          # Gemini (CLI-based, may be optional)
OPENAI_API_KEY=sk-...           # Codex (API-based)
OPENCODE_API_KEY=...            # OpenCode (self-hosted, optional)
OPENCODE_API_URL=...            # OpenCode endpoint (required if using OpenCode)
```

**Note:** CLI agents (Claude, Gemini) may use API keys configured in their respective CLI tools, not necessarily environment variables.

### Session Isolation

- **Tmux sessions**: Isolated by user account
- **File permissions**: Session directories are user-private (0700)
- **Socket security**: Tmux sockets checked for correct permissions
- **Lock files**: Prevent concurrent access to same session

### Lock Management

**Global tmux lock**:
- Path: `/tmp/agm-tmux.lock`
- Purpose: Prevent concurrent tmux commands
- Timeout: 5 seconds (configurable)
- Auto-release: On process exit

**Session locks**:
- Path: `~/sessions/<name>/.lock`
- Purpose: Prevent concurrent session operations
- Timeout: 30 seconds (configurable)
- Stale detection: PID validation

---

## Performance Considerations

### Optimization Strategies

1. **Caching**:
   - Health checks cached for 5 seconds
   - Session list cached during command execution
   - History.jsonl parsed once per command

2. **Lazy loading**:
   - Manifests loaded on-demand
   - Agent adapters initialized when needed
   - History parsing skipped for non-UUID commands

3. **Parallel operations**:
   - Multiple agents checked concurrently (`agm agent list`)
   - Batch cleanup operations parallelized
   - Backup creation non-blocking

4. **Timeouts**:
   - Tmux commands: 5 seconds default
   - Health checks: 5 seconds
   - Lock acquisition: 30 seconds
   - UUID detection: 5 minutes

### Scalability Limits

- **Sessions**: No hard limit (filesystem-bound)
- **History.jsonl**: ~100MB before performance degradation
- **Concurrent operations**: Limited by tmux socket contention
- **Message logs**: Daily rotation, 90-day retention

### Benchmarks

```
BenchmarkHasSession-8           5000    250 µs/op
BenchmarkListSessions-8         2000    650 µs/op
BenchmarkManifestRead-8        10000    120 µs/op
BenchmarkUUIDDetection-8        1000   1200 µs/op
```

---

## Extensibility

### Adding New Agents

1. Implement `Agent` interface (`internal/agent/`)
2. Implement `CommandTranslator` (`internal/command/`)
3. Register in `AgentRegistry`
4. Add configuration validation
5. Update documentation

**Example** (minimal GPT adapter):

```go
// internal/agent/gpt_adapter.go
type GPTAdapter struct {
    apiKey string
}

func (a *GPTAdapter) Start(ctx context.Context, sessionID string, opts *StartOptions) error {
    // Start OpenAI CLI or API client
    return openai.Start(ctx, a.apiKey, opts)
}

func (a *GPTAdapter) IsAvailable() bool {
    return os.Getenv("OPENAI_API_KEY") != ""
}

func (a *GPTAdapter) GetTranslator() command.Translator {
    return &command.GPTTranslator{client: a.client}
}
```

### Adding New Commands

1. Define command in `CommandTranslator` interface
2. Implement for each agent adapter
3. Add CLI command (`cmd/csm/`)
4. Update tests and documentation

---

## Error Handling

### Error Categories

1. **User errors**: Invalid input, typos (exit code 2)
2. **Session errors**: Not found, corrupted manifest (exit code 3)
3. **Lock errors**: Timeout, contention (exit code 4)
4. **Agent errors**: API key missing, network failure (exit code 1)
5. **System errors**: Tmux not installed, permission denied (exit code 1)

### Error Recovery

- **UUID detection failure**: Offer manual fix via `agm fix`
- **Manifest corruption**: Offer restore from backup
- **Lock timeout**: Auto-retry with backoff
- **Tmux crash**: Detect and recreate session

---

## Testing Strategy

### Test Coverage

- **Unit tests**: 80%+ coverage for core modules
- **Integration tests**: BDD scenarios (Gherkin)
- **End-to-end tests**: `agm test` subcommands

### BDD Scenarios

**Location**: `test/bdd/*.feature`

**Framework**: Cucumber (Go)

**Example scenario**:
```gherkin
Scenario: Create session with specific agent
  Given no existing sessions
  When I run "agm new --harness gemini-cli research-task"
  Then a new session "research-task" is created
  And the session uses agent "gemini"
  And the tmux session "research-task" exists
```

**Coverage**: 8 feature files, 20+ scenarios

---

## Deployment

### Installation Methods

1. **Go install** (recommended):
   ```bash
   go install github.com/vbonnet/ai-tools/agm/cmd/agm@latest
   ```

2. **Binary release**:
   ```bash
   curl -L https://github.com/.../releases/download/v3.0.0/agm-linux-amd64 -o agm
   chmod +x agm
   mv agm /usr/local/bin/
   ```

3. **From source**:
   ```bash
   git clone https://github.com/vbonnet/ai-tools.git
   cd ai-tools/agm
   go build ./cmd/agm
   ```

### System Requirements

- **OS**: Linux, macOS (Windows via WSL2)
- **Go**: 1.24+ (for building)
- **tmux**: 3.0+ (required)
- **Claude CLI**: Latest (for Claude agent)
- **Gemini CLI**: Latest (for Gemini agent)

### Configuration

**Minimal configuration** (`~/.config/agm/config.yaml`):

```yaml
defaults:
  interactive: true
  auto_associate_uuid: true
```

**Full configuration**: See [AGM-COMMAND-REFERENCE.md](AGM-COMMAND-REFERENCE.md#configuration-file)

---

## Monitoring and Observability

### Health Checks

**Command**: `agm doctor [--validate] [--fix]`

**Checks**:
- Agent installation
- Tmux availability
- User lingering (systemd)
- Session health
- UUID associations
- Duplicate sessions

### Logging

**Message logs**: `~/.agm/logs/messages/YYYY-MM-DD.jsonl`

**Format**:
```json
{
  "message_id": "1738612345678-agm-send-001",
  "timestamp": "2026-02-04T10:30:00Z",
  "sender": "agm-send",
  "recipient": "my-session",
  "message": "Please analyze the code",
  "reply_to": null
}
```

**Retention**: 90 days (configurable)

**Cleanup**: `agm logs clean --older-than 90`

---

## Future Roadmap

### Planned Features (v3.1+)

1. **Unified storage migration** (`agm migrate --to-unified-storage`)
2. **Workflow automation** (deep-research, code-review, architect)
3. **Agent routing** (AGENTS.md integration)
4. **Multi-conversation support** (multiple conversations per session)
5. **Cloud sync** (session sync across machines)
6. **Web UI** (optional web interface)

### Experimental Features

- **Gateway layer** (`internal/gateway/`): Load balancing, failover
- **MCP integration** (Model Context Protocol)
- **Astrocyte daemon**: Automatic recovery from stuck sessions

---

## Related Documentation

### User Guides

- **[Quick Reference](AGM-QUICK-REFERENCE.md)** - One-page cheat sheet
- **[Command Reference](AGM-COMMAND-REFERENCE.md)** - Complete CLI reference
- **[Getting Started](GETTING-STARTED.md)** - Installation and first steps
- **[Examples](EXAMPLES.md)** - Real-world usage scenarios
- **[Multi-Session Guide](MULTI_SESSION_GUIDE.md)** - Coordination workflows (Phase 2)
- **[Migration Guide](AGM-MIGRATION-GUIDE.md)** - AGM to AGM migration

### Operations & Performance

- **[Operations Runbook](OPERATIONS_RUNBOOK.md)** - Production operations, incident response (Phase 2)
- **[Performance Tuning](PERFORMANCE_TUNING.md)** - Optimization strategies (Phase 2)
- **[Troubleshooting](TROUBLESHOOTING.md)** - Common issues and solutions

### Architecture & Design

- **[BDD Catalog](BDD-CATALOG.md)** - Living documentation
- **[AGM Daemon Architecture](../cmd/agm-daemon/ARCHITECTURE.md)** - Daemon internals (Phase 2)
- **[AGM CLI Architecture](../cmd/agm/ARCHITECTURE.md)** - CLI design patterns
- **[MCP Server Architecture](../cmd/agm-mcp-server/ARCHITECTURE.md)** - MCP integration

---

**Maintained by**: Foundation Engineering
**License**: MIT
**Repository**: https://github.com/vbonnet/ai-tools
