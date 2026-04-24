# Wayfinder Workspace Isolation Tests

Comprehensive test suite for verifying workspace isolation in multi-workspace Wayfinder deployments.

## Overview

This test suite ensures that OSS and Acme (or other) workspaces maintain complete data isolation with zero cross-contamination. This is a **critical security and privacy requirement** for multi-workspace Wayfinder.

## Architecture

### Workspace Detection

Workspaces are detected from project paths using the pattern:
```
~/src/ws/{workspace}/wf/{project}
```

Example:
- OSS workspace: `the git history`
- Acme workspace: `~/src/ws/acme/wf/my-project`

### Test Components

1. **workspace_isolation_test.go** - Core isolation tests
2. **testdata_generator.go** - Test data generation utilities
3. **benchmark_test.go** - Performance benchmarks
4. **workspace.go** - Helper functions for workspace detection and management

## Running Tests

### Quick Start

```bash
# Run all tests with automation script
./run_tests.sh
```

### Individual Test Suites

```bash
# Unit tests only
go test -v -run "^Test"

# Integration tests (workspace isolation)
WAYFINDER_TEST_INTEGRATION=1 go test -v -run "TestWorkspaceIsolation"

# Edge case tests
WAYFINDER_TEST_INTEGRATION=1 go test -v -run "TestWorkspaceFilterEdgeCases"

# Test data generator
go test -v -run "TestGenerateTestData"

# Performance benchmarks
go test -bench=. -benchmem

# With race detector
go test -race -run "^Test"

# With coverage
go test -cover -coverprofile=coverage.out
go tool cover -html=coverage.out
```

## Test Coverage

### Test Categories

1. **Workspace Detection** (8 tests)
   - Path-based workspace detection
   - Invalid path handling
   - Edge cases

2. **Project Isolation** (15 tests)
   - Project creation in separate workspaces
   - Overlapping project names
   - Session ID uniqueness
   - Phase data isolation

3. **Data Integrity** (12 tests)
   - No cross-contamination
   - Update operations respect boundaries
   - Delete operations respect boundaries
   - List operations filter by workspace

4. **Performance** (10 benchmarks)
   - Load/save operations
   - List projects
   - Workspace detection
   - Monolithic vs multi-workspace comparison
   - Scalability tests (10-200 projects)

### Critical Security Tests

These tests verify **zero cross-contamination**:

```go
// CRITICAL: Verify no cross-contamination
if ossLoaded.CurrentPhase == acmeLoaded.CurrentPhase {
    t.Error("SECURITY VIOLATION: OSS and Acme projects have same phase")
}
```

All tests marked with `SECURITY VIOLATION` are **zero-tolerance** failures that must be fixed immediately.

## Test Data Generation

### Creating Test Data

```go
config := TestDataConfig{
    RootDir:           "/tmp/test",
    OSSProjects:       3,
    AcmeProjects:    3,
    IncludePhaseFiles: true,
}

if err := GenerateTestData(config); err != nil {
    log.Fatal(err)
}
```

### Validating Test Data

```go
result, err := ValidateTestData(config)
if err != nil {
    log.Fatal(err)
}

if !result.IsValid {
    for _, violation := range result.Violations {
        fmt.Printf("Violation: %s\n", violation)
    }
}
```

## Performance Requirements

### Target: <10ms Overhead

Multi-workspace operations should add **less than 10ms overhead** compared to monolithic Wayfinder.

### Benchmark Results

Expected performance (on typical hardware):

| Operation | Target | Typical |
|-----------|--------|---------|
| Load project | <5ms | ~2ms |
| Save project | <5ms | ~3ms |
| Detect workspace | <1ms | ~0.1ms |
| List 10 projects | <20ms | ~10ms |
| List 100 projects | <100ms | ~50ms |

Run benchmarks to verify:

```bash
go test -bench=BenchmarkWorkspaceQueries -benchmem
```

## Integration with AGM

This test suite is modeled after the AGM workspace isolation tests at:
```
main/agm/internal/dolt/workspace_isolation_test.go
```

