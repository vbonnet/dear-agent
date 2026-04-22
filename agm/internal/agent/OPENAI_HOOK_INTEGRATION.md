# OpenAI Adapter Hook Integration Summary

**Task:** Task 3.3 - Integrate Hooks into OpenAI Adapter (bead: oss-5fh9)
**Date:** 2026-02-24
**Status:** ✅ Complete

## Overview

Successfully integrated synthetic hook support into the OpenAI adapter, enabling lifecycle hooks (SessionStart, SessionEnd) for API-based sessions. This brings the OpenAI adapter to feature parity with Gemini CLI adapter for hook support.

## Changes Made

### 1. Core Implementation Files

#### `main/agm/internal/agent/openai_adapter.go`

**Added Methods:**
- `RunHook(sessionID SessionID, hookName string) error` (line 573-587)
  - Public method to execute hooks on demand
  - Validates session exists
  - Delegates to executeHook helper

- `executeHook(sessionID SessionID, sessionInfo *openai.SessionInfo, hookName string) error` (line 589-640)
  - Creates hook context files in `~/.agm/openai-hooks/`
  - Writes JSON with session metadata (session_id, hook_name, session_name, working_dir, model, timestamp)
  - Implements graceful degradation (hook failures are logged but don't block operations)
  - Returns nil on errors (non-fatal hook execution)

**Updated Methods:**
- `CreateSession()` (line 128-195)
  - Added SessionStart hook execution after session creation
  - Hook runs asynchronously, failures don't block session creation

- `TerminateSession()` (line 198-217)
  - Added SessionEnd hook execution before session deletion
  - Hook captures session info before cleanup

- `ExecuteCommand()` (line 553-566)
  - Changed `CommandRunHook` handling from `ErrUnsupportedCommand` to actual hook execution
  - Extracts hook_name parameter
  - Gets session info and executes hook

- `Capabilities()` (line 389-427)
  - Changed `SupportsHooks` from `false` to `true`
  - Updated comment to reflect synthetic hook support

**Added Import:**
- `path/filepath` for hook directory path construction

### 2. Test Files

#### `main/agm/internal/agent/openai_adapter_test.go`

**New Tests Added:**
1. `TestOpenAIAdapter_RunHook()` (lines 839-910)
   - Tests hook execution for all hook types: SessionStart, SessionEnd, BeforeAgent, AfterAgent
   - Verifies hook context file creation
   - Validates hook context JSON content
   - Tests error handling for invalid sessions

2. `TestOpenAIAdapter_ExecuteCommand_RunHook()` (lines 912-935)
   - Tests CommandRunHook via ExecuteCommand interface
   - Verifies hook file creation via command pattern

3. `TestOpenAIAdapter_SessionStartHook()` (lines 937-960)
   - Tests automatic SessionStart hook on CreateSession
   - Verifies hook fires without explicit invocation

4. `TestOpenAIAdapter_SessionEndHook()` (lines 962-989)
   - Tests automatic SessionEnd hook on TerminateSession
   - Verifies hook fires during session cleanup

5. `TestOpenAIAdapter_HookFailureGraceful()` (lines 991-1020)
   - Tests graceful degradation when hooks fail
   - Verifies session remains functional after hook errors

6. `TestOpenAIAdapter_Capabilities_HooksSupported()` (lines 1022-1030)
   - Verifies SupportsHooks capability is true

**Updated Tests:**
- `TestCapabilities()` (line 565)
  - Changed expectation from `SupportsHooks=false` to `SupportsHooks=true`
  - Updated error message to reflect synthetic hook support

- `TestExecuteCommand()` (lines 667-688)
  - Changed "run hook (not supported)" test to "run hook (now supported)"
  - Added hook file verification
  - Removed ErrUnsupportedCommand check

### 3. Documentation Updates

#### `main/agm/internal/agent/OPENAI_ADAPTER_IMPLEMENTATION.md`

- Updated Capabilities section to show `SupportsHooks: true`
- Added hook integration notes
- Changed CommandRunHook from "unsupported" to "executes synthetic hooks"
- Updated architecture comparison table

## Implementation Pattern

The implementation follows the **Gemini CLI adapter pattern** for consistency:

```go
// Pattern from gemini_cli_adapter.go (lines 454-523)
1. Create hook directory (~/.agm/{adapter}-hooks)
2. Generate hook context JSON with session metadata
3. Write to file: {session-id}-{hook-name}.json
4. Log execution to stderr
5. Return nil (graceful degradation)

// Applied to OpenAI adapter
func (a *OpenAIAdapter) executeHook(sessionID SessionID, sessionInfo *openai.SessionInfo, hookName string) error {
    hookDir := filepath.Join(homeDir, ".agm", "openai-hooks")
    hookFile := filepath.Join(hookDir, fmt.Sprintf("%s-%s.json", string(sessionID), hookName))

    hookContext := map[string]interface{}{
        "session_id":   string(sessionID),
        "hook_name":    hookName,
        "session_name": sessionInfo.Title,
        "working_dir":  sessionInfo.WorkingDirectory,
        "model":        sessionInfo.Model,
        "timestamp":    time.Now().Format(time.RFC3339),
    }

    // Write and log (all errors are non-fatal)
    // ...
}
```

## Hook Context File Format

Hook context files are written to `~/.agm/openai-hooks/{session-id}-{hook-name}.json`:

```json
{
  "session_id": "abc123-def456",
  "hook_name": "SessionStart",
  "session_name": "my-session",
  "working_dir": "/path/to/workspace",
  "model": "gpt-4-turbo-preview",
  "timestamp": "2026-02-24T16:22:50Z"
}
```

These files can be consumed by external scripts for integration with other systems.

## Lifecycle Integration

### SessionStart Hook
```
CreateSession()
├─> Create session via SessionManager
├─> Update session title
├─> Add workflow system message (if specified)
├─> Get session info
└─> Execute SessionStart hook ✅ (new)
    ├─> Create hook context file
    ├─> Log execution
    └─> Return (errors are non-fatal)
```

### SessionEnd Hook
```
TerminateSession()
├─> Get session info ✅ (new - before deletion)
├─> Execute SessionEnd hook ✅ (new)
│   ├─> Create hook context file
│   ├─> Log execution
│   └─> Return (errors are non-fatal)
└─> Delete session via SessionManager
```

## Test Coverage

All tests pass with comprehensive coverage:

```bash
$ go test ./internal/agent -run "TestOpenAIAdapter.*Hook"
=== RUN   TestOpenAIAdapter_RunHook
=== RUN   TestOpenAIAdapter_RunHook/SessionStart_hook
=== RUN   TestOpenAIAdapter_RunHook/SessionEnd_hook
=== RUN   TestOpenAIAdapter_RunHook/BeforeAgent_hook
=== RUN   TestOpenAIAdapter_RunHook/AfterAgent_hook
=== RUN   TestOpenAIAdapter_RunHook/Invalid_session
--- PASS: TestOpenAIAdapter_RunHook (0.01s)
=== RUN   TestOpenAIAdapter_ExecuteCommand_RunHook
--- PASS: TestOpenAIAdapter_ExecuteCommand_RunHook (0.01s)
=== RUN   TestOpenAIAdapter_SessionStartHook
--- PASS: TestOpenAIAdapter_SessionStartHook (0.11s)
=== RUN   TestOpenAIAdapter_SessionEndHook
--- PASS: TestOpenAIAdapter_SessionEndHook (0.11s)
=== RUN   TestOpenAIAdapter_HookFailureGraceful
--- PASS: TestOpenAIAdapter_HookFailureGraceful (0.01s)
=== RUN   TestOpenAIAdapter_Capabilities_HooksSupported
--- PASS: TestOpenAIAdapter_Capabilities_HooksSupported (0.00s)
PASS
ok      github.com/vbonnet/ai-tools/agm/internal/agent      0.243s
```

**Full test suite:**
```bash
$ go test ./internal/agent
ok      github.com/vbonnet/ai-tools/agm/internal/agent      0.394s
```

## Key Design Decisions

### 1. Synthetic vs Native Hooks
**Decision:** Use synthetic hooks (file-based signals) rather than subprocess execution.

**Rationale:**
- OpenAI adapter is API-based, no subprocess to attach hooks to
- Matches Gemini CLI adapter pattern (also uses file-based approach)
- Simple, testable, debuggable
- External scripts can consume hook files for integration

### 2. Graceful Degradation
**Decision:** Hook failures are logged but don't block session operations.

**Rationale:**
- Hooks are auxiliary functionality, not core to session management
- Session creation/termination must be reliable
- Follows error handling pattern from Gemini adapter
- All hook errors return nil (logged to stderr)

### 3. Hook Context Files
**Decision:** Write hook context to `~/.agm/openai-hooks/` directory.

**Rationale:**
- Consistent with Gemini adapter (`~/.agm/gemini-hooks/`)
- Provides audit trail of hook executions
- Enables external script integration
- Facilitates debugging and testing

### 4. Automatic Hook Execution
**Decision:** Trigger SessionStart/SessionEnd hooks automatically in CreateSession/TerminateSession.

**Rationale:**
- Matches expected behavior from CLI adapters
- Provides seamless integration with AGM lifecycle
- No manual hook invocation required
- Consistent with architecture design

## Architecture Alignment

This implementation aligns with:
- **ADR-007:** Hook-Based State Detection (synthetic hooks for API agents)
- **Gemini CLI Adapter:** File-based hook pattern (consistency)
- **Agent Interface:** ExecuteCommand(CommandRunHook) contract
- **Phase 3 Requirements:** Hook support for all adapters

## Usage Example

```go
// Create OpenAI adapter
adapter, _ := NewOpenAIAdapter(ctx, &OpenAIConfig{
    APIKey: "sk-...",
    Model:  "gpt-4-turbo-preview",
})

// Create session (SessionStart hook fires automatically)
sessionID, _ := adapter.CreateSession(SessionContext{
    Name:             "my-session",
    WorkingDirectory: "/workspace",
})
// Hook file created: ~/.agm/openai-hooks/{sessionID}-SessionStart.json

// Execute custom hook
adapter.RunHook(sessionID, "BeforeAgent")
// Hook file created: ~/.agm/openai-hooks/{sessionID}-BeforeAgent.json

// Terminate session (SessionEnd hook fires automatically)
adapter.TerminateSession(sessionID)
// Hook file created: ~/.agm/openai-hooks/{sessionID}-SessionEnd.json
```

## Future Enhancements

While not in scope for this task, potential improvements include:

1. **Subprocess Execution:** Execute actual hook scripts (like Claude adapter)
2. **Hook Output Parsing:** Parse JSON output from hooks and inject into session
3. **Hook Timeouts:** Add configurable timeouts for hook execution
4. **Hook Configuration:** Support hook scripts in config file
5. **Hook Chaining:** Support multiple hooks per lifecycle event

## Verification Checklist

✅ All requirements met:
- [x] Hook execution in CreateSession (SessionStart)
- [x] Hook execution in TerminateSession (SessionEnd)
- [x] ExecuteCommand handles CommandRunHook
- [x] RunHook() helper method added
- [x] Hook context files created
- [x] Graceful error handling
- [x] Comprehensive tests (6 new tests)
- [x] All tests pass
- [x] Capabilities.SupportsHooks = true
- [x] Follows Gemini adapter pattern
- [x] Documentation updated

## Related Files

**Implementation:**
- `main/agm/internal/agent/openai_adapter.go`
- `main/agm/internal/agent/openai_adapter_test.go`

**Documentation:**
- `main/agm/internal/agent/OPENAI_ADAPTER_IMPLEMENTATION.md`
- `main/agm/docs/HOOKS-SETUP.md`
- `main/agm/docs/adr/ADR-007-hook-based-state-detection.md`

**Reference Implementations:**
- `main/agm/internal/agent/gemini_cli_adapter.go` (lines 417-523)
- `main/agm/internal/agent/gemini_cli_adapter_test.go` (lines 9-150)

## Completion

Task 3.3 (bead: oss-5fh9) is **complete** with all deliverables met:
- ✅ Hook architecture reviewed
- ✅ Gemini adapter pattern followed
- ✅ SessionStart/SessionEnd hooks integrated
- ✅ CommandRunHook implemented
- ✅ Comprehensive tests added
- ✅ All tests passing
- ✅ Documentation updated

The OpenAI adapter now has full synthetic hook support, bringing it to feature parity with the Gemini CLI adapter for lifecycle hooks.
