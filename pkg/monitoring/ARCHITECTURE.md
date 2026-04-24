# Agent Monitoring - Architecture

## System Overview

The monitoring package implements a multi-layered approach to detecting fake implementations in sub-agent executions. It combines real-time file system monitoring, git hook integration, output parsing, and multi-signal validation to ensure sub-agents perform actual work rather than submitting stubs.

**Critical Design Characteristic**: The system uses a **sparse event persistence model** where only git commits are persisted to disk, while file and test events remain in-memory for real-time coordination. Validation relies heavily on fallback mechanisms rather than complete event logs.

## Architectural Principles

1. **Defense in Depth**: Multiple independent signals reduce false positives
2. **Best-Effort Monitoring**: Failures don't block sub-agent execution
3. **Fallback Verification**: Missing event data triggers direct checks
4. **Event-Driven Architecture**: Loose coupling via in-memory event bus
5. **Separation of Concerns**: Each component has a single responsibility
6. **Sparse Persistence**: Only significant events (git commits) written to disk
7. **Fallback-First Validation**: Validator designed to work without complete event logs

## Component Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                         AgentMonitor                            │
│  (Coordinates monitoring lifecycle and validation)              │
└──────┬──────────────────┬──────────────────┬────────────────────┘
       │                  │                  │
       ▼                  ▼                  ▼
┌─────────────┐    ┌─────────────┐    ┌─────────────┐
│ FileWatcher │    │ GitHookMgr  │    │OutputParser │
│  (fsnotify) │    │(post-commit)│    │  (regex)    │
│ IN-MEMORY   │    │ DISK WRITE  │    │ IN-MEMORY   │
└──────┬──────┘    └──────┬──────┘    └──────┬──────┘
       │                  │                  │
       │                  │                  │
       │  EventBus        │  Direct File     │  EventBus
       │  Publish         │  Append          │  Publish
       │  (in-memory)     │  (JSONL)         │  (in-memory)
       │                  │                  │
       └──────────┬───────┴──────────┬───────┘
                  ▼                  ▼
           ┌─────────────┐    ┌─────────────┐
           │  EventBus   │    │ Event Log   │
           │ (in-memory  │    │   (JSONL)   │
           │  pub/sub)   │    │ Git commits │
           │             │    │    ONLY     │
           └─────────────┘    └──────┬──────┘
                                     │
                                     ▼
                              ┌─────────────┐
                              │  Validator  │
                              │(multi-signal│
                              │  +fallback) │
                              └─────────────┘
```

## Data Flow Diagrams

### Monitoring Phase - Event Generation

```
Sub-Agent Activity
    │
    ├─→ File Operations
    │   └→ fsnotify detects change
    │       └→ FileWatcher filters & publishes
    │           └→ EventBus (IN-MEMORY ONLY)
    │               └→ [Event discarded after processing]
    │
    ├─→ Git Commit
    │   └→ post-commit hook executes
    │       └→ Bash script extracts metadata
    │           └→ cat >> event.jsonl (DISK WRITE)
    │               └→ [Event persisted to JSONL file]
    │
    └─→ stdout/stderr output
        └→ OutputParser matches patterns
            └→ EventBus (IN-MEMORY ONLY)
                └→ [Event discarded after processing]
```

### Validation Phase - Data Sources

```
Validator.Validate()
    │
    ├─→ Git Commits Signal
    │   ├─→ Primary: Read JSONL log (git commits present)
    │   └─→ Fallback: exec `git log --oneline`
    │
    ├─→ File Count Signal
    │   ├─→ Primary: Read JSONL log (NO file events present)
    │   └─→ Fallback: filepath.Walk() directory (ALWAYS USED)
    │
    ├─→ Line Count Signal
    │   └─→ Only: filepath.Walk() + count lines (NO event tracking)
    │
    ├─→ Test Execution Signal
    │   ├─→ Primary: Read JSONL log (NO test events present)
    │   └─→ Fallback: exec tests (go test, npm test, pytest)
    │
    └─→ Stub Keywords Signal
        └─→ Only: exec `grep -r -E "TODO|FIXME|..."` (NO event tracking)
