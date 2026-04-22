# Agent Monitoring - Architectural Decision Records

## ADR-001: Multi-Signal Validation Strategy

**Status**: Accepted

**Context**:
We need to detect fake implementations where sub-agents submit stubs or incomplete work. Relying on a single signal (e.g., only checking if tests run) creates easy circumvention paths.

**Decision**:
Implement multi-signal validation with weighted scoring across 5 independent signals:
1. Git commits (weight: 0.2)
2. File count (weight: 0.2)
3. Line count (weight: 0.2)
4. Test execution (weight: 0.3)
5. Stub keywords (weight: 0.1)

Pass threshold: 0.6 (60% of maximum score)

**Rationale**:
- **Defense in depth**: Multiple signals harder to fake than single signal
- **Weighted importance**: Test execution (0.3) is strongest quality indicator
- **Balanced thresholds**: 60% prevents false negatives while catching obvious fakes
- **No silver bullet**: Even if one signal fails, others can compensate

**Consequences**:
- **Positive**: Robust against sophisticated fakes, flexible scoring
- **Negative**: More complex validation logic, potential false positives
- **Mitigation**: Configurable thresholds (SimpleTaskConfig, ComplexTaskConfig)

**Alternatives Considered**:
1. **Single-signal validation** (e.g., only test execution)
   - Rejected: Too easy to circumvent
2. **Boolean AND of all signals** (all must pass)
   - Rejected: Too strict, many false negatives
3. **Machine learning classifier**
   - Deferred: Requires training data, adds complexity

---

## ADR-002: Best-Effort Monitoring Philosophy

**Status**: Accepted

**Context**:
Monitoring components can fail for various reasons: inotify limits, git repository not initialized, permission errors, etc. Should monitoring failures block sub-agent execution?

**Decision**:
Adopt best-effort monitoring philosophy:
- Component initialization failures are logged as warnings, not errors
- Monitoring continues with degraded capabilities
- Validation uses fallback verification when event data is missing
- Sub-agent execution is never blocked by monitoring failures

**Rationale**:
- **Primary goal**: Sub-agent produces useful output, monitoring is secondary
- **Partial data valuable**: Even incomplete monitoring provides insights
- **Graceful degradation**: Fallback verification ensures validation still works
- **Developer experience**: Monitoring shouldn't require perfect environment setup

**Consequences**:
- **Positive**: Robust to environment variations, better DX
- **Negative**: Silent failures possible if warnings not monitored
- **Mitigation**: Fallback verification, clear warning messages with remediation

**Example**:
```go
// File watcher fails (inotify limit)
if err := am.fileWatcher.Start(); err != nil {
    fmt.Printf("Warning: file watcher failed to start: %v\n", err)
    // Continue execution, fallback to directory walking in validation
}
```

**Alternatives Considered**:
1. **Strict monitoring** (fail fast on errors)
   - Rejected: Too brittle, poor developer experience
2. **Silent failures** (no warnings)
   - Rejected: Makes debugging impossible

---

## ADR-003: Fallback Verification Strategy

**Status**: Accepted

**Context**:
Event log can be missing or corrupted. Git hooks may not fire. File watcher may not detect all events. How do we ensure validation reliability?

**Decision**:
Implement three-tier fallback strategy for each signal:

**Tier 1 (Primary)**: Event log parsing
- Fast, captures real-time events
- Example: Count `sub_agent.git.commit` events

**Tier 2 (Secondary)**: Direct verification
- Slower, but authoritative
- Example: Parse `git log --oneline` output

**Tier 3 (Fallback)**: Best-effort approximation
- May not be perfect, but provides some signal
- Example: Run tests ourselves if no test events detected

**Rationale**:
- **Reliability**: Validation works even with monitoring failures
- **Performance**: Primary path is fast (event log parsing)
- **Accuracy**: Fallback to authoritative sources when needed
- **Flexibility**: Can add new fallback strategies per signal

**Consequences**:
- **Positive**: Highly reliable validation, works in degraded scenarios
- **Negative**: Slower validation when fallbacks are used
- **Mitigation**: Optimize fallback paths, cache results

