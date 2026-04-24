# Testing Documentation

This directory contains comprehensive testing documentation for AGM (AI Agent Manager).

## Documents

### [ARCHIVE-DOLT-RUNBOOK.md](./ARCHIVE-DOLT-RUNBOOK.md)
**Purpose**: Manual testing runbook for the Dolt-based archive migration

**Use When**:
- Verifying the archive command fix
- Testing Dolt-based identifier resolution
- Regression testing after code changes
- QA sign-off before deployment

**Contents**:
- Step-by-step test scenarios
- Expected outputs for each scenario
- Dolt query verification
- Rollback procedures
- Sign-off checklist

**Quick Start**:
```bash
# Follow the runbook scenarios in order
cat docs/testing/ARCHIVE-DOLT-RUNBOOK.md
```

---

## Test Types

### Unit Tests

**Location**: `internal/*/.../*_test.go`

**Run All**:
```bash
cd main/agm
go test ./... -v
```

**Run Specific Package**:
```bash
go test ./internal/dolt/... -v
go test ./cmd/agm/... -v
```

**Coverage Report**:
```bash
go test ./... -cover
go test ./... -coverprofile=coverage.out
go tool cover -html=coverage.out
```

**Key Test Files**:
- `internal/dolt/adapter_test.go` - Dolt adapter tests (CRUD, ResolveIdentifier)
- `internal/session/session_test.go` - Session management tests
- `internal/manifest/migrate_test.go` - Manifest migration tests

---

### Integration Tests

**Location**: `test/integration/**/*_test.go`

**Prerequisites**:
- Dolt server running
- `DOLT_TEST_INTEGRATION=1` environment variable set
- Test workspace configured

**Run All Integration Tests**:
```bash
export DOLT_TEST_INTEGRATION=1
go test ./test/integration/... -tags=integration -v
```

**Run Lifecycle Tests**:
```bash
export DOLT_TEST_INTEGRATION=1
go test ./test/integration/lifecycle/... -tags=integration -v
```

**Key Test Files**:
- `test/integration/lifecycle/archive_test.go` - Archive command integration tests
- `test/integration/lifecycle/lifecycle_suite_test.go` - Full lifecycle tests
- `test/integration/lifecycle/list_test.go` - List command tests

---

### Manual Testing

**Runbooks**:
- [ARCHIVE-DOLT-RUNBOOK.md](./ARCHIVE-DOLT-RUNBOOK.md) - Archive command verification

**When to Use**:
- Pre-release verification
- Feature acceptance
- Bug reproduction
- Performance testing
- User acceptance testing (UAT)

**Process**:
1. Review runbook scenarios
2. Execute each scenario step-by-step
3. Document actual vs expected results
4. Complete sign-off checklist
5. Report any discrepancies

---

## Test Environment Setup

### Local Development

```bash
# 1. Set workspace
export WORKSPACE=oss  # or your workspace

# 2. Start Dolt SQL server
cd ~/src/ws/${WORKSPACE}/.dolt/dolt-db
dolt sql-server -H 127.0.0.1 -P 3307 &

# 3. Verify connection
dolt sql -q "SHOW TABLES"

# 4. Build AGM
cd main/agm
go build -o ~/go/bin/agm ./cmd/agm

# 5. Run tests
go test ./... -v
```

### CI/CD Environment

**GitHub Actions** (example):
```yaml
name: Test
on: [push, pull_request]
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - uses: actions/setup-go@v4
        with:
          go-version: '1.21'

      - name: Start Dolt
        run: |
          # Install Dolt
          sudo bash -c 'curl -L https://github.com/dolthub/dolt/releases/latest/download/install.sh | sudo bash'

          # Initialize database
          mkdir -p /tmp/dolt-test
          cd /tmp/dolt-test
          dolt init
          dolt sql -q "CREATE DATABASE test"
          dolt sql-server -H 127.0.0.1 -P 3307 &
          sleep 5

      - name: Run Tests
        env:
          WORKSPACE: test
          DOLT_TEST_INTEGRATION: 1
        run: |
          cd agm
          go test ./... -v
          go test ./test/integration/... -tags=integration -v
```

---

## Test Data Management

### Test Fixtures

**Location**: `test/fixtures/`

**Usage**:
```go
import "github.com/vbonnet/dear-agent/agm/test/fixtures"

// Load test manifest
manifest := fixtures.LoadManifest("test-session.yaml")

// Load test messages
messages := fixtures.LoadMessages("conversation.jsonl")
```

### Test Cleanup

**Automatic Cleanup**:
```go
// Use t.Cleanup() for automatic cleanup
func TestSomething(t *testing.T) {
    adapter := getTestAdapter(t)
    defer adapter.Close()  // Close connection

    session := createTestSession()
    t.Cleanup(func() {
        adapter.DeleteSession(session.SessionID)
    })

    // Test code...
}
```