```

## Component Details

### 1. AgentMonitor (Coordinator)

**Responsibility**: Orchestrate monitoring lifecycle and coordinate components

**Key Responsibilities**:
- Initialize and configure all monitoring components
- Manage start/stop lifecycle
- Coordinate event collection and validation
- Provide unified API for external callers

**State Management**:
```go
type AgentMonitor struct {
    AgentID      string              // Unique identifier
    WorkDir      string              // Directory being monitored
    EventLogPath string              // JSONL event log path (sparse - git commits only)
    Config       ValidationConfig    // Validation thresholds

    fileWatcher  *FileWatcher        // File system monitor (in-memory events)
    gitHooks     *GitHookManager     // Git hook installer (disk persistence)
    outputParser *OutputParser       // Output line parser (in-memory events)
    validator    *Validator          // Multi-signal validator (fallback-heavy)
    eventBus     *eventbus.Bus       // In-memory event publisher

    started      bool                // Monitoring active flag
    startTime    time.Time           // Execution start time
    stopTime     time.Time           // Execution end time
    ctx          context.Context     // Cancellation context
    cancel       context.CancelFunc  // Cancel function
}
```

**Lifecycle**:
1. **Construction**: `NewAgentMonitor()` creates components, initializes event bus
2. **Start**: Activates file watcher, installs git hooks, publishes start event
3. **Runtime**: Components publish events independently (FileWatcher/OutputParser in-memory, git hook to disk)
4. **Stop**: Deactivates watcher, uninstalls hooks, publishes completion event
5. **Validation**: Validator reads sparse event log and runs fallback verification

**Design Decisions**:
- Best-effort initialization: Component failures logged as warnings
- Context-based cancellation for clean shutdown
- Event log in temp directory to avoid workspace pollution
- Sparse event log accepted as design constraint

### 2. FileWatcher (Real-Time Monitoring)

**Responsibility**: Monitor file system changes using fsnotify

**Critical Characteristic**: Events published to EventBus for in-memory coordination ONLY. No disk persistence.

**Architecture**:
```go
type FileWatcher struct {
    agentID  string              // Agent identifier
    workDir  string              // Watched directory
    watcher  *fsnotify.Watcher   // fsnotify instance
    eventBus *eventbus.Bus       // Event publisher (in-memory)
    filters  []FileFilter        // Path filters
    ctx      context.Context     // Cancellation context
    cancel   context.CancelFunc  // Cancel function
}
```

**Event Processing Pipeline**:
```
fsnotify.Event → Filter Check → Event Type Mapping → EventBus.Publish (in-memory) → [discarded]
```

**Filtering Strategy**:
- Predefined filters: IsGitFile, IsTempFile, IsIDEFile
- Extensible via AddFilter()
- Filter on path, not file content (performance)

**Event Types Mapped**:
- `fsnotify.Create` → `sub_agent.file.created` (in-memory only)
- `fsnotify.Write` → `sub_agent.file.modified` (in-memory only)
- `fsnotify.Remove` → `sub_agent.file.deleted` (in-memory only)
- `fsnotify.Rename` → `sub_agent.file.deleted` (in-memory only)
- `fsnotify.Chmod` → (ignored)

**Concurrency Model**:
- Single goroutine processes all fsnotify events (watchLoop)
- Context-based cancellation on Stop()
- Non-blocking event publishing

**Error Handling**:
- inotify limit errors reported with remediation hint
- Watcher errors logged but monitoring continues
- Graceful shutdown on context cancellation

**Persistence**: NONE - events remain in-memory only

### 3. GitHookManager (Commit Tracking)

**Responsibility**: Install and manage post-commit git hooks

**Critical Characteristic**: The ONLY component that persists events to disk (JSONL file).

**Architecture**:
```go
type GitHookManager struct {
    agentID   string  // Agent identifier
    repoPath  string  // Git repository path
    eventFile string  // Event log path (JSONL)
}
```

**Hook Installation Flow**:
```
1. Check .git/hooks/ directory exists
2. Backup existing post-commit hook (if present)
3. Generate hook script from template
4. Write hook with 0755 permissions
```

**Hook Template Strategy**:
- Bash script generated from Go template
- Captures git metadata: commit hash, message, author, email
- Extracts stats: files changed, insertions, deletions
- **Appends JSONL event directly to file via `cat >>`**
- Self-contained: no external dependencies

**Hook Script Structure**:
```bash
#!/bin/bash
# Auto-generated by engram monitoring - DO NOT EDIT
AGENT_ID="{{.AgentID}}"
EVENT_FILE="{{.EventFile}}"

