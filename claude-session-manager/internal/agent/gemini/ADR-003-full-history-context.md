# ADR-003: Full History Context on Every API Call

**Status:** Accepted
**Date:** 2026-02-11
**Deciders:** Gemini Adapter Development Team
**Context:** V1 Implementation

## Context and Problem Statement

The Gemini API is stateless - each API call requires the full conversation context. We must decide how to manage conversation history when sending messages to Gemini.

**Key Requirements:**
- Maintain conversation continuity
- Preserve context across multiple turns
- Handle long conversations
- Minimize API overhead

**Constraints:**
- Gemini API doesn't store conversation state server-side
- Each API call is independent
- Context window: 1M tokens (2M for some models)
- Client responsible for history management

## Decision Drivers

- **API Design:** Gemini API requires client-side history
- **Conversation Quality:** Full context improves responses
- **Simplicity:** Avoid complex state management
- **Correctness:** Ensure assistant "remembers" previous turns
- **SDK Support:** Leverage `chat.History` feature

## Considered Options

### Option 1: Full History on Every Call (Chosen)
**Implementation:**
```go
// Load entire conversation history
history, err := a.GetHistory(sessionID)

// Convert to Gemini format
var geminiHistory []*genai.Content
for _, msg := range history {
    role := "user"
    if msg.Role == RoleAssistant {
        role = "model"
    }
    geminiHistory = append(geminiHistory, &genai.Content{
        Role:  role,
        Parts: []genai.Part{genai.Text(msg.Content)},
    })
}

// Start chat with full history
chat := model.StartChat()
chat.History = geminiHistory

// Send new message
resp, err := chat.SendMessage(ctx, genai.Text(message.Content))
```

**Pros:**
- ✅ Simple implementation
- ✅ Full conversation context maintained
- ✅ No context loss across turns
- ✅ Leverages SDK's built-in chat session
- ✅ Correctness guaranteed

**Cons:**
- ⚠️ O(n) history loading on every message
- ⚠️ Memory usage grows with conversation length
- ⚠️ API latency increases with history size
- ⚠️ No handling for context window overflow

### Option 2: Sliding Window Context
**Implementation:**
```go
// Load last N messages only
const maxHistoryMessages = 20

history, err := a.GetHistory(sessionID)
startIdx := max(0, len(history) - maxHistoryMessages)
recentHistory := history[startIdx:]

// Use truncated history
geminiHistory := convertToGemini(recentHistory)
```

**Pros:**
- ✅ Bounded memory usage
- ✅ Faster API calls
- ✅ Avoids context window overflow

**Cons:**
- ❌ Context loss for older messages
- ❌ Assistant forgets earlier conversation
- ❌ Complexity in window management
- ❌ Arbitrary cutoff may lose important context

### Option 3: Summarization-Based Context
**Implementation:**
```go
// Summarize old messages, keep recent ones full
if len(history) > threshold {
    summary := summarizeOldMessages(history[:threshold])
    recentHistory := history[threshold:]
    geminiHistory = [summary] + convertToGemini(recentHistory)
}
```

**Pros:**
- ✅ Preserves important context
- ✅ Bounded context window usage
- ✅ Scalable for long conversations

**Cons:**
- ❌ Complex implementation
- ❌ Additional API calls for summarization
- ❌ Information loss in summary
- ❌ Cost of summarization
- ❌ Latency overhead

### Option 4: Stateful Server-Side Context (Not Possible)
**Implementation:** Rely on Gemini API to maintain state

**Cons:**
- ❌ Gemini API doesn't support this
- ❌ Not available in current API design

## Decision Outcome

**Chosen Option:** **Option 1 - Full History on Every Call**

**Rationale:**
1. **API Requirement:** Gemini API requires client-side history
2. **SDK Design:** `chat.History` expects full conversation
3. **Simplicity:** V1 prioritizes correctness over optimization
4. **Conversation Quality:** Full context produces best responses
5. **Defer Optimization:** Handle long conversations in V2
6. **User Expectations:** Users expect assistant to remember everything

**Key Assumptions:**
- Most conversations stay under 100 messages
- Context window (1M tokens) sufficient for typical usage
- Performance acceptable for V1 development/testing
- Long conversation handling deferred to V2

**Trade-offs Accepted:**
- O(n) memory usage per message send (acceptable for typical conversations)
- Full history loaded from JSONL file (acceptable I/O overhead)
- No context window management (V1 limitation, documented)
- API call size grows with conversation length (acceptable within limits)

## Implementation Details

### History Loading
```go
func (a *GeminiAdapter) SendMessage(sessionID SessionID, message Message) error {
    // 1. Load full conversation history
    history, err := a.GetHistory(sessionID)
    if err != nil {
        return fmt.Errorf("failed to load history: %w", err)
    }

    // 2. Create client
    ctx := context.Background()
    client, err := genai.NewClient(ctx, option.WithAPIKey(a.apiKey))
    if err != nil {
        return fmt.Errorf("failed to create Gemini client: %w", err)
    }
    defer client.Close()

    // 3. Get model
    model := client.GenerativeModel(a.modelName)

    // 4. Convert history to Gemini format
    var geminiHistory []*genai.Content
    for _, msg := range history {
        role := "user"
        if msg.Role == RoleAssistant {
            role = "model"
        }
        geminiHistory = append(geminiHistory, &genai.Content{
            Role:  role,
            Parts: []genai.Part{genai.Text(msg.Content)},
        })
    }

    // 5. Start chat with full history
    chat := model.StartChat()
    chat.History = geminiHistory

    // 6. Send message
    resp, err := chat.SendMessage(ctx, genai.Text(message.Content))
    if err != nil {
        return fmt.Errorf("failed to send message to Gemini: %w", err)
    }

    // 7-8. Append user message and response to history
    // (implementation details omitted)
}
```

