# OpenAI Adapter Hook Architecture

**Status:** Approved (Phase 2, Task 3.1)
**Bead:** oss-0zkh
**Date:** 2026-02-24

## Context

AGM supports lifecycle hooks (SessionStart, SessionEnd, BeforeAgent, AfterAgent) for Claude and Gemini adapters. These hooks enable automation workflows like state detection, logging, and context injection.

The OpenAI adapter needs hook support to maintain feature parity with other AGM adapters.

## Challenge: API-Based Execution Model

**Key Difference:** OpenAI adapter is API-based, not subprocess-based.

| Adapter | Execution Model | Hook Model |
|---------|----------------|------------|
| Claude | CLI subprocess in tmux | **Native hooks** - Claude Code triggers hooks at lifecycle points |
| Gemini | CLI subprocess in tmux | **Native hooks** - Gemini CLI triggers hooks via settings.json |
| OpenAI | API calls in AGM process | **Synthetic hooks** - AGM adapter triggers hooks programmatically |

**Problem:** OpenAI has no official CLI, so there's no subprocess with natural lifecycle events.

## Decision: Synthetic Hook Events

Implement **synthetic hook execution** where the OpenAI adapter explicitly triggers hook scripts at session lifecycle points using subprocess execution.

### Architecture

**Hook Execution Flow:**
```
AGM OpenAI Adapter
    ├─> CreateSession()
    │       └─> triggerHook("SessionStart")
    │               └─> exec.Command(hookScript) with JSON stdin
    │
    ├─> SendMessage()
    │       ├─> triggerHook("BeforeAgent")
    │       ├─> API call to OpenAI
    │       └─> triggerHook("AfterAgent")
    │
    └─> TerminateSession()
            └─> triggerHook("SessionEnd")
```

**Hook Script Interface:**
```bash
#!/bin/bash
# Hook receives JSON context via stdin
# Example: SessionStart hook

input_json=$(cat)  # Read session context
session_id=$(echo "$input_json" | jq -r '.session_id')
working_dir=$(echo "$input_json" | jq -r '.working_directory')

# Perform hook logic (log, update state, etc.)
echo "Session $session_id started in $working_dir" >> ~/.agm/hooks.log

# Return JSON response (optional)
cat <<EOF
{
  "status": "success",
  "message": "SessionStart hook executed"
}
EOF
exit 0
```

**Hook Context JSON Schema:**
```json
{
  "session_id": "uuid-string",
  "hook_type": "SessionStart|SessionEnd|BeforeAgent|AfterAgent",
  "timestamp": "2026-02-24T14:30:00Z",
  "working_directory": "/path/to/project",
  "model": "gpt-4-turbo-preview",
  "session_name": "my-openai-session",
  "metadata": {
    "project": "ai-tools",
    "workflow": "deep-research"
  }
}
```

## Hook Types Supported

### Phase 2 (Initial Implementation)

1. **SessionStart** - Triggered after CreateSession() completes
   - Use case: Initialize logging, update AGM state tracking
   - Timing: After OpenAI session created, before first message

2. **SessionEnd** - Triggered before TerminateSession() completes
   - Use case: Cleanup, final logging, archive conversation
   - Timing: Before session deleted from storage

### Phase 3 (Future Enhancement)

3. **BeforeAgent** - Triggered before each OpenAI API call
   - Use case: Log user messages, validate input
   - Timing: After user message added to history, before API call

4. **AfterAgent** - Triggered after each OpenAI API response
   - Use case: Log assistant responses, extract metrics
   - Timing: After assistant response stored in history

## Hook Discovery

Hooks are discovered from AGM configuration directory:

```
~/.agm/hooks/
├── SessionStart.sh         # Executed on session creation
├── SessionEnd.sh           # Executed on session termination
├── BeforeAgent.sh          # (Phase 3) Pre-message hook
└── AfterAgent.sh           # (Phase 3) Post-message hook
```

**Hook Requirements:**
- Must be executable (`chmod +x`)
- Must accept JSON input via stdin
- Should exit with code 0 for success
- May return JSON output via stdout (optional)
- Timeout: 5 seconds (configurable)

## Implementation Strategy

### 1. Hook Executor Module (`openai/hooks.go`)

```go
// triggerHook executes a hook script with session context.
//
// Hooks are discovered from ~/.agm/hooks/ directory.
// Hook script receives JSON context via stdin.
// Returns error only if hook fails with exit code 2 (blocking).
func (a *OpenAIAdapter) triggerHook(
    sessionID SessionID,
    hookType HookType,
    context HookContext,
) error {
    // 1. Discover hook script
    // 2. Marshal context to JSON
    // 3. Execute hook via exec.Command
    // 4. Pipe JSON to stdin
    // 5. Capture stdout/stderr
    // 6. Handle exit codes (0=success, 1=warn, 2=block)
    // 7. Parse hook output (optional)
}
```

### 2. Integration Points

**CreateSession():**
```go
func (a *OpenAIAdapter) CreateSession(ctx SessionContext) (SessionID, error) {
    // ... existing session creation logic ...

    // Trigger SessionStart hook
    hookCtx := HookContext{
        SessionID:        sessionID,
        HookType:         HookTypeSessionStart,
        WorkingDirectory: ctx.WorkingDirectory,
        // ...
    }
    if err := a.triggerHook(sessionID, HookTypeSessionStart, hookCtx); err != nil {
        // Hook blocking (exit 2): abort session creation
        a.sessionManager.DeleteSession(string(sessionID))
        return "", fmt.Errorf("SessionStart hook blocked: %w", err)
    }

    return sessionID, nil
}
```

