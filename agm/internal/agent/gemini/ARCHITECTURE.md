# Gemini Adapter Architecture

**Version:** 1.0
**Last Updated:** 2026-02-11
**Status:** Implemented

## System Overview

The Gemini Adapter is a component within the Claude Session Manager (AGM) agent ecosystem that provides Google Gemini 2.0 integration. It implements the unified `agent.Agent` interface, enabling Gemini to be used interchangeably with Claude and GPT agents.

### Architecture Diagram

```
┌─────────────────────────────────────────────────────────────┐
│                 AGM (AI/Agent Gateway Manager)               │
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
│                           │                                   │
└───────────────────────────┼───────────────────────────────────┘
                            │
                            ▼
                 ┌────────────────────────────┐
                 │   Google Generative AI     │
                 │     API (Gemini 2.0)       │
                 └────────────────────────────┘
```

## Component Architecture

### Core Components

#### 1. GeminiAdapter (`gemini_adapter.go`)
**Responsibility:** Main entry point implementing `agent.Agent` interface

**Key Methods:**
- Session lifecycle: `CreateSession`, `ResumeSession`, `TerminateSession`
- Communication: `SendMessage`, `GetHistory`
- Serialization: `ExportConversation`, `ImportConversation`
- Commands: `ExecuteCommand`
- Metadata: `Name`, `Version`, `Capabilities`

**Dependencies:**
- `genai.Client` - Google Generative AI SDK client
- `SessionStore` - Session metadata storage
- File system - JSONL history storage

**Design Patterns:**
- **Adapter Pattern:** Translates `agent.Agent` interface to Google AI API
- **Repository Pattern:** File-based history storage with JSONL
- **Singleton:** Single Gemini client instance per adapter

#### 2. Session Storage

**Session Metadata:**
- Storage: `SessionStore` interface (typically JSON file at ~/.agm/sessions.json)
- Content: Session ID, creation time, working directory, project name
- Lifecycle: Created on session start, deleted on termination

**Conversation History:**
- Storage: JSONL file at ~/.agm/gemini/<session-id>/history.jsonl
- Format: One JSON message per line
- Lifecycle: Created on first message, preserved after termination

**File Layout:**
```
~/.agm/
├── sessions.json              # Metadata for all sessions
└── gemini/
    ├── abc-123.../
    │   └── history.jsonl      # Session abc-123 history
    └── def-456.../
        └── history.jsonl      # Session def-456 history
```

#### 3. Message Translation

**Role Mapping:**
```go
// Agent → Gemini
agent.RoleUser      → genai.Content{Role: "user"}
agent.RoleAssistant → genai.Content{Role: "model"}

// Gemini → Agent
genai.Content{Role: "user"}  → agent.Message{Role: RoleUser}
genai.Content{Role: "model"} → agent.Message{Role: RoleAssistant}
```

**Content Extraction:**
```go
// From Gemini response
for _, part := range content.Parts {
    if text, ok := part.(genai.Text); ok {
        responseText += string(text)
    }
}

// To Gemini request
genai.Content{
    Role: role,
    Parts: []genai.Part{
        genai.Text(message.Content),
    },
}
```

#### 4. Helper Functions

**Session Directory Management:**
- `getSessionDir(sessionID)` - Returns ~/.agm/gemini/<session-id>
- `getHistoryPath(sessionID)` - Returns ~/.agm/gemini/<session-id>/history.jsonl

**History Persistence:**
- `appendToHistory(sessionID, message)` - Appends message to JSONL
- `GetHistory(sessionID)` - Parses JSONL and returns messages

**Utilities:**
- `splitLines(s)` - JSONL line splitter preserving empty lines

## Data Flow

### Message Send Flow

