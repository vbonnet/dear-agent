# Review Instructions

## What Important means here
Reserve Important for findings that would break behavior, leak data, cause a security vulnerability, or break the CI/CD pipeline. Logic errors, unscoped operations, PII exposure, and backward-incompatible changes are Important. Style, naming, and refactoring suggestions are Nit at most.

## Cap the nits
Report at most five Nits per review. If more found, say "plus N similar items" in summary.

## Do not report
- Anything CI already enforces: lint (golangci-lint), formatting (gofmt), type errors
- Generated files, go.sum changes, vendor directory
- Test-only code that intentionally violates production rules (test helpers, mocks)

## Always check
- New exported functions have godoc comments
- Error handling: errors are wrapped with context (fmt.Errorf with %w), not silently dropped
- Concurrency: goroutines have recover(), channels are properly closed, mutexes aren't held across I/O
- No PII in log statements or error messages
- File operations use atomic writes (internal/fileutil) not direct os.WriteFile
- New CLI commands are registered in the Makefile install targets

## Repo-specific rules
- Go is the default language. Python/JS only with strong justification.
- All work happens in worktrees, never in ~/src/
- Force push is blocked at the settings level
- OTel spans should use gen_ai.* attribute naming conventions where applicable
