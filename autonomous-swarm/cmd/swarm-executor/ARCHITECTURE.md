# swarm-executor - Component Architecture

## Document Metadata

- **Component**: swarm-executor
- **Version**: 0.1.0
- **Status**: Active
- **Last Updated**: 2026-02-11
- **Related**: [SPEC.md](SPEC.md), [../../ARCHITECTURE.md](../../ARCHITECTURE.md)

## 1. Component Overview

### 1.1 Architectural Role

`swarm-executor` serves as the **thin CLI orchestration layer** between user/launcher and
the core autonomous swarm execution system. It follows the "humble object" pattern - minimal
logic in the entry point, maximum delegation to well-tested packages.

### 1.2 Design Philosophy

**Principles**:
1. **Separation of Concerns**: CLI logic separate from business logic
2. **Fail Fast**: Validate inputs before expensive operations
3. **Observable by Default**: Log all state transitions
4. **Exit Codes as API**: Use exit codes for programmatic integration
5. **Thin Layer**: < 200 LOC main function, delegate everything else

**Trade-offs**:
- **Low test coverage acceptable**: Integration tests cover critical paths, unit tests for
  packages provide real coverage
- **Hardcoded configuration**: Simplicity over flexibility in v0.1.0
- **Single execution model**: No daemon mode - simpler implementation, clearer semantics

### 1.3 Component Layers

```
┌─────────────────────────────────────────────────────────┐
│                    CLI Layer                             │
│                   (main.go)                              │
│                                                          │
│  ┌────────────┐  ┌──────────────┐  ┌────────────────┐  │
│  │ Flag       │  │ Version      │  │ Help           │  │
│  │ Parsing    │  │ Handling     │  │ Text           │  │
│  └────────────┘  └──────────────┘  └────────────────┘  │
│                                                          │
│  ┌─────────────────────────────────────────────────┐   │
│  │           executeBead() - Main Logic             │   │
│  │                                                   │   │
│  │  • Telemetry initialization                     │   │
│  │  • Component wiring                             │   │
│  │  • Error classification                         │   │
│  │  • Exit code mapping                            │   │
│  └─────────────────────────────────────────────────┘   │
└──────────────────────┬──────────────────────────────────┘
                       │
                       ▼
┌─────────────────────────────────────────────────────────┐
│                 Package Layer                            │
│      (Domain Logic - see ../../ARCHITECTURE.md)         │
│                                                          │
│  pkg/executor     pkg/taskqueue     pkg/csm             │
│  pkg/telemetry    pkg/validation    pkg/launcher        │
└─────────────────────────────────────────────────────────┘
```

## 2. Code Structure

### 2.1 File Organization

```
cmd/swarm-executor/
├── main.go              # Entry point and orchestration (193 LOC)
├── main_test.go         # Integration tests (290 LOC)
├── header_test.go       # Header formatting tests (61 LOC)
├── SPEC.md              # Component specification
├── ARCHITECTURE.md      # Component architecture (this file)
└── docs/
    └── adr/
        ├── ADR-001-exit-code-design.md
        ├── ADR-002-telemetry-location.md
        └── ADR-003-flag-validation-order.md
```

### 2.2 main.go Structure

**Global Variables**:
```go
var (
    Version   = "0.1.0-dev"  // Set via ldflags
    GitCommit = "unknown"     // Set via ldflags
    BuildDate = "unknown"     // Set via ldflags
)
```

**Functions**:
```go
main()                              // Entry point (40 LOC)
printUsage()                        // Help text (27 LOC)
executeBead(queuePath, beadID,      // Main execution logic (99 LOC)
            sessionName) → int
logError(logger, beadID,            // Error logging helper (9 LOC)
         message)
```

### 2.3 Control Flow

```
main()
  │
  ├─► flag.Parse()                  // Parse CLI flags
  │
  ├─► if *showVersion               // Early exit: version
  │     └─► fmt.Printf() + os.Exit(0)
  │
  ├─► fmt.Fprintf(stderr, header)   // Print header to stderr
  │
  ├─► if *showHelp                  // Early exit: help
  │     └─► printUsage() + os.Exit(0)
  │
  ├─► if missing required flags     // Validation
  │     └─► printUsage() + os.Exit(1)
  │
  └─► exitCode := executeBead(...)  // Main execution
      └─► os.Exit(exitCode)
```

