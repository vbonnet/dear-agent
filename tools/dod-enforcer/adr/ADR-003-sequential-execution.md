# ADR-003: Sequential Check Execution

## Status

Accepted

## Context

The `Validate()` method executes three types of checks:

1. File existence checks (can be many files)
2. Test execution (one command, potentially long-running)
3. Custom command validations (can be many commands)

These checks could be executed in two ways:

1. **Sequential**: Run checks one after another in a single goroutine
2. **Parallel**: Run checks concurrently using goroutines and synchronization

We need to decide which execution model to use.

## Decision

We will execute all validation checks **sequentially** in a single goroutine.

**Implementation**:
```go
func (d *BeadDoD) Validate() (*ValidationResult, error) {
    var allChecks []CheckResult

    // Sequential execution
    allChecks = append(allChecks, d.checkFilesExist()...)
    allChecks = append(allChecks, d.checkTestsPass()...)
    allChecks = append(allChecks, d.checkCommandsSucceed()...)

    // Aggregate results...
}
```

**Rationale**:
- Simplicity over performance
- Validation is not a performance-critical path
- Predictable execution order aids debugging
- No concurrency bugs or race conditions

## Consequences

### Positive

1. **Simplicity**: No synchronization complexity
   - No goroutines, channels, or WaitGroups
   - Straightforward control flow
   - Easy to reason about execution order

2. **Predictable Ordering**: Checks always run in same order
   - File checks → Test execution → Custom commands
   - Easier to debug failures
   - Consistent behavior across runs

3. **Easier Debugging**: Stack traces are clear
   - No goroutine interleaving
   - Panic stack traces point directly to failing check
   - Simpler profiling and tracing

4. **No Race Conditions**: Zero concurrency bugs
   - No shared mutable state
   - No need for locks or atomics
   - No risk of deadlocks

5. **Better Error Context**: Errors processed immediately
   - First failure detected in order
   - Error messages have clear context

### Negative

1. **Slower Validation**: Longer wall-clock time for validation
   - File checks run serially (O(n) instead of O(1) with parallelism)
   - Commands that could run concurrently wait on each other
   - Total time = sum of all check durations

2. **Poor Resource Utilization**: CPU idle during I/O
   - File stat calls block execution
   - Command execution blocks other checks
   - Could be running multiple commands simultaneously

3. **Longer Feedback Loop**: Developers wait longer for results
   - 10 file checks at 50ms each = 500ms sequential vs ~50ms parallel
   - Multiple commands at 2s each add up
   - Especially noticeable in CI/CD pipelines

4. **No Early Termination Optimization**: Cannot stop on first failure
   - Currently runs all checks even if first one fails
   - Could abort remaining checks for faster feedback
   - (Though we continue all checks to provide complete results)

## Alternatives Considered

### 1. Parallel File Checks

**Description**: Run file existence checks concurrently

```go
func (d *BeadDoD) checkFilesExist() []CheckResult {
    var wg sync.WaitGroup
    results := make([]CheckResult, len(d.FilesMustExist))

    for i, path := range d.FilesMustExist {
        wg.Add(1)
        go func(i int, path string) {
            defer wg.Done()
            // Check file existence
            results[i] = checkFile(path)
        }(i, path)
    }

    wg.Wait()
    return results
}
```

**Rejected because**:
- Added complexity not justified by performance gain
- File stat calls are very fast (< 10ms typically)
- Parallel overhead might exceed serial execution time for small counts
- Race condition risk if not implemented carefully

### 2. Parallel Commands

**Description**: Run custom commands concurrently

```go
func (d *BeadDoD) checkCommandsSucceed() []CheckResult {
    var wg sync.WaitGroup
    results := make([]CheckResult, len(d.CommandsMustSucceed))

    for i, cmd := range d.CommandsMustSucceed {
        wg.Add(1)
        go func(i int, cmd CommandCheck) {
            defer wg.Done()
            results[i] = executeAndCheck(cmd)
        }(i, cmd)
    }

    wg.Wait()
    return results
}
```

**Rejected because**:
- Commands may have dependencies (e.g., clean → build → test)
- Parallel execution could cause resource contention
- Harder to debug failures when interleaved
- Sequential order is more intuitive

### 3. Pipeline Pattern

**Description**: Stream check results through channels

