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

## Notes

- Archive command implementation completed: 2025-12-17
- Wayfinder session: 8c567cf5-da1c-48d8-84cc-db5b736882ea
- All items above extracted from retrospective and STATUS.md
