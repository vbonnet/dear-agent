# Concurrency Model

This document describes the concurrency guarantees, resource limits, and thread safety of the AGM sandbox system.

## Overview

The sandbox system is designed to handle multiple concurrent sandboxes safely, enabling AGM to run multiple agent sessions simultaneously without resource conflicts or data corruption.

## Thread Safety Guarantees

### Provider Interface

All `Provider` implementations MUST be safe for concurrent use. The interface contract guarantees:

- **Create**: Multiple goroutines can call `Create()` concurrently
- **Destroy**: Multiple goroutines can call `Destroy()` concurrently
- **Validate**: Multiple goroutines can call `Validate()` concurrently
- **Mixed Operations**: Any combination of Create/Destroy/Validate can run concurrently

### OverlayFS Provider

The `overlayfs.Provider` implementation ensures thread safety through:

1. **Internal State Protection**: Uses a mutex-protected map to track active sandboxes
2. **Atomic Operations**: All mount/unmount operations are atomic at the kernel level
3. **Unique Paths**: Each sandbox gets unique directory paths (no collisions)
4. **Independent Cleanup**: Each sandbox's cleanup is independent and idempotent

```go
type Provider struct {
    mu        sync.RWMutex
    sandboxes map[string]*sandbox.Sandbox
}
```

### APFS Provider

The `apfs.Provider` implementation (when available) provides similar guarantees:

1. **APFS Clones**: Each sandbox uses independent APFS filesystem clones
2. **Atomic Clone Operations**: Clone creation is atomic at the filesystem level
3. **Independent Snapshots**: Each sandbox snapshot is independent

## Resource Limits

### Concurrent Sandbox Limits

**Tested Limits**:
- ✅ **10 concurrent sandboxes**: Fully tested, recommended for production
- ✅ **50 concurrent sandboxes**: Tested, suitable for high-load scenarios
- ⚠️ **100+ concurrent sandboxes**: Not tested, may require system tuning

**Limiting Factors**:

1. **File Descriptors**
   - Each sandbox uses ~3-5 file descriptors
   - System default: `ulimit -n` (typically 1024)
   - Recommended increase for 50+ sandboxes: `ulimit -n 4096`

2. **Mount Table Size**
   - Each OverlayFS sandbox creates 1 mount entry
   - Linux default: `/proc/sys/fs/mount-max` (typically unlimited)
   - Check current: `cat /proc/mounts | wc -l`

3. **Memory Usage**
   - Upper directory size grows with modifications
   - Work directory holds temporary data during copy-up
   - Typical overhead: ~1-10 MB per sandbox (depends on modifications)

4. **Disk I/O**
   - Concurrent mount operations may stress I/O subsystem
   - SSD recommended for high concurrency (50+ sandboxes)

### System Tuning for High Concurrency

For 50+ concurrent sandboxes, recommended system configuration:

```bash
# Increase file descriptor limit
ulimit -n 8192

# Increase inotify watches (for monitoring)
sudo sysctl fs.inotify.max_user_watches=524288

# Check mount table size
cat /proc/mounts | wc -l

# Monitor file descriptor usage
ls /proc/$PID/fd | wc -l
```

## Concurrency Patterns

### Creating Multiple Sandboxes

**Pattern: Parallel Creation with Error Handling**

```go
import "golang.org/x/sync/errgroup"

func createManySandboxes(ctx context.Context, provider sandbox.Provider, count int) error {
    g, gctx := errgroup.WithContext(ctx)

    for i := 0; i < count; i++ {
        i := i  // Capture loop variable
        g.Go(func() error {
            sb, err := provider.Create(gctx, sandbox.SandboxRequest{
                SessionID:    fmt.Sprintf("session-%d", i),
                LowerDirs:    []string{"/path/to/repo"},
                WorkspaceDir: fmt.Sprintf("/tmp/sandbox-%d", i),
            })
            if err != nil {
                return fmt.Errorf("create sandbox %d failed: %w", i, err)
            }
            // Store sb for later cleanup
            return nil
        })
    }

    return g.Wait()
}
```

### Mixed Operations

**Pattern: Concurrent Create and Destroy**

```go
func mixedOperations(ctx context.Context, provider sandbox.Provider) error {
    var mu sync.Mutex
    activeSandboxes := make(map[string]*sandbox.Sandbox)

    g, gctx := errgroup.WithContext(ctx)

    // Create operation
    g.Go(func() error {
        sb, err := provider.Create(gctx, req)
        if err != nil {
            return err
        }

        mu.Lock()
        activeSandboxes[sb.ID] = sb
        mu.Unlock()

        return nil
    })

    // Destroy operation
    g.Go(func() error {
        mu.Lock()
        var targetID string
        for id := range activeSandboxes {
            targetID = id
            delete(activeSandboxes, id)
            break
        }
        mu.Unlock()

        if targetID != "" {
            return provider.Destroy(gctx, targetID)
        }
        return nil
    })

    return g.Wait()
}
```

