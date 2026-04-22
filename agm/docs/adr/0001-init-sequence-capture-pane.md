# ADR-0001: InitSequence Uses Capture-Pane Polling Instead of Control Mode

## Status

Accepted

## Date

2026-02-14

## Context

The `InitSequence` struct orchestrates initialization of new Claude sessions by sending `/rename` and `/agm:agm-assoc` commands. Prior to this change, it used tmux Control Mode with an `OutputWatcher` to detect when Claude was ready.

**The Problem:**

The original implementation had a critical bug in `waitForClaudePrompt()`:

```go
func (seq *InitSequence) waitForClaudePrompt(ctrl *ControlModeSession, watcher *OutputWatcher, timeout time.Duration) error {
    // Bug: GetRecentOutput() is non-blocking and returns empty if scanner not consumed
    lines := watcher.GetRecentOutput(5)

    for _, line := range lines {
        if containsClaudePromptPattern(line) {
            return nil  // Never reached if buffer empty!
        }
    }
}
```

**Root Cause:**

- `OutputWatcher.GetRecentOutput()` returns buffer contents WITHOUT consuming the scanner
- The scanner is only consumed by `OutputWatcher.WaitForPattern()` (which wasn't being called)
- Result: `GetRecentOutput()` always returned empty array → prompt never detected → timeout

**User Impact:**

```
$ agm session new test-session --harness=claude-code
Creating session...
[hangs for 30 seconds]
Warning: Claude initialization timeout (continuing anyway)
```

The session was created but `/rename` and `/agm:agm-assoc` commands were never executed.

## Decision

Replace control mode polling with **capture-pane polling** using `WaitForClaudePrompt()` from `prompt_detector.go`.

**Implementation:**

```go
// BEFORE (broken)
func (seq *InitSequence) Run() error {
    ctrl, err := StartControlMode(seq.SessionName)
    watcher := NewOutputWatcher(ctrl.Stdout)

    seq.sendRename(ctrl, watcher)      // Bug: polling GetRecentOutput()
    seq.sendAssociation(ctrl, watcher) // Bug: polling GetRecentOutput()
}

// AFTER (fixed)
func (seq *InitSequence) Run() error {
    seq.sendRename()      // Uses WaitForClaudePrompt (capture-pane)
    seq.sendAssociation() // Uses WaitForClaudePrompt (capture-pane)
}
```

**Why WaitForClaudePrompt:**

1. **Proven in production**: Already used in `new.go:421-423` for session creation
2. **Simpler implementation**: No control mode setup/teardown, no scanner management
3. **Better trust prompt handling**: Capture-pane sees ALL output (including trust prompts)
4. **Control mode deprecated**: Codebase has comments marking control mode as DEPRECATED

**How It Works:**

```go
// WaitForClaudePrompt polls every 500ms using capture-pane
func WaitForClaudePrompt(sessionName string, timeout time.Duration) error {
    for time.Now().Before(deadline) {
        // Capture last 50 lines from pane
        cmd := exec.Command("tmux", "capture-pane", "-t", sessionName, "-p", "-S", "-50")
        output, _ := cmd.CombinedOutput()

        if containsClaudePromptPattern(string(output)) {
            return nil  // ✅ Prompt found!
        }

        time.Sleep(500 * time.Millisecond)
    }

    return fmt.Errorf("timeout waiting for Claude prompt")
}
```

## Consequences

### Positive

- **Bug fixed**: InitSequence now correctly detects Claude prompt
- **Simpler code**: Removed 87 lines (control mode + watcher setup, waitForClaudePrompt method)
- **Easier trust prompt handling**: Capture-pane sees all output; no need to auto-answer trust prompts
- **Proven reliability**: Same code already working in production elsewhere
- **Better maintainability**: Fewer moving parts (no scanner, no control mode, no watcher)

### Negative

- **External process overhead**: Each poll spawns `tmux capture-pane` subprocess
  - Mitigated by: 500ms polling interval (6-10 calls typical = ~60ms overhead)
  - Acceptable for initialization path (not performance-critical)

- **Stateless polling**: No persistent connection to tmux output stream
  - Mitigated by: Polling is sufficient for prompt detection (don't need every line)

### Neutral

- **Trust prompt handling simplified**: Capture-pane approach sees trust prompts naturally
  - Old approach: Auto-answer trust prompts via control mode (complex, fragile)
  - New approach: Wait for user to manually answer, then detect "❯" prompt (simple, reliable)

## Alternatives Considered

### Alternative 1: Fix Control Mode (Use WaitForPattern)

**Approach**: Keep control mode, call `watcher.WaitForPattern()` instead of `GetRecentOutput()`

**Pros:**
- Minimal code change
- No external process overhead

**Cons:**
- Still using deprecated control mode
- More complex than capture-pane
- Trust prompt handling still complex

**Rejected**: Control mode marked DEPRECATED; simpler to migrate away

### Alternative 2: Hybrid Approach (Capture-Pane with Control Mode Fallback)

**Approach**: Try capture-pane first, fall back to control mode if fails

**Pros:**
- Maximum reliability (two approaches)

**Cons:**
- Doubled complexity
- Control mode already deprecated

**Rejected**: Over-engineering; capture-pane proven sufficient

### Alternative 3: Async Monitoring with Channels

**Approach**: Goroutine continuously monitors output, signals via channel

**Pros:**
- Lower latency (immediate detection vs 500ms poll)

**Cons:**
- Increased complexity (goroutines, channels, cleanup)
- Marginal benefit (500ms acceptable for init sequence)

**Rejected**: Complexity not justified for initialization path

## References

- Bug Report: AGM session new hangs during initialization
- Implementation: `internal/tmux/init_sequence.go`
- Proven Solution: `internal/tmux/prompt_detector.go:21-79`
- Control Mode Deprecation: See comments in `internal/tmux/control.go`

## Notes

**Performance Measurements** (if needed in future):

- Capture-pane overhead: ~10ms per call
- Typical polls until prompt: 6-10 calls (3-5 seconds)
- Total overhead: ~60-100ms (acceptable)

**Trust Prompt Handling:**

Trust prompts appear as text in pane output:

```
Do you want to allow access? [Yes, proceed / No, deny]
```

With capture-pane, `WaitForClaudePrompt()` continues polling until user manually answers and Claude prompt ("❯") appears. This is SIMPLER than the old control mode approach which tried to auto-answer trust prompts (complex, error-prone).
