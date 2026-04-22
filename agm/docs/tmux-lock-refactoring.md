# Tmux Lock Refactoring

## Problem

The global command lock (`/tmp/agm-{UID}/csm.lock`) was too coarse-grained:

1. **Locked entire AGM command execution** - from PersistentPreRunE to PersistentPostRunE
2. **Prevented concurrent operations** - `agm session list` couldn't run while `agm session resume` was executing
3. **Caused deadlocks** - Had to release lock early in several places to prevent:
   - `/agm:agm-assoc` skill from deadlocking (it runs `agm session associate` internally)
   - `AttachSession` from blocking indefinitely while holding the lock
4. **Wrong granularity** - The actual race condition is only in **tmux server updates**, not all AGM operations

## Root Cause

From user investigation: "The problem was with updating the tmux server too quickly in parallel"

Specifically, these tmux operations conflict when run in parallel:
```go
// In NewSession (tmux.go:66-82)
settings := []tmuxSetting{
    {"set-window-option", "-t", name, "aggressive-resize", "on"},
    {"set-option", "-t", name, "window-size", "latest"},
    {"set", "-t", name, "mouse", "on"},
    {"set", "-s", "set-clipboard", "on"},
}
```

These tmux commands update shared server state and can interleave if multiple `agm session new` processes run simultaneously.

## Solution

Replaced global command lock with **tmux-scoped lock** (`/tmp/agm-{UID}/tmux-server.lock`):

### 1. Created `internal/tmux/lock.go`
```go
func AcquireTmuxLock() error
func ReleaseTmuxLock() error
```

Lock is held **only during tmux server mutations**, not entire commands.

### 2. Protected tmux mutation operations

Added locks to:
- **NewSession** (tmux.go:44-48) - Session creation + settings
- **SendCommand** (tmux.go:185-189) - Command sends to panes
- **InitSequence.Run** (init_sequence.go:30-34) - `/rename` and `/agm-assoc` sequence

NOT locked (read-only or non-mutating):
- HasSession, ListSessions, ListSessionsWithInfo, ListClients
- GetCurrentSessionName, IsProcessRunning, WaitForProcessReady
- GetCurrentWorkingDirectory, Version
- **AttachSession** (can block indefinitely - must not hold lock)

### 3. Removed global command lock

Removed from `cmd/csm/main.go`:
- PersistentPreRunE: lock acquisition (lines 93-113)
- PersistentPostRunE: lock release (lines 124-128)
- Removed `globalLock` variable and `lock` import

### 4. Removed early lock releases

Removed obsolete lock releases from:
- **new.go:486-495** - Before InitSequence (deadlock prevention)
- **new.go:543-552** - For API-based agents
- **resume.go:555-562** - Before AttachSession

These are no longer needed because:
- Tmux lock is acquired/released per-operation (not held across entire command)
- Each operation uses `defer` for automatic cleanup
- No risk of deadlock since lock is not held during long-running operations

## Benefits

### Before (Global Command Lock)
```
agm session resume session-A  → Acquires lock → Blocks for hours (attached to tmux)
agm session list              → BLOCKED (waiting for lock)
agm version           → BLOCKED (waiting for lock)
```

### After (Tmux-Scoped Lock)
```
agm session resume session-A  → Acquires tmux lock → Releases → Attaches (no lock held)
agm session list              → Runs concurrently ✓
agm version           → Runs concurrently ✓
agm session new session-B     → Acquires tmux lock briefly → Creates session → Releases
```

### Specific improvements:
1. **Concurrent reads** - Multiple `agm session list`, `agm version`, etc. can run simultaneously
2. **No deadlocks** - `/agm:agm-assoc` skill can run without conflict
3. **No blocking on attach** - `AttachSession` doesn't hold any lock
4. **Correct granularity** - Only tmux server mutations are serialized

## Testing

Verified:
- ✅ Build succeeds: `go build ./cmd/agm`
- ✅ Install succeeds: `go install ./cmd/agm`
- ✅ List works: `agm session list`
- ✅ Concurrent commands work: `agm session list & agm version` (both complete without errors)

## Migration Notes

For users upgrading:
- Old lock file (`/tmp/agm-{UID}/csm.lock`) is no longer used
- New lock file (`/tmp/agm-{UID}/tmux-server.lock`) is created automatically
- No configuration changes needed
- `--no-lock` flag was removed (obsolete after deadlock fix in commit 262c069)

## Lock Hierarchy

AGM now uses **two levels of locking**:

| Lock Type | Scope | Location | Mechanism | When Held |
|-----------|-------|----------|-----------|-----------|
| **Tmux Server Lock** | Tmux mutations | `/tmp/agm-{UID}/tmux-server.lock` | File lock (syscall.Flock) | Only during NewSession, SendCommand, InitSequence |
| **Manifest Lock** | Manifest file | `{manifest-path}.lock` | PID + timestamp | During manifest read/write operations |

No global command lock - allows concurrent AGM operations while preventing race conditions.

## Related Files

**Modified:**
- `internal/tmux/lock.go` (new)
- `internal/tmux/tmux.go` (NewSession, SendCommand)
- `internal/tmux/init_sequence.go` (InitSequence.Run)
- `cmd/csm/main.go` (removed global lock)
- `cmd/csm/new.go` (removed early lock releases)
- `cmd/csm/resume.go` (removed early lock release)

**Unchanged:**
- `internal/manifest/lock.go` (manifest-level locks still used)
- `internal/lock/lock.go` (file lock implementation reused for tmux lock)

## Implementation Date

2026-01-23
