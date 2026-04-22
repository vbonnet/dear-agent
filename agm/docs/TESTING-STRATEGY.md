# AGM Testing Strategy
**Last Updated**: 2026-03-17
**Phase**: Phase 3 - Dolt Migration

---

## Overview

AGM uses a **dual-testing approach** during Phase 3 migration:
1. **MockAdapter** for fast unit tests (no database required)
2. **Real Dolt Adapter** for integration tests (database required)

This strategy allows developers to run fast tests locally while ensuring production behavior is validated in CI/CD.

---

## Test Infrastructure

### MockAdapter (`internal/dolt/mock_adapter.go`)

**Purpose**: In-memory implementation of the Storage interface for testing.

**Features**:
- ✅ Thread-safe (concurrent test support)
- ✅ Deep copy isolation (prevents test interference)
- ✅ Full CRUD operations
- ✅ Filtering (lifecycle, agent, workspace, limit/offset)
- ✅ Error simulation (duplicate IDs, not found, closed adapter)
- ✅ Reset() for test cleanup

**When to use**:
- Unit tests that don't need actual database
- Fast test iteration during development
- CI/CD pipelines (no database setup required)
- Testing error conditions

**Example**:
```go
func TestMyFeature(t *testing.T) {
    adapter := dolt.NewMockAdapter()
    defer adapter.Close()

    session := &manifest.Manifest{
        SessionID: "test-123",
        Name:      "test-session",
    }

    err := adapter.CreateSession(session)
    require.NoError(t, err)

    retrieved, err := adapter.GetSession("test-123")
    require.NoError(t, err)
    assert.Equal(t, "test-session", retrieved.Name)
}
```

### Storage Interface (`internal/dolt/storage.go`)

**Purpose**: Abstract interface for polymorphic testing.

**Benefits**:
- Write tests once, run against both MockAdapter and real Adapter
- Easy to swap implementations
- Forces consistent API across implementations

**Example**:
```go
func testStorageOperations(t *testing.T, storage dolt.Storage) {
    // This function works with ANY Storage implementation
    session := &manifest.Manifest{SessionID: "poly-test"}
    err := storage.CreateSession(session)
    require.NoError(t, err)
}

t.Run("with MockAdapter", func(t *testing.T) {
    adapter := dolt.NewMockAdapter()
    defer adapter.Close()
    testStorageOperations(t, adapter)
})

t.Run("with real Adapter", func(t *testing.T) {
    adapter, _ := dolt.New(&dolt.Config{...})
    defer adapter.Close()
    testStorageOperations(t, adapter)
})
```

---

## Test Types

### 1. Unit Tests (MockAdapter)

**Location**: `internal/dolt/mock_adapter_test.go`, `*_test.go` files

**Run Command**:
```bash
go test ./internal/dolt -v
```

**Characteristics**:
- Fast (<100ms for entire suite)
- No external dependencies
- Run on every commit
- Focus on business logic

**Coverage**:
- CRUD operations
- Filtering logic
- Error handling
- Concurrent access
- Edge cases

### 2. Integration Tests (Real Adapter)

**Location**: `internal/dolt/*_integration_test.go`

**Run Command**:
```bash
# Requires Dolt SQL server running
DOLT_TEST_INTEGRATION=1 go test ./internal/dolt -v
```

**Characteristics**:
- Slower (~1-5s per test)
- Requires database setup
- Run in CI/CD only
- Focus on database interactions

**Coverage**:
- SQL query correctness
- Schema migrations
- Transaction handling
- Database-specific behavior

### 3. Integration Tests (MockAdapter Examples)

**Location**: `internal/dolt/mock_adapter_integration_test.go`

**Purpose**: Demonstrate MockAdapter usage patterns for developers.

**Run Command**:
```bash
go test ./internal/dolt -v -run TestMockAdapterIntegration
```

**Coverage**:
- Session lifecycle (create, update, archive, delete)
- Concurrent operations
- Error handling scenarios
- Filter combinations
- Polymorphic testing patterns

---

## Migration Strategy (Phase 3)

### Current State

**✅ Completed**:
- MockAdapter created with 14/14 tests passing
- Storage interface defined
- Integration test examples added
- Documentation created

**🔄 In Progress**:
- Migrating existing tests to use MockAdapter
- Adding MockAdapter support to test utilities

**⏸️ Deferred to Phase 6**:
- Complete removal of YAML-based tests
- Migration of all test fixtures to Dolt

### Decision: Keep YAML Tests for Now

During Phase 3, we maintain **dual-read compatibility**:
- YAML tests remain for backward compatibility validation
- New tests use MockAdapter
- Full YAML removal in Phase 6 (after all code migrated to Dolt)

