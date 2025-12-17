# S9: Validation - Session Persistence Feature

**Date**: December 7, 2025
**Phase**: S9 (Validation)
**Status**: 📋 PLANNING
**Prerequisites**: ✅ S6 Sprint 2 Complete and Approved (9.3/10)

---

## Phase Overview

**Purpose**: Validate that Sprint 1 + Sprint 2 implementation works correctly through smoke testing, integration verification, and final cleanup.

**Scope**: End-to-end validation of:
- Manifest v2 schema (Sprint 1)
- Status computation (Sprint 2 - D2.1)
- Enhanced resume with auto-recreation (Sprint 2 - D2.2)
- Backup management (Sprint 2 - D2.3)

**NOT in scope**:
- New features or functionality
- Performance optimization beyond critical issues
- Production deployment (S10)
- Retrospective (S11)

---

## Validation Strategy

### V1: Smoke Testing (Manual)
**Estimated**: 2 hours

Test critical user workflows end-to-end:

1. **Create New Session**
   ```bash
   csm new ~/test-project
   # Verify: tmux session created, Claude started, manifest created
   ```

2. **List Sessions with Status**
   ```bash
   csm list
   # Verify: Shows active session with green status
   # Verify: Color coding works (active=green, stopped=yellow, archived=red)
   ```

3. **Stop and Resume Session**
   ```bash
   # Exit tmux session
   csm list
   # Verify: Session shows as "stopped" (yellow)

   csm resume <identifier>
   # Verify: Tmux session recreated
   # Verify: Working directory correct
   # Verify: Claude resume command sent
   ```

4. **Archive and Resume Protection**
   ```bash
   # Manually edit manifest to set lifecycle: "archived"
   csm list --all
   # Verify: Shows archived session (red)

   csm resume <identifier>
   # Verify: Error message: "Cannot resume archived session"
   # Verify: Suggests unarchive command
   ```

5. **Backup Creation and Restoration**
   ```bash
   # Make changes to manifest
   csm backup list <identifier>
   # Verify: Shows backups with numbers

   csm backup restore <identifier> <num>
   # Verify: Confirmation dialog appears
   # Verify: Restoration succeeds
   # Verify: Current state backed up before restore
   ```

6. **Backup Rotation**
   ```bash
   # Create 15 backups programmatically
   csm backup list <identifier>
   # Verify: Only 10 backups remain (oldest deleted)
   ```

7. **Session Discovery (Orphaned Sessions)**
   ```bash
   csm sync
   # Verify: Finds orphaned sessions from history.jsonl
   # Verify: Offers to create manifests
   # Verify: Creates manifests with correct v2 schema
   ```

### V2: Integration Testing (Automated)
**Estimated**: 3 hours

Create integration test suite:

**Test File**: `test/integration_test.go`

```go
// TestFullResumeWorkflow tests create → stop → resume flow
func TestFullResumeWorkflow(t *testing.T) {
    // 1. Create session via CLI
    // 2. Verify manifest exists with v2 schema
    // 3. Stop session (kill tmux)
    // 4. Resume session
    // 5. Verify tmux recreated with correct working dir
}

// TestBackupRotation tests backup creation and rotation
func TestBackupRotation(t *testing.T) {
    // 1. Create manifest
    // 2. Create 15 backups
    // 3. Verify only 10 remain
    // 4. Verify oldest deleted
}

// TestStatusComputation tests list command status display
func TestStatusComputation(t *testing.T) {
    // 1. Create manifest
    // 2. Start tmux session
    // 3. Run csm list
    // 4. Verify status = "active"
    // 5. Stop tmux
    // 6. Run csm list
    // 7. Verify status = "stopped"
}

// TestArchivedSessionProtection tests resume rejection
func TestArchivedSessionProtection(t *testing.T) {
    // 1. Create manifest
    // 2. Set lifecycle = "archived"
    // 3. Run csm resume
    // 4. Verify error contains "Cannot resume archived"
}
```

**Acceptance Criteria**:
- [ ] All integration tests pass
- [ ] No test flakiness (run 5 times)
- [ ] Tests run in <30 seconds total

### V3: Regression Testing
**Estimated**: 1 hour

Verify all existing tests still pass:

