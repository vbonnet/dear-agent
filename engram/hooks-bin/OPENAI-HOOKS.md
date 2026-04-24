# OpenAI Hook Integration

## Summary

**Decision**: OpenAI sessions **reuse existing hooks** without requiring special wrappers.

## Architecture Overview

### Hook Types and Input Formats

Engram supports three hook execution contexts:

1. **Claude Code** - Direct binary execution
2. **Gemini CLI** - Requires JSON wrappers (see `hooks/gemini-wrappers/`)
3. **OpenAI API** - Reuses Claude Code hooks directly

### Why OpenAI Doesn't Need Wrappers

#### SessionStart/SessionEnd Hooks
These hooks are **standalone executables** that:
- Run without input arguments
- Use environment variables and filesystem for context
- Output to stdout/stderr for logging
- Exit with code 0 (success) or non-zero (failure)

**Example**: `sessionstart-mcp-wizard`
```go
func main() {
    os.Exit(runHook(os.Stdout, os.Stderr))
}

func runHook(stdout, stderr io.Writer) int {
    log := newLogger(stderr)
    log.info("Session start: Executing MCP health check")
    // No input parsing required
    return 0
}
```

#### PreToolUse/PostToolUse Hooks
These hooks receive **tool invocation JSON via stdin**:
```json
{
  "name": "Bash",
  "parameters": {
    "command": "bd create oss-example"
  }
}
```

This format is **platform-agnostic** and works identically for:
- Claude Code (native)
- OpenAI API (via synthetic execution layer)
- Any other API-based agent framework

**Example**: `posttool-auto-commit-beads`
```go
type ToolInput struct {
    Name       string                 `json:"name"`
    Parameters map[string]interface{} `json:"parameters"`
}

func main() {
    var toolInput ToolInput
    decoder := json.NewDecoder(os.Stdin)
    decoder.Decode(&toolInput)
    // Process tool input
}
```

### Gemini vs OpenAI: Why Different Approaches?

**Gemini CLI Wrappers Required**:
- Gemini uses **different JSON schema** for SessionStart/SessionEnd:
  ```json
  {
    "session_id": "string",
    "transcript_path": "string",
    "cwd": "string",
    "hook_event_name": "SessionStart",
    "timestamp": "2026-02-24T10:30:00Z",
    "source": "startup"
  }
  ```
- Wrappers translate Gemini JSON → engram hook execution
- See `hooks/gemini-wrappers/sessionstart-agm-safe-auto-detect.sh` for example

**OpenAI API No Wrappers Needed**:
- OpenAI sessions use **same tool invocation format** as Claude Code
- SessionStart/SessionEnd hooks execute as standalone binaries
- Session context passed via **environment variables**:
  - `PWD` - Current working directory
  - `HOME` - User home directory
  - Custom vars like `OPENAI_SESSION_ID` (optional)

## OpenAI Session Context Passing

### Environment Variables
The OpenAI adapter should set:
```bash
export OPENAI_SESSION_ID="sess_abc123"
export PWD="/path/to/working/directory"
```

### Session Storage
OpenAI sessions store data in `~/.agm/openai-sessions/` (JSONL format):
```jsonl
{"timestamp": "2026-02-24T10:30:00Z", "event": "session_start", "session_id": "sess_abc123"}
{"timestamp": "2026-02-24T10:35:00Z", "event": "tool_use", "tool": "Bash", "command": "ls -la"}
```

Hooks can read this data if needed:
```go
sessionID := os.Getenv("OPENAI_SESSION_ID")
sessionFile := filepath.Join(os.Getenv("HOME"), ".agm", "openai-sessions", sessionID + ".jsonl")
```

## Available Hooks for OpenAI

All existing hooks work with OpenAI sessions:

### SessionStart Hooks
- `sessionstart-guardian` - Validates hook configuration
- `sessionstart-mcp-wizard` - MCP health checks
- `sessionstart-agm-safe-auto-detect` - Auto-detects agent context
- `token-tracker-init` - Initializes token tracking