# Extract git metadata
COMMIT=$(git rev-parse HEAD)
MESSAGE=$(git log -1 --pretty=%B | head -1)
AUTHOR=$(git log -1 --pretty=%an)
EMAIL=$(git log -1 --pretty=%ae)
TIMESTAMP=$(date -Iseconds)
FILES_CHANGED=$(git diff-tree --no-commit-id --name-only -r HEAD | wc -l)
STATS=$(git diff HEAD~ HEAD --shortstat)
INSERTIONS=$(echo "$STATS" | grep -oE '[0-9]+ insertion' | grep -oE '[0-9]+' || echo 0)
DELETIONS=$(echo "$STATS" | grep -oE '[0-9]+ deletion' | grep -oE '[0-9]+' || echo 0)

# Write JSONL event directly to file (ONLY disk persistence in system)
cat >> "$EVENT_FILE" <<EOF
{"event_type":"sub_agent.git.commit","agent_id":"$AGENT_ID","timestamp":"$TIMESTAMP","data":{"commit":"$COMMIT","message":"$MESSAGE","author":"$AUTHOR","email":"$EMAIL","files_changed":$FILES_CHANGED,"insertions":$INSERTIONS,"deletions":$DELETIONS}}
EOF
```

**Uninstallation Flow**:
```
1. Remove installed post-commit hook
2. Restore .backup file (if exists)
3. Clean up backup file
```

**Design Decisions**:
- Backup strategy prevents data loss
- JSONL format for append-only event log
- Template-based generation for maintainability
- Marker comment for hook detection
- **Direct file write bypasses EventBus** (simplicity, reliability)

**Persistence**: FULL - all git commit events written to JSONL file

### 4. OutputParser (Command Detection)

**Responsibility**: Parse sub-agent stdout/stderr to detect test execution

**Critical Characteristic**: Events published to EventBus for in-memory coordination ONLY. No disk persistence.

**Architecture**:
```go
type OutputParser struct {
    agentID  string             // Agent identifier
    eventBus *eventbus.Bus      // Event publisher (in-memory)
    patterns []CommandPattern   // Regex patterns
}