## 3. Execution Architecture

### 3.1 executeBead() Orchestration

```
executeBead(queuePath, beadID, sessionName) → exitCode
  │
  ├─ Initialize Telemetry
  │    ├─► workDir := filepath.Dir(queuePath)
  │    ├─► logFile := workDir/EXECUTION-LOG.jsonl
  │    ├─► roadmapFile := workDir/ROADMAP.md
  │    └─► logger := telemetry.NewLogger(logFile)
  │
  ├─ Log Start Event
  │    └─► logger.LogEvent(&ExecutionEvent{
  │           BeadID: beadID,
  │           Event: "execute",
  │           Details: {"session": sessionName, "action": "start"}
  │         })
  │
  ├─ Initialize Task Queue
  │    ├─► coord := taskqueue.NewCoordinator(queuePath)
  │    ├─► coord.Load()
  │    └─► if err: logError() + return 1
  │
  ├─ Initialize Components
  │    ├─► orch := csm.NewOrchestrator()
  │    ├─► prompter := csm.NewPrompter()
  │    └─► harness := executor.NewHarness(coord, orch, prompter, 3)
  │
  ├─ Execute Bead
  │    └─► err := harness.ExecuteBead(beadID, sessionName)
  │
  └─ Handle Result
       │
       ├─ if err == nil                    // SUCCESS PATH
       │    ├─► Log completion event
       │    ├─► Generate roadmap
       │    └─► return 0
       │
       ├─ if IsEscalationError(err)        // ESCALATION PATH
       │    ├─► Print "Escalation: ..." to stderr
       │    ├─► logError()
       │    ├─► Generate roadmap
       │    └─► return 2
       │
       └─ else                             // ERROR PATH
            ├─► Print "Error: ..." to stderr
            ├─► logError()
            ├─► Generate roadmap
            └─► return 1
```

### 3.2 Error Handling Strategy

**Error Classification**:
```
┌─────────────────────────────────────────────────────────┐
│                   Error Decision Tree                    │
├─────────────────────────────────────────────────────────┤
│                                                          │
│  err := harness.ExecuteBead(...)                        │
│    │                                                     │
│    ├─ err == nil                                        │
│    │    └─► Exit 0 (Success)                           │
│    │                                                     │
│    ├─ executor.IsEscalationError(err)                  │
│    │    └─► Exit 2 (Escalation - human needed)         │
│    │                                                     │
│    └─ else                                              │
│         └─► Exit 1 (Error - retry or fix)              │
│                                                          │
└─────────────────────────────────────────────────────────┘
```

**Escalation Detection**:
```go
// Escalation errors are typed errors from pkg/executor
if executor.IsEscalationError(err) {
    // err.(*ExecutionError).Type == ErrorEscalation
    // Reasons:
    //  - Max iterations exceeded
    //  - Explicit ESCALATE: signal in output
    fmt.Fprintf(os.Stderr, "Escalation: %v\n", err)
    return 2
}
```

**Logging Failures**:
```go
// Logging failures are non-fatal - execution continues
if err := logger.LogEvent(startEvent); err != nil {
    fmt.Fprintf(os.Stderr, "Warning: Failed to log start event: %v\n", err)
    // Continue execution - telemetry is best-effort
}
```

**Roadmap Generation Failures**:
```go
// Roadmap generation failures are non-fatal - roadmap is derived state
if err := telemetry.GenerateRoadmap(queuePath, roadmapFile); err != nil {
    fmt.Fprintf(os.Stderr, "Warning: Failed to generate roadmap: %v\n", err)
    // Continue - roadmap can be regenerated from queue
}
```

### 3.3 Telemetry Initialization

**File Path Derivation**:
```go
workDir := filepath.Dir(queuePath)
// Example:
//   queuePath = "/home/user/project/TASK-QUEUE.yaml"
//   workDir   = "/home/user/project"

logFile := filepath.Join(workDir, "EXECUTION-LOG.jsonl")
// logFile = "/home/user/project/EXECUTION-LOG.jsonl"

roadmapFile := filepath.Join(workDir, "ROADMAP.md")
// roadmapFile = "/home/user/project/ROADMAP.md"
```

**Rationale**: All output files colocated with queue file - simplifies discovery and cleanup.

**Logger Initialization**:
```go
logger := telemetry.NewLogger(logFile)
// Logger is append-only - file created on first LogEvent()
// No explicit Open/Close - file handle managed per write
```

