# AGM Architecture Overview

Complete architectural documentation for the AI/Agent Gateway Manager (AGM).

**Version**: 3.0
**Last Updated**: 2026-02-04

---

## Table of Contents

- [System Overview](#system-overview)
- [Core Architecture](#core-architecture)
- [Component Structure](#component-structure)
- [Data Flow](#data-flow)
- [Storage Architecture](#storage-architecture)
- [Multi-Agent System](#multi-agent-system)
- [Session Lifecycle](#session-lifecycle)
- [Command Translation Layer](#command-translation-layer)
- [Security Model](#security-model)
- [Performance Considerations](#performance-considerations)

---

## System Overview

### What is AGM?

AGM (AI/Agent Gateway Manager) is a sophisticated session management system that provides unified access to multiple AI agents (Claude, Gemini, GPT) through a consistent command-line interface. It evolved from Claude Session Manager (CSM) to support multi-agent workflows.

### Design Principles

1. **Multi-agent abstraction** - Single interface for multiple AI providers
2. **Session persistence** - Long-lived conversations across terminal sessions
3. **Backward compatibility** - CSM sessions migrate seamlessly
4. **Tmux integration** - Leverages tmux for session management
5. **Zero-downtime** - Sessions persist across reboots and network interruptions
6. **Explicit configuration** - No hidden state, manifest-driven design

### High-Level Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                        AGM CLI                               │
│  (Unified command interface for all agents)                  │
└────────┬────────────────────────────────┬───────────────────┘
         │                                 │
    ┌────▼─────┐                     ┌────▼─────┐
    │  Command │                     │ Session  │
    │Translator│                     │ Manager  │
    └────┬─────┘                     └────┬─────┘
         │                                 │
    ┌────▼──────────────────┐         ┌───▼──────┐
    │   Agent Adapters      │         │ Manifest │
    │ ┌─────┬──────┬──────┐ │         │  Store   │
    │ │Claude│Gemini│ GPT │ │         └──────────┘
    │ └──┬──┴───┬──┴───┬──┘ │
    └────│──────│──────│────┘
         │      │      │
    ┌────▼──────▼──────▼────┐
    │   Tmux Integration    │
    │  (Session multiplexer)│
    └───────────────────────┘
```

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
- Agent-specific adapters (Claude, Gemini, GPT)
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
│   ├── gemini_adapter.go   # Gemini-specific implementation
│   └── gpt_adapter.go      # GPT-specific implementation (future)
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
│   ├── control.go      # Control mode (-C) for programmatic control
│   ├── output_watcher.go   # Output stream monitoring
│   ├── socket.go       # Unix socket management
│   ├── lock.go         # Global tmux lock
│   └── prompt.go       # Prompt injection (send/reject)
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
├── lock/               # Locking primitives
│   └── filelock.go     # File-based locking with timeout
│
├── backup/             # Backup and restore
│   ├── backup.go       # Create numbered backups
│   └── restore.go      # Restore from backup
│
├── messages/           # Message logging and rate limiting
│   ├── logger.go       # JSONL message logging
│   ├── id.go           # Message ID generation
│   └── rate_limit.go   # Token bucket rate limiter
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
1. User: agm new --agent gemini research-task
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

~/.config/csm/config.yaml            # User configuration
~/.csm/logs/messages/                # Message logs (JSONL per day)
    ├── 2026-02-01.jsonl
    ├── 2026-02-02.jsonl
    └── 2026-02-03.jsonl
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

**Default agent**: Claude (if `--agent` flag not specified)

**Selection order**:
1. Explicit `--agent` flag
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

## Command Translation Layer

### Purpose

Provide unified command interface across agents with different capabilities.

### Supported Commands

| Command | Claude | Gemini | GPT |
|---------|--------|--------|-----|
| RenameSession | ✅ (slash command) | ✅ (API call) | 🔜 Planned |
| SetDirectory | ✅ (slash command) | ✅ (metadata) | 🔜 Planned |
| RunHook | ✅ (tmux send) | ⚠️ Limited | 🔜 Planned |

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

**Claude**:
- Commands sent via tmux (`tmux send-keys`)
- Slash commands: `/rename`, `/csm-assoc`
- Synchronous execution

**Gemini**:
- Commands sent via API (Google AI SDK)
- `UpdateConversationTitle`, `UpdateMetadata`
- Asynchronous execution

---

## Security Model

### API Key Management

- **Storage**: Environment variables (not in config files)
- **Validation**: Checked before agent start
- **Rotation**: User responsibility (update env vars)

**Required environment variables**:
```bash
ANTHROPIC_API_KEY=sk-ant-...    # Claude
GEMINI_API_KEY=AIza...          # Gemini
OPENAI_API_KEY=sk-...           # GPT
```

### Session Isolation

- **Tmux sessions**: Isolated by user account
- **File permissions**: Session directories are user-private (0700)
- **Socket security**: Tmux sockets checked for correct permissions
- **Lock files**: Prevent concurrent access to same session

### Lock Management

**Global tmux lock**:
- Path: `/tmp/csm-tmux.lock`
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
  When I run "agm new --agent gemini research-task"
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
   go install github.com/vbonnet/ai-tools/claude-session-manager/cmd/csm@latest
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
   cd ai-tools/claude-session-manager
   go build ./cmd/csm
   ```

### System Requirements

- **OS**: Linux, macOS (Windows via WSL2)
- **Go**: 1.24+ (for building)
- **tmux**: 3.0+ (required)
- **Claude CLI**: Latest (for Claude agent)
- **Gemini CLI**: Latest (for Gemini agent)

### Configuration

**Minimal configuration** (`~/.config/csm/config.yaml`):

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

**Message logs**: `~/.csm/logs/messages/YYYY-MM-DD.jsonl`

**Format**:
```json
{
  "message_id": "1738612345678-csm-send-001",
  "timestamp": "2026-02-04T10:30:00Z",
  "sender": "csm-send",
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

- **[Quick Reference](AGM-QUICK-REFERENCE.md)** - One-page cheat sheet
- **[Command Reference](AGM-COMMAND-REFERENCE.md)** - Complete CLI reference
- **[Getting Started](GETTING-STARTED.md)** - Installation and first steps
- **[Examples](EXAMPLES.md)** - Real-world usage scenarios
- **[Troubleshooting](TROUBLESHOOTING.md)** - Common issues and solutions
- **[Migration Guide](AGM-MIGRATION-GUIDE.md)** - CSM to AGM migration
- **[BDD Catalog](BDD-CATALOG.md)** - Living documentation

---

**Maintained by**: Foundation Engineering
**License**: MIT
**Repository**: https://github.com/vbonnet/ai-tools
