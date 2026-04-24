# Gemini Adapter

**Google Gemini 2.0 integration for AGM (AI/Agent Gateway Manager)**

## Overview

The Gemini Adapter enables AGM to use Google's Gemini 2.0 models (including Gemini 2.0 Flash, Gemini 2.0 Flash Thinking) as AI agents. It implements the unified `agent.Agent` interface, allowing seamless switching between Claude, Gemini, and GPT agents.

## Quick Start

### Prerequisites

```bash
# 1. Get Gemini API key from Google AI Studio
# https://makersuite.google.com/app/apikey

# 2. Set environment variable
export GEMINI_API_KEY="your-api-key-here"
```

### Basic Usage

```go
package main

import (
    "fmt"
    "github.com/vbonnet/dear-agent/agm/internal/agent"
)

func main() {
    // Create Gemini adapter
    gemini, err := agent.NewGeminiAdapter(nil)
    if err != nil {
        panic(err)
    }

    // Create session
    sessionID, err := gemini.CreateSession(agent.SessionContext{
        Name:             "my-session",
        WorkingDirectory: "~/project",
        Project:          "demo",
    })
    if err != nil {
        panic(err)
    }

    // Send message
    err = gemini.SendMessage(sessionID, agent.Message{
        Role:    agent.RoleUser,
        Content: "Hello, Gemini!",
    })
    if err != nil {
        panic(err)
    }

    // Get conversation history
    history, err := gemini.GetHistory(sessionID)
    if err != nil {
        panic(err)
    }

    for _, msg := range history {
        fmt.Printf("%s: %s\n", msg.Role, msg.Content)
    }
}
```

## Features

### Supported
- ✅ Conversation sessions with persistence
- ✅ Full conversation history context
- ✅ File-based JSONL storage
- ✅ Export/Import (JSONL, Markdown)
- ✅ Multi-turn conversations
- ✅ Session metadata management

### Not Yet Implemented (V2)
- ⏳ Streaming responses
- ⏳ Function/tool calling
- ⏳ Vision input (images)
- ⏳ System instructions
- ⏳ Context window management
- ⏳ Token usage tracking
- ⏳ Retry logic with exponential backoff

## Configuration

### Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `GEMINI_API_KEY` | Yes | None | Google AI API key |

### Model Selection

```go
import "github.com/vbonnet/dear-agent/agm/internal/agent"

// Custom model
gemini, err := agent.NewGeminiAdapter(&agent.GeminiConfig{
    ModelName: "gemini-2.0-flash-thinking-exp", // Default: gemini-2.0-flash-exp
    APIKey:    "your-key",                      // Optional: uses env var if empty
})
```

### Available Models

| Model | Context Window | Features |
|-------|----------------|----------|
| `gemini-2.0-flash-exp` | 1M tokens (2M thinking) | Fast, general-purpose |
| `gemini-2.0-flash-thinking-exp` | 2M tokens | Extended thinking mode |
| `gemini-1.5-pro` | 2M tokens | Previous generation |
| `gemini-1.5-flash` | 1M tokens | Faster, previous gen |

## Storage

### Session Directory Structure

```
~/.agm/
├── sessions.json              # Session metadata (all agents)
└── gemini/
    ├── abc-123.../
    │   └── history.jsonl      # Conversation history
    └── def-456.../
        └── history.jsonl
```

### History Format (JSONL)

```jsonl
{"id":"msg-1","role":"user","content":"Hello","timestamp":"2026-02-11T10:00:00Z"}
{"id":"msg-2","role":"assistant","content":"Hi!","timestamp":"2026-02-11T10:00:01Z"}
```

## API Reference

See [Agent Interface](../interface.go) for full API documentation.

### Core Methods

```go
// Session management
CreateSession(ctx SessionContext) (SessionID, error)
ResumeSession(sessionID SessionID) error
TerminateSession(sessionID SessionID) error
GetSessionStatus(sessionID SessionID) (Status, error)

// Communication
SendMessage(sessionID SessionID, message Message) error
GetHistory(sessionID SessionID) ([]Message, error)

// Serialization
ExportConversation(sessionID SessionID, format ConversationFormat) ([]byte, error)
ImportConversation(data []byte, format ConversationFormat) (SessionID, error)

// Metadata
Name() string
Version() string
Capabilities() Capabilities
```

## Testing

### Unit Tests

```bash
# Run all tests (no API key required)
go test ./internal/agent -run TestGeminiAdapter -v

# Test coverage
go test ./internal/agent -run TestGeminiAdapter -cover
```

### Integration Tests (Optional)

