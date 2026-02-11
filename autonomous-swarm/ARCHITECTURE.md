# Autonomous Swarm - Architecture

## Document Metadata

- **Version**: 1.0.0
- **Status**: Active
- **Last Updated**: 2026-02-11
- **Related**: [SPEC.md](SPEC.md), [README.md](README.md)

## 1. System Overview

### 1.1 Architecture Vision

Autonomous Swarm is designed as a **lightweight, file-based task orchestration system** for autonomous AI agents. The architecture emphasizes simplicity, observability, and reliability through:

- **File-based state**: YAML queue persistence (no database required)
- **Process isolation**: CSM sessions in separate tmux instances
- **Append-only logging**: JSON Lines for auditability
- **Dependency awareness**: DAG-based execution ordering

### 1.2 Design Principles

1. **Crash-only Design**: No graceful shutdown required - all state in files
2. **Single Responsibility**: Each package has one clear purpose
3. **Test-Driven**: High coverage for core logic packages
4. **External Dependencies**: Minimal (only YAML parser + stdlib)
5. **Observable**: Every state change logged and persisted

### 1.3 High-Level Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                     CLI Entry Point                              │
│                  cmd/swarm-executor                              │
│                                                                   │
│  • Flag parsing                                                  │
│  • Telemetry initialization                                      │
│  • Orchestration coordination                                    │
│  • Exit code mapping                                             │
└─────────────────────┬───────────────────────────────────────────┘
                      │
                      ▼
┌─────────────────────────────────────────────────────────────────┐
│                   Core Orchestration Layer                       │
│                    pkg/executor/Harness                          │
│                                                                   │
│  • Execution lifecycle management                               │
│  • Iteration tracking                                            │
│  • Error classification                                          │
│  • State transition coordination                                │
└───────┬──────────────────────┬─────────────────────┬────────────┘
        │                      │                     │
        ▼                      ▼                     ▼
┌──────────────┐     ┌──────────────┐     ┌──────────────┐
│ Task Queue   │     │ CSM Session  │     │ Validation   │
│ Management   │     │ Orchestrator │     │ Engine       │
│              │     │              │     │              │
│ taskqueue.   │     │ csm.         │     │ validation.  │
│ Coordinator  │     │ Orchestrator │     │ ValidateS8   │
│              │     │              │     │ ValidateS9   │
│ • Load/Save  │     │ • Create     │     │              │
│ • Claim      │     │ • Monitor    │     │ • File check │
│ • Complete   │     │ • Extract    │     │ • Syntax     │
│ • Unblock    │     │ • Archive    │     │ • Test exec  │
│              │     │              │     │ • Coverage   │
└──────┬───────┘     └──────┬───────┘     └──────────────┘
       │                    │
       │                    ▼
       │           ┌──────────────┐
       │           │ External     │
       │           │ Dependencies │
       │           │              │
       │           │ • csm CLI    │
       │           │ • tmux       │
       │           │ • gofmt      │
       │           └──────────────┘
       │
       ▼
┌─────────────────────────────────────────┐
│         Telemetry & Logging              │
│         pkg/telemetry                    │
│                                          │
│  • Logger (JSON Lines)                   │
│  • RoadmapGenerator                      │
│  • Event types & formatting              │
└─────────────────────────────────────────┘
       │
       ▼
┌─────────────────────────────────────────┐
│         Persistent State                 │
│                                          │
│  • TASK-QUEUE.yaml (queue state)        │
│  • EXECUTION-LOG.jsonl (event log)       │
│  • ROADMAP.md (human summary)            │
└─────────────────────────────────────────┘
```

## 2. Package Architecture

### 2.1 Package Dependency Graph

```
cmd/swarm-executor
    ├── pkg/executor
    │   ├── pkg/taskqueue
    │   ├── pkg/csm
    │   └── pkg/validation
    ├── pkg/telemetry
    │   └── pkg/taskqueue (for roadmap)
    └── internal/config

pkg/launcher
    └── pkg/taskqueue

pkg/agm (Alternative orchestration - future)
    └── pkg/taskqueue
