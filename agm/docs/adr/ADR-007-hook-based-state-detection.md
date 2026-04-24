# ADR-007: Hook-Based State Detection

## Status
**Accepted** (2026-02-02)

## Context

AGM needs to detect when Claude sessions transition between states (DONE → WORKING, WORKING → DONE, etc.) to enable state-aware message delivery. Sessions should only receive messages when DONE, not when WORKING or COMPACTING.

**Requirements:**
- Detect state transitions in real-time (within seconds)
- No polling overhead (inefficient to check every session every second)
- Accurate state detection (no false positives/negatives)
- Works with Claude Code's built-in state management
- Minimal changes to existing session workflow

**States to Detect:**
- **DONE:** Session idle, can receive messages
- **WORKING:** Session processing a message, defer delivery
- **COMPACTING:** Session compacting context, defer delivery
- **OFFLINE:** Session not running, retry later

**Constraints:**
- Must integrate with Claude Code hooks (SessionStart:compact, etc.)
- Cannot modify Claude Code binary
- Must work with existing manifest.json format
- No external dependencies

## Decision

**Implement hook-based state detection using Claude Code's hook system + manifest tracking.**

### Architecture

**Hook Integration:**
```bash
# ~/.claude/hooks/SessionStart:compact.sh
#!/bin/bash
agm session update-state "${CLAUDE_SESSION_NAME}" COMPACTING "compact-hook"
```

**State Detection Function:**
```go
func DetectState(sessionName string) (StateType, error) {
    manifest, err := LoadManifest(sessionName)
    if err != nil {
        return StateOffline, nil  // Session not running
    }

    // Check hook-updated state field
    switch manifest.State {
    case "DONE":
        return StateDone, nil
    case "WORKING":
        return StateWorking, nil
    case "COMPACTING":
        return StateCompacting, nil
    default:
        return StateOffline, nil
    }
}
```

**Manifest Schema Addition:**
```json
{
  "version": 3,
  "state": "DONE",
  "state_updated_at": "2026-02-02T14:30:00Z",
  "state_updated_by": "compact-hook"
}
```

## Alternatives Considered

### 1. Polling-Based State Detection
**Implementation:** Daemon polls every 1s, checks tmux pane content for Claude prompt
**Pros:** Simple, no hooks required
**Cons:** High overhead (N sessions × 1s = high CPU), fragile (depends on prompt format)
**Rejected:** Doesn't scale beyond 10 sessions, unreliable

### 2. tmux Control Mode Event Streaming
**Implementation:** Listen to `%output` events from tmux control mode
**Pros:** Real-time, no polling
**Cons:** Complex parsing, race conditions, requires separate daemon per session
**Rejected:** Too complex, fragile with concurrent tmux commands

### 3. File-Based State Signals (PID Files)
**Implementation:** Sessions write state to `~/.agm/state-{session-name}` files
**Pros:** Simple, no hooks
**Cons:** Manual cleanup, stale files, no atomic updates
**Rejected:** Brittle, requires cleanup logic

### 4. Process-Based Detection (Check Claude PID)
**Implementation:** Parse `ps aux` to see if Claude is running
**Pros:** No hooks
**Cons:** Can't distinguish DONE vs WORKING, platform-specific
**Rejected:** Insufficient granularity

## Consequences

### Positive
✅ **Real-time:** State changes detected immediately when hooks fire
✅ **Low overhead:** No polling, hooks only fire on actual state changes
✅ **Accurate:** Hooks fire at exact transition points (e.g., SessionStart:compact)
✅ **Integrated:** Uses Claude Code's existing hook system (no new infrastructure)
✅ **Auditable:** `state_updated_by` tracks which hook/component changed state
✅ **Scalable:** Works for 1 or 100 sessions without performance degradation

### Negative
❌ **Hook dependency:** Requires hooks to be installed correctly
❌ **Manual setup:** Users must install hooks (documented in setup guide)
❌ **Missed transitions:** If hook fails, state may be stale (mitigated by fallback logic)

### Neutral
🔵 **Manifest writes:** Each state change writes manifest file (acceptable frequency)
🔵 **Hook failures:** Need error handling for failed hook executions

## Implementation

### Key Files
- `internal/session/state.go` (150 lines)
- `internal/session/state_test.go` (220 lines)
- Hooks: `~/.claude/hooks/SessionStart:compact.sh`, etc.

### State Detection Logic
```go
func DetectState(sessionName string) (StateType, error) {
    // 1. Load manifest
    manifest, _, err := session.ResolveIdentifier(sessionName, "")
    if err != nil {
        return StateOffline, nil  // Session doesn't exist
    }

    // 2. Check if tmux session exists
    if !tmux.SessionExists(manifest.Tmux.SessionName) {
        return StateOffline, nil
    }

    // 3. Check manifest state field (set by hooks)
    switch manifest.State {
    case StateDone:
        return StateDone, nil
    case StateWorking:
        return StateWorking, nil
    case StateCompacting:
        return StateCompacting, nil
    default:
        // Fallback: if session exists but state unknown, assume WORKING
        return StateWorking, nil
    }
}
```

### Hook Installation
```bash
# agm admin doctor --fix
# Installs hooks to ~/.claude/hooks/
install -m 755 hooks/SessionStart:compact.sh ~/.claude/hooks/
```

### State Update Function
```go
func UpdateSessionState(manifestPath string, newState StateType, updatedBy string) error {
    manifest, err := LoadManifestFile(manifestPath)
    if err != nil {
        return err
    }

    manifest.State = newState
    manifest.StateUpdatedAt = time.Now()
    manifest.StateUpdatedBy = updatedBy

    return SaveManifestFile(manifestPath, manifest)
}
```

## Validation

### Functional Tests
- ✅ State transitions from DONE → WORKING → DONE
- ✅ Hook failures don't crash daemon
- ✅ Offline sessions detected correctly
- ✅ Concurrent state updates are atomic (file lock)

### Integration Tests
- ✅ Daemon defers messages when session is WORKING
- ✅ Daemon delivers messages when session transitions to DONE
- ✅ State persists across daemon restarts

### Hook Tests
- ✅ SessionStart:compact hook sets COMPACTING state
- ✅ Hook execution errors logged but don't block workflow

## Related Decisions
- **ADR-006:** Message Queue Architecture (uses state detection for delivery logic)
- **ADR-008:** Status Aggregation (queries state for dashboard)
- **Phase 2 Task 2.1:** Delivery Daemon (implements state-aware routing)

## Migration Notes

**Backward Compatibility:**
- Manifest v3 adds `state`, `state_updated_at`, `state_updated_by` fields
- Old manifests without state field default to WORKING (safe fallback)
- Hooks are optional (graceful degradation if not installed)

**Installation:**
```bash
agm admin doctor
# Checks: ✓ Hooks installed, ✓ Hook permissions, ✓ Hook syntax
agm admin doctor --fix
# Fixes: Installs missing hooks
```

**Future Enhancements:**
- State transition logging (audit trail)
- Webhook support for external state listeners
- State change notifications (desktop alerts)

---

**Deciders:** Foundation Engineering
**Date:** 2026-02-02
**Implementation:** Phase 1, Task 1.2
**Bead:** oss-bqz4 (closed)
