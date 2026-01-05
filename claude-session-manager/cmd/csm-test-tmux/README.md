# csm-test-tmux

Automated CSM (Claude Session Manager) testing in isolated tmux sessions.

## Overview

`csm-test-tmux` creates isolated test environments for Claude Code development by:
- Running detached tmux sessions with independent CSM state
- Enabling parallel test execution without session pollution
- Automating command injection and output capture
- Supporting both AI agents and human developers

**Use Cases**:
- Automated CSM testing workflows
- Interactive CSM debugging sessions
- Parallel feature development without conflicts
- CI/CD integration for CSM tests

## Quick Start

```bash
# Build the tool
cd ~/src/ws/oss/repos/ai-tools/base/claude-session-manager
go build -o ~/bin/csm-test-tmux ./cmd/csm-test-tmux

# Create a test session
csm-test-tmux create my-test

# Send a command
csm-test-tmux send my-test "git status"

# Capture output
csm-test-tmux capture my-test --lines 20

# Cleanup when done
csm-test-tmux cleanup my-test
```

## Installation

### From Source

```bash
# Clone the repository
git clone https://github.com/vbonnet/ai-tools
cd ai-tools/base/claude-session-manager

# Build and install
go build -o csm-test-tmux ./cmd/csm-test-tmux
mv csm-test-tmux ~/bin/  # or /usr/local/bin
```

### Prerequisites

- **tmux**: Required for session isolation (`tmux -V`)
- **claude**: Claude Code CLI must be in PATH (`which claude`)
- **Go 1.21+**: For building from source

## Commands

### create

Create a new isolated test session.

```bash
# Basic usage
csm-test-tmux create my-test

# With custom working directory
csm-test-tmux create my-test --working-dir ~/myproject

# With longer startup timeout (for slow machines)
csm-test-tmux create my-test --startup-timeout 60

# Get JSON output for AI agents
csm-test-tmux create my-test --format json
```

**Options**:
- `--working-dir`: Working directory (default: current directory)
- `--sessions-dir`: CSM state directory (default: `/tmp/csm-test-<name>`)
- `--startup-timeout`: Timeout in seconds (default: 30)
- `--format`: Output format: text|json (default: text)

**JSON Output**:
```json
{
  "name": "my-test",
  "tmux_session": "csm-test-my-test",
  "sessions_dir": "/tmp/csm-test-my-test",
  "working_dir": "/home/user/myproject",
  "created_at": "2026-01-05T12:00:00Z",
  "startup_time_ms": 1250
}
```

### send

Send a command to the test session.

```bash
# Send a regular command
csm-test-tmux send my-test "git status"

# Send a slash command with autocomplete
csm-test-tmux send my-test "/commit" --autocomplete

# Custom delay for autocomplete (milliseconds)
csm-test-tmux send my-test "/help" --autocomplete --delay 200
```

**Options**:
- `--autocomplete`: Send additional Enter after delay (for slash commands)
- `--delay`: Delay in milliseconds before autocomplete (default: 100)
- `--sessions-dir`: CSM state directory
- `--format`: Output format

**Use Cases**:
- Testing CSM commands programmatically
- Automating Claude interactions
- Injecting test inputs

### capture

Capture output from the test session.

```bash
# Capture last 10 lines (default)
csm-test-tmux capture my-test

# Capture last 50 lines
csm-test-tmux capture my-test --lines 50

# Get JSON array of lines
csm-test-tmux capture my-test --format json
```

**Options**:
- `--lines`: Number of lines to capture (default: 10)
- `--sessions-dir`: CSM state directory
- `--format`: Output format

**Text Output**: Plain lines (one per line)
**JSON Output**:
```json
{
  "lines": ["Claude>", "git status", "..."],
  "count": 3
}
```

### cleanup

Cleanup a test session (tmux + state).

```bash
# Cleanup with default directory
csm-test-tmux cleanup my-test

# Cleanup with custom directory
csm-test-tmux cleanup my-test --sessions-dir /tmp/my-tests

# Get cleanup status as JSON
csm-test-tmux cleanup my-test --format json
```

**Options**:
- `--sessions-dir`: CSM state directory
- `--format`: Output format

**Cleanup Steps** (best-effort):
1. Kill tmux session (`csm-test-<name>`)
2. Archive CSM session (future)
3. Remove sessions directory

**JSON Output**:
```json
{
  "tmux_killed": true,
  "csm_archived": true,
  "directory_clean": true
}
```

## AI Agent Usage

The tool is optimized for AI agent integration:

### JSON Output

All commands support `--format json` for structured output:

```python
import subprocess
import json

# Create session
result = subprocess.run(
    ["csm-test-tmux", "create", "test-1", "--format", "json"],
    capture_output=True,
    text=True
)
session = json.loads(result.stdout)
print(f"Created session: {session['tmux_session']}")

# Capture output
result = subprocess.run(
    ["csm-test-tmux", "capture", "test-1", "--format", "json"],
    capture_output=True,
    text=True
)
output = json.loads(result.stdout)
for line in output["lines"]:
    print(line)
```

