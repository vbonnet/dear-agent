# GPT Adapter Architecture

**Version:** 1.0
**Last Updated:** 2026-02-11
**Status:** Implemented

## System Overview

The GPT Adapter is a component within the Claude Session Manager (CSM) agent ecosystem that provides OpenAI GPT-4 integration. It implements the unified `agent.Agent` interface, enabling GPT-4 to be used interchangeably with Claude and Gemini agents.

### Architecture Diagram

```
┌─────────────────────────────────────────────────────────────┐
│                    Claude Session Manager                    │
│                                                               │
│  ┌───────────────────────────────────────────────────────┐  │
│  │              Agent Interface (12 methods)              │  │
│  └───────────────────────────────────────────────────────┘  │
│           │                  │                  │             │
│           ▼                  ▼                  ▼             │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐       │
│  │    Claude    │  │    Gemini    │  │     GPT      │       │
│  │   Adapter    │  │   Adapter    │  │   Adapter    │       │
│  └──────────────┘  └──────────────┘  └──────────────┘       │
│                                              │                │
└──────────────────────────────────────────────┼────────────────┘
                                               │
                                               ▼
                              ┌────────────────────────────┐
                              │   OpenAI Chat Completion   │
                              │          API (GPT-4)        │
                              └────────────────────────────┘
```

## Component Architecture

### Core Components

#### 1. Adapter (`adapter.go`)
**Responsibility:** Main entry point implementing `agent.Agent` interface

**Key Methods:**
- Session lifecycle: `CreateSession`, `ResumeSession`, `TerminateSession`
- Communication: `SendMessage`, `GetHistory`
- Serialization: `ExportConversation`, `ImportConversation`
- Commands: `ExecuteCommand`
- Metadata: `Name`, `Version`, `Capabilities`

**Dependencies:**
- `openai.Client` - OpenAI SDK client
- `Session` - Session data structure
- `sync.RWMutex` - Thread-safety lock

**Design Patterns:**
- **Adapter Pattern:** Translates `agent.Agent` interface to OpenAI API
- **Repository Pattern:** In-memory session storage with map
- **Singleton:** Single OpenAI client instance per adapter

#### 2. Session (`session.go`)
**Responsibility:** Data structure for conversation state

**Fields:**
```go
type Session struct {
    ID        agent.SessionID      // UUID identifier
    Context   agent.SessionContext // Session metadata
    Messages  []agent.Message      // Conversation history
    Status    agent.Status         // active/terminated
    CreatedAt time.Time           // Creation timestamp
    UpdatedAt time.Time           // Last modification timestamp
}
```

**Lifecycle:**
1. Created via `CreateSession()` with UUID
2. Updated via `SendMessage()` (appends messages)
3. Queried via `GetHistory()`, `GetSessionStatus()`
4. Deleted via `TerminateSession()`

#### 3. Translator (`translator.go`)
**Responsibility:** Bidirectional message format conversion

**Functions:**
- `toOpenAIMessage(agent.Message) → openai.ChatCompletionMessage`
- `fromOpenAIMessage(openai.ChatCompletionMessage) → agent.Message`
- `toOpenAIMessages([]agent.Message) → []openai.ChatCompletionMessage`

**Translation Table:**

| Agent Field | OpenAI Field | Transformation |
|-------------|--------------|----------------|
| `agent.RoleUser` | `openai.ChatMessageRoleUser` | Direct mapping |
| `agent.RoleAssistant` | `openai.ChatMessageRoleAssistant` | Direct mapping |
| `message.Content` | `message.Content` | Passthrough |
| `message.ID` | N/A | Generated UUID |
| `message.Timestamp` | N/A | Current time |
| N/A | `message.Role` | From response |

#### 4. Error Handler (`errors.go`)
**Responsibility:** Centralized error definitions and handling

**Error Types:**
```go
var (
    ErrAPIKeyNotSet       // OPENAI_API_KEY not configured
    ErrSessionNotFound    // Session ID not in storage
    ErrInvalidSessionID   // Empty or malformed ID
    ErrInvalidFormat      // Unsupported export/import format
    ErrMaxRetriesExceeded // Retry limit reached
)

type APIError struct {
    Operation  string // e.g., "sendMessage"
    StatusCode int    // HTTP status (401, 429, etc.)
    Message    string // Human-readable description
    Err        error  // Underlying error
}
```

