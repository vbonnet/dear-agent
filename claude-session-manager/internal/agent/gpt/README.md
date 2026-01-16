# GPT Adapter

OpenAI GPT adapter implementation for the Agent interface.

## Overview

This package provides a full implementation of the `agent.Agent` interface for OpenAI's GPT API. It enables CSM (Claude Session Manager) users to use GPT-4 as an alternative agent.

**Status**: V1 Complete (all 12 interface methods implemented)

## Setup

### Prerequisites

1. **OpenAI API Key**: Obtain from https://platform.openai.com/api-keys
2. **Go 1.24+**: Required for this project

### Installation

The GPT adapter is part of the `ai-tools` repository. The OpenAI SDK dependency is already included in `go.mod`.

### Configuration

Set your OpenAI API key as an environment variable:

```bash
export OPENAI_API_KEY="sk-your-api-key-here"
```

**Security**: Never commit API keys to version control. Use environment variables only.

## Usage

### Basic Example

```go
package main

import (
    "fmt"
    "log"

    "github.com/user/ai-tools/agm/internal/agent"
    "github.com/user/ai-tools/agm/internal/agent/gpt"
)

func main() {
    // Create adapter
    adapter, err := gpt.NewAdapter()
    if err != nil {
        log.Fatal(err) // OPENAI_API_KEY not set
    }

    // Create session
    ctx := agent.SessionContext{
        Name:             "my-session",
        WorkingDirectory: "/home/user/project",
    }
    sessionID, err := adapter.CreateSession(ctx)
    if err != nil {
        log.Fatal(err)
    }

    // Send message
    err = adapter.SendMessage(sessionID, agent.Message{
        Role:    agent.RoleUser,
        Content: "Explain recursion in one sentence.",
    })
    if err != nil {
        log.Fatal(err)
    }

    // Get response
    history, err := adapter.GetHistory(sessionID)
    if err != nil {
        log.Fatal(err)
    }

    fmt.Println(history[1].Content) // Assistant's response
}
```

### Export/Import Conversations

```go
// Export to JSONL
data, err := adapter.ExportConversation(sessionID, agent.FormatJSONL)
if err != nil {
    log.Fatal(err)
}

// Save to file
os.WriteFile("conversation.jsonl", data, 0644)

// Import from JSONL
jsonlData, _ := os.ReadFile("conversation.jsonl")
newSessionID, err := adapter.ImportConversation(jsonlData, agent.FormatJSONL)
```

## Features

### V1 Implementation

- ✅ All 12 Agent interface methods implemented
- ✅ In-memory session storage (thread-safe)
- ✅ Conversation history management
- ✅ JSONL and Markdown export formats
- ✅ Error handling with exponential backoff (rate limits)
- ✅ 30-second API timeout
- ✅ Command execution (rename, setdir)

### Capabilities

```go
caps := adapter.Capabilities()

// caps.SupportsSlashCommands: false  (API agent, not CLI)
// caps.SupportsTools:         true   (GPT-4 supports function calling)
// caps.SupportsVision:        true   (GPT-4V capable)
// caps.MaxContextWindow:      128000 (gpt-4-turbo: 128K tokens)
// caps.ModelName:             "gpt-4-turbo"
```

### Error Handling

The adapter handles common OpenAI API errors:

- **401 (Authentication)**: Returns immediate error with clear message
- **429 (Rate Limit)**: Retries with exponential backoff (1s, 2s, 4s, 8s, 16s)
- **Timeout**: 30-second default timeout per API call
- **Max Retries**: 5 attempts before failing

## Testing

### Unit Tests (Mocked, No API Key Required)

```bash
cd ~/src/ws/oss/repos/ai-tools/main/claude-session-manager
go test ./internal/agent/gpt -v
```

**Coverage**: >90% (12+ test scenarios)

### Integration Tests (Live API, Requires API Key)

```bash
export OPENAI_API_KEY="sk-your-key"
export INTEGRATION_TESTS=true
go test ./internal/agent/gpt -v
```

**Note**: Integration tests make real API calls and incur costs. Use sparingly.

### Race Condition Testing

```bash
go test ./internal/agent/gpt -race
```

All public methods are thread-safe (validated with `sync.RWMutex`).

## Limitations (V1)

1. **No Persistence**: Sessions are stored in-memory only. Restarting the process loses all sessions.
2. **No Streaming**: Responses are returned only when complete (no real-time streaming).
3. **No Tool Calling**: V1 does not implement GPT-4's function calling feature.
4. **No Vision Input**: V1 does not support image inputs (GPT-4V capability).
5. **Basic Context Management**: No automatic truncation when conversation exceeds 128K token limit.

## V2 Roadmap

Planned enhancements:

- File-based session persistence (JSONL storage)
- Streaming response support
- Tool/function calling implementation
- Vision input handling (GPT-4V)
- Automatic context window management
- Token usage tracking and cost estimation

## Architecture

### File Structure

```
internal/agent/gpt/
├── adapter.go       # Main adapter (12 Agent methods)
├── session.go       # Session data structure
├── translator.go    # Message translation (agent ↔ OpenAI)
├── errors.go        # Error types and constants
├── adapter_test.go  # Test suite
└── README.md        # This file
```

### Thread Safety

All public methods use `sync.RWMutex`:

- **Read operations** (RLock): GetHistory, GetSessionStatus, ResumeSession
- **Write operations** (Lock): CreateSession, TerminateSession, SendMessage

### API Integration

Uses official OpenAI SDK:

```go
import (
    "github.com/openai/openai-go"
    "github.com/openai/openai-go/option"
)
```

**SDK Version**: v0.1.0-alpha.33 (pinned in go.mod)

## Troubleshooting

### "OPENAI_API_KEY environment variable not set"

**Cause**: API key not configured
**Solution**: Run `export OPENAI_API_KEY="sk-your-key"`

### "authentication failed (HTTP 401)"

**Cause**: Invalid or expired API key
**Solution**: Check your API key at https://platform.openai.com/api-keys

### "maximum retries exceeded"

**Cause**: OpenAI API is rate limiting or unavailable
**Solution**: Wait and retry. Check OpenAI status at https://status.openai.com/

### Race condition warnings

**Cause**: Concurrent access to non-thread-safe code
**Solution**: Report as bug (all methods should be thread-safe)

## Contributing

When modifying the GPT adapter:

1. **Maintain Interface Compliance**: Ensure `var _ agent.Agent = (*Adapter)(nil)` still compiles
2. **Add Tests**: All new features must have test coverage
3. **Thread Safety**: Use mutex for all session map access
4. **Document Changes**: Update README and godoc

## License

Same as parent repository (ai-tools).

## Support

For issues related to:

- **GPT Adapter**: File issue in ai-tools repository
- **OpenAI API**: See https://platform.openai.com/docs
- **Agent Interface**: See `internal/agent/interface.go`

## References

- [Agent Interface Definition](../interface.go)
- [Gemini Adapter](../gemini/adapter.go) (reference implementation)
- [OpenAI API Documentation](https://platform.openai.com/docs)
- [OpenAI Go SDK](https://pkg.go.dev/github.com/openai/openai-go)