**Manual Cleanup**:
```bash
# Clean up test sessions
dolt sql -q "DELETE FROM agm_sessions WHERE id LIKE 'test-%'"

# Reset test database
cd ~/src/ws/test/.dolt/dolt-db
dolt sql -q "DROP DATABASE IF EXISTS test"
dolt sql -q "CREATE DATABASE test"
```

---

## Test Coverage Goals

### Current Coverage (2026-03-12)

| Package | Coverage | Status |
|---------|----------|--------|
| `internal/dolt` | 85% | ✅ Good |
| `cmd/agm` | 65% | ⚠️ Needs improvement |
| `internal/session` | 78% | ✅ Good |
| `internal/manifest` | 82% | ✅ Good |
| `internal/tmux` | 70% | ⚠️ Needs improvement |

### Coverage Targets

- **Minimum**: 70% overall coverage
- **Goal**: 80% overall coverage
- **Critical paths**: 90%+ coverage
  - Session creation/archival
  - Dolt adapter operations
  - Identifier resolution

### Measuring Coverage

```bash
# Generate coverage report
go test ./... -coverprofile=coverage.out

# View in browser
go tool cover -html=coverage.out

# View summary
go tool cover -func=coverage.out | grep total
```

---

## Debugging Tests

### Verbose Output

```bash
# Show all test output
go test ./... -v

# Show test names only
go test ./... -v | grep -E "^=== RUN|PASS|FAIL"

# Run specific test
go test ./internal/dolt/... -v -run TestResolveIdentifier
```

### Debug with Delve

```bash
# Install delve
go install github.com/go-delve/delve/cmd/dlv@latest

# Debug specific test
dlv test ./internal/dolt/... -- -test.run TestResolveIdentifier

# In delve:
(dlv) break ResolveIdentifier
(dlv) continue
(dlv) print identifier
```

### Test Logging

```go
func TestSomething(t *testing.T) {
    // Enable verbose logging in tests
    t.Logf("Testing with identifier: %s", identifier)

    // Use t.Helper() to improve stack traces
    t.Helper()

    // Fail with detailed message
    if got != want {
        t.Errorf("got %v, want %v", got, want)
    }
}
```

---

## Best Practices

### Test Naming

```go
// Good: Descriptive names
func TestResolveIdentifierBySessionID(t *testing.T) {}
func TestResolveIdentifierExcludesArchived(t *testing.T) {}

// Bad: Generic names
func TestResolve(t *testing.T) {}
func TestFunction1(t *testing.T) {}
```

### Test Organization

```go
// Use subtests for related scenarios
func TestResolveIdentifier(t *testing.T) {
    t.Run("by session ID", func(t *testing.T) {
        // Test implementation
    })

    t.Run("by tmux name", func(t *testing.T) {
        // Test implementation
    })

    t.Run("excludes archived", func(t *testing.T) {
        // Test implementation
    })
}
```

### Test Isolation

```go
// Bad: Tests depend on each other
func TestCreate(t *testing.T) {
    session = createSession()  // Global state
}

func TestUpdate(t *testing.T) {
    updateSession(session)  // Depends on TestCreate
}

// Good: Tests are independent
func TestCreate(t *testing.T) {
    session := createSession()
    defer deleteSession(session.ID)
    // Test implementation
}

func TestUpdate(t *testing.T) {
    session := createSession()
    defer deleteSession(session.ID)
    updateSession(session)
    // Test implementation
}
```

---

## Continuous Improvement

### Adding New Tests

1. **Identify Gap**: Find untested code path
2. **Write Test**: Create failing test first (TDD)
3. **Implement**: Make test pass
4. **Refactor**: Clean up code and test
5. **Document**: Add test description

### Test Maintenance

- **Review Periodically**: Monthly test review
- **Remove Obsolete**: Delete tests for removed features
- **Update Fixtures**: Keep test data current
- **Improve Coverage**: Target low-coverage areas

### Test Metrics

Track over time:
- Test count (unit + integration)
- Code coverage percentage
- Test execution time
- Flaky test rate
- Bug escape rate (bugs not caught by tests)

---

## Quick Reference

### Essential Commands

```bash
# Run all tests
go test ./...

# Run with coverage
go test ./... -cover

# Run integration tests
DOLT_TEST_INTEGRATION=1 go test ./test/integration/... -tags=integration

# Run specific test
go test ./internal/dolt/... -run TestResolveIdentifier

# Verbose output
go test ./... -v

# Race detection
go test ./... -race
```

### Environment Variables

```bash
export WORKSPACE=oss                # Required for Dolt tests
export DOLT_PORT=3307               # Dolt server port
export DOLT_TEST_INTEGRATION=1      # Enable integration tests
export DOLT_HOST=127.0.0.1          # Dolt server host (default)
```

---

## Support

### Getting Help

- **Documentation**: Start with `docs/` directory
- **Runbooks**: Check `docs/testing/` for procedures
- **Issues**: File bugs in GitHub repository
- **Team**: Open a GitHub issue

### Contributing Tests

See [CONTRIBUTING.md](../../CONTRIBUTING.md) for:
- Test writing guidelines
- Code review process
- CI/CD integration
- Test coverage requirements