## 4. Component Integration

### 4.1 Dependency Injection

**Pattern**: Constructor injection at component boundaries

```go
// Initialize independent components
coord := taskqueue.NewCoordinator(queuePath)
orch := csm.NewOrchestrator()
prompter := csm.NewPrompter()

// Inject dependencies into harness
harness := executor.NewHarness(
    coord,      // Task queue coordinator
    orch,       // CSM orchestrator
    prompter,   // Prompt injector
    3,          // Max iterations
)
```

**Benefits**:
- Clear dependency graph (no singletons)
- Testable (can inject mocks)
- Explicit initialization order
- No hidden global state

### 4.2 Package Interactions

```
swarm-executor (main.go)
    │
    ├──► pkg/taskqueue/Coordinator
    │     ├─ NewCoordinator(filePath) → Coordinator
    │     ├─ Load() → error
    │     └─ Save() → error
    │
    ├──► pkg/csm/Orchestrator
    │     └─ NewOrchestrator() → Orchestrator
    │
    ├──► pkg/csm/Prompter
    │     └─ NewPrompter() → Prompter
    │
    ├──► pkg/executor/Harness
    │     ├─ NewHarness(coord, orch, prompter, maxIter) → Harness
    │     ├─ ExecuteBead(beadID, sessionName) → error
    │     └─ IsEscalationError(err) → bool
    │
    └──► pkg/telemetry
          ├─ NewLogger(filePath) → Logger
          ├─ Logger.LogEvent(event) → error
          └─ GenerateRoadmap(queuePath, outputPath) → error
```

### 4.3 Exit Code Mapping

```
Harness Execution Result → Exit Code Mapping

┌────────────────────────────┬─────────────┬─────────────┐
│ Result                     │ Exit Code   │ Action      │
├────────────────────────────┼─────────────┼─────────────┤
│ err == nil                 │ 0           │ Success     │
│ IsEscalationError(err)     │ 2           │ Escalate    │
│ err != nil (other)         │ 1           │ Error       │
│ Missing flags              │ 1           │ Error       │
│ Queue load failure         │ 1           │ Error       │
│ --version flag             │ 0           │ Success     │
│ --help flag                │ 0           │ Success     │
└────────────────────────────┴─────────────┴─────────────┘
```

## 5. Testing Architecture

### 5.1 Test Strategy

**Philosophy**: Integration tests for CLI, unit tests for packages

**Coverage Breakdown**:
- main.go: 1.9% (thin glue layer - integration tests provide coverage)
- Integration tests: CLI behavior, flag handling, exit codes
- Package tests: 50-90% coverage (real logic lives here)

### 5.2 Integration Test Patterns

**Binary Build and Execute**:
```go
func buildBinary(t *testing.T) string {
    t.Helper()
    tmpDir := t.TempDir()
    binary := filepath.Join(tmpDir, "swarm-executor")

    // Build binary with test version
    cmd := exec.Command("go", "build", "-o", binary, ".")
    output, err := cmd.CombinedOutput()
    if err != nil {
        t.Fatalf("failed to build binary: %v\nOutput: %s", err, output)
    }
    return binary
}

func TestCLIVersion(t *testing.T) {
    binary := buildBinary(t)
    defer os.Remove(binary)

    cmd := exec.Command(binary, "--version")
    output, err := cmd.CombinedOutput()

    // Assertions on output and exit code
}
```

**Test Fixtures**:
```go
func TestExecuteBeadSuccess(t *testing.T) {
    tmpDir := t.TempDir()

    // Create test queue file
    queueFile := filepath.Join(tmpDir, "queue.yaml")
    queueData := `schema_version: "1.0.0"
last_updated: 2024-01-01T00:00:00Z
ready:
  - id: bead-test
    title: Test bead
    tier: 1
    prompts:
      start: "echo test"
in_progress: []
blocked: []
completed: []
`
    os.WriteFile(queueFile, []byte(queueData), 0644)

    // Run binary
    binary := buildBinary(t)
    cmd := exec.Command(binary,
        "--queue", queueFile,
        "--bead-id", "bead-test",
        "--session", "test-session")
    output, err := cmd.CombinedOutput()

    // Verify behavior
}
```

### 5.3 Test Coverage Matrix

