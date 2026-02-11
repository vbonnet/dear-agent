# ADR-002: Telemetry File Location Strategy

## Status

**Accepted** - Implemented in swarm-executor v0.1.0

## Context

swarm-executor generates multiple output files during execution:
- EXECUTION-LOG.jsonl (append-only event log)
- ROADMAP.md (human-readable progress summary)
- TASK-QUEUE.yaml (updated queue state)

We need to determine where these files should be located and how their paths are derived.

### Requirements

1. **Discoverability**: Users should easily find telemetry files
2. **Cleanup**: All execution artifacts should be easy to identify and archive together
3. **No Configuration Required**: v0.1.0 targets zero-config usage
4. **Consistent Location**: All execution-related files in predictable locations
5. **Multiple Queue Support**: Support different queues in different directories

### Constraints

- Queue file path is user-provided (--queue flag)
- Cannot require additional flags for output paths (simplicity goal)
- Must work when queue path is absolute or relative
- Should avoid cluttering user's home directory or current working directory

## Decision

**Colocate all telemetry files with the queue file** by deriving output paths from the
queue file's directory:

```go
workDir := filepath.Dir(queuePath)
logFile := filepath.Join(workDir, "EXECUTION-LOG.jsonl")
roadmapFile := filepath.Join(workDir, "ROADMAP.md")
```

### File Layout

```
/path/to/project/
├── TASK-QUEUE.yaml          # Input (user-provided path via --queue)
├── EXECUTION-LOG.jsonl      # Output (derived: workDir/EXECUTION-LOG.jsonl)
└── ROADMAP.md               # Output (derived: workDir/ROADMAP.md)
```

### Implementation

```go
func executeBead(queuePath string, beadID string, sessionName string) int {
    // Initialize telemetry
    workDir := filepath.Dir(queuePath)
    logFile := filepath.Join(workDir, "EXECUTION-LOG.jsonl")
    roadmapFile := filepath.Join(workDir, "ROADMAP.md")

    logger := telemetry.NewLogger(logFile)

    // ... execution logic ...

    // Generate roadmap in same directory
    if err := telemetry.GenerateRoadmap(queuePath, roadmapFile); err != nil {
        fmt.Fprintf(os.Stderr, "Warning: Failed to generate roadmap: %v\n", err)
    }

    return 0
}
```

### Path Derivation Examples

```
Queue Path                           | Log File                              | Roadmap File
-------------------------------------|---------------------------------------|--------------------------------------
./TASK-QUEUE.yaml                    | ./EXECUTION-LOG.jsonl                 | ./ROADMAP.md
/home/user/project/TASK-QUEUE.yaml   | /home/user/project/EXECUTION-LOG.jsonl| /home/user/project/ROADMAP.md
../other/queue.yaml                  | ../other/EXECUTION-LOG.jsonl          | ../other/ROADMAP.md
```

## Consequences

### Positive

**Zero Configuration Required**:
- No additional flags for output paths
- Works out-of-box with just --queue
- Simple mental model: all files together

**Easy Discovery**:
```bash
# All execution artifacts in one place
ls /path/to/project/
TASK-QUEUE.yaml
EXECUTION-LOG.jsonl
ROADMAP.md

# Easy cleanup
rm /path/to/project/*.jsonl /path/to/project/ROADMAP.md
```

**Archive-Friendly**:
```bash
# Archive entire execution context
tar -czf execution-2024-01-01.tar.gz /path/to/project/

# Move to different directory preserving relationships
mv /path/to/project /archive/completed/project-123
```

**Multiple Queue Support**:
```
projects/
├── project-a/
│   ├── TASK-QUEUE.yaml
│   ├── EXECUTION-LOG.jsonl
│   └── ROADMAP.md
└── project-b/
    ├── TASK-QUEUE.yaml
    ├── EXECUTION-LOG.jsonl
    └── ROADMAP.md
```

Each project's telemetry isolated in its own directory.

**Intuitive Behavior**:
- Users expect output files near input files
- Matches common tool behavior (make, go build, etc.)

### Negative

**Directory Pollution**:
- Adds two files to queue directory (may clutter)
- No control over output location without additional complexity
- Mitigated: Only 2 predictable files, easy to .gitignore

**Write Permission Required**:
- Queue directory must be writable
- Fails if queue in read-only location
- Mitigated: Queue must be writable anyway (executor updates it)

**No Central Log Directory**:
- Cannot aggregate logs from multiple queues easily
- Each queue has its own log file
- Mitigated: Future enhancement with --log-dir flag

**Hardcoded Filenames**:
- Cannot run multiple executors on same queue simultaneously
- EXECUTION-LOG.jsonl and ROADMAP.md names fixed
- Mitigated: Single executor per queue is current design

## Alternatives Considered

### Alternative 1: Current Working Directory

**Approach**: Write telemetry files to current working directory

```go
logFile := "EXECUTION-LOG.jsonl"  // Relative to cwd
roadmapFile := "ROADMAP.md"
```

**Pros**:
- Simple implementation
- User controls location via cd before execution

**Cons**:
- Files separated from queue (discoverability issue)
- Clutters cwd (especially if running from home)
- Multiple queues in different dirs → all logs mixed in cwd
- **REJECTED**: Poor discoverability, confusing for multi-queue usage

### Alternative 2: Dedicated Log Directory

**Approach**: Require --log-dir flag for output location

```bash
swarm-executor --queue Q.yaml --bead-id B --session S --log-dir ./logs
```

