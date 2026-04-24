# Agent Monitoring - Specification

## Overview

The monitoring package provides comprehensive runtime monitoring and validation for sub-agent executions within the Engram system. It detects fake implementations and ensures quality by tracking multiple signals including file operations, git commits, test execution, and code quality metrics.

## Purpose

**Primary Goal**: Prevent sub-agents from submitting fake or incomplete implementations by validating actual work performed.

**Key Capabilities**:
- Real-time file system monitoring via in-memory events
- Git commit tracking with JSONL persistence
- Test execution detection via output parsing
- Multi-signal quality validation with fallback mechanisms
- Configurable thresholds per task complexity

## Functional Requirements

### FR-1: Agent Monitoring Lifecycle

The system SHALL provide complete lifecycle management for monitoring sub-agent executions:

- **FR-1.1**: Initialize monitoring for a specific agent ID and work directory
- **FR-1.2**: Start monitoring (activate all watchers and hooks)
- **FR-1.3**: Stop monitoring (cleanup watchers, uninstall hooks, persist final state)
- **FR-1.4**: Validate results using multi-signal scoring with fallback verification
- **FR-1.5**: Retrieve real-time statistics during execution

### FR-2: File System Monitoring

The system SHALL monitor file system changes using fsnotify:

- **FR-2.1**: Detect file creation events
- **FR-2.2**: Detect file modification events
- **FR-2.3**: Detect file deletion events
- **FR-2.4**: Detect file rename events
- **FR-2.5**: Filter out irrelevant files (.git/, temp files, IDE configs, node_modules/)
- **FR-2.6**: Publish events to in-memory event bus with agent ID, path, operation, and timestamp
- **FR-2.7**: Events are NOT persisted to disk (in-memory coordination only)

### FR-3: Git Commit Monitoring

The system SHALL monitor git operations through post-commit hooks:

- **FR-3.1**: Install post-commit hook in .git/hooks/
- **FR-3.2**: Backup existing hooks before installation
- **FR-3.3**: Capture commit metadata (hash, message, author, email, timestamp)
- **FR-3.4**: Capture commit statistics (files changed, insertions, deletions)
- **FR-3.5**: Write commit events DIRECTLY to JSONL event log (bash script append)
- **FR-3.6**: Git hooks are the ONLY component that persists events to disk
- **FR-3.7**: Uninstall hooks and restore backups on cleanup

### FR-4: Output Parsing

The system SHALL parse sub-agent output to detect command execution:

- **FR-4.1**: Detect Go test execution (`go test`)
- **FR-4.2**: Detect npm test execution (`npm test`)
- **FR-4.3**: Detect pytest execution (`pytest`)
- **FR-4.4**: Detect git commands (`git commit`, `git push`)
- **FR-4.5**: Detect build commands (`make build`)
- **FR-4.6**: Extract test results (passed/failed counts, duration)
- **FR-4.7**: Publish command execution events to in-memory event bus
- **FR-4.8**: Events are NOT persisted to disk (in-memory coordination only)

### FR-5: Multi-Signal Validation

The system SHALL validate sub-agent work using weighted signals with robust fallback mechanisms:

- **FR-5.1**: Git Commits Signal (weight: 0.2)
  - Primary: Count `sub_agent.git.commit` events in JSONL log
  - Fallback: Parse `git log --oneline` output
  - Threshold: MinCommitCount (default: 2)
  - **Note**: Git hooks ensure this data is available in JSONL

- **FR-5.2**: File Count Signal (weight: 0.2)
  - Primary: Count `sub_agent.file.created` events from JSONL log
  - Fallback: Walk directory and count source files (ALWAYS used - events not persisted)
  - Threshold: MinFileCount (default: 3)
  - **Note**: FileWatcher events are in-memory only, so fallback is primary mechanism

- **FR-5.3**: Line Count Signal (weight: 0.2)
  - Primary/Only: Walk directory and sum lines in all source files
  - Exclude .git/, node_modules/, IDE files
  - Threshold: MinLineCount (default: 50)
  - **Note**: No event-based tracking; direct filesystem analysis only

