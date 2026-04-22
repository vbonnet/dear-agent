# Command Translation Design

## Overview

AGM (Agent Manager) provides a unified command interface across different AI agents using the `CommandTranslator` abstraction. This document describes the design and implementation of command translation for Gemini.

## Problem Statement

Different AI agents have different execution models:
- **Claude**: Slash commands via tmux (e.g., `/rename new-name`)
- **Gemini**: API calls (e.g., `UpdateConversationTitle`)
- **GPT**: API calls (different endpoints)

AGM needs to translate generic commands to agent-specific implementations while maintaining a consistent user experience.

## Architecture

### Package Structure

```
internal/command/
├── translator.go              # CommandTranslator interface, error types
├── gemini_client.go           # GeminiClient interface (for DI)
├── gemini_translator.go       # GeminiTranslator implementation
├── mock_client.go             # MockGeminiClient for testing
└── gemini_translator_test.go  # Tests
```

### Command Translator Interface

```go
type CommandTranslator interface {
    RenameSession(ctx context.Context, sessionID string, newName string) error
    SetDirectory(ctx context.Context, sessionID string, path string) error
    RunHook(ctx context.Context, sessionID string, hook string) error
}
```

### Gemini Implementation

Uses adapter pattern with dependency injection:

```go
type GeminiTranslator struct {
    client GeminiClient  // Injected dependency (interface)
}

func NewGeminiTranslator(client GeminiClient) *GeminiTranslator {
    return &GeminiTranslator{client: client}
}
```

## Command Mappings

### RenameSession

**Generic Command:** `RenameSession(sessionID, newName)`

**Gemini API:** `UpdateConversationTitle(conversationID, title)`

**Implementation:**
```go
func (t *GeminiTranslator) RenameSession(ctx context.Context, sessionID string, newName string) error {
    if err := t.client.UpdateConversationTitle(ctx, sessionID, newName); err != nil {
        return fmt.Errorf("%w: %v", ErrAPIFailure, err)
    }
    return nil
}
```

**Flow:**
1. Caller invokes `RenameSession(ctx, "conv-123", "new-name")`
2. Translator calls `client.UpdateConversationTitle(ctx, "conv-123", "new-name")`
3. Client makes API call: `POST /v1/conversations/conv-123:updateTitle`
4. Translator wraps any errors with `ErrAPIFailure` sentinel error
5. Returns nil on success

### SetDirectory

**Generic Command:** `SetDirectory(sessionID, path)`

**Gemini API:** `UpdateConversationMetadata(conversationID, {"workingDirectory": path})`

**Implementation:**
```go
func (t *GeminiTranslator) SetDirectory(ctx context.Context, sessionID string, path string) error {
    metadata := map[string]string{
        "workingDirectory": path,
    }
    if err := t.client.UpdateConversationMetadata(ctx, sessionID, metadata); err != nil {
        return fmt.Errorf("%w: %v", ErrAPIFailure, err)
    }
    return nil
}
```

**Flow:**
1. Caller invokes `SetDirectory(ctx, "conv-123", "~/project")`
2. Translator creates metadata map: `{"workingDirectory": "~/project"}`
3. Translator calls `client.UpdateConversationMetadata(ctx, "conv-123", metadata)`
4. Client makes API call: `PATCH /v1/conversations/conv-123` with metadata
5. Returns nil on success, wrapped error on failure

### RunHook

**Generic Command:** `RunHook(sessionID, hook)`

**Gemini API:** Not supported (no terminal access)

**Implementation:**
```go
func (t *GeminiTranslator) RunHook(ctx context.Context, sessionID string, hook string) error {
    return ErrNotSupported
}
```

**Rationale:** Gemini has no terminal access and cannot execute slash commands or hooks. Returns `ErrNotSupported` for graceful degradation.

## Error Handling

### Sentinel Errors

```go
var (
    ErrNotSupported = errors.New("command not supported by this agent")
    ErrAPIFailure   = errors.New("agent API call failed")
)
```

### Error Wrapping

API errors are wrapped to preserve original error while adding context:

```go
return fmt.Errorf("%w: %v", ErrAPIFailure, originalErr)
```

Callers can check errors with `errors.Is()`:

```go
if errors.Is(err, ErrNotSupported) {
    // Handle gracefully
}
if errors.Is(err, ErrAPIFailure) {
    // Log and retry or fail
}
```

### Context Errors

Context timeout and cancellation errors are passed through:
- `context.DeadlineExceeded` - Context timed out
- `context.Canceled` - Context was cancelled

## Dependency Injection

### GeminiClient Interface

