# S3 Phase 2: CLI Commands Implementation

**Status**: ✅ COMPLETE - Ready for Review
**Prerequisite**: Phase 1 (Core Parsing & Deduplication) - ✅ APPROVED (8.9/10)

---

## Goal

Implement functional CLI commands that leverage the Phase 1 parsing library to:
1. List all Claude sessions with formatted output
2. Integrate with existing manifest/discovery system
3. Provide complete end-to-end functionality for `csm list` and `csm sync`

---

## Scope

### In Scope for Phase 2
- ✅ Complete `csm list` command (display sessions)
- ✅ Complete `csm sync` command integration
- ✅ CLI output formatting and UX
- ✅ Integration tests for commands
- ✅ Error handling and user feedback

### Out of Scope (Future Phases)
- Session resumption (`csm resume` - Phase 3)
- Tmux integration (Phase 3)
- Session health checks (Phase 3)
- Manifest creation/updates (partially in sync, full in Phase 3)

---

## Implementation Tasks

### 1. Complete `csm list` Command

**File**: `cmd/csm/list.go`

**Current State**: Skeleton implementation exists

**Requirements**:
- Parse history.jsonl using Phase 1 library
- Deduplicate to get unique sessions
- Display table with columns:
  - UUID (first 8 chars)
  - Project directory
  - Message count
  - Duration (hours)
  - Last activity (human-readable)
- Sort by last activity (newest first)
- Handle errors gracefully with user-friendly messages

**Acceptance Criteria**:
- ✅ Command runs without errors
- ✅ Output is properly formatted and readable
- ✅ Shows real data from ~/.claude/history.jsonl
- ✅ Gracefully handles missing/malformed history file

### 2. Enhance `csm sync` Command

**File**: `cmd/csm/sync.go`

**Current State**: Partially implemented, uses old API

**Requirements**:
- Already updated to use Phase 1 ParseHistory + Deduplicate ✅
- Display parse statistics when available ✅
- Integrate with discovery.MatchToManifests ✅
- Create manifests for orphaned sessions (already implemented) ✅
- Verify functionality end-to-end

**Acceptance Criteria**:
- ✅ Full integration with Phase 1 library
- ✅ Parse statistics displayed
- ✅ Orphan detection works
- ✅ Manual testing confirms functionality

### 3. Improve CLI UX

**Files**: All command files

**Requirements**:
- Consistent error messages across commands
- Use ui.Print* helpers for colored output
- Progress indicators for long operations
- Help text with examples
- Version command shows correct version

**Acceptance Criteria**:
- ✅ Consistent error handling pattern
- ✅ Color-coded output (success=green, warning=yellow, error=red)
- ✅ Helpful error messages with actionable suggestions
- ✅ `--help` shows clear usage examples

---

## Test Plan

### Unit Tests
- ✅ Phase 1 library (already tested at 90.9% coverage)
- Test CLI command logic (parsers, formatters)

### Integration Tests
- `csm list` with real history.jsonl
- `csm sync` discovers and matches sessions
- Error handling for missing files
- Error handling for corrupted data

### Manual Tests
1. Run `csm list` - verify output matches expectations
2. Run `csm sync` - verify orphan detection
3. Run `csm doctor` - verify health checks
4. Test with empty history file
5. Test with malformed history file

---

## Success Criteria

1. **Functionality**: All commands work end-to-end
2. **Test Coverage**: >80% for new code
3. **Clean Build**: No compiler warnings, passes `go vet`
4. **Documentation**: Each command has clear help text
5. **UX**: Error messages are helpful and actionable
6. **Integration**: Works with existing manifest system

---

## Deliverables

1. Fully functional `csm list` command
2. Enhanced `csm sync` command with Phase 1 integration
3. Comprehensive test suite
4. Clean, formatted code (`gofmt`)
5. Passing multi-persona review (≥8.5/10)

---

## Implementation Order

1. **First**: Complete `csm list` (simple, no external dependencies)
2. **Second**: Verify `csm sync` integration (already mostly done)
3. **Third**: Add integration tests
4. **Fourth**: Improve UX and error handling
5. **Fifth**: Multi-persona review and fixes

---

## Dependencies

- Phase 1 library (`internal/claude/*.go`) - ✅ COMPLETE
- Existing manifest system (`internal/manifest/*.go`)
- Existing discovery system (`internal/discovery/*.go`)
- UI helpers (`internal/ui/*.go`)

---

## Timeline Estimate

- Complete `csm list`: 1 hour
- Verify `csm sync`: 30 minutes
- Integration tests: 1 hour
- UX improvements: 30 minutes
- Review and fixes: 1 hour

**Total**: ~4 hours
