# Reaper Hang Diagnosis - Skill Completion Issue

## Problem Statement

After `/agm:agm-exit` skill completes, the system enters a zero-token waiting loop for 60 seconds before Astrocyte sends ESC to recover. The reaper, which was spawned asynchronously, is also stuck waiting for the Claude prompt to appear.

## Root Cause Analysis

### Incident Timeline (2026-02-02T21:22:24)

1. `/agm:agm-exit` skill executes successfully
2. Skill output includes multi-part message (reaper details + project summary + emoji)
3. Async reaper spawns (PID: 3738395) and begins waiting for Claude prompt
4. Claude Code enters zero-token waiting state (internal bug)
5. **No prompt appears** because Claude is stuck in internal waiting loop
6. Reaper stuck in `WaitForClaudePrompt()` trying to detect prompt
7. After 60 seconds, Astrocyte detects hang and sends ESC
8. ESC breaks Claude's waiting state, prompt appears
9. Reaper finally detects prompt and proceeds with archival

### Technical Root Cause: Blocking Scanner in GetRawLine

**File**: `internal/tmux/output_watcher.go:218-244`

**Problem Code**:
```go
func (w *OutputWatcher) GetRawLine(timeout time.Duration) (string, error) {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		// ❌ BLOCKING CALL - this waits indefinitely for data
		if w.scanner.Scan() {
			line := w.scanner.Text()
			w.addToBuffer(line)
			return line, nil
		}

		// ❌ These lines are UNREACHABLE while Scan() is blocked
		time.Sleep(10 * time.Millisecond)
		if time.Now().After(deadline) {
			break
		}
	}

	return "", fmt.Errorf("timeout reading line (waited %v)", timeout)
}
```

**Why It Fails**:

1. `bufio.Scanner.Scan()` is a **blocking call** - it waits for data from the reader
2. When Claude enters zero-token waiting state, tmux control mode has **no output**
3. Scanner blocks indefinitely waiting for data from tmux stdout pipe
4. The timeout check `time.Now().After(deadline)` is **never reached** while blocked
5. The `consecutiveIdleLines` counter in `WaitForClaudePrompt()` **never increments**
6. Fallback idle detection (15 consecutive idles = 3 seconds) **never triggers**
7. Reaper stays stuck until the full 5-minute timeout or external intervention (Astrocyte ESC)

**Expected Behavior**:
- GetRawLine should timeout after 200ms if no data available
- consecutiveIdleLines should increment
- After 15 consecutive timeouts (3 seconds), prompt detection should conclude "ready"

**Actual Behavior**:
- GetRawLine blocks forever inside Scan()
- Never returns, never times out
- consecutiveIdleLines stays at 0
- Reaper waits full 5 minutes (or until Astrocyte sends ESC)

## Fix Strategy

### Option 1: Use ReadLine (Goroutine-Based Timeout)

The `output_watcher.go` file already has a `ReadLine()` function (lines 151-178) that uses a goroutine and channel to enforce timeout:

```go
func (w *OutputWatcher) ReadLine(timeout time.Duration) (string, error) {
	done := make(chan bool, 1)
	var line string

	go func() {
		if w.scanner.Scan() {
			line = w.scanner.Text()
			w.addToBuffer(line)
			done <- true
		} else {
			done <- false
		}
	}()

	select {
	case success := <-done:
		if success {
			return line, nil
		}
		return "", fmt.Errorf("EOF")
	case <-time.After(timeout):
		return "", fmt.Errorf("timeout reading line")
	}
}
```

**Advantages**:
- ✅ True timeout enforcement via `time.After()` in select
- ✅ Scanner runs in goroutine, can't block main flow
- ✅ Already exists in codebase

**Disadvantages**:
- ⚠️ Creates goroutine per line read (might be resource-intensive)
- ⚠️ Goroutine may stay blocked if scanner never returns

### Option 2: Fix GetRawLine with Context and Goroutine