**Example Implementation**:
```go
// Git commits signal
commitCount := v.countEventsByType(EventGitCommit)  // Tier 1

if commitCount == 0 {  // Tier 2
    cmd := exec.Command("git", "log", "--oneline")
    output, err := cmd.Output()
    if err == nil {
        commitCount = len(strings.Split(string(output), "\n"))
    }
}

// No Tier 3 for git commits (can't infer from other data)
```

**Alternatives Considered**:
1. **Event log only** (no fallbacks)
   - Rejected: Too brittle, fails with monitoring issues
2. **Always use direct verification** (no event log)
   - Rejected: Slower, misses real-time signals

---

## ADR-004: fsnotify for File Monitoring

**Status**: Accepted

**Context**:
Need real-time file system monitoring to detect file creation/modification. Options: polling, fsnotify (inotify/kqueue wrapper), OS-specific APIs.

**Decision**:
Use fsnotify library for file system monitoring.

**Rationale**:
- **Cross-platform**: Works on Linux (inotify), macOS (kqueue), Windows (ReadDirectoryChangesW)
- **Performance**: Kernel-level notifications, no polling overhead
- **Mature library**: fsnotify v1.7.0 is stable, widely used
- **Go-native**: Idiomatic Go API, goroutine-friendly
- **Event granularity**: Detects Create, Write, Remove, Rename, Chmod

**Consequences**:
- **Positive**: Real-time events, low CPU usage, battle-tested
- **Negative**: inotify watch limits on Linux, requires error handling
- **Mitigation**: Detect inotify errors, provide remediation hints

**Alternatives Considered**:
1. **Polling** (periodic directory scans)
   - Rejected: High latency, CPU overhead, miss rapid changes
2. **OS-specific APIs** (inotify directly)
   - Rejected: Not cross-platform, more complex
3. **fanotify** (Linux-specific)
   - Rejected: Requires CAP_SYS_ADMIN, not suitable for user-space

**Technical Details**:
```go
watcher, err := fsnotify.NewWatcher()
if err != nil {
    return nil, fmt.Errorf("failed to create fsnotify watcher: %w", err)
}

if err := watcher.Add(workDir); err != nil {
    if strings.Contains(err.Error(), "no space left on device") {
        // Provide remediation hint
        return fmt.Errorf("inotify watch limit reached (increase with: sysctl fs.inotify.max_user_watches=524288)")
    }
}
```

---

## ADR-005: Git Post-Commit Hooks for Commit Tracking

**Status**: Accepted

**Context**:
Need to detect git commits made by sub-agent. Options: git log polling, post-commit hooks, git event stream API.

**Decision**:
Install post-commit git hooks that append events to JSONL log file.

**Rationale**:
- **Real-time detection**: Hook fires immediately after commit
- **Rich metadata**: Access to commit hash, message, author, diff stats
- **No polling**: Event-driven, no performance overhead
- **Reliable**: Git guarantees hook execution
- **Simple**: Bash script, no external dependencies

**Consequences**:
- **Positive**: Real-time, accurate, rich metadata
- **Negative**: Requires .git repository, hook conflicts possible
- **Mitigation**: Backup existing hooks, uninstall on cleanup

**Hook Design**:
```bash
#!/bin/bash
# Auto-generated by engram monitoring - DO NOT EDIT
AGENT_ID="agent-123"
EVENT_FILE="/tmp/sub-agent-monitor-agent-123.jsonl"

COMMIT=$(git rev-parse HEAD)
MESSAGE=$(git log -1 --pretty=%B | head -1)
AUTHOR=$(git log -1 --pretty=%an)
FILES_CHANGED=$(git diff-tree --no-commit-id --name-only -r HEAD | wc -l)

cat >> "$EVENT_FILE" <<EOF
{"event_type":"sub_agent.git.commit","agent_id":"$AGENT_ID","timestamp":"$(date -Iseconds)","data":{"commit":"$COMMIT","message":"$MESSAGE","author":"$AUTHOR","files_changed":$FILES_CHANGED}}
EOF
```

**Alternatives Considered**:
1. **Polling git log**
   - Rejected: High latency, misses rapid commits, CPU overhead
2. **libgit2 bindings** (e.g., git2go)
   - Rejected: Adds CGO dependency, complex API
