# OpenAI SDK Integration

This package provides a Go client for the OpenAI API with support for both standard OpenAI and Azure OpenAI endpoints.

## Features

- **Conversation History Support**: Send full conversation history with each request
- **Error Classification**: Structured error types for better error handling
- **Azure OpenAI Support**: Works with both OpenAI and Azure OpenAI endpoints
- **Configuration Flexibility**: Environment variables or explicit configuration
- **Type Safety**: Strongly typed request/response structures

## Installation

The package uses the official `github.com/sashabaranov/go-openai` SDK:

```bash
go get github.com/sashabaranov/go-openai
```

## Quick Start

### Standard OpenAI

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/vbonnet/ai-tools/agm/internal/agent/openai"
)

func main() {
    // Create client using environment variable OPENAI_API_KEY
    config := openai.DefaultConfig()
    client, err := openai.NewClient(context.Background(), config)
    if err != nil {
        log.Fatal(err)
    }

    // Send a message
    messages := []openai.Message{
        {
            Role:    "system",
            Content: "You are a helpful assistant.",
        },
        {
            Role:    "user",
            Content: "What is the capital of France?",
        },
    }

    resp, err := client.CreateChatCompletion(context.Background(), messages)
    if err != nil {
        log.Fatal(err)
    }

    fmt.Println("Response:", resp.Content)
    fmt.Printf("Tokens used: %d\n", resp.Usage.TotalTokens)
}
```

### Azure OpenAI

```go
config := openai.Config{
    APIKey:          "your-azure-api-key",
    BaseURL:         "https://your-resource.openai.azure.com",
    Model:           "gpt-4",
    IsAzure:         true,
    AzureAPIVersion: "2024-02-15-preview",
}

client, err := openai.NewClient(context.Background(), config)
// ... use client as normal
```

### With Custom Configuration

```go
config := openai.Config{
    APIKey:      "your-api-key",
    Model:       "gpt-4-turbo",
    Temperature: 0.3,
    MaxTokens:   2000,
}

client, err := openai.NewClient(context.Background(), config)
```

## Conversation History

The client supports multi-turn conversations by maintaining message history:

```go
messages := []openai.Message{
    {
        Role:    "system",
        Content: "You are a helpful assistant.",
    },
    {
        Role:    "user",
        Content: "My name is Alice.",
    },
    {
        Role:    "assistant",
        Content: "Hello Alice! Nice to meet you.",
    },
    {
        Role:    "user",
        Content: "What is my name?",
    },
}

resp, err := client.CreateChatCompletion(context.Background(), messages)
// Response will correctly identify the user as Alice
```

## Error Handling

The client provides structured error types for better error handling:

```go
resp, err := client.CreateChatCompletion(ctx, messages)
if err != nil {
    var clientErr *openai.ClientError
    if errors.As(err, &clientErr) {
        switch clientErr.Type {
        case openai.ErrorTypeAPIKeyMissing:
            // Handle missing API key
        case openai.ErrorTypeRateLimit:
            // Handle rate limiting (429)
        case openai.ErrorTypeAuthError:
            // Handle authentication failure (401)
        case openai.ErrorTypeInvalidRequest:
            // Handle invalid request (400)
        case openai.ErrorTypeAPIError:
            // Handle general API errors
        }
    }
}
```

### Error Types

- `ErrorTypeAPIKeyMissing`: API key not configured
- `ErrorTypeAuthError`: Authentication failed (401)
- `ErrorTypeRateLimit`: Rate limit exceeded (429)
- `ErrorTypeInvalidRequest`: Invalid request parameters (400)
- `ErrorTypeAPIError`: General API error

## Model Selection

The OpenAI adapter supports multiple models with different capabilities:

### Supported Models

| Model | Description | Streaming | Use Case |
|-------|-------------|-----------|----------|
| `gpt-4` | Most capable GPT-4 model | ✅ | Complex reasoning, code |
| `gpt-4-turbo` | Faster GPT-4 variant | ✅ | General purpose, balanced |
| `gpt-4-turbo-preview` | Latest preview (default) | ✅ | Cutting edge features |
| `gpt-4.1` | GPT-4.1 base model | ✅ | Enhanced reasoning |
| `gpt-4.1-mini` | Smaller, faster GPT-4.1 | ✅ | Quick tasks |
| `gpt-4o` | Optimized GPT-4 | ✅ | Production workloads |
| `gpt-4o-mini` | Mini optimized variant | ✅ | Cost-effective |
| `o3` | Reasoning model | ❌ | Deep reasoning tasks |
| `o4-mini` | Mini reasoning model | ❌ | Light reasoning |
| `gpt-3.5-turbo` | Fast, cost-effective | ✅ | Simple tasks, chat |

**Note:** Reasoning models (o3, o4-mini) do not support streaming responses.

### Setting the Model

You can specify the model in three ways:

1. **Environment Variable** (highest priority):
   ```bash
   export OPENAI_MODEL=gpt-4o
   ```

2. **Config Parameter** (overrides environment):
   ```go
   config := openai.Config{
       APIKey: "your-key",
       Model:  "gpt-4-turbo",
   }
   ```

3. **Default**: `gpt-4-turbo-preview` (if neither environment nor config is set)

### Example: Using Different Models

```go
// Use GPT-4o for production
config := openai.Config{
    APIKey: os.Getenv("OPENAI_API_KEY"),
    Model:  "gpt-4o",
}
client, err := openai.NewClient(ctx, config)

