# Contributing to AGM

Thank you for your interest in contributing to AI/Agent Gateway Manager (AGM)!

*Note: This project was renamed from AGM (Agent Session Manager) to AGM in 2026-02.*

## Development Setup

### Prerequisites

- Go 1.24 or later
- tmux (for integration tests)
- Git

### Clone and Build

```bash
# Clone the repository
git clone https://github.com/vbonnet/ai-tools.git
cd ai-tools/agm

# Install dependencies
go mod download

# Build AGM
go build -o agm ./cmd/agm

# Run tests
go test ./...

# Set up git hooks (required)
make init
```

### Git Hooks — Test Enforcement Gates

Running `make init` configures git hooks that **block commits and merges** when Go
source files (`*.go`) are changed without corresponding test files (`*_test.go`).

**What the hooks enforce:**
- Any commit/merge changing `*.go` files (excluding `*_test.go`) must also include
  at least one `*_test.go` file change
- `t.Skip` calls in test files trigger a warning (not a block)
- `TODO test` / `deferred test` patterns in code trigger a warning (not a block)
- Infrastructure-only changes (`.md`, `.sh`, `Makefile`, `go.mod`, `go.sum`, docs,
  scripts, etc.) are exempt and do not require tests

**Override for legitimate infrastructure-only changes:**
```bash
AGM_SKIP_TEST_GATE=1 git commit -m "chore: update CI config"
```

**Testing the hooks themselves:**
```bash
make test-hooks
```

## Testing

AGM has a comprehensive test suite with multiple levels of testing.

### Test Structure

```
agm/
├── cmd/agm/*_test.go           # Command integration tests
├── internal/*/                  # Package tests alongside source
│   ├── package.go
│   └── package_test.go
├── test/
│   ├── e2e/                    # End-to-end tests (testscript)
│   └── integration/            # Integration tests
└── TEST-PLAN.md                # Comprehensive testing roadmap
```

### Running Tests

```bash
# Run all tests
go test ./...

# Run with coverage
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out

# Run with race detector
go test -race ./...

# Run specific package tests
go test ./internal/tmux

# Run specific test
go test -v ./internal/tmux -run TestSendCommand

# Run tests with tmux integration (requires tmux installed)
AGM_TEST_TMUX=1 go test ./...
```

### Test Categories

#### 1. Unit Tests

Located alongside source code (`*_test.go`). These test individual functions and packages in isolation.

**Example:**
```go
func TestFunctionName(t *testing.T) {
    tests := []struct {
        name    string
        input   string
        want    string
        wantErr bool
    }{
        {"case 1", "input", "output", false},
        {"error case", "bad", "", true},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got, err := FunctionName(tt.input)
            if (err != nil) != tt.wantErr {
                t.Errorf("unexpected error: %v", err)
            }
            if got != tt.want {
                t.Errorf("got %v, want %v", got, tt.want)
            }
        })
    }
}
```

#### 2. Integration Tests

Test multiple components working together. Located in `test/integration/`.

**Example:**
```go
func TestSessionLifecycle(t *testing.T) {
    // Create session
    session := createTestSession(t, "test-session")
    defer cleanupSession(t, session)

    // Verify manifest created
    manifest, err := readManifest(session.Path())
    require.NoError(t, err)

    // Verify tmux session exists
    exists, err := tmux.HasSession(session.Name)
    require.NoError(t, err)
    assert.True(t, exists)
}
```

#### 3. E2E Tests (testscript)

Test complete workflows from the CLI. Located in `test/e2e/`.

**Example (`test/e2e/testdata/example.txtar`):**
```
# Test: Create and list sessions
exec agm new --detached test-session
stdout 'Session.*created'

exec agm list
stdout 'test-session'

exec agm archive test-session --force
```

See `test/e2e/README.md` for detailed testscript documentation.

### Test Guidelines

1. **Write tests for new code**: All new features and bug fixes must include tests
2. **Use table-driven tests**: For testing multiple scenarios
3. **Test both success and failure**: Include error cases
4. **Use descriptive names**: Test names should clearly describe what they test
5. **Keep tests focused**: One test per scenario
6. **Use test helpers**: Leverage `t.Helper()` for reusable setup code
7. **Clean up resources**: Use `defer` or `t.Cleanup()` for resource cleanup
8. **Mock external dependencies**: Use interfaces for dependency injection