type CommandPattern struct {
    Name        string                                    // Pattern name
    Regex       *regexp.Regexp                            // Compiled regex
    EventType   string                                    // Event topic
    ExtractData func([]string) map[string]interface{}    // Data extractor
}
```

**Pattern Matching Strategy**:
- Sequential pattern matching (first match wins)
- Framework-specific patterns for Go, npm, pytest
- Extract structured data from matches
- Publish events with agent_id, timestamp, raw_output

**Supported Patterns**:

| Framework | Pattern | Event Type | Extracted Data | Persisted? |
|-----------|---------|------------|----------------|------------|
| Go test | `go test` | `sub_agent.test.started` | command | NO (in-memory) |
| Go result | `^(ok\|FAIL)\s+(\S+)\s+([\d.]+s)` | `sub_agent.test.passed` | status, package, duration | NO (in-memory) |
| npm test | `npm test` | `sub_agent.test.started` | command | NO (in-memory) |
| Jest summary | `Tests:\s+(\d+) passed` | `sub_agent.test.passed` | passed, failed, total | NO (in-memory) |
| pytest | `pytest` | `sub_agent.test.started` | command | NO (in-memory) |
| pytest summary | `=+\s+(\d+)\s+passed` | `sub_agent.test.passed` | passed, duration | NO (in-memory) |
| git commit | `git commit` | `sub_agent.command.git_commit` | command | NO (in-memory) |

**Extensibility**:
- AddPattern() for custom patterns
- Data extraction via callback functions
- Supports multi-line output via bufio.Scanner

**Design Decisions**:
- Regex over AST parsing for performance
- Framework detection via command output, not config files
- First-match strategy prevents duplicate events

**Persistence**: NONE - events remain in-memory only

### 5. Validator (Multi-Signal Scoring)

**Responsibility**: Aggregate signals and validate sub-agent output quality

**Critical Characteristic**: Designed to work with sparse event log (only git commits). Heavily relies on fallback mechanisms.

**Architecture**:
```go
type Validator struct {
    agentID  string             // Agent identifier
    workDir  string             // Work directory
    eventLog string             // Event log path (sparse - git commits only)
    config   ValidationConfig   // Thresholds
}
```

**Validation Pipeline**:
```
1. Run 5 validation signals (most use fallbacks)
2. Calculate weighted score
3. Compare to threshold
4. Generate summary
5. Return structured result
```

**Signal Implementations**:

#### Signal 1: Git Commits (Weight: 0.2)
```
Primary: Count sub_agent.git.commit events in JSONL log
         SUCCESS - git hooks write commits to log
Fallback: Parse `git log --oneline` output
Threshold: MinCommitCount (default: 2)
Rationale: Real work generates multiple commits
Data Source: JSONL log (reliable) OR git CLI (fallback)
```

#### Signal 2: File Count (Weight: 0.2)
```
Primary: Count sub_agent.file.created events in JSONL log
         FAILS - FileWatcher doesn't persist events
Fallback: Walk directory, count source files
         ALWAYS USED - no persisted file events
Threshold: MinFileCount (default: 3)
Rationale: Implementations span multiple files
Data Source: filepath.Walk() (always used)
```

#### Signal 3: Line Count (Weight: 0.2)
```
Primary: Walk directory, sum lines in source files
         ONLY OPTION - no event-based tracking
Filters: Exclude .git/, node_modules/, IDE files
Threshold: MinLineCount (default: 50)
Rationale: Substantial implementations have meaningful LOC
Data Source: filepath.Walk() + bufio.Scanner (only option)
```

#### Signal 4: Test Execution (Weight: 0.3)
```
Primary: Count sub_agent.test.started events in JSONL log
         FAILS - OutputParser doesn't persist events
Secondary: Count sub_agent.test.passed events in JSONL log
           FAILS - OutputParser doesn't persist events
Fallback: Run tests (go test, npm test, pytest)
         TYPICALLY USED - no persisted test events
Threshold: MinTestRuns (default: 1)
Rationale: Quality implementations include tests
Data Source: exec.Command() running tests (typically used)
```

#### Signal 5: Stub Keywords (Weight: 0.1)
```
Primary: grep -r -E "TODO|FIXME|NotImplemented|panic(\"not implemented\")"
         ONLY OPTION - no event-based tracking