- **FR-5.4**: Test Execution Signal (weight: 0.3)
  - Primary: Count test events from OutputParser (in-memory only)
  - Secondary: Count `sub_agent.test.started` or `sub_agent.test.passed` events from JSONL
  - Fallback: Attempt to run tests (go test, npm test, pytest)
  - Threshold: MinTestRuns (default: 1)
  - **Note**: OutputParser events not persisted; relies on fallback test execution

- **FR-5.5**: Stub Keywords Signal (weight: 0.1)
  - Primary/Only: Use grep to count TODO, FIXME, NotImplemented, panic("not implemented")
  - Threshold: MaxStubKeywords (default: 3)
  - **Note**: No event-based tracking; direct grep analysis only

- **FR-5.6**: Calculate aggregate score as weighted sum
- **FR-5.7**: Pass if score >= PassThreshold (default: 0.6)
- **FR-5.8**: Validation HEAVILY relies on fallback mechanisms due to sparse event log

### FR-6: Validation Configurations

The system SHALL provide preset configurations for different task complexities:

- **FR-6.1**: DefaultValidationConfig (balanced)
  - MinFileCount: 3, MinLineCount: 50, MinCommitCount: 2
  - MinTestRuns: 1, MaxStubKeywords: 3, PassThreshold: 0.6

- **FR-6.2**: SimpleTaskConfig (relaxed)
  - MinFileCount: 1, MinLineCount: 10, MinCommitCount: 1
  - MinTestRuns: 0, MaxStubKeywords: 5, PassThreshold: 0.5

- **FR-6.3**: ComplexTaskConfig (strict)
  - MinFileCount: 5, MinLineCount: 200, MinCommitCount: 5
  - MinTestRuns: 2, MaxStubKeywords: 0, PassThreshold: 0.7

- **FR-6.4**: Support custom validation configurations

### FR-7: Event Bus Integration

The system SHALL use EventBus for in-memory event coordination:

- **FR-7.1**: Event types published to in-memory bus:
  - `sub_agent.started` - Agent execution started
  - `sub_agent.completed` - Agent execution completed
  - `sub_agent.file.created` - File created (in-memory only)
  - `sub_agent.file.modified` - File modified (in-memory only)
  - `sub_agent.file.deleted` - File deleted (in-memory only)
  - `sub_agent.git.commit` - Git commit detected (persisted by git hook)
  - `sub_agent.test.started` - Test execution started (in-memory only)
  - `sub_agent.test.passed` - Test passed (in-memory only)
  - `sub_agent.test.failed` - Test failed (in-memory only)

- **FR-7.2**: All events include agent_id and timestamp
- **FR-7.3**: EventBus provides in-memory pub/sub coordination ONLY
- **FR-7.4**: ONLY git commit events are persisted to JSONL (via hook script)
- **FR-7.5**: FileWatcher and OutputParser events remain in-memory for real-time coordination

### FR-8: Validation Results

The system SHALL return structured validation results:

- **FR-8.1**: Overall pass/fail status
- **FR-8.2**: Aggregate score (0.0-1.0)
- **FR-8.3**: Individual signal results with:
  - Signal name
  - Actual value
  - Expected threshold
  - Weight
  - Pass/fail status
  - Human-readable message
- **FR-8.4**: Summary message explaining outcome

### FR-9: Statistics Reporting

The system SHALL provide real-time statistics:

- **FR-9.1**: Agent ID
- **FR-9.2**: Start time
- **FR-9.3**: Duration (elapsed or total)
- **FR-9.4**: Files created count (from JSONL event log)
- **FR-9.5**: Files modified count (from JSONL event log)
- **FR-9.6**: Commits detected count (from JSONL event log)
- **FR-9.7**: Test runs count (from JSONL event log)
- **FR-9.8**: Statistics read from sparse event log (only git commits reliably present)

## Non-Functional Requirements

### NFR-1: Performance

- **NFR-1.1**: File watcher SHALL process events with < 100ms latency
- **NFR-1.2**: Validation SHALL complete in < 5 seconds for typical projects
- **NFR-1.3**: Event log writes SHALL be direct file appends (no buffering)
- **NFR-1.4**: Fallback verification (file walking, test execution) may take longer

### NFR-2: Reliability

