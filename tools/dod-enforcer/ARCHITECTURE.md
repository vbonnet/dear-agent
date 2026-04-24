# Bead Definition of Done (DoD) - Architecture

## System Overview

The bead DoD package implements declarative task completion validation through YAML-defined criteria. It provides a simple, extensible architecture for machine-checkable Definition of Done specifications, enabling automated verification of file existence, test execution, and custom command validations.

## Architectural Principles

1. **Declarative Over Imperative**: DoD specified in YAML, not code
2. **Fail-Fast Validation**: First failure determines overall result
3. **Detailed Feedback**: Individual check results for debugging
4. **Timeout Protection**: All external operations have time limits
5. **Zero Dependencies**: Uses only standard library (except YAML parsing)

## Component Architecture

```
┌─────────────────────────────────────────────────────────┐
│                      LoadDoD()                          │
│         (YAML file → BeadDoD structure)                 │
└────────────────────┬────────────────────────────────────┘
                     │
                     ▼
              ┌─────────────┐
              │   BeadDoD   │
              │  (Data)     │
              └──────┬──────┘
                     │
                     │ Validate()
                     ▼
              ┌─────────────┐
              │  Validator  │
              │ (Orchestrator)
              └──────┬──────┘
                     │
        ┏━━━━━━━━━━━━┻━━━━━━━━━━━━┓
        ▼            ▼             ▼
┌──────────────┐ ┌─────────┐ ┌──────────────┐
│checkFilesExist│checkTests│checkCommands  │
│              │ │Pass     │ │Succeed       │
└──────┬───────┘ └────┬────┘ └──────┬───────┘
       │              │             │
       ▼              ▼             ▼
   expandPath()  executeCommand() executeCommand()
       │              │             │
       └──────────────┴─────────────┘
                      │
                      ▼
               ┌─────────────┐
               │ValidationResult
               │ (Aggregated)│
               └─────────────┘
```

## Component Details

### 1. BeadDoD (Data Structure)

**Responsibility**: Hold DoD specification parsed from YAML

**Structure**:
```go
type BeadDoD struct {
    FilesMustExist      []string       // Required file paths
    TestsMustPass       bool           // Test execution required
    CommandsMustSucceed []CommandCheck // Custom validations
}
```

**Design Decisions**:
- Immutable after loading (no setters)
- YAML field tags for declarative mapping
- Optional fields via pointer types (future: benchmarks)
- Validation logic separate from data structure

### 2. LoadDoD (Parser)

**Responsibility**: Load and parse YAML DoD specifications

**Flow**:
```
Read File → Parse YAML → Validate Structure → Return BeadDoD
```

**Error Handling**:
- File read errors: Wrap with "failed to read DoD file"
- Parse errors: Wrap with "failed to parse DoD YAML"
- Return nil BeadDoD on error

**Design Decisions**:
- Single function for loading (no builder pattern)
- Fail-fast on malformed YAML
- Descriptive error wrapping for debugging
- No validation of field values (e.g., empty arrays OK)

### 3. Validator (Orchestrator)

**Responsibility**: Execute all DoD checks and aggregate results

**Implementation**: `BeadDoD.Validate()` method

**Validation Pipeline**:
```
1. Start timer
2. Run checkFilesExist()      → []CheckResult
3. Run checkTestsPass()        → []CheckResult
4. Run checkCommandsSucceed()  → []CheckResult
5. Aggregate all results       → []CheckResult
6. Determine overall success   → bool
7. Extract first error message → string
8. Stop timer                  → Duration
9. Return ValidationResult
```

**Aggregation Logic**:
```go
success := true
for _, check := range allChecks {
    if !check.Success {
        success = false
        break
    }
}
```

**Design Decisions**:
- Sequential execution (not parallel) for predictable ordering
- Continue all checks even after first failure (complete feedback)
- Overall success requires all checks to pass
- First error message used for summary
- Duration includes all check execution time

### 4. checkFilesExist (File Validator)

**Responsibility**: Verify required files exist

**Algorithm**:
```
For each path in FilesMustExist:
    1. Expand path (tilde, env vars)
    2. Check if file exists (os.Stat)
    3. Record result (success or error)
```

**Path Expansion**:
```go
expandPath(path):
    if starts with "~/":
        replace with UserHomeDir()
    expand environment variables (os.ExpandEnv)
    return expanded path
```

**Error Reporting**:
- Include both original and expanded paths
- Clear message: "file does not exist: {original} (expanded: {expanded})"