3. **Git event stream API**
   - Rejected: Not available in standard git
4. **Pre-commit hook**
   - Rejected: Fires before commit completes, can't capture commit hash

**Hook Management**:
- Install: Backup existing post-commit → Write generated hook
- Uninstall: Remove generated hook → Restore backup
- Detection: Check for marker comment "Auto-generated by engram monitoring"

---

## ADR-006: Regex-Based Output Parsing

**Status**: Accepted

**Context**:
Need to detect test execution and other commands from sub-agent stdout/stderr. Options: regex patterns, AST parsing, log structured output.

**Decision**:
Use regex patterns to match command output and extract structured data.

**Rationale**:
- **Simplicity**: Regex is well-understood, easy to maintain
- **Performance**: Fast pattern matching, no parsing overhead
- **Flexibility**: Different patterns for different frameworks
- **Extensibility**: AddPattern() for custom patterns
- **No sub-agent changes**: Works with any output format

**Consequences**:
- **Positive**: Simple, fast, framework-agnostic
- **Negative**: Brittle if output format changes, limited multiline support
- **Mitigation**: Multiple patterns per framework, regex escaping

**Pattern Examples**:
```go
// Go test start
{
    Regex:     regexp.MustCompile(`go test`),
    EventType: EventTestStarted,
}

// Go test result
{
    Regex: regexp.MustCompile(`^(ok|FAIL)\s+(\S+)\s+([\d.]+s)`),
    EventType: EventTestPassed,
    ExtractData: func(matches []string) map[string]interface{} {
        return map[string]interface{}{
            "status":   matches[1],
            "package":  matches[2],
            "duration": matches[3],
        }
    },
}
```

**Alternatives Considered**:
1. **AST parsing** (parse test framework output as structured data)
   - Rejected: Requires framework-specific parsers, high complexity
2. **Require structured logging** (force sub-agents to log JSON)
   - Rejected: Intrusive, limits sub-agent flexibility
3. **Machine learning** (classify output lines)
   - Rejected: Overkill for deterministic patterns

**Extensibility**:
```go
parser.AddPattern(CommandPattern{
    Name:      "custom_test_runner",
    Regex:     regexp.MustCompile(`my-test-runner started`),
    EventType: EventTestStarted,
    ExtractData: func(matches []string) map[string]interface{} {
        return map[string]interface{}{"command": matches[0]}
    },
})
```

---

## ADR-007: JSONL Event Log Format

**Status**: Accepted

**Context**:
Need persistent event storage for validation. Options: JSON array, JSONL, SQLite, in-memory only.

**Decision**:
Use JSONL (newline-delimited JSON) format for event log.

**Rationale**:
- **Append-only**: No file locking, safe for concurrent writes
- **Line-delimited**: Easy to parse (one event per line)
- **Human-readable**: Can inspect with cat, grep, jq
- **Streaming-friendly**: Process events incrementally
- **No schema**: Flexible event structure

**Consequences**:
- **Positive**: Simple, fast, debuggable, no dependencies
- **Negative**: No indexing, linear scan for queries
- **Mitigation**: Small event volumes (< 1000 events per agent)

**Format Example**:
```json
{"event_type":"sub_agent.started","agent_id":"agent-123","timestamp":"2024-01-01T12:00:00Z","data":{"workdir":"/tmp/agent-123"}}
{"event_type":"sub_agent.file.created","agent_id":"agent-123","timestamp":"2024-01-01T12:00:01Z","data":{"path":"/tmp/agent-123/main.go"}}
{"event_type":"sub_agent.git.commit","agent_id":"agent-123","timestamp":"2024-01-01T12:00:05Z","data":{"commit":"abc123","message":"Initial commit"}}
```

**Alternatives Considered**:
1. **JSON array** (entire log is one JSON array)
   - Rejected: Requires file locking, can't append safely
2. **SQLite database**
   - Rejected: Overkill for small event volumes, adds dependency
3. **In-memory only** (no persistence)
   - Rejected: Can't inspect events post-execution, lost on crash
4. **Protocol Buffers**
   - Rejected: Not human-readable, requires schema

