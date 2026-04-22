# Testing Guide - agm

**Last Updated**: 2026-03-20

## Test Workspace Isolation

**CRITICAL**: All tests MUST use isolated test workspace to prevent production data pollution.

### Why Isolation Matters

Tests that write to production workspaces (oss, acme) cause:
- Data pollution in production Dolt databases
- Test artifacts appearing in production session lists
- Data cleanup burden
- Potential data corruption

### Enforcement Mechanisms

Two layers of enforcement prevent production pollution:

1. **Fail-Fast Enforcement** (`internal/dolt/adapter.go`)
   - Detects test execution context
   - Blocks production workspace names: oss, acme, prod, production, main
   - Requires `ENGRAM_TEST_MODE=1` environment variable

2. **Interactive Prompt** (`cmd/agm/new.go`)
   - Detects "test" anywhere in session name (case-insensitive)
   - Forces user to use `--test` flag or rename
   - No CLI bypass - scripts MUST use `--test` flag explicitly

## Quick Start - Writing Tests

### Using testutil Package (Recommended)

```go
import (
    "testing"
    "github.com/vbonnet/ai-tools/agm/internal/testutil"
)

func TestExample(t *testing.T) {
    // Setup test environment - prevents production pollution
    testutil.SetupTestEnvironment(t)

    // Your test code here
    // WORKSPACE environment variable is now set to "test"
}
```

### Manual Setup (Alternative)

```go
func TestExample(t *testing.T) {
    // Set test mode
    os.Setenv("ENGRAM_TEST_MODE", "1")
    t.Cleanup(func() { os.Unsetenv("ENGRAM_TEST_MODE") })

    // Set test workspace
    os.Setenv("ENGRAM_TEST_WORKSPACE", "test")
    t.Cleanup(func() { os.Unsetenv("ENGRAM_TEST_WORKSPACE") })

    // Set WORKSPACE for Dolt
    originalWorkspace := os.Getenv("WORKSPACE")
    os.Setenv("WORKSPACE", "test")
    t.Cleanup(func() {
        if originalWorkspace != "" {
            os.Setenv("WORKSPACE", originalWorkspace)
        } else {
            os.Unsetenv("WORKSPACE")
        }
    })

    // Your test code here
}
```

## Environment Variables

### ENGRAM_TEST_MODE
- **Purpose**: Signals code is running in test context
- **Required**: YES (tests fail without it)
- **Value**: `"1"` or `"true"`
- **Set by**: `testutil.SetupTestEnvironment(t)` or manual setup

### ENGRAM_TEST_WORKSPACE
- **Purpose**: Specifies test workspace name
- **Required**: YES (when ENGRAM_TEST_MODE=1)
- **Value**: `"test"` (recommended) or custom test workspace name
- **Set by**: `testutil.SetupTestEnvironment(t)` or manual setup

### WORKSPACE
- **Purpose**: Dolt workspace/database selector
- **Required**: YES (Dolt adapter requires it)
- **Value**: Must match ENGRAM_TEST_WORKSPACE value
- **Set by**: `testutil.SetupTestEnvironment(t)` or manual setup

## Running Tests

### Run All Tests (Recommended)
```bash
ENGRAM_TEST_MODE=1 ENGRAM_TEST_WORKSPACE=test WORKSPACE=test \
  go test ./...
```

### Run Specific Package
```bash
ENGRAM_TEST_MODE=1 ENGRAM_TEST_WORKSPACE=test WORKSPACE=test \
  go test -v ./cmd/agm
```

### Run Single Test
```bash
ENGRAM_TEST_MODE=1 ENGRAM_TEST_WORKSPACE=test WORKSPACE=test \
  go test -v -run TestArchiveSession_Success ./cmd/agm
```

### CI/CD Integration
```yaml
test:
  runs-on: ubuntu-latest
  steps:
    - uses: actions/checkout@v3
    - uses: actions/setup-go@v4
      with:
        go-version: '1.21'
    - name: Run tests with isolation
      run: |
        export ENGRAM_TEST_MODE=1
        export ENGRAM_TEST_WORKSPACE=test
        export WORKSPACE=test
        go test -v ./...
```

## Fail-Fast Error Messages

### Production Workspace Blocked

```
TEST POLLUTION BLOCKED: Tests cannot write to production workspace 'oss'

Why: Production workspaces contain real data that tests would corrupt.

Fix: Set ENGRAM_TEST_WORKSPACE to a test-specific value:
  ENGRAM_TEST_MODE=1 ENGRAM_TEST_WORKSPACE=test go test ./...

Or use testutil.SetupTestEnvironment(t) which auto-sets workspace='test'.
```