### Cleanup Verification

**Pattern: Resource Leak Detection**

```go
func verifyNoLeaks(t *testing.T, provider sandbox.Provider) {
    // Record initial state
    fdsBefore := countOpenFileDescriptors()
    mountsBefore := countMounts()

    // Perform operations
    // ...

    // Clean up
    // ...

    // Wait for kernel cleanup
    time.Sleep(500 * time.Millisecond)

    // Verify cleanup
    fdsAfter := countOpenFileDescriptors()
    mountsAfter := countMounts()

    fdDelta := fdsAfter - fdsBefore
    mountDelta := mountsAfter - mountsBefore

    if fdDelta > 10 {
        t.Logf("WARNING: Possible FD leak: delta=%d", fdDelta)
    }

    if mountDelta != 0 {
        t.Errorf("Mount leak detected: delta=%d", mountDelta)
    }
}
```

## Isolation Guarantees

### Filesystem Isolation

Each sandbox provides complete filesystem isolation:

1. **Read-Only Lower Layer**: All sandboxes share the same read-only view
2. **Copy-on-Write Upper Layer**: Each sandbox has its own private upper directory
3. **Independent Modifications**: Changes in one sandbox don't affect others
4. **Whiteout Propagation**: File deletions in one sandbox don't affect others

**Test Verification**:

```go
// Create 20 sandboxes sharing the same lower directory
// Each modifies the same file with unique content
// Verify each sandbox sees only its own modifications
// Verify original file is unchanged
```

### Resource Isolation

Each sandbox has isolated resources:

1. **Separate Mount Namespace**: Each OverlayFS mount is independent
2. **Separate Work Directory**: Temporary copy-up data is isolated
3. **Separate Upper Directory**: Modified files are isolated
4. **Independent Cleanup**: Destroying one sandbox doesn't affect others

## Error Handling

### Context Cancellation

All provider methods honor `context.Context` cancellation:

```go
ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()

sb, err := provider.Create(ctx, req)
if err != nil {
    if errors.Is(err, context.DeadlineExceeded) {
        // Handle timeout
    }
    if errors.Is(err, context.Canceled) {
        // Handle cancellation
    }
}
```

### Partial Failure Handling

If some concurrent operations fail:

```go
g, gctx := errgroup.WithContext(ctx)
var successCount atomic.Int64

for i := 0; i < n; i++ {
    g.Go(func() error {
        sb, err := provider.Create(gctx, req)
        if err != nil {
            return err  // Will cancel context for other goroutines
        }
        successCount.Add(1)
        return nil
    })
}

if err := g.Wait(); err != nil {
    log.Printf("Some operations failed, %d succeeded", successCount.Load())
    // Clean up partial state
}
```

## Troubleshooting

### "Too many open files" Error

**Symptom**: `Create()` fails with "too many open files"

**Cause**: File descriptor limit exceeded

**Solution**:
```bash
# Check current limit
ulimit -n

# Increase limit (temporary)
ulimit -n 4096

# Increase limit (permanent)
echo "* soft nofile 8192" | sudo tee -a /etc/security/limits.conf
echo "* hard nofile 8192" | sudo tee -a /etc/security/limits.conf
```

### "Device or resource busy" on Unmount

**Symptom**: `Destroy()` fails with "device or resource busy"

**Cause**: Files are still open in the merged directory

**Solution**:
1. Ensure all file handles are closed before calling `Destroy()`
2. Use `defer file.Close()` consistently
3. Check for leaked goroutines holding file references

**Debugging**:
```bash
# List processes using a mount point
lsof +D /path/to/merged

# Force unmount (last resort)
umount -l /path/to/merged
```

### Mount Table Full

**Symptom**: `Create()` fails with "mount table full"

**Cause**: Too many active mounts on system

**Solution**:
```bash
# Check current mount count
cat /proc/mounts | wc -l

# Clean up stale mounts
umount /path/to/stale/mounts

# Ensure proper cleanup in code
defer provider.Destroy(ctx, sb.ID)
```

### Memory Usage Growing

**Symptom**: Memory usage grows with each sandbox creation

**Cause**: Upper directory accumulating large amounts of data

**Solution**:
1. Limit modifications in each sandbox
2. Clean up sandboxes promptly after use
3. Monitor upper directory size: `du -sh /path/to/upper`

### Race Conditions

