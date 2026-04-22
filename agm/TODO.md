# AGM TODO List

*Historical note: This project was renamed from AGM (Agent Session Manager) to AGM (AI/Agent Gateway Manager) in 2026-02. References below reflect historical context.*

## Recently Completed (2026-01-12)

- [x] **Enhanced `agm session unarchive` command** (2026-01-12)
  - File: `cmd/csm/unarchive.go`
  - Added confirmation prompt before unarchiving (matching archive.go pattern)
  - Added `--force/-f` flag to skip confirmation for automation
  - Features: Pattern matching (`*`, `?`, `[abc]`), interactive picker for multiple matches
  - Testing: All 8 tests passing
  - Commit: f16c620

- [x] **Add unit tests for archive command** (2026-01-12)
  - File: `cmd/csm/archive_test.go` (724 lines)
  - Implemented 18 test functions (exceeded requirement of 10)
  - Coverage: 57% of archive.go (all non-interactive paths fully covered)
  - All tests passing
  - Commit: 788d7f4

- [x] **Fix pre-existing test failure in autoimport_test.go** (2026-01-12)
  - Root cause: Global `cfg` variable could be nil in edge cases
  - Fix: Added defensive nil checks in 3 functions:
    - `offerToImportOrphanedSession()` at line 622
    - `resolveSessionIdentifier()` at line 213
    - `ValidArgsFunction` in resumeCmd at line 121
  - Test `TestOfferToImportOrphanedSession_NoHistory` now passes
  - Full test suite passing
  - Commit: 298acea

- [x] **Add bulk archive functionality** (2026-01-12)
  - New flags: `--all`, `--older-than=<duration>`, `--dry-run`
  - Duration parsing supports: 30d, 7d, 1w, 24h formats
  - Batch processing with confirmation prompts and detailed previews
  - Safety checks: skip active sessions, prevent incompatible flag combinations
  - Code: +288 lines in archive.go, +733 lines in tests
  - Commit: 788d7f4

- [x] **Standardize AGM UX Patterns and Error Handling** (2026-01-12)
  - Added `--no-color` and `--screen-reader` flags for WCAG AA accessibility
  - Created `internal/ui/errors.go` with 7 standardized error helpers
  - Updated 14 command files to use helpers and add actionable solutions
  - Eliminated all empty solution fields in error messages
  - Standardized warning symbol (⚠ vs ⚠️ inconsistency fixed)
  - Created comprehensive documentation:
    - `docs/UX_PATTERNS.md` - UX guide for developers
    - `docs/UX-ACCESSIBILITY-REVIEW.md` - Infrastructure review
    - `docs/UX-SPRINT1-REVIEW.md` - Sprint documentation
  - Updated README.md with accessibility section
  - All 26 packages passing tests
  - Commit: d86fbe2

## Recently Completed

- [x] **Fix critical UUID collision bug in `agm admin sync`** (2025-12-17)
  - Issue: `agm admin sync` auto-assigned the same Claude UUID to ALL sessions with empty UUIDs, causing 12 sessions to share the same conversation
  - Root cause: Auto-assignment logic in `syncActiveTmuxSessions()` used "latest UUID from history" for all sessions
  - Solution:
    - Removed auto-UUID assignment; new sessions created with empty UUID
    - Added prompt for manual association via `agm session associate`
    - Enhanced `agm admin doctor` to detect UUID collisions and duplicates
  - Files modified:
    - `cmd/csm/sync.go` (refactored `syncActiveTmuxSessions()`)
    - `cmd/csm/doctor.go` (added duplicate detection)
  - Testing: All tests pass (go test ./...)
  - Documentation:
    - `AGM-BUG-FIX-REPORT.md` (technical analysis)
    - `QUICK-START-FIXES.md` (user remediation guide)
  - Commit: 19eeb9a

- [x] **Auto-exit tmux sessions when Claude exits** (2025-12-17)
  - Issue: Typing `/exit` in Claude left user in tmux session requiring second `exit`
  - Solution: Append `; exit` to all claude commands sent to tmux
  - Files modified:
    - `cmd/csm/resume.go`
    - `internal/session/resume.go`
    - `cmd/csm/new.go`
  - Testing: All existing tests pass (go test ./... -v)
  - Documentation: `~/src/ws/csm-auto-exit-implementation.md`

## Recently Completed (2026-01-11)

- [x] **Tmux Refactor: Enhanced Socket Management & Session Persistence**
  - Added isolated Unix socket support (`/tmp/agm.sock`)
  - Implemented stale socket cleanup and lock mechanisms
  - Created `internal/tmux/socket.go` (237 lines) with comprehensive socket management
  - Added unit tests: `internal/tmux/socket_test.go` (297 lines, 17 test functions)
  - Coverage: 75.3% of new socket code

- [x] **Systemd Lingering Integration**
  - Implemented user lingering detection via loginctl
  - Prevents sessions from being killed on SSH logout
  - Created `internal/tmux/linger.go` (166 lines)
  - Updated `agm admin doctor` to check lingering status
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
  - All commands now use `-S /tmp/agm.sock` flag

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