Threshold: MaxStubKeywords (default: 3)
Rationale: Stubs indicate incomplete work
Data Source: exec.Command("grep", ...) (only option)
```

**Scoring Algorithm**:
```go
score := 0.0
for _, signal := range signals {
    if signal.Passed {
        score += signal.Weight
    }
}
passed := score >= config.PassThreshold
```

**Fallback Strategy**:
- Event log missing → run direct verification
- Event log sparse (normal case) → use fallbacks for most signals
- Git commits present in log → use log data
- File/test events absent from log → always use fallbacks
- Git commands fail → skip signal (reduces score)
- Test execution fails → counts as 0 test runs

**Source File Detection**:
- Extensions: .go, .js, .ts, .py, .java, .c, .cpp, .h, .rb, .rs
- Excludes: .git/, node_modules/, __pycache__, .vscode/, .idea/

**Design Decisions**:
- Higher weight for test execution (0.3) - strongest signal
- Lower weight for stubs (0.1) - can have legitimate TODOs
- Fallback verification ensures reliability despite sparse event log
- Conservative threshold (0.6) balances false positives/negatives
- **Validator doesn't rely on complete event log** - design choice

### 6. EventBus (Decoupling Layer)

**Responsibility**: In-memory publish/subscribe event distribution

**Critical Characteristic**: NO disk persistence. Events exist only in memory for real-time coordination.

**Integration**:
- Provided by `github.com/vbonnet/engram/core/pkg/eventbus`
- Components publish events via `bus.Publish(ctx, event)`
- **NO file subscriber** - events NOT persisted to disk by EventBus
- FileWatcher and OutputParser use EventBus for in-memory coordination only

**Event Structure**:
```go
type Event struct {
    Topic     string                 // Event type (e.g., "sub_agent.file.created")
    Publisher string                 // Component name
    Data      map[string]interface{} // Event payload
}
```

**Event Lifetime**:
- Published by FileWatcher/OutputParser
- Delivered to subscribers (if any)
- **Discarded after delivery** - not persisted

**Event Log Format** (JSONL) - Git Commits ONLY:
```json
{"event_type":"sub_agent.git.commit","agent_id":"agent-123","timestamp":"2024-01-01T12:01:00Z","data":{"commit":"abc123","message":"Initial commit","author":"John Doe","email":"john@example.com","files_changed":3,"insertions":42,"deletions":5}}
```

**Design Decisions**:
- JSONL for append-only, line-delimited events
- Temp directory for event logs (cleanup on reboot)
- Synchronous publishing (simplicity over performance)
- **No automatic file subscriber** - reduces complexity
- **Git hooks bypass EventBus** - write directly to file for reliability

## Sparse Persistence Architecture

### What Gets Persisted?

**JSONL Event Log Contains**:
- Git commit events (written by git hook bash script)
- Agent start/stop events (if future enhancement adds persistence)

**JSONL Event Log Does NOT Contain**:
- File creation/modification/deletion events (in-memory only)
- Test start/pass/fail events (in-memory only)
- Command execution events (in-memory only)

### Why Sparse Persistence?

**Advantages**:
1. **Minimal Disk I/O**: Only significant events (commits) persisted
2. **Simplicity**: No complex event log subscriber management
3. **Reliability**: Git hooks write directly to file (no EventBus dependency)
4. **Performance**: In-memory events for real-time coordination without disk overhead

**Trade-offs**:
1. **Validation depends on fallbacks**: Most signals use direct verification
2. **Statistics are sparse**: GetStats() mostly counts git commits from log
3. **No historical record**: File/test events lost after monitoring session

**Design Philosophy**: The system prioritizes simplicity and reliability over comprehensive event logging. Validation is designed to work with minimal event data, using fallback mechanisms as primary validation methods.

## Concurrency Model

### Goroutines

1. **FileWatcher.watchLoop()**: Single goroutine processes fsnotify events
2. **OutputParser**: Caller-provided goroutine for ParseOutput()
3. **EventBus**: Synchronous publishing (no dedicated goroutines)
4. **GitHook**: Runs in git's post-commit subprocess (separate process)

### Synchronization

- Context-based cancellation for clean shutdown
- No shared mutable state between components
- Event log uses append-only writes (no locks needed)
- FileWatcher/OutputParser publish to EventBus asynchronously
- Git hook writes to file directly (no synchronization with Go process)

### Thread Safety

- AgentMonitor: Not thread-safe (single caller expected)
- FileWatcher: Thread-safe (internal goroutine + context)
- EventBus: Thread-safe (internal locking)
- Validator: Thread-safe (read-only access to event log)
- GitHook: Thread-safe (separate process, append-only file writes)

## Configuration Management

### Preset Configurations

```go
// DefaultValidationConfig: Balanced for medium tasks
MinFileCount: 3, MinLineCount: 50, MinCommitCount: 2,
MinTestRuns: 1, MaxStubKeywords: 3, PassThreshold: 0.6