**Design Decisions**:
- Expansion happens per check (not during load)
- os.Stat used (checks existence, not readability)
- Empty FilesMustExist returns empty results (valid)
- No content validation (only existence)

### 5. checkTestsPass (Test Validator)

**Responsibility**: Execute Go tests if required

**Algorithm**:
```
If TestsMustPass is false:
    return empty results (skip)

Execute: go test ./...
    - Timeout: 60 seconds
    - Capture output
    - Check exit code

If success:
    return CheckResult{Success: true}
Else:
    return CheckResult{Success: false, Error: details}
```

**Execution Context**:
```go
ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
defer cancel()
cmd := exec.CommandContext(ctx, "go", "test", "./...")
```

**Error Handling**:
- Timeout: "tests timed out after 60 seconds"
- Failure: "tests failed: {err}\nOutput: {output}"
- Combined output (stdout + stderr) for debugging

**Design Decisions**:
- Hardcoded to Go tests (go test ./...)
- Fixed 60-second timeout (not configurable)
- Optional validation (controlled by boolean flag)
- No test result parsing (just pass/fail)

### 6. checkCommandsSucceed (Command Validator)

**Responsibility**: Execute custom commands and validate exit codes

**Algorithm**:
```
For each CommandCheck:
    1. Execute command via shell (sh -c)
    2. Capture exit code and output
    3. Compare exit code to expected
    4. Record result
```

**Command Execution**:
```go
executeCommand(cmdStr, 30*time.Second):
    ctx, cancel := context.WithTimeout(...)
    defer cancel()

    cmd := exec.CommandContext(ctx, "sh", "-c", cmdStr)
    output, err := cmd.CombinedOutput()

    if timeout:
        return -1, output, "command timed out"

    exitCode := extract from ExitError or 0
    return exitCode, output, nil
```

**Exit Code Validation**:
```go
if exitCode != expected:
    error = "command exit code mismatch: expected {expected}, got {actual}"
```

**Design Decisions**:
- Shell execution (sh -c) for flexibility
- 30-second timeout per command (not configurable)
- Exit code comparison (not just zero/non-zero)
- Full output captured for debugging
- Sequential execution (not parallel)

### 7. ValidationResult (Output)

**Responsibility**: Structured validation results

**Structure**:
```go
type ValidationResult struct {
    Success  bool          // Overall pass/fail
    Checks   []CheckResult // Individual check results
    Error    string        // First error message
    Duration time.Duration // Total validation time
}

type CheckResult struct {
    Type    string // "file", "test", "command"
    Name    string // Identifier (path or command)
    Success bool   // Check passed
    Error   string // Error message (if failed)
}
```

**Design Decisions**:
- Self-contained (no external references)
- Detailed enough for debugging
- Concise enough for automation
- Duration for performance monitoring
- First error for quick failure diagnosis

## Data Flow

### Loading Phase

```
YAML File
    │
    ├─ os.ReadFile()
    │       │
    │       ▼
    │   File Content (bytes)
    │       │
    │       ├─ yaml.Unmarshal()
    │       │       │
    │       │       ▼
    │       │   BeadDoD struct
    │       │       │
    │       │       └─ Return
    │       │
    │       └─ Error: Parse failure
    │
    └─ Error: File not found
```

### Validation Phase

```
BeadDoD.Validate()
    │
    ├─ time.Now() → start
    │
    ├─ checkFilesExist()
    │   └─ For each file:
    │       ├─ expandPath()
    │       └─ os.Stat()
    │           ├─ Success → CheckResult{Success: true}
    │           └─ Error → CheckResult{Success: false, Error: msg}
    │
    ├─ checkTestsPass()
    │   └─ If required:
    │       └─ exec.CommandContext("go", "test", "./...")
    │           ├─ Success → CheckResult{Success: true}
    │           └─ Error → CheckResult{Success: false, Error: msg}
    │
    ├─ checkCommandsSucceed()
    │   └─ For each command:
    │       └─ executeCommand(cmd, 30s)
    │           ├─ Exit code matches → CheckResult{Success: true}
    │           └─ Exit code mismatch → CheckResult{Success: false, Error: msg}
    │
    ├─ Aggregate all CheckResults
    │   └─ All pass → Success: true
    │       Any fail → Success: false, Error: first failure
    │
    ├─ time.Since(start) → duration
    │
    └─ Return ValidationResult
```

## Concurrency Model

### No Concurrency

**Design Decision**: All validation runs sequentially in single goroutine

