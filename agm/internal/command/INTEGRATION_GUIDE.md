# Gemini Client Factory - Integration Guide

## Overview

This guide explains how the Gemini client factory integrates with the rest of the AGM (Agent Manager) system and how to use it in different contexts.

## Architecture Context

```
AGM (agm)
├── internal/agent/
│   ├── gemini_adapter.go         # Session management (uses GenAI SDK directly)
│   └── claude_adapter.go         # Claude tmux integration
├── internal/command/
│   ├── translator.go             # CommandTranslator interface
│   ├── gemini_translator.go      # Gemini command translator
│   ├── gemini_client_factory.go  # Auto-detection logic (THIS)
│   ├── gemini_vertexai_client.go # VertexAI implementation
│   └── gemini_genai_client.go    # GenAI implementation
└── cmd/agm/
    └── session/
        └── send.go               # Uses translator for commands
```

## Integration Points

### 1. Command Translation Layer (PRIMARY USE CASE)

**File**: `internal/command/gemini_translator.go`

The factory is primarily used by the command translator to translate AGM commands (rename, set directory, run hook) to Gemini API calls.

**Before (Task 1.3)**:
```go
// Manual client injection required
client := &SomeGeminiClient{}
translator := command.NewGeminiTranslator(client)
```

**After (Task 1.3)**:
```go
// Auto-detect SDK based on environment
translator, err := command.NewGeminiTranslatorWithAutoDetect(ctx)
if err != nil {
    log.Fatalf("No Gemini credentials: %v", err)
}

// Translator uses appropriate SDK automatically
translator.RenameSession(ctx, sessionID, "new-name")
```

### 2. Session Management Layer (FUTURE USE CASE)

**File**: `internal/agent/gemini_adapter.go`

Currently, `gemini_adapter.go` uses the GenAI SDK directly for sending messages. In the future, it could use the factory to support VertexAI.

**Current Implementation**:
```go
// Lines 54-61 in gemini_adapter.go
apiKey := os.Getenv("GEMINI_API_KEY")
if apiKey == "" {
    return nil, fmt.Errorf("GEMINI_API_KEY environment variable not set")
}
```

**Potential Future Enhancement**:
```go
// Use factory for SDK detection
client, err := command.NewGeminiClient(ctx)
if err != nil {
    return nil, fmt.Errorf("no Gemini credentials: %w", err)
}

// Store client for message sending
adapter.client = client
```

**Note**: This is not required for Task 1.3. The adapter works fine with GenAI SDK only. VertexAI support in the adapter is a future enhancement.

### 3. AGM CLI Commands

**File**: `cmd/agm/session/send.go` (example)

AGM CLI commands use the translator to execute operations on Gemini sessions.

**Example Usage**:
```go
// In session send command
func sendToGeminiSession(ctx context.Context, sessionID string, message string) error {
    // Create translator with auto-detection
    translator, err := command.NewGeminiTranslatorWithAutoDetect(ctx)
    if err != nil {
        return fmt.Errorf("failed to create Gemini translator: %w", err)
    }

    // Log which SDK is being used
    log.Printf("Using Gemini SDK: %s", translator.GetClientType())

    // Execute command translation
    if err := translator.SetDirectory(ctx, sessionID, workingDir); err != nil {
        return fmt.Errorf("failed to set directory: %w", err)
    }

    // Send message via adapter (separate from translator)
    // ...
}
```

## Usage Patterns

### Pattern 1: Auto-Detection (Recommended)

**When to use**: Production code, CLI tools, default behavior

```go
import "github.com/vbonnet/dear-agent/agm/internal/command"

func main() {
    ctx := context.Background()

    // Let factory detect SDK
    client, err := command.NewGeminiClient(ctx)
    if err != nil {
        log.Fatalf("Setup Gemini credentials: %v", err)
    }

    log.Printf("Detected SDK: %s", client.GetClientType())

    // Use client
    translator := command.NewGeminiTranslator(client)
    // or
    translator, err := command.NewGeminiTranslatorWithAutoDetect(ctx)
}
```

### Pattern 2: Manual Selection (Testing/Debugging)

**When to use**: Tests, debugging, forcing specific SDK