**File Location**:
```go
eventLogPath := filepath.Join(os.TempDir(), fmt.Sprintf("sub-agent-monitor-%s.jsonl", agentID))
```
- Temp directory: Automatic cleanup on reboot
- Agent ID in filename: Avoid collisions

---

## ADR-008: EventBus for Component Decoupling

**Status**: Accepted

**Context**:
Components (FileWatcher, OutputParser) need to publish events. Options: direct calls to event log, shared channel, event bus pattern.

**Decision**:
Use EventBus (publish/subscribe pattern) to decouple event producers from consumers.

**Rationale**:
- **Decoupling**: Components don't know about event log
- **Testability**: Can mock EventBus in tests
- **Extensibility**: New subscribers without changing producers
- **Existing library**: Reuse `engram/core/pkg/eventbus`
- **Thread-safe**: Built-in synchronization

**Consequences**:
- **Positive**: Clean separation, easy testing, extensible
- **Negative**: Indirect call path, harder to trace
- **Mitigation**: Clear topic naming, event logging for debugging

**Usage**:
```go
// Producer (FileWatcher)
fw.eventBus.Publish(context.Background(), &eventbus.Event{
    Topic:     EventFileCreated,
    Publisher: "file-watcher",
    Data: map[string]interface{}{
        "agent_id": fw.agentID,
        "path":     event.Name,
    },
})

// Consumer (Event Log Writer - in EventBus)
// Automatically writes to JSONL file
```

**Alternatives Considered**:
1. **Direct calls to event log**
   - Rejected: Tight coupling, hard to test
2. **Shared channel**
   - Rejected: Requires explicit wiring, less flexible
3. **Observer pattern**
   - Rejected: More complex than EventBus, less idiomatic in Go

**Event Topics**:
- `sub_agent.started` / `sub_agent.completed`
- `sub_agent.file.created` / `modified` / `deleted`
- `sub_agent.git.commit`
- `sub_agent.test.started` / `passed` / `failed`

---

## ADR-009: Preset Validation Configurations

**Status**: Accepted

**Context**:
Different tasks have different complexity levels. Simple tasks (add logging) need relaxed thresholds. Complex tasks (multi-component features) need strict thresholds.

**Decision**:
Provide three preset configurations:
1. **SimpleTaskConfig**: Relaxed (MinFileCount=1, PassThreshold=0.5)
2. **DefaultValidationConfig**: Balanced (MinFileCount=3, PassThreshold=0.6)
3. **ComplexTaskConfig**: Strict (MinFileCount=5, PassThreshold=0.7)

Allow custom configurations via SetValidationConfig().

**Rationale**:
- **Usability**: Most users pick preset, don't tune manually
- **Flexibility**: Advanced users can customize
- **Intent-revealing**: Config name documents task complexity
- **Avoid tuning**: Presets eliminate trial-and-error

**Consequences**:
- **Positive**: Better defaults, fewer false positives/negatives
- **Negative**: Users may pick wrong preset
- **Mitigation**: Document when to use each config

**Preset Values**:
```go
SimpleTaskConfig = ValidationConfig{
    MinFileCount:    1,
    MinLineCount:    10,
    MinCommitCount:  1,
    MinTestRuns:     0,
    MaxStubKeywords: 5,
    PassThreshold:   0.5,
}

DefaultValidationConfig = ValidationConfig{
    MinFileCount:    3,
    MinLineCount:    50,
    MinCommitCount:  2,
    MinTestRuns:     1,
    MaxStubKeywords: 3,
    PassThreshold:   0.6,
}

ComplexTaskConfig = ValidationConfig{
    MinFileCount:    5,
    MinLineCount:    200,
    MinCommitCount:  5,
    MinTestRuns:     2,
    MaxStubKeywords: 0,
    PassThreshold:   0.7,
}
```

**Alternatives Considered**:
1. **Single global config**
   - Rejected: One-size-fits-all doesn't work
2. **Auto-detect complexity** (ML or heuristics)
   - Deferred: Requires more research, error-prone
3. **Per-signal thresholds only** (no presets)
   - Rejected: Too many knobs, poor UX

**Usage**:
```go
monitor := monitoring.NewAgentMonitor("agent-123", workDir)
monitor.SetValidationConfig(monitoring.ComplexTaskConfig)
```

