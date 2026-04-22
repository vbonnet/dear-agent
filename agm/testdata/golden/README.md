# Golden Files

This directory contains expected CLI output for regression testing.

## What are Golden Files?

Golden files store the expected output of commands. Tests compare actual output to golden files to detect unintended changes.

## Naming Convention

Files use the pattern `{feature}-{variant}.golden`:

- `session-list.golden` - Output of `agm list`
- `session-list-json.golden` - Output of `agm list --format json`
- `session-create-success.golden` - Success message for `agm create`
- `session-create-error.golden` - Error when session already exists

## Updating Golden Files

When CLI output intentionally changes, update golden files:

```bash
# Update all golden files
go test -update ./...

# Update specific test
go test -update -run TestSessionList ./test/unit/

# Review changes
git diff testdata/golden/
```

## Workflow

1. Write test with golden file comparison
2. Run test (fails - golden file missing)
3. Run with `-update` flag to create golden file
4. Review golden file content
5. Commit golden file with test

## Example Test

```go
func TestSessionList(t *testing.T) {
    stdout, _, _ := helpers.RunCLI(t, "list")
    helpers.CompareGolden(t, "testdata/golden/session-list.golden", stdout)
}
```

## Git Tracking

All golden files are tracked in git to detect output changes in code review.

## Corruption Detection

Tests automatically detect corrupted golden files:
- Empty files (0 bytes)
- Binary corruption (null bytes)
- Invalid UTF-8

Error messages provide recovery instructions.