```
User Code
   │
   ├─► adapter.SendMessage(sessionID, message)
   │       │
   │       ├─► 1. Validate session exists (check metadata store)
   │       │
   │       ├─► 2. Load full conversation history
   │       │       └─► GetHistory(sessionID)
   │       │           └─► Read ~/.agm/gemini/<id>/history.jsonl
   │       │
   │       ├─► 3. Create Gemini client
   │       │       └─► genai.NewClient(ctx, option.WithAPIKey(apiKey))
   │       │
   │       ├─► 4. Build conversation context
   │       │       └─► Convert []agent.Message → []genai.Content
   │       │
   │       ├─► 5. Start chat session with history
   │       │       └─► chat := model.StartChat()
   │       │           chat.History = geminiHistory
   │       │
   │       ├─► 6. Send message
   │       │       └─► resp, err := chat.SendMessage(ctx, genai.Text(...))
   │       │           │
   │       │           ├─► SUCCESS → extract response text
   │       │           │
   │       │           └─► ERROR → return with context
   │       │
   │       ├─► 7. Append user message to history
   │       │       └─► appendToHistory(sessionID, userMsg)
   │       │
   │       └─► 8. Append assistant response to history
   │               └─► appendToHistory(sessionID, assistantMsg)
   │
   └─► return nil (success) or error
```

### Session Creation Flow

```
User Code
   │
   ├─► adapter.CreateSession(ctx)
   │       │
   │       ├─► 1. Generate UUID
   │       │       └─► sessionID = uuid.New().String()
   │       │
   │       ├─► 2. Create session directory
   │       │       └─► sessionDir = ~/.agm/gemini/<sessionID>
   │       │           os.MkdirAll(sessionDir, 0755)
   │       │
   │       ├─► 3. Store session metadata
   │       │       └─► metadata = SessionMetadata{
   │       │               TmuxName: sessionID,
   │       │               CreatedAt: now(),
   │       │               WorkingDir: ctx.WorkingDirectory,
   │       │               Project: ctx.Project,
   │       │           }
   │       │           sessionStore.Set(sessionID, metadata)
   │       │
   │       └─► return sessionID, nil
   │
   └─► Use sessionID for subsequent operations
```

### History Loading Flow

```
GetHistory(sessionID)
   │
   ├─► 1. Get history file path
   │       └─► historyPath = ~/.agm/gemini/<sessionID>/history.jsonl
   │
   ├─► 2. Check file exists
   │       └─► If not exists → return []
   │
   ├─► 3. Read file
   │       └─► data = os.ReadFile(historyPath)
   │
   ├─► 4. Parse JSONL
   │       └─► lines = splitLines(data)
   │           for each line:
   │               var msg Message
   │               json.Unmarshal(line, &msg)
   │               messages.append(msg)
   │
   └─► return messages
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
        │       │       └─► for msg in messages:
        │       │               json.Marshal(msg) + "\n"
        │       │
        │       ├─► FormatMarkdown → exportMarkdown(messages)
        │       │       └─► "## {role} ({timestamp})\n\n{content}\n\n"
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
        │       └─► lines = splitLines(data)
        │           for line: json.Unmarshal(line, &msg)
        │
        ├─► CreateSession(default context)
        │       └─► sessionID = new UUID
        │           create session directory
        │
        ├─► Write messages to history file
        │       └─► file = os.Create(historyPath)
        │           for msg: json.Encode(msg)
        │
        └─► return sessionID, nil
```

## Storage Architecture

### File-Based Persistence

**Rationale:**
- Session history persists across process restarts
- Human-readable JSONL format
- Compatible with export/import feature
- No database dependency

**JSONL Format:**
```jsonl
{"id":"msg-1","role":"user","content":"Hello","timestamp":"2026-02-11T10:00:00Z"}
{"id":"msg-2","role":"assistant","content":"Hi!","timestamp":"2026-02-11T10:00:01Z"}
```

**Append-Only Writes:**
```go
file, err := os.OpenFile(historyPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
encoder := json.NewEncoder(file)
encoder.Encode(message)
```