| Test | Purpose | Exit Code | Output Verification |
|------|---------|-----------|-------------------|
| TestCLIVersion | --version flag | 0 | Version string format |
| TestCLIHelp | --help flag | 0 | Help sections present |
| TestCLIMissingFlags | Validation | 1 | Error + usage printed |
| TestExecuteBeadInvalidQueue | Queue load fail | 1 | Error message |
| TestExecuteBeadSuccess | Happy path | 0 or 2 | Log file created |
| TestPrintUsage | Usage text | N/A | Section content |
| TestVersionVariables | Build config | N/A | Variables defined |
| TestHeaderFormat | Header output | N/A | Format string |

### 5.4 Mock Strategies

**External Dependencies (CSM, tmux)**:
- Integration tests attempt execution but expect failure (CSM unavailable)
- Tests verify CLI correctly handles CSM errors
- Package-level tests mock CSM orchestrator

**File System**:
- Use t.TempDir() for isolated test directories
- Create fixture files for queue/log tests
- Verify file creation/modification

## 6. Configuration Architecture

### 6.1 Build-Time Configuration

**ldflags Injection**:
```bash
# Makefile pattern
VERSION := $(shell git describe --tags --always --dirty)
COMMIT := $(shell git rev-parse HEAD)
DATE := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

LDFLAGS := -X main.Version=$(VERSION) \
           -X main.GitCommit=$(COMMIT) \
           -X main.BuildDate=$(DATE)

go build -ldflags "$(LDFLAGS)" -o bin/swarm-executor ./cmd/swarm-executor
```

**Version Variables**:
```go
var (
    Version   = "0.1.0-dev"  // Default for go run
    GitCommit = "unknown"
    BuildDate = "unknown"
)

// Accessed in:
// - --version flag output
// - Header printed to stderr
```

### 6.2 Runtime Configuration

**Hardcoded Constants**:
```go
const (
    maxIterations = 3           // Line 128 - passed to NewHarness()
    sessionTimeout = 5000000000 // 5 seconds - line 46 WaitForSession()
)
```

**Future**: Configuration file support via `--config` flag
```yaml
# swarm-executor.yaml (future)
max_iterations: 3
session_timeout: 5s
log_level: info
telemetry:
  enabled: true
  log_file: EXECUTION-LOG.jsonl
```

### 6.3 File Path Configuration

**Derivation from Queue Path**:
```go
workDir := filepath.Dir(queuePath)  // Extract directory
logFile := filepath.Join(workDir, "EXECUTION-LOG.jsonl")
roadmapFile := filepath.Join(workDir, "ROADMAP.md")
```

**Rationale**: Colocated files simplify discovery and cleanup. All execution artifacts
live alongside queue file.

## 7. Observability Architecture

### 7.1 Logging Levels

**Error Stream (stderr)**:
- Header (always printed except --version)
- Error messages (execution failures)
- Escalation messages (human intervention needed)
- Warnings (non-fatal failures like roadmap generation)

**Output Stream (stdout)**:
- Version string (--version only)
- Execution progress ("Executing bead...", "✓ Bead executed successfully")

**Rationale**: stderr for diagnostics, stdout for parseable output (version, success messages)

### 7.2 Telemetry Events

**Event Lifecycle**:
```
Execute Start
  └─► Event: "execute", Action: "start"
      Details: {session: <name>, action: "start"}

[Harness executes bead]

Execute Complete (success)
  └─► Event: "complete", Status: "success"
      Details: {session: <name>, status: "success"}

Execute Complete (error)
  └─► Event: "error"
      Details: {message: "execution failed: <details>"}

Execute Complete (escalation)
  └─► Event: "error"
      Details: {message: "escalation: <reason>"}
```

### 7.3 Roadmap Generation

**Trigger Points**:
- After successful execution (line 174-177)
- After error (line 153-156)
- After escalation (line 142-144)

**Generation Logic**:
```go
if err := telemetry.GenerateRoadmap(queuePath, roadmapFile); err != nil {
    // Non-fatal warning - roadmap is derived state
    fmt.Fprintf(os.Stderr, "Warning: Failed to generate roadmap: %v\n", err)
}
// Continue execution regardless of roadmap generation result
```

**Rationale**: Roadmap is derived from queue state - can be regenerated anytime. Failure
to generate roadmap should not prevent execution completion.

## 8. Performance Characteristics

### 8.1 Execution Phases

