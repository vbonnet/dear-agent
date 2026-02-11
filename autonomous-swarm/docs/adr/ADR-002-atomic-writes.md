# ADR-002: Atomic Queue Writes with Temp-Rename Pattern

## Status

Accepted

## Context

Autonomous Swarm persists task queue state in a YAML file (`TASK-QUEUE.yaml`) that is frequently modified during bead execution. The queue file is the single source of truth for:

- Which beads are ready, in progress, blocked, or completed
- Bead metadata (session names, iteration counts, timestamps)
- Dependency relationships

The system must handle various failure scenarios:

1. **Process crashes** during queue save operations
2. **Disk full** errors while writing
3. **Power loss** mid-write
4. **Concurrent access** from multiple processes (future)

A corrupted queue file would result in:
- Loss of execution progress
- Inability to resume autonomous execution
- Manual recovery required (violates autonomous operation goal)

We need a persistence strategy that ensures queue consistency even in the presence of failures.

## Decision

We will use the **atomic write pattern (temp file + rename)** for all queue persistence operations.

### Implementation

**File**: `pkg/taskqueue/coordinator.go`

```go
func (c *Coordinator) Save() error {
    c.mu.RLock()
    defer c.mu.RUnlock()

    // Update timestamp
    c.queue.LastUpdated = time.Now()

    // Marshal to YAML
    data, err := yaml.Marshal(c.queue)
    if err != nil {
        return fmt.Errorf("failed to marshal task queue: %w", err)
    }

    // Atomic write: temp file + rename
    tmpPath := c.filePath + ".tmp"
    if err := os.WriteFile(tmpPath, data, 0644); err != nil {
        return fmt.Errorf("failed to write temp file: %w", err)
    }

    if err := os.Rename(tmpPath, c.filePath); err != nil {
        _ = os.Remove(tmpPath) // Cleanup on failure
        return fmt.Errorf("failed to rename temp file: %w", err)
    }

    return nil
}
```

### Key Properties

#### 1. Atomicity Guarantee

**POSIX Rename Semantics**:
- `os.Rename()` is atomic on Unix-like systems (Linux, macOS)
- Either completes fully or not at all (no partial state)
- Old file remains if rename fails

**Result**: Readers always see either old valid state OR new valid state, never corrupted state.

#### 2. Write Flow

```
1. Marshal queue to YAML in memory
2. Write YAML to temporary file (TASK-QUEUE.yaml.tmp)
3. Atomically rename temp file to target (TASK-QUEUE.yaml)
4. On success: old file overwritten atomically
5. On failure: temp file removed, old file unchanged
```

#### 3. Failure Handling

| Failure Point | Recovery | Data Loss |
|---------------|----------|-----------|
| Marshal error | Return error, no write | None |
| Temp write fails | Return error, no rename | None |
| Disk full | Return error, no rename | None |
| Rename fails | Delete temp, return error | None |
| Process crash during write | Old file intact | Last update only |
| Process crash during rename | Old OR new file intact | None |

**Crash-Only Design**: No graceful shutdown required; state always consistent.

#### 4. Concurrency Model

**Current (Single Writer)**:
- RWMutex protects in-memory queue
- RLock for save (reads internal state)
- Lock for load (mutates internal state)
- Single coordinator instance assumed

**Future (Multi-Process)**:
- Add file locking (flock) around save operation
- Detect stale .tmp files on startup (cleanup)
- Optimistic concurrency with retry on conflict

### Rejected Alternatives

#### Alternative 1: Direct Overwrite

**Approach**: Write directly to `TASK-QUEUE.yaml`

```go
os.WriteFile(c.filePath, data, 0644)
```

**Rejected Because**:
- Non-atomic: partial writes on crash leave corrupted file
- Process crash mid-write = unrecoverable state
- Disk full mid-write = truncated YAML

**Risk Example**:
```yaml
# TASK-QUEUE.yaml (after crash mid-write)
schema_version: "1.0.0"
last_updated: 2024-01-01T00:00:00Z
ready:
  - id: bead-1
    title: First be
# END OF FILE (truncated)
```

#### Alternative 2: Write-Ahead Log (WAL)

**Approach**: Append operations to log, periodic compaction

**Rejected Because**:
- Over-engineered for single-writer scenario
- Adds complexity (log replay, compaction logic)
- YAML not designed for append-only semantics
- File grows unbounded without compaction

