---
title: GeminiAdapter Implementation Guide
bead: oss-agm-g1-implementation (proposed)
reference_bead: oss-agm-g2
date: 2026-02-03
---

# GeminiAdapter Implementation Guide

## Quick Start

This guide shows exactly what needs to be implemented to complete GeminiAdapter and enable feature parity testing.

**Current File**: `main/agm/internal/agent/gemini_adapter.go`

**Current Status**: Stub (86 lines, 3/11 methods functional)

**Required Status**: Full implementation (est. 300-400 lines, 11/11 methods functional)

## Missing Methods (9 of 11)

### 1. CreateSession

**Current**:
```go
func (a *GeminiAdapter) CreateSession(ctx SessionContext) (SessionID, error) {
    return "", fmt.Errorf("not implemented: Gemini adapter CreateSession")
}
```

**Needs to**:
1. Initialize Google Gemini API client
2. Create new Gemini chat session
3. Generate unique SessionID (UUID)
4. Store session metadata (similar to ClaudeAdapter's SessionStore)
5. Return SessionID

**Reference**: See `claude_adapter.go:53-86` for pattern

**Dependencies**:
- `github.com/google/generative-ai-go/genai` (Gemini SDK)
- Session storage mechanism (file-based or in-memory)
- Google API key from environment (`GOOGLE_API_KEY`)

### 2. ResumeSession

**Current**:
```go
func (a *GeminiAdapter) ResumeSession(sessionID SessionID) error {
    return fmt.Errorf("not implemented: Gemini adapter ResumeSession")
}
```

**Needs to**:
1. Load session metadata from storage
2. Restore Gemini chat context
3. Load conversation history
4. Verify session is valid (not terminated)
5. Return nil on success

**Reference**: `claude_adapter.go:88-112`

**Note**: For API agents (Gemini), "resume" means loading history, not attaching to a process like Claude/tmux.

### 3. TerminateSession

**Current**:
```go
func (a *GeminiAdapter) TerminateSession(sessionID SessionID) error {
    return fmt.Errorf("not implemented: Gemini adapter TerminateSession")
}
```

**Needs to**:
1. Load session metadata
2. Close Gemini API client for this session
3. Delete session from storage
4. Clean up conversation history files
5. Return nil on success

**Reference**: `claude_adapter.go:114-136`

### 4. GetSessionStatus

**Current**:
```go
func (a *GeminiAdapter) GetSessionStatus(sessionID SessionID) (Status, error) {
    return StatusActive, fmt.Errorf("not implemented: Gemini adapter GetSessionStatus")
}
```

**Needs to**:
1. Load session metadata from storage
2. Check if session exists
3. Return appropriate Status:
   - `StatusTerminated` if not in storage
   - `StatusActive` if in storage and valid
   - `StatusSuspended` if paused (optional)
4. Return status and nil error

**Reference**: `claude_adapter.go:138-161`

**Note**: Gemini is stateless API, so status is based on metadata existence, not active connection.

### 5. SendMessage

**Current**:
```go
func (a *GeminiAdapter) SendMessage(sessionID SessionID, message Message) error {
    return fmt.Errorf("not implemented: Gemini adapter SendMessage")
}
```

**Needs to**:
1. Load session metadata and history
2. Verify session is active
3. Create Gemini API request with:
   - User message content
   - Conversation history (for context)
4. Call Gemini API
5. Append user message and assistant response to history
6. Save updated history
7. Return nil on success

**Reference**: `claude_adapter.go:163-178` (basic pattern)

**Gemini-Specific**:
```go
// Example Gemini API call
model := genai.NewGenerativeModel(client, "gemini-1.5-pro")
chat := model.StartChat()
chat.History = loadHistoryForSession(sessionID)

resp, err := chat.SendMessage(ctx, genai.Text(message.Content))
if err != nil {
    return fmt.Errorf("failed to send message: %w", err)
}

// Save response to history
saveMessageToHistory(sessionID, message)          // user message
saveMessageToHistory(sessionID, responseMessage)  // assistant response
```

### 6. GetHistory

**Current**:
```go
func (a *GeminiAdapter) GetHistory(sessionID SessionID) ([]Message, error) {
    return nil, fmt.Errorf("not implemented: Gemini adapter GetHistory")
}
```

**Needs to**:
1. Load session metadata
2. Find conversation history file/storage
3. Parse history into []Message
4. Return messages in chronological order
5. Return empty slice (not nil) if no history exists

**Reference**: `claude_adapter.go:180-227`

**Storage Options**:
- **Option A**: JSONL file (like Claude) at `~/.agm/gemini-sessions/<session-id>/history.jsonl`
- **Option B**: In-memory map (loses history on restart)
- **Option C**: SQLite database

**Recommended**: JSONL file for consistency with Claude

### 7. ExportConversation

**Current**:
```go
func (a *GeminiAdapter) ExportConversation(sessionID SessionID, format ConversationFormat) ([]byte, error) {
    return nil, fmt.Errorf("not implemented: Gemini adapter ExportConversation")
}
```

**Needs to**:
1. Get conversation history via `GetHistory()`
2. Convert to requested format:
   - `FormatJSONL`: One JSON object per line
   - `FormatMarkdown`: Human-readable format
   - `FormatHTML`: Optional (ClaudeAdapter also returns "not implemented")
3. Return serialized data as []byte

**Reference**: `claude_adapter.go:229-271`

**Implementation**:
```go
switch format {
case FormatJSONL:
    return serializeJSONL(messages), nil
case FormatMarkdown:
    return serializeMarkdown(messages), nil
case FormatHTML:
    return nil, fmt.Errorf("HTML export not yet implemented")
default:
    return nil, fmt.Errorf("unsupported format: %s", format)
}
```

### 8. ImportConversation

**Current**:
```go
func (a *GeminiAdapter) ImportConversation(data []byte, format ConversationFormat) (SessionID, error) {
    return "", fmt.Errorf("not implemented: Gemini adapter ImportConversation")
}
```

**Needs to**:
1. Parse conversation data based on format
2. Create new session
3. Inject parsed messages as initial history
4. Return new SessionID

**Reference**: `claude_adapter.go:273-283`

**Note**: ClaudeAdapter also returns "not implemented" - this is acceptable for V1. Can match Claude behavior:

```go
func (a *GeminiAdapter) ImportConversation(data []byte, format ConversationFormat) (SessionID, error) {
    return "", fmt.Errorf("conversation import not yet implemented")
}
```

### 9. ExecuteCommand

**Current**:
```go
func (a *GeminiAdapter) ExecuteCommand(cmd Command) error {
    return fmt.Errorf("not implemented: Gemini adapter ExecuteCommand")
}
```

**Needs to**:
Implement commands that make sense for API-based agent:

1. **CommandRename**: Update session name in metadata
2. **CommandSetDir**: Update working directory in context
3. **CommandAuthorize**: Add directory to authorized list (or return "not supported")
4. **CommandRunHook**: Execute hook (or return "not yet implemented" like Claude)

**Reference**: `claude_adapter.go:298-335`

**Implementation**:
```go
func (a *GeminiAdapter) ExecuteCommand(cmd Command) error {
    sessionID := agent.SessionID(cmd.Params["session_id"].(string))

    switch cmd.Type {
    case CommandRename:
        name := cmd.Params["name"].(string)
        return a.renameSession(sessionID, name)

    case CommandSetDir:
        path := cmd.Params["path"].(string)
        return a.setWorkingDirectory(sessionID, path)

    case CommandAuthorize:
        // Can implement or return error
        return fmt.Errorf("authorize command not yet implemented")

    case CommandRunHook:
        // Can implement or return error
        return fmt.Errorf("run_hook command not yet implemented")

    default:
        return fmt.Errorf("unsupported command type: %s", cmd.Type)
    }
}
```

## Already Implemented (3 of 11)

These methods are complete and don't need changes:

### Name()
```go
func (a *GeminiAdapter) Name() string {
    return "gemini"
}
```
✅ **Status**: Complete

### Version()
```go
func (a *GeminiAdapter) Version() string {
    return "gemini-1.5-pro"
}
```
✅ **Status**: Complete (but could update to latest model like "gemini-2.0-flash")

### Capabilities()
```go
func (a *GeminiAdapter) Capabilities() Capabilities {
    return Capabilities{
        SupportsSlashCommands: false,
        SupportsHooks:         false,
        SupportsTools:         true,
        SupportsVision:        true,
        SupportsMultimodal:    false,
        MaxContextWindow:      1000000,
        ModelName:             "gemini-1.5-pro",
    }
}
```
✅ **Status**: Complete (but update ModelName if changing to gemini-2.0)

## Implementation Strategy

### Phase 1: Core Session Management (2-3 hours)
1. Add Google Gemini SDK dependency
2. Implement session storage (SessionStore pattern)
3. Implement CreateSession
4. Implement GetSessionStatus
5. Implement TerminateSession
6. Write unit tests

### Phase 2: Messaging (2-3 hours)
1. Implement SendMessage with Gemini API
2. Implement GetHistory
3. Add conversation history persistence
4. Write unit tests

### Phase 3: Data Exchange (1-2 hours)
1. Implement ExportConversation (JSONL, Markdown)
2. Optionally implement ImportConversation (or defer like Claude)
3. Write unit tests

### Phase 4: Commands (1-2 hours)
1. Implement ExecuteCommand (rename, setdir)
2. Defer authorize/hooks (match Claude behavior)
3. Write unit tests

### Phase 5: Testing (1-2 hours)
1. Run unit tests
2. Run integration tests from oss-agm-g2 test plan
3. Verify feature parity with Claude
4. Fix any bugs

**Total Estimated Time**: 8-12 hours

## Dependencies

### Required Go Packages

```go
import (
    "context"
    "encoding/json"
    "fmt"
    "os"
    "path/filepath"
    "sync"
    "time"

    "github.com/google/uuid"
    "github.com/google/generative-ai-go/genai"
    "google.golang.org/api/option"
)
```

### Required Environment Variables

```bash
# Google API key for Gemini
export GOOGLE_API_KEY="your-api-key-here"

# Optional: Vertex AI configuration
export GOOGLE_CLOUD_PROJECT="your-project-id"
export GOOGLE_APPLICATION_CREDENTIALS="/path/to/service-account.json"
```

### go.mod Updates

```bash
go get github.com/google/generative-ai-go@latest
go get google.golang.org/api@latest
```

## Session Storage Design

### Recommended Structure

```
~/.agm/gemini-sessions/
├── sessions.json                    # SessionID -> Metadata mapping
└── <session-uuid>/
    ├── history.jsonl                # Conversation history
    └── context.json                 # Session context (working dir, etc.)
```

### Metadata Format (sessions.json)

```json
{
  "550e8400-e29b-41d4-a716-446655440000": {
    "name": "gemini-session-1",
    "created_at": "2026-02-03T12:00:00Z",
    "working_dir": "~/project",
    "project": "my-project",
    "model": "gemini-1.5-pro"
  }
}
```

### History Format (history.jsonl)

```jsonl
{"id":"msg-001","role":"user","content":"Hello","timestamp":"2026-02-03T12:01:00Z"}
{"id":"msg-002","role":"assistant","content":"Hi there!","timestamp":"2026-02-03T12:01:01Z"}
```

## Testing Checklist

After implementation, verify:

- [ ] All 11 Agent interface methods implemented
- [ ] Unit tests pass for GeminiAdapter
- [ ] `go test ./internal/agent/...` passes
- [ ] Can create Gemini session
- [ ] Can send messages to Gemini
- [ ] Can retrieve conversation history
- [ ] Can export conversation (JSONL, Markdown)
- [ ] Can terminate session
- [ ] Session metadata persists across restarts
- [ ] Conversation history persists
- [ ] Feature parity tests from oss-agm-g2 pass

## Example Implementation Skeleton

```go
package agent

import (
    "context"
    "encoding/json"
    "fmt"
    "os"
    "path/filepath"
    "sync"
    "time"

    "github.com/google/uuid"
    "github.com/google/generative-ai-go/genai"
    "google.golang.org/api/option"
)

type GeminiAdapter struct {
    sessionStore SessionStore
    sessions     map[SessionID]*geminiSession
    mu           sync.RWMutex
}

type geminiSession struct {
    client  *genai.Client
    model   *genai.GenerativeModel
    chat    *genai.ChatSession
    history []Message
}

func NewGeminiAdapter() (Agent, error) {
    store, err := NewJSONSessionStore(filepath.Join(os.Getenv("HOME"), ".agm", "gemini-sessions.json"))
    if err != nil {
        return nil, fmt.Errorf("failed to create session store: %w", err)
    }

    return &GeminiAdapter{
        sessionStore: store,
        sessions:     make(map[SessionID]*geminiSession),
    }, nil
}

// Implement all 11 methods here...
```

## Reference Files

Study these for implementation patterns:

1. **ClaudeAdapter**: `internal/agent/claude_adapter.go` (336 lines)
   - Session management patterns
   - SessionStore usage
   - Error handling

2. **SessionStore**: `internal/agent/session_store.go` (session persistence)
   - File-based storage
   - Metadata structure

3. **Mock Gemini**: `test/bdd/internal/adapters/mock/gemini.go` (178 lines)
   - Mock implementation (different API but shows Gemini patterns)
   - Response generation logic

4. **Agent Interface**: `internal/agent/interface.go` (271 lines)
   - Full interface definition
   - Type documentation

## Success Criteria

Implementation is complete when:

1. ✅ All 9 stub methods implemented
2. ✅ Unit tests pass
3. ✅ Can create Gemini session via `agm new --harness=gemini-cli`
4. ✅ Can send/receive messages
5. ✅ Conversation history persists
6. ✅ Integration tests from oss-agm-g2 pass
7. ✅ Feature parity with ClaudeAdapter verified

## Next Steps After Implementation

1. Resume bead oss-agm-g2 for feature parity testing
2. Run comprehensive integration test suite
3. Update documentation to mark Gemini as "production ready"
4. Add to CI/CD pipeline
5. Announce Gemini support to users

---

**This guide provides everything needed to complete GeminiAdapter implementation.**

**Estimated effort**: 8-12 hours
**Recommended approach**: Follow phases 1-5 sequentially
**Test plan ready**: See FEATURE-PARITY-TEST-PLAN.md