```
Phase               | Duration       | Bottleneck
--------------------|----------------|---------------------------
Flag parsing        | < 1ms          | Negligible
Header output       | < 1ms          | Negligible
Telemetry init      | < 10ms         | File creation
Queue load          | 50-100ms       | YAML parsing (varies with size)
Component init      | < 5ms          | Memory allocation
Harness execution   | Minutes-hours  | CSM session execution
Roadmap generation  | 10-50ms        | Queue serialization
--------------------|----------------|---------------------------
Total overhead      | < 200ms        | Everything except harness
```

### 8.2 Memory Profile

```
Component           | Memory Usage   | Notes
--------------------|----------------|---------------------------
Binary base         | ~10MB          | Go runtime
Queue in-memory     | ~1KB/bead      | Depends on bead count
Logger              | < 1MB          | Append-only, no buffering
Orchestrator        | < 1MB          | Stateless wrappers
Harness             | < 5MB          | Execution state
--------------------|----------------|---------------------------
Total typical       | < 50MB         | Small workload (< 100 beads)
```

### 8.3 I/O Operations

**Read Operations**:
- TASK-QUEUE.yaml: 1x read at startup (coordinator.Load())

**Write Operations**:
- EXECUTION-LOG.jsonl: 2-3x appends (start, complete/error events)
- TASK-QUEUE.yaml: 2x writes (coordinator saves after claim and complete)
- ROADMAP.md: 1x write (roadmap generation)

**Optimization**: Atomic writes use temp files (2x disk usage during write), acceptable
for reliability benefits.

## 9. Security Architecture

### 9.1 Input Validation

**Flag Validation**:
```go
// Required flags validation
if *queuePath == "" || *beadID == "" || *sessionName == "" {
    fmt.Fprintf(os.Stderr, "Error: Missing required flags\n\n")
    printUsage()
    os.Exit(1)
}
```

**File Path Handling**:
- Queue path passed directly to coordinator (validated at load time)
- No path construction from user input (no injection risk)
- workDir derived from trusted queue path

### 9.2 Error Information Disclosure

**Safe Error Messages**:
```go
// Error includes context but no sensitive data
fmt.Fprintf(os.Stderr, "Error: Failed to load task queue: %v\n", err)
```

**Logged Errors**:
```go
// Errors logged with bead ID and message only
logError(logger, beadID, fmt.Sprintf("execution failed: %v", err))
```

### 9.3 File Permissions

**Output Files**:
- Created with default umask (typically 0644)
- No explicit permission setting (inherits from OS)
- Future: Allow configurable permissions via config

## 10. Failure Modes and Recovery

### 10.1 Graceful Degradation

**Telemetry Failures**:
```
Logger.LogEvent() fails
  └─► Print warning to stderr
  └─► Continue execution
  └─► Execution result unaffected
```

**Roadmap Generation Failures**:
```
GenerateRoadmap() fails
  └─► Print warning to stderr
  └─► Continue execution
  └─► Roadmap can be regenerated manually
```

### 10.2 Fatal Failures

**Queue Load Failures**:
```
coordinator.Load() fails
  └─► Log error event (best effort)
  └─► Print error to stderr
  └─► Exit 1 (cannot proceed without queue)
```

**Execution Failures**:
```
harness.ExecuteBead() fails
  └─► Log error event
  └─► Generate roadmap (best effort)
  └─► Exit 1 (error) or 2 (escalation)
```

### 10.3 Recovery Procedures

**Queue Corruption**:
1. Check EXECUTION-LOG.jsonl for recent events
2. Restore TASK-QUEUE.yaml from backup
3. Manually verify bead state consistency
4. Re-run failed beads

**Log File Issues**:
1. Logs are append-only (corruption unlikely)
2. If corrupted, rename and start fresh log
3. Historical events lost but execution continues

## 11. Extension Points

### 11.1 Future Enhancement Hooks

**Configuration Extension**:
```go
// Future: Load config from file
config := loadConfig(*configPath)
harness := executor.NewHarness(coord, orch, prompter, config.MaxIterations)
```

**Custom Telemetry Backends**:
```go
// Future: Support multiple loggers
loggers := []telemetry.Logger{
    telemetry.NewLogger(logFile),
    telemetry.NewRemoteLogger(metricsEndpoint),
}
```

