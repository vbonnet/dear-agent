# GPT Adapter Technical Specification

**Version:** 1.0
**Status:** Implemented
**Last Updated:** 2026-02-11

## Overview

The GPT Adapter provides OpenAI GPT-4 integration for the Claude Session Manager's unified Agent interface. It enables CSM users to use GPT-4 as an alternative AI agent while maintaining API compatibility with the broader agent ecosystem.

## Purpose

Provide a production-ready adapter that:
- Implements all 12 methods of the `agent.Agent` interface
- Supports stateful conversation management with in-memory storage
- Handles OpenAI API authentication, rate limiting, and error recovery
- Enables conversation import/export in standard formats
- Maintains thread safety for concurrent operations

## Requirements

### Functional Requirements

#### FR1: Agent Interface Compliance
- **ID:** FR1
- **Priority:** P0 (Critical)
- **Description:** Adapter MUST implement all 12 methods of `agent.Agent` interface
- **Methods:**
  - `Name() string` - Returns "gpt"
  - `Version() string` - Returns model name (e.g., "gpt-4o")
  - `Capabilities() Capabilities` - Returns feature support flags
  - `CreateSession(SessionContext) (SessionID, error)` - Creates new session
  - `ResumeSession(SessionID) error` - Validates session exists
  - `TerminateSession(SessionID) error` - Deletes session
  - `GetSessionStatus(SessionID) (Status, error)` - Returns active/terminated
  - `SendMessage(SessionID, Message) error` - Sends message and gets response
  - `GetHistory(SessionID) ([]Message, error)` - Retrieves conversation
  - `ExportConversation(SessionID, ConversationFormat) ([]byte, error)` - Exports data
  - `ImportConversation([]byte, ConversationFormat) (SessionID, error)` - Imports data
  - `ExecuteCommand(Command) error` - Handles rename/setdir commands
- **Validation:** Compile-time check: `var _ agent.Agent = (*Adapter)(nil)`

#### FR2: Session Management
- **ID:** FR2
- **Priority:** P0 (Critical)
- **Description:** Adapter MUST manage stateful conversation sessions
- **Behavior:**
  - Each session identified by unique UUID (SessionID)
  - Sessions stored in-memory map (no persistence in V1)
  - Session context includes: name, working directory, environment
  - Session data includes: messages, status, timestamps
  - Thread-safe access via `sync.RWMutex`
- **Constraints:**
  - Session names must be non-empty
  - Working directory must be non-empty
  - Sessions lost on process restart (V1 limitation)

#### FR3: OpenAI API Integration
- **ID:** FR3
- **Priority:** P0 (Critical)
- **Description:** Adapter MUST integrate with OpenAI Chat Completions API
- **Implementation:**
  - Uses official `github.com/sashabaranov/go-openai` SDK
  - Default model: `gpt-4o` (128K context window)
  - API key from `OPENAI_API_KEY` environment variable
  - 30-second timeout per API request
  - Exponential backoff for rate limits (429 errors)
  - Maximum 5 retry attempts
- **Error Handling:**
  - 401 (Auth): Immediate failure with clear error
  - 429 (Rate Limit): Retry with backoff (1s, 2s, 4s, 8s, 16s)
  - Timeout: Return error after 30 seconds
  - Max retries: Return `ErrMaxRetriesExceeded` after 5 attempts

#### FR4: Message Translation
- **ID:** FR4
- **Priority:** P0 (Critical)
- **Description:** Adapter MUST translate between agent.Message and OpenAI formats
- **Translation Rules:**
  - `agent.RoleUser` ↔ `openai.ChatMessageRoleUser`
  - `agent.RoleAssistant` ↔ `openai.ChatMessageRoleAssistant`
  - Each message assigned unique UUID
  - Timestamps recorded for all messages
  - Model name stored in message metadata
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
  - Markdown: Format as `## {role}\n\n{content}\n\n`
- **Import Behavior:**
  - JSONL only (V1)
  - Creates new session with imported messages
  - Default context: name="imported-session", dir="/tmp"

