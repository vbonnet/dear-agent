# Session Lifecycle Test Coverage

## Overview

This document provides comprehensive documentation for the session lifecycle tests in the Agent Session Manager (AGM). These tests cover all critical session operations across all supported agent types (Claude, Gemini, GPT).

## Test Files

### 1. `session_lifecycle_test.go`
**Purpose**: Basic session lifecycle operations
**Coverage**:
- Session creation with AGM new command
- Manifest validation (fields, timestamps, UUIDs)
- Session listing and filtering
- Session archiving
- Lifecycle state transitions
- Metadata preservation during archive
- Hook execution (placeholder for future implementation)
- Health checks
- A2A (agent-to-agent) messaging
- Cleanup on termination

**Key Tests**:
- `TestSessionCreation_FullLifecycle`: Complete workflow from creation to archive
- `TestSessionCreation_WithHooks`: Hook execution order (pending implementation)
- `TestSessionTermination_CleanupResources`: Resource cleanup verification
- `TestA2AMessaging_SendReceive`: Inter-session messaging
- `TestSessionCreation_ManifestFields`: Manifest field validation
- `TestSessionHealth_Checks`: Health check functionality
- `TestSessionArchive_PreservesMetadata`: Metadata preservation
- `TestConcurrentSessions_NoConflict`: Multiple concurrent sessions
- `TestSessionReidentify_AfterTmuxRestart`: Session recovery (requires special setup)
- `TestPromptDetection`: Shell prompt detection after command execution

### 2. `session_error_scenarios_test.go`
**Purpose**: Error handling and edge cases
**Coverage**:
- Duplicate session creation
- Missing session resumption
- Active session archiving
- Missing target messaging
- Corrupted manifest handling
- Missing required manifest fields
- Concurrent archive operations
- Invalid session names
- Permission denied scenarios
- Tmux server death handling
- Manifest version mismatches
- Race condition manifest updates
- Empty message sending
- Large message handling

**Key Tests**:
- `TestError_CreateDuplicateSession`: Duplicate detection
- `TestError_ResumeMissingSession`: Missing session errors
- `TestError_ArchiveActiveSession`: Active session archive warnings
- `TestError_CorruptedManifest`: YAML parsing error handling
- `TestError_MissingManifestField`: Incomplete manifest handling
- `TestError_ConcurrentArchive`: Concurrent operation safety
- `TestError_InvalidSessionName`: Input validation
- `TestError_PermissionDenied`: Permission error handling
- `TestError_ManifestVersionMismatch`: Version compatibility
- `TestError_RaceConditionManifestUpdate`: Concurrent update safety
- `TestError_SendEmptyMessage`: Empty input handling
- `TestError_MessageTooLarge`: Large input handling

### 3. `session_edge_cases_test.go`
**Purpose**: Edge cases and boundary conditions
**Coverage**:
- Empty session names
- Very long session names (300+ characters)
- Special characters (unicode, emoji, symbols)
- Existing directory conflicts
- Missing project directories
- Zero-byte manifests
- Future timestamps
- Sessions without tmux
- Multiple archive operations
- Symlinked project directories
- Read-only manifests
- Session directories without manifests
- Timestamp precision and timezone handling

**Key Tests**:
- `TestEdgeCase_EmptySessionName`: Empty name validation
- `TestEdgeCase_VeryLongSessionName`: Filesystem limit handling
- `TestEdgeCase_SpecialCharactersInName`: Character set validation
- `TestEdgeCase_SessionDirectoryAlreadyExists`: Directory conflict resolution
- `TestEdgeCase_ManifestWithoutProjectDirectory`: Missing path handling
- `TestEdgeCase_ZeroByteManifest`: Empty file handling
- `TestEdgeCase_ManifestWithFutureTimestamp`: Clock skew handling
- `TestEdgeCase_SessionWithNoTmuxSession`: Suspended state verification
- `TestEdgeCase_MultipleArchiveOperations`: Idempotency verification
- `TestEdgeCase_SessionWithSymlinkedProject`: Symlink support
- `TestEdgeCase_ReadOnlyManifest`: Permission boundary testing
- `TestEdgeCase_SessionDirWithNoManifest`: Invalid directory handling
- `TestEdgeCase_TimestampPrecision`: Timestamp serialization accuracy

### 4. `state_transitions_test.go`
**Purpose**: Session state transition validation
**Coverage**:
- Active to suspended transitions
- Suspended to active transitions
- Active/suspended to archived transitions
- State transition validation
- Invalid state transitions
- Timestamp updates during transitions

**Key Tests**:
- `TestStateTransition_ActiveToSuspended`: Tmux kill → suspended state
- `TestStateTransition_SuspendedToActive`: Resume → active state
- Additional transition tests (to be added)

### 5. `concurrent_operations_test.go`
**Purpose**: Concurrent operation safety and race condition handling
**Coverage**:
- Concurrent session creation
- Concurrent archiving
- Concurrent manifest updates
- Session ID uniqueness under load
- Race condition detection

**Key Tests**:
- `TestConcurrent_CreateMultipleSessions`: Parallel session creation
- `TestConcurrent_ArchiveMultipleSessions`: Parallel archiving
- Additional concurrent tests (to be added)

### 6. `comprehensive_session_lifecycle_test.go` (NEW)
**Purpose**: Complete lifecycle testing across all agent types
**Coverage**:
- Full lifecycle for Claude, Gemini, and GPT agents
- Create → Verify → Suspend → Resume → Archive → Verify workflow
- State transition validation
- Concurrent operations at scale
- Error handling parity across agents
- Cross-agent compatibility
- Metadata preservation
- Prompt detection