```go
func (d *BeadDoD) Validate() (*ValidationResult, error) {
    checkCh := make(chan CheckResult)

    go func() {
        for _, path := range d.FilesMustExist {
            checkCh <- checkFile(path)
        }
        if d.TestsMustPass {
            checkCh <- runTests()
        }
        for _, cmd := range d.CommandsMustSucceed {
            checkCh <- executeCommand(cmd)
        }
        close(checkCh)
    }()

    var results []CheckResult
    for check := range checkCh {
        results = append(results, check)
    }
}
```

**Rejected because**:
- Overkill for simple validation
- Harder to maintain and understand
- No significant benefit over sequential

### 4. Fail-Fast Parallel

**Description**: Run checks in parallel, stop on first failure

```go
ctx, cancel := context.WithCancel(context.Background())
defer cancel()

// Run checks concurrently
// Cancel context on first failure
```

**Rejected because**:
- Loses complete feedback (want all failures reported)
- Adds cancellation complexity
- Non-deterministic results (which checks run depends on timing)

### 5. Worker Pool

**Description**: Use fixed goroutine pool for all checks

```go
type check func() CheckResult

checks := []check{
    // File checks
    // Command checks
    // Test checks
}

pool := NewWorkerPool(runtime.NumCPU())
results := pool.Execute(checks)
```

**Rejected because**:
- Over-engineered for simple validation
- Obscures execution order
- Adds significant complexity

## Performance Analysis

### Typical Validation Times (Sequential)

| Check Type | Count | Time/Check | Total Time |
|------------|-------|------------|------------|
| File checks | 5 | 10ms | 50ms |
| Test execution | 1 | 2s | 2s |
| Commands | 3 | 500ms | 1.5s |
| **Total** | - | - | **3.55s** |

### Theoretical Parallel Times

| Check Type | Count | Time/Check | Parallel Time |
|------------|-------|------------|---------------|
| File checks | 5 | 10ms | ~10ms (all parallel) |
| Test execution | 1 | 2s | 2s |
| Commands | 3 | 500ms | ~500ms (all parallel) |
| **Total** | - | - | **2.51s** |

**Speedup**: ~1.4x (not significant for human perception)

### Real-World Considerations

- File checks are I/O bound (limited parallelism benefit)
- Test execution dominates total time (cannot parallelize)
- Commands may have side effects (parallelism dangerous)
- Context switching overhead reduces parallel speedup

**Conclusion**: Sequential execution is acceptable given the use case.

## Implementation Notes

**Current Structure**:
```go
func (d *BeadDoD) Validate() (*ValidationResult, error) {
    start := time.Now()
    var allChecks []CheckResult

    // Run all checks sequentially
    allChecks = append(allChecks, d.checkFilesExist()...)
    allChecks = append(allChecks, d.checkTestsPass()...)
    allChecks = append(allChecks, d.checkCommandsSucceed()...)

    // Determine success
    success := true
    var errorMsg string
    for _, check := range allChecks {
        if !check.Success {
            success = false
            if errorMsg == "" {
                errorMsg = check.Error
            }
            break
        }
    }

    return &ValidationResult{
        Success:  success,
        Checks:   allChecks,
        Error:    errorMsg,
        Duration: time.Since(start),
    }, nil
}
```

**Benefits of Current Design**:
- Single pass through checks
- Clear aggregation logic
- Easy to add new check types
- Duration measurement is accurate

## Future Optimization Path

If validation performance becomes a bottleneck, we can:

1. **Add parallel file checks** (low risk, easy win)
2. **Make parallelism optional** (flag or config)
3. **Profile validation** to find actual bottlenecks
4. **Cache file checks** for repeated validations

**Migration Path**:
```go
type ValidationConfig struct {
    ParallelFileChecks bool // Default: false (sequential)
    ParallelCommands   bool // Default: false (sequential)
}

func (d *BeadDoD) ValidateWithConfig(cfg ValidationConfig) (*ValidationResult, error) {
    // Use parallel checks if enabled
}
```

## Design Philosophy

**YAGNI (You Aren't Gonna Need It)**: Don't add concurrency until it's proven necessary.

**Simplicity First**: Start with the simplest implementation that works.

**Optimize Later**: Profile before optimizing. Premature optimization is the root of all evil.

**Debuggability Matters**: Simple code is easier to debug than fast code.

## References

- [Go Concurrency Patterns](https://go.dev/blog/pipelines)
- [Effective Go: Concurrency](https://go.dev/doc/effective_go#concurrency)
- [Dave Cheney: Simplicity](https://dave.cheney.net/2015/03/08/simplicity-and-collaboration)
- [Rob Pike: Simplicity is Complicated](https://www.youtube.com/watch?v=rFejpH_tAHM)