#### FR6: Command Execution
- **ID:** FR6
- **Priority:** P2 (Medium)
- **Description:** Adapter MUST handle generic agent commands
- **Supported Commands:**
  - `CommandRename`: Updates session name
  - `CommandSetDir`: Updates working directory
  - `CommandAuthorize`: No-op (API agents have no directory restrictions)
- **Unsupported Commands:**
  - `CommandRunHook`: Returns error (hooks not supported for API agents)
  - `CommandClearHistory`: Not implemented in V1
  - `CommandSetSystemPrompt`: Not implemented in V1

### Non-Functional Requirements

#### NFR1: Thread Safety
- **ID:** NFR1
- **Priority:** P0 (Critical)
- **Description:** All public methods MUST be thread-safe
- **Implementation:**
  - Read operations: Use `mu.RLock()` / `mu.RUnlock()`
  - Write operations: Use `mu.Lock()` / `mu.Unlock()`
  - Session map protected by `sync.RWMutex`
- **Validation:** Pass `go test -race` with no warnings

#### NFR2: Performance
- **ID:** NFR2
- **Priority:** P1 (High)
- **Description:** Adapter MUST meet performance targets
- **Targets:**
  - Session creation: < 1ms (in-memory only)
  - API call timeout: 30 seconds
  - Retry backoff: Exponential (1s → 16s max)
  - Memory footprint: O(n) where n = total messages across sessions

#### NFR3: Reliability
- **ID:** NFR3
- **Priority:** P1 (High)
- **Description:** Adapter MUST handle API failures gracefully
- **Guarantees:**
  - No panics on API errors
  - Clear error messages with context
  - Automatic retry for transient failures (rate limits)
  - Immediate failure for permanent errors (auth)

#### NFR4: Testability
- **ID:** NFR4
- **Priority:** P1 (High)
- **Description:** Adapter MUST have comprehensive test coverage
- **Requirements:**
  - Unit tests: >90% code coverage
  - No API key required for unit tests
  - Integration tests: Available but optional (require OPENAI_API_KEY)
  - Race condition testing: `go test -race` passes

## Data Structures

### Session
```go
type Session struct {
    ID        agent.SessionID      // UUID
    Context   agent.SessionContext // name, working dir, etc.
    Messages  []agent.Message      // Conversation history
    Status    agent.Status         // active/terminated
    CreatedAt time.Time           // Session creation time
    UpdatedAt time.Time           // Last modification time
}
```

### Adapter
```go
type Adapter struct {
    client   *openai.Client              // OpenAI SDK client
    sessions map[agent.SessionID]*Session // In-memory storage
    mu       sync.RWMutex                // Thread-safety lock
    model    string                      // GPT model name
}
```

## API Contract

### Input Validation

#### CreateSession
- **Name:** Must be non-empty string
- **WorkingDirectory:** Must be non-empty string
- **Returns:** UUID SessionID or error

#### SendMessage
- **SessionID:** Must exist in sessions map
- **Message.Role:** Must be agent.RoleUser or agent.RoleAssistant
- **Message.Content:** Can be empty (valid for some use cases)
- **Returns:** nil on success, error on failure (session not found, API error)

#### ExportConversation
- **SessionID:** Must exist in sessions map
- **Format:** Must be agent.FormatJSONL or agent.FormatMarkdown
- **Returns:** Serialized bytes or error

#### ImportConversation
- **Data:** Must be valid JSONL (one JSON object per line)
- **Format:** Must be agent.FormatJSONL (only supported format in V1)
- **Returns:** New SessionID or error

### Error Conditions

| Error | Condition | Handling |
|-------|-----------|----------|
| `ErrAPIKeyNotSet` | `OPENAI_API_KEY` not in environment | Return immediately from `NewAdapter()` |
| `ErrSessionNotFound` | SessionID not in map | Return error from any session operation |
| `ErrInvalidFormat` | Unsupported conversation format | Return from Export/Import |
| `ErrMaxRetriesExceeded` | 5 retry attempts exhausted | Return from `SendMessage()` |
| `APIError{StatusCode: 401}` | Invalid OpenAI API key | Return immediately (no retry) |
| `APIError{StatusCode: 429}` | Rate limit hit | Retry with exponential backoff |