**Benefits:**
- ✅ Atomic writes (single line = single message)
- ✅ No file corruption risk
- ✅ Simple implementation
- ✅ Compatible with line-based tools (grep, sed)

**Trade-offs:**

| Aspect | File-Based | In-Memory (GPT) |
|--------|------------|-----------------|
| Persistence | ✅ Survives restarts | ❌ Lost on restart |
| Speed | 🐢 I/O overhead | ⚡ Fast |
| Scalability | ✅ Disk-limited | ⚠️ RAM-limited |
| Complexity | ⚠️ File handling | ✅ Simple |

## Integration Points

### Google Generative AI API Integration

**SDK:** `github.com/google/generative-ai-go/genai`

**Client Configuration:**
```go
client, err := genai.NewClient(ctx, option.WithAPIKey(apiKey))
```

**Model Selection:**
```go
model := client.GenerativeModel("gemini-2.0-flash-exp")
```

**Chat Session:**
```go
chat := model.StartChat()
chat.History = []genai.Content{
    {Role: "user", Parts: []genai.Part{genai.Text("Hello")}},
    {Role: "model", Parts: []genai.Part{genai.Text("Hi!")}},
}
resp, err := chat.SendMessage(ctx, genai.Text("How are you?"))
```

**Response Format:**
```go
resp := genai.GenerateContentResponse{
    Candidates: []genai.Candidate{
        {
            Content: &genai.Content{
                Role: "model",
                Parts: []genai.Part{
                    genai.Text("I'm doing well!"),
                },
            },
        },
    },
}
```

### Session Store Integration

**SessionStore Interface:**
```go
type SessionStore interface {
    Set(sessionID SessionID, metadata *SessionMetadata) error
    Get(sessionID SessionID) (*SessionMetadata, error)
    Delete(sessionID SessionID) error
}
```

**Default Implementation:**
- JSON file at ~/.agm/sessions.json
- Simple key-value store
- Shared across all agents (Claude, Gemini, GPT)

### Agent Registry Integration

**Auto-Registration:**
```go
// Gemini adapter registers on import
func init() {
    adapter, _ := NewGeminiAdapter(nil)
    if adapter != nil {
        agent.Register("gemini", adapter)
    }
}
```

**Factory Access:**
```go
geminiAgent, err := agent.Get("gemini")
if err != nil {
    log.Fatal(err) // "gemini" not registered
}
```

## Error Handling Architecture

### Error Types

**Configuration Errors:**
- `GEMINI_API_KEY not set` - Missing API key
- Session store initialization failure

**Session Errors:**
- Session not found in store
- Session directory not found
- Session already exists

**API Errors:**
- Authentication failure (401)
- Network errors
- Rate limiting (429)
- Invalid request (400)

**File I/O Errors:**
- Cannot create session directory
- Cannot write history file
- Cannot read history file
- Malformed JSONL

### Error Context Enrichment

**Before:**
```
error: failed to send message
```

**After:**
```
failed to send message to Gemini: authentication failed
session: abc-123
model: gemini-2.0-flash-exp
```

### No Retry Logic (V1)

**Current Behavior:**
- API errors return immediately
- No exponential backoff
- No retry attempts

**V2 Enhancement:**
```go
for attempt := 0; attempt < maxRetries; attempt++ {
    resp, err := chat.SendMessage(ctx, msg)
    if err == nil {
        return resp, nil
    }
    if isRetryable(err) {
        time.Sleep(backoff(attempt))
        continue
    }
    return nil, err
}
```

## Configuration

### Environment Variables

| Variable | Required | Default | Purpose |
|----------|----------|---------|---------|
| `GEMINI_API_KEY` | Yes | None | Google AI API authentication |

### Model Configuration

**Current:** Hardcoded `gemini-2.0-flash-exp`
```go
modelName: "gemini-2.0-flash-exp"
```