```go
// Force GenAI for testing
client, err := command.NewGenAIClient(ctx, "test-api-key")
translator := command.NewGeminiTranslator(client)

// Force VertexAI for enterprise features
client, err := command.NewVertexAIClient(ctx, "project", "us-central1")
translator := command.NewGeminiTranslator(client)
```

### Pattern 3: Mock Injection (Unit Tests)

**When to use**: Unit tests, behavior verification

```go
// Create mock client
mock := &command.MockGeminiClient{
    UpdateTitleFunc: func(ctx, id, title string) error {
        return nil // or simulate error
    },
}

// Inject into translator
translator := command.NewGeminiTranslator(mock)

// Verify behavior
translator.RenameSession(ctx, "id", "name")
assert.Equal(t, 1, len(mock.CallLog))
```

## Environment Setup

### Development Environment

For local development, use GenAI (simplest):

```bash
# .envrc or .env
export GEMINI_API_KEY="AIza..."

# Or in shell profile
echo 'export GEMINI_API_KEY="AIza..."' >> ~/.bashrc
```

### CI/CD Environment

For CI/CD, use service account:

```yaml
# .github/workflows/test.yml
env:
  GOOGLE_APPLICATION_CREDENTIALS: ${{ secrets.GCP_SA_KEY_PATH }}
  GOOGLE_CLOUD_PROJECT: ${{ secrets.GCP_PROJECT_ID }}
```

### Production Environment (GCP)

For production on GCP, use instance service account:

```bash
# No environment variables needed
# Metadata service provides credentials automatically

# Optional: Explicit project ID
export GOOGLE_CLOUD_PROJECT="production-project-id"
```

## Migration Guide

### Migrating from Hardcoded GenAI

**Before**:
```go
apiKey := os.Getenv("GEMINI_API_KEY")
client, err := genai.NewClient(ctx, option.WithAPIKey(apiKey))
// Use client...
```

**After**:
```go
client, err := command.NewGeminiClient(ctx)
// Auto-detects GenAI or VertexAI
// Use client...
```

### Migrating from Direct VertexAI

**Before**:
```go
client, err := aiplatform.NewPredictionClient(ctx)
// Use client...
```

**After**:
```go
client, err := command.NewGeminiClient(ctx)
// Auto-detects VertexAI if credentials available
// Use client...
```

## Error Handling Best Practices

### Startup Errors

```go
client, err := command.NewGeminiClient(ctx)
if err != nil {
    // Error message contains setup instructions
    log.Println("Gemini setup required:")
    log.Println(err.Error())

    // Provide actionable guidance
    log.Println("\nQuick setup:")
    log.Println("  export GEMINI_API_KEY='your-key'")
    log.Println("  Get key: https://aistudio.google.com/apikey")

    os.Exit(1)
}
```

### Runtime Errors

```go
err := translator.RenameSession(ctx, sessionID, newName)
if err != nil {
    if errors.Is(err, command.ErrAPIFailure) {
        // API call failed - retry or graceful degradation
        log.Printf("Gemini API error (retrying): %v", err)
        // Implement retry logic
    } else if errors.Is(err, command.ErrNotSupported) {
        // Command not supported - skip or alternative approach
        log.Printf("Rename not supported for %s, skipping",
                   translator.GetClientType())
    } else {
        // Other error (context timeout, etc.)
        return fmt.Errorf("failed to rename session: %w", err)
    }
}
```

## Testing Strategy

### Unit Tests

Test auto-detection logic in isolation:

```go
func TestAutoDetection(t *testing.T) {
    os.Setenv("GEMINI_API_KEY", "test-key")
    defer os.Unsetenv("GEMINI_API_KEY")

    client, err := command.NewGeminiClient(context.Background())
    assert.NoError(t, err)
    assert.Equal(t, command.ClientTypeGenAI, client.GetClientType())
}
```

### Integration Tests

Test with real APIs (requires credentials):

```go
// +build integration

func TestGeminiAPI(t *testing.T) {
    if testing.Short() {
        t.Skip("Skipping integration test")
    }

    ctx := context.Background()
    client, err := command.NewGeminiClient(ctx)
    require.NoError(t, err)

    err = client.UpdateConversationTitle(ctx, "test-conv", "Test Title")
    // Verify API response...
}
```