### Error Handling

Errors include actionable solutions:

```json
{
  "error": "Session 'my-test' already exists",
  "type": "user_error",
  "title": "session name collision",
  "solutions": [
    "Cleanup existing session: csm-test-tmux cleanup my-test --sessions-dir /tmp/csm-test-my-test",
    "Use different name: csm-test-tmux create my-test-2"
  ]
}
```

### Exit Codes

- `0`: Success
- `1`: System error (tmux not found, command failed)
- `2`: Timeout error (Claude startup timeout)
- `3`: User error (invalid session name, session not found)

```python
result = subprocess.run(["csm-test-tmux", "create", "test"])
if result.returncode == 3:
    print("User error - check session name")
elif result.returncode == 2:
    print("Timeout - increase --startup-timeout")
elif result.returncode == 1:
    print("System error - check tmux/claude installation")
```

## Troubleshooting

### Session Creation Fails

**Error**: "failed to create tmux session"

**Solutions**:
```bash
# Check if tmux is installed
tmux -V

# Check if tmux is in PATH
which tmux

# Try creating tmux session manually
tmux new-session -d -s test-session
```

### Claude Startup Timeout

**Error**: "Claude startup timeout after 30s"

**Solutions**:
```bash
# Increase timeout
csm-test-tmux create my-test --startup-timeout 60

# Check if claude is working
claude --version

# View session to debug
tmux attach -t csm-test-my-test
```

### Session Name Collision

**Error**: "Session 'my-test' already exists"

**Solutions**:
```bash
# Cleanup existing session
csm-test-tmux cleanup my-test

# Use different name
csm-test-tmux create my-test-2

# List tmux sessions
tmux ls | grep csm-test
```

### Command Not Found

**Error**: "Session 'my-test' does not exist"

**Solutions**:
```bash
# List all csm-test sessions
tmux ls | grep csm-test

# Create session first
csm-test-tmux create my-test

# Check sessions directory
ls -la /tmp/csm-test-*
```

## Architecture

```
csm-test-tmux create my-test
    ↓
1. Validate session name (alphanumeric, hyphens, underscores)
    ↓
2. Create tmux session (csm-test-my-test)
    ↓
3. Create isolated CSM state directory (/tmp/csm-test-my-test)
    ↓
4. Start Claude in tmux session
    ↓
5. Wait for Claude prompt (polling every 500ms, 30s timeout)
    ↓
6. Return session metadata
```

## Examples

### Test Workflow Automation

```bash
#!/bin/bash
# Automated test workflow

SESSION="test-$$"  # Unique name with PID

# Create session
csm-test-tmux create $SESSION || exit 1

# Run test commands
csm-test-tmux send $SESSION "git status"
sleep 1

# Capture and verify output
OUTPUT=$(csm-test-tmux capture $SESSION --lines 20)
if echo "$OUTPUT" | grep -q "On branch main"; then
    echo "✅ Test passed"
else
    echo "❌ Test failed"
    csm-test-tmux cleanup $SESSION
    exit 1
fi

# Cleanup
csm-test-tmux cleanup $SESSION
```

### Parallel Testing

```bash
#!/bin/bash
# Run 3 tests in parallel

for i in 1 2 3; do
    (
        csm-test-tmux create test-$i
        csm-test-tmux send test-$i "git status"
        csm-test-tmux capture test-$i
        csm-test-tmux cleanup test-$i
    ) &
done

wait
echo "All tests complete"
```

### CI/CD Integration

```yaml
# .github/workflows/csm-test.yml
name: CSM Tests

on: [push]

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v2

      - name: Install tmux
        run: sudo apt-get install -y tmux

      - name: Install csm-test-tmux
        run: |
          go build -o csm-test-tmux ./cmd/csm-test-tmux
          sudo mv csm-test-tmux /usr/local/bin/

      - name: Run CSM tests
        run: |
          csm-test-tmux create ci-test
          csm-test-tmux send ci-test "git status"
          csm-test-tmux cleanup ci-test
```

## Security Considerations

**IMPORTANT**: `csm-test-tmux send` executes arbitrary commands in the tmux session. This is by design for testing purposes.

**Safe Usage**:
- Only use in controlled test environments
- Never pass untrusted user input to `send` command
- Session names are validated (alphanumeric, hyphens, underscores only)

**Sandboxing**:
- Each session has isolated CSM state (`--sessions-dir`)
- Tmux sessions are prefixed (`csm-test-<name>`)
- No interaction with production CSM sessions

## Contributing

See main CSM repository for contribution guidelines.

## License

See main CSM repository for license information.
