# Cross-Session Integration Tests (Task 2.4, bead oss-ji5p)

## Overview

This test suite validates the cross-session message delivery infrastructure for Phase 2 of the AI Coordination System. It tests state-based routing, message queueing, and delivery across multiple AGM sessions.

## Test Coverage

### 1. TestCrossSession_StateTransitions
Tests the READY→THINKING→READY state transition cycle across 3 test sessions.

**Validates:**
- State changes are persisted to manifest
- State transitions work correctly across multiple sessions
- Hook-based state detection updates manifest properly

### 2. TestCrossSession_MessageDelivery
Tests message queueing and automatic delivery when session becomes READY.

**Validates:**
- Messages queue correctly when target session is THINKING
- Messages deliver automatically when session transitions to READY
- Delivery latency <30s (acceptance criteria)

### 3. TestCrossSession_HookAccuracy
Measures false positive rate for hook-based state detection.

**Validates:**
- Hook-based state detection accuracy
- False positive rate <5% (acceptance criteria)
- State changes tracked correctly across multiple transitions

### 4. TestCrossSession_DeliveryLatency
Measures end-to-end latency from message queue to delivery.

**Validates:**
- Average delivery latency for READY sessions <5s
- Maximum delivery latency <30s (acceptance criteria)
- Latency statistics across multiple messages

### 5. TestCrossSession_DeliverySuccessRate
Tests overall message delivery reliability across sessions.

**Validates:**
- Delivery success rate >95% (acceptance criteria)
- Messages queue and deliver correctly across session state changes
- Queue persistence and delivery reliability

## Running Tests

### Standard Run
```bash
cd agm
go test -tags=integration ./test/integration -run TestCrossSession -v
```

### Skip Tests (for CI)
```bash
SKIP_E2E=1 go test -tags=integration ./test/integration -run TestCrossSession -v
```

### Run Specific Test
```bash
go test -tags=integration ./test/integration -run TestCrossSession_StateTransitions -v
```

### With Verbose Output
```bash
go test -tags=integration ./test/integration -run TestCrossSession -v -ginkgo.v
```

## Requirements

- tmux installed and accessible
- AGM tmux socket at `/tmp/agm.sock`
- Write access to `~/.config/agm/` for message queue database
- Sufficient permissions to create/kill tmux sessions

## Test Environment

Tests create temporary AGM sessions with the prefix `csm-test-cross-session-*`. All test sessions are automatically cleaned up after tests complete.

### Session Lifecycle
1. Create 3 AGM test sessions with manifests
2. Initialize message queue database
3. Run state transition and message delivery tests
4. Cleanup sessions and manifests on completion

### Preserved on Failure
If a test fails, sessions are preserved for debugging:
```bash
tmux attach -t csm-test-cross-session-1-<timestamp>
tmux attach -t csm-test-cross-session-2-<timestamp>
tmux attach -t csm-test-cross-session-3-<timestamp>
```

## Test Helpers

### createTestSession(name, workDir)
Creates a test AGM session with:
- Tmux session
- Manifest directory
- Manifest file with initial READY state

### simulateStateChange(session, state)
Triggers a state transition by updating the manifest:
- Updates state field
- Sets state_updated_at timestamp
- Sets state_source to "hook"

### measureDeliveryLatency(messageID, queuedAt)
Calculates time from message queue to delivery completion.

### deliverPendingMessages(queue, sessionName)
Simulates daemon delivery logic:
- Checks if session is READY
- Delivers all pending messages for session
- Marks messages as delivered in queue

## Acceptance Criteria

All tests must pass with the following metrics:

| Metric | Target | Test |
|--------|--------|------|
| False Positive Rate | <5% | TestCrossSession_HookAccuracy |
| Delivery Success Rate | >95% | TestCrossSession_DeliverySuccessRate |
| Delivery Latency | <30s | TestCrossSession_MessageDelivery |
| Max Delivery Latency | <30s | TestCrossSession_DeliveryLatency |
| Avg Delivery Latency | <5s | TestCrossSession_DeliveryLatency |

## Troubleshooting

### Tests Fail to Create Sessions
- Verify tmux is installed: `which tmux`
- Check AGM socket exists: `ls -l /tmp/agm.sock`
- Ensure permissions: `ls -ld ~/.config/agm/`

### Message Queue Errors
- Check database exists: `ls -l ~/.config/agm/message_queue.db`
- Verify write permissions
- Check for locked database (close other AGM processes)

### State Transitions Fail
- Verify manifest directory structure
- Check manifest file permissions
- Ensure YAML is valid

## Implementation Details

**File**: `test/integration/cross_session_test.go`

**Dependencies:**
- `internal/daemon` - Daemon delivery logic
- `internal/manifest` - Session manifest read/write
- `internal/messages` - Message queue operations
- `test/integration/helpers` - Test utilities

**Database**: `~/.config/agm/message_queue.db` (SQLite with WAL mode)

## Related Documentation

- ROADMAP.md - Phase 2 Task 2.4 specification
- ADR-006 - Message Queue Architecture
- internal/daemon/daemon.go - Delivery daemon implementation
- internal/messages/queue.go - Message queue API
