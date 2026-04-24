# Investigation: Reaper False Positive Prompt Detection

## Issue Summary

**Incident**: 2026-02-02T21:21:36Z - Reaper failed to archive session after sending /exit

**Symptom**: Pane never closed after /exit was sent, despite reaper detecting prompt successfully

**Log Evidence**: `/tmp/csm-reaper-agentic-standards-2026.log`
```
2026/02/02 21:21:39.061498 ✓ Prompt detected - Claude is ready
2026/02/02 21:21:39.076144 ✓ /exit sent successfully
2026/02/02 21:21:39.076182 ⏳ Waiting for pane to close...
2026/02/02 21:22:09.139007 ❌ Reaper failed: pane did not close within 30s
```

**Timeline**:
- 21:21:36 - Reaper spawned by /csm-exit skill
- 21:21:39 - Prompt detected (3 seconds later)
- 21:21:39 - /exit sent
- 21:22:09 - Reaper timeout (pane still active)
- 21:27:49 - Claude hung mid-sentence during completion message

**Total duration**: Claude was generating output for ~6 minutes (21:21:36 to 21:27:49)

## Root Cause Hypothesis: False Positive Prompt Detection

### Current Prompt Detection Logic

`WaitForClaudePrompt()` in `internal/tmux/prompt_detector.go`:

**Prompt Patterns** (lines 12-18):
```go
var ClaudePromptPatterns = []string{
	"❯",  // Claude Code primary prompt
	"▌",  // Claude cursor
	"> ", // Common prompt
	"> ", // Shell-style prompt
	"# ", // Root prompt
}
```

**Detection Code Path** (lines 98-103):
```go
// Check for prompt patterns
if containsPromptPattern(content) {
	debug.Log("✓ Prompt pattern detected in line %d: %q", linesChecked, content)
	// Wait a bit more to ensure it's stable (increased to 2s to avoid false positives)
	time.Sleep(2 * time.Second)
	return nil  // ← Returns immediately after 2s sleep
}
```

### Problem

1. **Patterns are too generic**: `"> "`, `"$ "`, `"# "` can appear in Claude's normal output:
   - Markdown blockquotes: `> This is a quote`
   - Code examples: `$ npm install`, `# root command`
   - Shell script examples in responses

2. **Insufficient validation**: After detecting pattern, code only sleeps 2 seconds before returning success
   - No verification that prompt is stable
   - No check that Claude stopped generating output
   - Sleep can occur while Claude continues generating

3. **Race condition window**:
   ```
   T+0s:   Detect "> " pattern in Claude's output
   T+0s:   Sleep for 2 seconds
   T+0s-2s: Claude continues generating completion message
   T+2s:   Return "prompt detected" (false positive)
   T+2s:   Reaper sends /exit (Claude still generating, /exit queued)
   T+2s+:  Claude never processes /exit because it never returns to prompt
   ```

### Evidence Supporting Hypothesis

1. **Only 15ms between detection and /exit**: Too fast for Claude to start new output
   - This suggests prompt was detected while Claude was already generating output
   - False positive in ongoing output stream

2. **Claude continued generating for 6 minutes after "prompt detected"**:
   - If prompt detection was accurate, Claude should have been idle
   - Instead, Claude generated completion message for 6 more minutes before hanging

3. **Reaper log shows pane remained active**:
   - 59 checks over 30 seconds, pane never closed
   - /exit was sent but never processed
   - Suggests /exit sat in input buffer waiting for prompt that never came

## Proposed Fixes

### Option 1: More Conservative Prompt Detection (Recommended)

**Change lines 98-103** to require idle period confirmation:

```go
// Check for prompt patterns
if containsPromptPattern(content) {
	// DON'T return immediately - let idle detection confirm
	// Just note that we saw a pattern
	promptPatternsSeen++
	debug.Log("📋 Potential prompt pattern in line %d: %q (total: %d)",
		linesChecked, content, promptPatternsSeen)
	continue  // ← Keep monitoring instead of returning
}
```

**Rely on idle detection** (lines 61-64, 68-71) which requires:
- 10+ consecutive idle reads (2 seconds) after seeing prompt pattern
- OR 15+ consecutive idle reads (3 seconds) regardless of pattern

