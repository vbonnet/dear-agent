# Integration Testing: AGM Mode Persistence

## Overview

Manual integration testing checklist for AGM mode persistence feature.

## Prerequisites

1. Dolt server running on configured port
2. AGM built and installed: `go build -C cmd/agm -o ~/go/bin/agm`
3. Hooks installed: `agm admin install-hooks`
4. Working Claude Code installation

## Test 1: Mode Detection and Persistence

**Objective**: Verify that mode changes are detected and persisted to database

### Steps:

1. Create new session:
   ```bash
   agm session new test-mode-persist
   ```

2. In Claude Code, press Shift+Tab twice to reach "ask" mode
   - Verify mode indicator shows "ask"

3. Use a tool (e.g., read a file) to trigger PreToolUse hook
   ```
   Read the contents of README.md
   ```

4. Exit Claude (Ctrl+D)

5. Verify database has correct mode:
   ```bash
   agm session list --format json | jq '.[] | select(.name=="test-mode-persist") | {name, permission_mode}'
   ```
   - Expected output: `"permission_mode": "ask"`

**Pass Criteria**:
- Mode field shows "ask" in database
- permission_mode_updated_at timestamp is recent
- permission_mode_source is "hook"

## Test 2: Mode Restoration on Resume

**Objective**: Verify that saved mode is restored when resuming session

### Steps:

1. Resume the session from Test 1:
   ```bash
   agm session resume test-mode-persist
   ```

2. Wait for "Restoring permission mode: ask" message

3. Verify mode indicator in Claude shows "ask" mode

4. Test that mode is actually active by using a tool
   - Should prompt for confirmation (ask mode behavior)

**Pass Criteria**:
- Mode restoration message appears
- Mode indicator shows "ask" after attach
- Mode behavior matches expected (confirmation prompts)

## Test 3: Backward Compatibility

**Objective**: Verify that old sessions without mode data work correctly

### Steps:

1. Create a session using old AGM version OR manually delete permission_mode:
   ```sql
   UPDATE agm_sessions SET permission_mode = NULL WHERE session_id = 'session-xxx';
   ```

2. Resume the session:
   ```bash
   agm session resume session-xxx
   ```

3. Verify no errors occur

4. Verify session starts in "default" mode

**Pass Criteria**:
- No errors during resume
- Session works normally
- Mode defaults to "default"

## Test 4: Mode Persistence Across Modes

**Objective**: Verify all four modes persist and restore correctly

### Steps:

For each mode (default, plan, ask, allow):

1. Create session: `agm session new test-mode-{mode}`
2. Change to target mode via Shift+Tab
3. Use a tool to trigger hook
4. Exit Claude
5. Verify database: `agm session list --format json | jq '.[] | select(.name=="test-mode-{mode}")'`
6. Resume session: `agm session resume test-mode-{mode}`
7. Verify mode is restored correctly

**Pass Criteria**:
- All four modes persist to database
- All four modes restore on resume
- Mode indicators match expected values

## Test 5: Non-Claude Agents

**Objective**: Verify graceful handling for agents that don't support modes

### Steps:

1. Create a Gemini session (or another non-Claude agent)
2. Resume the session

**Pass Criteria**:
- No mode restoration attempted
- No errors occur
- Session resumes normally

## Test 6: Cache Deduplication

**Objective**: Verify that redundant mode updates are avoided

### Steps:

1. Create session and set mode to "plan"
2. Use multiple tools without changing mode
3. Check logs: `tail -f /var/log/syslog | grep agm-mode-tracker`

**Pass Criteria**:
- First tool use triggers update
- Subsequent tool uses do NOT trigger updates (cache hit)
- Database write happens only once

## Test 7: Error Handling

**Objective**: Verify graceful degradation when components fail

### Test 7a: Hook fails to update database

1. Stop Dolt server
2. Change mode in active Claude session
3. Use a tool
4. Verify Claude continues working (non-blocking)

### Test 7b: Mode restoration fails

1. Create session with mode "plan"
2. Mock tmux.SendKeys to fail (requires code modification for testing)
3. Resume session
4. Verify warning is shown but attach continues

**Pass Criteria**:
- Hook failures don't block tool execution
- Mode restoration failures don't block session attach
- Appropriate warnings are shown

## Test 8: End-to-End Workflow

**Objective**: Full workflow test simulating real usage

### Steps:

1. Start new session: `agm session new real-work`
2. Work in default mode for a bit
3. Switch to plan mode (Shift+Tab once)
4. Do some planning work
5. Exit Claude
6. Resume later: `agm session resume real-work`
7. Verify you're in plan mode immediately
8. Continue planning work
9. Switch to ask mode (Shift+Tab once more)
10. Do some careful work
11. Exit and resume again
12. Verify you're in ask mode

**Pass Criteria**:
- Mode persists through multiple exit/resume cycles
- Each resume restores the last saved mode
- No manual mode cycling required

## Cleanup

After testing, clean up test sessions:

```bash
agm session archive test-mode-persist
agm session archive test-mode-default
agm session archive test-mode-plan
agm session archive test-mode-ask
agm session archive test-mode-allow
agm session archive real-work
```

## Success Criteria

All 8 tests must pass for the feature to be considered production-ready:

- ✅ Mode detection works via hook
- ✅ Mode persistence to database
- ✅ Mode restoration on resume
- ✅ Backward compatibility
- ✅ All modes supported
- ✅ Non-Claude agents handled
- ✅ Cache deduplication works
- ✅ Error handling is graceful
- ✅ End-to-end workflow smooth
