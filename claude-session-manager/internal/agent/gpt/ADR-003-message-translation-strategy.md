# ADR-003: Message Translation Strategy (Agent ↔ OpenAI)

**Status:** Accepted
**Date:** 2026-02-11
**Deciders:** GPT Adapter Development Team
**Context:** V1 Implementation

## Context and Problem Statement

The GPT Adapter must translate between two different message formats:
1. **Agent Format:** `agent.Message` (CSM unified interface)
2. **OpenAI Format:** `openai.ChatCompletionMessage` (API-specific)

The adapter needs a strategy for:
- Converting between formats (bidirectional translation)
- Preserving message content and metadata
- Handling format differences (IDs, timestamps, roles)
- Maintaining conversation integrity

**Key Questions:**
- Should translation be stateless or stateful?
- How do we handle fields present in one format but not the other?
- Should we validate messages during translation?
- Where should translation logic live (adapter.go vs. dedicated file)?

## Decision Drivers

- **Data Integrity:** Messages must not be corrupted during translation
- **Simplicity:** Translation logic should be easy to understand and maintain
- **Testability:** Translation should be testable in isolation
- **Performance:** Translation happens on every message send/receive
- **Extensibility:** Easy to add new fields or validation in the future

## Considered Options

### Option 1: Dedicated Translator Functions (Chosen)
**Structure:**
```go
// translator.go
func toOpenAIMessage(msg agent.Message) openai.ChatCompletionMessage
func fromOpenAIMessage(msg openai.ChatCompletionMessage, model string) agent.Message
func toOpenAIMessages(messages []agent.Message) []openai.ChatCompletionMessage
```

**Pros:**
- ✅ Clear separation of concerns (translation isolated)
- ✅ Easy to test (pure functions)
- ✅ Reusable across adapter methods
- ✅ Stateless (no side effects)
- ✅ Simple to extend (add new translation functions)

**Cons:**
- ⚠️ Additional file (translator.go)

### Option 2: Inline Translation in Adapter Methods
**Structure:**
```go
// adapter.go
func (a *Adapter) SendMessage(...) {
    // Inline translation
    openaiMsg := openai.ChatCompletionMessage{
        Role:    convertRole(msg.Role),
        Content: msg.Content,
    }
    // ...
}
```

**Pros:**
- ✅ No additional file
- ✅ Translation logic near usage

**Cons:**
- ❌ Code duplication (translation logic repeated)
- ❌ Harder to test (coupled with adapter logic)
- ❌ Violates DRY principle
- ❌ Harder to maintain (scattered logic)

### Option 3: Method-Based Translation (Struct Methods)
**Structure:**
```go
func (m agent.Message) ToOpenAI() openai.ChatCompletionMessage
func (m openai.ChatCompletionMessage) ToAgent() agent.Message
```

**Pros:**
- ✅ Object-oriented style
- ✅ Easy to discover (method on struct)

**Cons:**
- ❌ Cannot add methods to external types (openai.ChatCompletionMessage)
- ❌ Cannot add methods to agent.Message (not in gpt package)
- ❌ Would require wrapper types (complexity)

## Decision Outcome

**Chosen Option:** **Option 1 - Dedicated Translator Functions**

**Rationale:**
1. **Separation of Concerns:** Translation logic isolated in `translator.go`
2. **Pure Functions:** Stateless, testable, no side effects
3. **Reusability:** Single source of truth for translation
4. **Maintainability:** Easy to locate and modify translation logic
5. **Testability:** Can test translation independently of adapter

**File Structure:**
```
gpt/
├── adapter.go      # Uses translator functions
├── translator.go   # Translation logic only
├── session.go      # Data structures
├── errors.go       # Error definitions
└── adapter_test.go # Tests both adapter and translator
```

## Implementation Details

### Translation Functions

#### Agent → OpenAI
```go
func toOpenAIMessage(msg agent.Message) openai.ChatCompletionMessage {
    role := openai.ChatMessageRoleUser
    if msg.Role == agent.RoleAssistant {
        role = openai.ChatMessageRoleAssistant
    }
    return openai.ChatCompletionMessage{
        Role:    role,
        Content: msg.Content,
    }
}
```

**Design Decisions:**
- Default to `ChatMessageRoleUser` (safest assumption)
- Only translate `Role` and `Content` (other fields ignored in V1)
- No validation (assume agent.Message is already valid)

#### OpenAI → Agent
```go
func fromOpenAIMessage(msg openai.ChatCompletionMessage, model string) agent.Message {
    role := agent.RoleAssistant
    if msg.Role == openai.ChatMessageRoleUser {
        role = agent.RoleUser
    }
    return agent.Message{
        ID:        uuid.New().String(), // Generate UUID
        Role:      role,
        Content:   msg.Content,
        Timestamp: time.Now(),          // Record current time
        Metadata: map[string]interface{}{
            "model": model,              // Store model name
        },
    }
}
```

