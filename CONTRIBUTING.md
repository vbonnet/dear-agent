# Contributing to dear-agent

Thanks for your interest in contributing. This document covers development setup, testing requirements, and the PR process.

## Development Setup

### Prerequisites

- Go 1.23 or later
- tmux (for integration tests)
- Git
- golangci-lint

### Clone and Build

```bash
git clone https://github.com/vbonnet/dear-agent.git
cd dear-agent

# Download dependencies
go mod download

# Build the CLI
go build -o agm ./cmd/agm

# Verify the build
./agm --version

# Set up git hooks (required)
make init
```

### Git Hooks

`make init` configures hooks that enforce testing discipline:

- Any commit changing `*.go` files must also change at least one `*_test.go` file
- `t.Skip` calls trigger a warning
- Infrastructure-only changes (`.md`, `Makefile`, `go.mod`, scripts) are exempt

To bypass for legitimate infrastructure-only commits:

```bash
AGM_SKIP_TEST_GATE=1 git commit -m "chore: update CI config"
```

---

## Testing

### Running Tests

```bash
# All tests
go test ./...

# With race detector (always use before submitting a PR)
go test -race ./...

# With coverage
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out

# Specific package
go test ./internal/circuitbreaker/...

# With tmux integration (requires tmux running)
AGM_TEST_TMUX=1 go test ./...
```

### Test Structure

```
dear-agent/
├── cmd/agm/*_test.go           # Command integration tests
├── internal/*/                  # Package tests alongside source
│   ├── package.go
│   └── package_test.go
└── test/
    ├── bdd/                    # Behavior-driven tests (Gherkin)
    ├── e2e/                    # End-to-end testscript tests
    └── integration/            # Multi-component integration tests
```

### Test Requirements

All new code requires tests. This is not optional or deferrable.

**Unit tests** — test individual functions and packages in isolation. Use table-driven tests:

```go
func TestCircuitBreakerLevel(t *testing.T) {
    tests := []struct {
        name    string
        load    float64
        workers int
        want    DEARLevel
    }{
        {"below threshold", 30.0, 1, LevelGreen},
        {"yellow zone", 50.0, 2, LevelYellow},
        {"red zone", 80.0, 3, LevelRed},
        {"emergency", 110.0, 5, LevelEmergency},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            cb := NewCircuitBreaker(defaultConfig())
            got := cb.Evaluate(tt.load, tt.workers)
            if got != tt.want {
                t.Errorf("got %v, want %v", got, tt.want)
            }
        })
    }
}
```

**Integration tests** — test multiple components working together:

```go
func TestSessionLifecycle(t *testing.T) {
    session := createTestSession(t, "test-session")
    defer cleanupSession(t, session)

    manifest, err := readManifest(session.Path())
    require.NoError(t, err)
    assert.Equal(t, "test-session", manifest.Name)
}
```

**Regression tests** — for every bug fix, write a test that fails before the fix and passes after. Document the bug in the test name.

### Coverage Goals

- Overall: 80%+
- Critical packages (`circuitbreaker`, `session`, `tmux`, `ops`): 75%+
- All packages: 50%+

Coverage is a floor, not a ceiling. Focus on tests that catch real failure modes.

---

## Code Quality

### Formatting

```bash
go fmt ./...
```

### Linting

All code must pass linting before a PR will be reviewed:

```bash
golangci-lint run
```

Linters enabled: `errcheck`, `govet`, `staticcheck`, `unused`, `gosec`, `bodyclose`, `errorlint`, `misspell`, `revive`, `gocyclo` (threshold 15), and others. See `.golangci.yml`.

Auto-fix what you can:

```bash
golangci-lint run --fix
```

### Error Handling

- Check all errors
- Return errors up the call stack; do not log-and-continue except at CLI boundaries
- Use structured error types (see `internal/ops/errors.go`)
- Include context in error messages: `fmt.Errorf("session %q: %w", name, err)`

---

## Commit Messages

Conventional commit format:

```
<type>(<scope>): <subject>

<body>
```

Types: `feat`, `fix`, `docs`, `test`, `refactor`, `perf`, `chore`

Good example:

```
fix(circuitbreaker): evaluate load before checking worker count

Previously the check order meant RED-level load could still allow
new spawns if the worker count was below the cap. Load check must
gate before all other conditions.

Fixes #47
```

---

## Pull Request Process

1. **Open an issue first** for non-trivial changes — discuss the approach before writing code
2. **Fork the repo** and create a feature branch: `feat/your-feature` or `fix/the-bug`
3. **Write code and tests together** — do not submit code without tests
4. **Run the full suite locally**:
   ```bash
   go test -race ./...
   golangci-lint run
   ```
5. **Submit the PR** against `main`
6. **CI must be green** — tests, linting, and build all pass
7. **Address review feedback** — maintainers may request changes or ask for more tests
8. **Squash if requested** — the project prefers a clean linear history

---

## Design Principles

Before adding a new feature, ask:

- **Does it belong in Define, Enforce, Audit, or Resolve?** If it doesn't fit the DEAR loop, it probably doesn't belong in the harness.
- **Is it adding a check or adding a bypass?** Bypasses are bugs.
- **What does it add to the closed loop?** Features that don't feed back into loop improvement are UI sugar at best.

See [ARCHITECTURE.md](agm/ARCHITECTURE.md) and the ADRs in [agm/docs/adr/](agm/docs/adr/) for design rationale.

---

## Questions

Open a GitHub issue. Check existing issues and PRs first — the question may already be answered.

## License

By contributing, you agree that your contributions will be licensed under the Apache 2.0 License.
