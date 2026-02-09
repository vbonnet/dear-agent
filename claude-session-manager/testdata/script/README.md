# testscript Examples

This directory contains testscript `.txtar` files for CLI testing.

## What is testscript?

testscript is a domain-specific language (DSL) for testing command-line tools, developed by the Go team and used to test the `go` command itself.

## File Format

`.txtar` files contain:
- Test script commands (like shell script)
- Embedded files (separated by `-- filename --`)

## Running Tests

```bash
# Run all testscript tests
go test ./test/unit -run TestScripts

# Run specific test
go test ./test/unit -run TestScripts/session_create
```

## Example

```txtar
# session_create.txtar
# Test: Create new AGM session

exec agm create test-session
stdout 'Session "test-session" created'

exists $HOME/.agm/sessions/test-session.json

exec agm list
stdout 'test-session'
```

## Common Commands

- `exec <command>` - Execute command (must succeed)
- `! exec <command>` - Execute command (must fail)
- `stdout <pattern>` - Verify stdout contains pattern
- `stderr <pattern>` - Verify stderr contains pattern
- `exists <path>` - Verify file/directory exists
- `cmp <file1> <file2>` - Compare files

## Environment

Tests run in isolated temp directories with:
- `$HOME` set to temp directory
- `$AGM_TEST_MODE=true` for test detection
- Automatic cleanup after test

## References

- [testscript Documentation](https://pkg.go.dev/github.com/rogpeppe/go-internal/testscript)
- [Go team testscript examples](https://github.com/golang/go/tree/master/src/cmd/go/testdata/script)