```

**Design Note**: No circular dependencies. All dependencies flow downward toward core types.

### 2.2 Package Descriptions

#### 2.2.1 cmd/swarm-executor

**Purpose**: CLI entry point and orchestration glue code

**Responsibilities**:
- Parse command-line flags
- Initialize telemetry (logger, roadmap)
- Wire up coordinator, orchestrator, harness
- Map execution results to exit codes
- Handle top-level errors

**Key Functions**:
```go
main()                      // Entry point
executeBead()               // Main execution logic
printUsage()                // Help text
logError()                  // Error logging helper
```

**Design Rationale**: Thin layer - most logic delegated to packages. Only 1.9% test coverage acceptable since it's pure orchestration.

#### 2.2.2 pkg/taskqueue

**Purpose**: Thread-safe task queue state management

**Components**:

1. **types.go**: Core data structures
   - `TaskQueue`: Four-section queue (ready/in_progress/blocked/completed)
   - `Bead`: Task unit with metadata
   - `BeadPrompts`: Stage-based prompts
   - `BeadMetadata`: Execution tracking

2. **coordinator.go**: State management
   - `Coordinator`: Thread-safe queue operations
   - RWMutex for concurrent access
   - Atomic writes (temp + rename pattern)
   - Operations: Load, Save, Claim, Complete, Unblock, QueryNext

3. **parser.go**: YAML serialization
   - `ParseTaskQueue()`: Read YAML from disk
   - Schema validation
   - Error reporting

**Concurrency Model**:
```go
type Coordinator struct {
    filePath string
    queue    *TaskQueue
    mu       sync.RWMutex  // Read-write mutex
}

// Pattern:
// - Read operations: RLock/RUnlock
// - Write operations: Lock/Unlock
// - Load: Lock (mutates internal state)
// - Save: RLock (reads internal state)
```

**State Transitions**:
```
         ┌──────────────┐
         │    Ready     │
         └──────┬───────┘
                │ Claim()
                ▼
         ┌──────────────┐
         │ In Progress  │
         └──┬───────┬───┘
            │       │ Complete()
            │       ▼
            │ ┌──────────────┐
            │ │  Completed   │
            │ └──────────────┘
            │
            │ (error + max retries)
            ▼
         ┌──────────────┐
         │   Blocked    │
         └──────┬───────┘
                │ Unblock() (when deps complete)
                ▼
         ┌──────────────┐
         │    Ready     │
         └──────────────┘
```

**Performance Characteristics**:
- Load: O(n) parse + O(1) assignment
- Save: O(n) marshal + O(1) atomic write
- Claim: O(n) linear search + O(d) dependency validation
- Complete: O(n) search + O(b*d) unblocking check
- QueryNext: O(1) first element access

**Design Rationale**:
- Simple O(n) operations acceptable for v1 (< 1000 beads)
- Atomic writes prevent corruption on crash
- Dependency validation at claim-time prevents invalid execution

#### 2.2.3 pkg/csm

**Purpose**: CSM session lifecycle management

**Components**:

1. **orchestrator.go**: Session operations
   - `Create()`: Spawn CSM session
   - `Monitor()`: Health check (CSM list + tmux)
   - `Extract()`: Get session UUID
   - `Archive()`: Clean up session
   - `WaitForSession()`: Polling with timeout

2. **prompter.go**: Prompt injection (future)
   - `InjectPrompt()`: Send prompt to session

3. **types.go**: Session metadata
   - `SessionState`: active/stopped/archived
   - `SessionMetadata`: Name, UUID, timestamps

**External Dependencies**:
```bash
# Required CLI tools
csm new <session-name>           # Create session
csm list --json                  # List sessions
csm get-uuid <session-name>      # Extract UUID
csm archive <session-name>       # Clean up

