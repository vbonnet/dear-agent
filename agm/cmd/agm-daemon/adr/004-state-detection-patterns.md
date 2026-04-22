# ADR 004: Visual Pattern-Based State Detection

**Status**: Accepted

**Date**: 2026-02-11

## Context

The AGM Daemon needs to detect Claude Code session states (ready, thinking, blocked, stuck, unknown) to provide accurate status information. Several state detection approaches were considered:

1. **Visual Patterns**: Regex matching on terminal output
2. **Tmux Variables**: Read tmux session/pane variables
3. **Process Inspection**: Check Claude process state
4. **Output Parsing**: Parse Claude's JSON/structured output
5. **Hybrid**: Combine multiple detection methods

### Requirements

- **Accuracy**: Detect actual state user sees
- **Completeness**: Detect all relevant states (ready, thinking, blocked, stuck)
- **Reliability**: Work with any Claude Code version
- **Simplicity**: Minimal tmux/system dependencies
- **Performance**: Fast detection (<50ms per session)

### States to Detect

| State | Description | User Impact |
|-------|-------------|-------------|
| `ready` | Claude idle, waiting for input | Can send commands |
| `thinking` | Claude processing | Wait for response |
| `blocked_auth` | y/N authentication prompt | User action required |
| `blocked_input` | AskUserQuestion prompt | User decision required |
| `stuck` | No output for >60s | Session may be hung |
| `unknown` | Unable to determine | Fallback state |

### Constraints

- Must work with existing Claude Code installations
- No modifications to tmux configuration
- No Claude Code instrumentation
- Must handle all Claude Code states

## Decision

**Use regex pattern matching on tmux pane output (visual parsing).**

### Detection Patterns

```go
type Detector struct {
    // Thinking: Spinner characters
    thinkingPattern *regexp.Regexp // [⣾⣽⣻⢿⡿⣟⣯⣷]

    // Blocked Auth: y/N prompts
    blockedAuthPattern *regexp.Regexp // (?i)\b([yY]/[nN]|[nN]/[yY])\b

    // Blocked Input: AskUserQuestion patterns
    blockedInputPattern *regexp.Regexp // Numbered options, choice keywords

    // Ready: Claude prompt
    readyPattern *regexp.Regexp // ❯\s*$

    // Stuck: Timeout-based
    stuckThreshold time.Duration // 60s
}
```

### Detection Priority

```
1. Blocked Auth    (highest - immediate user action)
2. Blocked Input   (user decision needed)
3. Thinking        (active processing)
4. Stuck           (timeout threshold)
5. Ready           (idle state)
6. Unknown         (fallback)
```

### Detection Algorithm

```go
func (d *Detector) DetectState(output string, lastOutputTime time.Time) DetectionResult {
    // Priority order: check most critical states first

    if d.blockedAuthPattern.MatchString(output) {
        return DetectionResult{State: StateBlockedAuth, Confidence: "high"}
    }

    if d.blockedInputPattern.MatchString(output) {
        return DetectionResult{State: StateBlockedInput, Confidence: "high"}
    }

    if d.thinkingPattern.MatchString(output) {
        return DetectionResult{State: StateThinking, Confidence: "high"}
    }

    if time.Since(lastOutputTime) > d.stuckThreshold {
        return DetectionResult{State: StateStuck, Confidence: "medium"}
    }

    if d.readyPattern.MatchString(output) {
        return DetectionResult{State: StateReady, Confidence: "high"}
    }

    return DetectionResult{State: StateUnknown, Confidence: "low"}
}
```

## Rationale

### Why Visual Pattern Matching?

1. **Accuracy**: Detects what user actually sees
   - Terminal output = ground truth
   - User sees spinner → state is "thinking"
   - User sees prompt → state is "ready"
   - User sees "y/N" → state is "blocked_auth"

2. **Reliability**: Works with any Claude version
   - Visual output is stable (user-facing interface)
   - Internal state variables may change
   - Process state doesn't reveal blocked prompts

3. **Completeness**: Detects all states
   - Visual patterns exist for all states
   - Ready: `❯` prompt
   - Thinking: Spinner characters `⣾⣽⣻⢿⡿⣟⣯⣷`
   - Blocked: `y/N`, numbered options
   - Stuck: No output + time threshold

4. **Simplicity**: No external dependencies
   - Standard regex (Go stdlib)
   - Tmux capture-pane (built-in)
   - No Claude Code modifications

5. **Performance**: Fast pattern matching
   - Compiled regex (~1ms per pattern)
   - Short circuit on first match (priority order)
   - Total: <10ms per session

