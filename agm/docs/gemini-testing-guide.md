# Gemini Agent Testing Guide

**Quick Reference for Testing Gemini Feature Parity**

## Overview

This guide covers testing the Gemini agent implementation to verify feature parity with the Claude agent in AGM (Agent Session Manager).

## Test Files

### Location
```
internal/agent/
├── gemini_adapter.go              # Implementation
├── gemini_adapter_test.go         # Basic unit tests
├── gemini_parity_test.go          # Comprehensive parity tests (NEW)
└── parity_comparison_test.go      # Cross-agent comparison (NEW)
```

### Documentation
```
docs/
├── gemini-parity-analysis.md      # Detailed gap analysis
├── gemini-test-summary.md         # Test execution summary
└── gemini-testing-guide.md        # This file
```

## Quick Start

### Run All Tests

```bash
cd main/agm
go test -v ./internal/agent/
```

Expected output:
```
PASS
ok      github.com/vbonnet/ai-tools/agm/internal/agent    0.358s
```

### Run Specific Test Suites

```bash
# Only parity tests
go test -v ./internal/agent/ -run Parity

# Only Gemini adapter tests
go test -v ./internal/agent/ -run Gemini

# Specific test case
go test -v ./internal/agent/ -run TestGeminiAdapter_SessionLifecycle
```

### Check Coverage

```bash
# Generate coverage report
go test -coverprofile=coverage.out ./internal/agent/

# View coverage in terminal
go tool cover -func=coverage.out

# View coverage in browser
go tool cover -html=coverage.out
```

## Test Categories

### 1. Interface Compliance Tests
**File:** `gemini_parity_test.go`

Tests that Gemini implements all Agent interface methods correctly.

```go
TestGeminiAdapter_FeatureParity_AgentInterface
```

### 2. Session Lifecycle Tests
**File:** `gemini_parity_test.go`

Tests complete session lifecycle: create → resume → terminate.

```go
TestGeminiAdapter_SessionLifecycle
TestGeminiAdapter_ResumeSession_EdgeCases
TestGeminiAdapter_CreateSession_ErrorHandling
```

### 3. History Tests
**File:** `gemini_parity_test.go`

Tests conversation history persistence and retrieval.

```go
TestGeminiAdapter_HistoryPersistence
TestGeminiAdapter_GetHistory_MalformedData
```

### 4. Export/Import Tests
**File:** `gemini_parity_test.go`

Tests conversation export to various formats and import back.

```go
TestGeminiAdapter_ExportImport_RoundTrip
TestGeminiAdapter_ExportMarkdown_Format
TestGeminiAdapter_ImportConversation_InvalidFormat
```

### 5. Capabilities Tests
**File:** `gemini_parity_test.go`

Tests that capabilities are correctly reported.

```go
TestGeminiAdapter_Capabilities_Parity
```

### 6. Command Tests
**File:** `gemini_parity_test.go`

Tests command execution support.

```go
TestGeminiAdapter_ExecuteCommand_Coverage
```

### 7. Parity Comparison Tests
**File:** `parity_comparison_test.go`

Cross-agent comparison tests (Claude vs Gemini).

```go
TestAgentParity_InterfaceCompliance
TestAgentParity_Capabilities
TestAgentParity_SessionLifecycle
TestAgentParity_ExportFormats
TestAgentParity_CommandSupport
```

## Writing New Tests

### Test Pattern

```go
func TestGeminiAdapter_YourFeature(t *testing.T) {
    // Setup temp directory
    tmpDir, err := os.MkdirTemp("", "gemini-test-*")
    require.NoError(t, err)
    defer os.RemoveAll(tmpDir)

    // Override HOME for isolated testing
    originalHome := os.Getenv("HOME")
    os.Setenv("HOME", tmpDir)
    defer os.Setenv("HOME", originalHome)

    // Create adapter with mock store
    adapter, err := NewGeminiAdapter(&GeminiConfig{
        APIKey:       "test-key",
        SessionStore: newTestMockStore(),
    })
    require.NoError(t, err)

    // Test your feature
    // ...

    // Assertions
    assert.Equal(t, expected, actual)
}
```

### Using MockSessionStore

```go
// Create mock store
store := newTestMockStore()

// Create adapter with mock
adapter, err := NewGeminiAdapter(&GeminiConfig{
    APIKey:       "test-key",
    SessionStore: store,
})

// Directly manipulate store for testing
store.sessions[sessionID] = &SessionMetadata{
    TmuxName:   "test",
    CreatedAt:  time.Now(),
    WorkingDir: "/tmp",
}
```

### Testing Error Cases

```go
tests := []struct {
    name        string
    setup       func() SessionID
    wantErr     bool
    errContains string
}{
    {
        name: "error scenario",
        setup: func() SessionID {
            return SessionID("non-existent")
        },
        wantErr:     true,
        errContains: "session not found",
    },
}

for _, tt := range tests {
    t.Run(tt.name, func(t *testing.T) {
        sessionID := tt.setup()
        err := adapter.SomeMethod(sessionID)

        if tt.wantErr {
            require.Error(t, err)
            if tt.errContains != "" {
                assert.Contains(t, err.Error(), tt.errContains)
            }
        } else {
            require.NoError(t, err)
        }
    })
}
```

## Common Test Scenarios

### Testing Session Creation

```go
ctx := SessionContext{
    Name:             "test-session",
    WorkingDirectory: tmpDir,
    Project:          "test-project",
}

sessionID, err := adapter.CreateSession(ctx)
require.NoError(t, err)
assert.NotEmpty(t, sessionID)

// Verify session directory exists
geminiAdapter := adapter.(*GeminiAdapter)
sessionDir, err := geminiAdapter.getSessionDir(sessionID)
require.NoError(t, err)

stat, err := os.Stat(sessionDir)
require.NoError(t, err)
assert.True(t, stat.IsDir())
```