tmux has-session -t <name>       # Health check
```

**Health Check Strategy**:
```go
func (o *Orchestrator) Monitor(sessionName string) (bool, error) {
    // 1. Check CSM session exists
    sessions := exec("csm list --json")
    if !contains(sessions, sessionName) {
        return false, nil
    }

    // 2. Check tmux session alive
    err := exec("tmux has-session -t", sessionName)
    return err == nil, nil
}
```

**Error Handling**:
- Create failure: Return error immediately (fatal)
- Monitor failure: Return false (recoverable if retry)
- Extract failure: Return error (fatal - cannot get results)
- Archive failure: Return error (resource leak warning)

**Design Rationale**:
- Thin wrapper around CSM CLI (no reimplementation)
- Health checks use both CSM and tmux for redundancy
- Polling-based wait (acceptable for v1, future: inotify)

#### 2.2.4 pkg/executor

**Purpose**: Execution lifecycle orchestration and error classification

**Components**:

1. **harness.go**: Main execution loop
   - `ExecuteBead()`: Full lifecycle (claim → execute → validate → complete)
   - Coordinates taskqueue, CSM, validation
   - Handles cleanup on errors

2. **iteration.go**: Retry logic
   - Track iteration count
   - Increment on recoverable errors
   - Escalate at max iterations

3. **escalation.go**: Escalation detection
   - `EscalationDetector`: Scans output for "ESCALATE:" keyword
   - `DetectEscalation()`: Parse reason from output
   - `CreateEscalationError()`: Generate structured error

4. **errors.go**: Error classification
   - `ExecutionError`: Typed error with ErrorType
   - Types: Recoverable, Escalation, Fatal
   - `IsEscalationError()`: Type checking

**Execution Flow**:
```
ExecuteBead(beadID, sessionName)
  │
  ├─1─► Claim(beadID, sessionName)
  │       └─► Validate dependencies
  │       └─► Move to in_progress
  │       └─► Save queue
  │
  ├─2─► Create(sessionName)
  │       └─► csm new
  │
  ├─3─► WaitForSession(5s timeout)
  │       └─► Poll Monitor() until ready
  │
  ├─4─► InjectPrompt(sessionName, beadID)
  │       └─► Send prompt to session
  │
  ├─5─► [Monitor health - Phase 0B placeholder]
  │
  ├─6─► Extract(sessionName)
  │       └─► csm get-uuid → UUID
  │
  ├─7─► Archive(sessionName)
  │       └─► csm archive
  │
  ├─8─► [Validate - S8/S9 placeholder]
  │
  └─9─► Complete(beadID)
          └─► Move to completed
          └─► Unblock dependents
          └─► Save queue
```

**Error Classification Logic**:
```go
type ErrorType int

const (
    ErrorRecoverable ErrorType = iota  // Retry
    ErrorEscalation                    // Move to blocked
    ErrorFatal                         // Stop immediately
)

// Decision tree:
if err == nil {
    return nil
} else if iterations >= maxIterations {
    return EscalationError("max iterations")
} else if strings.Contains(output, "ESCALATE:") {
    return EscalationError(extractReason(output))
} else if isTemporary(err) {
    iterations++
    return retry()
} else {
    return FatalError(err)
}
```

**Design Rationale**:
- Single-method execution API for simplicity
- Cleanup always attempted (defer-like logic)
- Escalation as typed error for clear handling
- Iteration tracking in metadata (persisted across runs)

#### 2.2.5 pkg/validation

**Purpose**: S8 (file validation) and S9 (test execution) validation phases

**Components**:

1. **s8.go**: Implementation file validation
   - `ValidateS8(filePaths)`: Check existence and syntax
   - File type detection by extension
   - Syntax validators:
     - `.go`: gofmt -e
     - `.yaml/.yml`: yaml.Unmarshal
   - Returns: S8ValidationResult with errors

2. **s9.go**: Test execution and coverage
   - `ValidateS9(packagePath, threshold)`: Run tests
   - Execute: go test -cover
   - Parse coverage percentage from output
   - Extract test failures from output
   - Returns: S9ValidationResult with coverage metrics

**Validation Result Structures**:
```go
type S8ValidationResult struct {
    Valid  bool       // Overall pass/fail
    Errors []string   // Error messages
    Files  []string   // Files validated
}