Replace blocking Scan() with goroutine + context cancellation:

```go
func (w *OutputWatcher) GetRawLine(timeout time.Duration) (string, error) {
	result := make(chan string, 1)
	errChan := make(chan error, 1)

	go func() {
		if w.scanner.Scan() {
			result <- w.scanner.Text()
		} else if err := w.scanner.Err(); err != nil {
			errChan <- err
		} else {
			errChan <- fmt.Errorf("EOF")
		}
	}()

	select {
	case line := <-result:
		w.addToBuffer(line)
		return line, nil
	case err := <-errChan:
		return "", err
	case <-time.After(timeout):
		return "", fmt.Errorf("timeout reading line (waited %v)", timeout)
	}
}
```

**Advantages**:
- ✅ True timeout enforcement
- ✅ Consistent with ReadLine pattern
- ✅ Fixes the blocking issue

**Disadvantages**:
- ⚠️ Still creates goroutine per call
- ⚠️ Goroutine leak if Scan() never returns (edge case)

### Option 3: Use SetReadDeadline on Underlying Reader (Best)

If tmux control mode stdout is a net.Conn or os.File, we can set read deadline:

```go
// Before creating scanner, set deadline on underlying reader
if conn, ok := reader.(interface{ SetReadDeadline(time.Time) error }); ok {
	conn.SetReadDeadline(time.Now().Add(timeout))
}
```

**Advantages**:
- ✅ No goroutines needed
- ✅ Scanner respects underlying reader timeout
- ✅ Most efficient solution

**Disadvantages**:
- ⚠️ Requires access to underlying reader (might not be exposed)
- ⚠️ Only works if reader supports deadlines (net.Conn, os.File)

## Recommended Fix

**Immediate**: Replace `GetRawLine()` with `ReadLine()` in `WaitForClaudePrompt()`

**File**: `internal/tmux/prompt_detector.go:44`

**Change**:
```go
// Before (blocking):
line, err := watcher.GetRawLine(200 * time.Millisecond)

// After (timeout-enforced):
line, err := watcher.ReadLine(200 * time.Millisecond)
```

This is a **one-line fix** that uses the existing timeout-enforced ReadLine method instead of the broken GetRawLine.

## Testing Plan

1. Create test session with `agm test create hang-test`
2. Send command that produces output then goes idle: `echo "test" && sleep 5`
3. Verify prompt detection completes within 3 seconds (not 60+ seconds)
4. Test with skill invocation that has multi-line output
5. Verify reaper doesn't hang when Claude returns to prompt

## Additional Improvements

### 1. Reduce PromptDetectionTimeout

**Current**: 5 minutes is too long
**Recommended**: 30-60 seconds

Most commands complete within seconds. If no prompt after 60 seconds, likely stuck.

### 2. Add Heartbeat Logging

Add periodic debug logging in wait loop to diagnose hangs:

```go
lastLog := time.Now()
for time.Now().Before(deadline) {
	// Log every 10 seconds
	if time.Since(lastLog) > 10*time.Second {
		debug.Log("⏳ Still waiting for prompt... (checked %d lines, %d consecutive idles)",
			linesChecked, consecutiveIdleLines)
		lastLog = time.Now()
	}
	// ... existing logic
}
```

### 3. Detect Claude Internal Waiting State

Look for patterns that indicate Claude is waiting (not processing):
- No output for N seconds
- No prompt patterns detected
- %end notification seen but no prompt follows

Could add heuristic: "If %end seen and 10+ seconds of idle, assume stuck"

## Conclusion

**Root Cause**: `GetRawLine()` uses blocking `scanner.Scan()` which prevents timeout logic from working when there's no output.

**Fix**: Replace `GetRawLine()` with `ReadLine()` which uses goroutine + select for true timeout enforcement.

**Impact**: This is a **critical bug** that causes reaper to hang for up to 5 minutes when Claude enters zero-token waiting state. The fix is a **one-line change** with high impact.
