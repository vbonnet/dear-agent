# Bead Definition of Done (DoD) - Specification

## Overview

The bead DoD package provides machine-checkable Definition of Done validation for bead task completion. It enables declarative specification of completion criteria through YAML files, including file existence checks, test execution requirements, and custom command validations.

## Purpose

**Primary Goal**: Enable automated, objective verification of bead task completion through machine-checkable criteria.

**Key Capabilities**:
- YAML-based DoD specification
- File existence validation with path expansion
- Test execution requirements
- Custom command validation with exit code checks
- Structured validation results with detailed check outcomes
- Extensible validation framework

## Functional Requirements

### FR-1: DoD Definition

The system SHALL support YAML-based Definition of Done specifications:

- **FR-1.1**: Define required files via `files_must_exist` array
- **FR-1.2**: Specify test execution requirement via `tests_must_pass` boolean
- **FR-1.3**: Define custom command validations via `commands_must_succeed` array
- **FR-1.4**: Support future extension fields (e.g., benchmarks_must_improve)
- **FR-1.5**: Parse YAML files into structured BeadDoD objects

### FR-2: File Existence Validation

The system SHALL validate required files exist:

- **FR-2.1**: Check existence of files specified in `files_must_exist`
- **FR-2.2**: Expand tilde (~/) paths to user home directory
- **FR-2.3**: Expand environment variables in paths
- **FR-2.4**: Report missing files with original and expanded paths
- **FR-2.5**: Pass check if all required files exist

### FR-3: Test Execution Validation

The system SHALL validate test execution when required:

- **FR-3.1**: Execute `go test ./...` when `tests_must_pass` is true
- **FR-3.2**: Skip test validation when `tests_must_pass` is false
- **FR-3.3**: Apply 60-second timeout for test execution
- **FR-3.4**: Report timeout failures with clear error message
- **FR-3.5**: Capture and report test output on failure
- **FR-3.6**: Pass check if tests complete successfully within timeout

### FR-4: Command Validation

The system SHALL validate custom command execution:

- **FR-4.1**: Execute commands specified in `commands_must_succeed`
- **FR-4.2**: Validate actual exit code matches expected exit code
- **FR-4.3**: Apply 30-second timeout per command
- **FR-4.4**: Report timeout failures with duration
- **FR-4.5**: Capture and report command output on failure
- **FR-4.6**: Pass check if exit code matches and completes within timeout

### FR-5: Validation Orchestration

The system SHALL orchestrate all validation checks:

- **FR-5.1**: Execute all checks in sequence (files, tests, commands)
- **FR-5.2**: Aggregate all check results
- **FR-5.3**: Determine overall success (all checks pass)
- **FR-5.4**: Measure total validation duration
- **FR-5.5**: Return structured ValidationResult
- **FR-5.6**: Include first error message in result summary

### FR-6: Validation Results

The system SHALL provide detailed validation results:

- **FR-6.1**: Overall success/failure status
- **FR-6.2**: Total validation duration
- **FR-6.3**: Individual check results with:
  - Check type (file, test, command)
  - Check name (path or command)
  - Success status
  - Error message (if failed)
- **FR-6.4**: Summary error message from first failure

### FR-7: DoD File Loading

The system SHALL load DoD specifications from files:

- **FR-7.1**: Read YAML files from specified paths
- **FR-7.2**: Parse YAML into BeadDoD structure
- **FR-7.3**: Report file read errors with clear messages
- **FR-7.4**: Report YAML parse errors with details
- **FR-7.5**: Return validated BeadDoD object

## Non-Functional Requirements

### NFR-1: Performance

- **NFR-1.1**: File existence checks SHALL complete in < 100ms per file
- **NFR-1.2**: Test execution SHALL timeout after 60 seconds
- **NFR-1.3**: Command execution SHALL timeout after 30 seconds
- **NFR-1.4**: Total validation SHALL report accurate duration
- **NFR-1.5**: No memory leaks from command execution

### NFR-2: Reliability

- **NFR-2.1**: Path expansion SHALL handle missing HOME gracefully
- **NFR-2.2**: Command failures SHALL not crash validation
- **NFR-2.3**: Timeout mechanisms SHALL reliably cancel execution
- **NFR-2.4**: Context cancellation SHALL clean up resources
- **NFR-2.5**: Test failures SHALL capture complete output