**Solution**: Add `testutil.SetupTestEnvironment(t)` to test setup function

### Missing Test Mode

```
TEST POLLUTION BLOCKED: Tests must set ENGRAM_TEST_MODE=1

Why: Without test isolation, tests write to production databases causing data pollution.

Fix: Run tests with proper isolation:
  ENGRAM_TEST_MODE=1 ENGRAM_TEST_WORKSPACE=test go test ./...

Or use testutil.SetupTestEnvironment(t) in your test setup function.
```

**Solution**: Set environment variables or use testutil package

## Common Patterns

### Pattern 1: Test Setup Helper

```go
func setupTest(t *testing.T) {
    t.Helper()
    testutil.SetupTestEnvironment(t)
    // Additional setup
}

func TestExample(t *testing.T) {
    setupTest(t)
    // Test code
}
```

### Pattern 2: Table-Driven Tests

```go
func TestCases(t *testing.T) {
    testutil.SetupTestEnvironment(t) // Once at top level

    tests := []struct {
        name string
        // test fields
    }{
        {name: "case1"},
        {name: "case2"},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // Subtests inherit environment
        })
    }
}
```

### Pattern 3: Getting Test Workspace

```go
func TestExample(t *testing.T) {
    testutil.SetupTestEnvironment(t)

    // Get workspace from environment
    testWorkspace := os.Getenv("WORKSPACE")
    if testWorkspace == "" {
        testWorkspace = "test"
    }

    // Use in manifest/config
    manifest := &manifest.Manifest{
        Workspace: testWorkspace,
        // ...
    }
}
```

## Migrating Existing Tests

### Before (Production Pollution)

```go
func TestExample(t *testing.T) {
    os.Setenv("WORKSPACE", "oss")  // ❌ Hardcoded production

    manifest := &manifest.Manifest{
        Workspace: "oss",  // ❌ Hardcoded production
    }

    adapter, _ := dolt.DefaultConfig()  // ❌ Uses production database
}
```

### After (Proper Isolation)

```go
func TestExample(t *testing.T) {
    testutil.SetupTestEnvironment(t)  // ✅ Auto-sets test workspace

    testWorkspace := os.Getenv("WORKSPACE")
    manifest := &manifest.Manifest{
        Workspace: testWorkspace,  // ✅ Uses test workspace
    }

    adapter, _ := dolt.DefaultConfig()  // ✅ Uses test database
}
```

## Verification

### Check No Production Pollution

```bash
# Count sessions before tests
BEFORE=$(agm session list --json | jq '.sessions | length')

# Run tests
ENGRAM_TEST_MODE=1 ENGRAM_TEST_WORKSPACE=test WORKSPACE=test go test ./...

# Count sessions after tests
AFTER=$(agm session list --json | jq '.sessions | length')

# Verify count unchanged
test "$BEFORE" -eq "$AFTER" && echo "✓ No pollution" || echo "✗ Tests created production sessions!"
```

### Test Isolation Manually

```bash
# This should FAIL with clear error
go test -v -run TestArchiveSession_Success ./cmd/agm

# This should PASS (or fail for other reasons, but not pollution)
ENGRAM_TEST_MODE=1 ENGRAM_TEST_WORKSPACE=test WORKSPACE=test \
  go test -v -run TestArchiveSession_Success ./cmd/agm
```

## Interactive Session Creation

When creating sessions interactively with names containing "test":

```bash
# This will trigger interactive prompt
agm session new my-test-session

# Prompt shows:
# ⚠️  Test Pattern Detected - Action Required
#
# Options:
#   1. Use --test flag → Isolated test workspace
#   2. Cancel and rename → Remove 'test' from name

# Scripts MUST use --test flag (no bypass)
agm session new --test my-test-session
```

## References

- **Fail-fast enforcement**: `internal/dolt/adapter.go`
- **Test utilities**: `internal/testutil/environment.go`
- **Interactive prompt**: `cmd/agm/new.go`
- **Example integration**: `cmd/agm/archive_test.go`

## Troubleshooting

### Test fails with "WORKSPACE environment variable not set"

**Solution**: Add `testutil.SetupTestEnvironment(t)` to test setup

### Test fails with "Tests cannot write to production workspace 'oss'"

**Solution**: Remove hardcoded `os.Setenv("WORKSPACE", "oss")` and use testutil

### Tests pass locally but fail in CI/CD

**Solution**: Ensure CI/CD sets environment variables (see CI/CD Integration above)

### Existing tests suddenly failing after upgrade

**Solution**: This is expected - tests need migration to use proper isolation. See "Migrating Existing Tests" section.
