# Autonomous Swarm - Specification

## Document Metadata

- **Version**: 1.0.0
- **Status**: Active
- **Last Updated**: 2026-02-11
- **Authors**: Autonomous Swarm Development Team

## 1. Overview

### 1.1 Purpose

Autonomous Swarm is a Go-based execution harness that orchestrates autonomous agent tasks ("beads") with priority-based queuing, dependency management, and AGM (Agent Session Manager) integration. The system enables distributed execution of AI agent tasks with built-in validation, telemetry, and escalation handling.

### 1.2 Scope

This specification covers:
- Task queue management and state transitions
- AGM session orchestration for agent execution
- Execution lifecycle with iteration limits and escalation detection
- S8/S9 validation phases
- Telemetry and roadmap generation
- Dependency graph resolution for parallel execution

### 1.3 Goals

1. **Autonomous Execution**: Enable AI agents to execute tasks without human intervention
2. **Reliability**: Handle failures gracefully with automatic retries and escalation
3. **Observability**: Provide comprehensive logging and progress tracking
4. **Scalability**: Support dependency-aware parallel execution
5. **Safety**: Validate work products and detect when human input is required

### 1.4 Non-Goals

- Real-time streaming execution monitoring (future enhancement)
- Multi-machine distributed execution (single-host only in v1)
- Built-in UI/dashboard (CLI-first design)

## 2. System Architecture

### 2.1 Core Components

#### 2.1.1 Task Queue (pkg/taskqueue)

**Responsibilities**:
- YAML-based queue persistence (TASK-QUEUE.yaml)
- Thread-safe state management with RWMutex
- Bead lifecycle state transitions: ready → in_progress → completed/blocked
- Dependency validation before bead claiming
- Automatic dependency resolution and unblocking

**Key Types**:
```go
type TaskQueue struct {
    SchemaVersion string
    LastUpdated   time.Time
    Ready         []Bead
    InProgress    []Bead
    Blocked       []Bead
    Completed     []Bead
}

type Bead struct {
    ID        string
    Tier      int          // 1-4 priority
    Title     string
    Phase     string
    DependsOn []string
    Prompts   BeadPrompts
    Metadata  BeadMetadata
}
```

**Operations**:
- `Load()`: Read queue from disk
- `Save()`: Atomic write (temp + rename pattern)
- `Claim(beadID, sessionName)`: Move bead to in_progress with validation
- `Complete(beadID)`: Move bead to completed and unblock dependents
- `QueryNext()`: Get first ready bead (O(n) scan acceptable for v1)
- `Unblock()`: Check blocked beads and move to ready if dependencies met

#### 2.1.2 AGM Orchestrator (pkg/csm)

**Responsibilities**:
- AGM session lifecycle management
- tmux session health monitoring
- Session UUID extraction for result tracking

**Operations**:
- `Create(sessionName)`: Spawn new AGM session
- `Monitor(sessionName)`: Check session health (tmux + AGM list)
- `Extract(sessionName)`: Retrieve session UUID
- `Archive(sessionName)`: Clean up session resources
- `WaitForSession(sessionName, timeout)`: Poll until session ready

**Integration Points**:
- Uses `csm` CLI binary (must be in PATH)
- Leverages tmux for session management
- Returns structured session metadata

#### 2.1.3 Execution Harness (pkg/executor)

**Responsibilities**:
- Orchestrate full bead execution lifecycle
- Iteration tracking (max 3 attempts)
- Escalation signal detection
- Error classification (recoverable vs fatal vs escalation)

**Execution Flow**:
```
1. Claim bead from queue
2. Create AGM session
3. Wait for session ready
4. Inject bead prompt
5. Monitor session health (placeholder)
6. Extract results (session UUID)
7. Archive session
8. Validate results (S8/S9 phases)
9. Complete bead or escalate
```

**Error Handling**:
- **Recoverable**: AGM timeout, parse errors → retry (increment iteration)
- **Escalation**: Max iterations exceeded, explicit ESCALATE signal → move to blocked
- **Fatal**: File not found, invalid config → fail immediately

#### 2.1.4 Validation Engine (pkg/validation)

**S8 Validation** (Implementation File Validation):
- Check file existence
- Syntax validation:
  - `.go` files: gofmt -e
  - `.yaml/.yml` files: yaml.Unmarshal
- Returns structured validation result with errors

**S9 Validation** (Test Execution):
- Run `go test -cover` on specified package
- Parse test results and coverage percentage
- Validate against configurable coverage threshold (default 80%)
- Returns pass/fail with detailed failure messages

#### 2.1.5 Telemetry (pkg/telemetry)

**Execution Logging** (EXECUTION-LOG.jsonl):
- Append-only JSON Lines format
- Event types: execute, complete, error, escalate
- Timestamped with bead ID and contextual details

**Roadmap Generation** (ROADMAP.md):
- Human-readable progress summary
- Section counts and selected items (max 1500 tokens)
- Progress percentage calculation
- Generated after each execution

#### 2.1.6 Launcher (pkg/launcher)