**Key Tests**:
- `TestSessionLifecycle_ComprehensiveCreateResumeTerminate`: Complete lifecycle for all agents
- `TestSessionStateTransitions`: All valid state transitions
- `TestConcurrentSessionOperations`: Large-scale concurrent operations (10+ sessions)
- `TestSessionErrorHandling_AgentParity`: Error handling across agent types
- `TestSessionEdgeCases_CrossAgent`: Cross-agent edge cases
- `TestSessionPromptDetection`: Shell prompt detection
- `TestSessionMetadataPreservation`: Metadata integrity

## Test Coverage Matrix

### Agent Coverage
| Test Category | Claude | Gemini | GPT | Notes |
|--------------|--------|--------|-----|-------|
| Session Creation | ✅ | ✅ | ✅ | All agents tested |
| Session Resume | ✅ | ⚠️ | ⚠️ | Requires agent setup |
| Session Archive | ✅ | ✅ | ✅ | Agent-agnostic |
| Error Handling | ✅ | ✅ | ✅ | Parity verified |
| Concurrent Ops | ✅ | ✅ | ✅ | All agents tested |

### Lifecycle States Covered
- ✅ New (empty lifecycle)
- ✅ Active (tmux session running)
- ✅ Suspended (tmux session dead, manifest intact)
- ✅ Archived (lifecycle = "archived")

### Operations Covered
- ✅ Create session
- ✅ List sessions
- ✅ Resume session
- ✅ Archive session
- ✅ Send message (A2A)
- ✅ Health check
- ✅ Metadata updates
- ⚠️ Session hooks (test exists, implementation pending)

### Error Scenarios Covered
- ✅ Duplicate creation
- ✅ Missing session
- ✅ Corrupted manifest
- ✅ Invalid names
- ✅ Permission denied
- ✅ Concurrent conflicts
- ✅ Version mismatches
- ⚠️ Disk full (requires special setup)
- ⚠️ Tmux server crash (requires isolation)

### Edge Cases Covered
- ✅ Empty names
- ✅ Long names (300+ chars)
- ✅ Special characters
- ✅ Unicode/emoji
- ✅ Symlinks
- ✅ Read-only files
- ✅ Missing directories
- ✅ Zero-byte files
- ✅ Future timestamps
- ✅ Idempotent operations

### Concurrent Operations Covered
- ✅ Concurrent creation (5-20 sessions)
- ✅ Concurrent archiving (5-15 sessions)
- ✅ Concurrent reads/writes
- ✅ Session ID uniqueness verification
- ✅ Race condition handling
- ✅ Manifest corruption prevention

## Running the Tests

### All Lifecycle Tests
```bash
cd main/agm
go test ./test/integration/lifecycle/... -v
```

### Specific Test File
```bash
go test ./test/integration/lifecycle/session_lifecycle_test.go -v
go test ./test/integration/lifecycle/comprehensive_session_lifecycle_test.go -v
```

### Short Mode (Skip Long-Running Tests)
```bash
go test ./test/integration/lifecycle/... -v -short
```

### With Race Detection
```bash
go test ./test/integration/lifecycle/... -v -race
```

### Specific Test
```bash
go test ./test/integration/lifecycle/... -v -run TestSessionLifecycle_ComprehensiveCreateResumeTerminate
```

## Test Requirements

### Environment
- **Tmux**: Required for most tests (skipped if unavailable)
- **Sessions Directory**: Temporary directory created by test env
- **Permissions**: Write access to test directories

### Agent Setup
- **Claude**: Default agent, no special setup
- **Gemini**: Requires `GEMINI_API_KEY` environment variable (tests use test key)
- **GPT**: Requires GPT configuration (tests may skip if not configured)

## Known Limitations and TODOs

### Pending Implementation
1. **Session Hooks**: Tests exist but hook execution not yet implemented
2. **Tmux Server Restart**: Requires isolated test environment
3. **Disk Full Simulation**: Needs quota/filesystem setup
4. **Cross-Platform Testing**: Linux-focused, needs Windows/macOS verification

### Test Gaps (Future Work)
1. **Long-Running Sessions**: Multi-hour session lifecycle
2. **Large-Scale Concurrent**: 100+ concurrent operations
3. **Network Failures**: Agent communication timeout handling
4. **Resource Exhaustion**: File descriptor/memory limits
5. **Migration Testing**: V1 → V2 → V3 manifest upgrades

## Test Quality Metrics

### Coverage Goals
- **Line Coverage**: Target 80%+ for session management code
- **Branch Coverage**: Target 75%+ for error paths
- **Concurrent Safety**: All operations tested under concurrent load

### Performance Benchmarks
- Session creation: < 100ms (without tmux)
- Session archiving: < 50ms
- Manifest read/write: < 10ms
- Concurrent operations: No deadlocks, graceful degradation

## Contributing

When adding new lifecycle tests:
1. Follow existing test structure and naming conventions
2. Test all agent types (Claude, Gemini, GPT) unless agent-specific
3. Include both success and failure scenarios
4. Add concurrent operation tests for thread-safe operations
5. Document test purpose and coverage in this file
6. Use `testing.Short()` for long-running tests
7. Clean up resources in `defer` statements

## References

- Session Management Code: `main/agm/internal/session/`
- Manifest Handling: `main/agm/internal/manifest/`
- Test Helpers: `main/agm/test/integration/helpers/`
- Astrocyte Daemon: `main/agm/astrocyte/`