**TerminateSession():**
```go
func (a *OpenAIAdapter) TerminateSession(sessionID SessionID) error {
    // Trigger SessionEnd hook BEFORE deletion
    hookCtx := HookContext{
        SessionID: sessionID,
        HookType:  HookTypeSessionEnd,
    }
    _ = a.triggerHook(sessionID, HookTypeSessionEnd, hookCtx)
    // Ignore errors - SessionEnd is fire-and-forget

    // ... existing termination logic ...
}
```

### 3. RunHook Method

Add `RunHook()` method to OpenAI adapter (matching Gemini API):

```go
// RunHook executes a lifecycle hook for the OpenAI session.
//
// Unlike CLI adapters (Claude/Gemini), OpenAI hooks are synthetic -
// they are executed by AGM, not by the agent itself.
func (a *OpenAIAdapter) RunHook(sessionID SessionID, hookName string) error {
    session, err := a.sessionManager.GetSession(string(sessionID))
    if err != nil {
        return fmt.Errorf("session not found: %w", err)
    }

    hookCtx := HookContext{
        SessionID:        sessionID,
        HookType:         parseHookType(hookName),
        WorkingDirectory: session.WorkingDirectory,
        Model:            a.model,
    }

    return a.triggerHook(sessionID, hookCtx.HookType, hookCtx)
}
```

## Differences vs Native Hooks

### Native Hooks (Claude/Gemini)
✅ Triggered by agent subprocess automatically
✅ Access to agent process environment
✅ Can interact with tmux session directly
✅ Hook output injected into agent UI

### Synthetic Hooks (OpenAI)
⚠️ Triggered by AGM adapter programmatically
⚠️ No access to agent subprocess (none exists)
⚠️ No tmux session interaction
⚠️ Hook output logged, not displayed in UI

## Limitations

1. **No BeforeAgent/AfterAgent in Phase 2**
   - Requires refactoring SendMessage() to support hook injection
   - Deferred to Phase 3 (per-message hooks)

2. **No Hook Output Injection**
   - OpenAI API calls don't support runtime context injection
   - Hook output is logged but not displayed to user
   - Workaround: Hooks can update AGM state/logs directly

3. **No Interactive Hooks**
   - Hooks cannot prompt user for input
   - Must be fire-and-forget or blocking (exit 2)

4. **No Native State Detection**
   - Unlike Claude (tmux polling), OpenAI has no "busy" state
   - Hooks must manually update AGM state tracking

## Testing Strategy

1. **Unit Tests:** Hook discovery, JSON marshaling, subprocess execution
2. **Integration Tests:** End-to-end hook execution with marker files
3. **Error Handling:** Hook timeouts, exit codes, malformed output
4. **Compatibility Tests:** Verify hooks work with Claude/Gemini hook scripts

## Validation Criteria

- ✅ SessionStart hook executes after CreateSession()
- ✅ SessionEnd hook executes before TerminateSession()
- ✅ Hook receives correct JSON context via stdin
- ✅ Blocking hooks (exit 2) abort session creation
- ✅ Hook timeouts handled gracefully (default 5s)
- ✅ Hook errors logged but don't crash adapter
- ✅ RunHook() method available for manual hook triggering

## Migration Path

**Backward Compatibility:**
- Hooks are optional - adapter works without hooks installed
- Graceful degradation if hook scripts missing or fail
- No breaking changes to existing OpenAI adapter API

**Installation:**
```bash
# Install hooks (same scripts work for OpenAI, Claude, Gemini)
agm admin doctor --fix
# Output: ✓ Hooks installed to ~/.agm/hooks/
```

## Future Enhancements (Phase 3+)

1. **Per-Message Hooks** (BeforeAgent, AfterAgent)
   - Requires SendMessage() refactoring
   - Hook context includes message content, token counts

2. **Async Hook Execution**
   - Background hooks that don't block operations
   - Useful for logging, metrics collection

3. **Hook Output Processing**
   - Parse hook JSON output for AGM state updates
   - Support for hook-driven metadata injection

4. **Custom Hook Events**
   - User-defined hook types (OnExport, OnImport, etc.)
   - Extensibility for third-party integrations

## Related Documents

- **ADR-007:** Hook-Based State Detection (Claude adapter)
- **Gemini SPEC.md:** Gemini CLI hook implementation
- **OpenAI EXECUTION-MODEL.md:** API-based adapter architecture
- **Task 3.3:** Hook implementation (implementation task)

## Decision Log

**Why synthetic hooks instead of no hooks?**
- Maintains feature parity with Claude/Gemini adapters
- Enables AGM state tracking for OpenAI sessions
- Reuses existing hook scripts across all adapters

**Why not integrate with OpenAI Functions?**
- Functions are for tool use, not lifecycle events
- Would require model support (API overhead)
- Synthetic hooks are simpler and more flexible

**Why subprocess execution instead of Go plugins?**
- Shell scripts are portable and easy to debug
- Matches Claude/Gemini hook model
- No dynamic linking complexity

---

**Approved by:** Foundation Engineering
**Implementation:** Phase 2, Task 3.3 (Hook Implementation)
**Bead:** oss-0zkh (architecture decision)
