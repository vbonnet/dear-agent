# swarm-executor - Component Specification

## Document Metadata

- **Component**: swarm-executor
- **Version**: 0.1.0
- **Status**: Active
- **Last Updated**: 2026-02-11
- **Related**: [ARCHITECTURE.md](ARCHITECTURE.md), [../../SPEC.md](../../SPEC.md)

## 1. Component Overview

### 1.1 Purpose

`swarm-executor` is the primary CLI harness for executing autonomous beads within the
Autonomous Swarm system. It orchestrates the complete lifecycle of a single bead execution:
claiming from queue, AGM session management, prompt injection, result extraction, validation,
and completion/escalation handling.

### 1.2 Scope

This component is responsible for:
- CLI interface for bead execution (flag parsing, usage documentation)
- Telemetry initialization (execution log, roadmap generation)
- Component coordination (taskqueue, AGM, executor harness)
- Exit code mapping (success, error, escalation)
- Top-level error handling and cleanup

### 1.3 Component Context

```
┌─────────────────────────────────────────────────────────┐
│                  Execution Context                       │
│                                                          │
│  User/Launcher                                          │
│       │                                                  │
│       ▼                                                  │
│  ┌──────────────────┐                                   │
│  │ swarm-executor   │  (this component)                 │
│  │   main.go        │                                   │
│  └────────┬─────────┘                                   │
│           │                                              │
│           ├──► pkg/executor/Harness                     │
│           ├──► pkg/taskqueue/Coordinator                │
│           ├──► pkg/csm/Orchestrator                     │
│           └──► pkg/telemetry/Logger                     │
│                                                          │
│  Outputs:                                                │
│  • Exit code (0=success, 1=error, 2=escalation)        │
│  • EXECUTION-LOG.jsonl (telemetry events)               │
│  • ROADMAP.md (progress summary)                        │
│  • TASK-QUEUE.yaml (updated state)                      │
└─────────────────────────────────────────────────────────┘
```

### 1.4 Goals

1. **Simple CLI Interface**: Clear flags, helpful errors, Unix-standard exit codes
2. **Reliable Orchestration**: Wire up components correctly with proper error handling
3. **Observable Execution**: Log all events, generate roadmap, capture errors
4. **Minimal Logic**: Delegate domain logic to packages (thin glue layer)
5. **Testable**: CLI integration tests for flag handling and exit codes

### 1.5 Non-Goals

- Business logic implementation (delegated to packages)
- Configuration file parsing (v1 uses flags only)
- Daemon mode / continuous execution (single-shot execution)
- Interactive mode (non-interactive CLI only)

## 2. Functional Requirements

### 2.1 Command-Line Interface

**FR-CLI-001**: Binary SHALL accept three required flags: `--queue`, `--bead-id`, `--session`

**FR-CLI-002**: Binary SHALL validate all required flags are provided before execution

**FR-CLI-003**: Binary SHALL support `--version` flag to display version information

**FR-CLI-004**: Binary SHALL support `--help` flag to display usage documentation

**FR-CLI-005**: Binary SHALL print usage and exit with code 1 when required flags missing

**FR-CLI-006**: Binary SHALL print header to stderr showing version and binary path

### 2.2 Execution Orchestration

**FR-EXEC-001**: Binary SHALL initialize telemetry logger before execution

**FR-EXEC-002**: Binary SHALL log execution start event with bead ID and session name

**FR-EXEC-003**: Binary SHALL coordinate taskqueue, AGM, and executor harness

**FR-EXEC-004**: Binary SHALL generate roadmap after execution completion or error

**FR-EXEC-005**: Binary SHALL log completion event on successful execution

**FR-EXEC-006**: Binary SHALL log error event on execution failure

### 2.3 Exit Code Handling

**FR-EXIT-001**: Binary SHALL exit with code 0 on successful bead execution

**FR-EXIT-002**: Binary SHALL exit with code 1 on execution errors

**FR-EXIT-003**: Binary SHALL exit with code 2 on escalation (human intervention required)