type S9ValidationResult struct {
    Valid             bool
    TestsPassed       bool
    CoverageMet       bool
    Coverage          float64
    CoverageThreshold float64
    Failures          []string
    TestOutput        string
}
```

**S9 Coverage Parsing**:
```go
// Example output: "coverage: 85.3% of statements"
re := regexp.MustCompile(`coverage:\s+([\d.]+)%`)
coverage := extractFloat(re.FindStringSubmatch(output))
```

**Design Rationale**:
- External tools for validation (gofmt, go test) - don't reinvent
- Structured results for programmatic use
- Configurable thresholds via environment variables
- Future: Plugin architecture for custom validators

#### 2.2.6 pkg/telemetry

**Purpose**: Event logging and progress reporting

**Components**:

1. **logger.go**: JSON Lines event logger
   - `Logger`: Append-only file writer
   - `LogEvent(event)`: Write single event
   - Automatic timestamp injection

2. **roadmap.go**: Human-readable roadmap generator
   - `GenerateRoadmap(queuePath, outputPath)`: Create summary
   - Section counts (ready/in_progress/blocked/completed)
   - Progress percentage
   - Max 1500 tokens (approximation for readability)

3. **types.go**: Event structures
   - `ExecutionEvent`: Timestamp, BeadID, Event, Details
   - `RoadmapEntry`: Section summary

**Event Schema**:
```json
{
  "timestamp": "2024-01-01T12:00:00Z",  // RFC3339
  "bead_id": "bead-1",
  "event": "execute",                    // execute|complete|error|escalate
  "details": {                           // Flexible map
    "session": "session-1",
    "action": "start",
    "iteration": 1
  }
}
```

**Roadmap Format**:
```markdown
# Task Roadmap

**Last Updated**: 2024-01-01 12:05:00

## Ready
**Count**: 5 beads ready for execution

## In Progress
- **bead-2**: Second bead (session: my-session, iteration: 1)

## Blocked
*No blocked beads*

## Completed
**Count**: 1 beads completed

---
**Progress**: 1/6 (17%) completed
```

**Design Rationale**:
- JSON Lines for streaming and easy parsing (jq compatible)
- Append-only for auditability (no mutations)
- Roadmap regenerated after each execution (stateless)
- Human-readable roadmap for quick status checks

#### 2.2.7 pkg/launcher

**Purpose**: Dependency-aware parallel bead launching

**Components**:

1. **graph.go**: Dependency graph and topological sort
   - `DependencyGraph`: DAG representation
   - `BuildGraph(beads)`: Construct from bead list
   - `TopologicalSort()`: Kahn's algorithm implementation
   - Cycle detection

2. **orchestrator.go**: Launch coordination
   - `LaunchReady()`: Launch all ready beads
   - Dependency order enforcement
   - Partial failure handling

**Graph Algorithm**:
```go
// Kahn's Algorithm (O(V+E) time complexity)
func TopologicalSort() ([]string, error) {
    // 1. Initialize queue with zero-degree nodes
    queue := findZeroDegree(inDegree)

    // 2. Process queue
    for len(queue) > 0 {
        node := queue.dequeue()
        sorted.append(node)

        // Reduce in-degree for dependents
        for dependent in adjacency[node] {
            inDegree[dependent]--
            if inDegree[dependent] == 0 {
                queue.enqueue(dependent)
            }
        }
    }

    // 3. Check for cycles
    if len(sorted) < len(nodes) {
        return error("circular dependency")
    }

    return sorted
}
```

**Launch Strategy**:
```
LaunchReady()
  │
  ├─1─► BuildGraph(queue.Ready)
  │       └─► Construct DAG
  │       └─► Detect cycles
  │
  ├─2─► TopologicalSort()
  │       └─► Determine order
  │
  └─3─► for each bead in order:
          └─► Claim(beadID, generateSessionName(beadID))
          └─► [Spawn agent process - future]
