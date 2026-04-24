# E2E Tests with testscript

This directory contains end-to-end tests for AGM using the [testscript](https://github.com/rogpeppe/go-internal/tree/master/testscript) framework.

## What is testscript?

testscript is a testing framework that uses a simple scripting language to test command-line programs. It's extracted from the Go toolchain's test infrastructure.

Key features:
- **Simple syntax**: Write tests in plain text with shell-like commands
- **Isolated environments**: Each test runs in a clean temporary directory
- **Cross-platform**: Works on Unix, Windows, macOS
- **File management**: Built-in commands for creating files, checking output, etc.
- **Coverage support**: Can collect coverage from tested binaries

## Directory Structure

```
test/e2e/
├── README.md              # This file
├── testscript_test.go     # Test runner (Go code)
└── testdata/              # Test scripts (.txtar files)
    ├── version.txtar          # Smoke test
    ├── session-lifecycle.txtar  # Full session lifecycle
    └── ...                    # More tests
```

## Running Tests

```bash
# Run all E2E tests
go test ./test/e2e

# Run with verbose output
go test -v ./test/e2e

# Run a specific test
go test -v ./test/e2e -run TestCSM/version

# Run with coverage (requires building AGM with -cover)
go build -cover -o agm-cover ./cmd/csm
GOCOVER DIR=coverage go test ./test/e2e
```

## Writing Tests

### Basic Syntax

Test files use `.txtar` format (text archive). Each file contains:
1. Comments (lines starting with `#`)
2. Commands to execute
3. Conditions to check

Example:
```
# This is a comment
exec agm --version
stdout 'agm version'
```

### Common Commands

#### Execute Commands
```
exec agm list                  # Run command, expect success
! exec agm invalid-command     # Run command, expect failure
```

#### Check Output
```
stdout 'pattern'               # Stdout must contain pattern (regex)
! stdout 'pattern'             # Stdout must NOT contain pattern
stderr 'error message'         # Stderr must contain pattern
```

#### File Operations
```
exists file.txt                # File must exist
! exists file.txt              # File must NOT exist
cmp file1.txt file2.txt        # Files must be identical
```

#### Conditionals
```
[!exec:tmux] skip 'need tmux'  # Skip test if tmux not available
[short] skip                   # Skip in short mode (-short)
```

### File Creation

Create files inline using `-- filename`:
```
-- sessions/test/manifest.yaml --
schema_version: "2.0"
session_id: "550e8400-e29b-41d4-a716-446655440000"
name: "test-session"
```

### Environment Variables

Available in all tests:
- `$WORK` - Temporary working directory (cleaned up after test)
- `$HOME` - Fake home directory (set to $WORK/home)
- `$PATH` - Includes test binary directory

Custom variables:
```
env MY_VAR=value
exec csm command
```

## Test Categories

### Smoke Tests
- **Purpose**: Verify basic functionality works
- **Examples**: `agm --version`, `agm --help`
- **Runtime**: <1 second

### Integration Tests
- **Purpose**: Test command interactions
- **Examples**: Create session → list → archive
- **Runtime**: 1-5 seconds
- **Requirements**: May need tmux

### Full E2E Tests
- **Purpose**: Test complete workflows with real tmux and Claude
- **Examples**: Complete session lifecycle with Claude interaction
- **Runtime**: 10-60 seconds
- **Requirements**: tmux, Claude CLI

## Current Test Status

### Implemented
- ✅ Test infrastructure setup
- ✅ Example smoke test (`version.txtar`)
- ✅ Placeholder for session lifecycle
- ✅ **Phase 0 BDD Test Suite** (see below)

### Phase 0 BDD Tests

The following tests validate Phase 0 deliverables (Agent interface, Claude adapter, JSONL format, Manifest v3):

1. **session-creation.txtar** - Validates session creation with Manifest v3 and JSONL format
2. **session-resumption.txtar** - Validates resume command and Claude adapter initialization
3. **session-listing.txtar** - Validates list command with multiple sessions
4. **session-archiving.txtar** - Validates archive command
5. **claude-adapter.txtar** - Validates Claude adapter behavioral correctness
6. **jsonl-format.txtar** - Validates JSONL round-trip conversion (XML→JSONL migration)
7. **manifest-v3.txtar** - Validates Manifest v3 structure and UUID format

#### Running Phase 0 BDD Tests

```bash
# Run all BDD tests
make test-bdd

# Run specific test
go test -v ./test/e2e -run TestCSM/session-creation

# Run with verbose output
go test -v ./test/e2e
```

#### Test Coverage

- **Core commands**: new, resume, list, archive
- **Phase 0 deliverables**: Agent interface, Claude adapter, JSONL format, Manifest v3
- **Regression prevention**: UUID format validation, no XML files for new sessions

### TODO
- [ ] Implement csmMain() to actually call AGM commands
- [ ] Add tests for error conditions
- [ ] Add tests for concurrent session creation
- [ ] Add tests for lock contention
- [ ] Add tests for recovery scenarios

## Best Practices

1. **Keep tests focused**: One test per scenario
2. **Use descriptive names**: `session-creation-with-detached.txtar`
3. **Comment your tests**: Explain what you're testing and why
4. **Clean up**: testscript handles this, but be aware of resources
5. **Test both success and failure**: Use `!` to expect failures

## Debugging Tests

### View test output
```bash
go test -v ./test/e2e
```

### Keep test work directory
```bash
go test -v ./test/e2e -testwork
# Prints: WORK=/tmp/go-test123456789
# Directory is preserved for inspection
```

### Run single test
```bash
go test -v ./test/e2e -run TestCSM/version
```

## Resources

- [testscript Documentation](https://pkg.go.dev/github.com/rogpeppe/go-internal/testscript)
- [Go Tool Testing](https://github.com/golang/go/tree/master/src/cmd/go/testdata/script) - Examples from Go itself
- [Blog: Testing CLI Applications](https://bitfieldconsulting.com/posts/cli-testing) - testscript tutorial

## Example: Complete Test

```
# Test: Session creation with detached mode
[!exec:tmux] skip 'tmux required'

# Create a detached session
exec agm new --detached test-session-1
stdout 'Session.*created'
stdout 'detached'

# Verify manifest was created
exists $HOME/sessions/test-session-1/manifest.yaml

# Check manifest contains proper UUID (not session-{name})
exec cat $HOME/sessions/test-session-1/manifest.yaml
stdout 'session_id: [0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}'
! stdout 'session-test-session-1'

# Verify session appears in list
exec agm list
stdout 'test-session-1'

# Clean up
exec agm archive test-session-1 --force
```

---

**Last Updated**: 2026-01-13
**Status**: Infrastructure complete, tests in development
