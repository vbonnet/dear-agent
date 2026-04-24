# AGM Session Lifecycle Test Suite

Comprehensive integration tests for Agent Session Manager (AGM) session lifecycle operations.

## Overview

This test suite covers the complete session lifecycle from creation to termination, including:
- Session creation and initialization
- Hook execution (pre/post init, archive)
- Agent-to-Agent (A2A) messaging
- Session termination and cleanup
- Error scenarios and edge cases
- Concurrent operations
- Resource management

## Test Files

### `session_lifecycle_test.go`
Core lifecycle tests covering the happy path and standard operations.

**Tests:**
- `TestSessionCreation_FullLifecycle` - Complete session lifecycle (create → list → archive)
- `TestSessionCreation_WithHooks` - Hook execution during creation
- `TestSessionTermination_CleanupResources` - Resource cleanup on termination
- `TestA2AMessaging_SendReceive` - Inter-session messaging
- `TestSessionCreation_ManifestFields` - Manifest field validation
- `TestSessionHealth_Checks` - Health check functionality
- `TestSessionCleanup_OnError` - Error cleanup behavior
- `TestConcurrentSessions_NoConflict` - Multiple concurrent sessions
- `TestSessionReidentify_AfterTmuxRestart` - Recovery after tmux restart
- `TestPromptDetection` - Command prompt detection
- `TestSessionArchive_PreservesMetadata` - Metadata preservation during archive

**Coverage:**
- Session creation workflow
- Manifest CRUD operations
- Health checks
- Cleanup procedures
- Concurrent session management
- Metadata preservation

### `session_error_scenarios_test.go`
Error handling and edge case tests.

**Tests:**
- `TestError_CreateDuplicateSession` - Duplicate session prevention
- `TestError_ResumeMissingSession` - Missing session handling
- `TestError_ArchiveActiveSession` - Active session archive behavior
- `TestError_SendToMissingSession` - Invalid message target
- `TestError_CorruptedManifest` - Corrupted manifest handling
- `TestError_MissingManifestField` - Incomplete manifest handling
- `TestError_ConcurrentArchive` - Concurrent archive operations
- `TestError_InvalidSessionName` - Invalid name validation
- `TestError_DiskFullSimulation` - Disk space exhaustion
- `TestError_PermissionDenied` - Permission errors
- `TestError_TmuxServerDead` - Tmux server failure
- `TestError_ManifestVersionMismatch` - Schema version compatibility
- `TestError_RaceConditionManifestUpdate` - Concurrent manifest updates
- `TestError_SendEmptyMessage` - Empty message handling
- `TestError_MessageTooLarge` - Large message handling

**Coverage:**
- Error detection and reporting
- Input validation
- Race conditions
- Resource constraints
- Recovery mechanisms
- Error messages and user guidance

### `hook_execution_test.go`
Hook system tests (documents expected behavior for future implementation).

**Tests:**
- `TestHooks_PostInitExecution` - Post-init hook execution
- `TestHooks_PreArchiveExecution` - Pre-archive hook execution
- `TestHooks_ErrorHandling` - Hook error handling
- `TestHooks_EnvironmentVariables` - Hook environment setup
- `TestHooks_ExecutionOrder` - Multiple hook ordering
- `TestHooks_Timeout` - Hook timeout handling
- `TestHooks_ShellTypes` - Multi-language hook support
- `TestAssociateCommand_SendsRename` - Command ordering verification
- `TestHookDirectory_Discovery` - Hook discovery locations
- `TestHooks_AsyncExecution` - Asynchronous hook execution

**Coverage:**
- Hook lifecycle (pre/post init, archive)
- Environment variable injection
- Error handling and timeouts
- Multi-language support (bash, python, go)
- Hook discovery and priority

### `edge_cases_test.go` (existing)
Edge case tests for specific scenarios.

## Running Tests

### Run All Lifecycle Tests
```bash
cd main/agm
go test ./test/integration/lifecycle/... -v
```

### Run Specific Test File
```bash
go test ./test/integration/lifecycle/session_lifecycle_test.go -v
go test ./test/integration/lifecycle/session_error_scenarios_test.go -v
go test ./test/integration/lifecycle/hook_execution_test.go -v
```

### Run Specific Test
```bash
go test ./test/integration/lifecycle/... -v -run TestSessionCreation_FullLifecycle
go test ./test/integration/lifecycle/... -v -run TestError_CorruptedManifest
```

### Run Short Tests Only (fast tests)
```bash
go test ./test/integration/lifecycle/... -v -short
```

### Run with Coverage
```bash
go test ./test/integration/lifecycle/... -v -coverprofile=coverage.out
go tool cover -html=coverage.out
```

### Run with Race Detection
```bash
go test ./test/integration/lifecycle/... -v -race
```

## Test Requirements

### Prerequisites
- Go 1.21+
- Tmux installed and available
- AGM binary built (`make build`)
- Sufficient permissions to create temp directories

### Environment Variables
Tests automatically create isolated temporary environments. No special configuration needed.

### Cleanup
Tests automatically clean up resources using `defer env.Cleanup(t)`. If tests are interrupted, you may need to manually clean up:

