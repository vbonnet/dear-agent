# Agent Abstraction

This package implements the Agent interface abstraction for supporting multiple AI agents in AGM (AI/Agent Gateway Manager).

## Overview

The Agent interface provides a unified API for managing AI agent sessions, enabling AGM to support multiple providers (Claude, Gemini, GPT, etc.) without duplicating session management code.

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
        │ GeminiAdapter │ (implemented)
        └───────────────┘

        ┌───────────────┐
        │  GPTAdapter   │ (future)
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

### gemini_adapter.go
GeminiAdapter implementation:
- Implements Agent interface for Google Gemini
- Uses Google Generative AI Go SDK (`github.com/google/generative-ai-go`)
- Client-side conversation history persistence
- Stores sessions in `~/.agm/gemini/<session-id>/history.jsonl`
- Default model: `gemini-2.0-flash-exp`
- API key from `GEMINI_API_KEY` environment variable

### session_store.go
SessionStore manages SessionID persistence:
- `SessionStore` interface - Get/Set/Delete/List operations
- `JSONSessionStore` - File-based implementation (~/.agm/sessions.json)
- `SessionMetadata` - Session information (tmux name, created time, working dir)
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
- GeminiAdapter with all 11 methods ✨ NEW
- SessionStore with JSON persistence
- Comprehensive unit tests for both adapters

### TODO 🚧
- HTML export for ClaudeAdapter
- Advanced command support (authorize, run_hook)
- Integration tests with real API calls
- GPTAdapter implementation (future)

## Testing

Run unit tests:
```bash
# All tests
cd internal/agent
go test -v

# Claude adapter only
go test -v -run TestClaude

# Gemini adapter only
go test -v -run TestGemini
```

Expected output:
```
=== RUN   TestClaudeAdapterImplementsAgentInterface
--- PASS: TestClaudeAdapterImplementsAgentInterface (0.00s)
=== RUN   TestGeminiAdapter_NewGeminiAdapter
--- PASS: TestGeminiAdapter_NewGeminiAdapter (0.00s)
=== RUN   TestGeminiAdapter_CreateSession
--- PASS: TestGeminiAdapter_CreateSession (0.00s)
...
PASS
ok  	github.com/vbonnet/ai-tools/agm/internal/agent	0.035s
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

## Agent Comparison

| Feature | ClaudeAdapter | GeminiAdapter | GPTAdapter |
|---------|---------------|---------------|------------|
| Type | CLI-based | API-based | API-based |
| Backend | tmux + Claude CLI | Google Gen AI SDK | OpenAI SDK |
| Session Storage | `~/.claude/sessions/` | `~/.agm/gemini/` | TBD |
| API Key | Built into CLI | `GEMINI_API_KEY` | `OPENAI_API_KEY` |
| Default Model | claude-sonnet-4.5 | gemini-2.0-flash-exp | TBD |
| Slash Commands | ✅ | ❌ | ❌ |
| Function Calling | ✅ | ✅ | ✅ |
| Vision | ✅ | ✅ | ✅ |
| Multimodal | ❌ | ✅ | ❌ |
| Max Context | 200K tokens | 1M tokens | 128K tokens |
| Status | ✅ Implemented | ✅ Implemented | 🚧 TODO |

## Future Extensions

### Additional Agents
The interface supports any AI provider:
- GPT-4 via OpenAI API (planned)
- Local models via Ollama
- Custom agents via plugin system

## References

- Agent interface: `internal/agent/interface.go`
- ClaudeAdapter: `internal/agent/claude_adapter.go`
- SessionStore: `internal/agent/session_store.go`
- Tests: `internal/agent/claude_adapter_test.go`
- Bead: oss-agm-r2 (Agent abstraction)
- Next bead: oss-agm-g1 (Implement GeminiAgent)