```bash
# Requires real Gemini API key
export GEMINI_API_KEY="your-key"
export INTEGRATION_TESTS=true

go test ./internal/agent -run TestGeminiIntegration -v
```

## Architecture

### High-Level Design

```
┌─────────────────────────────────────┐
│      Agent Interface (12 methods)    │
└─────────────────┬───────────────────┘
                  │
                  ▼
┌─────────────────────────────────────┐
│        GeminiAdapter                │
│  - sessionStore: SessionStore        │
│  - modelName: string                │
│  - apiKey: string                   │
└─────────────────┬───────────────────┘
                  │
      ┌───────────┴──────────┐
      ▼                       ▼
┌──────────────┐    ┌──────────────────┐
│ SessionStore │    │ Google Genai SDK │
│ (Metadata)   │    │  (API Client)    │
└──────────────┘    └──────────────────┘
      │                       │
      ▼                       ▼
~/.agm/sessions.json   Gemini 2.0 API
      │
      ▼
~/.agm/gemini/{id}/history.jsonl
```

### Key Design Patterns

- **Adapter Pattern:** Translates Agent interface to Google AI API
- **Repository Pattern:** File-based JSONL storage
- **Factory Pattern:** `NewGeminiAdapter()` with config

## Documentation

### Core Documentation
- [SPEC.md](SPEC.md) - Technical specification and requirements
- [ARCHITECTURE.md](ARCHITECTURE.md) - System architecture and data flow
- [INDEX.md](INDEX.md) - Document index and navigation

### Architecture Decision Records (ADRs)
- [ADR-001](ADR-001-file-based-storage.md) - File-Based Storage with JSONL
- [ADR-002](ADR-002-google-genai-sdk-selection.md) - Google Generative AI SDK Selection
- [ADR-003](ADR-003-full-history-context.md) - Full History Context on Every API Call

## Limitations (V1)

### Known Constraints
1. **No Retry Logic:** API failures return immediately
2. **No Streaming:** Responses returned when complete
3. **No Tool Calling:** Function calling not implemented
4. **No Vision Input:** Image support not implemented
5. **No System Instructions:** Custom prompts not supported
6. **No Context Management:** No automatic truncation at token limit
7. **JSONL Import Only:** Cannot import Markdown/HTML
8. **Session Preservation:** History files kept after termination

### Workarounds

**Long Conversations:**
- Manually start new session when approaching 1M tokens
- Export and archive old conversations

**Context Window Overflow:**
- Monitor conversation length
- Split into multiple sessions if needed

**Session Cleanup:**
- Manually delete `~/.agm/gemini/{session-id}/` directories

## Comparison with Other Adapters

| Feature | Gemini | GPT | Claude |
|---------|--------|-----|--------|
| Storage | File (JSONL) | In-memory (V1) | File (JSONL) |
| Persistence | ✅ Yes | ❌ No (V1) | ✅ Yes |
| Context Window | 1M-2M tokens | 128K tokens | 200K tokens |
| Streaming | ⏳ V2 | ⏳ V2 | ✅ Yes |
| Tools | ⏳ V2 | ⏳ V2 | ✅ Yes |
| Vision | ⏳ V2 | ⏳ V2 | ✅ Yes |

## Troubleshooting

### Common Issues

**"GEMINI_API_KEY not set"**
```bash
# Solution: Export API key
export GEMINI_API_KEY="your-key-here"
```

**"Session not found"**
```bash
# Verify session exists
ls ~/.agm/gemini/

# Check sessions.json
cat ~/.agm/sessions.json
```

**"Failed to send message to Gemini"**
- Check API key is valid
- Verify network connectivity
- Check for API service outages
- Review error message for details

**Long API response times**
- Check conversation length (very long histories slow down API)
- Consider starting new session
- Monitor network latency

## Contributing

### Adding New Features

1. Update SPEC.md with requirements
2. Add ADR for significant decisions
3. Implement feature with tests
4. Update ARCHITECTURE.md
5. Update this README

### Testing Guidelines

- All new code must have unit tests
- Integration tests optional but recommended
- Maintain >90% code coverage
- Test error paths

## References

### External
- [Google Generative AI Go SDK](https://pkg.go.dev/github.com/google/generative-ai-go/genai)
- [Google AI API Documentation](https://ai.google.dev/docs)
- [Gemini Models Overview](https://ai.google.dev/models/gemini)

### Internal
- [Agent Interface](../interface.go)
- [Session Store](../session_store.go)
- [GPT Adapter](../gpt/) - Comparison implementation
- [Claude Adapter](../claude/) - File-based storage reference

## License

Part of AGM (AI/Agent Gateway Manager)
Copyright 2026 - See main project LICENSE