```

**Design Rationale**:
- Topological sort ensures dependencies execute first
- Cycle detection prevents infinite waiting
- O(V+E) complexity acceptable for hundreds of beads
- Future: Actual process spawning for parallel execution

#### 2.2.8 internal/config

**Purpose**: Configuration management with environment variable support

**Components**:

1. **config.go**: Configuration types and loaders
   - `Config`: All configuration fields
   - `DefaultConfig()`: Sensible defaults
   - `LoadFromEnv()`: Overlay environment variables
   - `Validate()`: Constraint checking

**Configuration Fields**:
```go
type Config struct {
    QueueFilePath         string        // TASK-QUEUE.yaml
    LogFilePath           string        // EXECUTION-LOG.jsonl
    RoadmapFilePath       string        // ROADMAP.md
    MaxIterations         int           // Default: 3
    SessionTimeout        time.Duration // Default: 1h
    HeartbeatInterval     time.Duration // Default: 5m
    TestCoverageThreshold float64       // Default: 0.80
}
```

**Environment Variable Mapping**:
- SWARM_QUEUE_FILE → QueueFilePath
- SWARM_LOG_FILE → LogFilePath
- SWARM_ROADMAP_FILE → RoadmapFilePath
- SWARM_MAX_ITERATIONS → MaxIterations
- SWARM_SESSION_TIMEOUT → SessionTimeout
- SWARM_HEARTBEAT_INTERVAL → HeartbeatInterval
- SWARM_TEST_COVERAGE_THRESHOLD → TestCoverageThreshold

**Design Rationale**:
- Defaults allow zero-config usage
- Environment variables for 12-factor app compliance
- Validation prevents invalid configurations
- Not currently used in CLI (future: config file support)

## 3. Data Architecture

### 3.1 State Machine

```
Bead Lifecycle State Machine:

    ┌─────────┐
    │  Ready  │ ◄────────────┐
    └────┬────┘              │
         │                   │
         │ Claim()           │ Unblock()
         │                   │
         ▼                   │
  ┌─────────────┐            │
  │ In Progress │            │
  └──────┬──────┘            │
         │                   │
         ├──► Complete() ────┼───► ┌───────────┐
         │                         │ Completed │
         │                         └───────────┘
         │
         └──► Escalate() ────────► ┌─────────┐
                                    │ Blocked │
                                    └────┬────┘
                                         │
                                         └─────────┘

Transitions:
- Ready → In Progress: Claim() validates dependencies
- In Progress → Completed: Complete() unblocks dependents
- In Progress → Blocked: Escalate() on max retries or ESCALATE signal
- Blocked → Ready: Unblock() when dependencies complete
```

### 3.2 File Persistence Strategy

**TASK-QUEUE.yaml** (Mutable State):
- Format: YAML for human readability
- Persistence: Atomic write (temp + rename)
- Concurrency: RWMutex in coordinator
- Backup: None (future: versioned snapshots)

**EXECUTION-LOG.jsonl** (Immutable Log):
- Format: JSON Lines for streaming parsers
- Persistence: Append-only writes
- Concurrency: File-level locking via OS
- Rotation: None (future: date-based rotation)

**ROADMAP.md** (Derived State):
- Format: Markdown for human consumption
- Persistence: Overwrite on each generation
- Source of truth: TASK-QUEUE.yaml (can regenerate)
- Purpose: Quick status glance

### 3.3 Concurrency Model

**Thread Safety**:
- Coordinator: RWMutex for queue access
- Logger: OS-level file locking (append mode)
- CSM Orchestrator: Stateless (exec.Command is thread-safe)

**Reentrancy**:
- Same bead cannot be claimed twice (removed from ready on claim)
- Session names must be unique per execution
- Atomic queue saves prevent lost updates

**Race Conditions**:
- Queue: Protected by mutex
- Log: Append-only (no read-modify-write)
- Roadmap: Single-writer (regenerated after save)

## 4. Integration Architecture

### 4.1 CSM Integration

**Interface Contract**:
```bash
# Required operations
csm new <session-name>          → Exit 0 on success
csm list --json                 → JSON array of sessions
csm get-uuid <session-name>     → UUID string on stdout
csm archive <session-name>      → Exit 0 on success
```

**JSON Schema** (csm list --json):
```json
[
  {
    "name": "session-1",
    "uuid": "550e8400-e29b-41d4-a716-446655440000",
    "state": "active",
    "created_at": "2024-01-01T12:00:00Z"
  }
]
```

**Error Handling**:
- Non-zero exit → Capture combined output for error message
- JSON parse error → Return structured error
- Missing fields → Use empty values with warning

### 4.2 tmux Integration

**Health Check Pattern**:
```bash
tmux has-session -t <session-name>
# Exit 0: Session exists
# Exit 1: Session not found
```

**Assumptions**:
- tmux server is running (prerequisite)
- Session names match CSM session names
- No manual session manipulation (don't detach CSM sessions)

## 5. Testing Architecture

### 5.1 Test Strategy by Package

**pkg/taskqueue (91.4% coverage)**:
- Unit: State transitions, dependency validation
- Concurrency: Parallel claims, race detector
- Persistence: Atomic write verification

**pkg/csm (67.5% coverage)**:
- Unit: Command construction, JSON parsing
- Integration: Mock csm/tmux (test fixtures)
- Error paths: Command failures, timeouts

**pkg/executor (53.8% coverage)**:
- Unit: Error classification, iteration tracking
- Integration: Full execution with mock CSM
- Edge cases: Max retries, escalation signals

**pkg/validation (91.7% coverage)**:
- Unit: Syntax validators, coverage parsing
- File I/O: Temp files for test fixtures
- External deps: gofmt available check

**pkg/telemetry (90.6% coverage)**:
- Unit: Event marshaling, roadmap formatting
- File I/O: Temp log files, atomic writes
- Edge cases: Large queues, empty sections

**pkg/launcher (85% - estimated)**:
- Unit: Graph construction, topological sort
- Algorithm: Cycle detection, partial order
- Edge cases: Empty graph, disconnected nodes

### 5.2 Test Patterns

**Table-Driven Tests**:
```go
func TestCoordinator_Claim(t *testing.T) {
    tests := []struct {
        name    string
        beadID  string
        wantErr bool
    }{
        {"valid claim", "bead-1", false},
        {"missing dependencies", "bead-2", true},
        {"bead not found", "bead-999", true},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // Test logic
        })
    }
}
```

**Mock External Dependencies**:
```go
// Test CSM orchestrator without real csm binary
type mockOrchestrator struct {
    createErr error
    uuid      string
}

