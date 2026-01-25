# Agent Abstraction

This package implements the Agent interface abstraction for supporting multiple AI agents in CSM (Claude Session Manager).

## Overview

The Agent interface provides a unified API for managing AI agent sessions, enabling CSM to support multiple providers (Claude, Gemini, GPT, etc.) without duplicating session management code.

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
        │ GeminiAdapter │ (future)
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
- Delegates to existing CSM tmux infrastructure
- Maps SessionIDs to tmux session names
- Wraps existing session management logic

### session_store.go
SessionStore manages SessionID persistence:
- `SessionStore` interface - Get/Set/Delete/List operations
- `JSONSessionStore` - File-based implementation (~/.csm/sessions.json)
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
    WorkingDirectory: "/home/user/project",
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

**Storage:** `~/.csm/sessions.json`

**Format:**
```json
{
  "550e8400-e29b-41d4-a716-446655440000": {
    "tmux_name": "claude-session-1",
    "created_at": "2026-01-25T00:00:00Z",
    "working_dir": "/home/user/project",
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
- SessionStore with JSON persistence
- Basic unit tests

### TODO 🚧
- HTML export for ExportConversation
- ImportConversation implementation
- Advanced command support (authorize, run_hook)
- Integration tests
- GeminiAdapter implementation (separate bead: oss-csm-g1)

## Testing

Run unit tests:
```bash
cd internal/agent
go test -v
```

Expected output:
```
=== RUN   TestClaudeAdapterImplementsAgentInterface
--- PASS: TestClaudeAdapterImplementsAgentInterface (0.00s)
=== RUN   TestClaudeAdapterName
--- PASS: TestClaudeAdapterName (0.00s)
=== RUN   TestClaudeAdapterVersion
--- PASS: TestClaudeAdapterVersion (0.00s)
=== RUN   TestClaudeAdapterCapabilities
--- PASS: TestClaudeAdapterCapabilities (0.00s)
PASS
```

## Design Decisions

### 1. Adapter Pattern
**Chosen:** ClaudeAdapter wraps existing CSM code
**Alternative:** Refactor CSM internals to export reusable packages
**Rationale:** Minimizes changes, maintains backward compatibility

### 2. SessionID Type
**Chosen:** UUIDs (agent-agnostic)
**Alternative:** Use tmux session name directly
**Rationale:** Decouples abstraction from tmux, supports API agents

### 3. SessionStore Persistence
**Chosen:** JSON file
**Alternative:** In-memory map
**Rationale:** Survives restarts, simple to implement

## Future Extensions

### GeminiAgent (oss-csm-g1)
The Agent interface is designed to support GeminiAgent without modifications:
- CreateSession → Gemini API session creation
- SendMessage → Gemini API call
- GetHistory → Retrieve from Gemini conversation history
- Capabilities → Gemini-specific features (1M+ context window)

### Additional Agents
The interface supports any AI provider:
- GPT-4 via OpenAI API
- Local models via Ollama
- Custom agents via plugin system

## References

- Agent interface: `internal/agent/interface.go`
- ClaudeAdapter: `internal/agent/claude_adapter.go`
- SessionStore: `internal/agent/session_store.go`
- Tests: `internal/agent/claude_adapter_test.go`
- Bead: oss-csm-r2 (Agent abstraction)
- Next bead: oss-csm-g1 (Implement GeminiAgent)