**FR-EXIT-004**: Binary SHALL distinguish escalation errors from regular errors

**FR-EXIT-005**: Binary SHALL exit with code 0 when `--version` flag provided

**FR-EXIT-006**: Binary SHALL exit with code 0 when `--help` flag provided

### 2.4 Error Handling

**FR-ERR-001**: Binary SHALL print error messages to stderr

**FR-ERR-002**: Binary SHALL include contextual information in error messages

**FR-ERR-003**: Binary SHALL log errors to execution log before exiting

**FR-ERR-004**: Binary SHALL generate roadmap even when execution fails

**FR-ERR-005**: Binary SHALL continue with warning if roadmap generation fails

## 3. Non-Functional Requirements

### 3.1 Usability

**NFR-USE-001**: Help text SHALL include examples of common usage patterns

**NFR-USE-002**: Error messages SHALL be clear and actionable

**NFR-USE-003**: Version output SHALL include version number only (parseable)

**NFR-USE-004**: Header output SHALL go to stderr (not stdout) for pipeline compatibility

**NFR-USE-005**: Exit codes SHALL follow Unix conventions consistently

### 3.2 Reliability

**NFR-REL-001**: Binary SHALL handle initialization failures gracefully

**NFR-REL-002**: Binary SHALL not leave queue in inconsistent state on error

**NFR-REL-003**: Binary SHALL log errors even when logging infrastructure partially fails

### 3.3 Observability

**NFR-OBS-001**: All execution paths SHALL generate telemetry events

**NFR-OBS-002**: Exit code SHALL always match logged outcome (success/error/escalation)

**NFR-OBS-003**: Header SHALL be visible in all execution scenarios (except --version)

### 3.4 Performance

**NFR-PERF-001**: Binary startup SHALL complete in < 100ms for flag parsing

**NFR-PERF-002**: Telemetry initialization SHALL not block execution start

## 4. Interface Specification

### 4.1 Command-Line Flags

```
Required Flags:
  --queue <path>      Path to TASK-QUEUE.yaml file (required)
  --bead-id <id>      Bead ID to execute (required)
  --session <name>    AGM session name for execution (required)

Optional Flags:
  --version           Show version and exit
  --help              Show help and exit
```

**Flag Validation Rules**:
- `--queue`: Must be non-empty string, file existence checked at load time
- `--bead-id`: Must be non-empty string
- `--session`: Must be non-empty string
- `--version`: No argument, takes precedence over other flags
- `--help`: No argument, processed after version but before validation

### 4.2 Exit Codes

| Code | Meaning | Scenario |
|------|---------|----------|
| 0 | Success | Bead executed and completed successfully |
| 0 | Success | `--version` or `--help` flag provided |
| 1 | Error | Missing required flags |
| 1 | Error | Queue file not found or invalid |
| 1 | Error | Execution failed (AGM error, validation failure) |
| 2 | Escalation | Bead requires human intervention |

### 4.3 Output Streams

**stdout**:
- Version string (when `--version` provided)
- Execution progress messages ("Executing bead...", "✓ Bead executed successfully")

**stderr**:
- Header (version and binary path) - printed on all executions except `--version`
- Help text (when `--help` provided or validation fails)
- Error messages (failures, warnings)

### 4.4 Environment Variables

None currently used. Future: configuration file path via `SWARM_CONFIG`.

### 4.5 Files

**Input**:
- TASK-QUEUE.yaml (path via `--queue` flag)

**Output**:
- EXECUTION-LOG.jsonl (in same directory as queue file)
- ROADMAP.md (in same directory as queue file)
- TASK-QUEUE.yaml (updated in place)

## 5. Execution Flow

### 5.1 Main Execution Path

