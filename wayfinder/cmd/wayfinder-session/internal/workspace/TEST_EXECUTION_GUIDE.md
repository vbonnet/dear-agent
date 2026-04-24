# Wayfinder Workspace Isolation - Test Execution Guide

Quick guide for running workspace isolation tests.

## Prerequisites

- Go 1.21 or later
- Wayfinder workspace structure set up
- Test files in place

## Quick Start

### 1. Navigate to Test Directory

```bash
cd cortex/cmd/wayfinder-session/internal/workspace
```

### 2. Run All Tests

```bash
chmod +x run_tests.sh
./run_tests.sh
```

This will:
- Run all unit tests
- Run integration tests
- Run edge case tests
- Run performance benchmarks
- Generate coverage report
- Produce validation summary

Expected runtime: 30-60 seconds

## Individual Test Commands

### Unit Tests Only

```bash
go test -v -run "^Test"
```

### Workspace Isolation Tests

```bash
WAYFINDER_TEST_INTEGRATION=1 go test -v -run "TestWorkspaceIsolation"
```

### Edge Cases

```bash
WAYFINDER_TEST_INTEGRATION=1 go test -v -run "TestWorkspaceFilterEdgeCases"
```

### Test Data Generator

```bash
go test -v -run "TestGenerateTestData"
```

### Performance Benchmarks

```bash
# Quick benchmarks
go test -bench=BenchmarkWorkspaceQueries -benchtime=1s

# Comprehensive benchmarks
go test -bench=. -benchmem -benchtime=3s

# Scalability tests
go test -bench=BenchmarkWorkspaceScalability
```

### Race Detector

```bash
go test -race -run "^Test"
```

### Coverage Report

```bash
go test -cover -coverprofile=coverage.out
go tool cover -html=coverage.out -o coverage.html
open coverage.html  # or xdg-open on Linux
```

## Expected Results

### All Tests Pass

```
PASS: TestWorkspaceIsolation
PASS: TestWorkspaceFilterEdgeCases
PASS: TestGenerateTestData
PASS: TestValidateTestDataFailures
```

### No Security Violations

All tests should complete without any output containing:
```
SECURITY VIOLATION
```

If you see this message, **STOP** and investigate immediately.

### Performance Within Limits

Benchmark results should show:
- Load/Save operations: <5ms
- Workspace detection: <1ms
- List operations: <20ms for 10 projects

## Interpreting Results

### Success

```
✓ All tests passed!
✓ Zero cross-contamination verified
✓ Workspace isolation confirmed
✓ Performance benchmarks completed
```

### Failure

If tests fail, check:

1. **Path Issues**: Verify workspace directory structure
2. **Permissions**: Ensure test directories are writable
3. **Environment**: Check WAYFINDER_TEST_INTEGRATION is set
4. **Dependencies**: Verify all imports are available

## Continuous Integration

### Adding to CI/CD

```yaml
# Example GitHub Actions
- name: Wayfinder Workspace Tests
  run: |
    cd core/cortex/cmd/wayfinder-session/internal/workspace
    WAYFINDER_TEST_INTEGRATION=1 go test -v -race -cover
```

### Pre-commit Hook

```bash
#!/bin/bash
# .git/hooks/pre-commit
cd core/cortex/cmd/wayfinder-session/internal/workspace
go test -run "^Test" || exit 1
```

## Troubleshooting

### Tests Skip with "WAYFINDER_TEST_INTEGRATION not set"

**Solution**:
```bash
export WAYFINDER_TEST_INTEGRATION=1
go test -v
```

### Permission Denied

**Solution**:
```bash
chmod +x run_tests.sh
```

### Import Errors

**Solution**:
```bash
cd cortex/cmd/wayfinder-session
go mod tidy
```

### Benchmark Results Vary

This is normal. Run with `-benchtime=10s` for more stable results:
```bash
go test -bench=. -benchtime=10s
```

## Best Practices

### Before Committing

1. Run full test suite: `./run_tests.sh`
2. Verify no security violations
3. Check performance benchmarks
4. Review coverage report

### Before Deploying

1. Run with race detector: `go test -race`
2. Run benchmarks on production-like hardware
3. Review VALIDATION_REPORT.md
4. Confirm all known limitations are acceptable

### Regular Maintenance

1. Run tests weekly in CI/CD
2. Monitor performance trends
3. Update tests when adding features
4. Review security validation quarterly

## Contact

For issues or questions:
- Check README.md for detailed documentation
- Review VALIDATION_REPORT.md for known limitations
- Consult Wayfinder team

---

**Last Updated**: 2026-02-19
**Version**: 1.0
