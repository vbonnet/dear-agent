# Lock System Improvements

## Overview

This document describes the lock system improvements implemented to fix three critical issues:

1. Locks held during tmux attachment blocking concurrent commands
2. No safe way to remove stale locks
3. Read-only commands unnecessarily requiring locks

## Problems Solved

### Problem 1: Lock Held During Tmux Attachment

**Issue:**
- `agm session new` and `agm session resume` held the global lock for the entire duration of tmux session attachment
- `tmux attach` is a blocking call that doesn't return until user detaches (Ctrl+B, D)
- This meant the lock was held for hours/days while user worked in the tmux session
- Any other `agm` command would fail with "Another csm command is currently running"

**Root Cause:**
- Lock acquired in `PersistentPreRunE` (main.go)
- Lock released in `PersistentPostRunE` (main.go)
- `tmux.AttachSession()` blocks between these two hooks
- Lock held unnecessarily long

**Solution:**
- Release lock **before** calling `tmux.AttachSession()` in both `new.go` and `resume.go`
- Lock only protects the setup phase (creating tmux session, starting Claude, writing manifests)
- Set `globalLock = nil` to prevent double-unlock in `PersistentPostRunE`
- User can stay attached without blocking other csm commands

**Files Modified:**
- `cmd/csm/new.go` (lines 244-249)
- `cmd/csm/resume.go` (lines 458-463)

### Problem 2: No Safe Way to Remove Stale Locks

**Issue:**
- Agents were manually deleting lock files: `rm -rf /tmp/agm-*.lock`
- Error messages previously suggested dangerous `--no-lock` flag (removed in 2026)
- No built-in way to check if lock is stale (process crashed but lock file remains)

**Solution:**
- Added `agm admin unlock` command with PID validation
- Checks if lock-holding process is still running via:
  1. Linux `/proc/<pid>` directory check (most reliable)
  2. Fallback to `signal(0)` check (cross-platform)
- Removes lock only if process has exited (stale)
- `--force` flag to remove lock even if process is running (dangerous, but sometimes needed)
- Updated error message to suggest `agm admin unlock` instead of `--no-lock`

**Files Modified:**
- `internal/lock/lock.go`:
  - Added `LockInfo` struct
  - Added `CheckLock()` function
  - Added `processExists()` helper
  - Added `ForceUnlock()` function
  - Updated error message in `TryLock()`

**Files Created:**
- `cmd/csm/unlock.go` (complete command implementation)

### Problem 3: Read-Only Commands Required Locks