**Rationale**:
1. Low risk: Existing tests continue to work
2. Validates fallback paths during migration
3. Prevents regressions in YAML code (still used in some commands)
4. Easier rollback if issues found

---

## Best Practices

### 1. Use MockAdapter for Unit Tests

❌ **Don't**:
```go
func TestFeature(t *testing.T) {
    // Requires database, slow, fragile
    adapter, err := dolt.New(&dolt.Config{...})
    // ...
}
```

✅ **Do**:
```go
func TestFeature(t *testing.T) {
    // Fast, reliable, no dependencies
    adapter := dolt.NewMockAdapter()
    defer adapter.Close()
    // ...
}
```

### 2. Clean Up Resources

✅ **Always defer Close()**:
```go
adapter := dolt.NewMockAdapter()
defer adapter.Close()  // ← Critical!
```

### 3. Use Reset() for Test Isolation

```go
func TestSuite(t *testing.T) {
    adapter := dolt.NewMockAdapter()
    defer adapter.Close()

    t.Run("test 1", func(t *testing.T) {
        adapter.Reset()  // Clean slate for each subtest
        // ...
    })

    t.Run("test 2", func(t *testing.T) {
        adapter.Reset()
        // ...
    })
}
```

### 4. Test Error Conditions

```go
func TestErrors(t *testing.T) {
    adapter := dolt.NewMockAdapter()

    // Test duplicate
    session := &manifest.Manifest{SessionID: "dup"}
    adapter.CreateSession(session)
    err := adapter.CreateSession(session)
    assert.Error(t, err)
    assert.Contains(t, err.Error(), "already exists")

    // Test not found
    _, err = adapter.GetSession("missing")
    assert.Error(t, err)
    assert.Contains(t, err.Error(), "not found")

    // Test operations after close
    adapter.Close()
    err = adapter.CreateSession(&manifest.Manifest{SessionID: "after-close"})
    assert.Error(t, err)
    assert.Contains(t, err.Error(), "closed")
}
```

### 5. Write Polymorphic Tests

When possible, write tests that work with both MockAdapter and real Adapter:

```go
func testSessionLifecycle(t *testing.T, storage dolt.Storage) {
    session := &manifest.Manifest{
        SessionID: "lifecycle-test",
        Lifecycle: "",
    }

    // Create
    err := storage.CreateSession(session)
    require.NoError(t, err)

    // Update
    session.Lifecycle = manifest.LifecycleArchived
    err = storage.UpdateSession(session)
    require.NoError(t, err)

    // Verify
    retrieved, err := storage.GetSession("lifecycle-test")
    require.NoError(t, err)
    assert.Equal(t, manifest.LifecycleArchived, retrieved.Lifecycle)
}

// Run against both implementations
t.Run("MockAdapter", func(t *testing.T) {
    testSessionLifecycle(t, dolt.NewMockAdapter())
})

t.Run("RealAdapter", func(t *testing.T) {
    if os.Getenv("DOLT_TEST_INTEGRATION") != "1" {
        t.Skip("Skipping integration test")
    }
    adapter, _ := dolt.New(&dolt.Config{...})
    defer adapter.Close()
    testSessionLifecycle(t, adapter)
})
```

---

## Test Execution

### Local Development

```bash
# Fast unit tests (recommended for TDD)
go test ./internal/dolt -v

# Specific test pattern
go test ./internal/dolt -v -run TestMockAdapter

# With coverage
go test ./internal/dolt -v -coverprofile=coverage.out
go tool cover -html=coverage.out
```

### CI/CD

```bash
# Unit tests (always run)
go test ./... -v

# Integration tests (requires database)
dolt sql-server --port 3307 &
DOLT_TEST_INTEGRATION=1 go test ./internal/dolt -v
```

### Pre-Commit

```bash
# Run before committing
go test ./internal/dolt -v
golangci-lint run ./internal/dolt
```

---

## Common Patterns

### Pattern 1: Test Filters

```go
func TestFilters(t *testing.T) {
    adapter := dolt.NewMockAdapter()
    defer adapter.Close()

    // Create diverse session set
    sessions := []*manifest.Manifest{
        {SessionID: "oss-active", Workspace: "oss", Lifecycle: ""},
        {SessionID: "oss-archived", Workspace: "oss", Lifecycle: "archived"},
    }
    for _, s := range sessions {
        adapter.CreateSession(s)
    }

    // Test combined filter
    filter := &dolt.SessionFilter{
        Workspace: "oss",
        Lifecycle: "archived",
    }
    results, err := adapter.ListSessions(filter)
    require.NoError(t, err)
    assert.Len(t, results, 1)
    assert.Equal(t, "oss-archived", results[0].SessionID)
}
```