- **NFR-2.1**: Monitoring failures SHALL NOT block sub-agent execution
- **NFR-2.2**: Missing event log SHALL fall back to direct verification (git log, file count, test execution)
- **NFR-2.3**: Sparse event log (only git commits) SHALL NOT prevent validation
- **NFR-2.4**: Git hook installation SHALL preserve existing hooks via backup
- **NFR-2.5**: File watcher SHALL handle inotify limit errors gracefully
- **NFR-2.6**: Validation SHALL work even without EventBus events (fallback-first design)

### NFR-3: Compatibility

- **NFR-3.1**: Support Go projects (go test)
- **NFR-3.2**: Support npm projects (npm test)
- **NFR-3.3**: Support Python projects (pytest)
- **NFR-3.4**: Work with any git repository structure

### NFR-4: Security

- **NFR-4.1**: Event logs SHALL be written to secure temporary directories
- **NFR-4.2**: Git hooks SHALL be executable only by owner (0755)
- **NFR-4.3**: No sensitive data SHALL be logged in events

### NFR-5: Maintainability

- **NFR-5.1**: Validation thresholds SHALL be externally configurable
- **NFR-5.2**: Output patterns SHALL be extensible via AddPattern()
- **NFR-5.3**: File filters SHALL be extensible via AddFilter()
- **NFR-5.4**: All components SHALL be testable in isolation

## API Specification

### AgentMonitor API

```go
// Constructor
func NewAgentMonitor(agentID, workDir string) (*AgentMonitor, error)

// Configuration
func (am *AgentMonitor) SetValidationConfig(config ValidationConfig)

// Lifecycle
func (am *AgentMonitor) Start() error
func (am *AgentMonitor) Stop() error

// Runtime
func (am *AgentMonitor) ParseOutput(r io.Reader) error
func (am *AgentMonitor) GetStats() MonitorStats

// Validation
func (am *AgentMonitor) Validate() (*ValidationResult, error)

// Utility
func (am *AgentMonitor) GetEventLog() string
```

### Data Types

```go
type ValidationConfig struct {
    MinFileCount    int
    MinLineCount    int
    MinCommitCount  int
    MinTestRuns     int
    MaxStubKeywords int
    PassThreshold   float64
}

type ValidationResult struct {
    Passed  bool
    Score   float64
    Signals []ValidationSignal
    Summary string
}

type ValidationSignal struct {
    Name     string
    Value    interface{}
    Expected interface{}
    Weight   float64
    Passed   bool
    Message  string
}

type MonitorStats struct {
    AgentID         string
    StartTime       time.Time
    Duration        time.Duration
    FilesCreated    int
    FilesModified   int
    CommitsDetected int
    TestRuns        int
}
```

## Usage Patterns

### Pattern 1: Basic Monitoring

```go
monitor, _ := monitoring.NewAgentMonitor("agent-123", workDir)
monitor.Start()
defer monitor.Stop()

// ... sub-agent executes ...

result, _ := monitor.Validate()
if !result.Passed {
    log.Warnf("Validation failed: %s", result.Summary)
}
```

### Pattern 2: Output Parsing

```go
monitor, _ := monitoring.NewAgentMonitor("agent-123", workDir)
monitor.Start()
defer monitor.Stop()

cmd := exec.Command("sub-agent", "task")
stdout, _ := cmd.StdoutPipe()

go monitor.ParseOutput(stdout)
cmd.Run()

result, _ := monitor.Validate()
```

### Pattern 3: Custom Configuration

```go
monitor, _ := monitoring.NewAgentMonitor("agent-123", workDir)
monitor.SetValidationConfig(monitoring.ValidationConfig{
    MinFileCount:    5,
    MinLineCount:    200,
    MinCommitCount:  3,
    MinTestRuns:     2,
    MaxStubKeywords: 0,
    PassThreshold:   0.8,
})
```

### Pattern 4: Real-Time Monitoring

```go
monitor, _ := monitoring.NewAgentMonitor("agent-123", workDir)
monitor.Start()

ticker := time.NewTicker(5 * time.Second)
go func() {
    for range ticker.C {
        stats := monitor.GetStats()
        // Note: Stats read from sparse event log (mostly git commits)
        log.Printf("Files: %d, Commits: %d, Tests: %d",
            stats.FilesCreated, stats.CommitsDetected, stats.TestRuns)
    }
}()
```

## Architecture Notes

### Event Persistence Model