### NFR-3: Usability

- **NFR-3.1**: Error messages SHALL include both original and expanded paths
- **NFR-3.2**: Timeout errors SHALL specify duration
- **NFR-3.3**: Command failures SHALL include exit code mismatch details
- **NFR-3.4**: YAML parse errors SHALL be human-readable
- **NFR-3.5**: Validation results SHALL be self-explanatory

### NFR-4: Compatibility

- **NFR-4.1**: Support YAML v3 format
- **NFR-4.2**: Work on Linux, macOS, and Windows
- **NFR-4.3**: Support Go 1.18+ for test execution
- **NFR-4.4**: Execute commands via shell (sh -c)
- **NFR-4.5**: Handle both Unix and Windows path separators

### NFR-5: Maintainability

- **NFR-5.1**: DoD structure SHALL be extensible without breaking changes
- **NFR-5.2**: Check types SHALL be easily identifiable in results
- **NFR-5.3**: Path expansion logic SHALL be isolated and testable
- **NFR-5.4**: Command execution SHALL be isolated and testable
- **NFR-5.5**: Validation logic SHALL be testable without actual files

## API Specification

### Core Functions

```go
// Load a DoD specification from a YAML file
func LoadDoD(path string) (*BeadDoD, error)

// Validate all DoD criteria and return results
func (d *BeadDoD) Validate() (*ValidationResult, error)
```

### Data Structures

```go
// BeadDoD represents the Definition of Done for a bead
type BeadDoD struct {
    FilesMustExist      []string       `yaml:"files_must_exist"`
    TestsMustPass       bool           `yaml:"tests_must_pass"`
    CommandsMustSucceed []CommandCheck `yaml:"commands_must_succeed"`
}

// CommandCheck represents a command that must succeed
type CommandCheck struct {
    Cmd      string `yaml:"cmd"`
    ExitCode int    `yaml:"exit_code"`
}

// ValidationResult contains the outcome of DoD validation
type ValidationResult struct {
    Success  bool
    Checks   []CheckResult
    Error    string
    Duration time.Duration
}

// CheckResult represents the result of a single check
type CheckResult struct {
    Type    string // "file", "test", "command"
    Name    string // file path or command
    Success bool
    Error   string
}
```

### Helper Functions (Internal)

```go
// expandPath expands tilde and environment variables in a path
func expandPath(path string) string

// executeCommand runs a command with a timeout
func executeCommand(cmdStr string, timeout time.Duration) (int, string, error)

// checkFilesExist verifies required files exist
func (d *BeadDoD) checkFilesExist() []CheckResult

// checkTestsPass runs tests if required
func (d *BeadDoD) checkTestsPass() []CheckResult

// checkCommandsSucceed executes commands and validates exit codes
func (d *BeadDoD) checkCommandsSucceed() []CheckResult
```

## Usage Patterns

### Pattern 1: Basic DoD Validation

```go
dod, err := dod.LoadDoD("task.dod.yaml")
if err != nil {
    log.Fatalf("Failed to load DoD: %v", err)
}

result, err := dod.Validate()
if err != nil {
    log.Fatalf("Validation error: %v", err)
}

if !result.Success {
    log.Printf("DoD validation failed: %s", result.Error)
    for _, check := range result.Checks {
        if !check.Success {
            log.Printf("  [%s] %s: %s", check.Type, check.Name, check.Error)
        }
    }
}
```

### Pattern 2: Pre-Commit Hook

```go
// .git/hooks/pre-commit
dod, err := dod.LoadDoD(".bead.dod.yaml")
if err != nil {
    os.Exit(0) // No DoD file, allow commit
}

result, _ := dod.Validate()
if !result.Success {
    fmt.Fprintf(os.Stderr, "DoD validation failed:\n%s\n", result.Error)
    os.Exit(1)
}
```

### Pattern 3: CI/CD Integration

