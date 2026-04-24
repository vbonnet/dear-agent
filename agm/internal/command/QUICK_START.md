# Gemini Client Factory - Quick Start Guide

## 30-Second Setup

### For GenAI (Simple, Free Tier Available)

```bash
# 1. Get API key: https://aistudio.google.com/apikey
export GEMINI_API_KEY="AIza..."

# 2. Use in code
go run your_app.go
```

### For VertexAI (Enterprise, GCP Required)

```bash
# 1. Setup GCP credentials
gcloud auth application-default login
export GOOGLE_CLOUD_PROJECT="my-project-id"

# 2. Use in code (auto-detects VertexAI)
go run your_app.go
```

## Basic Usage

```go
package main

import (
    "context"
    "log"

    "github.com/vbonnet/dear-agent/agm/internal/command"
)

func main() {
    ctx := context.Background()

    // Auto-detect SDK (VertexAI or GenAI)
    client, err := command.NewGeminiClient(ctx)
    if err != nil {
        log.Fatalf("No credentials: %v", err)
    }

    // Use the client
    log.Printf("Using: %s", client.GetClientType())

    err = client.UpdateConversationTitle(ctx, "conv-123", "My Title")
    if err != nil {
        log.Printf("Failed: %v", err)
    }
}
```

## With Translator

```go
// Create translator with auto-detection
translator, err := command.NewGeminiTranslatorWithAutoDetect(ctx)
if err != nil {
    log.Fatalf("Failed: %v", err)
}

// Translate commands
translator.RenameSession(ctx, "session-123", "new-name")
translator.SetDirectory(ctx, "session-123", "/workspace")
```

## Troubleshooting

### "no Gemini credentials found"

**Solution 1 (GenAI - Easiest)**:
```bash
export GEMINI_API_KEY="your-api-key"
```

**Solution 2 (VertexAI)**:
```bash
gcloud auth application-default login
export GOOGLE_CLOUD_PROJECT="your-project"
```

### "VertexAI requires GOOGLE_CLOUD_PROJECT"

```bash
export GOOGLE_CLOUD_PROJECT="my-project-id"
# or
export GCP_PROJECT="my-project-id"
```

### Which SDK am I using?

```go
client, _ := command.NewGeminiClient(ctx)
log.Printf("SDK: %s", client.GetClientType())

// Output: "SDK: genai" or "SDK: vertexai"
```

## SDK Comparison

| Feature | GenAI | VertexAI |
|---------|-------|----------|
| Setup | ✅ Easy | ⚠️ Moderate |
| Auth | API Key | GCP Credentials |
| Free Tier | ✅ Yes | ❌ No |
| Enterprise | ❌ No | ✅ Yes |
| VPC-SC | ❌ No | ✅ Yes |
| Audit Logs | ❌ No | ✅ Yes |

## Next Steps

- **Full Docs**: See [GEMINI_CLIENT_DETECTION.md](GEMINI_CLIENT_DETECTION.md)
- **Examples**: See [example_factory_usage_test.go](example_factory_usage_test.go)
- **Tests**: Run `go test ./internal/command/... -v`

## Common Patterns

### Force Specific SDK

```go
// Force GenAI (bypass auto-detection)
client, err := command.NewGenAIClient(ctx, "my-api-key")

// Force VertexAI (bypass auto-detection)
client, err := command.NewVertexAIClient(ctx, "project", "us-central1")
```

### Check SDK Type

```go
switch client.GetClientType() {
case command.ClientTypeVertexAI:
    log.Println("Using enterprise VertexAI")
case command.ClientTypeGenAI:
    log.Println("Using consumer GenAI")
}
```

### Error Handling

```go
client, err := command.NewGeminiClient(ctx)
if err != nil {
    // Error contains setup instructions
    log.Println(err.Error())
    // Output: "no Gemini credentials found: set GOOGLE_APPLICATION_CREDENTIALS..."
}
```