func (m *mockOrchestrator) Create(name string) error {
    return m.createErr
}
```

**Temp File Pattern**:
```go
func TestLogger(t *testing.T) {
    tmpFile := filepath.Join(t.TempDir(), "test.jsonl")
    logger := NewLogger(tmpFile)
    // Test operations
    // Cleanup automatic via t.TempDir()
}
```

## 6. Security Architecture

### 6.1 Threat Model

**In Scope**:
- Queue file corruption (power loss, disk full)
- Concurrent access conflicts
- Session name collisions
- Resource exhaustion (too many sessions)

**Out of Scope** (assumes trusted environment):
- Malicious bead prompts (user-controlled)
- CSM binary tampering (trusted tool)
- Network attacks (local-only system)

### 6.2 Security Controls

**File Permissions**:
```bash
TASK-QUEUE.yaml:       0644 (rw-r--r--)
EXECUTION-LOG.jsonl:   0644 (rw-r--r--)
ROADMAP.md:            0644 (rw-r--r--)
```

**Input Validation**:
- Bead ID: Non-empty string check
- Session name: Non-empty string check
- File paths: Existence validation
- YAML: Schema validation on parse

**Resource Limits**:
- Session timeout: 1 hour default (configurable)
- Max iterations: 3 (prevents infinite loops)
- Roadmap size: 1500 tokens (prevents unbounded growth)

## 7. Performance Architecture

### 7.1 Performance Characteristics

**Operation Complexity**:
| Operation | Time | Space | Notes |
|-----------|------|-------|-------|
| Load queue | O(n) | O(n) | Parse YAML |
| Save queue | O(n) | O(n) | Marshal YAML |
| Claim bead | O(n+d) | O(1) | Linear search + dep check |
| Complete bead | O(n+b*d) | O(1) | Search + unblock scan |
| TopologicalSort | O(V+E) | O(V+E) | Kahn's algorithm |
| Log event | O(1) | O(1) | Append write |
| Generate roadmap | O(n) | O(n) | Queue scan |

**Scalability Limits** (v1):
- Queue size: ~1000 beads (O(n) operations acceptable)
- Dependencies: ~100 per bead (nested loop in unblock)
- Concurrent executors: 1 (single-process design)

### 7.2 Performance Optimizations

**Current**:
- Atomic writes prevent corruption, not performance bottleneck
- RLock for save (multiple readers allowed)
- Early return in dependency validation

**Future**:
- Index by bead ID (O(1) lookup)
- Dependency index (avoid O(b*d) unblock scan)
- Incremental roadmap updates (avoid full regeneration)
- Parallel executor processes (multi-bead concurrency)

## 8. Deployment Architecture

### 8.1 Prerequisites

**System Requirements**:
- OS: Linux, macOS (tmux available)
- Go: 1.25.1+ (for building)
- CSM: Latest version in PATH
- tmux: Any recent version

**File System**:
- Working directory: Writable for queue/log/roadmap
- Temp directory: For atomic write pattern

### 8.2 Installation Methods

**From Source**:
```bash
cd autonomous-swarm
make build          # → bin/swarm-executor
make install        # → ~/go/bin/swarm-executor
```

**Binary Distribution** (future):
```bash
curl -L https://releases/swarm-executor-v0.1.0-linux-amd64 -o swarm-executor
chmod +x swarm-executor
mv swarm-executor ~/go/bin/
```

### 8.3 Operational Model

**Single Execution**:
```bash
swarm-executor --queue TASK-QUEUE.yaml --bead-id bead-1 --session session-1
```

**Daemon Mode** (future):
```bash
swarm-executor --daemon --queue TASK-QUEUE.yaml
# Continuously polls ready queue and launches beads
```

**Multi-Instance**:
- Currently unsupported (single-process coordinator)
- Future: Distributed coordinator with file locking

## 9. Evolution and Extensibility

### 9.1 Extension Points

**Validation Phases**:
- Add new validators: Implement ValidateXX() interface
- Custom file types: Add to S8 switch statement
- Custom test runners: Add to S9 command execution

**Telemetry**:
- New event types: Add to ExecutionEvent schema
- Custom formats: Implement alternate logger
- External systems: Hook LogEvent() for webhooks

**Orchestrators**:
- Alternative CSM: Implement Orchestrator interface
- Local execution: Bypass CSM entirely
- Remote execution: Network-based orchestrator

### 9.2 Backward Compatibility

**Queue Schema Versioning**:
```yaml
schema_version: "1.0.0"  # Current
# Future: "2.0.0" with migration logic
```

**Migration Strategy**:
- Version field checked on parse
- Converters for old → new format
- Warnings for deprecated fields

## 10. Monitoring and Observability

### 10.1 Observability Interfaces

**Structured Logging** (EXECUTION-LOG.jsonl):
```bash
# Parse with jq
cat EXECUTION-LOG.jsonl | jq 'select(.event == "error")'