**Symptom**: Tests fail with `-race` flag

**Cause**: Unsynchronized access to shared state

**Solution**:
1. Run tests with race detector: `go test -race ./...`
2. Fix all reported races (none should exist in provider code)
3. Add proper synchronization (mutexes, channels, atomic operations)

**Example Fix**:
```go
// BAD: Race condition
var sandboxes []*sandbox.Sandbox
g.Go(func() error {
    sandboxes[i] = sb  // Race: concurrent writes
    return nil
})

// GOOD: No race
var mu sync.Mutex
var sandboxes []*sandbox.Sandbox
g.Go(func() error {
    mu.Lock()
    sandboxes[i] = sb
    mu.Unlock()
    return nil
})
```

## Performance Characteristics

### Creation Time

- **Single sandbox**: ~50-200ms (depends on lower dir size)
- **10 concurrent sandboxes**: ~100-300ms total (parallelized)
- **50 concurrent sandboxes**: ~500-1000ms total (I/O bound)

### Memory Overhead

- **Per sandbox (empty)**: ~1-2 MB (directory metadata)
- **Per sandbox (modified)**: Variable (depends on modifications)
- **100 empty sandboxes**: ~100-200 MB total overhead

### Scalability

**Linear Scaling**: Up to ~20 concurrent sandboxes
- Creation time scales linearly with count
- Each sandbox independent

**I/O Bound**: 20-50 concurrent sandboxes
- Mount operations become I/O bound
- SSD provides better performance than HDD

**System Limited**: 50+ concurrent sandboxes
- File descriptor limits become relevant
- Mount table size may need tuning
- Kernel overhead increases

## Testing

The concurrency model is validated by the following test suite:

### Test Coverage

- **TestConcurrentCreate_10**: 10 parallel sandbox creates
- **TestConcurrentCreate_50**: 50 parallel sandbox creates
- **TestConcurrentOperations**: Mixed create/destroy operations
- **TestConcurrentIsolation**: Independent modifications across 20 sandboxes
- **TestConcurrentCleanup**: Resource cleanup verification
- **TestRaceConditions**: Race detector validation (run with `-race`)

### Running Tests

```bash
# Run all concurrency tests
go test -v ./internal/sandbox/... -run=Concurrent

# Run with race detector
go test -race ./internal/sandbox/... -run=Concurrent

# Run specific concurrency level
go test -v ./internal/sandbox/... -run=TestConcurrentCreate_10

# Skip slow tests
go test -short ./internal/sandbox/...
```

### Expected Output

```
=== RUN   TestConcurrentCreate_10
    concurrency_test.go:XXX: Resource cleanup verified: FDs delta=2, mounts delta=0
--- PASS: TestConcurrentCreate_10 (1.23s)
=== RUN   TestConcurrentCreate_50
    concurrency_test.go:XXX: Resource cleanup verified: FDs delta=5, mounts delta=0
--- PASS: TestConcurrentCreate_50 (5.67s)
```

## Best Practices

### 1. Always Use Context with Timeout

```go
ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()

sb, err := provider.Create(ctx, req)
```

### 2. Use errgroup for Concurrent Operations

```go
import "golang.org/x/sync/errgroup"

g, gctx := errgroup.WithContext(ctx)
// ... launch goroutines
if err := g.Wait(); err != nil {
    // Handle error
}
```

### 3. Always Defer Cleanup

```go
sb, err := provider.Create(ctx, req)
if err != nil {
    return err
}
defer provider.Destroy(context.Background(), sb.ID)
```

### 4. Check File Descriptor Limits

```go
// Before creating many sandboxes
if numSandboxes > 50 {
    // Verify ulimit is sufficient
    // Implement backpressure/rate limiting
}
```

### 5. Monitor Resource Usage

```go
// Track metrics
metrics.GaugeSet("sandboxes.active", len(activeSandboxes))
metrics.GaugeSet("sandboxes.fds", countOpenFileDescriptors())
metrics.GaugeSet("sandboxes.mounts", countMounts())
```

## Future Improvements

### Planned Enhancements

1. **Built-in Rate Limiting**: Limit concurrent creates to prevent resource exhaustion
2. **Resource Pools**: Reuse sandbox directories to reduce creation overhead
3. **Health Monitoring**: Automatic detection of resource leaks
4. **Graceful Degradation**: Automatic backoff when system limits approached

### Research Areas

1. **User Namespaces**: Explore better isolation with user namespaces
2. **cgroup Integration**: Resource limits per sandbox (CPU, memory)
3. **Async Mount Operations**: Non-blocking mount operations
4. **Mount Pooling**: Pre-create mount namespaces for faster provisioning