---

## ADR-010: Weighted Signal Scoring

**Status**: Accepted

**Context**:
Not all validation signals are equally important. Test execution is a stronger quality indicator than file count.

**Decision**:
Assign weights to signals based on quality correlation:
- Git commits: 0.2
- File count: 0.2
- Line count: 0.2
- Test execution: 0.3 (highest)
- Stub keywords: 0.1 (lowest)

Score = sum of weights for passed signals

**Rationale**:
- **Prioritization**: Test execution weighted highest (0.3)
- **Balance**: No single signal dominates (max 0.3)
- **Flexibility**: Can pass without perfect score
- **Evidence-based**: Weights reflect quality indicators

**Consequences**:
- **Positive**: Nuanced scoring, reflects reality
- **Negative**: Magic numbers, weights subjective
- **Mitigation**: Document rationale, allow config overrides (future)

**Weight Rationale**:
- **Test execution (0.3)**: Strongest quality indicator, hard to fake
- **Git/File/Line (0.2 each)**: Structural signals, moderate importance
- **Stub keywords (0.1)**: Weak signal (legitimate TODOs exist)

**Scoring Examples**:
```
Scenario 1: All signals pass → Score = 1.0 (pass)
Scenario 2: No tests, but everything else passes → Score = 0.7 (pass)
Scenario 3: Only file count and line count pass → Score = 0.4 (fail)
Scenario 4: Only test execution passes → Score = 0.3 (fail)
```

**Alternatives Considered**:
1. **Equal weights** (0.2 each)
   - Rejected: Doesn't reflect relative importance
2. **Boolean AND** (all signals must pass)
   - Rejected: Too strict, many false negatives
3. **Boolean OR** (any signal passes)
   - Rejected: Too lenient, easy to fake
4. **Machine learning weights** (learned from data)
   - Deferred: Requires labeled dataset

---

## ADR-011: Synchronous Event Publishing

**Status**: Accepted

**Context**:
Event publishing can be synchronous (blocking) or asynchronous (buffered channel). What are the tradeoffs?

**Decision**:
Use synchronous event publishing (EventBus.Publish blocks until event is written).

**Rationale**:
- **Simplicity**: No goroutines, no buffering, no backpressure
- **Reliability**: Events guaranteed written before Publish returns
- **Debuggability**: Sequential execution, easier to trace
- **Event volume**: Low event rate (< 100 events/second)

**Consequences**:
- **Positive**: Simpler code, guaranteed delivery, no lost events
- **Negative**: Slower if event writing is slow
- **Mitigation**: JSONL writes are fast (< 1ms), acceptable overhead

**Performance Analysis**:
- Event publishing: ~10-100 events per sub-agent execution
- Write latency: < 1ms per JSONL append
- Total overhead: < 100ms (negligible vs. sub-agent execution time)

**Alternatives Considered**:
1. **Async with buffered channel**
   - Rejected: Adds complexity, potential event loss on crash
2. **Async with unbounded queue**
   - Rejected: Memory exhaustion risk
3. **Batched writes**
   - Rejected: Complexity not justified by performance gains

**Future Consideration**:
If event volume becomes a bottleneck (e.g., 1000+ events/second), consider async publishing with bounded buffering.

---

## ADR-012: Source File Extension Whitelist

**Status**: Accepted

**Context**:
Line count signal needs to identify source files. Options: file content analysis, extension whitelist, ignore patterns.

**Decision**:
Use extension whitelist for source file detection:
- Included: .go, .js, .ts, .py, .java, .c, .cpp, .h, .rb, .rs
- Excluded: .git/, node_modules/, __pycache__, .vscode/, .idea/

**Rationale**:
- **Simplicity**: Fast extension check, no file I/O
- **Coverage**: Covers common languages in Engram ecosystem
- **Accuracy**: Extension is reliable proxy for source code
- **Performance**: No need to read file content

**Consequences**:
- **Positive**: Fast, simple, works for most projects
- **Negative**: Misses extensionless files (Makefile, Dockerfile)
- **Mitigation**: Acceptable for validation purposes (line count is one signal)