```bash
# Kill test tmux sessions
tmux list-sessions | grep csm-test | cut -d: -f1 | xargs -I {} tmux kill-session -t {}

# Remove temp directories
rm -rf /tmp/csm-test-*
```

## Test Categories

### Lifecycle Tests (Happy Path)
✅ Session creation with all components
✅ Session listing and filtering
✅ Session archiving and restoration
✅ Metadata preservation
✅ Health checks
✅ Concurrent sessions

### Error Handling Tests
✅ Invalid inputs (names, paths, etc.)
✅ Missing resources (sessions, files)
✅ Corrupted data (manifests, configs)
✅ Resource constraints (disk, permissions)
✅ Race conditions
✅ Concurrent operations

### Hook Tests (Future Implementation)
⏳ Hook discovery and execution
⏳ Environment variable injection
⏳ Error handling and timeouts
⏳ Multi-language support
⏳ Execution ordering

### Messaging Tests
✅ A2A message sending
✅ Message delivery verification
✅ Large message handling
✅ Empty message handling

## Test Patterns

### Test Structure
```go
func TestFeature_Scenario(t *testing.T) {
    // Setup
    env := helpers.NewTestEnv(t)
    defer env.Cleanup(t)

    // Execute
    // ... test logic ...

    // Verify
    // ... assertions ...
}
```

### Using Test Helpers
```go
// Create session manifest
helpers.CreateSessionManifest(env.SessionsDir, sessionName, "claude")

// Archive session
helpers.ArchiveTestSession(env.SessionsDir, sessionName, "reason")

// List sessions
sessions, err := helpers.ListTestSessions(env.SessionsDir, helpers.ListFilter{All: true})

// Check tmux availability
if !helpers.IsTmuxAvailable() {
    t.Skip("Tmux not available")
}
```

### Isolated Testing
Each test creates isolated temp directories to avoid conflicts:
```go
env := helpers.NewTestEnv(t)  // Creates /tmp/csm-test-<timestamp>/
defer env.Cleanup(t)          // Removes temp directory
```

## Coverage Goals

| Component | Target | Current | Priority |
|-----------|--------|---------|----------|
| Session Creation | 90% | TBD | High |
| Session Archiving | 85% | TBD | High |
| A2A Messaging | 80% | TBD | Medium |
| Hook Execution | 75% | 0% | Low (not implemented) |
| Error Handling | 80% | TBD | High |
| Cleanup | 85% | TBD | High |

## Known Issues & Limitations

### Current Limitations
1. **Hook tests are skipped** - Hook execution not yet implemented in AGM
2. **Tmux server restart test skipped** - Dangerous to kill tmux server in CI
3. **Disk full test skipped** - Requires special filesystem setup
4. **Some error tests may succeed** - AGM may sanitize invalid inputs

### Test Stability
- Tests requiring tmux may be flaky in CI environments without proper tmux setup
- Concurrent tests may exhibit race conditions on slow systems
- Timing-dependent tests (prompt detection) may need adjustment

### Future Improvements
- [ ] Add more concurrent operation tests
- [ ] Implement hook execution tests when hooks are available
- [ ] Add performance benchmarks
- [ ] Add stress tests (many concurrent sessions)
- [ ] Add tmux socket isolation tests
- [ ] Add manifest migration tests (v1 → v2)

## Contributing

### Adding New Tests
1. Choose appropriate test file based on category
2. Follow naming convention: `Test<Component>_<Scenario>`
3. Use table-driven tests for multiple similar cases
4. Add test to this README under appropriate section
5. Ensure cleanup with `defer env.Cleanup(t)`

### Test Naming Convention
```
Test<Component>_<Scenario>
```

Examples:
- `TestSessionCreation_FullLifecycle`
- `TestError_CorruptedManifest`
- `TestHooks_PostInitExecution`

### Best Practices
1. **Use helpers** - Prefer `helpers.CreateSessionManifest()` over manual creation
2. **Clean up resources** - Always use `defer env.Cleanup(t)`
3. **Skip when appropriate** - Use `t.Skip()` for tests requiring special setup
4. **Test isolation** - Each test should be independent
5. **Descriptive errors** - Use `t.Logf()` to provide context on failures
6. **Check prerequisites** - Skip tests if tmux/other tools unavailable

## Related Documentation

- [Test Plan](../../../TEST-PLAN.md) - Overall testing strategy
- [Integration Test README](../README.md) - General integration test info
- [AGM Architecture](../../../docs/architecture.md) - System design
- [Hook Specification](../../../docs/hooks.md) - Hook system design (future)

## Maintenance

### Regular Tasks
- [ ] Update coverage metrics monthly
- [ ] Review and update skipped tests
- [ ] Add tests for new features
- [ ] Update this README when adding tests
- [ ] Monitor test execution time

### Test Health Checklist
- [ ] All non-skipped tests passing
- [ ] No flaky tests
- [ ] Coverage targets met
- [ ] Cleanup working (no temp files left)
- [ ] Race detector clean

## Contact

For questions or issues with these tests:
- File issue: [AGM Issues](https://github.com/vbonnet/dear-agent/issues)
- Check TEST-PLAN.md for testing strategy
- See CONTRIBUTING.md for development guidelines
