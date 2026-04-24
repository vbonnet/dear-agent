# Gemini CLI Readiness Detection

## Overview

This document describes the robust readiness detection implementation for Google Gemini CLI in agm-agent-wrapper, following the same pattern used for Claude.

## Problem Statement

Previously, agm-agent-wrapper used a simple 500ms delay when starting Gemini, which was:
- **Unreliable**: Too short on slow systems, unnecessarily long on fast ones
- **Fragile**: No verification that Gemini actually started
- **Poor UX**: Users might see uninitialized state or premature attachment

## Solution Architecture

The implementation uses a two-layer approach:

### Layer 1: Process Detection
```go
tmux.WaitForProcessReady(sessionName, "gemini", timeout)
```
Waits for the "gemini" process to appear in tmux pane's foreground process list.

**Limitations**:
- Only detects process start, not UI readiness
- Can't detect if Gemini is still initializing

### Layer 2: Prompt Detection (NEW)
```go
tmux.WaitForGeminiReady(sessionName, timeout)
```
Monitors tmux control mode output for Gemini's ready prompt patterns.

**Advantages**:
- Verifies Gemini UI is fully initialized
- More robust than timing-based detection
- Graceful degradation on timeout

## Gemini UI Patterns

Research revealed Gemini CLI has distinctive ready indicators:

### 1. Input Box
```
╭──────────────────────────────────────────────────────────────────────────────╮
│ >   Type your message or @path/to/file                                       │
╰──────────────────────────────────────────────────────────────────────────────╯
```

### 2. Status Bar
```
 ~/src                   no sandbox                    Auto (Gemini 2.5) /model
```

### 3. ASCII Banner (startup only)
```
   ███ ░░░     ███░    ███░███░░      ██████  ░██████░░███░░██████  ░█████  ███░
```

## Detection Patterns

The implementation looks for these patterns in tmux output:

```go
var GeminiPromptPatterns = []string{
    ">   Type your message",  // Gemini's input prompt text
    "@path/to/file",          // Part of Gemini's input prompt
    "╭─",                     // Box drawing characters (top)
    "╰─",                     // Box drawing characters (bottom)
}
```

### Pattern Matching Strategy

1. **Multiple Pattern Requirement**: Must see ≥2 patterns to confirm (reduces false positives)
2. **Idle Detection**: After seeing patterns, wait for output to stabilize
3. **Timeout with Graceful Degradation**: If detection fails, continue anyway (don't block user)

## Implementation Details

### WaitForGeminiReady Function

```go
func WaitForGeminiReady(sessionName string, timeout time.Duration) error
```

**Flow**:
1. Start tmux control mode session
2. Create output watcher to monitor %output lines
3. Parse tmux control mode output, unescape octal sequences
4. Count prompt pattern matches
5. Return when ≥2 patterns seen + output stabilizes
6. Or timeout with error (graceful degradation in caller)

**Key Features**:
- Uses existing `OutputWatcher` and `ControlModeSession` utilities
- Handles ANSI escape sequences via `stripANSI()`
- Debug logging for troubleshooting
- 500ms stability wait after detection

### Integration in agm-agent-wrapper

```go
// initGemini function
func initGemini(sessionName string) error {
    // 1. Start Gemini in tmux
    tmux.SendCommand(sessionName, "gemini && exit")

    // 2. Wait for process (Layer 1)
    tmux.WaitForProcessReady(sessionName, "gemini", timeout)

    // 2b. Wait for prompt (Layer 2) - NEW
    tmux.WaitForGeminiReady(sessionName, timeout)

    // 3. Extract UUID
    uuid, _ := extractGeminiUUID()

    // 4. Create ready-file
    readiness.CreateReadyFile(sessionName, manifestPath)

    return nil
}
```

## Alternative Approaches Considered

### 1. Window Title Status Icons
**Research Finding**: Gemini updates terminal window title with status:
- `◇` = Ready
- `✋` = Action Required
- `✦` = Working

**Why Not Used**:
- Harder to detect via tmux (requires OSC sequence parsing)
- Less reliable than visible output patterns
- Window title may not update in all terminals

### 2. API-Based Detection
**Research Finding**: No dedicated readiness API/flag in Gemini CLI

**Checked**:
- `gemini --help` - No `--ready-signal` flag
- `gemini --version` - No status command
- stdout/stderr - No "ready" message printed

### 3. File-Based Signals
**Why Not Used**: Gemini doesn't write ready-files like some daemons

### 4. Pure Timing-Based
**Why Rejected**: Original 500ms delay was unreliable

## Robustness Improvements Over Character Matching

While this implementation still uses pattern matching (like Claude), it's more robust:

1. **Multiple Pattern Confirmation**: Requires ≥2 patterns (not just 1 character)
2. **UI-Specific Patterns**: Box drawing + text = less likely false positive than generic ">"
3. **Stability Detection**: Waits for idle period after patterns seen
4. **Graceful Degradation**: Continues on timeout (doesn't block user)
5. **Debug Logging**: Helps diagnose issues in production

## Testing

### Unit Tests
```bash
go test ./internal/tmux -run TestContainsGeminiPromptPattern
go test ./internal/tmux -run TestGeminiPromptPatterns
```

### Integration Testing
1. Start Gemini session: `agm-agent-wrapper --harness=gemini-cli test-session`
2. Check debug logs: `AGM_DEBUG=1 agm-agent-wrapper --harness=gemini-cli test-session`
3. Verify no premature attachment
4. Confirm prompt is visible when attached

## Limitations & Future Work

### Current Limitations
1. **Pattern Fragility**: If Gemini changes UI, patterns break
2. **No API Access**: Can't query Gemini's internal state
3. **Character Encoding**: Unicode box drawing might render differently

### Future Improvements
1. **Add Fallback Patterns**: Support multiple UI versions
2. **Window Title Parsing**: Parse OSC sequences for status icons
3. **Contribute to Gemini CLI**: Request `--ready-signal` flag upstream
4. **Regex Patterns**: More flexible matching (current uses exact strings)

## References

- [Gemini CLI Documentation](https://geminicli.com/docs/get-started/)
- [Gemini CLI GitHub](https://github.com/google-gemini/gemini-cli)
- [Gemini CLI Issue #10175](https://github.com/google-gemini/gemini-cli/issues/10175) - Initialization timing issue
- Claude's `WaitForClaudeReady` implementation (internal/tmux/prompt_detector.go)
- tmux control mode documentation

## Sources

Research utilized:
- [Gemini CLI Documentation](https://docs.cloud.google.com/gemini/docs/codeassist/gemini-cli)
- [GitHub Repository](https://github.com/google-gemini/gemini-cli)
- [Hands-on with Gemini CLI Codelab](https://codelabs.developers.google.com/gemini-cli-hands-on)
- Direct testing with Gemini CLI v0.26.0+
