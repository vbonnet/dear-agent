# ADR-006: Test Isolation Enforcement

**Status:** Accepted
**Date:** 2026-03-20
**Deciders:** AGM Engineering Team
**Related:** Test infrastructure, database isolation, production data protection

---

## Context

### Problem Statement

**User Pain**: Tests were creating sessions in production workspaces (oss, acme), causing data pollution. `agm session list` showed dozens of "test-parent" sessions created by Go test runs, cluttering the production workspace and requiring manual cleanup.

**Root Cause**: No enforcement layer prevented tests from writing to production databases. Tests could hardcode `WORKSPACE="oss"` and bypass isolation mechanisms entirely.

**Business Impact**:
- Production workspace contaminated with test artifacts
- Manual cleanup burden (21+ test sessions accumulated)
- Risk of test data corruption affecting real sessions
- Developer confusion from polluted session lists

**Technical Constraint**: Tests must work on any developer machine regardless of their default workspace configuration (oss, acme, etc.).

---

## Decision

We will implement **fail-fast enforcement** (Option 1) with infrastructure-level blocking that prevents ANY test from writing to production workspaces.

**Architecture**:

### 1. Fail-Fast Enforcement Layer (`internal/dolt/adapter.go`)

```go
func isRunningInTest() bool {
    executable, _ := os.Executable()
    return strings.Contains(executable, ".test")
}

func DefaultConfig() (*Config, error) {
    if isRunningInTest() {
        // Requires ENGRAM_TEST_MODE=1
        // Blocks: oss, acme, prod, production, main
        // Provides clear error messages
    }
}
```

**Enforcement rules**:
- Detects test execution context (`.test` in executable name)
- Requires `ENGRAM_TEST_MODE=1` environment variable
- Blocks production workspace names: oss, acme, prod, production, main
- Fails immediately with actionable error messages

### 2. Test Isolation Helper (`internal/testutil/environment.go`)

```go
func SetupTestEnvironment(t *testing.T) {
    t.Helper()
    os.Setenv("ENGRAM_TEST_MODE", "1")
    os.Setenv("ENGRAM_TEST_WORKSPACE", "test")
    os.Setenv("WORKSPACE", "test")
    t.Cleanup(/* restore environment */)
}
```

**Usage pattern**:
```go
func TestExample(t *testing.T) {
    testutil.SetupTestEnvironment(t)  // One-line setup
    // Test code uses workspace="test" automatically
}
```

### 3. Interactive Prompt Enforcement (`cmd/agm/new.go`)

```go
sessionNameLower := strings.ToLower(sessionName)
containsTest := strings.Contains(sessionNameLower, "test")
if containsTest && !testMode {
    // Show prompt with only 2 options:
    // 1. Use --test flag (isolated workspace)
    // 2. Cancel and rename
    // NO bypass option for production
}
```

**Enforcement rules**:
- Catches "test" anywhere in name (case-insensitive)
- Pattern: test-*, *-test-*, *-test, Test*, *Test*
- Removed `--allow-test-name` bypass flag
- Scripts MUST use `--test` flag explicitly

---

## Alternatives Considered

### Alternative 1: Convention-Based Isolation (Status Quo)

**Approach**: Provide test helpers but rely on developers to use them.

**Pros**:
- Simple implementation
- No enforcement overhead
- Flexible for edge cases

**Cons**:
- ❌ No protection against mistakes
- ❌ Developers can bypass by accident
- ❌ Production pollution already occurred
- ❌ No way to detect violations

**Rejection Reason**: Already proven ineffective - tests polluted production despite helpers existing.

### Alternative 2: Cleanup-Based Approach

**Approach**: Allow test pollution, but auto-cleanup afterward via test teardown.

**Pros**:
- Simple implementation
- No test environment setup required

**Cons**:
- ❌ Cleanup may fail, leaving pollution
- ❌ No protection during test execution
- ❌ Risk of cleaning production data
- ❌ Doesn't prevent the problem, just reacts to it

**Rejection Reason**: Reactive solution doesn't prevent pollution, only attempts cleanup.

### Alternative 3: Separate Test Database

**Approach**: Run tests against a completely separate Dolt database.

**Pros**:
- Complete isolation
- No risk to production

**Cons**:
- ❌ Complex setup (requires local test database)
- ❌ CI/CD complexity (database per test run)
- ❌ Slower tests (database setup overhead)
- ❌ May not match production schema