```bash
go test ./... -v
```

**Acceptance Criteria**:
- [ ] All 34 unit tests pass
- [ ] No new warnings or errors
- [ ] Build succeeds on all platforms (Linux, macOS)

### V4: Edge Case Validation
**Estimated**: 2 hours

Test edge cases and error scenarios:

1. **Concurrent Operations**
   - Create backup while reading manifest
   - Resume session while another process modifies manifest
   - Verify: Atomic writes prevent corruption

2. **Missing Dependencies**
   - Run with tmux not installed
   - Verify: Graceful error message

3. **Malformed Manifests**
   - Invalid YAML syntax
   - Missing required fields
   - v1 schema (should auto-migrate)

4. **Disk Full Scenarios**
   - Backup creation when disk full
   - Manifest write when disk full
   - Verify: Atomic writes prevent partial corruption

5. **Long Paths**
   - Session with very long project path
   - Verify: No path truncation issues

6. **Special Characters in Paths**
   - Paths with spaces
   - Paths with quotes
   - Verify: Shell quoting works correctly

**Acceptance Criteria**:
- [ ] All edge cases handled gracefully
- [ ] No panics or crashes
- [ ] Error messages are helpful

### V5: Security Validation
**Estimated**: 1 hour

Verify security measures:

1. **Command Injection Protection**
   ```bash
   # Try resuming session with malicious project path
   # Verify: Shell quoting prevents injection
   ```

2. **File Permissions**
   ```bash
   # Check manifest permissions (should be 0600)
   # Check backup permissions (should be 0600)
   ```

3. **Path Traversal**
   ```bash
   # Try accessing ../../etc/passwd via identifiers
   # Verify: Path validation prevents traversal
   ```

4. **Input Validation**
   ```bash
   # Try invalid backup numbers (negative, huge, non-numeric)
   # Verify: Proper validation and error messages
   ```

**Acceptance Criteria**:
- [ ] No command injection vulnerabilities
- [ ] File permissions correct (0600)
- [ ] No path traversal issues
- [ ] Input validation comprehensive

### V6: Documentation Review
**Estimated**: 1 hour

Review and update documentation:

1. **README.md**
   - [ ] Add "Features" section listing new capabilities
   - [ ] Update examples with status computation
   - [ ] Document backup management commands

2. **CLI Help Text**
   - [ ] Verify all commands have help text
   - [ ] Check examples are accurate
   - [ ] Add status legend to list command

3. **Code Comments**
   - [ ] Add godoc comments to exported functions
   - [ ] Document complex algorithms
   - [ ] Add package-level documentation

4. **Architecture Documentation**
   - [ ] Document TmuxInterface pattern
   - [ ] Explain v2 schema migration
   - [ ] Document backup rotation logic

**Acceptance Criteria**:
- [ ] README up to date
- [ ] All commands documented
- [ ] Code has godoc comments
- [ ] Architecture decisions documented

### V7: Performance Validation
**Estimated**: 1 hour

Verify performance meets requirements:

1. **Batch Status Computation**
   ```bash
   # Create 100 manifests
   # Run csm list
   # Verify: Completes in <100ms
   ```

2. **Backup Creation**
   ```bash
   # Time backup creation
   # Verify: <50ms per backup
   ```

3. **Manifest Read/Write**
   ```bash
   # Benchmark atomic writes
   # Verify: <10ms per operation
   ```

**Acceptance Criteria**:
- [ ] Status computation: <100ms for 100 sessions
- [ ] Backup creation: <50ms
- [ ] Manifest I/O: <10ms

---

## Cleanup Tasks

### C1: Code Cleanup
**Estimated**: 1 hour

1. **Remove Dead Code**
   - [ ] Check for unused functions
   - [ ] Remove commented-out code
   - [ ] Clean up debug statements

2. **Improve Code Quality**
   - [ ] Run `go fmt` on all files
   - [ ] Run `go vet` and fix warnings
   - [ ] Run `golangci-lint` if available

3. **Organize Imports**
   - [ ] Group imports (stdlib, third-party, internal)
   - [ ] Remove unused imports
   - [ ] Sort imports alphabetically

**Acceptance Criteria**:
- [ ] No dead code
- [ ] All code formatted
- [ ] No linter warnings