**Rationale**:
- Simplicity over performance
- Predictable execution order
- No race conditions
- No synchronization overhead
- Validation is not performance-critical path

**Future Consideration**: Parallel check execution with sync.WaitGroup

## Error Handling Strategy

### Error Categories

1. **Load Errors**: File I/O, YAML parsing
   - Returned from LoadDoD()
   - Caller must handle (cannot proceed)

2. **Validation Errors**: Checks failed
   - Captured in CheckResult
   - Aggregated in ValidationResult
   - Not returned as Go errors (expected failures)

3. **System Errors**: Command execution, timeouts
   - Captured in CheckResult.Error
   - Treated as validation failures
   - Detailed messages for debugging

### Error Wrapping

```go
// LoadDoD errors
fmt.Errorf("failed to read DoD file: %w", err)
fmt.Errorf("failed to parse DoD YAML: %w", err)

// Validation errors (in CheckResult.Error)
fmt.Sprintf("file does not exist: %s (expanded: %s)", original, expanded)
fmt.Sprintf("tests failed: %s\nOutput: %s", err, output)
fmt.Sprintf("command exit code mismatch: expected %d, got %d", expected, actual)
fmt.Sprintf("command timed out after %v", timeout)
```

### Error Philosophy

**LoadDoD**: Fail-fast with clear errors (cannot validate without spec)

**Validate**: Never returns error (only ValidationResult with Success=false)
- Validation failures are expected outcomes, not exceptional errors
- Error field provides diagnostic information
- Caller checks Success, not error

## Path Handling

### Expansion Strategy

**Tilde Expansion**:
```go
if strings.HasPrefix(path, "~/") {
    home, err := os.UserHomeDir()
    if err != nil {
        return path // fallback: no expansion
    }
    return filepath.Join(home, path[2:])
}
```

**Environment Variable Expansion**:
```go
return os.ExpandEnv(path)
```

**Examples**:
- `~/config.yaml` → `~/config.yaml`
- `$HOME/.env` → `~/.env`
- `${GOPATH}/src` → `~/go/src`
- `/absolute/path` → `/absolute/path` (unchanged)

### Cross-Platform Considerations

- `filepath.Join()` handles platform-specific separators
- `os.UserHomeDir()` works on Windows, Linux, macOS
- `os.ExpandEnv()` supports both $VAR and ${VAR} syntax
- Shell commands use `sh -c` (Unix-biased, may need adjustment for Windows)

## Command Execution

### Security Considerations

**Shell Injection Risk**:
- Commands executed via `sh -c cmdStr`
- No sanitization or escaping
- **Trust Assumption**: DoD files are trusted sources (not user input)
- **Mitigation**: DoD files checked into version control, code review required

**Resource Limits**:
- 30-second timeout per command prevents infinite loops
- 60-second timeout for tests
- Context cancellation ensures cleanup

**Best Practices for DoD Authors**:
- Avoid user input in commands
- Use absolute paths for executables
- Quote arguments with spaces
- Test commands independently before adding to DoD

### Timeout Handling

**Implementation**:
```go
ctx, cancel := context.WithTimeout(context.Background(), timeout)
defer cancel()

cmd := exec.CommandContext(ctx, ...)
output, err := cmd.CombinedOutput()

if ctx.Err() == context.DeadlineExceeded {
    return -1, output, fmt.Errorf("command timed out after %v", timeout)
}
```

**Behavior**:
- Context cancels command on timeout
- Partial output captured
- Clear timeout message generated
- Exit code -1 indicates timeout

## Performance Characteristics

### Time Complexity

- **LoadDoD**: O(n) where n = file size (YAML parsing)
- **checkFilesExist**: O(f) where f = number of files (stat calls)
- **checkTestsPass**: O(1) call, but duration depends on test suite
- **checkCommandsSucceed**: O(c) where c = number of commands

### Space Complexity

- **BeadDoD**: O(f + c) where f = files, c = commands (data only)
- **ValidationResult**: O(f + c + 1) for all check results
- **Command Output**: O(n) where n = output size (transient)

### Performance Characteristics

| Operation | Typical Duration | Max Duration |
|-----------|------------------|--------------|
| LoadDoD | < 10ms | 100ms (large files) |
| checkFilesExist | < 50ms | 500ms (many files) |
| checkTestsPass | 100ms - 10s | 60s (timeout) |
| checkCommandsSucceed | 10ms - 5s | 30s per command |
| **Total Validation** | 1s - 15s | 60s + (30s * commands) |