**Implementation**:
```go
ext := filepath.Ext(path)
sourceExts := map[string]bool{
    ".go": true, ".js": true, ".ts": true, ".py": true,
    ".java": true, ".c": true, ".cpp": true, ".h": true,
    ".rb": true, ".rs": true,
}
return sourceExts[ext]
```

**Exclusion Patterns**:
- .git/ - Version control metadata
- node_modules/ - npm dependencies
- __pycache__/ - Python bytecode
- .vscode/, .idea/ - IDE configuration

**Alternatives Considered**:
1. **File content analysis** (detect shebang, syntax)
   - Rejected: Too slow, complex
2. **Glob patterns** (e.g., *.{go,js,py})
   - Rejected: Less efficient than map lookup
3. **No filtering** (count all files)
   - Rejected: Inflates line count with dependencies

**Extensibility**:
Future: Allow custom extension list via config

---

## ADR-013: Fallback Test Execution

**Status**: Accepted

**Context**:
If no test events are detected, should we run tests ourselves to verify they exist and pass?

**Decision**:
Implement fallback test execution:
1. Check if MinTestRuns > 0
2. Detect project type (go.mod, package.json, setup.py)
3. Run appropriate test command (go test, npm test, pytest)
4. If tests pass, count as 1 test run

**Rationale**:
- **Reliability**: Ensures test validation even with monitoring failures
- **Verification**: Actually runs tests, not just checks for output
- **Quality signal**: Tests that don't run are as bad as no tests
- **Fallback tier**: Only used when event log has no test events

**Consequences**:
- **Positive**: High reliability, verifies tests actually work
- **Negative**: Slower validation, may run tests twice
- **Mitigation**: Only used as fallback, can disable with MinTestRuns=0

**Implementation**:
```go
// Only attempt if no test events detected
if testRuns == 0 && v.config.MinTestRuns > 0 {
    if v.canRunTests() {
        if err := v.runTests(); err == nil {
            testRuns = 1  // Tests passed
        }
    }
}
```

**Project Detection**:
- Go: Check for go.mod → Run `go test ./...`
- npm: Check for package.json → Run `npm test`
- Python: Check for setup.py → Run `pytest`

**Alternatives Considered**:
1. **Never run tests** (rely on event log only)
   - Rejected: Too brittle, fails with monitoring issues
2. **Always run tests** (ignore event log)
   - Rejected: Slow, runs tests redundantly
3. **Check for test files** (without running)
   - Rejected: Doesn't verify tests actually work

**Timeout Consideration**:
Future: Add timeout for fallback test execution (e.g., 30 seconds)

---

## Summary of Key Decisions

| ADR | Decision | Rationale |
|-----|----------|-----------|
| ADR-001 | Multi-signal validation | Defense in depth, harder to fake |
| ADR-002 | Best-effort monitoring | Robustness, never block sub-agent |
| ADR-003 | Three-tier fallback strategy | Reliability with degraded monitoring |
| ADR-004 | fsnotify for file watching | Real-time, cross-platform, mature |
| ADR-005 | Git post-commit hooks | Real-time commit detection, rich metadata |
| ADR-006 | Regex output parsing | Simple, fast, framework-agnostic |
| ADR-007 | JSONL event log | Append-only, human-readable, streaming |
| ADR-008 | EventBus decoupling | Clean architecture, testability |
| ADR-009 | Preset configurations | Usability, intent-revealing |
| ADR-010 | Weighted signal scoring | Reflects relative importance |
| ADR-011 | Synchronous publishing | Simplicity, reliability |
| ADR-012 | Extension whitelist | Performance, simplicity |
| ADR-013 | Fallback test execution | Reliability, verification |

## Future ADRs to Consider

- **ADR-014**: Streaming validation (real-time scoring during execution)
- **ADR-015**: ML-based fake detection (supplement rule-based signals)
- **ADR-016**: Network activity monitoring (API calls, DB queries)
- **ADR-017**: Code quality metrics (complexity, coverage, duplication)
- **ADR-018**: Resource usage tracking (CPU, memory, disk I/O)
- **ADR-019**: Distributed monitoring (multi-agent workflows)
- **ADR-020**: Cryptographic event signatures (tamper detection)
