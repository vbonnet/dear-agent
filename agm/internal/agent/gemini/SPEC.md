# Gemini Adapter Technical Specification

**Version:** 1.0
**Status:** Implemented
**Last Updated:** 2026-02-11

## Overview

The Gemini Adapter provides Google Gemini 2.0 integration for the Claude Session Manager's unified Agent interface. It enables AGM users to use Gemini as an alternative AI agent while maintaining API compatibility with the broader agent ecosystem.

## Purpose

Provide a production-ready adapter that:
- Implements all 12 methods of the `agent.Agent` interface
- Supports stateful conversation management with file-based JSONL storage
- Handles Google AI API authentication and error recovery
- Enables conversation import/export in standard formats
- Maintains session persistence across process restarts

## Requirements

### Functional Requirements

#### FR1: Agent Interface Compliance
- **ID:** FR1
- **Priority:** P0 (Critical)
- **Description:** Adapter MUST implement all 12 methods of `agent.Agent` interface
- **Methods:**
  - `Name() string` - Returns "gemini"
  - `Version() string` - Returns model name (e.g., "gemini-2.0-flash-exp")
  - `Capabilities() Capabilities` - Returns feature support flags
  - `CreateSession(SessionContext) (SessionID, error)` - Creates new session
  - `ResumeSession(SessionID) error` - Validates session exists
  - `TerminateSession(SessionID) error` - Deletes session metadata
  - `GetSessionStatus(SessionID) (Status, error)` - Returns active/terminated
  - `SendMessage(SessionID, Message) error` - Sends message and gets response
  - `GetHistory(SessionID) ([]Message, error)` - Retrieves conversation
  - `ExportConversation(SessionID, ConversationFormat) ([]byte, error)` - Exports data
  - `ImportConversation([]byte, ConversationFormat) (SessionID, error)` - Imports data
  - `ExecuteCommand(Command) error` - Handles rename/setdir commands
- **Validation:** Compile-time check: `var _ agent.Agent = (*GeminiAdapter)(nil)`

#### FR2: Session Management
- **ID:** FR2
- **Priority:** P0 (Critical)
- **Description:** Adapter MUST manage stateful conversation sessions
- **Behavior:**
  - Each session identified by unique UUID (SessionID)
  - Session metadata stored in JSON store (~/.agm/sessions.json)
  - Session history stored as JSONL (~/.agm/gemini/<session-id>/history.jsonl)
  - Session context includes: name, working directory, project
  - History persists across process restarts
- **Constraints:**
  - Session names must be non-empty
  - Working directory must be non-empty
  - Session directory created at ~/.agm/gemini/<session-id>/

#### FR3: Google AI API Integration
- **ID:** FR3
- **Priority:** P0 (Critical)
- **Description:** Adapter MUST integrate with Google Generative AI API
- **Implementation:**
  - Uses official `github.com/google/generative-ai-go/genai` SDK
  - Default model: `gemini-2.0-flash-exp` (2M token context window)
  - API key from `GEMINI_API_KEY` environment variable
  - Full conversation history sent with each request
  - Chat session created with historical context
- **Error Handling:**
  - 401 (Auth): Immediate failure with clear error
  - Network errors: Return error immediately (no retry in V1)
  - API errors: Wrapped with context

#### FR4: Message Translation
- **ID:** FR4
- **Priority:** P0 (Critical)
- **Description:** Adapter MUST translate between agent.Message and Gemini formats
- **Translation Rules:**
  - `agent.RoleUser` ↔ `genai.Content{Role: "user"}`
  - `agent.RoleAssistant` ↔ `genai.Content{Role: "model"}`
  - Each message assigned unique UUID
  - Timestamps recorded for all messages
  - Content extracted from `genai.Text` parts
- **Data Integrity:**
  - Message content preserved exactly
  - Conversation order maintained
  - No message loss during translation

#### FR5: Conversation Export/Import
- **ID:** FR5
- **Priority:** P1 (High)
- **Description:** Adapter MUST support conversation serialization
- **Supported Formats:**
  - **JSONL** (Primary): One JSON message per line, universal format
  - **Markdown** (Secondary): Human-readable transcript
- **Export Behavior:**
  - JSONL: Serialize all messages with full metadata
  - Markdown: Format as `## {role} ({timestamp})\n\n{content}\n\n`
  - HTML: Not supported (returns error)
