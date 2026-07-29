# Harness Adapters

This package implements concrete harness adapters, descriptive harness
metadata, model routing, and shared adapter data types for AGM.

## Overview

The active parity harnesses are `claude-code`, `codex-cli`, `agy`,
`opencode-cli`, and `pi-cli`, with Claude Code as the reference implementation.
`gemini-cli` remains deprecated compatibility for old sessions. Constructors
return concrete adapters. Heterogeneous discovery uses the small `Harness`
metadata interface; lifecycle operations define the behavioral capabilities
they consume.

## Architecture

```
┌─────────────────────────────────────┐
│         Harness Metadata            │
│  - Name()                           │
│  - Version()                        │
│  - Capabilities()                   │
└───────────────┬─────────────────────┘
                │ described by
                ▼
        ┌───────────────┐
        │ ClaudeAdapter │ (implemented)
        └───────────────┘

        ┌───────────────┐
        │ CodexCLI      │ (implemented)
        └───────────────┘

        ┌───────────────┐
        │ AgyAdapter    │ (implemented)
        └───────────────┘

        ┌───────────────┐
        │ OpenCode      │ (implemented)
        └───────────────┘
```

## Components

### interface.go
Defines the metadata contract and supporting types:
- `Harness` - Metadata-only discovery interface
- `ContextMessageSender` and `ContextSessionStatusGetter` - narrow
  cancellation-aware capabilities used by pure API delivery
- `SessionContext` - Parameters for session creation
- `Message` - Conversation message structure
- `Capabilities` - Harness feature and model metadata
- `Command` - Generic agent operations
- `SessionID`, `Status`, `Role`, `ConversationFormat` - Type definitions

### claude_adapter.go
ClaudeAdapter implementation:
- Concrete adapter for Claude CLI
- Delegates to existing AGM tmux infrastructure
- Maps SessionIDs to tmux session names
- Wraps existing session management logic

### codex_cli_adapter.go
CodexCLIAdapter implementation:
- Concrete adapter for OpenAI Codex CLI
- Uses tmux-backed interactive sessions
- Does not route `codex-cli` through the OpenAI API adapter

### agy_adapter.go
AgyAdapter implementation:
- Concrete adapter for Antigravity/AGY
- Uses tmux-backed interactive sessions
- Uses the same model-aware launch command policy as the production lifecycle
- Normalizes workspaces and shares provider identity serialization with CLI and MCP creation so cross-surface concurrent creates cannot exchange native IDs
- Fails closed on unreadable metadata and rejects unsafe native AGY conversation IDs before launch, resume, or history path use
- Refuses cold-resume command injection when the recorded tmux pane contains another live harness
- Requires readiness before reporting create or cold-resume success
- Uses exact AGY process-tree liveness and native brain transcripts

### opencode_adapter.go
OpenCodeAdapter implementation:
- Concrete adapter for OpenCode
- Uses `opencode attach` against a running OpenCode server

### gemini_cli_adapter.go
GeminiCLIAdapter implementation:
- Deprecated compatibility path for old sessions
- Not part of active parity enforcement or new defaults

### session_store.go
SessionStore manages SessionID persistence:
- `SessionStore` interface - Get/Set/Delete/List operations
- `JSONSessionStore` - File-based implementation (~/.agm/sessions.json)
- `SessionMetadata` - Session information (tmux name, created time, working dir,
  selected model/mode/directories, and native harness identifier)
- Thread-safe with sync.RWMutex
- Atomic file writes for data integrity

## Usage

### Creating a ClaudeAdapter

```go
// Create adapter with default store
adapter, err := agent.NewClaudeAdapter(nil)
if err != nil {
    log.Fatal(err)
}

// Create adapter with custom store
store, _ := agent.NewJSONSessionStore("/custom/path/sessions.json")
adapter, err := agent.NewClaudeAdapter(store)
```

### Managing Sessions

```go
// Create new session
sessionID, err := adapter.CreateSession(agent.SessionContext{
    Name:             "my-session",
    WorkingDirectory: "~/project",
    Project:          "my-project",
    Model:            "3.5-flash-low",
})

// Resume existing session
err = adapter.ResumeSession(sessionID)

// Check session status
status, err := adapter.GetSessionStatus(sessionID)
// status: StatusActive, StatusSuspended, StatusTerminated

// Send message
msg := agent.Message{
    Role:    agent.RoleUser,
    Content: "Hello, can you help me?",
}
err = adapter.SendMessage(sessionID, msg)

// Get conversation history
messages, err := adapter.GetHistory(sessionID)

// Export conversation
data, err := adapter.ExportConversation(sessionID, agent.FormatJSONL)

// Terminate session
err = adapter.TerminateSession(sessionID)
```

### Checking Capabilities

