# ADR-005: Hardcoded Timeout Values

## Status

Accepted

## Context

The DoD validation executes potentially long-running operations:

1. **Test execution**: `go test ./...` can take seconds to minutes
2. **Custom commands**: Build, lint, and other commands vary in duration

These operations need timeouts to prevent infinite waits and resource exhaustion. There are several approaches to timeout configuration:

1. **Hardcoded Timeouts**: Fixed values in code (60s tests, 30s commands)
2. **Configurable Timeouts**: Allow DoD authors to specify timeouts
3. **Adaptive Timeouts**: Learn from past executions, adjust dynamically
4. **No Timeouts**: Let operations run indefinitely

We need to decide how to handle operation timeouts.

## Decision

We will use **hardcoded timeout values**:

- **Test execution**: 60 seconds
- **Command execution**: 30 seconds per command

**Implementation**:
```go
// Fixed timeout for test execution
ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)

// Fixed timeout for command execution
exitCode, output, err := executeCommand(cmdStr, 30*time.Second)
```

**Rationale**:
- Simpler API (no timeout configuration)
- Sane defaults for 95% of use cases
- Prevents runaway processes
- Easier to document and understand

## Consequences

### Positive

1. **Simplicity**: No configuration needed
   ```yaml
   # No timeout fields required
   tests_must_pass: true
   commands_must_succeed:
     - cmd: "make lint"
       exit_code: 0
   ```

2. **Sane Defaults**: Values work for most projects
   - 60s sufficient for typical test suites
   - 30s sufficient for build, lint, format checks
   - Prevents hangs without being too restrictive

3. **Predictable Behavior**: Same timeout across all projects
   - Consistent CI/CD behavior
   - No surprises from different configurations
   - Easier to debug timeout issues

4. **No Configuration Burden**: DoD authors don't need to tune timeouts
   - One less thing to configure
   - Faster DoD file creation
   - Less trial-and-error

5. **Clear Error Messages**: Timeout value known at error time
   ```
   tests timed out after 60 seconds
   command timed out after 30 seconds
   ```

### Negative

1. **Inflexible**: Cannot handle slow operations
   - Large test suites may exceed 60s
   - Complex builds may exceed 30s
   - Integration tests may be slow
   - **Workaround**: Split large test suites, optimize slow operations

2. **One-Size-Fits-All**: Different projects have different needs
   - Microservices may need < 10s
   - Legacy codebases may need > 5min
   - Cannot optimize for specific contexts

3. **No Per-Command Tuning**: All commands get same timeout
   - Fast commands (lint) wait as long as slow commands (build)
   - Cannot prioritize critical checks
   - Cannot express urgency/priority

4. **Potential False Failures**: Legitimate slow operations fail
   - CI environment slower than local
   - Cold cache vs warm cache
   - Network-dependent tests
   - **Impact**: Developers must optimize or work around

5. **No Escape Hatch**: Cannot override even when necessary
   - Stuck with 60s test limit
   - Cannot accommodate special cases
   - May prevent DoD adoption for slow projects

## Alternatives Considered

### 1. Configurable Timeouts

**Description**: Allow timeout specification in DoD files

```yaml
tests_must_pass: true
test_timeout: 300s  # 5 minutes

commands_must_succeed:
  - cmd: "make build"
    exit_code: 0
    timeout: 120s  # 2 minutes
  - cmd: "make lint"
    exit_code: 0
    timeout: 10s   # 10 seconds
```

**Rejected because**:
- Adds complexity to DoD schema
- More configuration burden on users
- Different timeout syntax (duration strings)
- Requires parsing and validation
- Most users would use defaults anyway

**Future consideration**: May add if hardcoded timeouts prove insufficient

### 2. Adaptive Timeouts

**Description**: Learn from execution history, adjust automatically

```go
type TimeoutLearner struct {
    history map[string][]time.Duration
}

func (t *TimeoutLearner) GetTimeout(cmd string) time.Duration {
    avg := average(t.history[cmd])
    return avg * 1.5  // 50% buffer
}
```

**Rejected because**:
- Much more complex implementation
- Requires persistent state
- Unpredictable behavior (changes over time)
- Hard to debug timeout issues
- Overkill for simple validation

### 3. No Timeouts

**Description**: Let operations run indefinitely

```go
cmd := exec.Command("go", "test", "./...")
cmd.Run()  // No context, no timeout
```

**Rejected because**:
- Risk of infinite hangs
- Bad CI/CD experience (jobs hang forever)
- Resource exhaustion (zombie processes)
- Cannot detect stuck operations

### 4. Global Timeout Configuration

**Description**: Single timeout in environment variable or config file

```bash
export DOD_TIMEOUT=120s
```

```go
timeout := os.Getenv("DOD_TIMEOUT")
if timeout == "" {
    timeout = "60s"
}
```

**Rejected because**:
- Still requires configuration
- Per-environment differences (local vs CI)
- Hard to discover (not in DoD file)
- Affects all operations equally

### 5. Progressive Timeouts

**Description**: Start with short timeout, retry with longer timeout on failure

```go
timeouts := []time.Duration{30*time.Second, 60*time.Second, 120*time.Second}
for _, timeout := range timeouts {
    err := runWithTimeout(cmd, timeout)
    if err != context.DeadlineExceeded {
        return err
    }
}
```