**Responsibilities**:
- Dependency graph construction from ready beads
- Topological sort using Kahn's algorithm
- Orchestrate parallel bead launching in dependency order
- Detect circular dependencies

**Key Operations**:
- `BuildGraph(beads)`: Construct DAG from bead dependencies
- `TopologicalSort()`: Determine launch order
- `LaunchReady()`: Launch all ready beads respecting dependencies

### 2.2 Data Flow

```
TASK-QUEUE.yaml
    ↓ (Load)
Coordinator (in-memory queue)
    ↓ (Claim)
Execution Harness
    ↓ (Create session)
AGM Orchestrator
    ↓ (Execute)
Agent Session (tmux + Claude)
    ↓ (Results)
Validation Engine (S8/S9)
    ↓ (Complete/Escalate)
Coordinator (update state)
    ↓ (Save)
TASK-QUEUE.yaml + EXECUTION-LOG.jsonl + ROADMAP.md
```

## 3. Functional Requirements

### 3.1 Task Queue Management

**FR-TQ-001**: System SHALL persist task queue state in YAML format
**FR-TQ-002**: System SHALL support four queue sections: ready, in_progress, blocked, completed
**FR-TQ-003**: System SHALL validate dependencies before claiming beads
**FR-TQ-004**: System SHALL use atomic writes (temp + rename) for queue persistence
**FR-TQ-005**: System SHALL automatically unblock beads when dependencies complete

### 3.2 Bead Execution

**FR-EX-001**: System SHALL execute beads through AGM sessions
**FR-EX-002**: System SHALL retry beads up to 3 times on recoverable errors
**FR-EX-003**: System SHALL detect explicit escalation signals ("ESCALATE:" keyword)
**FR-EX-004**: System SHALL track iteration count in bead metadata
**FR-EX-005**: System SHALL move escalated beads to blocked section

### 3.3 Validation

**FR-VAL-001**: System SHALL validate file existence and syntax in S8 phase
**FR-VAL-002**: System SHALL execute tests and check coverage in S9 phase
**FR-VAL-003**: System SHALL support configurable coverage thresholds
**FR-VAL-004**: System SHALL return structured validation results

### 3.4 Telemetry

**FR-TEL-001**: System SHALL log execution events in JSON Lines format
**FR-TEL-002**: System SHALL generate human-readable roadmap after each execution
**FR-TEL-003**: System SHALL timestamp all events with RFC3339 format
**FR-TEL-004**: System SHALL include bead ID and session name in all events

### 3.5 Dependency Management

**FR-DEP-001**: System SHALL resolve dependencies using topological sort
**FR-DEP-002**: System SHALL detect circular dependencies
**FR-DEP-003**: System SHALL launch beads in dependency order
**FR-DEP-004**: System SHALL prevent claiming beads with incomplete dependencies

## 4. Non-Functional Requirements

### 4.1 Performance