```go
caps := adapter.Capabilities()

if caps.SupportsSlashCommands {
    // Can use /rename, /clear, etc.
}

if caps.SupportsTools {
    // Harness can expose tool calling
}

fmt.Printf("Model: %s\n", caps.ModelName)  // "claude-sonnet-4.5"
fmt.Printf("Context window: %d tokens\n", caps.MaxContextWindow)  // 200000
```

### Executing Commands

```go
// Rename session
err = adapter.ExecuteCommand(agent.Command{
    Type: agent.CommandRename,
    Params: map[string]interface{}{
        "session_id": string(sessionID),
        "name":       "new-session-name",
    },
})

// Change working directory
err = adapter.ExecuteCommand(agent.Command{
    Type: agent.CommandSetDir,
    Params: map[string]interface{}{
        "session_id": string(sessionID),
        "path":       "/new/working/directory",
    },
})
```

## SessionID Mapping

ClaudeAdapter maintains a persistent mapping between UUIDs (SessionID) and tmux session names:

**Storage:** `~/.agm/sessions.json`

**Format:**
```json
{
  "550e8400-e29b-41d4-a716-446655440000": {
    "tmux_name": "claude-session-1",
    "created_at": "2026-01-25T00:00:00Z",
    "working_dir": "~/project",
    "project": "my-project"
  }
}
```

This decouples adapter persistence from tmux naming conventions, allowing:
- Harness-agnostic SessionIDs (UUIDs work across adapters)
- Tmux session names to change without breaking SessionID references
- Pure API adapters to use a non-tmux storage locator

## Implementation Status

### Implemented ✅
- Harness metadata and consumer capability definitions
- ClaudeAdapter concrete lifecycle mechanisms
- CodexCLIAdapter for tmux-backed Codex CLI sessions
- AgyAdapter for tmux-backed Antigravity/AGY sessions
- OpenCodeAdapter for OpenCode attach sessions
- PiAdapter for tmux-backed Pi CLI sessions
- GeminiCLIAdapter as deprecated compatibility
- SessionStore with JSON persistence

### TODO 🚧
- HTML export for ClaudeAdapter
- Advanced command support (authorize, run_hook)
- Integration tests with real API calls

## Testing

Run unit tests:
```bash
# All tests
cd internal/agent
go test -v

# Claude adapter only
go test -v -run TestClaude

# Active/deprecated harness contract
go test -v -run 'Test(ActiveHarnesses|GeminiCLIIsDeprecated|CodexFactory)'
```

Expected output:
```
=== RUN   TestClaudeAdapterImplementsHarnessContract
--- PASS: TestClaudeAdapterImplementsHarnessContract (0.00s)
=== RUN   TestActiveHarnessesCanonicalParitySet
--- PASS: TestActiveHarnessesCanonicalParitySet (0.00s)
=== RUN   TestCodexFactoryUsesCLIAdapter
--- PASS: TestCodexFactoryUsesCLIAdapter (0.00s)
...
PASS
ok  	github.com/vbonnet/dear-agent/agm/internal/agent	0.035s
```

## Design Decisions

### 1. Adapter Pattern
**Chosen:** ClaudeAdapter wraps existing AGM code
**Alternative:** Refactor AGM internals to export reusable packages
**Rationale:** Minimizes changes, maintains backward compatibility

### 2. SessionID Type
**Chosen:** UUIDs (agent-agnostic)
**Alternative:** Use tmux session name directly
**Rationale:** Decouples abstraction from tmux, supports API agents

### 3. SessionStore Persistence
**Chosen:** JSON file
**Alternative:** In-memory map
**Rationale:** Survives restarts, simple to implement

## Harness Comparison

| Harness | Status | Backend | Notes |
|---------|--------|---------|-------|
| `claude-code` | Active reference | tmux + Claude Code | Most battle-tested implementation |
| `codex-cli` | Active parity | tmux + Codex CLI | Must not route through the OpenAI API adapter |
| `agy` | Active parity | tmux + Antigravity CLI | Replacement path for non-enterprise Gemini CLI users |
| `opencode-cli` | Active parity | tmux + `opencode attach` | Requires a running OpenCode server |
| `gemini-cli` | Deprecated compatibility | tmux + Gemini CLI | Kept for old sessions only |
| `openai` / `gpt` | Legacy compatibility | OpenAI Chat Completions API + local JSONL | Separate from the `codex-cli` harness |

## Future Extensions

### Additional Harnesses

A new interactive harness adds a concrete adapter plus finite harness and model
catalog entries. A shared operation depends only on the capability-sized
interface it owns. The legacy OpenAI API adapter remains a separate pure API
delivery path rather than an interactive harness catalog entry.

## References

- Harness metadata contract: `internal/agent/interface.go`
- ClaudeAdapter: `internal/agent/claude_adapter.go`
- SessionStore: `internal/agent/session_store.go`
- Tests: `internal/agent/claude_adapter_test.go`
- Historical bead: oss-agm-r2 (original adapter abstraction)
