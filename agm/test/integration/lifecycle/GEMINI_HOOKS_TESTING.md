# Gemini CLI Hook Integration Tests

This document describes the integration tests for Gemini CLI hook execution via AGM (Agent Gateway Manager).

## Overview

The Gemini CLI hook integration tests verify that:

1. **SessionStart hooks execute** when a Gemini session is created via AGM
2. **SessionEnd hooks execute** when a Gemini session terminates
3. **Hook context is passed correctly** (session_id, cwd, project, etc.)
4. **Hook blocking works** (exit code 2 blocks operations)
5. **Hook timeouts are enforced** (configured timeout in settings.json)
6. **Multiple hooks execute in order** (when configured)

## Test Files

- `gemini_hooks_test.go` - Main integration test file
- `run-gemini-hooks-tests.sh` - Test runner script
- `GEMINI_HOOKS_TESTING.md` - This documentation

## Prerequisites

### Required
- **Go 1.21+** - Go programming language
- **tmux** - Terminal multiplexer for session management
- **Gemini CLI** - Google's Gemini CLI tool (`gemini` command)
- **AGM binary** - Built from `cmd/agm/main.go`

### Optional
- **jq** - JSON processor (recommended for hook JSON parsing)

### Installation

```bash
# Install Gemini CLI (if not already installed)
# See: https://github.com/google/gemini-cli

# Install jq (for JSON parsing in hooks)
brew install jq          # macOS
apt-get install jq       # Debian/Ubuntu
dnf install jq           # Fedora

# Build AGM binary
cd agm
make build
make install
```

## Running Tests

### Quick Start

```bash
# Run all Gemini hook tests
./test/integration/lifecycle/run-gemini-hooks-tests.sh

# Run with verbose output
./test/integration/lifecycle/run-gemini-hooks-tests.sh -v

# Skip integration tests (short mode)
./test/integration/lifecycle/run-gemini-hooks-tests.sh -short
```

### Manual Test Execution

```bash
# From agm directory
cd agm

# Run all Gemini hook tests
go test -tags=integration -v ./test/integration/lifecycle -run TestGeminiHooks

# Run specific test
go test -tags=integration -v ./test/integration/lifecycle -run TestGeminiHooks_SessionStartExecution

# Run with timeout
go test -tags=integration -v -timeout 5m ./test/integration/lifecycle -run TestGeminiHooks
```

## Test Cases

### 1. TestGeminiHooks_SessionStartExecution

**Objective**: Verify SessionStart hook executes when Gemini session is created.

**Steps**:
1. Create test hook script that writes to `/tmp/hook-executed`
2. Configure hook in test `settings.json`
3. Create Gemini session via `GeminiCLIAdapter.CreateSession()`
4. Manually trigger hook via `adapter.RunHook(sessionID, "SessionStart")`
5. Verify hook marker file exists
6. Verify marker contains session context (session_id, cwd, etc.)

**Expected Result**: Hook executes and writes marker file with session context.

### 2. TestGeminiHooks_SessionEndExecution

**Objective**: Verify SessionEnd hook executes when Gemini session terminates.

**Steps**:
1. Create SessionEnd test hook
2. Configure in `settings.json`
3. Create Gemini session
4. Trigger SessionEnd hook via `adapter.RunHook(sessionID, "SessionEnd")`
5. Verify marker file exists

**Expected Result**: SessionEnd hook executes successfully.

### 3. TestGeminiHooks_BlockingWithExitCode2

**Objective**: Verify hooks can block operations by exiting with code 2.

**Steps**:
1. Create hook that exits with code 2
2. Execute hook directly
3. Verify exit code is 2

**Expected Result**: Hook exits with code 2 (blocking signal).

**Note**: Actual blocking behavior depends on Gemini CLI implementation.

### 4. TestGeminiHooks_TimeoutHandling

**Objective**: Verify hook timeout configuration is respected.

**Steps**:
1. Create slow hook (sleeps 10 seconds)
2. Configure with 1-second timeout in `settings.json`
3. Verify timeout configuration in parsed settings

**Expected Result**: Timeout is configured correctly.

**Note**: Actual timeout enforcement depends on Gemini CLI.

### 5. TestGeminiHooks_MultipleHooksExecution

**Objective**: Verify multiple hooks can be configured and executed.

**Steps**:
1. Create 3 test hooks
2. Configure all 3 in `settings.json` SessionStart array
3. Verify all hooks are present in settings

**Expected Result**: All 3 hooks are configured.

**Note**: Execution order depends on Gemini CLI implementation.

## Test Hook Format

### SessionStart Hook

**Input** (stdin JSON):
```json
{
  "session_id": "uuid-string",
  "transcript_path": "/path/to/transcript",
  "cwd": "/current/working/directory",
  "hook_event_name": "SessionStart",
  "timestamp": "2024-01-01T12:00:00Z",
  "source": "startup"
}
```