**Error Propagation:**
- Custom errors: Returned directly
- API errors: Wrapped in `APIError` with context
- Transient errors: Retried via exponential backoff

#### 5. Tests (`adapter_test.go`)
**Responsibility:** Comprehensive test coverage

**Test Categories:**
- **Unit Tests:** No API key required, mock responses
- **Integration Tests:** Live API calls, requires `OPENAI_API_KEY` and `INTEGRATION_TESTS=true`
- **Race Tests:** Thread safety validation via `go test -race`

**Coverage:** >90% of production code

## Data Flow

### Message Send Flow

```
User Code
   │
   ├─► adapter.SendMessage(sessionID, message)
   │       │
   │       ├─► 1. Validate session exists (RLock)
   │       │
   │       ├─► 2. Append user message (Lock)
   │       │       └─► session.Messages += message
   │       │           session.UpdatedAt = now()
   │       │
   │       ├─► 3. Build OpenAI request
   │       │       └─► toOpenAIMessages(session.Messages)
   │       │
   │       ├─► 4. Call API with retry
   │       │       └─► sendWithRetry(ctx, request)
   │       │               │
   │       │               ├─► client.CreateChatCompletion()
   │       │               │       │
   │       │               │       ├─► SUCCESS → return response
   │       │               │       │
   │       │               │       └─► ERROR
   │       │               │           ├─► 429 (rate limit) → retry with backoff
   │       │               │           ├─► 401 (auth) → return error immediately
   │       │               │           └─► other → return error
   │       │               │
   │       │               └─► Max retries → ErrMaxRetriesExceeded
   │       │
   │       └─► 5. Store assistant response (Lock)
   │               └─► session.Messages += fromOpenAIMessage(response)
   │                   session.UpdatedAt = now()
   │
   └─► return nil (success) or error
```

### Session Creation Flow

```
User Code
   │
   ├─► adapter.CreateSession(ctx)
   │       │
   │       ├─► 1. Validate context
   │       │       ├─► ctx.Name != "" ?
   │       │       └─► ctx.WorkingDirectory != "" ?
   │       │
   │       ├─► 2. Generate UUID
   │       │       └─► sessionID = uuid.New().String()
   │       │
   │       ├─► 3. Create session object
   │       │       └─► session = &Session{
   │       │               ID: sessionID,
   │       │               Context: ctx,
   │       │               Messages: [],
   │       │               Status: StatusActive,
   │       │               CreatedAt: now(),
   │       │               UpdatedAt: now(),
   │       │           }
   │       │
   │       ├─► 4. Store in map (Lock)
   │       │       └─► adapter.sessions[sessionID] = session
   │       │
   │       └─► return sessionID, nil
   │
   └─► Use sessionID for subsequent operations
```

### Export/Import Flow

```
Export:
    adapter.ExportConversation(sessionID, format)
        │
        ├─► GetHistory(sessionID) → []Message
        │
        ├─► switch format:
        │       ├─► FormatJSONL → exportJSONL(messages)
        │       │       └─► json.Marshal each message + "\n"
        │       │
        │       ├─► FormatMarkdown → exportMarkdown(messages)
        │       │       └─► "## {role}\n\n{content}\n\n"
        │       │
        │       └─► other → ErrInvalidFormat
        │
        └─► return []byte

Import:
    adapter.ImportConversation(data, format)
        │
        ├─► format == FormatJSONL? else error
        │
        ├─► parseJSONL(data) → []Message
        │       └─► bufio.Scanner + json.Unmarshal per line
        │
        ├─► CreateSession(default context)
        │       └─► sessionID = new UUID
        │
        ├─► Store messages (Lock)
        │       └─► adapter.sessions[sessionID].Messages = messages
        │
        └─► return sessionID, nil
```

## Concurrency Model

### Thread Safety Strategy

**Problem:** Multiple goroutines may access sessions concurrently

**Solution:** Read-Write Mutex (`sync.RWMutex`)