### Pattern 2: Test Concurrent Access

```go
func TestConcurrency(t *testing.T) {
    adapter := dolt.NewMockAdapter()
    defer adapter.Close()

    done := make(chan bool, 10)
    for i := 0; i < 10; i++ {
        go func(id int) {
            session := &manifest.Manifest{
                SessionID: fmt.Sprintf("session-%d", id),
            }
            err := adapter.CreateSession(session)
            assert.NoError(t, err)
            done <- true
        }(i)
    }

    // Wait for all goroutines
    for i := 0; i < 10; i++ {
        <-done
    }

    sessions, _ := adapter.ListSessions(&dolt.SessionFilter{})
    assert.Len(t, sessions, 10)
}
```

### Pattern 3: Table-Driven Tests

```go
func TestValidation(t *testing.T) {
    tests := []struct {
        name      string
        session   *manifest.Manifest
        expectErr bool
        errMsg    string
    }{
        {
            name:      "missing session_id",
            session:   &manifest.Manifest{Name: "test"},
            expectErr: true,
            errMsg:    "session_id is required",
        },
        {
            name: "valid session",
            session: &manifest.Manifest{
                SessionID: "valid",
                Name:      "test",
            },
            expectErr: false,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            adapter := dolt.NewMockAdapter()
            defer adapter.Close()

            err := adapter.CreateSession(tt.session)
            if tt.expectErr {
                assert.Error(t, err)
                assert.Contains(t, err.Error(), tt.errMsg)
            } else {
                assert.NoError(t, err)
            }
        })
    }
}
```

---

## Test Coverage

### Current Coverage (Phase 3)

**MockAdapter**: 100% (14/14 unit tests + 3 integration example tests)
- CreateSession: ✅
- GetSession: ✅
- UpdateSession: ✅
- DeleteSession: ✅
- ListSessions: ✅ (all filter combinations)
- Close: ✅
- Reset: ✅
- ApplyMigrations: ✅
- Deep copy: ✅
- Concurrency: ✅

**Real Adapter**: ~80% (8 integration tests, require DOLT_TEST_INTEGRATION=1)
- Session CRUD: ✅
- Message tracking: ✅
- Tool call tracking: ✅
- Identifier resolution: ✅
- Workspace isolation: ✅

**Target Coverage**: >90% for all Dolt-related code by end of Phase 3

---

## Troubleshooting

### Tests Fail with "adapter is closed"

**Cause**: Forgot to defer Close() or calling operations after Close()

**Fix**:
```go
adapter := dolt.NewMockAdapter()
defer adapter.Close()  // ← Add this
```

### Tests Interfere with Each Other

**Cause**: Shared MockAdapter state between subtests

**Fix**: Use Reset() or create new adapter per subtest:
```go
t.Run("test 1", func(t *testing.T) {
    adapter := dolt.NewMockAdapter()  // Fresh instance
    defer adapter.Close()
    // ...
})
```

### Integration Tests Skip

**Cause**: DOLT_TEST_INTEGRATION not set

**Fix**:
```bash
DOLT_TEST_INTEGRATION=1 go test ./internal/dolt -v
```

---

## Future Work (Phase 4+)

### Phase 4: Internal Modules
- Add MockAdapter support to internal/session tests
- Migrate internal/discovery tests
- Migrate internal/detection tests

### Phase 5: Full Test Suite
- Create test helper: setupTestDolt() for all tests
- Migrate all YAML fixture tests
- Add performance benchmarks

### Phase 6: YAML Removal
- Delete all YAML-based tests
- Remove test/fixtures/*.yaml files
- Update test documentation

---

## References

- **MockAdapter Implementation**: `internal/dolt/mock_adapter.go`
- **MockAdapter Tests**: `internal/dolt/mock_adapter_test.go`
- **Integration Examples**: `internal/dolt/mock_adapter_integration_test.go`
- **Storage Interface**: `internal/dolt/storage.go`
- **Real Adapter Tests**: `internal/dolt/adapter_test.go`

---

## Summary

**Key Takeaways**:
1. ✅ Use MockAdapter for fast unit tests (default)
2. ✅ Use real Adapter for integration tests (CI/CD only)
3. ✅ Always defer Close()
4. ✅ Use Reset() for test isolation
5. ✅ Write polymorphic tests when possible
6. ✅ Test error conditions explicitly
7. ✅ Keep YAML tests during Phase 3, remove in Phase 6

**Test Execution Time**:
- Unit tests (MockAdapter): <100ms for full suite
- Integration tests (real Adapter): ~5s for full suite

**Next Steps**:
- Review `mock_adapter_integration_test.go` for usage examples
- Add MockAdapter to your tests
- Run `go test ./internal/dolt -v` to validate
