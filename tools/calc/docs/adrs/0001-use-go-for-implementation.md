# ADR 0001: Use Go for Implementation

**Date**: 2024 (inferred from codebase)

**Status**: Accepted

## Context

We need to implement a simple command-line calculator tool that can perform basic arithmetic operations. The tool should be:
- Fast to execute (minimal startup time)
- Easy to distribute (single binary)
- Simple to maintain
- Cross-platform compatible

Several language options were available: Go, Rust, Python, Shell script, or C.

## Decision

We will implement calc using Go.

## Rationale

### Advantages of Go

1. **Static Binary**: Go compiles to a single static binary with no runtime dependencies, making distribution trivial
2. **Fast Startup**: Compiled binaries start instantly, unlike interpreted languages
3. **Standard Library**: Built-in `flag` package handles CLI parsing without external dependencies
4. **Cross-Compilation**: Easy to build for multiple platforms (`GOOS` and `GOARCH` environment variables)
5. **Simplicity**: Go's straightforward syntax matches the tool's simple requirements
6. **Testing**: First-class testing support with `go test`

### Why Not Other Options

- **Python**: Requires Python runtime on target system, slower startup
- **Rust**: Overkill for this simple tool, steeper learning curve, longer compile times
- **Shell Script**: Limited error handling, platform-specific, harder to test
- **C**: Manual memory management unnecessary, more complex build process

## Consequences

### Positive

- Single binary distribution (no installation steps for end users)
- Fast execution suitable for CLI usage
- Excellent tooling (`go build`, `go test`, `go fmt`)
- Easy to read and maintain

### Negative

- Requires Go toolchain for development (but not for end users)
- Binary size larger than shell script (but acceptable for this use case)
- Must recompile for each platform (but Go makes this easy)

### Neutral

- Standard library only (no external dependencies to manage)
- Statically typed (catches errors at compile time)

## Compliance

This decision aligns with common practices for CLI tools in the Go ecosystem (e.g., Docker, kubectl, Terraform).