**NFR-PERF-001**: Queue load/save operations SHALL complete in < 1 second for queues with < 1000 beads
**NFR-PERF-002**: Dependency graph sort SHALL use O(V+E) algorithm (Kahn's)
**NFR-PERF-003**: Session health checks SHALL timeout after 5 seconds

### 4.2 Reliability

**NFR-REL-001**: System SHALL use atomic writes to prevent queue corruption
**NFR-REL-002**: System SHALL use RWMutex for thread-safe queue access
**NFR-REL-003**: System SHALL clean up sessions on failure (archive)

### 4.3 Observability

**NFR-OBS-001**: All errors SHALL include contextual information (bead ID, session)
**NFR-OBS-002**: Execution log SHALL be append-only and parseable
**NFR-OBS-003**: Roadmap SHALL update within 1 second of execution completion

### 4.4 Usability

**NFR-USE-001**: CLI SHALL provide clear usage documentation (--help)
**NFR-USE-002**: Exit codes SHALL follow Unix conventions (0=success, 1=error, 2=escalation)
**NFR-USE-003**: Error messages SHALL be actionable and include remediation hints

## 5. Data Specifications

### 5.1 TASK-QUEUE.yaml Schema

```yaml
schema_version: "1.0.0"          # Semantic version
last_updated: 2024-01-01T00:00:00Z  # RFC3339 timestamp
ready:
  - id: string                   # Unique bead identifier
    tier: int                    # Priority tier (1-4)
    title: string                # Human-readable title
    phase: string                # Optional phase label
    depends_on: [string]         # List of bead IDs
    prompts:
      start: string              # Initial prompt
      verify_s8: string          # S8 validation prompt
      verify_s9: string          # S9 validation prompt
      done: string               # Completion prompt
    metadata:
      session_name: string       # Assigned session
      iterations: int            # Attempt count
      last_attempt: timestamp    # Last execution time
      escalation_reason: string  # Reason for escalation
in_progress: [bead]
blocked: [bead]
completed: [bead]
```

### 5.2 EXECUTION-LOG.jsonl Format

```json
{
  "timestamp": "2024-01-01T12:00:00Z",
  "bead_id": "bead-1",
  "event": "execute|complete|error|escalate",
  "details": {
    "session": "session-1",
    "action": "start|finish",
    "message": "error description",
    "iteration": 1
  }
}
```

### 5.3 Exit Codes

- `0`: Success - bead executed and completed successfully
- `1`: Error - execution failed (see stderr for details)
- `2`: Escalation - bead requires human intervention

## 6. Configuration

### 6.1 Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| SWARM_QUEUE_FILE | TASK-QUEUE.yaml | Path to queue file |
| SWARM_LOG_FILE | EXECUTION-LOG.jsonl | Path to event log |
| SWARM_ROADMAP_FILE | ROADMAP.md | Path to roadmap |
| SWARM_MAX_ITERATIONS | 3 | Max retry attempts |
| SWARM_SESSION_TIMEOUT | 1h | Session timeout duration |
| SWARM_HEARTBEAT_INTERVAL | 5m | Health check interval |
| SWARM_TEST_COVERAGE_THRESHOLD | 0.80 | S9 coverage threshold |

### 6.2 CLI Flags

```
--queue <path>    Path to TASK-QUEUE.yaml (required)
--bead-id <id>    Bead ID to execute (required)
--session <name>  AGM session name (required)
--version         Show version and exit
--help            Show help and exit
```

## 7. Integration Points

### 7.1 AGM Integration

**Requirements**:
- AGM binary (`csm`) must be in PATH
- Supported operations: new, list, get-uuid, archive
- JSON output format for `list` command

**Session Lifecycle**:
1. Create: `csm new <session-name>`
2. Monitor: `csm list --json` + `tmux has-session`
3. Extract: `csm get-uuid <session-name>`
4. Archive: `csm archive <session-name>`

### 7.2 tmux Integration

**Requirements**:
- tmux server must be running
- Sessions named by swarm-executor
- Health checks via `tmux has-session -t <name>`

## 8. Testing Strategy

### 8.1 Unit Tests

**Coverage Targets**:
- pkg/taskqueue: 90%+ (core state management)
- pkg/csm: 65%+ (integration-heavy)
- pkg/executor: 50%+ (orchestration layer)
- pkg/validation: 90%+ (pure logic)
- pkg/telemetry: 90%+ (file I/O)
- pkg/launcher: 85%+ (graph algorithms)

### 8.2 Test Categories

1. **State Management**: Queue operations, concurrency
2. **Dependency Resolution**: Graph construction, topological sort, cycle detection
3. **Error Classification**: Recoverable vs fatal vs escalation
4. **Validation**: S8 syntax checks, S9 test execution
5. **Telemetry**: JSON Lines format, roadmap generation

### 8.3 Integration Testing

- End-to-end bead execution (requires AGM + tmux)
- Queue persistence across restarts
- Dependency chain execution

## 9. Security Considerations

### 9.1 File Access

- Queue files use 0644 permissions (user read/write, group/other read)
- Log files use append-only writes
- Atomic writes prevent partial corruption

### 9.2 Session Isolation

- AGM sessions isolated via tmux
- Session names prevent conflicts
- Archive cleanup prevents resource leaks

### 9.3 Input Validation

- Bead IDs validated before execution
- Session names checked for non-empty
- File paths validated for existence

## 10. Future Enhancements

### 10.1 Planned (Not in Scope)

1. **Real-time Monitoring**: WebSocket-based session output streaming
2. **Multi-host Execution**: Distributed queue with coordination
3. **Advanced Scheduling**: Time-based triggers, priority preemption
4. **Result Storage**: Structured artifact storage beyond session UUID
5. **Heartbeat Monitoring**: Active session health checks (Phase 0B)
6. **Web Dashboard**: UI for queue visualization and management

### 10.2 Potential Extensions

- Plugin system for custom validation phases
- Webhooks for event notifications
- Metrics export (Prometheus format)
- Rate limiting for agent spawning

## 11. Glossary

- **Bead**: Autonomous task unit with defined prompts and dependencies
- **AGM**: Agent Session Manager - CLI tool for managing Claude agent sessions
- **Escalation**: Condition requiring human intervention (max retries or explicit signal)
- **S8**: Implementation file validation phase (syntax checks)
- **S9**: Test execution and coverage validation phase
- **Tier**: Priority level (1=critical, 4=nice-to-have)
- **DAG**: Directed Acyclic Graph for dependency resolution
- **Topological Sort**: Ordering algorithm for dependency graphs

## 12. References

- [README.md](README.md) - User documentation
- [ARCHITECTURE.md](ARCHITECTURE.md) - Detailed architecture design
- [ADR-001-dependency-graph.md](docs/adr/ADR-001-dependency-graph.md) - Dependency management decision
- [ADR-002-atomic-writes.md](docs/adr/ADR-002-atomic-writes.md) - Queue persistence decision

## 13. Revision History

| Version | Date | Changes | Author |
|---------|------|---------|--------|
| 1.0.0 | 2026-02-11 | Initial specification | Backfill Documentation |

## 14. Approval

This specification represents the implemented behavior of Autonomous Swarm v0.1.0 and serves as the authoritative reference for system capabilities and constraints.