### Performance Characteristics

**Time Complexity:**
- History loading: O(n) where n = number of messages
- History conversion: O(n)
- API call: O(n) (larger request with full history)
- Total: O(n)

**Space Complexity:**
- History array: O(n)
- Gemini history: O(n)
- Total: O(n)

**Example Performance (Typical Conversation):**
```
10 messages:  <100ms file read, ~1s API call
50 messages:  ~200ms file read, ~2s API call
100 messages: ~500ms file read, ~3s API call
500 messages: ~2s file read, ~10s API call (approaching limits)
```

## Consequences

### Positive
- ✅ Simple, maintainable implementation
- ✅ Full conversation context preserved
- ✅ No conversation continuity issues
- ✅ Leverages SDK chat session feature
- ✅ Correct behavior guaranteed

### Negative
- ⚠️ Performance degrades with conversation length
- ⚠️ No handling for context window overflow
- ⚠️ Memory usage grows linearly
- ⚠️ File I/O on every message send

### Neutral
- ℹ️ Acceptable for V1 (development/testing)
- ℹ️ Optimization deferred to V2
- ℹ️ User responsible for keeping conversations manageable
- ℹ️ Documented limitation in SPEC.md

## Validation

### Success Metrics
- [x] Assistant remembers previous turns
- [x] Conversations maintain context
- [x] No conversation continuity bugs
- [x] Tests pass with multi-turn conversations

### Known Limitations (V1)
- ⚠️ No automatic truncation at context window limit
- ⚠️ No handling for 1M/2M token overflow
- ⚠️ Performance degrades for very long conversations
- ⚠️ User must manage conversation length manually

### Error Scenarios

**Context Window Overflow:**
```
Current Behavior (V1):
- Gemini API returns error
- Error propagated to user
- No automatic recovery

Desired Behavior (V2):
- Detect approaching limit
- Automatic truncation or summarization
- Graceful degradation
```

## Future Enhancements (V2)

### Context Window Management
```go
// V2: Automatic truncation
func (a *GeminiAdapter) prepareHistory(history []Message) []*genai.Content {
    // Estimate token count
    estimatedTokens := estimateTokens(history)

    // If over limit, truncate or summarize
    if estimatedTokens > a.contextWindowLimit {
        // Option A: Sliding window
        history = history[len(history) - maxMessages:]

        // Option B: Summarize old messages
        summary := summarizeOldMessages(history[:cutoff])
        history = append([]Message{summary}, history[cutoff:]...)
    }

    return convertToGemini(history)
}
```

### Token Counting
```go
// V2: Track token usage
func estimateTokens(messages []Message) int {
    total := 0
    for _, msg := range messages {
        // Rough estimate: 1 token ≈ 4 characters
        total += len(msg.Content) / 4
    }
    return total
}
```

### Caching Optimization
```go
// V2: Cache converted history
type GeminiAdapter struct {
    historyCache map[SessionID]struct{
        messages []Message
        gemini   []*genai.Content
    }
    mu sync.RWMutex
}

func (a *GeminiAdapter) getCachedHistory(sessionID SessionID) []*genai.Content {
    a.mu.RLock()
    defer a.mu.RUnlock()

    cached, exists := a.historyCache[sessionID]
    if !exists {
        return nil
    }

    // Check if cache is valid
    currentHistory, _ := a.GetHistory(sessionID)
    if len(currentHistory) == len(cached.messages) {
        return cached.gemini // Cache hit
    }

    return nil // Cache miss
}
```

### Metrics and Monitoring
```go
// V2: Track performance metrics
type SendMessageMetrics struct {
    HistoryLoadTime   time.Duration
    ConversionTime    time.Duration
    APICallTime       time.Duration
    MessageCount      int
    EstimatedTokens   int
}

func (a *GeminiAdapter) SendMessageWithMetrics(sessionID SessionID, message Message) (*SendMessageMetrics, error) {
    // Track timing and metrics
    metrics := &SendMessageMetrics{}
    // ... implementation
    return metrics, nil
}
```

## Alternative Considered: Hybrid Approach

**Proposal:** Full history for short conversations, sliding window for long ones

```go
const shortConversationThreshold = 50

func (a *GeminiAdapter) prepareHistory(history []Message) []*genai.Content {
    if len(history) <= shortConversationThreshold {
        // Full history for short conversations
        return convertToGemini(history)
    } else {
        // Sliding window for long conversations
        recentHistory := history[len(history)-shortConversationThreshold:]
        return convertToGemini(recentHistory)
    }
}
```

**Why Not Chosen:**
- Added complexity for V1
- Arbitrary threshold hard to tune
- Context loss for longer conversations
- Can be added in V2 based on user feedback

## References

- [Gemini API Documentation](https://ai.google.dev/docs)
- [SPEC.md](SPEC.md) - Context window limitations
- [ARCHITECTURE.md](ARCHITECTURE.md) - Message send flow
- [ADR-001](ADR-001-file-based-storage.md) - History storage format