### Regression Tests

When fixing a bug, add a regression test that:
1. Documents the bug and its fix
2. Fails before the fix
3. Passes after the fix
4. Prevents the bug from recurring

**Example:** See `internal/tmux/send_command_test.go` for the Enter key regression test.

### Coverage Goals

- **Overall**: 80%+ coverage
- **Critical packages** (`cmd/agm`, `internal/tmux`, `internal/session`): 75%+
- **All packages**: >50%

Coverage is a useful metric but not the goal. Focus on **effective testing** that catches real bugs.

## Code Style

### Formatting

- Run `go fmt ./...` before committing
- Follow [Effective Go](https://go.dev/doc/effective_go) guidelines
- Use `gofmt -s` for simplified formatting

### Linting

```bash
# Run golangci-lint
golangci-lint run

# Fix auto-fixable issues
golangci-lint run --fix
```

### Commit Messages

Follow conventional commit format:

```
<type>(<scope>): <subject>

<body>

<footer>
```

**Types:**
- `feat`: New feature
- `fix`: Bug fix
- `docs`: Documentation changes
- `test`: Adding or updating tests
- `refactor`: Code refactoring
- `perf`: Performance improvements
- `chore`: Maintenance tasks

**Example:**
```
fix(tmux): send Enter key separately in SendCommand

Previously, SendCommand sent command text and Enter key (C-m) in a
single tmux send-keys call, which caused the Enter key to appear as
a newline in the prompt instead of executing the command.

This splits the operation into two calls:
1. Send command text with -l flag (literal)
2. Send C-m separately to execute

Fixes #123
```

## Pull Request Process

1. **Fork and create a branch**
   ```bash
   git checkout -b feature/your-feature-name
   ```

2. **Make your changes**
   - Write code
   - Add tests
   - Update documentation

3. **Run tests locally**
   ```bash
   go test ./...
   go test -race ./...
   AGM_TEST_TMUX=1 go test ./...
   ```

4. **Commit your changes**
   ```bash
   git add .
   git commit -m "feat: add new feature"
   ```

5. **Push and create PR**
   ```bash
   git push origin feature/your-feature-name
   ```

6. **CI checks must pass**
   - Tests
   - Linting
   - Coverage threshold
   - Build

7. **Address review feedback**

8. **Merge**
   - Squash commits if requested
   - Maintainer will merge when ready

## Development Workflow

### Adding a New Feature

1. Check if an issue exists; if not, create one
2. Discuss the approach in the issue
3. Create a branch
4. Implement the feature with tests
5. Update documentation
6. Submit PR

### Fixing a Bug

1. Create an issue describing the bug
2. Write a failing test that reproduces the bug
3. Fix the bug
4. Verify the test now passes
5. Submit PR with the fix and test

### Project Structure

```
agm/
├── cmd/
│   ├── agm/           # Main AGM binary
│   ├── csm/           # Compatibility wrapper (forwards to agm)
│   └── agm-mcp-server/    # MCP server for Claude Code integration
├── internal/
│   ├── backup/        # Backup rotation
│   ├── claude/        # Claude history parsing
│   ├── config/        # Configuration management
│   ├── discovery/     # Session discovery
│   ├── manifest/      # Manifest handling
│   ├── session/       # Session management
│   ├── tmux/          # Tmux integration
│   └── ...
├── test/
│   ├── e2e/           # End-to-end tests
│   └── integration/   # Integration tests
├── .github/
│   └── workflows/     # CI/CD pipelines
├── TEST-PLAN.md       # Testing roadmap
└── CONTRIBUTING.md    # This file
```

## Resources

- [Go Testing](https://go.dev/doc/tutorial/add-a-test)
- [testscript Documentation](https://pkg.go.dev/github.com/rogpeppe/go-internal/testscript)
- [Table-Driven Tests](https://go.dev/wiki/TableDrivenTests)
- [Effective Go](https://go.dev/doc/effective_go)
- [TEST-PLAN.md](./TEST-PLAN.md) - Comprehensive testing strategy

## Questions?

- Open an issue for questions
- Check existing issues and PRs
- Review the codebase for examples

## License

By contributing, you agree that your contributions will be licensed under the same license as the project.