// Use GPT-3.5-turbo for cost savings
config := openai.Config{
    APIKey: os.Getenv("OPENAI_API_KEY"),
    Model:  "gpt-3.5-turbo",
}
client, err := openai.NewClient(ctx, config)

// Use o3 for deep reasoning (no streaming)
config := openai.Config{
    APIKey: os.Getenv("OPENAI_API_KEY"),
    Model:  "o3",
}
client, err := openai.NewClient(ctx, config)
```

### Model Validation

The client validates models at initialization:

```go
config := openai.Config{
    APIKey: "your-key",
    Model:  "invalid-model",
}

client, err := openai.NewClient(ctx, config)
// Returns: ErrorTypeInvalidModel with list of supported models
```

## Configuration Options

### Config Fields

| Field | Type | Description | Default |
|-------|------|-------------|---------|
| `APIKey` | string | OpenAI API key | `OPENAI_API_KEY` env var |
| `BaseURL` | string | API base URL | `https://api.openai.com/v1` |
| `Model` | string | Model to use | `OPENAI_MODEL` env var or `gpt-4-turbo-preview` |
| `Temperature` | float32 | Randomness (0.0-2.0) | `0.7` |
| `MaxTokens` | int | Max tokens to generate | `1000` |
| `IsAzure` | bool | Use Azure OpenAI | `false` |
| `AzureAPIVersion` | string | Azure API version | `2024-02-15-preview` |

### Environment Variables

- `OPENAI_API_KEY`: API key (required if not set in Config)
- `OPENAI_MODEL`: Model name (optional, defaults to `gpt-4-turbo-preview`)

## Testing

### Unit Tests

Run unit tests (no API key required):

```bash
cd internal/agent/openai
go test -v
```

### Integration Tests

Run integration tests (requires API key):

```bash
OPENAI_API_KEY=sk-... go test -v -run Integration
```

## Architecture

The client is designed to integrate with the AGM (Agent Manager) multi-agent architecture:

1. **Client Layer** (`client.go`): Low-level OpenAI API client
   - Handles authentication
   - Manages HTTP communication
   - Provides error classification

2. **Adapter Layer** ✅ IMPLEMENTED: Implements the `agent.Agent` interface
   - Session management (CreateSession, ResumeSession, TerminateSession)
   - Conversation history persistence (JSONL storage)
   - Command translation (RenameSession, SetDirectory, ClearHistory)
   - Hook integration (synthetic SessionStart/SessionEnd hooks)

3. **Registry Integration** ✅ COMPLETE: Registered with `agent.Register()` for discovery

## Hook Support (Phase 3)

The OpenAI adapter supports **synthetic lifecycle hooks** for session events. Since OpenAI is API-based (no CLI subprocess), hooks are triggered by the adapter at key lifecycle points.

### Supported Hooks

- **SessionStart**: Triggered when a new session is created
- **SessionEnd**: Triggered when a session is terminated

### Hook Architecture

Hooks are file-based:
- Hook context files created in `~/.agm/openai-hooks/`
- JSON format with session metadata (session_id, hook_name, session_name, working_dir, model, timestamp)
- External scripts can monitor these files for integration

### Example Hook Context

```json
{
  "session_id": "abc-123",
  "hook_name": "SessionStart",
  "session_name": "my-session",
  "working_dir": "/workspace/project",
  "model": "gpt-4-turbo-preview",
  "timestamp": "2026-02-24T16:30:00Z"
}
```

### Documentation

- `HOOKS-ARCHITECTURE.md` - Hook design and implementation
- `OPENAI_HOOK_INTEGRATION.md` - Integration guide
- `HOOK-TEST-RESULTS.md` - Test results and validation

## Future Enhancements

- [x] Session management and persistence ✅ (Phase 2)
- [x] Hook integration (SessionStart/SessionEnd) ✅ (Phase 3)
- [ ] Streaming support for real-time responses (planned)
- [ ] Function/tool calling support (planned)
- [ ] Token counting and context window management (planned)
- [ ] Retry logic with exponential backoff (planned)
- [ ] Request/response logging (planned)
- [ ] Metrics collection (planned)
- [ ] Image input support (vision models) (planned)
- [ ] BeforeAgent/AfterAgent hooks (future phase)

## Reference

This implementation follows the pattern established in `internal/evaluation/gpt4_judge.go` but uses the official OpenAI SDK instead of manual HTTP requests.

### Key Differences from gpt4_judge.go

1. **SDK vs Manual HTTP**: Uses `go-openai` SDK instead of manual HTTP client
2. **Error Handling**: Structured error types instead of generic errors
3. **Configuration**: More flexible configuration with Azure support
4. **Conversation Support**: Built-in support for conversation history
5. **Type Safety**: Leverages SDK's type definitions

## License

Same as parent project.