```
main()
  │
  ├─1─► Parse flags
  │       └─► flag.Parse()
  │
  ├─2─► Handle --version (early exit)
  │       └─► Print version to stdout
  │       └─► Exit(0)
  │
  ├─3─► Print header to stderr
  │       └─► "swarm-executor <version> (<path>)"
  │
  ├─4─► Handle --help (early exit)
  │       └─► Print usage to stderr
  │       └─► Exit(0)
  │
  ├─5─► Validate required flags
  │       └─► If missing: print error + usage → Exit(1)
  │
  └─6─► Execute bead
          └─► Call executeBead()
          └─► Exit with returned code
```

### 5.2 executeBead() Flow

```
executeBead(queuePath, beadID, sessionName) → exitCode
  │
  ├─1─► Initialize telemetry
  │       ├─► workDir := filepath.Dir(queuePath)
  │       ├─► logFile := workDir/EXECUTION-LOG.jsonl
  │       ├─► roadmapFile := workDir/ROADMAP.md
  │       └─► logger := telemetry.NewLogger(logFile)
  │
  ├─2─► Log execution start event
  │       └─► event: "execute", action: "start"
  │
  ├─3─► Load task queue
  │       ├─► coord := taskqueue.NewCoordinator(queuePath)
  │       ├─► coord.Load()
  │       └─► If error: log error → return 1
  │
  ├─4─► Initialize components
  │       ├─► orch := csm.NewOrchestrator()
  │       ├─► prompter := csm.NewPrompter()
  │       └─► harness := executor.NewHarness(coord, orch, prompter, 3)
  │
  ├─5─► Execute bead
  │       └─► err := harness.ExecuteBead(beadID, sessionName)
  │
  ├─6─► Handle result
  │       ├─► If err == nil:
  │       │     ├─► Log completion event
  │       │     ├─► Generate roadmap
  │       │     └─► return 0
  │       │
  │       ├─► If IsEscalationError(err):
  │       │     ├─► Print escalation to stderr
  │       │     ├─► Log escalation error
  │       │     ├─► Generate roadmap
  │       │     └─► return 2
  │       │
  │       └─► Else (regular error):
  │             ├─► Print error to stderr
  │             ├─► Log error
  │             ├─► Generate roadmap
  │             └─► return 1
  │
  └─► (Roadmap generation failures logged as warnings, not fatal)
```

### 5.3 Error Paths

**Queue Load Failure**:
```
coord.Load() fails
  └─► Print "Error: Failed to load task queue: <err>"
  └─► Log error event
  └─► return 1
```

**Execution Failure (Regular Error)**:
```
harness.ExecuteBead() returns non-escalation error
  └─► Print "Error: Execution failed: <err>"
  └─► Log error event
  └─► Generate roadmap
  └─► return 1
```

**Execution Failure (Escalation)**:
```
harness.ExecuteBead() returns escalation error
  └─► Print "Escalation: <err>"
  └─► Log error event (with escalation details)
  └─► Generate roadmap
  └─► return 2
```

## 6. Component Dependencies

### 6.1 Internal Dependencies

**pkg/executor**:
- `NewHarness(coordinator, orchestrator, prompter, maxIterations)` → Harness
- `Harness.ExecuteBead(beadID, sessionName)` → error
- `IsEscalationError(err)` → bool

**pkg/taskqueue**:
- `NewCoordinator(filePath)` → Coordinator
- `Coordinator.Load()` → error
- `Coordinator.Save()` → error

**pkg/csm**:
- `NewOrchestrator()` → Orchestrator
- `NewPrompter()` → Prompter

**pkg/telemetry**:
- `NewLogger(filePath)` → Logger
- `Logger.LogEvent(event)` → error
- `GenerateRoadmap(queuePath, outputPath)` → error
- `ExecutionEvent` struct

### 6.2 External Dependencies

**Go Standard Library**:
- `flag`: Command-line flag parsing
- `fmt`: String formatting and output
- `os`: File operations, exit codes, executable path
- `path/filepath`: Path manipulation

### 6.3 Build-Time Dependencies

**ldflags** (version injection):
```bash
go build -ldflags "
  -X main.Version=0.1.0
  -X main.GitCommit=$(git rev-parse HEAD)
  -X main.BuildDate=$(date -u +%Y-%m-%dT%H:%M:%SZ)
"
```

