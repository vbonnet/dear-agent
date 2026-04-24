# Session Lifecycle Tests - Quick Start

## TL;DR

```bash
# Run all tests
cd main/agm
go test ./test/integration/lifecycle/... -v

# Or use the test runner
./scripts/run-lifecycle-tests.sh
```

## What's Tested

### ✅ Core Lifecycle
- Session creation with manifest generation
- Session listing and filtering
- Session archiving and metadata preservation
- Health checks and validation

### ✅ A2A Messaging
- Sending messages between sessions
- Message delivery verification
- Empty/large message handling

### ✅ Error Handling
- Duplicate session prevention
- Missing session handling
- Corrupted manifest recovery
- Permission errors
- Race conditions

### ⏳ Hooks (Future)
- Hook execution (documented, tests skipped)
- Environment variables
- Multi-language support

## Quick Commands

```bash
# All tests (verbose)
go test ./test/integration/lifecycle/... -v

# Fast tests only
go test ./test/integration/lifecycle/... -v -short

# Specific test
go test ./test/integration/lifecycle/... -v -run TestSessionCreation_FullLifecycle

# With coverage
./scripts/run-lifecycle-tests.sh coverage

# With race detection
./scripts/run-lifecycle-tests.sh race

# Compile check
go test -c ./test/integration/lifecycle/...
```

## Test Files

| File | Tests | Purpose |
|------|-------|---------|
| `session_lifecycle_test.go` | 13 | Core lifecycle operations |
| `session_error_scenarios_test.go` | 18 | Error handling |
| `hook_execution_test.go` | 10 | Hook system (future) |

## Prerequisites

- Go 1.21+
- Tmux (for tmux-dependent tests)
- AGM built (`make build`)

## Common Issues

**Tests fail with "tmux not found"**
```bash
# Install tmux
sudo apt install tmux  # or brew install tmux
```

**Tests fail with "csm not found"**
```bash
cd main/agm
make build
```

**Cleanup not working**
```bash
# Manual cleanup
tmux list-sessions | grep csm-test | cut -d: -f1 | xargs -I {} tmux kill-session -t {}
rm -rf /tmp/csm-test-*
```

## Next Steps

- Read [README.md](./README.md) for detailed documentation
- See [SESSION_LIFECYCLE_TESTS.md](../../../docs/SESSION_LIFECYCLE_TESTS.md) for implementation summary
- Check [TEST-PLAN.md](../../../TEST-PLAN.md) for overall testing strategy
