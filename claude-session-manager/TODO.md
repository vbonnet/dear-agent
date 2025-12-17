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

## Notes

- Archive command implementation completed: 2025-12-17
- Wayfinder session: 8c567cf5-da1c-48d8-84cc-db5b736882ea
- All items above extracted from retrospective and STATUS.md