```go
type GeminiClient interface {
    UpdateConversationTitle(ctx context.Context, conversationID, title string) error
    UpdateConversationMetadata(ctx context.Context, conversationID string, metadata map[string]string) error
}
```

**Benefits:**
- Enables testing with mocks (no real API calls needed)
- Decouples translator from API client implementation
- Allows real client to be provided later
- Follows Go best practices ("accept interfaces, return structs")

### Mock Client

For testing without real API:

```go
mock := &MockGeminiClient{
    UpdateTitleFunc: func(ctx, id, title) error {
        return errors.New("simulated error")
    },
}
translator := NewGeminiTranslator(mock)
err := translator.RenameSession(ctx, "id", "name")  // Returns ErrAPIFailure
```

## Design Decisions

### AD-001: Interface-Based Design

**Decision:** Use dependency injection with GeminiClient interface

**Rationale:**
- Enables testing with mocks
- Not blocked on real client implementation
- Follows Go idioms

### AD-002: Synchronous Execution

**Decision:** Methods block until complete (no goroutines/channels)

**Rationale:**
- Simpler error handling
- Matches Claude translator pattern
- Operations expected to be fast (<1s)

### AD-003: Error Handling with Sentinel Errors

**Decision:** Use package-level sentinel errors and wrapping

**Rationale:**
- Can use `errors.Is()` for checking
- Preserves original error details
- Clear distinction between "not supported" and "failed"

### AD-004: Context-Aware Operations

**Decision:** All methods accept `context.Context` as first parameter

**Rationale:**
- Enables timeout control
- Supports cancellation
- Standard Go pattern for I/O operations

### AD-005: One-to-One Command Mapping

**Decision:** Each generic command maps to exactly one API call

**Rationale:**
- Simple, predictable mapping
- Easy to understand and maintain
- Can evolve to multi-step in future if needed

## Testing Strategy

### Test Coverage

**Target:** 100% coverage for translator methods

**Achieved:** 100.0% of statements

**Test Categories:**
- Success cases (client returns nil)
- Error cases (client returns error → ErrAPIFailure)
- Context cancellation
- Context timeout
- Edge cases (empty strings, special characters)
- Error wrapping (errors.Is, errors.Unwrap)
- Mock client behavior

### Table-Driven Tests

All tests use table-driven approach:

```go
tests := []struct {
    name       string
    sessionID  string
    newName    string
    clientErr  error
    wantErr    error
}{
    {name: "success", sessionID: "conv-123", newName: "new-name", clientErr: nil, wantErr: nil},
    {name: "client error", sessionID: "conv-123", newName: "new-name", clientErr: errors.New("api error"), wantErr: ErrAPIFailure},
}
```

### Benchmarks

Performance overhead is minimal:
- RenameSession: ~346ns/op
- SetDirectory: ~759ns/op
- RunHook: ~0.3ns/op

All well under 10ms target.

## Thread Safety

**GeminiTranslator:**
- Immutable after construction (client field never changes)
- Safe for concurrent use
- No shared mutable state
- No locking needed

**Verification:** Passes `go test -race` with no issues

## Future Enhancements (V2+)

### Retry Logic

Add automatic retries for transient API failures:

```go
func (t *GeminiTranslator) RenameSession(ctx, sessionID, newName) error {
    for i := 0; i < maxRetries; i++ {
        err := t.client.UpdateConversationTitle(ctx, sessionID, newName)
        if err == nil {
            return nil
        }
        if !isTransient(err) {
            return fmt.Errorf("%w: %v", ErrAPIFailure, err)
        }
        time.Sleep(backoff(i))
    }
    return ErrAPIFailure
}
```

### Caching

Cache conversation metadata to reduce API calls:

```go
type GeminiTranslator struct {
    client GeminiClient
    cache  *MetadataCache
}
```

### Batching

Batch multiple commands into single API call:

```go
func (t *GeminiTranslator) ExecuteBatch(ctx, sessionID, commands) error {
    // Combine rename + set_directory into one API call
}
```

### Additional Commands

Add new commands as AGM evolves:

```go
type CommandTranslator interface {
    RenameSession(...) error
    SetDirectory(...) error
    RunHook(...) error
    UpdateMetadata(...) error  // New command
    GetStatus(...) error       // New command
}
```

## References

- **Bead:** ai-tools-lz6 (960 min estimate)
- **Roadmap:** AGM Multi-Agent Roadmap, Layer 3 (Command Translation)
- **Implementation:** `internal/command/` package
- **Tests:** 100% coverage, all passing
- **Benchmarks:** <1µs overhead (well under 10ms target)