## 7. Testing Strategy

### 7.1 Test Coverage

**Target**: 1.9% (low coverage acceptable - thin glue layer)

**Rationale**: Most logic delegated to packages. CLI tests focus on integration and
flag handling, not unit testing orchestration logic.

### 7.2 Test Categories

**Integration Tests** (main_test.go):
1. `TestCLIVersion`: Verify `--version` flag output
2. `TestCLIHelp`: Verify `--help` flag output and sections
3. `TestCLIMissingFlags`: Verify error handling for missing flags
4. `TestExecuteBeadInvalidQueue`: Verify queue load error handling
5. `TestExecuteBeadSuccess`: Verify successful execution path (with mocks)

**Unit Tests** (header_test.go):
1. `TestVersionVariablesExist`: Verify version variables defined
2. `TestHeaderFormat`: Verify header format string
3. `TestPrintUsage`: Verify usage text content

### 7.3 Test Patterns

**Binary Build Pattern**:
```go
func buildBinary(t *testing.T) string {
    tmpDir := t.TempDir()
    binary := filepath.Join(tmpDir, "swarm-executor")
    cmd := exec.Command("go", "build", "-o", binary, ".")
    output, err := cmd.CombinedOutput()
    if err != nil {
        t.Fatalf("failed to build binary: %v\nOutput: %s", err, output)
    }
    return binary
}
```

**CLI Test Pattern**:
```go
func TestCLIVersion(t *testing.T) {
    binary := buildBinary(t)
    defer os.Remove(binary)

    cmd := exec.Command(binary, "--version")
    output, err := cmd.CombinedOutput()

    // Verify output and exit code
}
```

### 7.4 Test Limitations

**AGM Availability**: Tests requiring AGM will fail without real AGM binary. Tests verify
CLI correctly attempts execution but don't mock full AGM integration.

**tmux Availability**: Session creation tests require tmux server running.

**Concurrency**: Single-threaded execution - no concurrency tests needed at CLI level.

## 8. Configuration

### 8.1 Version Information

Version variables injected at build time via ldflags:

```go
var (
    Version   = "0.1.0-dev"  // Semantic version
    GitCommit = "unknown"     // Git commit hash
    BuildDate = "unknown"     // RFC3339 build timestamp
)
```

### 8.2 Constants

**maxIterations**: 3 (hardcoded in main.go, line 128)
- Max retry attempts for bead execution
- Future: Make configurable via flag or config file

**sessionTimeout**: 5 seconds (hardcoded in main.go, line 46)
- Timeout for WaitForSession polling
- Future: Make configurable

## 9. Security Considerations

### 9.1 Input Validation

**Flag Validation**:
- All required flags checked for non-empty strings
- File paths validated at load time (existence, readability)
- No shell injection risk (flags passed directly to packages)

**Output Safety**:
- Stderr output controlled (no user input echoed without sanitization)
- Roadmap generation failures logged as warnings (no escalation to errors)

### 9.2 File System Access

**File Permissions**:
- Queue file: Must be readable/writable by executor
- Log file: Created with default permissions (0644)
- Roadmap file: Created with default permissions (0644)

**Path Traversal**:
- No path construction from user input (flags are paths)
- workDir derived from queue file directory (trusted)

## 10. Observability

### 10.1 Telemetry Events

**Execution Start**:
```json
{
  "timestamp": "2026-02-11T12:00:00Z",
  "bead_id": "bead-1",
  "event": "execute",
  "details": {
    "session": "session-1",
    "action": "start"
  }
}
```

**Execution Success**:
```json
{
  "timestamp": "2026-02-11T12:05:00Z",
  "bead_id": "bead-1",
  "event": "complete",
  "details": {
    "session": "session-1",
    "status": "success"
  }
}
```

**Execution Error**:
```json
{
  "timestamp": "2026-02-11T12:02:00Z",
  "bead_id": "bead-1",
  "event": "error",
  "details": {
    "message": "execution failed: <error details>"
  }
}
```

