# ADR-0002: InitSequence Timing Delays and Lock-Free Implementation

## Status

Accepted

## Date

2026-02-14

## Context

After fixing the capture-pane polling bug (see ADR-0001), InitSequence still had critical failures in production:

**Problem 1: Double-Lock Error**

```
Error: tmux lock already held by this process
```

InitSequence.Run() wrapped operations in `withTmuxLock()`, but SendCommandLiteral() internally called SendCommand(), which also acquired the tmux lock. This caused a double-lock error.

**Problem 2: Command Queueing Bug**

Both `/rename` and `/agm:agm-assoc` commands appeared on the SAME input line:

```
❯ /rename test-session /agm:agm-assoc test-session
```

Only the first command executed; the second was ignored. This happened because commands were sent too quickly (<1 second apart), queuing in tmux's input buffer before the first Enter was processed.

**User Impact:**

```
$ agm session new test --harness=claude-code --detached
Creating session...
[hangs for 30 seconds]
Error: timeout waiting for ready-file
```

Sessions created successfully but initialization commands never executed.

## Decision

### Fix 1: Remove Lock Wrapper from InitSequence.Run()

**Before (broken)**:

```go
func (seq *InitSequence) Run() error {
    return withTmuxLock(func() error {
        // SendCommandLiteral calls SendCommand
        // SendCommand also calls withTmuxLock()
        return seq.sendRename()  // ERROR: double-lock
    })
}
```

**After (fixed)**:

```go
func (seq *InitSequence) Run() error {
    // No lock wrapper - each tmux command acquires lock independently
    if err := seq.sendRename(); err != nil {
        return fmt.Errorf("rename failed: %w", err)
    }

    if err := seq.sendAssociation(); err != nil {
        return fmt.Errorf("association failed: %w", err)
    }

    return nil
}
```

**Rationale:**

- InitSequence orchestrates multiple operations, doesn't need its own lock
- Each low-level tmux command (NewSession, SendCommand) handles locking internally
- Lock granularity should be at the individual command level, not the sequence level

### Fix 2: Rewrite SendCommandLiteral to Use exec.Command Directly

**Before (broken - called SendCommand)**:

```go
func SendCommandLiteral(sessionName, command string) error {
    sendKeysCmd := fmt.Sprintf("-l %q", command)
    if err := SendCommand(sessionName, sendKeysCmd); err != nil {
        return err  // SendCommand calls withTmuxLock()
    }
    time.Sleep(100 * time.Millisecond)
    // ...
}
```

**After (fixed - direct exec.Command)**:

```go
func SendCommandLiteral(sessionName, command string) error {
    socketPath := GetSocketPath()

    // Send command text with -l flag (literal interpretation)
    cmdText := exec.Command("tmux", "-S", socketPath, "send-keys", "-t", sessionName, "-l", command)
    if err := cmdText.Run(); err != nil {
        return fmt.Errorf("failed to send command text: %w", err)
    }

    time.Sleep(500 * time.Millisecond)  // Increased from 100ms

    // Send Enter separately (C-m = Enter key)
    cmdEnter := exec.Command("tmux", "-S", socketPath, "send-keys", "-t", sessionName, "C-m")
    if err := cmdEnter.Run(); err != nil {
        return fmt.Errorf("failed to send Enter: %w", err)
    }

    return nil
}
```

**Key Changes:**

1. Uses `exec.Command` directly instead of calling SendCommand
2. No lock acquisition (each exec.Command is atomic)
3. Increased delay from 100ms to 500ms

### Fix 3: Add 5-Second Wait After /rename

**Implementation**:

```go
func (seq *InitSequence) sendRename() error {
    renameCmd := fmt.Sprintf("/rename %s", seq.SessionName)
    if err := SendCommandLiteral(seq.SessionName, renameCmd); err != nil {
        return fmt.Errorf("failed to send /rename: %w", err)
    }

    // Wait longer for /rename to fully complete and Claude to be ready
    time.Sleep(5 * time.Second)
    debug.Log("sendRename: Wait complete, /rename should be done")

    return nil
}
```

**Rationale:**

- Ensures /rename command fully executes before /agm:agm-assoc is sent
- 5-second wait is empirically sufficient for Claude to process rename
- Prevents command queueing on input buffer

## Timing Analysis

**Minimum Total Duration**: ≥6 seconds

Breakdown:
1. SendCommandLiteral(/rename): 500ms
2. Post-rename wait: 5000ms
3. SendCommandLiteral(/agm:agm-assoc): 500ms
4. **Total**: 6000ms (6 seconds)

**Why These Values:**

