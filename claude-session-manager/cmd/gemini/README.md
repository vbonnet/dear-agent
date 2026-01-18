# Gemini CLI Tool

A command-line interface for interacting with Google's Gemini API.

## Installation

```bash
# Build the binary
cd ~/src/ws/oss/repos/ai-tools/main/claude-session-manager
make build

# Install to $HOME/go/bin
make install
```

Ensure `$HOME/go/bin` is in your PATH.

## Setup

### 1. Get API Key

Get your Gemini API key from Google AI Studio:
https://makersuite.google.com/app/apikey

### 2. Set Environment Variable

```bash
export GOOGLE_API_KEY="your-api-key-here"
```

Add this to your `~/.bashrc` or `~/.zshrc` to make it permanent.

## Usage

### Create a Session

```bash
gemini create my-session
```

Output:
```
✓ Created Gemini session: my-session
  Model: gemini-pro
  Ready to send messages with: gemini send my-session "your message"
```

### Send a Message

```bash
gemini send my-session "What is the capital of France?"
```

Output:
```
Assistant: The capital of France is Paris.
```

### Check History

```bash
gemini history my-session
```

Output (V1 limitation):
```
ℹ History not available (V1 limitation)

Each 'gemini send' invocation is independent in V1.
Conversation history is not persisted between CLI invocations.

V2 will add session persistence and history retrieval.
```

## Commands Reference

### `gemini create <session-name>`

Creates a new Gemini chat session and validates API connectivity.

**Arguments:**
- `session-name`: Name for the session (required)

**Example:**
```bash
gemini create my-project
```

### `gemini send <session-name> <message>`

Sends a message to Gemini and prints the response.

**Arguments:**
- `session-name`: Session identifier (required)
- `message`: Your message to Gemini (required, use quotes for multi-word messages)

**Examples:**
```bash
gemini send my-project "Explain quantum computing"
gemini send my-project "Write a haiku about coding"
```

### `gemini history <session-name>`

Displays conversation history (V1: shows limitation message).

**Arguments:**
- `session-name`: Session identifier (required)

**Example:**
```bash
gemini history my-project
```

## V1 Limitations

The current version (V1) has the following limitations:

1. **No Session Persistence**: Sessions are not saved between CLI invocations
2. **No Conversation Context**: Each `gemini send` is independent with no history
3. **No Configuration File**: All settings are hardcoded or from env vars
4. **Model Selection**: Only `gemini-pro` (text) model is supported
5. **No Streaming**: Responses are returned all at once (no streaming)

These limitations will be addressed in V2.

## Environment Variables

| Variable | Required | Description |
|----------|----------|-------------|
| `GOOGLE_API_KEY` | Yes | Your Gemini API key from Google AI Studio |

## Error Handling

The CLI provides clear error messages:

```bash
# Missing API key
$ unset GOOGLE_API_KEY
$ gemini create test
Error: GOOGLE_API_KEY environment variable not set

Get API key at: https://makersuite.google.com/app/apikey

# Network error
$ gemini create test
Error: Failed to connect to Gemini API. Check your network connection.

# Empty message
$ gemini send test ""
Error: message cannot be empty
```

**Exit Codes:**
- `0`: Success
- `1`: User error (missing arguments, invalid input)
- `2`: API error (network failure, authentication failure)

## Testing

### Unit Tests

```bash
# Run all unit tests
go test ./internal/gemini/...

# With coverage
go test -cover ./internal/gemini/...
```

### Integration Tests

Integration tests require a valid API key and make real API calls.

```bash
# Set API key
export GOOGLE_API_KEY="your-api-key"

# Run integration tests
go test -tags=integration ./tests/
```

## Development

### Project Structure

```
cmd/gemini/
  main.go              # CLI entry point, Cobra commands
  README.md            # This file

internal/gemini/
  client.go            # GeminiClient interface and RealClient implementation
  client_mock.go       # MockClient for unit testing
  client_test.go       # Unit tests for client
  errors.go            # UserError and APIError types
  errors_test.go       # Unit tests for errors

tests/
  gemini_integration_test.go  # Integration tests with real API calls
```

### Building

```bash
# Build for current platform
make build

# Build for all platforms
GOOS=linux GOARCH=amd64 go build -o bin/gemini-linux-amd64 ./cmd/gemini
GOOS=darwin GOARCH=amd64 go build -o bin/gemini-darwin-amd64 ./cmd/gemini
GOOS=darwin GOARCH=arm64 go build -o bin/gemini-darwin-arm64 ./cmd/gemini
GOOS=windows GOARCH=amd64 go build -o bin/gemini-windows-amd64.exe ./cmd/gemini
```

## Troubleshooting

### API Key Issues

**Problem**: `Error: GOOGLE_API_KEY environment variable not set`

**Solution**: Set the environment variable:
```bash
export GOOGLE_API_KEY="your-api-key-here"
```

Verify it's set:
```bash
echo $GOOGLE_API_KEY
```

### Connection Issues

**Problem**: `Error: Failed to connect to Gemini API`

**Possible causes:**
1. No internet connection
2. Firewall blocking Google API endpoints
3. Invalid API key

**Solutions:**
- Check internet connection
- Verify API key is correct
- Try accessing https://generativelanguage.googleapis.com in browser

### Rate Limiting

**Problem**: API returns rate limit errors

**Solution**: Wait for the rate limit window to reset (typically 60 seconds). Consider spacing out requests in automated scripts.

## V2 Roadmap

Planned features for V2:

- [ ] Session persistence (save conversations locally)
- [ ] History retrieval (view past conversations)
- [ ] Interactive chat mode (REPL)
- [ ] Configuration file (~/.config/gemini/config.yaml)
- [ ] Model selection (gemini-pro, gemini-pro-vision)
- [ ] Streaming responses
- [ ] Multimodal inputs (images, audio)
- [ ] Function calling / tool use

## License

See repository root LICENSE file.

## Contributing

This is part of the ai-tools repository. For contributions, see the main repository documentation.