- **Import Behavior:**
  - JSONL only (V1)
  - Creates new session with imported messages
  - Default context: name="imported-{timestamp}", dir=os.TempDir()

#### FR6: Command Execution
- **ID:** FR6
- **Priority:** P2 (Medium)
- **Description:** Adapter MUST handle generic agent commands
- **Supported Commands:**
  - `CommandRename`: No-op (metadata update could be added)
  - `CommandSetDir`: No-op (API agents don't have working directory)
  - `CommandAuthorize`: No-op (API agents have no directory restrictions)
- **Unsupported Commands:**
  - `CommandRunHook`: Returns error (hooks not supported for API agents)
  - `CommandClearHistory`: Not implemented in V1
  - `CommandSetSystemPrompt`: Not implemented in V1

### Non-Functional Requirements

#### NFR1: File-Based Persistence
- **ID:** NFR1
- **Priority:** P0 (Critical)
- **Description:** Sessions MUST persist across process restarts
- **Implementation:**
  - Session metadata: ~/.agm/sessions.json (JSON store)
  - Conversation history: ~/.agm/gemini/<session-id>/history.jsonl
  - JSONL format: One JSON message per line
  - Append-only writes for history
- **Guarantees:**
  - Session survives process restart
  - History preserved on disk
  - Session directory retained after termination

#### NFR2: Performance
- **ID:** NFR2
- **Priority:** P1 (High)
- **Description:** Adapter MUST meet performance targets
- **Targets:**
  - Session creation: < 10ms (create directory + write metadata)
  - API call: Depends on Gemini API latency
  - History loading: O(n) where n = number of messages
  - Memory footprint: O(n) for loaded history

#### NFR3: Reliability
- **ID:** NFR3
- **Priority:** P1 (High)
- **Description:** Adapter MUST handle API failures gracefully
- **Guarantees:**
  - No panics on API errors
  - Clear error messages with context
  - File I/O errors handled with cleanup
  - Partial writes avoided (append-only)

#### NFR4: Testability
- **ID:** NFR4
- **Priority:** P1 (High)
- **Description:** Adapter MUST have comprehensive test coverage
- **Requirements:**
  - Unit tests: >90% code coverage
  - No API key required for unit tests
  - Integration tests: Available but optional (require GEMINI_API_KEY)
  - Mock session store for testing

## Data Structures

### GeminiAdapter
```go
type GeminiAdapter struct {
    sessionStore SessionStore // Metadata storage
    modelName    string       // Gemini model name
    apiKey       string       // Google AI API key
}
```

### GeminiConfig
```go
type GeminiConfig struct {
    ModelName    string       // Default: gemini-2.0-flash-exp
    APIKey       string       // From env or explicit
    SessionStore SessionStore // Custom store or default JSON
}
```

## API Contract

### Input Validation

#### CreateSession
- **Name:** Must be non-empty string
- **WorkingDirectory:** Must be non-empty string
- **Returns:** UUID SessionID or error

#### SendMessage
- **SessionID:** Must exist in session store
- **Message.Role:** Must be agent.RoleUser or agent.RoleAssistant
- **Message.Content:** Can be empty (valid for some use cases)
- **Returns:** nil on success, error on failure (session not found, API error)

#### ExportConversation
- **SessionID:** Must exist in session store
- **Format:** Must be agent.FormatJSONL or agent.FormatMarkdown
- **Returns:** Serialized bytes or error

#### ImportConversation
- **Data:** Must be valid JSONL (one JSON object per line)
- **Format:** Must be agent.FormatJSONL (only supported format in V1)
- **Returns:** New SessionID or error

### Error Conditions

| Error | Condition | Handling |
|-------|-----------|----------|
| `ErrAPIKeyNotSet` | `GEMINI_API_KEY` not in environment | Return immediately from `NewGeminiAdapter()` |
| `ErrSessionNotFound` | SessionID not in store | Return error from any session operation |
| `ErrInvalidFormat` | Unsupported conversation format | Return from Export/Import |
| `APIError` | Gemini API error | Return from `SendMessage()` |
| `FileError` | Session directory or history file error | Return with context |

## Capabilities

```go
Capabilities{
    SupportsSlashCommands: false,   // API agent, not CLI
    SupportsHooks:         false,   // AGM-level feature
    SupportsTools:         true,    // Gemini supports function calling
    SupportsVision:        true,    // Gemini 2.0 supports vision
    SupportsMultimodal:    true,    // Gemini 2.0 supports audio/video
    SupportsStreaming:     true,    // Gemini API supports streaming (not impl in V1)
    SupportsSystemPrompts: true,    // Gemini supports system instructions
    MaxContextWindow:      1000000, // 1M tokens (2M for 2.0 Flash Thinking)
    ModelName:             "gemini-2.0-flash-exp",
}
```

## Storage Layout

### Session Directory Structure
```
~/.agm/
├── sessions.json              # Session metadata store
└── gemini/
    └── {session-id}/
        └── history.jsonl      # Conversation history
```

### history.jsonl Format
```jsonl
{"id":"msg-1","role":"user","content":"Hello","timestamp":"2026-02-11T10:00:00Z"}
{"id":"msg-2","role":"assistant","content":"Hi!","timestamp":"2026-02-11T10:00:01Z"}
```

## Limitations (V1)

### Known Constraints
1. **No Retry Logic:** API failures return immediately (no exponential backoff)
2. **No Streaming:** Responses returned only when complete
3. **No Tool Calling:** Gemini function calling not implemented
4. **No Vision Input:** Image support not implemented
5. **No System Instructions:** Custom system prompts not supported
6. **No Context Management:** No automatic truncation at 1M/2M token limit
7. **JSONL Import Only:** Cannot import Markdown or HTML formats
8. **Session Directory Preserved:** Termination doesn't delete history files

### V2 Roadmap
- Streaming response support (`chat.SendMessageStream`)
- Tool/function calling implementation
- Vision input handling (image URLs in messages)
- System instruction configuration
- Automatic context window management (truncation/summarization)
- Token usage tracking and cost estimation
- Retry logic with exponential backoff
- Session directory cleanup options

## Dependencies

### External Packages
- `github.com/google/generative-ai-go/genai` - Official Google Generative AI Go SDK
- `google.golang.org/api/option` - Google API client options
- `github.com/google/uuid` - UUID generation for session/message IDs
- `github.com/stretchr/testify` - Testing assertions (dev only)

### Internal Packages
- `github.com/vbonnet/dear-agent/agm/internal/agent` - Agent interface

## Security

### API Key Management
- **Storage:** Environment variable `GEMINI_API_KEY` only
- **Validation:** Checked at adapter creation time
- **Never:** Logged, stored in config files, or committed to git

### Error Disclosure
- API errors sanitized (no raw API key exposure)
- Stack traces excluded from production errors
- Generic messages for auth failures

### File Permissions
- Session directories: 0755 (world-readable)
- History files: 0644 (world-readable)
- No sensitive data in history (user/assistant messages only)

## Testing Strategy

### Unit Tests (No API Key Required)
```bash
go test ./internal/agent -run TestGeminiAdapter
```
- Interface compliance check
- Session CRUD operations
- Message history (read/write)
- Export/Import (JSONL, Markdown)
- Command execution (no-ops)
- Error handling (session not found, invalid format)

### Integration Tests (Requires API Key)
```bash
export GEMINI_API_KEY="..."
export INTEGRATION_TESTS=true
go test ./internal/agent -run TestGeminiIntegration
```
- Live API calls to Gemini
- End-to-end conversation flow
- Authentication validation

## Acceptance Criteria

### V1 Completion Checklist
- [x] All 12 Agent interface methods implemented
- [x] Compile-time interface compliance verified
- [x] File-based session storage (JSONL)
- [x] Google AI API integration
- [x] JSONL and Markdown export
- [x] JSONL import
- [x] Command execution (no-ops documented)
- [x] >90% unit test coverage
- [x] Integration tests (optional, documented)
- [x] Error handling with clear messages
- [x] Session persistence across restarts

## References

- [Agent Interface](../interface.go)
- [Google Generative AI API Documentation](https://ai.google.dev/docs)
- [Gemini Go SDK](https://pkg.go.dev/github.com/google/generative-ai-go/genai)
- [Gemini Adapter Implementation](../gemini_adapter.go)
