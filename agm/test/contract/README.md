# Contract Tests

Contract tests verify end-to-end workflows with real AI CLI tools (Claude, Gemini).

## Overview

Contract tests differ from unit and integration tests:

| Test Type | Dependencies | Speed | API Calls | Build Tag |
|-----------|-------------|-------|-----------|-----------|
| Unit | None | <1s | 0 | None |
| Integration | Mocks | <5s | 0 | `integration` |
| Contract | Real APIs | <30s | <20 | `contract` |

## Requirements

### API Keys

Contract tests require real API keys:

```bash
export ANTHROPIC_API_KEY=sk-ant-...
export GOOGLE_API_KEY=...
```

Tests gracefully skip if keys are missing.

### Dependencies

- **tmux**: Required for session management
- **agm**: AGM binary must be in PATH

### Quota Limits

- Maximum 20 API calls per test run
- Tests consume quota and skip when exhausted
- Quota is shared across all contract tests
- Each test consumes 1-2 API calls

## Running Tests

### Run All Contract Tests

```bash
# Requires API keys
ANTHROPIC_API_KEY=sk-... GOOGLE_API_KEY=... \
  go test -tags=contract ./test/contract/...
```

### Run Specific Contract Test

```bash
# Claude contract tests only
ANTHROPIC_API_KEY=sk-... \
  go test -tags=contract ./test/contract -run TestClaudeAPI

# Gemini contract tests only
GOOGLE_API_KEY=... \
  go test -tags=contract ./test/contract -run TestGeminiAPI
```

### Run with Verbose Output

```bash
ANTHROPIC_API_KEY=sk-... \
  go test -tags=contract ./test/contract -v
```

## Test Coverage

### Claude Contract Tests

- `TestClaudeAPI_SessionCreation`: Create session with Claude agent
- `TestClaudeAPI_BasicPrompt`: Send prompt and verify response
- `TestClaudeAPI_SessionArchive`: Archive session lifecycle

### Gemini Contract Tests

- `TestGeminiAPI_SessionCreation`: Create session with Gemini agent
- `TestGeminiAPI_BasicPrompt`: Send prompt and verify response
- `TestGeminiAPI_SessionArchive`: Archive session lifecycle
- `TestGeminiAPI_AgentParity`: Verify feature parity with Claude

## Quota Management

Contract tests use `helpers.GetAPIQuota()` to track API usage:

```go
func TestExample(t *testing.T) {
    quota := helpers.GetAPIQuota()
    if !quota.Consume() {
        t.Skip("API quota exhausted")
    }
    // ... test code that calls API ...
}
```

Quota features:
- **Global quota**: Shared across all tests (20 calls)
- **Thread-safe**: Safe for parallel test execution
- **Graceful degradation**: Tests skip when quota exhausted

## CI/CD Integration

Contract tests run only on main branch (not every PR):

```yaml
# .github/workflows/contract-tests.yml
name: Contract Tests
on:
  push:
    branches: [main]
  schedule:
    - cron: '0 0 * * 0'  # Weekly

jobs:
  contract:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - uses: actions/setup-go@v4
      - name: Run Contract Tests
        env:
          ANTHROPIC_API_KEY: ${{ secrets.ANTHROPIC_API_KEY }}
          GOOGLE_API_KEY: ${{ secrets.GOOGLE_API_KEY }}
        run: go test -tags=contract ./test/contract/...
```

## Troubleshooting

### Tests Skip with "API key not set"

**Cause**: Environment variable not set

**Solution**:
```bash
export ANTHROPIC_API_KEY=sk-ant-...
export GOOGLE_API_KEY=...
```

### Tests Skip with "API quota exhausted"

**Cause**: More than 20 tests ran in single execution

**Solution**: Run fewer tests or reset quota:
```bash
# Run subset of tests
go test -tags=contract ./test/contract -run TestClaudeAPI_SessionCreation
```

### Tests Fail with "agm: command not found"

**Cause**: AGM binary not in PATH

**Solution**:
```bash
# Install AGM
go install github.com/vbonnet/dear-agent/agm/cmd/agm@latest

# Or add to PATH
export PATH=$PATH:$(go env GOPATH)/bin
```

### Tests Fail with "tmux: command not found"

**Cause**: tmux not installed

**Solution**:
```bash
# Ubuntu/Debian
sudo apt-get install tmux

# macOS
brew install tmux
```

## Writing New Contract Tests

### Template

```go
// +build contract

package contract

import (
    "os"
    "testing"
    "github.com/vbonnet/dear-agent/agm/test/helpers"
)

func TestNewContractTest(t *testing.T) {
    // 1. Check API key
    if os.Getenv("ANTHROPIC_API_KEY") == "" {
        t.Skip("ANTHROPIC_API_KEY not set")
    }

    // 2. Consume quota
    quota := helpers.GetAPIQuota()
    if !quota.Consume() {
        t.Skip("API quota exhausted")
    }

    // 3. Run CLI command
    result := helpers.RunCLI(t, "command", "args...")

    // 4. Verify result
    if result.ExitCode != 0 {
        t.Fatalf("Command failed: %s", result.Stderr)
    }
}
```

### Best Practices

1. **Always check API key availability** before consuming quota
2. **Consume quota** before making API calls
3. **Use helpers.RunCLI()** for isolated execution
4. **Skip gracefully** on missing keys or exhausted quota
5. **Keep tests fast** (<30s total runtime)
6. **Limit API calls** (max 2 per test)
7. **Clean up resources** (archive/delete test sessions)

## References

- [Test Helpers](../helpers/README.md)
- [Quota Management](../helpers/quota.go)
- [CLI Execution](../helpers/cli.go)