**Pros**:
- Centralized log aggregation
- Clean separation of input/output
- Multiple executors per queue (different log dirs)

**Cons**:
- Requires additional flag (violates zero-config goal)
- Users must create log directory
- More complex to document and explain
- **REJECTED**: Too much configuration for v0.1.0

### Alternative 3: Home Directory Convention

**Approach**: Write logs to ~/.swarm-executor/logs/

```go
homeDir, _ := os.UserHomeDir()
logDir := filepath.Join(homeDir, ".swarm-executor", "logs")
logFile := filepath.Join(logDir, fmt.Sprintf("%s.jsonl", sessionName))
```

**Pros**:
- Centralized location for all executions
- Doesn't clutter project directories
- Standard convention for CLI tools

**Cons**:
- Logs separated from queue (discoverability)
- Requires directory creation logic
- Hard to clean up old logs
- Session name collision risk
- **REJECTED**: Separation from queue is anti-pattern for this use case

### Alternative 4: Timestamped Files

**Approach**: Use timestamps in filenames to avoid conflicts

```go
timestamp := time.Now().Format("20060102-150405")
logFile := filepath.Join(workDir, fmt.Sprintf("EXECUTION-LOG-%s.jsonl", timestamp))
```

**Pros**:
- Multiple concurrent executors possible
- Historical log preservation
- No file clobbering

**Cons**:
- Logs accumulate (cleanup required)
- Harder to find "current" log
- Roadmap generation more complex (which log to read?)
- **REJECTED**: Over-engineering for single-executor design

### Alternative 5: Environment Variable Override

**Approach**: Default to workDir, allow override via env var

```go
logDir := os.Getenv("SWARM_LOG_DIR")
if logDir == "" {
    logDir = filepath.Dir(queuePath)
}
logFile := filepath.Join(logDir, "EXECUTION-LOG.jsonl")
```

**Pros**:
- Zero-config default behavior
- Power users can centralize if needed
- Backward compatible

**Cons**:
- Harder to discover (environment variables less visible)
- Testing complexity (must mock env vars)
- Documentation overhead (explain default + override)
- **CONSIDERED FOR FUTURE**: Good v0.2.0 enhancement

## Implementation Notes

### Path Derivation

```go
// workDir extraction uses filepath.Dir
queuePath := "/home/user/project/TASK-QUEUE.yaml"
workDir := filepath.Dir(queuePath)  // → "/home/user/project"

// Edge case: Current directory
queuePath := "TASK-QUEUE.yaml"
workDir := filepath.Dir(queuePath)  // → "."

// Edge case: Root directory (unusual but valid)
queuePath := "/TASK-QUEUE.yaml"
workDir := filepath.Dir(queuePath)  // → "/"
```

### File Creation

```go
// Logger creates file on first write (lazy initialization)
logger := telemetry.NewLogger(logFile)
// File not created yet

logger.LogEvent(event)  // File created here if doesn't exist
```

### Error Handling

```go
// Telemetry failures are non-fatal
if err := logger.LogEvent(startEvent); err != nil {
    fmt.Fprintf(os.Stderr, "Warning: Failed to log start event: %v\n", err)
    // Continue execution - telemetry is best-effort
}
```

**Rationale**: Execution should not fail due to logging issues. Telemetry is observability,
not critical path.

### Testing Pattern

```go
func TestTelemetryFileLocation(t *testing.T) {
    tmpDir := t.TempDir()
    queueFile := filepath.Join(tmpDir, "TASK-QUEUE.yaml")

    // Create queue file
    createTestQueue(queueFile)

    // Run executor
    runExecutor(queueFile, "bead-1", "test-session")

    // Verify files in same directory
    logFile := filepath.Join(tmpDir, "EXECUTION-LOG.jsonl")
    roadmapFile := filepath.Join(tmpDir, "ROADMAP.md")

    if _, err := os.Stat(logFile); err != nil {
        t.Errorf("log file not created: %v", err)
    }
    if _, err := os.Stat(roadmapFile); err != nil {
        t.Errorf("roadmap file not created: %v", err)
    }
}
```

## Migration Path

### Future Enhancement: Configurable Paths

v0.2.0 could add optional configuration:

```yaml
# swarm-executor.yaml (optional config file)
telemetry:
  log_file: ./logs/EXECUTION-LOG.jsonl    # Override default
  roadmap_file: ./docs/ROADMAP.md         # Override default
  enabled: true                            # Disable if false
```

**Backward Compatibility**:
- If no config file → use current default (colocated files)
- If config file exists → use specified paths
- Never break existing behavior

### Environment Variable Support

```bash
# Override log location
SWARM_LOG_DIR=/var/log/swarm swarm-executor --queue Q.yaml ...

# Disable telemetry (future)
SWARM_TELEMETRY_ENABLED=false swarm-executor --queue Q.yaml ...
```

## Related Decisions

- **ADR-001: Exit Code Design** - Telemetry events correlate with exit codes
- **ADR-004: Atomic Queue Writes** - Queue updates colocated with telemetry

## References

- [Go filepath.Dir documentation](https://pkg.go.dev/path/filepath#Dir)
- [XDG Base Directory Specification](https://specifications.freedesktop.org/basedir-spec/basedir-spec-latest.html)
- [12-Factor App: Logs](https://12factor.net/logs)

## Revision History

| Version | Date | Changes | Author |
|---------|------|---------|--------|
| 1.0 | 2026-02-11 | Initial decision record | Backfill Documentation |