**Plugin Architecture**:
```go
// Future: Support custom validators
harness.RegisterValidator("s10", customValidator)
```

### 11.2 Backward Compatibility

**Version Output**:
- Stable format: "swarm-executor version X.Y.Z"
- Parseable by scripts (no extra decorations)

**Exit Codes**:
- Stable contract: 0=success, 1=error, 2=escalation
- New exit codes (3+) only with major version bump

**Flag Names**:
- Existing flags never removed or renamed
- New flags added with backward-compatible defaults

## 12. Related Architecture Decisions

### 12.1 ADR References

**ADR-001: Exit Code Design**
- Decision: Use 3-tier exit codes (0/1/2)
- Rationale: Distinguish escalation from errors for automation
- Alternative: Use 0/1 with stderr parsing (rejected - harder to script)

**ADR-002: Telemetry File Location**
- Decision: Colocate logs with queue file
- Rationale: Simplified discovery, cleanup, and archival
- Alternative: Separate log directory (rejected - requires config)

**ADR-003: Flag Validation Order**
- Decision: Validate after --version/--help handling
- Rationale: Allow help even with invalid flags
- Alternative: Validate first (rejected - poor UX)

### 12.2 Implementation Patterns

**Error Wrapping**:
```go
if err != nil {
    return fmt.Errorf("failed to load task queue: %w", err)
}
```

**Best Effort Operations**:
```go
if err := logger.LogEvent(event); err != nil {
    // Warning, not fatal
    fmt.Fprintf(os.Stderr, "Warning: Failed to log event: %v\n", err)
}
```

**Atomic Exit Codes**:
```go
exitCode := executeBead(...)  // All paths return int
os.Exit(exitCode)             // Single exit point
```

## 13. Deployment Considerations

### 13.1 Binary Distribution

**Installation Methods**:
```bash
# From source
cd autonomous-swarm
make build-executor

# Direct install
go install github.com/[REDACTED_EMPLOYER]-src/ai-tools/autonomous-swarm/cmd/swarm-executor@latest
```

### 13.2 Runtime Requirements

**Prerequisites**:
- Go 1.25+ (for building from source)
- csm binary in PATH
- tmux server running
- Writable working directory

**Optional**:
- gofmt (for S8 validation)
- go test (for S9 validation)

### 13.3 Operational Patterns

**Single Execution**:
```bash
swarm-executor --queue ./TASK-QUEUE.yaml --bead-id bead-1 --session sess-1
```

**Scripted Execution** (launcher pattern):
```bash
#!/bin/bash
for bead in $(yq '.ready[].id' TASK-QUEUE.yaml); do
    swarm-executor --queue TASK-QUEUE.yaml --bead-id "$bead" --session "auto-$bead"
    if [ $? -eq 2 ]; then
        echo "Escalation detected for $bead - pausing"
        exit 2
    fi
done
```

## 14. Monitoring and Diagnostics

### 14.1 Health Checks

**Execution Verification**:
```bash
# Check exit code
swarm-executor --queue Q.yaml --bead-id B --session S
echo $?  # 0=success, 1=error, 2=escalation
```

**Log Analysis**:
```bash
# Parse execution log
cat EXECUTION-LOG.jsonl | jq 'select(.event == "error")'

# Count events
cat EXECUTION-LOG.jsonl | jq -r '.event' | sort | uniq -c
```

### 14.2 Debugging Techniques

**Verbose Output** (future):
```bash
swarm-executor --verbose --queue Q.yaml --bead-id B --session S
# Prints component initialization, package calls, state transitions
```

**Trace Execution** (current):
```bash
# Enable Go tracing
GOTRACEBACK=all swarm-executor --queue Q.yaml --bead-id B --session S 2>&1 | tee debug.log
```

## 15. References

- [SPEC.md](SPEC.md) - Component specification
- [main.go](main.go) - Implementation source
- [../../ARCHITECTURE.md](../../ARCHITECTURE.md) - System architecture
- [../../pkg/executor/README.md](../../pkg/executor/README.md) - Executor harness docs

## 16. Revision History

| Version | Date | Changes | Author |
|---------|------|---------|--------|
| 0.1.0 | 2026-02-11 | Initial architecture documentation | Backfill Documentation |

## 17. Approval

This architecture document reflects the implemented design of swarm-executor v0.1.0 and
serves as the authoritative reference for component structure, integration patterns, and
operational characteristics.