**Output** (stdout JSON):
```json
{
  "systemMessage": "Optional system message",
  "suppressOutput": false,
  "hookSpecificOutput": {
    "additionalContext": "Optional context"
  }
}
```

**Exit Codes**:
- `0` - Success (continue)
- `1` - Error (log warning, continue)
- `2` - Block operation (abort session creation)

### SessionEnd Hook

**Input** (stdin JSON):
```json
{
  "session_id": "uuid-string",
  "transcript_path": "/path/to/transcript",
  "cwd": "/current/working/directory",
  "hook_event_name": "SessionEnd",
  "timestamp": "2024-01-01T12:00:00Z"
}
```

**Output**: None (fire-and-forget)

**Exit Codes**: Ignored (SessionEnd hooks are non-blocking)

## Test Infrastructure

### TestEnv

`test/integration/helpers/test_env.go` provides test environment setup:

```go
env := helpers.NewTestEnv(t)
defer env.Cleanup(t)

// Use env.TempDir for temporary files
markerFile := filepath.Join(env.TempDir, "hook-marker.txt")

// Generate unique session name
sessionName := "test-session-" + helpers.RandomString(6)
```

### GeminiCLIAdapter

`internal/agent/gemini_cli_adapter.go` provides Gemini CLI integration:

```go
// Create adapter with test session store
store, _ := agent.NewJSONSessionStore(tempStorePath)
adapter, _ := agent.NewGeminiCLIAdapter(store)

// Create session
ctx := agent.SessionContext{
    Name:             "test-session",
    WorkingDirectory: "/tmp/test",
    Project:          "test-project",
}
sessionID, _ := adapter.CreateSession(ctx)

// Trigger hook manually
adapter.RunHook(sessionID, "SessionStart")
```

## Configuration

### Test settings.json

Tests create isolated `settings.json` files in `env.TempDir`:

```json
{
  "hooks": {
    "SessionStart": [
      {
        "command": "/path/to/hook.sh",
        "description": "Test hook",
        "timeout": 5000
      }
    ],
    "SessionEnd": [
      {
        "command": "/path/to/sessionend-hook.sh",
        "description": "SessionEnd test hook",
        "timeout": 5000
      }
    ]
  }
}
```

### Environment Variables

Tests use `GEMINI_CONFIG_DIR` to isolate configuration:

```go
os.Setenv("GEMINI_CONFIG_DIR", geminiConfigDir)
defer os.Unsetenv("GEMINI_CONFIG_DIR")
```

## Troubleshooting

### Test Skips

**"Gemini CLI not installed"**
- Install Gemini CLI: https://github.com/google/gemini-cli
- Ensure `gemini` is in PATH

**"Tmux not available"**
- Install tmux: `brew install tmux` / `apt-get install tmux`

### Hook Execution Issues

**Marker file not created**
- Check hook script permissions (`chmod +x hook.sh`)
- Verify hook script path in `settings.json`
- Check stderr for hook errors
- Verify `GEMINI_CONFIG_DIR` is set correctly

**Hook timeout**
- Increase timeout in `settings.json` (default: 5000ms)
- Check if hook script is hanging
- Add logging to hook script for debugging

### Integration Test Failures

**"Session not found"**
- Verify `GeminiCLIAdapter` creates session correctly
- Check session store path (`tempStore`)
- Ensure session store is writable

**"Hook context file not created"**
- Check `~/.agm/gemini-hooks/` directory exists
- Verify `executeHook()` writes context file
- Check disk permissions

## Limitations

1. **Gemini CLI required**: Tests skip if `gemini` command not found
2. **Hook implementation**: Actual hook execution depends on Gemini CLI internals
3. **Timeout enforcement**: Timeout behavior depends on Gemini CLI
4. **Blocking semantics**: Exit code 2 blocking depends on Gemini CLI

## Future Work

- [ ] Test actual hook execution via Gemini CLI (not just manual RunHook)
- [ ] Verify hook output is parsed and used by Gemini CLI
- [ ] Test hook error handling (malformed JSON output)
- [ ] Test hook environment variables (AGM_SESSION_NAME, etc.)
- [ ] Integration with actual Gemini CLI lifecycle events
- [ ] Test BeforeAgent and AfterAgent hooks

## References

- **Phase 2 Task 2.4**: Add Hook Integration Tests (Bead: oss-o7g)
- **Phase 2 Task 2.1**: Gemini Hook Wrappers (`./worktrees/engram/phase2-hooks-wrappers/`)
- **Phase 2 Task 2.3**: RunHook Support (commit 0a6c84e)
- **Gemini settings.json**: `~/.gemini/settings.json`

## Related Files

- `gemini_hooks_test.go` - Test implementation
- `internal/agent/gemini_cli_adapter.go` - GeminiCLIAdapter with RunHook support
- `internal/agent/gemini_cli_adapter_test.go` - Unit tests for hook execution
- `test/integration/lifecycle/hook_execution_test.go` - Claude hook tests (reference)