## Capabilities

```go
Capabilities{
    SupportsSlashCommands: false,  // API agent, not CLI
    SupportsHooks:         false,  // V1: not implemented
    SupportsTools:         true,   // GPT-4 supports function calling (V2)
    SupportsVision:        true,   // GPT-4V capable (V2)
    SupportsMultimodal:    false,  // Future feature
    SupportsStreaming:     false,  // V1: not implemented
    SupportsSystemPrompts: false,  // V1: not implemented
    MaxContextWindow:      128000, // gpt-4o: 128K tokens
    ModelName:             "gpt-4o",
}
```

## Limitations (V1)

### Known Constraints
1. **No Persistence:** Sessions stored in-memory only, lost on restart
2. **No Streaming:** Responses returned only when complete
3. **No Tool Calling:** GPT-4 function calling not implemented
4. **No Vision Input:** Image support not implemented
5. **No System Prompts:** Custom system prompts not supported
6. **No Context Management:** No automatic truncation at 128K token limit
7. **JSONL Import Only:** Cannot import Markdown or HTML formats

### V2 Roadmap
- File-based session persistence (JSONL storage)
- Streaming response support (`client.CreateChatCompletionStream`)
- Tool/function calling implementation
- Vision input handling (image URLs in messages)
- System prompt configuration
- Automatic context window management (truncation/summarization)
- Token usage tracking and cost estimation

## Dependencies

### External Packages
- `github.com/sashabaranov/go-openai` - Official OpenAI Go SDK
- `github.com/google/uuid` - UUID generation for session IDs
- `github.com/stretchr/testify` - Testing assertions (dev only)

### Internal Packages
- `github.com/vbonnet/ai-tools/claude-session-manager/internal/agent` - Agent interface

## Security

### API Key Management
- **Storage:** Environment variable `OPENAI_API_KEY` only
- **Validation:** Checked at adapter creation time
- **Never:** Logged, stored in config files, or committed to git

### Error Disclosure
- API errors sanitized (no raw API key exposure)
- Stack traces excluded from production errors
- Generic messages for auth failures

## Testing Strategy

### Unit Tests (No API Key Required)
```bash
go test ./internal/agent/gpt -v
```
- Interface compliance check
- Session CRUD operations
- Message translation (agent ↔ OpenAI)
- Export/Import (JSONL, Markdown)
- Command execution (rename, setdir)
- Error handling (session not found, invalid format)
- Thread safety (concurrent access)

### Integration Tests (Requires API Key)
```bash
export OPENAI_API_KEY="sk-..."
export INTEGRATION_TESTS=true
go test ./internal/agent/gpt -v
```
- Live API calls to OpenAI
- End-to-end conversation flow
- Rate limit handling (manual trigger)
- Authentication validation

### Race Condition Testing
```bash
go test ./internal/agent/gpt -race
```
- Validates thread-safe mutex usage
- Detects data races in concurrent operations

## Acceptance Criteria

### V1 Completion Checklist
- [x] All 12 Agent interface methods implemented
- [x] Compile-time interface compliance verified
- [x] Thread-safe session management
- [x] OpenAI API integration with retry logic
- [x] JSONL and Markdown export
- [x] JSONL import
- [x] Command execution (rename, setdir)
- [x] >90% unit test coverage
- [x] Integration tests (optional, documented)
- [x] Race condition testing passes
- [x] README documentation complete
- [x] Error handling with clear messages

## References

- [Agent Interface](../interface.go)
- [OpenAI API Documentation](https://platform.openai.com/docs)
- [OpenAI Go SDK](https://pkg.go.dev/github.com/sashabaranov/go-openai)
- [GPT Adapter README](README.md)