// SimpleTaskConfig: Relaxed for trivial tasks
MinFileCount: 1, MinLineCount: 10, MinCommitCount: 1,
MinTestRuns: 0, MaxStubKeywords: 5, PassThreshold: 0.5

// ComplexTaskConfig: Strict for multi-component features
MinFileCount: 5, MinLineCount: 200, MinCommitCount: 5,
MinTestRuns: 2, MaxStubKeywords: 0, PassThreshold: 0.7
```

### Configuration Strategy

- Immutable configs (no runtime modification)
- SetValidationConfig() recreates validator
- Caller selects config based on task complexity

## Error Handling Strategy

### Error Categories

1. **Initialization Errors**: Fatal (return error from NewAgentMonitor)
2. **Monitoring Errors**: Non-fatal (log warning, continue)
3. **Validation Errors**: Non-fatal (fallback verification)

### Error Recovery

```
FileWatcher fails → Log warning, continue without file monitoring
GitHooks fail → Log warning, continue without commit tracking (validation uses git log)
Event log missing → Use fallback verification (git log, file count, run tests)
Event log sparse (normal) → Use fallback verification (expected behavior)
Test execution fails → Count as 0 test runs (reduces score)
Git CLI fails → Skip signal (reduces score)
```

### Design Philosophy

**Best-Effort Monitoring**: Partial monitoring is better than no monitoring. Components fail independently without cascading failures. Sparse event log is accepted as normal operation mode.

## Performance Characteristics

### Time Complexity

- File watching: O(1) per event
- Event log parsing: O(n) where n = number of events (typically small - git commits only)
- Directory walking: O(f) where f = number of files (used for most signals)
- Git log parsing: O(c) where c = number of commits
- Test execution: O(t) where t = test suite duration (potentially slow)

### Space Complexity

- Event log: O(c) where c = number of commits (sparse - no file/test events)
- In-memory state: O(1) (no event buffering)

### Bottlenecks

- Directory walking for line count (large projects)
- grep for stub keywords (large codebases)
- Fallback test execution (slow test suites) - **frequently used**
- File count via directory walk (large projects) - **always used**

### Optimizations

- Filter files before counting lines
- Use git log --oneline for commit count
- Skip fallback test execution if MinTestRuns = 0
- Sparse event log reduces disk I/O (only git commits)

## Security Considerations

### Attack Vectors

1. **Event Log Tampering**: Sub-agent could modify event log
   - Mitigation: Event log in temp directory with restricted permissions
   - Mitigation: Validator uses fallback verification (git log, file walk, test execution)
   - Impact: Limited - event log is sparse; most validation uses fallbacks anyway

2. **Git Hook Bypass**: Sub-agent could uninstall hook
   - Mitigation: Fallback to `git log` verification
   - Impact: Minimal - git commit signal uses fallback automatically

3. **Output Spoofing**: Sub-agent could fake test output
   - Mitigation: Fallback test execution (actually run tests)
   - Impact: Minimal - OutputParser events not persisted; validator runs tests directly

4. **Resource Exhaustion**: Sub-agent creates millions of files
   - Mitigation: Event processing is async, validation has timeouts
   - Impact: FileWatcher events in-memory only (not persisted)
   - Future: Rate limiting, file count limits

### Security Best Practices

- Event logs in secure temp directories (mode 0600)
- Git hooks executable only by owner (mode 0755)
- No sensitive data in events (paths only, no content)
- Validation runs in sub-agent's security context
- Sparse event log reduces attack surface (less data to tamper)

## Testing Strategy

### Unit Tests

- Mock fsnotify for FileWatcher tests
- Template rendering for GitHookManager tests
- Pattern matching for OutputParser tests
- Signal calculation for Validator tests
- Fallback mechanism tests (git log, file walk, test execution)

### Integration Tests

- Create temp git repository
- Simulate sub-agent activity (file creation, commits)
- Verify event detection and validation scoring
- Test validation with sparse event log (only git commits)
- Test validation with missing event log (full fallback mode)

### Test Coverage

- Critical paths: 100% (validation logic, scoring)
- Error handling: 80% (fallback strategies)
- Edge cases: 90% (empty directories, no git, sparse event log)

## Deployment Considerations

### System Requirements

- Linux/Unix with fsnotify support
- Git CLI available in PATH
- grep command available
- Write access to /tmp

### Resource Requirements

- Memory: ~10MB per monitor instance (minimal - no event buffering)
- Disk: Event log size ~1KB per 10 commits (sparse - git commits only)
- CPU: Negligible (async event processing, sparse persistence)

### Monitoring Limits

- inotify watches: Limited by fs.inotify.max_user_watches
- Concurrent monitors: No hard limit (resource-dependent)
- Event log retention: Cleaned on reboot (temp directory)
- Event log size: Small (sparse persistence - git commits only)

## Future Architecture Enhancements

### Planned Improvements

1. **Full Event Persistence**: Add file subscriber to EventBus for complete event logging
2. **Streaming Validation**: Real-time validation during execution using in-memory events
3. **Machine Learning**: Fake implementation detection via ML models
4. **Distributed Monitoring**: Multi-agent workflow tracking
5. **Code Quality Metrics**: Cyclomatic complexity, test coverage
6. **Network Monitoring**: API calls, database queries
7. **Resource Tracking**: CPU, memory, disk I/O profiling

### Extensibility Points

- FileFilter interface for custom file filtering
- CommandPattern for custom output parsing
- ValidationSignal for additional validation dimensions
- EventBus subscribers for custom event processing (in-memory)
- Future: EventBus file subscriber for full event persistence

## Dependencies

### Direct Dependencies

- `github.com/fsnotify/fsnotify v1.7.0` - File system notifications
- `github.com/vbonnet/engram/core/pkg/eventbus` - In-memory event bus (NO persistence)

### Transitive Dependencies

- `github.com/google/uuid` - UUID generation (via eventbus)

### System Dependencies

- git CLI (commit tracking, fallback verification)
- grep (stub keyword detection)
- Platform-specific test runners (go, npm, pytest) - used by fallback validation

## Architectural Decision Records

### ADR-001: Sparse Event Persistence

**Decision**: Only git commit events are persisted to disk. FileWatcher and OutputParser events remain in-memory only.

**Rationale**:
- Simplicity: No complex event log subscriber management
- Performance: Minimal disk I/O
- Reliability: Git hooks write directly to file (no EventBus dependency)
- Validator design: Fallback verification works reliably without complete event log

**Consequences**:
- Validation primarily uses fallback mechanisms (git log, file walk, test execution)
- GetStats() returns sparse data (mostly git commits)
- No historical record of file/test events
- Reduced attack surface (less data to tamper)

**Status**: Accepted as core design principle

### ADR-002: Fallback-First Validation

**Decision**: Validator designed to work without complete event logs, using fallback mechanisms as primary validation methods.

**Rationale**:
- Sparse event log (only git commits) is intentional design
- Direct verification (git log, file walk, test execution) is reliable
- Reduces dependency on event log completeness
- Simplifies implementation (no complex event log management)

**Consequences**:
- Validation time may be longer (directory walking, test execution)
- More reliable than event-only validation (handles missing events gracefully)
- Validator works even with empty event log

**Status**: Accepted as core design principle

### ADR-003: Git Hooks Bypass EventBus

**Decision**: Git hooks write directly to JSONL file, bypassing EventBus.

**Rationale**:
- Reliability: No dependency on Go process or EventBus
- Simplicity: Bash script with direct file append
- Performance: No EventBus overhead
- Isolation: Git hook runs in separate process

**Consequences**:
- Git commit events guaranteed to be persisted
- No EventBus coordination for git commits
- Event log format must be JSONL (not binary)

**Status**: Accepted as core design principle
