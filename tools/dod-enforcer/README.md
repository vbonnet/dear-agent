# Bead DoD (Definition of Done)

Machine-checkable completion criteria for autonomous bead execution.

## Overview

The DoD package provides a simple YAML-based format for defining when a bead is "done". It supports three types of absolute checks:

1. **Files must exist** - Required files are present
2. **Tests must pass** - Test suite succeeds
3. **Commands must succeed** - Specific commands return expected exit codes

This is the **Phase 0 simple format** for autonomous execution. Phase 2 will add `benchmarks_must_improve` for relative checks.

## Schema

```yaml
files_must_exist:
  - ~/path/to/file1.go
  - ~/path/to/file2.go

tests_must_pass: true

commands_must_succeed:
  - cmd: "go build ./pkg/example"
    exit_code: 0
  - cmd: "go vet ./pkg/example"
    exit_code: 0
```

### Fields

- **files_must_exist** (array of strings): File paths that must exist after completion
  - Supports tilde expansion (`~/`)
  - Supports environment variables (`$HOME`)
  - Can be absolute or relative paths

- **tests_must_pass** (boolean): Whether all tests must pass
  - `true`: Runs `go test ./...` and checks for success (exit code 0)
  - `false`: Skips test execution

- **commands_must_succeed** (array of objects): Commands that must succeed
  - `cmd` (string): Shell command to execute
  - `exit_code` (integer): Expected exit code (usually 0)
  - Commands run with 30-second timeout
  - Commands execute in shell context (supports pipes, `&&`, etc.)

## Usage

### Load and validate a DoD file

```go
import "github.com/vbonnet/engram/pkg/bead/dod"

// Load DoD from YAML file
dodSpec, err := dod.LoadDoD("oss-niv5.dod.yaml")
if err != nil {
    log.Fatalf("Failed to load DoD: %v", err)
}

// Run validation
result, err := dodSpec.Validate()
if err != nil {
    log.Fatalf("Validation error: %v", err)
}

// Check result
if !result.Success {
    log.Printf("DoD validation failed: %s", result.Error)
    for _, check := range result.Checks {
        if !check.Success {
            log.Printf("  [FAIL] %s: %s - %s", check.Type, check.Name, check.Error)
        }
    }
    os.Exit(1)
}

log.Printf("DoD validation passed in %v", result.Duration)
```

### Validation result structure

```go
type ValidationResult struct {
    Success  bool          // Overall pass/fail
    Checks   []CheckResult // Individual check results
    Error    string        // First error message (if any)
    Duration time.Duration // Total validation time
}

type CheckResult struct {
    Type    string // "file", "test", "command"
    Name    string // File path or command string
    Success bool   // Pass/fail for this check
    Error   string // Error message (if failed)
}
```

## Examples

See `examples/` directory for complete DoD files:

- **docs-bead.dod.yaml** - Documentation bead (file checks only)
- **code-bead.dod.yaml** - Code implementation (files + tests + build)
- **test-bead.dod.yaml** - Test creation (files + tests)
- **infra-bead.dod.yaml** - Infrastructure (files + commands)

## File Naming Convention

DoD files should be named: `{bead-id}.dod.yaml`

Example: `oss-niv5.dod.yaml` for bead `oss-niv5`

## Performance

- File checks: < 10ms per file
- Command execution: Limited by command runtime + 30s timeout
- Test execution: Limited by test suite + 60s timeout
- Overall validation: < 5 seconds for typical bead

## Future (Phase 2)

Phase 2 will add support for relative checks:

```yaml
# Phase 2 placeholder (not yet implemented)
benchmarks_must_improve:
  - metric: "api_latency_p95"
    baseline: 250ms
    max_regression: 10%
```

## Error Messages

Validation provides clear, actionable error messages:

```
DoD validation failed: file does not exist
  Required: ./engram/pkg/bead/dod.go
  Expanded: ./engram/pkg/bead/dod.go
  Error: file not found
```

```
DoD validation failed: command exit code mismatch
  Command: go build ./pkg/bead/dod
  Expected exit code: 0
  Actual exit code: 2
  Error: package not found
```

## License

Same as engram project.