# Count events by type
cat EXECUTION-LOG.jsonl | jq -r '.event' | sort | uniq -c

# Extract escalations
cat EXECUTION-LOG.jsonl | jq 'select(.event == "escalate") | .details'
```

**Progress Tracking** (ROADMAP.md):
```bash
# Quick status
cat ROADMAP.md | grep "Progress:"

# Ready count
cat ROADMAP.md | grep "Count:" | head -1
```

**Queue State** (TASK-QUEUE.yaml):
```bash
# Parse with yq
yq '.in_progress | length' TASK-QUEUE.yaml

# Show blocked beads
yq '.blocked[] | .id' TASK-QUEUE.yaml
```

### 10.2 Metrics (Future)

**Proposed Metrics**:
- Beads executed per hour
- Average execution time per bead
- Escalation rate
- Queue depth over time
- Session creation failures

**Export Format** (Prometheus-compatible):
```
# HELP swarm_beads_total Total beads processed
# TYPE swarm_beads_total counter
swarm_beads_total{status="completed"} 42
swarm_beads_total{status="escalated"} 3
```

## 11. Related Documents

- [SPEC.md](SPEC.md) - Functional and non-functional requirements
- [README.md](README.md) - User documentation and getting started
- [ADR-001-dependency-graph.md](docs/adr/ADR-001-dependency-graph.md) - Dependency management
- [ADR-002-atomic-writes.md](docs/adr/ADR-002-atomic-writes.md) - Queue persistence
- [ADR-003-escalation-model.md](docs/adr/ADR-003-escalation-model.md) - Error handling

## 12. Revision History

| Version | Date | Changes | Author |
|---------|------|---------|--------|
| 1.0.0 | 2026-02-11 | Initial architecture documentation | Backfill Documentation |