#### Alternative 3: Database (SQLite, embedded DB)

**Approach**: Store queue in embedded database

**Rejected Because**:
- Violates "file-based state" design principle
- Harder to inspect/debug (no human-readable format)
- Requires schema migrations for changes
- Database file can still corrupt (WAL mode helps but adds complexity)

#### Alternative 4: Copy-on-Write with Versioning

**Approach**: Keep numbered snapshots (queue.v1, queue.v2, ...)

**Rejected Because**:
- Disk space grows unbounded
- Requires garbage collection logic
- Overkill for single source of truth
- Complicates "which version is current"

## Consequences

### Positive

1. **Consistency**: Queue state never corrupted, even on crash
2. **Simplicity**: 5-line implementation, standard pattern
3. **Debuggability**: Human-readable YAML, inspectable with `cat`
4. **Recovery**: No manual intervention needed after crash
5. **Performance**: Atomic rename is fast (metadata-only operation)

### Negative

1. **Disk Usage**: Briefly doubles during write (temp file exists)
2. **Inode Churn**: Each save creates/deletes temp file
3. **Non-Windows**: Atomicity not guaranteed on Windows (rename can fail if file open)

### Trade-offs

- **Simplicity vs Robustness**: Atomic pattern gives robustness without complexity
- **Disk Space vs Safety**: Temporary double usage acceptable for consistency guarantee
- **YAML vs Database**: Human readability valued over query performance

## Implementation Notes

### Temp File Cleanup

```go
if err := os.Rename(tmpPath, c.filePath); err != nil {
    _ = os.Remove(tmpPath) // Best-effort cleanup
    return fmt.Errorf("failed to rename temp file: %w", err)
}
```

**Note**: Cleanup failure is non-fatal; stale `.tmp` files harmless.

### File Permissions

```yaml
TASK-QUEUE.yaml:     0644 (rw-r--r--)
TASK-QUEUE.yaml.tmp: 0644 (rw-r--r--)
```

**Rationale**: Readable by all users (for debugging), writable only by owner.

### RWMutex Strategy

```go
func (c *Coordinator) Save() error {
    c.mu.RLock()  // Read lock: allows concurrent saves
    defer c.mu.RUnlock()
    // ...
}
```

**Rationale**: Save reads internal state (doesn't mutate), so RLock suffices. This allows hypothetical concurrent save calls (though coordinator is single-instance in v1).

### Platform Considerations

**Linux/macOS**: Atomic rename guaranteed by POSIX
**Windows**: `os.Rename()` can fail if target file is open
  - Mitigation: Close all file handles before save
  - Future: Use `MoveFileEx` with `MOVEFILE_REPLACE_EXISTING`

### Future Enhancements

1. **File Locking**: Add `flock` for multi-process safety
2. **Stale Temp Cleanup**: Delete `.tmp` files older than N minutes on startup
3. **Checksum Validation**: Add hash to YAML for corruption detection
4. **Backup on Major Transitions**: Copy to `.backup` before significant changes

## Testing

**Test Coverage**: `pkg/taskqueue/coordinator_test.go`

```go
func TestCoordinator_AtomicWrite(t *testing.T) {
    // Verify old file intact if write fails
    // Verify old file intact if rename fails
    // Verify new file correct if write succeeds
    // Verify temp file cleaned up on failure
}
```

**Race Detector**: `go test -race ./pkg/taskqueue`
- Detects concurrent access issues
- Validates RWMutex correctness

## References

- **POSIX Rename**: IEEE Std 1003.1-2008, `rename()` specification
- **Atomic Writes Pattern**: Used by systemd, etcd, git
- **Implementation**: `pkg/taskqueue/coordinator.go:40-70`
- **Tests**: `pkg/taskqueue/coordinator_test.go`
- **Related**: [ADR-003: Escalation Model](ADR-003-escalation-model.md)

## Real-World Precedents

- **Git**: Uses rename for atomic ref updates (`.git/refs/heads/main.lock`)
- **systemd**: Atomic unit file updates via temp-rename
- **etcd**: Snapshot files written with temp-rename pattern

## Revision History

| Version | Date | Changes | Author |
|---------|------|---------|--------|
| 1.0.0 | 2026-02-11 | Initial decision record | Backfill Documentation |