### Testing History Operations

```go
// Add message to history
msg := Message{
    ID:        "msg-1",
    Role:      RoleUser,
    Content:   "Test message",
    Timestamp: time.Now(),
}

geminiAdapter := adapter.(*GeminiAdapter)
err := geminiAdapter.appendToHistory(sessionID, msg)
require.NoError(t, err)

// Retrieve history
history, err := adapter.GetHistory(sessionID)
require.NoError(t, err)
assert.Len(t, history, 1)
assert.Equal(t, "Test message", history[0].Content)
```

### Testing Export/Import

```go
// Export
exportedData, err := adapter.ExportConversation(sessionID, FormatJSONL)
require.NoError(t, err)
assert.NotEmpty(t, exportedData)

// Import
importedSessionID, err := adapter.ImportConversation(exportedData, FormatJSONL)
require.NoError(t, err)
assert.NotEmpty(t, importedSessionID)

// Verify imported history matches original
importedHistory, err := adapter.GetHistory(importedSessionID)
require.NoError(t, err)
assert.Equal(t, originalHistory, importedHistory)
```

## Testing Best Practices

### 1. Isolation
✅ Always use temp directories for test data
✅ Override HOME environment variable
✅ Use MockSessionStore instead of real file store
✅ Clean up resources in defer statements

### 2. Clarity
✅ Use descriptive test names
✅ Test one thing per test function
✅ Use table-driven tests for similar scenarios
✅ Add comments explaining non-obvious test logic

### 3. Assertions
✅ Use require.NoError for setup steps
✅ Use assert.Error for expected errors
✅ Check both error and return values
✅ Verify error messages contain expected strings

### 4. Coverage
✅ Test happy path
✅ Test error paths
✅ Test edge cases (empty, nil, corrupted data)
✅ Test concurrent scenarios if applicable

## Debugging Tests

### Run with Verbose Output

```bash
go test -v ./internal/agent/ -run YourTest
```

### Print Debug Info

```go
t.Logf("Debug info: %v", someVariable)
t.Logf("Session directory: %s", sessionDir)
```

### Run Single Test

```bash
go test -v ./internal/agent/ -run TestGeminiAdapter_SessionLifecycle
```

### Check Test Coverage

```bash
go test -cover ./internal/agent/
```

## CI/CD Integration

### GitHub Actions Example

```yaml
name: Test Gemini Agent

on: [push, pull_request]

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v2

      - name: Set up Go
        uses: actions/setup-go@v2
        with:
          go-version: 1.21

      - name: Run tests
        run: |
          cd agm
          go test -v -cover ./internal/agent/

      - name: Check coverage
        run: |
          cd agm
          go test -coverprofile=coverage.out ./internal/agent/
          go tool cover -func=coverage.out
```

## Known Limitations

### What's Not Tested

❌ **Real API Integration**
- SendMessage with actual Gemini API
- API error handling (rate limits, auth failures)
- Network failures and timeouts

**Reason:** Requires API keys, quota, and external dependencies

**Workaround:** Add integration tests separately with `-tags=integration`

### What Can't Be Tested

❌ **CLI Slash Commands**
- Gemini is API-only, no CLI interface

❌ **Interactive Session State**
- API sessions don't have persistent state like tmux

**Reason:** Architectural differences between API and CLI agents

## FAQ

### Q: Why do some tests use `_ = variable`?

A: To satisfy Go's "declared and not used" compiler check when we create variables for documentation purposes.

### Q: Why mock the SessionStore?

A: To avoid filesystem dependencies and speed up tests. MockSessionStore is in-memory.

### Q: How do I test with a real Gemini API?

A: Create separate integration tests with build tag:
```go
//go:build integration

func TestGeminiAdapter_SendMessage_Integration(t *testing.T) {
    // Requires GEMINI_API_KEY environment variable
}
```

Run with: `go test -tags=integration`

### Q: Why do some commands return no-op?

A: API agents don't have interactive CLI state. Commands like "rename" and "cd" don't apply to stateless API sessions.

### Q: What's the difference between gemini_parity_test.go and parity_comparison_test.go?

A:
- `gemini_parity_test.go`: Tests Gemini in isolation
- `parity_comparison_test.go`: Compares Gemini vs Claude side-by-side

## Resources

### Documentation
- [Gemini Parity Analysis](./gemini-parity-analysis.md) - Detailed gap analysis
- [Test Summary](./gemini-test-summary.md) - Test execution results
- [Agent Interface](../internal/agent/interface.go) - Interface definition

### Related Code
- [Gemini Adapter](../internal/agent/gemini_adapter.go) - Implementation
- [Claude Adapter](../internal/agent/claude_adapter.go) - Reference implementation
- [Mock Agent](../internal/agent/interface_test.go) - Test utilities

## Contributing

### Adding New Tests

1. Identify the feature to test
2. Choose appropriate test file:
   - `gemini_adapter_test.go` for basic unit tests
   - `gemini_parity_test.go` for parity-specific tests
   - `parity_comparison_test.go` for cross-agent tests
3. Follow existing test patterns
4. Run tests locally: `go test -v ./internal/agent/`
5. Update documentation if needed

### Test Checklist

- [ ] Test passes locally
- [ ] Test is isolated (uses temp dir, mock store)
- [ ] Test has descriptive name
- [ ] Test includes error cases
- [ ] Test cleans up resources
- [ ] Coverage increased or maintained
- [ ] Documentation updated if needed

---

**Last Updated:** 2026-02-04
**Maintained By:** AI Tools Team
