# Gemini Client Auto-Detection

## Overview

The Gemini client factory (`gemini_client_factory.go`) automatically detects which Google SDK to use based on available credentials. This allows seamless switching between VertexAI (enterprise) and Google Generative AI (consumer) without code changes.

## Detection Priority

The factory tries credentials in this order (highest to lowest priority):

### 1. VertexAI via Application Default Credentials (ADC)

**Credentials Sources** (checked in order by Google's SDK):
- `GOOGLE_APPLICATION_CREDENTIALS` environment variable pointing to service account JSON
- User credentials from `gcloud auth application-default login`
- GCE/GKE metadata service (when running on Google Cloud)

**Required Environment Variables**:
- `GOOGLE_CLOUD_PROJECT` or `GCP_PROJECT` - GCP project ID
- `GOOGLE_CLOUD_LOCATION` (optional) - Region, defaults to `us-central1`

**Example Setup**:
```bash
# Option A: Service account
export GOOGLE_APPLICATION_CREDENTIALS="/path/to/service-account.json"
export GOOGLE_CLOUD_PROJECT="my-project-id"
export GOOGLE_CLOUD_LOCATION="us-central1"

# Option B: User credentials
gcloud auth application-default login
export GOOGLE_CLOUD_PROJECT="my-project-id"

# Option C: On GCE/GKE (automatic)
export GOOGLE_CLOUD_PROJECT="my-project-id"
```

**SDK Used**: `cloud.google.com/go/aiplatform/apiv1`

### 2. GenAI via API Key

**Required Environment Variable**:
- `GEMINI_API_KEY` - API key from Google AI Studio

**Example Setup**:
```bash
export GEMINI_API_KEY="AIza..."
```

**Get API Key**: https://aistudio.google.com/apikey

**SDK Used**: `github.com/google/generative-ai-go/genai`

### 3. No Credentials Available

If neither VertexAI nor GenAI credentials are found, the factory returns an error with helpful guidance:

```
no Gemini credentials found: set GOOGLE_APPLICATION_CREDENTIALS
(for VertexAI), run 'gcloud auth application-default login'
(for VertexAI), or set GEMINI_API_KEY (for GenAI)
```

## Usage

### Automatic Detection

```go
import "github.com/vbonnet/dear-agent/agm/internal/command"

ctx := context.Background()

// Auto-detect SDK based on environment
client, err := command.NewGeminiClient(ctx)
if err != nil {
    log.Fatalf("No Gemini credentials: %v", err)
}

// Check which SDK was selected
log.Printf("Using SDK: %s", client.GetClientType())

// Use the client
err = client.UpdateConversationTitle(ctx, "conv-123", "New Title")
```

### With Translator

```go
// Auto-detect and create translator
translator, err := command.NewGeminiTranslatorWithAutoDetect(ctx)
if err != nil {
    log.Fatalf("Failed to create translator: %v", err)
}

log.Printf("Translator using: %s", translator.GetClientType())

// Translate commands
err = translator.RenameSession(ctx, sessionID, "new-name")
```

### Manual SDK Selection

```go
// Force VertexAI (skip detection)
client, err := command.NewVertexAIClient(ctx, "my-project", "us-central1")

// Force GenAI (skip detection)
client, err := command.NewGenAIClient(ctx, apiKey)
```

## Implementation Status

### GenAI Client ✅ COMPLETE (Task 1.2)

**File**: `gemini_genai_client.go`

**Status**:
- ✅ Client initialization
- ✅ Auto-detection integration
- ✅ No-op implementations (GenAI SDK is stateless)

**Note**: GenAI SDK doesn't maintain server-side conversations. The `UpdateConversationTitle` and `UpdateConversationMetadata` methods are no-ops because AGM tracks metadata separately in `~/.agm/sessions.json`.

### VertexAI Client ⚠️ PARTIAL (Task 1.1 in progress)

**File**: `gemini_vertexai_client.go`

**Status**:
- ✅ Client initialization
- ✅ Auto-detection integration
- ⚠️ Placeholder implementations (returns errors)

**TODO**: Task 1.1 needs to implement:
- `UpdateConversationTitle` via VertexAI API
- `UpdateConversationMetadata` via VertexAI API

**Challenge**: VertexAI Prediction API doesn't have built-in conversation management. Possible approaches:
1. Store metadata in Cloud Firestore
2. Store metadata in Cloud Storage
3. Use BigQuery for queryable metadata
4. Maintain local metadata file (simpler, less durable)

### Factory ✅ COMPLETE (Task 1.3)

**File**: `gemini_client_factory.go`

**Status**:
- ✅ Auto-detection logic
- ✅ VertexAI credential detection (ADC)
- ✅ GenAI credential detection (API key)
- ✅ Error messages with setup guidance
- ✅ `GetClientType()` method
- ✅ Translator integration

## Testing

### Unit Tests

**File**: `gemini_client_factory_test.go`

**Coverage**:
- ✅ Detection with GenAI API key
- ✅ Detection with no credentials (error case)
- ✅ Empty/invalid API key handling
- ✅ Translator auto-detection
- ✅ Client type constants
- ✅ GetClientType() with different client types
- ✅ GenAI no-op behavior
- ⚠️ VertexAI tests (skipped - requires GCP credentials)

**Run Tests**:
```bash
cd agm
go test ./internal/command/... -v
```

### Integration Tests

**Setup for GenAI**:
```bash
export GEMINI_API_KEY="your-api-key"
go test ./internal/command/... -v -run TestNewGeminiClient_WithGenAIKey
```

**Setup for VertexAI** (requires GCP project):
```bash
export GOOGLE_APPLICATION_CREDENTIALS="/path/to/service-account.json"
export GOOGLE_CLOUD_PROJECT="my-project-id"
go test ./internal/command/... -v
```

## Error Handling

### Detection Failures

```go
client, err := command.NewGeminiClient(ctx)
if err != nil {
    // Check error type
    if strings.Contains(err.Error(), "no Gemini credentials found") {
        // No credentials configured - guide user to setup
        log.Println("Setup instructions:")
        log.Println("- VertexAI: gcloud auth application-default login")
        log.Println("- GenAI: export GEMINI_API_KEY=...")
    }
}
```

### API Call Failures

```go
err := client.UpdateConversationTitle(ctx, sessionID, title)
if err != nil {
    if errors.Is(err, command.ErrAPIFailure) {
        // API call failed - log and retry or fail gracefully
        log.Printf("Gemini API error: %v", err)
    }
}
```

### Client Type Detection

```go
if clientWithType, ok := client.(command.GeminiClientWithType); ok {
    switch clientWithType.GetClientType() {
    case command.ClientTypeVertexAI:
        // VertexAI-specific handling
    case command.ClientTypeGenAI:
        // GenAI-specific handling
    }
}
```

## Architecture

### Interface Hierarchy

```
GeminiClient (base interface)
    ├─ UpdateConversationTitle()
    └─ UpdateConversationMetadata()

GeminiClientWithType (extends GeminiClient)
    └─ GetClientType()

Implementations:
    ├─ VertexAIClient (implements GeminiClientWithType)
    └─ GenAIClient (implements GeminiClientWithType)
```

### Factory Pattern

```
NewGeminiClient(ctx)
    └─> tryVertexAI(ctx)
        ├─ Success → return VertexAIClient
        └─ Failure → try GenAI
            ├─ Success → return GenAIClient
            └─ Failure → return error
```

## Edge Cases

### Multiple Credentials Available

If both VertexAI and GenAI credentials are set, VertexAI takes precedence:

```bash
export GOOGLE_APPLICATION_CREDENTIALS="/path/to/sa.json"
export GOOGLE_CLOUD_PROJECT="my-project"
export GEMINI_API_KEY="AIza..."

# Result: Uses VertexAI (priority 1 beats priority 3)
```

### Invalid Credentials

The factory only checks if credentials **exist**, not if they're **valid**. Invalid credentials will fail during API calls:

```go
client, err := command.NewGeminiClient(ctx) // Succeeds (credentials found)
err = client.UpdateConversationTitle(ctx, ...) // Fails (invalid API key)
```

### Missing Project ID (VertexAI)

VertexAI requires a project ID. If ADC credentials exist but no project ID is set, detection fails:

```bash
export GOOGLE_APPLICATION_CREDENTIALS="/path/to/sa.json"
# Missing: GOOGLE_CLOUD_PROJECT

# Result: Falls back to GenAI (if GEMINI_API_KEY set) or error
```

### Running on GCE/GKE

On Google Compute Engine or Kubernetes, ADC automatically uses the instance/pod service account:

```bash
# No environment variables needed
# Factory uses instance metadata service

# Optional: Set project explicitly
export GOOGLE_CLOUD_PROJECT="my-project"
```

## Future Enhancements

### Planned (Task 1.1)

- [ ] Implement VertexAI conversation metadata storage
- [ ] Add retry logic for VertexAI API calls
- [ ] Add rate limit handling
- [ ] Add context timeout configuration

### Potential

- [ ] Cache credentials to avoid repeated detection
- [ ] Add credential validation (test API call on startup)
- [ ] Support custom VertexAI endpoints
- [ ] Add telemetry/metrics for SDK usage
- [ ] Support switching SDKs at runtime
- [ ] Add credential rotation support

## Related Files

- `gemini_client.go` - GeminiClient interface definition
- `gemini_client_factory.go` - Auto-detection logic (this implementation)
- `gemini_vertexai_client.go` - VertexAI SDK implementation
- `gemini_genai_client.go` - GenAI SDK implementation
- `gemini_translator.go` - Command translator (uses factory)
- `gemini_client_factory_test.go` - Unit tests

## References

- [Google Cloud Auth Guide](https://cloud.google.com/docs/authentication/provide-credentials-adc)
- [VertexAI Go SDK](https://pkg.go.dev/cloud.google.com/go/aiplatform)
- [Generative AI Go SDK](https://pkg.go.dev/github.com/google/generative-ai-go)
- [Google AI Studio API Keys](https://aistudio.google.com/apikey)