**Critical Implementation Detail**: The monitoring system has a **sparse event persistence model**:

1. **Git Hooks Write to JSONL**: Only git post-commit hooks write events directly to the JSONL event log file. This happens via a bash script that appends JSON to the file.

2. **FileWatcher In-Memory Only**: FileWatcher publishes events to the in-memory EventBus for real-time coordination, but these events are NOT persisted to disk.

3. **OutputParser In-Memory Only**: OutputParser publishes events to the in-memory EventBus for real-time coordination, but these events are NOT persisted to disk.

4. **No File Subscriber**: The EventBus does NOT have a file subscriber. There is no automatic persistence of EventBus events to the JSONL log.

5. **Validation Uses Fallbacks**: Because the event log only contains git commit events, validation heavily relies on fallback mechanisms:
   - File count: Always walks directory (event log has no file events)
   - Line count: Always walks directory (no event-based tracking)
   - Test execution: Attempts to run tests directly (events not persisted)
   - Stub keywords: Always uses grep (no event-based tracking)
   - Git commits: Reads from event log OR falls back to `git log`

### Why This Design?

- **Minimal Disk I/O**: Only git commits (significant events) are persisted
- **Real-time Coordination**: FileWatcher and OutputParser provide in-memory events for real-time monitoring
- **Fallback-First Validation**: Validator doesn't depend on complete event log; uses direct verification
- **Simplicity**: No complex event log subscriber management

## Constraints and Assumptions

### Constraints

- **C-1**: Requires Linux/Unix system with fsnotify support
- **C-2**: Requires git CLI for commit tracking and fallback verification
- **C-3**: Requires grep for stub keyword detection
- **C-4**: Event log stored in system temp directory
- **C-5**: Event log contains ONLY git commit events (sparse persistence)

### Assumptions

- **A-1**: Sub-agent operates in a single work directory
- **A-2**: Sub-agent uses git for version control
- **A-3**: Test commands follow common conventions (go test, npm test, pytest)
- **A-4**: Source files use standard extensions (.go, .js, .py, etc.)
- **A-5**: Single monitor instance per sub-agent
- **A-6**: Validation fallback mechanisms (git log, file walking, test execution) are reliable

## Error Handling

### Error Categories

1. **Initialization Errors**: Invalid work directory, fsnotify unavailable
2. **Runtime Errors**: File watcher errors, git hook installation failures
3. **Validation Errors**: Event log corruption, git command failures, fallback execution failures

### Error Strategies

- **Best-effort monitoring**: Log warnings but continue execution
- **Fallback verification**: If events unavailable, run direct checks (git log, file count, run tests)
- **Graceful degradation**: Missing signals reduce score but don't fail validation
- **Sparse event log**: Validation works even with only git commit events in log

## Testing Requirements

### Unit Tests

- **T-1**: File watcher event detection and filtering
- **T-2**: Git hook installation, backup, and restoration
- **T-3**: Output parser pattern matching
- **T-4**: Validator signal calculation and scoring
- **T-5**: Event counting from JSONL log (git commits only)
- **T-6**: Fallback verification mechanisms (git log, file walking, test execution)

### Integration Tests

- **T-7**: End-to-end monitoring of simulated sub-agent
- **T-8**: Validation scoring with fake implementation
- **T-9**: Multi-signal validation with real project
- **T-10**: Validation with sparse event log (only git commits)
- **T-11**: Validation with missing event log (full fallback mode)

## Dependencies

- `github.com/fsnotify/fsnotify v1.7.0` - File system notifications
- `github.com/vbonnet/engram/core/pkg/eventbus` - In-memory event pub/sub
- `github.com/google/uuid` - Agent ID generation (transitive via eventbus)

## Future Considerations

- **F-1**: Support for additional test frameworks (JUnit, RSpec, etc.)
- **F-2**: Machine learning for fake implementation detection
- **F-3**: Code quality metrics (cyclomatic complexity, test coverage)
- **F-4**: Network activity monitoring (API calls, database queries)
- **F-5**: Resource usage tracking (CPU, memory, disk I/O)
- **F-6**: Distributed tracing for multi-agent workflows
- **F-7**: Full event log persistence (file subscriber for all EventBus events)
- **F-8**: Streaming validation (real-time validation during execution)