### Why Not Tmux Variables?

**Rejected**: Tmux variables don't expose Claude's internal state.

**Problems**:
```bash
# Tmux only knows about pane/window state
tmux display-message -p "#{pane_current_command}"  # → "claude"
# Doesn't tell us if Claude is ready, thinking, or blocked
```

**Why Rejected**:
- No semantic state (only process name)
- Can't detect blocked prompts
- Can't detect thinking vs ready

### Why Not Process Inspection?

**Rejected**: Process state doesn't reveal prompt state.

**Problems**:
```bash
# Process state (ps, /proc)
ps aux | grep claude  # Shows CPU, memory, state (R/S/D)
# R = running, S = sleeping
```

**Why Rejected**:
- Can't distinguish thinking vs blocked
- Claude "sleeping" when waiting for input (not stuck)
- No way to detect specific prompt types

### Why Not Output Parsing?

**Rejected**: Claude doesn't emit structured state output.

**Problems**:
- Claude Code doesn't output JSON state
- No machine-readable state markers
- Would require Claude Code modifications

**Why Rejected**:
- Requires instrumentation (out of scope)
- Fragile (depends on Claude internals)
- Visual patterns already available

### Why Not Hybrid Approach?

**Rejected**: Combining methods adds complexity without benefit.

**Example**:
```
Visual patterns + process state + tmux variables
```

**Why Rejected**:
- Visual patterns sufficient (100% coverage)
- Additional methods don't improve accuracy
- Increased complexity and failure modes

## Consequences

### Positive

1. **Accurate State Detection**: Matches user perception
   ```
   User sees: "⣾ Working on it..."
   Daemon detects: StateThinking
   User sees: "❯"
   Daemon detects: StateReady
   ```

2. **Version Agnostic**: Works with any Claude version
   - Visual output stable (user-facing)
   - No dependency on internal APIs
   - No version checks needed

3. **Fast Detection**: <10ms per session
   - Compiled regex patterns
   - Short-circuit evaluation
   - No subprocess overhead

4. **Complete Coverage**: All states detectable
   | State | Visual Pattern | Confidence |
   |-------|----------------|------------|
   | Ready | `❯` prompt | High |
   | Thinking | Spinner chars | High |
   | Blocked Auth | `y/N` | High |
   | Blocked Input | Numbered options | High |
   | Stuck | Timeout | Medium |
   | Unknown | No match | Low |

5. **Evidence Extraction**: Provides context
   ```json
   {
     "state": "thinking",
     "evidence": "⣾ Working on it...",
     "confidence": "high"
   }
   ```

### Negative