### 10.2 Console Output

**Success Path**:
```
swarm-executor 0.1.0 (/path/to/binary)
Executing bead bead-1 in session session-1...
✓ Bead bead-1 executed successfully
```

**Error Path**:
```
swarm-executor 0.1.0 (/path/to/binary)
Executing bead bead-1 in session session-1...
Error: Execution failed: <error details>
```

**Escalation Path**:
```
swarm-executor 0.1.0 (/path/to/binary)
Executing bead bead-1 in session session-1...
Escalation: <escalation reason>
```

## 11. Performance Characteristics

### 11.1 Startup Time

- Flag parsing: < 1ms
- Telemetry init: < 10ms (file creation)
- Queue load: < 100ms (YAML parse for typical queue)
- **Total startup overhead**: < 150ms before harness execution

### 11.2 Memory Usage

- Base binary: ~10MB (Go runtime)
- Queue in memory: ~1KB per bead (typical)
- Log events: ~200 bytes per event (appended, not held in memory)
- **Peak memory**: < 50MB for typical workload

### 11.3 Execution Time

- Dominated by AGM session execution (minutes to hours)
- CLI overhead negligible compared to bead execution time

## 12. Failure Modes

### 12.1 Known Failure Scenarios

**Missing Queue File**:
```
Error: Failed to load task queue: open TASK-QUEUE.yaml: no such file or directory
Exit code: 1
```

**Invalid Queue YAML**:
```
Error: Failed to load task queue: yaml: unmarshal error
Exit code: 1
```

**AGM Not Available**:
```
Error: Execution failed: failed to create session: exec: "csm": executable file not found
Exit code: 1
```

**Bead Not Found**:
```
Error: Execution failed: failed to claim bead: bead not found in queue
Exit code: 1
```

**Max Iterations Exceeded**:
```
Escalation: max iterations exceeded (3/3)
Exit code: 2
```

### 12.2 Recovery Strategies

**Transient AGM Errors**: Retry bead execution (harness increments iteration count)

**Queue Corruption**: Restore from backup (manual intervention)

**Telemetry Log Failure**: Warning logged, execution continues (telemetry non-critical)

**Roadmap Generation Failure**: Warning logged, execution continues (roadmap derived state)

## 13. Future Enhancements

### 13.1 Planned Features

1. **Configuration File Support**: `--config` flag for YAML configuration
2. **Daemon Mode**: `--daemon` flag for continuous execution
3. **Dry Run Mode**: `--dry-run` to validate without execution
4. **Verbose Logging**: `--verbose` flag for debug output
5. **Multiple Bead Execution**: `--bead-ids` flag for batch execution

### 13.2 Potential Improvements

- Structured logging (JSON to stderr via `--log-format json`)
- Progress bar for long-running executions
- Timeout configuration via flag
- Signal handling (SIGTERM graceful shutdown)
- Metrics export (Prometheus endpoint via `--metrics-port`)

## 14. Glossary

- **Bead**: Autonomous task unit with defined prompts and dependencies
- **AGM**: Agent Session Manager - CLI tool for managing Claude sessions
- **Escalation**: Condition requiring human intervention (max retries or explicit signal)
- **Harness**: Executor component managing bead lifecycle
- **Session**: AGM session running autonomous agent
- **Telemetry**: Event logging and progress tracking system

## 15. References

- [../../SPEC.md](../../SPEC.md) - System-wide specification
- [../../ARCHITECTURE.md](../../ARCHITECTURE.md) - System architecture
- [ARCHITECTURE.md](ARCHITECTURE.md) - Component architecture
- [main.go](main.go) - Implementation
- [main_test.go](main_test.go) - Integration tests

## 16. Revision History

| Version | Date | Changes | Author |
|---------|------|---------|--------|
| 0.1.0 | 2026-02-11 | Initial specification | Backfill Documentation |

## 17. Approval

This specification documents the implemented behavior of swarm-executor v0.1.0 and serves
as the authoritative reference for CLI interface, execution flow, and error handling.