### C2: Test Cleanup
**Estimated**: 30 minutes

1. **Remove Obsolete Tests**
   - Already done (removed list_test.go, resume_test.go)

2. **Improve Test Coverage**
   - [ ] Add missing resume tests (from review feedback)
   - [ ] Add backup CLI tests

3. **Test Documentation**
   - [ ] Add comments to complex test cases
   - [ ] Document test fixtures

**Acceptance Criteria**:
- [ ] No obsolete tests
- [ ] Missing tests added
- [ ] Tests well-documented

### C3: Build Verification
**Estimated**: 30 minutes

1. **Cross-Platform Build**
   ```bash
   GOOS=linux go build ./cmd/csm
   GOOS=darwin go build ./cmd/csm
   ```

2. **Binary Size**
   ```bash
   ls -lh csm
   # Verify: Reasonable size (<50MB)
   ```

3. **Dependencies**
   ```bash
   go mod tidy
   go mod verify
   ```

**Acceptance Criteria**:
- [ ] Builds on Linux and macOS
- [ ] Binary size reasonable
- [ ] Dependencies clean

---

## Acceptance Criteria (Overall)

### Critical (Must Pass)
- [ ] All smoke tests pass
- [ ] All unit tests pass (34 total)
- [ ] Integration tests pass (if created)
- [ ] No security vulnerabilities
- [ ] No regressions from Sprint 1

### High Priority
- [ ] Edge cases handled gracefully
- [ ] Documentation updated
- [ ] Code cleanup complete
- [ ] Performance requirements met

### Medium Priority
- [ ] godoc comments added
- [ ] Integration test suite created
- [ ] README updated with new features

### Low Priority
- [ ] Linter warnings fixed
- [ ] Cross-platform build verified

---

## Timeline Estimate

| Task | Estimated | Priority |
|------|-----------|----------|
| V1: Smoke Testing | 2h | Critical |
| V2: Integration Tests | 3h | High |
| V3: Regression Testing | 1h | Critical |
| V4: Edge Cases | 2h | High |
| V5: Security | 1h | Critical |
| V6: Documentation | 1h | Medium |
| V7: Performance | 1h | High |
| C1: Code Cleanup | 1h | High |
| C2: Test Cleanup | 0.5h | Medium |
| C3: Build Verification | 0.5h | High |
| **Total** | **13h** | |

---

## Deliverables

1. **Smoke Test Report**
   - Document: `S9-SMOKE-TEST-REPORT.md`
   - All workflows tested and verified

2. **Integration Test Suite** (if created)
   - File: `test/integration_test.go`
   - All tests passing

3. **Security Validation Report**
   - Document: `S9-SECURITY-VALIDATION.md`
   - All security checks passed

4. **Updated Documentation**
   - README.md with new features
   - godoc comments added
   - Architecture documentation

5. **Validation Summary**
   - Document: `S9-VALIDATION-SUMMARY.md`
   - All acceptance criteria status
   - Known issues (if any)
   - Recommendations for S10 (Deploy)

---

## Risk Assessment

### Low Risk
- Unit tests already comprehensive (34 passing)
- Core functionality tested during development
- Security measures already in place

### Medium Risk
- Integration testing may reveal workflow issues
- Edge cases might uncover bugs
- Performance under load untested

### Mitigation
- Thorough smoke testing before integration tests
- Edge case testing with fault injection
- Performance testing with realistic data

---

## Next Steps

1. ✅ Get user approval for S9 Validation plan
2. ⏭️ Execute V1: Smoke Testing (2 hours)
3. ⏭️ Execute V3: Regression Testing (1 hour)
4. ⏭️ Execute V5: Security Validation (1 hour)
5. ⏭️ Execute remaining validation tasks
6. ⏭️ Create validation summary document
7. ⏭️ Multi-persona review of validation results
8. ⏭️ Report completion to user

---

**Status**: 📋 READY FOR VALIDATION

**Blocker**: ❌ STOP - Need user approval to proceed with S9 Validation per Wayfinder command

Per Wayfinder instructions:
> ✅ STOP when: You have a question that requires my input

S6 (Sprint 2) has been approved (9.3/10). Now waiting for user approval to proceed to S9 (Validation).