**Issue:**
- Commands like `agm version` and `agm session list` required locks
- Agents previously used `agm version --no-lock` to bypass locks (flag removed after deadlock fix)
- No reason for read-only commands to need locks (they don't modify state)

**Solution:**
- Added lock-free command list in `PersistentPreRunE`
- Read-only commands skip lock acquisition entirely
- Only commands that modify state require locks

**Lock-Free Commands:**
- `version` - Show version information
- `list` - List sessions (read-only query)
- `doctor` - Diagnostic checks (read-only query)
- `unlock` - Remove stale locks (must work even when locked!)
- `backup` - Backup operations (read-only)

**Commands That Still Need Locks:**
- `new` - Creates tmux session, starts Claude, writes manifests
- `resume` - Starts Claude, modifies manifests
- `associate` - Modifies manifests
- `sync` - Modifies manifests
- `archive` - Modifies manifests

**Files Modified:**
- `cmd/csm/main.go` (lines 55-74)

## Technical Implementation

### Lock Path

Default: `/tmp/agm-<UID>/csm.lock`

User-specific directory ensures no permission conflicts between users.

### Lock Content

Lock file contains the process ID (PID) of the process holding the lock:
```
12345
```

### Lock Mechanism

Uses POSIX `flock()` syscall with:
- `LOCK_EX` - Exclusive lock
- `LOCK_NB` - Non-blocking mode (fail immediately if locked)

Benefits:
- Automatically released when process exits (even on crash)
- Kernel-level enforcement (no race conditions)
- Cross-platform (Linux, macOS, BSD)

### PID Validation

```go
func processExists(pid int) bool {
    // Method 1: Linux /proc filesystem (most reliable)
    if _, err := os.Stat(fmt.Sprintf("/proc/%d", pid)); err == nil {
        return true
    }

    // Method 2: Signal 0 (doesn't send signal, just checks existence)
    process, err := os.FindProcess(pid)
    if err != nil {
        return false
    }
    return process.Signal(syscall.Signal(0)) == nil
}
```

### Lock Lifecycle Changes

**Before:**
```
agm session new → PersistentPreRunE (lock) → RunE (create tmux, start Claude, ATTACH) → PersistentPostRunE (unlock)
          ^                                                                                      ^
          |                                                                                      |
          +--- Lock held for entire tmux session (hours/days) ---------------------------------+
```

**After:**
```
agm session new → PersistentPreRunE (lock) → RunE (create tmux, start Claude, UNLOCK, attach) → PersistentPostRunE (noop)
          ^                                                          ^
          |                                                          |
          +--- Lock only held during setup (seconds) ---------------+
```

## Usage

### Check Lock Status

```bash
agm admin unlock
```

Output possibilities:
- ✅ No lock file found
- 🔓 Lock is stale (process 12345 no longer running). Removed.
- ❌ Lock is held by active process 12345

### Force Remove Lock

```bash
agm admin unlock --force
```

⚠️ WARNING: Only use if you're certain the process is dead. Can cause race conditions.

### Verify Lock-Free Commands

```bash
# These work even if another csm is running
agm version
agm session list
agm admin doctor
```

## Testing

### Unit Tests

Added to `internal/lock/lock_test.go`:
- `TestCheckLock_NoLockExists` - No lock file
- `TestCheckLock_StaleLock` - Non-existent PID
- `TestCheckLock_ActiveLock` - Current process PID
- `TestCheckLock_EmptyLock` - Empty lock file
- `TestCheckLock_InvalidPID` - Invalid PID format
- `TestForceUnlock_RemovesLock` - Force unlock success
- `TestForceUnlock_NonExistentLock` - Force unlock non-existent
- `TestProcessExists_CurrentProcess` - Process exists check
- `TestProcessExists_NonExistentProcess` - Process not exists

Run with:
```bash
go test ./internal/lock/
```

### Integration Tests

Manual test script: `~/src/ws/test-agm-lock-fixes.sh`

Tests:
1. Lock-free commands work without locks
2. `agm admin unlock` detects no lock when clean
3. `agm admin unlock` removes stale locks
4. `agm version` doesn't create locks

### Manual Testing Scenarios

**Concurrent Commands:**
```bash
# Terminal 1
agm session new test-session-1
# (stay attached)

# Terminal 2 - should work now!
agm session new test-session-2
```

**Stale Lock Recovery:**
```bash
# Simulate crash
echo "99999" > /tmp/agm-$(id -u)/csm.lock

# Clean up stale lock
agm admin unlock
# Output: Lock is stale (process 99999 no longer running). Removed.
```

## Impact

### Before

- ❌ Agents manually deleting lock files with `rm -rf /tmp/agm-*.lock`
- ❌ `agm session new` in one terminal blocked all other csm commands
- ❌ `agm version` required `--no-lock` flag to bypass locks
- ❌ No safe way to remove stale locks after crashes

### After

- ✅ Safe `agm admin unlock` command with PID validation
- ✅ Multiple csm commands can run concurrently while tmux attached
- ✅ Read-only commands never require locks
- ✅ Proper stale lock detection and cleanup
- ✅ Better error messages guiding users to solutions

## Future Improvements

Possible enhancements (not implemented):

1. **Lock timeout**: Instead of failing immediately, wait for lock with timeout
2. **Lock watch**: Monitor lock file changes to detect when lock is released
3. **Per-session locks**: Lock individual sessions instead of global lock
4. **Lock metrics**: Track lock acquisition time, conflicts, stale locks

## References

- Main lock implementation: `internal/lock/lock.go`
- Lock acquisition: `cmd/csm/main.go` (`PersistentPreRunE`)
- Lock release in new: `cmd/csm/new.go` (lines 244-249)
- Lock release in resume: `cmd/csm/resume.go` (lines 458-463)
- Unlock command: `cmd/csm/unlock.go`
- Lock tests: `internal/lock/lock_test.go`
