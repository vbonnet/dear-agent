# ADR 0004: Single-File Implementation

**Date**: 2024 (inferred from codebase)

**Status**: Accepted

## Context

The calculator implementation could be organized in several ways:

1. **Single file**: All code in `main.go`
2. **Package structure**: Separate packages for operations, validation, CLI
3. **File-per-concern**: `operations.go`, `validation.go`, `main.go`
4. **Internal packages**: `internal/ops`, `internal/validate`, `cmd/calc`

## Decision

We will implement the entire calculator in a single `main.go` file (~115 lines).

## Rationale

### Why Single File

1. **Simplicity**: Entire implementation visible at a glance
2. **Appropriate Scale**:
   - 4 operation functions (~20 lines)
   - 1 validation function (~40 lines)
   - 1 main function (~40 lines)
   - Total: ~115 lines (well below complexity threshold)

3. **No Reusability Needed**:
   - Functions not used outside this program
   - No library extraction planned
   - No shared code with other tools

4. **Easy Navigation**:
   - No need to jump between files
   - Clear top-to-bottom flow
   - All context in one place

5. **Fast Compilation**: Single file compiles instantly

6. **Minimal Cognitive Load**:
   - New contributors see entire implementation
   - No package hierarchy to understand
   - No import paths to navigate

### Why Not Multi-File/Package

**Package structure** (`internal/ops`, etc.):
- Over-engineering for ~100 lines of code
- Adds navigation overhead
- No abstraction benefit (no external consumers)
- Makes simple tool feel complex

**File-per-concern**:
- Useful when files exceed ~500 lines
- Our concerns are ~20-40 lines each
- More ceremony than value
- Splits related code (operations are all similar)

**Internal packages**:
- Appropriate for libraries or complex CLIs
- Unnecessary indirection for this tool
- Would need 4 files for 115 lines of code

## Consequences

### Positive

- One-file readability
- No package management
- Clear code organization (top-to-bottom)
- Easy to copy/modify for similar tools
- Quick onboarding for contributors

### Negative

- All functions in `main` package (not reusable elsewhere)
- Harder to refactor if tool grows significantly
- All tests in one file (but test file is also simple)

### Neutral

- Standard Go pattern for simple commands
- If complexity grows, easy to refactor later

## Code Organization Within File

To maintain clarity, code is organized in logical sections:

```go
// 1. Package and imports (lines 1-8)
package main
import (...)

// 2. Operation functions (lines 10-29)
func add(...)
func subtract(...)
func multiply(...)
func divide(...)

// 3. Validation function (lines 33-70)
func validate(...)

// 4. Main function (lines 74-115)
func main()
```

This top-to-bottom flow matches the call hierarchy:
```
main() → validate() → operations
```

## Refactoring Threshold

Consider splitting into multiple files/packages if:

1. **File exceeds ~300 lines** (indication of complexity growth)
2. **Functions are reused** in other programs (extract to shared package)
3. **Complex state management** emerges (needs separation of concerns)
4. **Multiple contributors** find navigation difficult

**Current status**: None of these conditions apply (115 lines, no reuse, stateless, simple)

## Comparison to Similar Tools

- `bc`: Single C file for core logic
- `dc`: Single C file implementation
- Go stdlib `gofmt`: Multiple packages (complex tool, needs abstraction)
- Go stdlib `go doc`: Multiple packages (complex tool, library reuse)

Our tool matches the "simple command" pattern (like `bc`), not the "complex tool" pattern.

## Related Decisions

- See ADR-0001: Go choice enables concise single-file implementation
- See ADR-0002: Flag-based operations reduce parsing code
- See ADR-0003: float64 uniformity removes type-handling code

All decisions contribute to keeping implementation simple and single-file viable.
