# Workspace Detection Library - Testing Guide

Quick reference for running and understanding the test suite.

## Quick Start

```bash
# Navigate to package directory
cd pkg/workspace

# Run all tests
go test -v

# Run with coverage
go test -v -cover

# Generate HTML coverage report
go test -coverprofile=coverage.out
go tool cover -html=coverage.out -o coverage.html
```

## Running Specific Tests

```bash
# Run only detector tests
go test -v -run TestDetect

# Run only config tests
go test -v -run TestLoadConfig
go test -v -run TestValidateConfig

# Run only path tests
go test -v -run TestExpandHome
go test -v -run TestNormalizePath
go test -v -run TestIsSubpath

# Run only interactive tests
go test -v -run TestPrompt
```

## Test Organization

### detector_test.go
Tests the 6-priority workspace detection algorithm:
1. Explicit flag (`--workspace`)
2. Environment variable (`WORKSPACE`)
3. Auto-detection from PWD
4. Default workspace
5. Interactive prompt
6. Error fallback

Key functions tested:
- `Detect()` - Main detection logic
- `DetectWithEnv()` - Custom env var
- `GetWorkspace()` - Lookup by name
- `ListWorkspaces()` - List all
- `matchWorkspace()` - Path matching

### config_test.go
Tests configuration loading and validation:
- YAML parsing
- Path expansion (tilde, env vars)
- Validation rules
- Config saving

Key functions tested:
- `LoadConfig()` - Load from file
- `SaveConfig()` - Save to file
- `ValidateConfig()` - Validation
- `ExpandPaths()` - Path expansion
- `GetDefaultConfigPath()` - Default location

### paths_test.go
Tests path manipulation utilities:
- Home directory expansion
- Path normalization
- Subpath checking
- Absolute path validation

Key functions tested:
- `ExpandHome()` - Tilde expansion
- `NormalizePath()` - Full normalization
- `IsSubpath()` - Parent/child checking
- `ValidateAbsolutePath()` - Validation

### interactive_test.go
Tests interactive user prompts:
- Workspace selection
- Yes/No confirmations
- TTY vs non-TTY behavior

Key functions tested:
- `PromptWorkspace()` - Select workspace
- `PromptConfirm()` - Yes/No prompt

## Test Patterns

### Table-Driven Tests
Many tests use table-driven patterns for multiple scenarios:

```go
tests := []struct {
    name     string
    input    string
    expected string
}{
    {"case 1", "input1", "expected1"},
    {"case 2", "input2", "expected2"},
}

for _, tt := range tests {
    t.Run(tt.name, func(t *testing.T) {
        // Test logic
    })
}
```

### Temporary Directories
Filesystem tests use `t.TempDir()` for automatic cleanup:

```go
tmpDir := t.TempDir()  // Auto-cleaned after test
configPath := filepath.Join(tmpDir, "config.yaml")
```

### Mocked I/O
Interactive tests mock stdin/stdout:

```go
stdin := strings.NewReader("1\n")
stdout := &bytes.Buffer{}
prompter := NewPrompterWithIO(stdin, stdout, true)
```

## Coverage Goals

| Component | Target Coverage | Priority |
|-----------|----------------|----------|
| Detector | 95%+ | High |
| Config | 90%+ | High |
| Paths | 95%+ | High |
| Interactive | 85%+ | Medium |

## Benchmarks

```bash
# Run performance benchmarks
go test -bench=. -benchmem

# Benchmark specific functions
go test -bench=BenchmarkNormalizePath
go test -bench=BenchmarkIsSubpath
```

## Common Issues

### Test Failures
1. **Path separators**: Tests assume Unix-like paths (`/`)
2. **HOME not set**: Some tests require `$HOME` environment variable
3. **Permissions**: Temp directory creation needs write access

### Skipped Tests
Some tests may skip on certain platforms:
```
=== SKIP TestIsSubpath_SymlinkHandling
    Symlinks not supported on this platform
```

## Test Data

Sample configs in `testdata/`:
- `valid_config.yaml` - Standard multi-workspace config
- `invalid_version.yaml` - Bad version number
- `duplicate_names.yaml` - Duplicate workspace names
- `no_workspaces.yaml` - Empty workspaces
- `minimal_config.yaml` - Minimal valid config
- `with_env_vars.yaml` - Environment variable usage

## Integration with CI

```bash
# Typical CI test command
go test -v -race -coverprofile=coverage.out -covermode=atomic ./...

# Check coverage threshold
go tool cover -func=coverage.out | grep total | awk '{print $3}' | sed 's/%//'
```

## Debugging Tests

```bash
# Run single test with verbose output
go test -v -run TestDetect_Priority1_ExplicitFlag

# Run with race detector
go test -race -run TestDetect

# Print test output even on success
go test -v -run TestDetect 2>&1 | tee test.log
```

## Test Maintenance

When modifying code:
1. Run all tests: `go test -v`
2. Check coverage: `go test -cover`
3. Update tests if behavior changes
4. Add new tests for new features
5. Ensure table-driven tests cover edge cases

## Continuous Integration

Example GitHub Actions workflow:

```yaml
- name: Run tests
  run: |
    cd core/pkg/workspace
    go test -v -race -coverprofile=coverage.out

- name: Upload coverage
  uses: codecov/codecov-action@v3
  with:
    files: ./coverage.out
```

## Additional Resources

- [Go Testing Best Practices](https://go.dev/doc/tutorial/add-a-test)
- [Table Driven Tests](https://dave.cheney.net/2019/05/07/prefer-table-driven-tests)
- [Test Coverage](https://go.dev/blog/cover)
