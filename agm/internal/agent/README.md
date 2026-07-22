# Agent Abstraction

This package implements the Agent interface abstraction for supporting multiple AI agents in AGM (AI/Agent Gateway Manager).

## Overview

The Agent interface provides a unified API for managing AI harness sessions. The active parity harnesses are `claude-code`, `codex-cli`, `agy`, `opencode-cli`, and `pi-cli`, with Claude Code as the reference implementation. `gemini-cli` remains deprecated compatibility for old sessions.

## Architecture

```
┌─────────────────────────────────────┐
│         Agent Interface             │
│  - CreateSession()                  │
│  - ResumeSession()                  │
│  - TerminateSession()               │
│  - GetSessionStatus()               │
│  - SendMessage()                    │
│  - GetHistory()                     │
│  - ExportConversation()             │
│  - ImportConversation()             │
│  - Capabilities()                   │
│  - ExecuteCommand()                 │
│  - Name(), Version()                │
└───────────────┬─────────────────────┘
                │ implements
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
Defines the Agent interface and supporting types:
- `Agent` - Main interface with 11 methods
- `SessionContext` - Parameters for session creation
- `Message` - Conversation message structure
- `Capabilities` - Agent feature capabilities
- `Command` - Generic agent operations
- `SessionID`, `Status`, `Role`, `ConversationFormat` - Type definitions

### claude_adapter.go
ClaudeAdapter implementation:
- Implements Agent interface for Claude CLI
- Delegates to existing AGM tmux infrastructure
- Maps SessionIDs to tmux session names
- Wraps existing session management logic

### codex_cli_adapter.go
CodexCLIAdapter implementation:
- Implements Agent interface for OpenAI Codex CLI
- Uses tmux-backed interactive sessions
- Does not route `codex-cli` through the OpenAI API adapter

### agy_adapter.go
AgyAdapter implementation:
- Implements Agent interface for Antigravity/AGY
- Uses tmux-backed interactive sessions
- Uses the same model-aware launch command policy as the production lifecycle
- Normalizes workspaces and shares provider identity serialization with CLI and MCP creation so cross-surface concurrent creates cannot exchange native IDs
- Fails closed on unreadable metadata and rejects unsafe native AGY conversation IDs before launch, resume, or history path use
- Refuses cold-resume command injection when the recorded tmux pane contains another live harness
- Requires readiness before reporting create or cold-resume success
- Uses exact AGY process-tree liveness and native brain transcripts

### opencode_adapter.go
OpenCodeAdapter implementation:
- Implements Agent interface for OpenCode
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
    // Agent can use tool calling
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

This decouples the Agent abstraction from tmux naming conventions, allowing:
- Agent-agnostic SessionIDs (UUIDs work for all agents)
- Tmux session names to change without breaking SessionID references
- Easy migration to non-tmux backends for API-based agents

## Implementation Status

### Implemented ✅
- Agent interface definition
- ClaudeAdapter with all 11 methods
- CodexCLIAdapter for tmux-backed Codex CLI sessions
- AgyAdapter for tmux-backed Antigravity/AGY sessions
- OpenCodeAdapter for OpenCode attach sessions
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
=== RUN   TestClaudeAdapterImplementsAgentInterface
--- PASS: TestClaudeAdapterImplementsAgentInterface (0.00s)
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

### Additional Agents
The interface supports any AI provider:
- GPT models via the legacy OpenAI API adapter (implemented compatibility path)
- Local models via Ollama
- Custom agents via plugin system

## References

- Agent interface: `internal/agent/interface.go`
- ClaudeAdapter: `internal/agent/claude_adapter.go`
- SessionStore: `internal/agent/session_store.go`
- Tests: `internal/agent/claude_adapter_test.go`
- Bead: oss-agm-r2 (Agent abstraction)
- Next bead: oss-agm-g1 (Implement GeminiAgent)