**Design Decisions:**
- Generate UUID (OpenAI messages don't have IDs)
- Use current timestamp (OpenAI doesn't provide timestamps)
- Store model in metadata (for debugging/auditing)
- Default to `RoleAssistant` (OpenAI responses are from assistant)

#### Batch Translation
```go
func toOpenAIMessages(messages []agent.Message) []openai.ChatCompletionMessage {
    result := make([]openai.ChatCompletionMessage, len(messages))
    for i, msg := range messages {
        result[i] = toOpenAIMessage(msg)
    }
    return result
}
```

**Design Decisions:**
- Preallocate slice (performance optimization)
- Preserve message order (critical for conversation context)
- No filtering (translate all messages)

### Field Mapping

| Agent Field | OpenAI Field | Translation |
|-------------|--------------|-------------|
| `Role` | `Role` | Direct mapping: `user` ↔ `user`, `assistant` ↔ `assistant` |
| `Content` | `Content` | Passthrough (no transformation) |
| `ID` | N/A | Generated UUID on OpenAI→Agent |
| `Timestamp` | N/A | Current time on OpenAI→Agent |
| `Metadata["model"]` | N/A | Model name stored on OpenAI→Agent |
| N/A | `Name` | Ignored (V1 doesn't use function calling) |
| N/A | `FunctionCall` | Ignored (V1 doesn't use function calling) |

**Ignored OpenAI Fields (V1):**
- `Name` (used for function calling)
- `FunctionCall` (used for tool calls)

**Ignored Agent Fields (V1):**
- `Metadata` (not sent to OpenAI, preserved in history)

### Role Translation

```go
// Agent → OpenAI
agent.RoleUser      → openai.ChatMessageRoleUser
agent.RoleAssistant → openai.ChatMessageRoleAssistant

// OpenAI → Agent
openai.ChatMessageRoleUser      → agent.RoleUser
openai.ChatMessageRoleAssistant → agent.RoleAssistant
openai.ChatMessageRoleSystem    → agent.RoleAssistant (V1: treat as assistant)
openai.ChatMessageRoleFunction  → agent.RoleAssistant (V1: not supported)
```

**V1 Simplification:** Only support `user` and `assistant` roles
**V2 Enhancement:** Support `system` role for system prompts

## Consequences

### Positive
- ✅ Clear separation: Translation logic isolated from adapter logic
- ✅ Testable: Pure functions, easy to unit test
- ✅ Reusable: Single source of truth for translation
- ✅ Maintainable: Easy to extend (add new fields, validation)
- ✅ Performance: Minimal overhead (simple field mapping)

### Negative
- ⚠️ Additional file (translator.go) - minor overhead
- ⚠️ Ignored fields in V1 (function calls, system messages)

### Neutral
- ⚠️ UUID/timestamp generation on every OpenAI→Agent translation
- ⚠️ Model name stored in metadata (small memory overhead)

## Data Integrity Guarantees

### Content Preservation
```go
// GUARANTEED: Content never modified
msg.Content == toOpenAIMessage(msg).Content
```

### Order Preservation
```go
// GUARANTEED: Message order maintained
messages := []agent.Message{msg1, msg2, msg3}
openaiMessages := toOpenAIMessages(messages)
// openaiMessages[0] corresponds to msg1, etc.
```

### Round-Trip Consistency (Lossy)
```go
// NOT GUARANTEED: Round-trip loses ID, Timestamp, Metadata
original := agent.Message{ID: "123", Content: "Hello", ...}
openai := toOpenAIMessage(original)
back := fromOpenAIMessage(openai, "gpt-4o")
// back.ID != original.ID (new UUID generated)
// back.Timestamp != original.Timestamp (new timestamp)
// back.Metadata != original.Metadata (new metadata)

// GUARANTEED: Content preserved
// back.Content == original.Content
```

**Why Lossy:** OpenAI API doesn't preserve IDs/timestamps, so round-trip cannot recover them

## Validation Strategy

### No Validation in Translator (V1 Decision)
```go
// Translator does NOT validate
func toOpenAIMessage(msg agent.Message) openai.ChatCompletionMessage {
    // No check: if msg.Content == "" { panic() }
    // No check: if msg.Role == "" { panic() }
    // Assumes: msg is already valid
}
```

**Rationale:**
- Validation is adapter's responsibility (before calling translator)
- Translator is a pure transformation (no side effects, no errors)
- Simplifies translator testing (no error handling needed)

**Validation Location:**
```go
// adapter.go
func (a *Adapter) SendMessage(sessionID agent.SessionID, message agent.Message) error {
    // Validate BEFORE translation
    if message.Content == "" {
        return errors.New("message content required")
    }

    // Translate (assumes valid input)
    openaiMsg := toOpenAIMessage(message)
    // ...
}
```

**V2 Enhancement:** Add optional validation in translator if needed

## Testing Strategy

### Unit Tests (translator.go)
```go
func TestToOpenAIMessage(t *testing.T) {
    msg := agent.Message{
        Role:    agent.RoleUser,
        Content: "Hello",
    }
    openaiMsg := toOpenAIMessage(msg)
    assert.Equal(t, openai.ChatMessageRoleUser, openaiMsg.Role)
    assert.Equal(t, "Hello", openaiMsg.Content)
}

func TestFromOpenAIMessage(t *testing.T) {
    openaiMsg := openai.ChatCompletionMessage{
        Role:    openai.ChatMessageRoleAssistant,
        Content: "Hi!",
    }
    msg := fromOpenAIMessage(openaiMsg, "gpt-4o")
    assert.Equal(t, agent.RoleAssistant, msg.Role)
    assert.Equal(t, "Hi!", msg.Content)
    assert.NotEmpty(t, msg.ID) // UUID generated
    assert.NotZero(t, msg.Timestamp) // Timestamp set
    assert.Equal(t, "gpt-4o", msg.Metadata["model"])
}

func TestToOpenAIMessages_PreservesOrder(t *testing.T) {
    messages := []agent.Message{
        {Content: "First"},
        {Content: "Second"},
        {Content: "Third"},
    }
    openaiMessages := toOpenAIMessages(messages)
    assert.Len(t, openaiMessages, 3)
    assert.Equal(t, "First", openaiMessages[0].Content)
    assert.Equal(t, "Second", openaiMessages[1].Content)
    assert.Equal(t, "Third", openaiMessages[2].Content)
}
```

### Integration Tests (adapter.go)
```go
func TestSendMessage_TranslationE2E(t *testing.T) {
    // Verify translation works in full send/receive flow
    adapter := createTestAdapter()
    sessionID, _ := adapter.CreateSession(testContext)

    // Send user message
    adapter.SendMessage(sessionID, agent.Message{
        Role:    agent.RoleUser,
        Content: "Test message",
    })

    // Get history (includes translated assistant response)
    history, _ := adapter.GetHistory(sessionID)
    assert.Len(t, history, 2) // User + Assistant
    assert.Equal(t, agent.RoleUser, history[0].Role)
    assert.Equal(t, agent.RoleAssistant, history[1].Role)
}
```

## Future Enhancements (V2)

### System Prompt Support
```go
func toOpenAIMessage(msg agent.Message) openai.ChatCompletionMessage {
    role := openai.ChatMessageRoleUser
    if msg.Role == agent.RoleAssistant {
        role = openai.ChatMessageRoleAssistant
    }
    if msg.Role == agent.RoleSystem { // V2: New role
        role = openai.ChatMessageRoleSystem
    }
    return openai.ChatCompletionMessage{Role: role, Content: msg.Content}
}
```

### Function Calling Support
```go
func toOpenAIMessage(msg agent.Message) openai.ChatCompletionMessage {
    openaiMsg := openai.ChatCompletionMessage{
        Role:    convertRole(msg.Role),
        Content: msg.Content,
    }

    // V2: Translate tool calls from metadata
    if toolCalls, ok := msg.Metadata["tool_calls"].([]ToolCall); ok {
        openaiMsg.FunctionCall = convertToolCalls(toolCalls)
    }

    return openaiMsg
}
```

### Vision Support (Images)
```go
func toOpenAIMessage(msg agent.Message) openai.ChatCompletionMessage {
    // V2: Support multi-part content (text + images)
    if images, ok := msg.Metadata["images"].([]string); ok {
        content := []openai.ChatMessagePart{
            {Type: "text", Text: msg.Content},
        }
        for _, imageURL := range images {
            content = append(content, openai.ChatMessagePart{
                Type:     "image_url",
                ImageURL: imageURL,
            })
        }
        return openai.ChatCompletionMessage{
            Role:    convertRole(msg.Role),
            Content: content, // Multi-part content
        }
    }

    // Fallback: Text-only
    return openai.ChatCompletionMessage{Role: convertRole(msg.Role), Content: msg.Content}
}
```

## References

- [Agent Interface](../interface.go) - `agent.Message` definition
- [OpenAI SDK](https://pkg.go.dev/github.com/sashabaranov/go-openai) - `ChatCompletionMessage`
- [ADR-001](ADR-001-in-memory-storage.md) - Storage decision
- [ADR-002](ADR-002-exponential-backoff.md) - Error handling
- [SPEC.md](SPEC.md) - Message translation requirements

## Notes

- Pure function approach enables easy testing and refactoring
- Lossy round-trip is acceptable (OpenAI API doesn't preserve IDs/timestamps)
- V1 focuses on text-only messages (images/tools deferred to V2)
- Translator functions are package-private (not exported) - internal detail