Key differences:
- **AGM**: Uses Dolt database with workspace column
- **Wayfinder**: Uses filesystem with workspace-based directory structure

Both achieve the same goal: **complete workspace isolation**.

## Validation Checklist

Before deploying multi-workspace Wayfinder:

- [ ] All isolation tests pass
- [ ] Zero cross-contamination violations
- [ ] Performance within acceptable limits (<10ms overhead)
- [ ] Edge cases handled correctly
- [ ] Race detector passes
- [ ] Test coverage >80%
- [ ] Documentation reviewed
- [ ] Manual testing in both workspaces

## Known Limitations

### 1. Filesystem-Based Isolation

**Limitation**: Relies on directory structure for isolation.

**Mitigation**:
- Validate paths on every operation
- Test workspace detection thoroughly
- Use absolute paths consistently

**Risk Level**: LOW (with proper validation)

### 2. No Database Constraints

**Limitation**: Unlike AGM (which uses database constraints), Wayfinder relies on application logic.

**Mitigation**:
- Comprehensive test coverage
- Automated validation in CI/CD
- Regular security audits

**Risk Level**: MEDIUM (requires discipline)

### 3. Manual Workspace Setup

**Limitation**: Users must manually create workspace directories.

**Mitigation**:
- Clear documentation
- Setup automation scripts
- Validation warnings

**Risk Level**: LOW (operational issue, not security)

### 4. Shared Git History

**Limitation**: Both workspaces may share the same git repository.

**Mitigation**:
- Use separate branches or repos for sensitive work
- Git hooks to prevent cross-workspace commits
- Regular audits of git history

**Risk Level**: MEDIUM (depends on workflow)

### 5. Performance at Scale

**Limitation**: Filesystem operations slower than database queries at scale (1000+ projects).

**Mitigation**:
- Archive old projects regularly
- Use indexes if needed
- Consider database backend for large deployments

**Risk Level**: LOW (manageable with archival)

## Troubleshooting

### Test Failures

**Symptom**: `SECURITY VIOLATION` errors in test output

**Cause**: Cross-contamination detected between workspaces

**Fix**:
1. Check workspace detection logic in `workspace.go`
2. Verify directory paths are correct
3. Review recent code changes
4. Run tests in isolation to identify the issue

### Performance Issues

**Symptom**: Benchmarks show >10ms overhead

**Cause**: Inefficient file I/O or path operations

**Fix**:
1. Profile with `go test -bench=. -cpuprofile=cpu.prof`
2. Analyze with `go tool pprof cpu.prof`
3. Optimize hot paths
4. Consider caching workspace detection

### Race Conditions

**Symptom**: Race detector failures

**Cause**: Concurrent access to shared state

**Fix**:
1. Use mutexes for shared data
2. Make workspace detection stateless
3. Ensure tests are independent

## Future Enhancements

### Potential Improvements

1. **Database Backend**: Add optional database storage for better performance
2. **Workspace Configuration**: Support workspace-specific settings
3. **Cross-Workspace Search**: Safe search across workspaces with explicit opt-in
4. **Audit Logging**: Track all cross-workspace operations
5. **Workspace Quotas**: Limit resources per workspace

### Migration Path

If database backend is needed:

1. Keep filesystem as default
2. Add database adapter interface
3. Implement database backend (SQLite, Postgres)
4. Maintain backward compatibility
5. Provide migration tools

## References

- **AGM Workspace Tests**: `main/agm/internal/dolt/workspace_isolation_test.go`
- **Wayfinder Status**: `cortex/cmd/wayfinder-session/internal/status/`
- **Multi-Workspace Design**: Architecture documentation (if available)

## Support

For issues or questions:

1. Review this README
2. Check test output for specific errors
3. Run tests in verbose mode: `go test -v`
4. Review git history for recent changes
5. Consult team workspace isolation expert

---

**Last Updated**: 2026-02-19
**Test Suite Version**: 1.0
**Maintainer**: Engram Team
