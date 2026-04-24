# Quick Start - Wayfinder V2 Integration Tests

Get up and running with the integration test suite in 5 minutes.

## Prerequisites

- Go 1.21 or higher
- Git installed and configured
- bash shell

## Quick Test Run

```bash
# 1. Navigate to cortex directory
cd cortex

# 2. Run all tests with make
make -C test/integration test

# OR use the test runner script
cd test/integration
chmod +x run_tests.sh
./run_tests.sh
```

## Run Specific Tests

```bash
cd cortex/test/integration

# E2E full workflow
make test-e2e

# D4 stakeholder approval
make test-d4

# S6 test generation
make test-s6

# S8 BUILD loop
make test-s8

# Risk-adaptive workflows
make test-risk

# Phase transitions
make test-transitions

# Schema validation
make test-schema
```

## Generate Coverage Report

```bash
cd cortex/test/integration
make coverage
# Opens coverage.html in browser
```

## Test Output Examples

### Success
```
=== RUN   TestE2E_V2FullWorkflow
    wayfinder_v2_test.go:55: Testing phase 1/14: discovery.problem
    wayfinder_v2_test.go:55: Testing phase 2/14: discovery.solutions
    ...
    wayfinder_v2_test.go:113: ✅ Full V2 workflow completed successfully
--- PASS: TestE2E_V2FullWorkflow (15.23s)
PASS
```

### Failure
```
=== RUN   TestE2E_V2FullWorkflow
    wayfinder_v2_test.go:42: Expected version v2, got v1
--- FAIL: TestE2E_V2FullWorkflow (0.12s)
FAIL
```

## Common Issues

### Issue: wayfinder-session: command not found
**Solution**:
```bash
cd cortex
go build -o /tmp/wayfinder-session ./cmd/wayfinder-session
export PATH="/tmp:$PATH"
```

### Issue: Git not configured
**Solution**:
```bash
git config --global user.email "test@example.com"
git config --global user.name "Test User"
```

### Issue: Tests take too long
**Solution**: Run in short mode to skip integration tests
```bash
make test-short
# OR
./run_tests.sh --short
```

## What Each Test Does

| Test | Duration | What It Tests |
|------|----------|---------------|
| TestE2E_V2FullWorkflow | ~15s | Complete 14-phase workflow |
| TestD4_StakeholderApproval | ~5s | Stakeholder sign-off in D4 |
| TestS6_TestsFeatureGeneration | ~5s | TESTS.feature creation |
| TestS8_BuildLoop | ~5s | TDD enforcement in BUILD |
| TestRiskAdaptiveReview | ~10s | 5 project size scenarios |
| TestPhaseTransitions | ~3s | Phase ordering validation |
| TestSchemaValidation | ~2s | V2 schema compliance |

**Total Runtime**: ~45 seconds for full suite

## Makefile Targets

```bash
make build        # Build wayfinder-session binary
make test         # Run all integration tests
make test-short   # Skip integration tests
make test-e2e     # Run E2E test only
make test-d4      # Run D4 test only
make test-s6      # Run S6 test only
make test-s8      # Run S8 test only
make test-risk    # Run risk-adaptive test only
make test-transitions  # Run transitions test only
make test-schema  # Run schema test only
make coverage     # Generate coverage report
make clean        # Clean build artifacts
make help         # Show all targets
```

## Manual Test Execution

If you prefer manual control:

```bash
# Build binary
cd cortex
go build -o /tmp/wayfinder-session ./cmd/wayfinder-session

# Add to PATH
export PATH="/tmp:$PATH"

# Verify binary works
wayfinder-session --version

# Run tests
cd test/integration
go test -v ./...

# Run specific test
go test -v ./... -run TestE2E_V2FullWorkflow

# Run with coverage
go test -v -cover ./...
```

## Next Steps

After running tests:

1. Check test output for any failures
2. Review coverage report (if generated)
3. Read [README.md](README.md) for detailed test documentation
4. Review [TEST_SCENARIOS.md](TEST_SCENARIOS.md) for scenario details
5. Check [IMPLEMENTATION_SUMMARY.md](IMPLEMENTATION_SUMMARY.md) for complete overview

## Getting Help

- **Test Documentation**: [README.md](README.md)
- **Test Scenarios**: [TEST_SCENARIOS.md](TEST_SCENARIOS.md)
- **Implementation Details**: [IMPLEMENTATION_SUMMARY.md](IMPLEMENTATION_SUMMARY.md)
- **V2 Schema**: `cortex/cmd/wayfinder-session/internal/status/types.go`

## One-Liner Test Run

```bash
cd cortex && make -C test/integration test
```

That's it! You're ready to run the Wayfinder V2 integration tests.
