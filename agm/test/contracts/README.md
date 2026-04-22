# Pact Contract Tests

This directory contains Pact contract tests for AGM's AI provider adapters (Claude, Gemini, GPT).

## Overview

Pact tests define consumer-provider contracts between AGM and external AI APIs:

| Consumer           | Provider    | Contract File                    |
|--------------------|-------------|----------------------------------|
| agm-claude-client  | claude-api  | claude_adapter_pact_test.go      |
| agm-gemini-client  | gemini-api  | gemini_adapter_pact_test.go      |
| agm-gpt-client     | openai-api  | gpt_adapter_pact_test.go         |

## Requirements

### Pact FFI Library

Pact tests require the Pact FFI library to be installed:

```bash
# Install pact-go CLI
go install github.com/pact-foundation/pact-go/v2@latest

# Download and install Pact FFI library
pact-go -l DEBUG install
```

This will install the `libpact_ffi` library to the default location.

### API Keys (Optional)

While Pact tests use mock servers and don't require real API keys, you can run integration tests with:

```bash
export ANTHROPIC_API_KEY=sk-ant-...
export GOOGLE_API_KEY=...
export OPENAI_API_KEY=sk-...
```

## Running Tests

### Run All Pact Tests

```bash
go test -tags=contract ./test/contracts/...
```

### Run Specific Provider Tests

```bash
# Claude contracts only
go test -tags=contract ./test/contracts -run TestClaudeAdapterPact

# Gemini contracts only
go test -tags=contract ./test/contracts -run TestGeminiAdapterPact

# GPT contracts only
go test -tags=contract ./test/contracts -run TestGPTAdapterPact
```

### Run with Verbose Output

```bash
go test -tags=contract ./test/contracts/... -v
```

## Contract Coverage

### Claude Adapter (claude_adapter_pact_test.go)

Tests interactions with Anthropic's Claude API:

1. **Create session** - POST /v1/messages with initial context
   - Headers: `anthropic-version`, `x-api-key`, `Content-Type`
   - Request: model, messages array, max_tokens
   - Response: id, type, role, content, usage

2. **Send message** - POST /v1/messages with conversation history
   - Same structure as create session
   - Validates message-response cycle

3. **Stream response** - POST /v1/messages with stream=true
   - Headers: `text/event-stream`, `Cache-Control`, `Connection`
   - Body: Server-Sent Events (SSE) format

### Gemini Adapter (gemini_adapter_pact_test.go)

Tests interactions with Google's Gemini API:

1. **Create session** - POST /v1beta/models/gemini-2.0-flash-exp:generateContent
   - Query params: `key`
   - Request: contents array, generationConfig
   - Response: candidates, usageMetadata

2. **Send message** - POST /v1beta/models/gemini-2.0-flash-exp:generateContent
   - Same structure as create session
   - Validates multi-turn conversations

3. **Stream response** - POST /v1beta/models/gemini-2.0-flash-exp:streamGenerateContent
   - Query params: `key`, `alt=sse`
   - Response: SSE format with partial candidates

### GPT Adapter (gpt_adapter_pact_test.go)

Tests interactions with OpenAI's GPT API:

1. **Create session** - POST /v1/chat/completions
   - Header: `Authorization: Bearer sk-...`
   - Request: model, messages, max_tokens, temperature
   - Response: id, object, choices, usage

2. **Send message** - POST /v1/chat/completions
   - Supports multi-message history
   - Response includes finish_reason

3. **Stream response** - POST /v1/chat/completions with stream=true
   - Response: SSE format with chunked choices
   - Delta objects contain incremental content

## Generated Pact Files

After successful test execution, Pact JSON files are generated in `./pacts/`:

- `agm-claude-client-claude-api.json`
- `agm-gemini-client-gemini-api.json`
- `agm-gpt-client-openai-api.json`

These files can be:
- Shared with provider teams for verification
- Published to a Pact Broker
- Used in CI/CD pipelines

## Matchers

Tests use Pact matchers for flexible contract validation:

| Matcher               | Purpose                                    | Example                                      |
|-----------------------|--------------------------------------------|----------------------------------------------|
| `matchers.String()`   | Match any string                           | `matchers.String("example")`                 |
| `matchers.Integer()`  | Match any integer                          | `matchers.Integer(1024)`                     |
| `matchers.Decimal()`  | Match any decimal number                   | `matchers.Decimal(0.7)`                      |
| `matchers.Regex()`    | Match string against regex pattern         | `matchers.Regex("sk-ant-.*", "sk-ant-key")`  |
| `matchers.EachLike()` | Match array with at least N items          | `matchers.EachLike(map[...], 1)`             |
| `matchers.Like()`     | Match type, ignore value                   | `matchers.Like(true)`                        |

## Troubleshooting

### Error: "cannot find -lpact_ffi"

**Cause**: Pact FFI library not installed

**Solution**: Run `pact-go -l DEBUG install` to download and install the library

### Tests Compile But Don't Run

**Cause**: Build tag not specified

**Solution**: Always use `-tags=contract` flag:
```bash
go test -tags=contract ./test/contracts/...
```

### Mock Server Port Conflicts

**Cause**: Previous mock server still running

**Solution**: Kill existing pact processes:
```bash
pkill -f pact
```

## CI/CD Integration

Example GitHub Actions workflow:

```yaml
name: Pact Contract Tests
on: [push, pull_request]

jobs:
  contract:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - uses: actions/setup-go@v4
        with:
          go-version: '1.24'

      - name: Install Pact CLI
        run: |
          go install github.com/pact-foundation/pact-go/v2@latest
          pact-go -l DEBUG install

      - name: Run Pact Tests
        run: go test -tags=contract ./test/contracts/... -v

      - name: Publish Pacts
        if: github.ref == 'refs/heads/main'
        run: |
          pact-go publish \
            --pact-dir ./test/contracts/pacts \
            --consumer-version $GITHUB_SHA \
            --broker-base-url https://your-pact-broker.example.com
```

## Best Practices

1. **Keep contracts focused** - Test specific interactions, not entire workflows
2. **Use matchers appropriately** - Match types, not exact values (except where needed)
3. **Document provider states** - Use clear "Given" clauses
4. **Version contracts** - Tag pacts with consumer version for tracking
5. **Validate both ways** - Consumers test requests, providers verify pacts

## References

- [Pact Go Documentation](https://docs.pact.io/implementation_guides/go/readme)
- [Consumer Tests Guide](https://docs.pact.io/implementation_guides/go/docs/consumer)
- [Pact Go Examples](https://github.com/pact-foundation/pact-workshop-go)
- [Matchers Reference](https://pkg.go.dev/github.com/pact-foundation/pact-go/v2/matchers)