**Alternative Models:**
- `gemini-2.0-flash-thinking-exp` - 2M context, thinking mode
- `gemini-1.5-pro` - 2M context, previous generation
- `gemini-1.5-flash` - 1M context, faster

**V2 Enhancement:** Make configurable via:
- Environment variable: `GEMINI_MODEL`
- Constructor parameter: `NewGeminiAdapter(&GeminiConfig{ModelName: "..."})`

## Security Architecture

### API Key Protection

**Storage:**
- ✅ Environment variable only
- ❌ Never in config files
- ❌ Never in git
- ❌ Never in logs

**Access Control:**
```go
apiKey := config.APIKey
if apiKey == "" {
    apiKey = os.Getenv("GEMINI_API_KEY")
}
if apiKey == "" {
    return nil, fmt.Errorf("GEMINI_API_KEY not set")
}
```

**Error Sanitization:**
```go
// BAD: Leaks API key
return fmt.Errorf("auth failed with key %s", apiKey)

// GOOD: No key exposure
return fmt.Errorf("failed to create Gemini client: authentication failed")
```

### File Permissions

**Session Directories:**
- Permissions: 0755 (rwxr-xr-x)
- Owner: Current user
- World-readable but not writable

**History Files:**
- Permissions: 0644 (rw-r--r--)
- Owner: Current user
- World-readable but not writable

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
if _, err := a.sessionStore.Get(sessionID); err != nil {
    return fmt.Errorf("session not found: %w", err)
}
```

## Testing Architecture

### Test Structure

**Unit Tests:**
- No external dependencies
- No API key required
- Mock session store
- Temporary directories for file I/O
- Fast (<1 second total)

**Integration Tests:**
- Real Gemini API calls
- Requires valid API key
- Test end-to-end flows
- Optional (documented)

**Test Coverage:**
- >90% code coverage
- All error paths tested
- File I/O edge cases covered

### Mock Components

**MockSessionStore:**
```go
type MockSessionStore struct {
    sessions map[SessionID]*SessionMetadata
}

func (m *MockSessionStore) Set(id SessionID, meta *SessionMetadata) error {
    m.sessions[id] = meta
    return nil
}
```

**Temporary Directories:**
```go
tmpDir := t.TempDir()
originalHome := os.Getenv("HOME")
os.Setenv("HOME", tmpDir)
defer os.Setenv("HOME", originalHome)
```

## Deployment

### Build Process
```bash
cd main/agm
go build ./internal/agent
```

### Runtime Requirements
- Go 1.24+
- Environment variable: `GEMINI_API_KEY`
- Network access to `generativelanguage.googleapis.com`
- Write access to ~/.agm/ directory

## Future Architecture (V2)

### Planned Enhancements

1. **Streaming Support**
   ```go
   stream := chat.SendMessageStream(ctx, msg)
   for {
       resp, err := stream.Recv()
       if err == io.EOF {
           break
       }
       fmt.Print(resp.Text())
   }
   ```

2. **Function Calling**
   ```go
   model.Tools = []*genai.Tool{
       {FunctionDeclarations: []*genai.FunctionDeclaration{...}},
   }
   ```

3. **Vision Input**
   ```go
   content := genai.Content{
       Parts: []genai.Part{
           genai.Text("Describe this image"),
           genai.ImageData("jpeg", imageBytes),
       },
   }
   ```

4. **System Instructions**
   ```go
   model.SystemInstruction = &genai.Content{
       Parts: []genai.Part{genai.Text("You are a helpful assistant.")},
   }
   ```

5. **Retry Logic**
   - Exponential backoff for rate limits
   - Configurable retry attempts
   - Circuit breaker pattern

## References

- [Agent Interface Specification](../interface.go)
- [Gemini Adapter Technical Spec](SPEC.md)
- [Google Generative AI API Reference](https://ai.google.dev/api/rest)
- [Gemini Go SDK Documentation](https://pkg.go.dev/github.com/google/generative-ai-go/genai)