Run integration tests:
```bash
go test -tags=integration ./internal/command/...
```

### Mock Tests

Test command translation without real API calls:

```go
func TestCommandTranslation(t *testing.T) {
    mock := &command.MockGeminiClient{}
    translator := command.NewGeminiTranslator(mock)

    translator.RenameSession(ctx, "id", "name")

    assert.Equal(t, []string{"UpdateTitle(id, name)"}, mock.CallLog)
}
```

## Monitoring and Observability

### Log SDK Type on Startup

```go
client, err := command.NewGeminiClient(ctx)
if err != nil {
    log.Fatalf("Gemini client error: %v", err)
}

log.Printf("Gemini SDK initialized: type=%s", client.GetClientType())
```

### Add Metrics

```go
// Track SDK usage
switch client.GetClientType() {
case command.ClientTypeVertexAI:
    metrics.Increment("gemini.sdk.vertexai")
case command.ClientTypeGenAI:
    metrics.Increment("gemini.sdk.genai")
}
```

### Error Rate Tracking

```go
err := translator.RenameSession(ctx, sessionID, newName)
if err != nil {
    metrics.Increment("gemini.command.rename.error")
    log.Printf("Rename failed: sdk=%s error=%v",
               translator.GetClientType(), err)
}
```

## Performance Considerations

### Detection Overhead

Auto-detection adds minimal overhead (~1-5ms for ADC search):

```go
start := time.Now()
client, _ := command.NewGeminiClient(ctx)
log.Printf("Detection took: %v", time.Since(start))
// Typical: 1-5ms
```

### Caching Clients

For high-throughput scenarios, cache the client:

```go
var (
    clientOnce sync.Once
    cachedClient command.GeminiClientWithType
    clientErr error
)

func getGeminiClient(ctx context.Context) (command.GeminiClientWithType, error) {
    clientOnce.Do(func() {
        cachedClient, clientErr = command.NewGeminiClient(ctx)
    })
    return cachedClient, clientErr
}
```

## Security Considerations

### Credential Storage

- **GenAI API Key**: Store in secrets manager (e.g., GCP Secret Manager, AWS Secrets Manager)
- **VertexAI**: Use service account with minimal permissions

### Least Privilege

For VertexAI service accounts, grant only required permissions:

```bash
gcloud projects add-iam-policy-binding PROJECT_ID \
    --member="serviceAccount:SA_EMAIL" \
    --role="roles/aiplatform.user"
```

### API Key Rotation

For GenAI, rotate API keys periodically:

```bash
# Generate new key in Google AI Studio
# Update secret in secrets manager
# Restart services to pick up new key
```

## Future Enhancements

### Credential Caching

```go
// TODO: Add credential caching with TTL
type CachedClient struct {
    client command.GeminiClientWithType
    expires time.Time
}
```

### Runtime SDK Switching

```go
// TODO: Support switching SDK without creating new client
func (c *Client) SwitchSDK(newType command.ClientType) error {
    // Re-initialize with new SDK
}
```

### Credential Validation

```go
// TODO: Add optional credential validation
func ValidateCredentials(ctx context.Context) error {
    client, err := command.NewGeminiClient(ctx)
    if err != nil {
        return err
    }
    // Make test API call to verify credentials work
    return client.UpdateConversationTitle(ctx, "test", "test")
}
```

## Related Documentation

- **Quick Start**: [QUICK_START.md](QUICK_START.md) - 30-second setup guide
- **Detection Details**: [GEMINI_CLIENT_DETECTION.md](GEMINI_CLIENT_DETECTION.md) - Full algorithm documentation
- **Task Report**: [TASK_1.3_COMPLETION_REPORT.md](TASK_1.3_COMPLETION_REPORT.md) - Implementation details
- **Examples**: [example_factory_usage_test.go](example_factory_usage_test.go) - Runnable code examples

## Support

For issues or questions:
1. Check error message for setup instructions
2. Review [QUICK_START.md](QUICK_START.md) for common scenarios
3. Check [GEMINI_CLIENT_DETECTION.md](GEMINI_CLIENT_DETECTION.md) for edge cases
4. Review examples in [example_factory_usage_test.go](example_factory_usage_test.go)