```go
// Validate DoD in CI pipeline
dod, err := dod.LoadDoD(os.Getenv("DOD_FILE"))
if err != nil {
    fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
    os.Exit(1)
}

result, err := dod.Validate()
if err != nil {
    fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
    os.Exit(1)
}

if !result.Success {
    fmt.Printf("::error::DoD validation failed: %s\n", result.Error)
    os.Exit(1)
}

fmt.Printf("DoD validation passed in %v\n", result.Duration)
```

### Pattern 4: Detailed Reporting

```go
result, _ := dod.Validate()

fmt.Printf("Validation Result: %s (%.2fs)\n",
    map[bool]string{true: "PASS", false: "FAIL"}[result.Success],
    result.Duration.Seconds())

for _, check := range result.Checks {
    status := "PASS"
    if !check.Success {
        status = "FAIL"
    }
    fmt.Printf("  [%s] %s: %s\n", status, check.Type, check.Name)
    if check.Error != "" {
        fmt.Printf("    Error: %s\n", check.Error)
    }
}
```

## YAML Specification Format

### Schema

```yaml
files_must_exist:
  - path/to/file1.go
  - ~/config/settings.yaml
  - $HOME/.env

tests_must_pass: true

commands_must_succeed:
  - cmd: "make lint"
    exit_code: 0
  - cmd: "golangci-lint run"
    exit_code: 0
  - cmd: "test -f output.txt"
    exit_code: 0
```

### Field Descriptions

**files_must_exist** (array of strings):
- List of file paths that must exist
- Supports tilde expansion (~/)
- Supports environment variable expansion ($VAR, ${VAR})
- Relative paths resolved from DoD file location

**tests_must_pass** (boolean):
- true: Execute `go test ./...` and require success
- false: Skip test validation

**commands_must_succeed** (array of CommandCheck):
- cmd (string): Shell command to execute
- exit_code (int): Expected exit code (usually 0)

## Constraints and Assumptions

### Constraints

- **C-1**: Requires Go toolchain for test execution
- **C-2**: Requires shell (sh) for command execution
- **C-3**: File paths resolved relative to current working directory
- **C-4**: Commands executed with 30-second timeout
- **C-5**: Tests executed with 60-second timeout

### Assumptions

- **A-1**: DoD files use .dod.yaml extension by convention
- **A-2**: Go projects use standard `go test ./...` for testing
- **A-3**: Commands are idempotent and safe to execute
- **A-4**: Validation runs from repository root or work directory
- **A-5**: File existence checks do not verify content

## Error Handling

### Error Categories

1. **File I/O Errors**: DoD file not found, permission denied
2. **Parse Errors**: Invalid YAML syntax, type mismatches
3. **Validation Errors**: Tests failed, commands failed, files missing
4. **Timeout Errors**: Tests or commands exceed time limits
5. **System Errors**: Command execution failures, context cancellation

### Error Strategies

- **LoadDoD**: Return error for file read or parse failures
- **Validate**: Return ValidationResult with Success=false, never error
- **Individual Checks**: Capture errors in CheckResult, continue validation
- **Timeouts**: Treat as validation failure with clear timeout message

## Testing Requirements

### Unit Tests

- **T-1**: LoadDoD parsing with valid YAML
- **T-2**: LoadDoD error handling for invalid files
- **T-3**: checkFilesExist with existing and missing files
- **T-4**: checkTestsPass with passing and failing tests
- **T-5**: checkCommandsSucceed with various exit codes
- **T-6**: expandPath with tilde and environment variables
- **T-7**: executeCommand with timeouts and cancellation
- **T-8**: Validate aggregation and result structure

### Integration Tests

- **T-9**: End-to-end validation with complete DoD file
- **T-10**: Validation with real go test execution
- **T-11**: Validation with actual file creation
- **T-12**: Timeout behavior verification

## Dependencies

- `gopkg.in/yaml.v3` - YAML parsing
- Standard library only (os, exec, context, time, path/filepath, strings)

## Future Considerations

- **F-1**: Benchmark validation (benchmarks_must_improve)
- **F-2**: Code coverage requirements (min_coverage: 80%)
- **F-3**: Linting requirements (lint_must_pass: true)
- **F-4**: Git commit message validation
- **F-5**: Documentation completeness checks
- **F-6**: Dependency vulnerability scanning
- **F-7**: Performance regression detection
- **F-8**: Custom validator plugins