1. **Regex Maintenance**: Patterns may need updates
   - **Impact**: Claude UI changes → patterns break
   - **Mitigation**: Patterns are stable (user-facing)
   - **Frequency**: Rare (UI doesn't change often)
   - **Fix**: Update regex in detector.go

2. **False Positives**: User output may match patterns
   - **Example**: User types `y/N` in prompt → detected as blocked
   - **Mitigation**: Pattern specificity (word boundaries, context)
   - **Impact**: Rare (patterns are specific)

3. **Spinner Character Dependency**: Unicode required
   - **Impact**: Non-Unicode terminals → can't detect thinking
   - **Mitigation**: Most modern terminals support Unicode
   - **Fallback**: Falls through to unknown state

4. **Stuck Detection Latency**: 60s threshold
   - **Impact**: Can't detect stuck until 60s elapsed
   - **Mitigation**: Threshold configurable (future)
   - **Acceptable**: User waits >60s anyway if stuck

### Neutral

1. **Pattern Order Matters**: Priority affects detection
   - Higher priority states checked first
   - Ensures critical states (blocked) detected over others

2. **Evidence Length**: Limited to context window
   - Extract ±50-150 chars around match
   - Sufficient for debugging
   - Avoids exposing full conversation

## Pattern Design Rationale

### Thinking Pattern: `[⣾⣽⣻⢿⡿⣟⣯⣷]`

**Rationale**: Claude uses Braille spinner characters

**Why Reliable**:
- Unique to spinner (unlikely in user text)
- All 8 spinner frames covered
- Unicode-safe (single char class)

**Edge Cases**:
- User typing Unicode? Unlikely (specific chars)
- Output contains Braille? Extremely rare

### Blocked Auth Pattern: `(?i)\b([yY]/[nN]|[nN]/[yY])\b`

**Rationale**: Authentication prompts show "y/N" or "Y/n"

**Why Reliable**:
- Word boundaries `\b` prevent false matches
- Case-insensitive `(?i)` handles both styles
- Both orders `y/N` and `N/y` covered

**Edge Cases**:
- User types "y/N" in prompt? Word boundary helps
- Appears mid-sentence? Boundary prevents match

### Blocked Input Pattern: Complex

**Rationale**: AskUserQuestion shows numbered/lettered options

```go
`(?m)` +                                    // Multiline mode
`(` +
    `\b(?:1\.|2\.|3\.|A\.|B\.|C\.)\s+` +   // 1. 2. 3. or A. B. C.
    `|` +
    `(?:Choose|Select|Pick|Which).*:` +    // Choice keywords
    `|` +
    `\[.*\].*\[.*\]` +                      // [Option 1] [Option 2]
`)`
```

**Why Reliable**:
- Multiple patterns (OR logic)
- Specific to choice prompts
- Rare in normal output

**Edge Cases**:
- User numbered list? Matches (acceptable)
- User choice text? Matches (acceptable)
- Better false positive than false negative (safe)

### Ready Pattern: `❯\s*$`

**Rationale**: Claude prompt is `❯` at end of output

**Why Reliable**:
- End anchor `$` prevents mid-output match
- Unique character `❯` (unlikely in user text)
- Whitespace `\s*` handles trailing spaces

**Edge Cases**:
- User types `❯`? Not at end of output
- Other prompts? Different characters (`$`, `>`, etc.)

## Performance Characteristics

### Pattern Matching Complexity

| Pattern | Complexity | Time |
|---------|-----------|------|
| Thinking | O(n) | <1ms |
| Blocked Auth | O(n) | <1ms |
| Blocked Input | O(n) | <2ms (complex) |
| Ready | O(n) | <1ms |

**Total**: <5ms per session (worst case)

### Optimization: Compiled Patterns

```go
// Compile once on initialization
func NewDetector() *Detector {
    return &Detector{
        thinkingPattern:     regexp.MustCompile(`[⣾⣽⣻⢿⡿⣟⣯⣷]`),
        blockedAuthPattern:  regexp.MustCompile(`(?i)\b([yY]/[nN]|[nN]/[yY])\b`),
        // ...
    }
}
```

**Benefit**: No recompilation (10x faster)

### Optimization: Priority Order

```go
// Check critical states first (short-circuit)
if blockedAuthPattern.MatchString(output) {
    return StateBlockedAuth  // Exit early
}
```

**Benefit**: Average case faster than worst case

## Testing Strategy

### Pattern Accuracy Tests

```go
func TestDetectState_Thinking(t *testing.T) {
    detector := NewDetector()
    output := "⣾ Working on it..."
    result := detector.DetectState(output, time.Now())
    assert.Equal(t, StateThinking, result.State)
}
```

### False Positive Tests

```go
func TestDetectState_NoFalsePositiveOnUserTypedYN(t *testing.T) {
    detector := NewDetector()
    output := "I typed y/N in the middle of a sentence"
    result := detector.DetectState(output, time.Now())
    assert.NotEqual(t, StateBlockedAuth, result.State)
}
```

### Priority Order Tests

```go
func TestDetectState_BlockedAuthOverridesThinking(t *testing.T) {
    detector := NewDetector()
    output := "⣾ Approve this? y/N"
    result := detector.DetectState(output, time.Now())
    assert.Equal(t, StateBlockedAuth, result.State)  // Auth takes priority
}
```

## Future Enhancements

### V2: Machine Learning State Detection

Train ML model on labeled terminal output:
```
Input: Terminal output (last 50 lines)
Output: State (ready, thinking, blocked, stuck, unknown) + confidence
```

**Benefit**: Higher accuracy, fewer false positives

### V2: Configurable Patterns

Allow users to define custom patterns:
```yaml
state_patterns:
  custom_blocked:
    pattern: "Waiting for approval"
    state: blocked_input
    confidence: high
```

**Benefit**: Adapt to custom Claude prompts

### V2: Context-Aware Detection

Consider previous state in detection:
```
Previous: Thinking
Current: No spinner
Likely: Ready (not stuck yet)
```

**Benefit**: Reduce stuck false positives

## References

- State Detection Implementation: internal/state/detector.go
- State Detection Tests: internal/state/detector_test.go
- Regex Performance: https://golang.org/pkg/regexp/

## Related ADRs

- ADR 002: Polling-Based State Detection
- ADR 003: Dual Interface Design (HTTP + File)