```go
type Adapter struct {
    mu       sync.RWMutex
    sessions map[agent.SessionID]*Session
    // ...
}
```

**Locking Rules:**

| Operation | Lock Type | Reason |
|-----------|-----------|--------|
| `CreateSession` | Write (`Lock`) | Modifies map |
| `TerminateSession` | Write (`Lock`) | Modifies map |
| `SendMessage` | Write (`Lock`) | Modifies session.Messages |
| `GetHistory` | Read (`RLock`) | Reads session.Messages |
| `GetSessionStatus` | Read (`RLock`) | Reads map existence |
| `ResumeSession` | Read (`RLock`) | Reads map existence |
| `ExportConversation` | Read (`RLock`) | Calls GetHistory (RLock) |
| `ExecuteCommand` | Write (`Lock`) | Modifies session.Context |

**Critical Sections:**
```go
// Read example (GetHistory)
a.mu.RLock()
session, exists := a.sessions[sessionID]
a.mu.RUnlock()

// Write example (SendMessage)
a.mu.Lock()
session.Messages = append(session.Messages, message)
session.UpdatedAt = time.Now()
a.mu.Unlock()
```

**Design Decisions:**
- Fine-grained locking (lock/unlock around minimal critical sections)
- Copy on read (GetHistory returns copy to prevent external modification)
- Defer unlocks for exception safety

## Integration Points

### OpenAI API Integration

**SDK:** `github.com/sashabaranov/go-openai`

**Client Configuration:**
```go
client := openai.NewClient(apiKey) // from OPENAI_API_KEY env var
```

**Request Format:**
```go
req := openai.ChatCompletionRequest{
    Model:    "gpt-4o",
    Messages: []openai.ChatCompletionMessage{
        {Role: "user", Content: "Hello"},
    },
}
```

**Response Format:**
```go
resp := openai.ChatCompletionResponse{
    Choices: []openai.ChatCompletionChoice{
        {
            Message: openai.ChatCompletionMessage{
                Role:    "assistant",
                Content: "Hi! How can I help?",
            },
        },
    },
}
```

**Timeout:** 30 seconds per request
```go
ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()
```

### Agent Registry Integration

**Registration:**
```go
func init() {
    adapter, _ := NewAdapter()
    if adapter != nil {
        agent.Register("gpt", adapter)
    }
}
```

**Factory Access:**
```go
gptAgent, err := agent.Get("gpt")
if err != nil {
    log.Fatal(err) // "gpt" not registered
}
```

## Error Handling Architecture

### Retry Strategy

**Exponential Backoff Algorithm:**
```
Attempt 1: Immediate
Attempt 2: 1 second delay
Attempt 3: 2 second delay
Attempt 4: 4 second delay
Attempt 5: 8 second delay
Attempt 6: 16 second delay
Max: ErrMaxRetriesExceeded
```

**Implementation:**
```go
for attempt := 0; attempt < maxRetries; attempt++ {
    resp, err := a.client.CreateChatCompletion(ctx, req)
    if err == nil {
        return resp, nil
    }

    var apiErr *openai.APIError
    if errors.As(err, &apiErr) && apiErr.HTTPStatusCode == 429 {
        delay := baseDelay * time.Duration(1 << attempt) // exponential
        time.Sleep(delay)
        continue
    }

    return openai.ChatCompletionResponse{}, err // non-retryable
}
```

**Retryable Errors:**
- 429 (Rate Limit Exceeded)
- Network timeouts (if within 30s total timeout)

**Non-Retryable Errors:**
- 401 (Unauthorized)
- 400 (Bad Request)
- Context timeout

### Error Context Enrichment

**Before:**
```
error: invalid_request_error
```

**After:**
```
APIError{
    Operation:  "sendMessage",
    StatusCode: 401,
    Message:    "authentication failed",
    Err:        <original error>,
}
```

## Storage Architecture

### In-Memory Storage (V1)

**Data Structure:**
```go
sessions: map[agent.SessionID]*Session
```

**Characteristics:**
- **Lifetime:** Process lifetime only
- **Capacity:** Limited by available RAM
- **Performance:** O(1) lookups
- **Persistence:** None

**Trade-offs:**

