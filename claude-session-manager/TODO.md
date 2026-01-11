# CSM TODO List

## Archive Command Follow-ups

### HIGH Priority

- [ ] **Add `csm unarchive` command**
  - File: `cmd/csm/unarchive.go`
  - Functionality: Sets `lifecycle: ""` to restore archived sessions
  - Estimated effort: 1 hour (~50 lines)
  - Reference: `wayfinder-projects/archive-command/STATUS.md` line 284

### MEDIUM Priority

- [ ] **Add unit tests for archive command**
  - File: `cmd/csm/archive_test.go`
  - Test cases: 10 functions (per D2 spec)
  - Use mock tmux interface
  - Estimated effort: 2 hours
  - Reference: `wayfinder-projects/archive-command/D2-DESIGN.md` section "Testing Strategy"

### Bug Fixes

- [ ] **Fix pre-existing test failure in autoimport_test.go**
  - Location: `cmd/csm/autoimport_test.go:152`
  - Test: `TestOfferToImportOrphanedSession_NoHistory`
  - Error: `panic: runtime error: invalid memory address or nil pointer dereference`
  - Function: `offerToImportOrphanedSession` at `resume.go:553`
  - Status: Pre-existing (not caused by archive command)
  - Impact: Blocks clean test suite pass
  - Reference: `wayfinder-projects/archive-command/STATUS.md` line 185

### LOW Priority

- [ ] **Add bulk archive functionality**
  - Feature: `csm archive --all --older-than=30d`
  - Archive sessions inactive for N days
  - Estimated effort: 3 hours
  - Reference: `wayfinder-projects/archive-command/STATUS.md` line 296

## Recently Completed

- [x] **Fix critical UUID collision bug in `csm sync`** (2025-12-17)
  - Issue: `csm sync` auto-assigned the same Claude UUID to ALL sessions with empty UUIDs, causing 12 sessions to share the same conversation
  - Root cause: Auto-assignment logic in `syncActiveTmuxSessions()` used "latest UUID from history" for all sessions
  - Solution:
    - Removed auto-UUID assignment; new sessions created with empty UUID
    - Added prompt for manual association via `csm associate`
    - Enhanced `csm doctor` to detect UUID collisions and duplicates
  - Files modified:
    - `cmd/csm/sync.go` (refactored `syncActiveTmuxSessions()`)
    - `cmd/csm/doctor.go` (added duplicate detection)
  - Testing: All tests pass (go test ./...)
  - Documentation:
    - `CSM-BUG-FIX-REPORT.md` (technical analysis)
    - `QUICK-START-FIXES.md` (user remediation guide)
  - Commit: 19eeb9a

- [x] **Auto-exit tmux sessions when Claude exits** (2025-12-17)
  - Issue: Typing `/exit` in Claude left user in tmux session requiring second `exit`
  - Solution: Append `; exit` to all claude commands sent to tmux
  - Files modified:
    - `cmd/csm/resume.go` (lines 428, 431)
    - `internal/session/resume.go` (lines 70, 73)
    - `cmd/csm/new.go` (lines 179, 319)
  - Testing: All existing tests pass (go test ./... -v)
  - Documentation: `~/src/ws/csm-auto-exit-implementation.md`

## Recently Completed (2026-01-11)

- [x] **Tmux Refactor: Enhanced Socket Management & Session Persistence**
  - Added isolated Unix socket support (`/tmp/csm.sock`)
  - Implemented stale socket cleanup and lock mechanisms
  - Created `internal/tmux/socket.go` (237 lines) with comprehensive socket management
  - Added unit tests: `internal/tmux/socket_test.go` (297 lines, 17 test functions)
  - Coverage: 75.3% of new socket code

- [x] **Systemd Lingering Integration**
  - Implemented user lingering detection via loginctl
  - Prevents sessions from being killed on SSH logout
  - Created `internal/tmux/linger.go` (166 lines)
  - Updated `csm doctor` to check lingering status
  - Added unit tests: `internal/tmux/linger_test.go` (213 lines)

- [x] **Zero-Overhead Tmux Attachment**
  - Refactored AttachSession to use syscall.Exec
  - Eliminates Go process overhead (~10-20MB memory per session)
  - Process is replaced with tmux (no background runtime)
  - Modified: `internal/tmux/tmux.go:139-166`

- [x] **Tmux Settings Injection for Better UX**
  - `set-window-option -g aggressive-resize on` - Fixes multi-device layout issues
  - `set-option -g window-size latest` - Forces window to fit current screen
  - `set -g mouse on` - Enables mouse scrolling
  - `set -s set-clipboard on` - Enables OSC 52 for Cmd-C over SSH
  - Modified: `internal/tmux/tmux.go:59-77`

- [x] **Control Mode Support**
  - Created `internal/tmux/control.go` (313 lines)
  - Enables programmatic tmux interaction with `%end` notifications
  - Provides command verification and output capture
  - Future-proofs for advanced automation scenarios

- [x] **Updated All Tmux Commands with Socket Support**
  - HasSession, NewSession, AttachSession, SendCommand
  - ListSessions, GetCurrentSessionName, IsProcessRunning
  - GetCurrentWorkingDirectory
  - All commands now use `-S /tmp/csm.sock` flag

- [x] **Enhanced Doctor Checks**
  - Added socket health check (shows stale vs. active)
  - Added lingering status check
  - Provides actionable recommendations for issues
  - Modified: `cmd/csm/doctor.go:54-89`

## Notes

- Archive command implementation completed: 2025-12-17
- Tmux refactor implementation completed: 2026-01-11
- Multi-persona code review: APPROVED (8.8/10 overall score)
- Test coverage: 75.3% for new tmux code
- Wayfinder session: 8c567cf5-da1c-48d8-84cc-db5b736882ea
- All items above extracted from retrospective and STATUS.md