**Benefits**:
- Eliminates false positives from prompt patterns in output
- Ensures Claude has actually stopped generating before returning
- Idle period proves Claude is at prompt, not just outputting text containing patterns

### Option 2: More Specific Prompt Patterns

**Require context around patterns**:

```go
var ClaudePromptPatterns = []string{
	"❯",          // Claude Code primary prompt (unique Unicode)
	"▌",          // Claude cursor (unique Unicode)
	// Remove generic patterns that appear in output:
	// "> ", "$ ", "# "
}

// Additional check for shell prompts - require start of line + trailing space
func containsPromptPattern(content string) bool {
	trimmed := strings.TrimSpace(content)

	// Check Unicode prompts (high confidence)
	for _, pattern := range []string{"❯", "▌"} {
		if strings.Contains(trimmed, pattern) {
			return true
		}
	}

	// For ASCII prompts, require they're at start of line
	for _, pattern := range []string{"> ", "$ ", "# "} {
		if strings.HasPrefix(trimmed, pattern) {
			// Also check it's the ONLY content (not followed by more text)
			if len(trimmed) <= len(pattern)+5 {  // Allow prompt + short command
				return true
			}
		}
	}

	return false
}
```

**Benefits**:
- Reduces false positives by requiring patterns at line start
- Prioritizes unique Unicode patterns (❯, ▌) over generic ASCII
- Still detects actual prompts reliably

### Option 3: Verify Prompt Stability with Additional Read

**After detecting pattern, verify it persists**:

```go
if containsPromptPattern(content) {
	debug.Log("📋 Prompt pattern detected, verifying stability...")

	// Wait 2 seconds
	time.Sleep(2 * time.Second)

	// Read next line - should timeout (idle) if truly at prompt
	line, err := watcher.ReadLine(1 * time.Second)
	if err != nil {
		// Timeout = no new output = stable prompt
		debug.Log("✓ Prompt confirmed (stable)")
		return nil
	} else {
		// Got new output = false positive, continue monitoring
		debug.Log("⚠ False positive (still generating): %q", line)
		continue
	}
}
```

**Benefits**:
- Confirms prompt is stable before returning
- Catches cases where Claude continues generating after pattern
- Low overhead (1-2 second additional wait)

## Recommended Implementation

**Combine Option 1 + Option 2**:

1. Make prompt patterns more specific (Option 2)
2. Remove immediate return after pattern detection (Option 1)
3. Rely on idle period detection to confirm Claude is truly at prompt

This provides defense in depth:
- Fewer false positives from pattern matching
- Idle period requirement prevents race conditions
- Compatible with existing timeout logic

## Testing Plan

1. **Unit Tests**: Create test cases with output containing prompt-like patterns
   - Claude output with markdown quotes: `> This is a quote`
   - Claude output with shell examples: `$ npm install`
   - Verify function doesn't return early on these false positives

2. **Integration Tests**: Use `csm test` to create test scenarios
   - Session with slow Claude responses
   - Session with completion messages containing prompt patterns
   - Verify reaper successfully detects prompt and exits

3. **Regression Test**: Reproduce original incident
   - Spawn reaper during long completion message
   - Verify reaper doesn't detect false positive
   - Verify /exit successfully closes pane

## Related Files

- `internal/tmux/prompt_detector.go` - Prompt detection logic
- `internal/reaper/reaper.go` - Reaper workflow
- `~/.agm/astrocyte/diagnoses/agentic-standards-2026-2026-02-02T21-27-49.md` - Original incident diagnosis

## Status

- [x] Blocking scanner bug fixed (commit 78fa160)
- [ ] False positive prompt detection issue - **requires further investigation**
- [ ] Integration tests for reaper workflow
- [ ] Proposed fixes for prompt detection logic

## Next Steps

1. Review proposed fixes with maintainer
2. Implement chosen fix (recommend Option 1 + Option 2)
3. Add unit tests for prompt detection edge cases
4. Run integration tests with `csm test`
5. Deploy and monitor for reaper success rate

---

**Investigated by**: Claude Sonnet 4.5
**Date**: 2026-02-02
**Commit**: 78fa160 (fixed blocking scanner bug)
**Remaining work**: Fix false positive prompt detection
