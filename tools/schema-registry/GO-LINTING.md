# Go Linting Configuration

**Status**: ✅ Configured (Phase 4, Task 4.4)
**Date**: 2026-03-17

## Overview

This document describes the Go linting configuration for corpus-callosum.

## Configuration File

### .golangci.yml

Updated with comprehensive linter suite (matching engram repo standards):

**Essential Linters (Catches Real Bugs):**
- `errcheck` - Checks for unchecked errors
- `govet` - Official Go static analyzer
- `ineffassign` - Detects ineffectual assignments
- `staticcheck` - Advanced static analysis
- `unused` - Finds unused code

**Security:**
- `gosec` - Security audit for Go code

**Recommended (Low False-Positive Rate):**
- `bodyclose` - Checks HTTP response body is closed
- `errorlint` - Ensures proper error wrapping
- `misspell` - Finds commonly misspelled words
- `nilerr` - Finds nil error returns
- `noctx` - Ensures context is used properly
- `revive` - Fast, configurable linter (replacement for golint)
- `unconvert` - Removes unnecessary type conversions
- `unparam` - Finds unused function parameters

**Complexity:**
- `gocyclo` - Cyclomatic complexity checking (threshold: 15)

## Usage

### Linting

```bash
# From corpus-callosum directory
golangci-lint run ./...

# Fix auto-fixable issues
golangci-lint run --fix ./...

# Check specific file
golangci-lint run path/to/file.go
```

### Configuration Details

**errcheck settings:**
- `check-blank: true` - Enforces explicit blank identifier usage for ignored errors

**gocyclo settings:**
- `min-complexity: 15` - Functions with complexity > 15 trigger warnings

**revive rules enabled:**
- blank-imports, context-as-argument, dot-imports
- error-return, error-strings, error-naming
- exported, if-return, increment-decrement
- var-naming, package-comments, range
- receiver-naming, indent-error-flow
- superfluous-else, unreachable-code
- redefines-builtin-id

## Test File Relaxations

Test files (`*_test.go`) have relaxed rules:
- `errcheck` - Warnings instead of errors
- `gosec` - Security checks relaxed
- `unparam` - Unused parameter checks disabled
- `gocyclo` - Complexity checks disabled

This allows for more flexible test code while maintaining strict production standards.

## CI/CD Integration

Add to GitHub Actions workflow:

```yaml
- name: Lint Go code
  run: |
    cd corpus-callosum
    golangci-lint run --max-warnings 0 ./...

- name: Run tests
  run: |
    cd corpus-callosum
    go test ./...
```

## Expected Violations (Before Fixes)

Current status: **Configuration created, violation fixes pending**

Common violations to expect:
- Unchecked errors (errcheck)
- Unused variables/functions (unused)
- HTTP response bodies not closed (bodyclose)
- Ineffectual assignments (ineffassign)
- Security issues (gosec)

Run `golangci-lint run ./...` from the corpus-callosum directory to see all violations.

## Fixing Violations

### Example: errcheck

```go
// Before (error)
result, _ := doSomething()

// After (fixed)
result, err := doSomething()
if err != nil {
    return fmt.Errorf("do something: %w", err)
}
```

### Example: bodyclose

```go
// Before (error)
resp, err := http.Get(url)
if err != nil {
    return err
}

// After (fixed)
resp, err := http.Get(url)
if err != nil {
    return err
}
defer resp.Body.Close()
```

### Example: gosec

```go
// Before (G304 warning)
data, err := os.ReadFile(userInput)

// After (fixed with validation)
if !isValidPath(userInput) {
    return errors.New("invalid path")
}
data, err := os.ReadFile(userInput)
```

## Maintenance

### Adding New Code

1. Write code following Go best practices
2. Run `golangci-lint run ./...` before committing
3. Fix all violations
4. Ensure tests pass: `go test ./...`

### Updating Dependencies

After updating dependencies:
```bash
go mod tidy
golangci-lint run ./...
```

## References

- [golangci-lint Documentation](https://golangci-lint.run/)
- [Go Code Review Comments](https://github.com/golang/go/wiki/CodeReviewComments)
- [Effective Go](https://go.dev/doc/effective_go)

---

**Phase 4 Status**: Task 4.4 ✅ Complete (Configuration) | Violations fixing optional
**Last Updated**: 2026-03-17