### Optimization Opportunities

**Current**: Sequential execution
**Future**: Parallel file checks with sync.WaitGroup
**Benefit**: Reduce file check time from O(n) to O(1) with parallelism

## Testing Strategy

### Unit Test Coverage

**Core Functions**:
- `LoadDoD`: Valid/invalid YAML, missing files
- `expandPath`: Tilde, env vars, absolute paths
- `executeCommand`: Success, failure, timeout, exit codes
- `checkFilesExist`: Existing/missing files, expanded paths
- `checkTestsPass`: Skip, success, failure, timeout
- `checkCommandsSucceed`: Various exit codes, timeout

**Test Isolation**:
- Use `t.TempDir()` for file operations
- Create minimal YAML fixtures
- Mock environment variables where needed
- Use fast commands (echo, exit) for command tests

### Integration Testing

**End-to-End Validation**:
1. Create temporary DoD file
2. Create required files
3. Run validation
4. Assert results match expectations

**Negative Testing**:
- Missing files trigger failures
- Command exit code mismatches detected
- Timeouts handled correctly

## Dependencies

### External Dependencies

- `gopkg.in/yaml.v3` - YAML parsing
  - Well-maintained, stable
  - Standard for Go YAML handling
  - Alternative: `github.com/goccy/go-yaml` (faster, but less common)

### Standard Library Dependencies

- `context` - Timeout management
- `fmt` - Error formatting
- `os` - File operations, environment variables
- `os/exec` - Command execution
- `path/filepath` - Path manipulation
- `strings` - String utilities
- `time` - Duration tracking

### System Dependencies

- `go` command (for tests_must_pass)
- `sh` shell (for commands_must_succeed)

## Future Architecture Enhancements

### Planned Features

1. **Benchmark Validation**:
   ```yaml
   benchmarks_must_improve:
     - name: "BenchmarkFoo"
       baseline: "baseline.txt"
       max_regression: "5%"
   ```

2. **Parallel Validation**:
   - Run file checks concurrently
   - Run independent commands in parallel
   - Aggregate results with sync.WaitGroup

3. **Custom Validators**:
   ```yaml
   custom_validators:
     - plugin: "coverage"
       config: {min_coverage: 80}
   ```

4. **Conditional Checks**:
   ```yaml
   commands_must_succeed:
     - cmd: "npm test"
       exit_code: 0
       condition: "exists('package.json')"
   ```

5. **Pre/Post Hooks**:
   ```yaml
   before_validation:
     - "make clean"
   after_validation:
     - "make report"
   ```

### Extensibility Points

**Check Types**: Add new check methods to BeadDoD
**Path Expansion**: Enhance expandPath() with custom logic
**Command Execution**: Support different shells or executors
**Result Formatters**: Add JSON, XML output formats

## Architectural Decision Records

### ADR-001: YAML for DoD Specification

**Decision**: Use YAML for DoD files

**Rationale**:
- Human-readable and writable
- Supports complex structures (arrays, maps)
- Standard in DevOps/CI tooling
- Good library support in Go

**Alternatives Considered**:
- JSON: Less human-friendly, no comments
- TOML: Less common, harder arrays
- Custom DSL: Overkill, poor tooling

### ADR-002: Fail-Fast Validation

**Decision**: Stop validation on first failure (for overall result)

**Rationale**:
- Quick feedback for CI/CD
- Clear failure reason
- Simpler logic

**Trade-off**: Continue all checks for complete feedback (current implementation)

### ADR-003: Zero Validation Errors

**Decision**: Validate() returns (ValidationResult, nil) always

**Rationale**:
- Validation failures are expected outcomes
- Go errors for exceptional conditions only
- Simpler caller logic (check Success field)

**Alternatives**: Return error on validation failure (more Go-idiomatic, but less convenient)

### ADR-004: Sequential Check Execution

**Decision**: Run checks sequentially, not in parallel

**Rationale**:
- Simpler implementation
- Predictable ordering
- Easier debugging
- Performance not critical

**Future**: May parallelize for large file sets

### ADR-005: Hardcoded Timeouts

**Decision**: Fixed 30s/60s timeouts, not configurable

**Rationale**:
- Simpler API
- Sane defaults for most use cases
- Prevents infinite waits

**Future**: May add timeout configuration fields

## References

- [YAML v3 Specification](https://yaml.org/spec/1.2.2/)
- [Go exec Package](https://pkg.go.dev/os/exec)
- [Context-Based Cancellation](https://go.dev/blog/context)