- **500ms delay**: Empirically tested; 100ms was insufficient, commands still queued
- **5s wait**: Ensures first command completes before second starts
- **Trade-off**: Reliability > Speed (initialization is infrequent operation)

## Consequences

### Positive

- **Bug fixed**: No more double-lock errors
- **Bug fixed**: Commands execute on separate lines
- **Simpler locking**: No nested lock acquisition
- **Deterministic timing**: Predictable behavior, no race conditions
- **Production verified**: Manual E2E test confirmed fix works

### Negative

- **Slower initialization**: 6+ seconds minimum (vs ~1 second before)
  - **Mitigated by**: Initialization is infrequent (once per session creation)
  - **Acceptable**: Reliability more important than speed

- **Hard-coded delays**: Not configurable
  - **Mitigated by**: Values are empirically validated
  - **Future**: Could make configurable if needed

### Neutral

- **Simpler implementation**: Fewer levels of indirection (no SendCommand wrapper)
- **Direct tmux commands**: More transparent what's happening

## Regression Prevention

### Test Coverage

**Regression Tests** (`internal/tmux/init_sequence_regression_test.go`):

1. `TestSendCommandLiteral_DoesNotUseSendCommand`
   - Verifies SendCommandLiteral uses exec.Command, not SendCommand
   - Ensures no double-lock errors

2. `TestSendCommandLiteral_Timing`
   - Verifies 500ms delay between text and Enter
   - Tests two rapid calls take ≥1 second

3. `TestInitSequence_NoDoubleLock`
   - Runs full InitSequence.Run()
   - Verifies NO "lock already held" errors

4. `TestSendCommandLiteral_UsesLiteralFlag`
   - Verifies -l flag usage (literal text interpretation)
   - Prevents special character issues

5. `TestInitSequence_DetachedMode`
   - Tests detached mode (primary use case)
   - Verifies timeout behavior

**BDD Scenarios** (`test/bdd/features/session_initialization.feature`):

- Commands execute on separate lines (lines 72-79)
- Sufficient delay between commands (lines 81-86)
- Detached sessions initialize automatically (lines 95-101)

**Documentation**: `docs/testing/INIT_SEQUENCE_TEST_COVERAGE.md`

## Alternatives Considered

### Alternative 1: Configurable Delays

**Approach**: Make 500ms and 5s delays configurable via environment variables.

**Pros:**
- Flexibility for different environments
- Users could tune for their latency

**Cons:**
- More complexity (config parsing, validation)
- Risk of users setting too-low values and breaking initialization

**Rejected**: Hard-coded values are empirically validated and work reliably. Configurability adds complexity without clear benefit.

### Alternative 2: Adaptive Delays (Measure Claude Response Time)

**Approach**: Measure how long Claude takes to respond to /rename, adjust future delays.

**Pros:**
- Optimal performance for each environment
- Self-tuning

**Cons:**
- Significantly more complex
- Requires persistent state (delay history)
- Unpredictable timing (harder to test)

**Rejected**: Over-engineering for infrequent operation. Fixed delays are simpler and reliable.

### Alternative 3: Polling /rename Completion (Check Pane Content)

**Approach**: After sending /rename, poll pane content until "Session renamed to:" appears.

**Pros:**
- Deterministic (waits exact time needed)
- Potentially faster (no unnecessary 5s wait)

**Cons:**
- More complex (pattern matching, timeout handling)
- Fragile (depends on exact Claude output format)
- Minimal benefit (saves 1-3 seconds on infrequent operation)

**Rejected**: Fixed 5s wait is simpler, more reliable, and acceptable for initialization.

## References

- **Bug Fix Commit**: d8f1a61 - fix: resolve InitSequence double-lock and command queueing issues
- **Test Coverage Commit**: 473e13a - test: add comprehensive regression tests for InitSequence bugs
- **Implementation**: `internal/tmux/init_sequence.go`
- **Related ADR**: ADR-0001 (capture-pane polling approach)
- **Test Documentation**: `docs/testing/INIT_SEQUENCE_TEST_COVERAGE.md`

## Notes

**Production Validation**:

```bash
$ agm session new post-restart-test --harness=claude-code --detached
Creating session...
✓ Claude is ready and session associated!
```

Verified pane content shows commands on separate lines:

```
❯ /rename post-restart-test
  ⎿  Session renamed to: post-restart-test

❯ /agm:agm-assoc post-restart-test
  ✓ Session associated successfully
  UUID: 79c25e50
```

**Future Improvements**:

- If initialization time becomes a bottleneck, consider polling approach (Alternative 3)
- Monitor production for any remaining timing issues
- Add telemetry to track initialization duration distribution