| Aspect | In-Memory | File-Based (V2) |
|--------|-----------|-----------------|
| Speed | ⚡ Fast | 🐢 Slow |
| Persistence | ❌ Lost on restart | ✅ Survives restarts |
| Scalability | ⚠️ RAM-limited | ✅ Disk-limited |
| Complexity | ✅ Simple | ⚠️ Complex |

**V1 Rationale:** Simplicity for initial implementation. File persistence deferred to V2.

## Configuration

### Environment Variables

| Variable | Required | Default | Purpose |
|----------|----------|---------|---------|
| `OPENAI_API_KEY` | Yes | None | API authentication |
| `INTEGRATION_TESTS` | No | `false` | Enable integration tests |

### Model Configuration

**Current:** Hardcoded `gpt-4o`
```go
model: openai.GPT4o
```

**V2 Enhancement:** Make configurable via:
- Environment variable: `OPENAI_MODEL`
- Constructor parameter: `NewAdapterWithModel(model string)`

## Security Architecture

### API Key Protection

**Storage:**
- ✅ Environment variable only
- ❌ Never in config files
- ❌ Never in git
- ❌ Never in logs

**Access Control:**
```go
apiKey := os.Getenv("OPENAI_API_KEY") // Read once at startup
if apiKey == "" {
    return nil, ErrAPIKeyNotSet // Fail fast
}
```

**Error Sanitization:**
```go
// BAD: Leaks API key
return fmt.Errorf("auth failed with key %s", apiKey)

// GOOD: No key exposure
return &APIError{
    Operation:  "sendMessage",
    StatusCode: 401,
    Message:    "authentication failed",
}
```

### Input Validation

**Session Creation:**
```go
if ctx.Name == "" {
    return "", errors.New("session name required")
}
if ctx.WorkingDirectory == "" {
    return "", errors.New("working directory required")
}
```

**Session Access:**
```go
if _, exists := a.sessions[sessionID]; !exists {
    return ErrSessionNotFound
}
```

## Testing Architecture

### Test Pyramid

```
        /\
       /  \  Integration Tests (few, slow, realistic)
      /____\
     /      \
    / Unit   \ Unit Tests (many, fast, isolated)
   /  Tests   \
  /__________\
```

### Test Structure

**Unit Tests:**
- No external dependencies
- Mock OpenAI client (future enhancement)
- Test individual functions
- Fast (<1 second total)

**Integration Tests:**
- Real OpenAI API calls
- Requires valid API key
- Test end-to-end flows
- Slow (3-10 seconds per test)
- Optional (gated by `INTEGRATION_TESTS=true`)

**Race Tests:**
```bash
go test -race
```
- Detects concurrent access bugs
- Validates mutex usage
- Run on every commit

## Deployment

### Build Process
```bash
cd ~/src/ws/oss/repos/ai-tools/main/claude-session-manager
go build ./internal/agent/gpt
```

### Runtime Requirements
- Go 1.24+
- Environment variable: `OPENAI_API_KEY`
- Network access to `api.openai.com`

### Monitoring
- No built-in metrics (V1)
- V2: Token usage tracking, cost estimation

## Future Architecture (V2)

### Planned Enhancements

1. **File-Based Persistence**
   ```
   ~/.csm/sessions/gpt/{session-id}.jsonl
   ```

2. **Streaming Support**
   ```go
   stream, err := adapter.SendMessageStream(sessionID, message)
   for {
       chunk := <-stream
       fmt.Print(chunk.Delta)
   }
   ```

3. **Tool Calling**
   ```go
   capabilities.SupportsTools = true
   message.Metadata["tool_calls"] = [...]
   ```

4. **Vision Input**
   ```go
   message.Content = []ContentPart{
       {Type: "text", Text: "Describe this image"},
       {Type: "image_url", ImageURL: "https://..."},
   }
   ```

5. **System Prompts**
   ```go
   ctx.SystemPrompt = "You are a helpful coding assistant."
   ```

## References

- [Agent Interface Specification](../interface.go)
- [GPT Adapter Technical Spec](SPEC.md)
- [OpenAI API Reference](https://platform.openai.com/docs/api-reference/chat)
- [Go Concurrency Patterns](https://go.dev/blog/pipelines)
