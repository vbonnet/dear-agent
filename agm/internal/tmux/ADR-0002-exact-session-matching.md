# ADR-0002: Exact Session Matching in Tmux Commands

**Status**: Accepted
**Date**: 2026-03-18
**Context**: Fix astrocyte tmux prefix matching bug

## Context

Tmux uses prefix matching by default when targeting sessions with `-t session-name`. This caused critical bugs where commands intended for one session would target a different session with a matching prefix.

### Bug Example

```bash
# Two sessions exist: "astrocyte" and "astrocyte-improvements"
tmux kill-session -t astrocyte  # Kills "astrocyte"
tmux send-keys -t astrocyte "test"  # ERROR: Targets "astrocyte-improvements" instead!
```

**Impact**: Astrocyte daemon was sending ESC keys to wrong sessions, interrupting active work.

## Investigation

### Tmux 3.4 Exact Matching Behavior

Testing revealed tmux 3.4 has **two different behaviors** for exact matching:

#### 1. Session-Level Commands (= prefix WORKS)
```bash
tmux has-session -t =session-name    # ✅ Works - exact match
tmux kill-session -t =session-name   # ✅ Works - exact match
tmux list-clients -t =session-name   # ✅ Works - exact match
```

#### 2. Pane-Level Commands (= prefix FAILS)
```bash
tmux send-keys -t =session-name      # ❌ Error: "can't find pane: =session-name"
tmux capture-pane -t =session-name   # ❌ Error: "can't find pane: =session-name"
tmux display-message -t =session-name # ❌ Error: "can't find pane: =session-name"
tmux paste-buffer -t =session-name   # ❌ Error: "can't find pane: =session-name"
```

**Root Cause**: Pane-level commands in tmux 3.4 don't support exact matching syntax.

## Decision

Use exact matching (`=session-name`) **only for session-level commands**:

### Go Implementation

```go
// FormatSessionTarget formats a session name for exact matching.
// IMPORTANT: This ONLY works for session-level commands:
//   - has-session, kill-session, list-sessions, list-clients, etc.
//
// For pane-level commands (send-keys, capture-pane), the = prefix does NOT work in tmux 3.4.
// Those commands should use plain session names and rely on session validation via HasSession.
func FormatSessionTarget(sessionName string) string {
    return "=" + sessionName
}

// Usage:
tmux.RunCommand("has-session", "-t", FormatSessionTarget(name))   // ✅ Use =prefix
tmux.RunCommand("send-keys", "-t", normalizedName, "text")         // ✅ Plain name
```

### Python Implementation

```python
# Session-level: Use = prefix
subprocess.run(["tmux", "-S", socket_path, "has-session", "-t", f"={session_name}"])

# Pane-level: Use plain name (no = prefix)
subprocess.run(["tmux", "-S", socket_path, "send-keys", "-t", session_name, "Escape"])
subprocess.run(["tmux", "-S", socket_path, "capture-pane", "-t", session_name, "-p"])
```

## Command Classification

### Session-Level (use `=prefix`)
- `has-session`
- `kill-session`
- `list-sessions`
- `list-clients`
- `list-panes` (when using session target)

### Pane-Level (use plain name)
- `send-keys`
- `capture-pane`
- `display-message`
- `paste-buffer`

## Safety Mechanism

**Two-layer protection**:
1. **Session validation**: Always call `HasSession()` before pane operations
2. **Name normalization**: Convert dots/colons to dashes via `NormalizeTmuxSessionName()`

```go
// Validate session exists (uses = prefix for exact match)
if err := HasSession(sessionName); err != nil {
    return fmt.Errorf("session not found: %w", err)
}

// Normalize name (dots → dashes)
normalizedName := NormalizeTmuxSessionName(sessionName)

// Safe to use plain name for pane operations
RunCommand("send-keys", "-t", normalizedName, "text")
```

## Files Modified

### Go
- `internal/tmux/tmux.go` - Added `FormatSessionTarget()`, updated session-level commands
- `cmd/agm/kill.go` - Applied exact matching to `killTmuxSession()` function (2026-03-21)
- `internal/tmux/control.go` - Session-level: `has-session`; Pane-level: `send-keys`, `capture-pane`
- `internal/tmux/prompt.go` - Removed `=` from pane commands
- `internal/tmux/send.go` - Removed `=` from pane commands
- `internal/tmux/init_sequence.go` - Removed `=` from pane commands
- `internal/tmux/pane_monitor.go` - Removed `=` from pane commands
- `internal/tmux/capture.go` - Removed `=` from pane commands

### Python
- `astrocyte/astrocyte.py` - Fixed 5 commands (session-level kept `=`, pane-level removed)
- `astrocyte/astrocyte_ctrlc_recovery.py` - Removed `=` from pane commands

## Testing

### Go Tests
- All tests pass (110s runtime)
- Verified pane commands work without `=` prefix
- Verified session commands work with `=` prefix

### Python Tests
- 38 passed, 2 xfailed (pre-existing endpoint detection features)
- Test expectations updated to match actual behavior

## Consequences

### Positive
- ✅ Bug fixed: Commands no longer target wrong sessions
- ✅ Clear documentation of tmux behavior
- ✅ Consistent pattern across Go and Python
- ✅ Safety mechanism prevents future issues

### Negative
- ⚠️ Different syntax for session-level vs pane-level commands
- ⚠️ Developers must understand the distinction

## Alternatives Considered

1. **Use `=` prefix everywhere**: Rejected - doesn't work for pane commands in tmux 3.4
2. **Never use `=` prefix**: Rejected - loses exact matching protection for session-level commands
3. **Upgrade to newer tmux**: Rejected - can't control user environments

## References

- Bug report: Astrocyte daemon sending ESC to wrong sessions
- Tmux issue: https://github.com/tmux/tmux/issues/1778 (send-keys literal mode)
- Plan: `~/.claude/plans/glittery-drifting-scroll.md`