**Rejected because**:
- Wasteful (reruns on timeout)
- Unpredictable duration (3x longer)
- Complex retry logic
- Confusing error messages

## Timeout Rationale

### Test Execution: 60 Seconds

**Why 60s**:
- Long enough for most unit test suites
- Go test suites typically < 30s
- Allows for slower CI environments
- Prevents infinite loops in tests

**Not suitable for**:
- Integration tests (may need minutes)
- End-to-end tests (may need hours)
- **Solution**: Use custom commands for slow tests

```yaml
# Workaround for slow tests
commands_must_succeed:
  - cmd: "timeout 300s go test -tags=integration ./..."
    exit_code: 0
```

### Command Execution: 30 Seconds

**Why 30s**:
- Sufficient for linters (golangci-lint, eslint)
- Sufficient for formatters (gofmt, prettier)
- Sufficient for simple builds (small projects)
- Prevents runaway commands

**Not suitable for**:
- Large builds (may need minutes)
- Complex code generation
- Database migrations
- **Solution**: Use `timeout` wrapper command

```yaml
commands_must_succeed:
  - cmd: "timeout 180s make build"
    exit_code: 0
```

## Workarounds for Slow Operations

### 1. Use External Timeout Command

```yaml
commands_must_succeed:
  # GNU timeout command (Linux, macOS with coreutils)
  - cmd: "timeout 300s go test -tags=integration ./..."
    exit_code: 0

  # Fallback for macOS without coreutils
  - cmd: "gtimeout 300s ./slow-script.sh"
    exit_code: 0
```

### 2. Split Large Test Suites

```yaml
# Instead of single slow test run
tests_must_pass: false  # Disable built-in test check

# Use multiple fast command checks
commands_must_succeed:
  - cmd: "go test ./pkg/module1/..."
    exit_code: 0
  - cmd: "go test ./pkg/module2/..."
    exit_code: 0
  - cmd: "go test ./pkg/module3/..."
    exit_code: 0
```

### 3. Optimize Slow Operations

- Cache dependencies (go mod download)
- Use parallel test execution (go test -p)
- Skip slow tests in DoD validation
- Run integration tests separately

### 4. Use Separate DoD Files

```yaml
# .bead.dod.yaml - Fast checks
files_must_exist: [...]
tests_must_pass: true

# .bead.dod.full.yaml - Complete checks (manual)
files_must_exist: [...]
commands_must_succeed:
  - cmd: "timeout 600s make integration-test"
    exit_code: 0
```

## Implementation Details

**Test Timeout**:
```go
ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
defer cancel()

cmd := exec.CommandContext(ctx, "go", "test", "./...")
output, err := cmd.CombinedOutput()

if ctx.Err() == context.DeadlineExceeded {
    check.Success = false
    check.Error = "tests timed out after 60 seconds"
}
```

**Command Timeout**:
```go
func executeCommand(cmdStr string, timeout time.Duration) (int, string, error) {
    ctx, cancel := context.WithTimeout(context.Background(), timeout)
    defer cancel()

    cmd := exec.CommandContext(ctx, "sh", "-c", cmdStr)
    output, err := cmd.CombinedOutput()

    if ctx.Err() == context.DeadlineExceeded {
        return -1, string(output), fmt.Errorf("command timed out after %v", timeout)
    }
    // ...
}
```

**Timeout Detection**:
- Use `context.WithTimeout()` for automatic cancellation
- Check `ctx.Err() == context.DeadlineExceeded` after execution
- Return clear error message with timeout duration

## Future Enhancements

### 1. Add Timeout Configuration (If Needed)

```yaml
test_timeout: 300s

commands_must_succeed:
  - cmd: "make build"
    exit_code: 0
    timeout: 120s
```

**Implementation**:
```go
type BeadDoD struct {
    TestTimeout         *time.Duration `yaml:"test_timeout,omitempty"`
    CommandsMustSucceed []CommandCheck `yaml:"commands_must_succeed"`
}

type CommandCheck struct {
    Cmd      string         `yaml:"cmd"`
    ExitCode int            `yaml:"exit_code"`
    Timeout  *time.Duration `yaml:"timeout,omitempty"`
}

// Use configured timeout or default
timeout := 60 * time.Second
if dod.TestTimeout != nil {
    timeout = *dod.TestTimeout
}
```

### 2. Environment Variable Override

```bash
DOD_TEST_TIMEOUT=300s
DOD_COMMAND_TIMEOUT=60s
```

```go
func getTestTimeout() time.Duration {
    if env := os.Getenv("DOD_TEST_TIMEOUT"); env != "" {
        if d, err := time.ParseDuration(env); err == nil {
            return d
        }
    }
    return 60 * time.Second
}
```

### 3. Per-Project Configuration File

```yaml
# .dod-config.yaml
default_test_timeout: 120s
default_command_timeout: 45s
```

## Design Philosophy

**Sane Defaults**: 80% of users should never need to configure timeouts.

**YAGNI**: Don't add configuration until proven necessary.

**Fail-Fast**: Better to timeout too early than hang forever.

**Escape Hatches**: Workarounds exist (`timeout` command, custom commands).

**Progressive Enhancement**: Can add configuration later without breaking changes.

## References

- [Go Context Package](https://pkg.go.dev/context)
- [GNU Timeout Command](https://man7.org/linux/man-pages/man1/timeout.1.html)
- [Martin Fowler: Yagni](https://martinfowler.com/bliki/Yagni.html)
