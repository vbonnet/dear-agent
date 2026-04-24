# Contributing to dear-agent

Thank you for your interest in contributing!

## Development Setup

### Prerequisites

- Go 1.25 or later
- tmux (for AGM integration tests)
- Git
- [act](https://github.com/nektos/act) (optional, for local CI)

### Clone and Build

```bash
git clone https://github.com/vbonnet/dear-agent.git
cd dear-agent
```

### Build Products

```bash
# AGM
go build -o bin/agm ./agm/cmd/agm

# Engram
go build -o bin/engram ./engram/cmd/engram

# Wayfinder
go build ./wayfinder/...

# Tools
go build -o bin/benchmark-query ./tools/benchmark-query
```

### Running Tests

```bash
# Run all tests
GOWORK=off go test ./...

# Run tests for a specific product
go test ./agm/...
go test ./engram/...
go test ./wayfinder/...

# Run tests for a shared package
go test ./pkg/costtrack/...
```

### Using the Root Makefile

```bash
# Run full local CI validation (lint + tests) via act
make act-validate

# Run lint only
make act-lint

# Run tests only
make act-test

# Install git pre-push hook
make install-hooks
```

## Pre-commit Hooks

This project includes a pre-push hook that runs local CI checks before pushing.

### Install

```bash
make install-hooks
```

This installs a Git pre-push hook that runs `go vet`, `go test`, and `golangci-lint`
before allowing a push. If any check fails, the push is aborted so you can fix the issue.

### Manual Installation

```bash
cp scripts/hooks/pre-push .git/hooks/pre-push
chmod +x .git/hooks/pre-push
```

### What the Hook Checks

| Check | Command | Purpose |
|-------|---------|---------|
| Vet | `go vet ./...` | Catches common Go mistakes |
| Test | `go test -race ./...` | Runs the full test suite with race detection |
| Lint | `golangci-lint run ./...` | Enforces code style and quality |

## Making Changes

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/my-change`)
3. Make your changes
4. Run tests to ensure nothing is broken
5. Commit with a clear message (use [Conventional Commits](https://www.conventionalcommits.org/))
6. Push and open a Pull Request

## Code Style

- Run `gofmt` on all Go code
- Use `golangci-lint` for linting
- Follow standard Go project layout conventions
- Add package doc comments to new packages (see `pkg/*/doc.go`)

## Repository Layout

| Directory | What belongs here |
|-----------|-------------------|
| `agm/` | AGM session management product |
| `engram/` | Engram memory system product |
| `wayfinder/` | Wayfinder SDLC workflow product |
| `tools/` | Standalone CLI utilities |
| `cmd/` | Additional CLI entry points |
| `codegen/` | Code generation framework |
| `pkg/` | Shared packages (importable by external projects) |
| `internal/` | Private packages (not importable externally) |
| `scripts/` | Build, CI, and utility scripts |
| `docs/` | Cross-cutting documentation |

## Pull Request Process

1. Ensure all tests pass
2. Update documentation if needed
3. Describe what your PR does and why
4. Link any related issues

## License

By contributing, you agree that your contributions will be licensed under the
Apache License 2.0.
