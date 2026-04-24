# AGM Integration Tests

Integration tests for Agent Session Manager (AGM) that verify the `agm new` command and related functionality.

## Overview

This test suite uses Ginkgo/Gomega BDD framework to test AGM integration with:
- Real tmux sessions
- Manifest file creation and validation
- Tmux configuration settings
- Error handling

## Requirements

- **tmux**: Must be installed (`tmux -V`)
- **Go 1.21+**: Required for testing
- **Ginkgo/Gomega**: Test framework (installed via go.mod)

## Running Tests

### Run all integration tests

```bash
cd main/agm
go test -v ./test/integration/...
```

### Run with coverage

```bash
go test -v -cover -coverprofile=coverage.out ./test/integration/...
go tool cover -html=coverage.out
```

### Run specific test file

```bash
go test -v ./test/integration/ -run="SessionCreation"
go test -v ./test/integration/ -run="ManifestValidation"
go test -v ./test/integration/ -run="TmuxConfiguration"
go test -v ./test/integration/ -run="ErrorScenarios"
```

### Run with race detection

```bash
go test -v -race ./test/integration/...
```

## Test Modes

### Mock Mode (Default)

By default, tests use mock Claude implementation for fast, deterministic testing:

```bash
go test -v ./test/integration/...
```

### Real Mode

To test with real Claude binary (requires Claude installed and configured):

```bash
AGM_TEST_CLAUDE_MODE=real go test -v ./test/integration/...
```

**Note**: Real mode is not fully implemented yet. Tests will use mock even in real mode.

## Test Organization

### Test Files

| File | Purpose | Test Count |
|------|---------|------------|
| `session_creation_test.go` | Session creation scenarios | 4 |
| `manifest_validation_test.go` | Manifest validation tests | 6 |
| `tmux_configuration_test.go` | Tmux configuration verification | 4 |
| `error_scenarios_test.go` | Error handling tests | 7 |

**Total**: 21 integration test scenarios

### Helper Files

| File | Purpose |
|------|---------|
| `helpers/claude_interface.go` | Claude interface definition |
| `helpers/claude_mock.go` | Mock Claude implementation |
| `helpers/tmux_helpers.go` | Tmux utility functions |
| `helpers/test_env.go` | Test environment setup |

### Test Data

| File | Purpose |
|------|---------|
| `testdata/manifests/valid-v2.yaml` | Valid v2 manifest fixture |
| `testdata/manifests/missing-session-id.yaml` | Invalid manifest (missing field) |
| `testdata/manifests/invalid-schema.yaml` | Old v1 schema manifest |

## Test Coverage

**Target**: >80% code coverage of Claude-related code paths

**Measured Files**:
- `cmd/csm/new.go`
- `internal/tmux/tmux.go`
- `internal/manifest/manifest.go`
- `internal/agent/claude_adapter.go`

**View Coverage**:
```bash
go test -coverprofile=coverage.out ./test/integration/...
go tool cover -func=coverage.out | grep -E "new.go|tmux.go|manifest.go|claude_adapter.go"
```

## Cleanup

Tests create tmux sessions with the `agm-test-` prefix. These are automatically cleaned up after each test.

**Manual Cleanup** (if tests fail and leave sessions):
```bash
# List test sessions
tmux list-sessions | grep agm-test

# Kill all test sessions
tmux list-sessions -F "#{session_name}" | grep agm-test | xargs -I {} tmux kill-session -t {}
```

## Test Isolation

Each test:
- Creates a unique tmux session (e.g., `agm-test-creation-1705684800123456789`)
- Uses temporary manifest directory (`/tmp/agm-test-sessions/`)
- Cleans up on success
- Preserves sessions on failure for debugging

## Debugging Failed Tests

When a test fails, the session is preserved:

```
Test failed, preserving session: agm-test-creation-1705684800
Attach with: tmux attach -t agm-test-creation-1705684800
```

Attach to the session to inspect state:
```bash
tmux attach -t agm-test-creation-1705684800
```

## Performance

**Target**: <5 minutes for full test suite

**Current**: ~30 seconds (with mock Claude)

## Writing New Tests

### Example Test Structure

```go
var _ = Describe("Feature Name", func() {
    var sessionName string

    BeforeEach(func() {
        sessionName = testEnv.UniqueSessionName("feature")
    })

    AfterEach(func() {
        if !CurrentSpecReport().Failed {
            helpers.KillTmuxSession(sessionName)
            os.RemoveAll(testEnv.ManifestDir(sessionName))
        }
    })

    Context("when condition", func() {
        It("should behavior", func() {
            // Test implementation
        })
    })
})
```

### Table-Driven Tests

```go
DescribeTable("multi-agent tests",
    func(agent string) {
        // Test implementation
    },
    Entry("claude agent", "claude"),
    Entry("gemini agent", "gemini"),
)
```

## CI Integration

Tests can run in CI with mock mode (no Claude binary required):

```yaml
# .github/workflows/test.yml
- name: Run integration tests
  run: go test -v ./test/integration/...
```

## Troubleshooting

### "tmux must be installed"

Install tmux:
```bash
# Ubuntu/Debian
sudo apt-get install tmux

# macOS
brew install tmux
```

### "failed to create tmux session"

Check tmux is running:
```bash
tmux -V
tmux list-sessions
```

### Tests hang or timeout

- Verify tmux server isn't hung: `pkill tmux`, then restart
- Check for leftover test sessions: `tmux list-sessions | grep agm-test`

## Temporal Backend E2E Tests

### Overview

The `temporal_e2e_test.go` file contains end-to-end integration tests for the AGM Temporal backend (Phase 1 deliverable). These tests verify the complete session lifecycle using a real Temporal server.

### Prerequisites for Temporal Tests

1. **Temporal Server Running**:
   ```bash
   cd main/agm
   docker-compose up -d
   ```

2. **Verify Temporal Connectivity**:
   ```bash
   temporal server health
   ```

### Running Temporal E2E Tests

```bash
# Run with integration tag (Temporal tests will skip if server not available)
go test -tags=integration ./test/integration/... -v -run "Temporal"
```

### Test Scenarios (5 total)

1. **Session Creation** - Create session with Temporal backend (`AGM_SESSION_BACKEND=temporal`)
2. **Client Attachment** - Attach/detach clients to running sessions
3. **Workflow State Verification** - Query workflow state via Temporal API
4. **Full Lifecycle** - Complete state transitions (create → active → stopped → archived)
5. **Crash Resilience** - Session survives worker restart (workflow state persists)

### Skip Behavior

Temporal E2E tests automatically skip if Temporal server is not available:

```
S S S S S  # 5 skipped tests (Temporal server not running)
```

This is expected and allows the integration test suite to run without Temporal infrastructure.

### Temporal Test Configuration

- **Task Queue**: `agm-test-queue`
- **Workflow Timeout**: 30 seconds
- **Default Host**: `localhost:7233`
- **Default Namespace**: `default`

### Troubleshooting Temporal Tests

**Tests Skipped**: Temporal server not running → Start with `docker-compose up -d`

**Connection Refused**: Port 7233 not listening → Verify containers: `docker ps | grep temporal`

**Test Timeout**: Increase timeout → `go test -timeout 5m ...`

## Contributing

When adding new integration tests:

1. Follow BDD style (Describe/Context/It)
2. Use unique session names (`testEnv.UniqueSessionName()`)
3. Clean up in AfterEach (with failure preservation)
4. Add table-driven tests for multi-agent scenarios
5. For Temporal tests: use `//go:build integration` tag and graceful skip when server unavailable
6. Update this README if adding new test files