### SessionEnd Hooks
- `sessionend-wayfinder-consolidation` - Consolidates Wayfinder session data
- `sessionend-retrospective-processor` - Processes session retrospectives
- `sessionend-transcript-capture` - Captures session transcripts
- `token-tracker-summary` - Displays token usage summary

### PreToolUse Hooks
- `pretool-worktree-enforcer` - Enforces worktree boundaries
- `pretool-bash-blocker` - Blocks dangerous bash commands
- `pretool-beads-protection` - Protects bead database integrity
- `pretool-validate-paired-files` - Validates paired file requirements

### PostToolUse Hooks
- `posttool-auto-commit-beads` - Auto-commits bead database changes

## Implementation Checklist for OpenAI Adapter

### Minimal Viable Hook Support (MVP)

- [ ] Set environment variables before hook execution
  - `OPENAI_SESSION_ID` - Current session identifier
  - `PWD` - Working directory
  - `HOME` - User home directory

- [ ] Execute SessionStart hooks at session initialization
  - Run binaries from `~/.claude/hooks/session-start/`
  - Capture stdout/stderr for logging
  - Handle exit codes (0 = success, non-zero = warning/error)

- [ ] Execute SessionEnd hooks at session termination
  - Run binaries from `~/.claude/hooks/session-end/`
  - Pass session context via environment variables
  - Allow graceful failure (don't block session close)

### Optional: Tool Hook Support

- [ ] Execute PreToolUse hooks before tool invocation
  - Send tool JSON to stdin: `{"name": "Bash", "parameters": {...}}`
  - Capture exit code (0 = allow, non-zero = block)

- [ ] Execute PostToolUse hooks after tool invocation
  - Send tool JSON to stdin
  - Capture stdout/stderr for feedback
  - Always allow (non-blocking)

## Testing OpenAI Hook Integration

### Manual Testing
```bash
# Test SessionStart hook
export OPENAI_SESSION_ID="test_session_123"
export PWD="$HOME/src/test-project"
~/.claude/hooks/session-start/sessionstart-mcp-wizard

# Test PostToolUse hook
echo '{"name":"Bash","parameters":{"command":"bd create test-bead"}}' | \
  ~/.claude/hooks/posttool/posttool-auto-commit-beads
```

### Integration Testing
1. Start OpenAI session with hooks enabled
2. Verify SessionStart hooks execute automatically
3. Perform bead operations, verify PostToolUse auto-commit
4. Exit session, verify SessionEnd hooks execute

## Performance Expectations

All hooks designed for sub-second execution:
- SessionStart: <50ms (target <20ms)
- SessionEnd: <200ms
- PreToolUse: <100ms (blocking)
- PostToolUse: <200ms (non-blocking)

## Troubleshooting

### Hook Not Found
```bash
# Verify hook exists
ls -la ~/.claude/hooks/session-start/
ls -la ~/.claude/hooks/session-end/
ls -la ~/.claude/hooks/posttool/
```

### Hook Fails Silently
```bash
# Run hook manually with debug output
CLAUDE_PLUGIN_RELOAD_DEBUG=1 ~/.claude/hooks/session-start/sessionstart-guardian
```

### Session Context Missing
```bash
# Verify environment variables are set
echo $OPENAI_SESSION_ID
echo $PWD
echo $HOME
```

## References

- **Hook README**: `hooks/README.md` - Complete hook catalog
- **Gemini Wrappers**: `hooks/gemini-wrappers/` - Example wrapper implementations
- **Hook Specs**: `hooks/cmd/*/SPEC.md` - Individual hook specifications
- **Performance**: `hooks/PERFORMANCE.md` - Performance benchmarks

## Conclusion

OpenAI sessions require **zero additional hook infrastructure**. All existing engram hooks work out-of-the-box with OpenAI API-based execution by:

1. Reusing the Claude Code tool invocation JSON format
2. Passing session context via environment variables
3. Executing hooks as standalone binaries

This design eliminates duplication and ensures consistent behavior across all agent platforms (Claude Code, Gemini CLI, OpenAI API).