**Rejection Reason**: Over-engineered for the problem. Workspace isolation sufficient.

---

## Consequences

### Positive

1. **Production Protection**
   - ✅ Tests cannot write to production workspaces
   - ✅ Clear error messages guide developers
   - ✅ No more manual cleanup burden

2. **Developer Experience**
   - ✅ One-line test setup: `testutil.SetupTestEnvironment(t)`
   - ✅ Actionable error messages with fix instructions
   - ✅ Works on any developer machine (workspace-agnostic)

3. **Maintainability**
   - ✅ Infrastructure-level enforcement (can't be bypassed)
   - ✅ Clear migration path for existing tests
   - ✅ Self-documenting via error messages

### Negative

1. **Migration Burden**
   - ⚠️ ~100+ test files need `testutil.SetupTestEnvironment(t)` added
   - ⚠️ Existing tests fail until fixed
   - ✅ Mitigation: Clear error messages guide migration

2. **Test Environment Setup**
   - ⚠️ Tests must set environment variables
   - ⚠️ CI/CD pipelines need environment updates
   - ✅ Mitigation: `testutil` package makes this one line

3. **Interactive Workflow Impact**
   - ⚠️ Sessions with "test" in name require `--test` flag
   - ⚠️ Scripts must be updated to use `--test` flag
   - ✅ Mitigation: Prompt provides clear options, no bypass

### Risks and Mitigations

| Risk | Impact | Likelihood | Mitigation |
|------|--------|------------|------------|
| Tests fail after upgrade | High | Certain | Clear error messages with fix instructions |
| Developer bypasses enforcement | Medium | Low | No bypass mechanism exists |
| CI/CD breaks | High | Medium | Documentation includes CI/CD setup |
| False positives (valid "test" names) | Low | Low | Prompt allows using --test flag |

---

## Implementation

### Phase 1: Enforcement Layer (Complete)

**Files**:
- `internal/dolt/adapter.go` - Fail-fast enforcement
- `internal/testutil/environment.go` - Test isolation helper
- `cmd/agm/archive_test.go` - Proof-of-concept integration
- `cmd/agm/new.go` - Interactive prompt enforcement
- `TESTING.md` - Comprehensive documentation

**Verification**:
- ✅ 60+ packages tested
- ✅ Only 2 expected failures (test database doesn't exist)
- ✅ No production pollution

### Phase 2: Test Migration (TODO)

**Approach**:
1. Identify all failing tests
2. Add `testutil.SetupTestEnvironment(t)` to test setup
3. Remove hardcoded workspace values
4. Verify tests pass

**Estimated files**: ~10-20 test files
**Estimated time**: 2-4 hours

### Phase 3: CI/CD Integration (TODO)

**Updates**:
```yaml
test:
  env:
    ENGRAM_TEST_MODE: "1"
    ENGRAM_TEST_WORKSPACE: "test"
    WORKSPACE: "test"
  run: go test -v ./...
```

---

## Validation

### Success Criteria

1. ✅ No test sessions in production workspace after test runs
2. ✅ Tests fail immediately with clear error if misconfigured
3. ✅ `testutil.SetupTestEnvironment(t)` works in all test files
4. ✅ Interactive prompt enforces --test flag for test-named sessions
5. ✅ Documentation guides developers to correct usage

### Validation Results

**Before Enforcement**:
- 21+ test-parent sessions in production
- Tests could write to oss/acme workspaces
- No protection mechanism

**After Enforcement**:
- 0 test sessions in production after test runs
- Tests blocked with clear error messages
- Interactive prompt forces --test flag usage

**Test Coverage**:
```bash
# 60+ packages tested
ENGRAM_TEST_MODE=1 ENGRAM_TEST_WORKSPACE=test WORKSPACE=test go test ./...

# Result: 2 expected failures (test database), 60+ pass
# No production pollution verified
```

---

## Related Documents

- `TESTING.md` - Comprehensive test isolation guide
- `cmd/agm/archive_test.go` - Integration example
- `/tmp/test-isolation-summary.md` - Implementation summary

---

## Decision Log

**2026-03-20**: Initial implementation
- Created fail-fast enforcement layer
- Implemented testutil helper package
- Enhanced interactive prompt
- Documented in TESTING.md

**Future Considerations**:
- Add pre-commit hook to verify test environment setup
- Create linter to detect hardcoded workspace values
- Add test coverage metrics to CI/CD
