# Test Data Fixtures

This directory contains test data fixtures for health check and auto-fix testing.

## Settings.json Fixtures

### `settings-valid.json`
Clean, valid settings.json with no issues.
- Used for baseline testing
- Verifies checks pass on valid config

### `settings-broken-extensions.json`
Settings with .py extension mismatches.
- Hook commands reference .py files but binaries exist without extension
- Tests fixHookExtensionMismatches()
- Expected fix: Remove .py extensions

### `settings-wrong-paths.json`
Settings with incorrect hook paths.
- Contains common path errors:
  - `engram/main/hooks/` → should be `engram/hooks/`
  - `sessionstart/` → should be `session-start/`
  - `./.claude/` → should be different path
- Tests fixHookPaths()
- Expected fix: Correct paths to actual locations

## Marketplace Fixtures

### `marketplace-valid.json`
Valid marketplace configuration.
- Direct path strings for source
- Used for baseline testing

### `marketplace-invalid-source.json`
Marketplace config with source="directory" (invalid).
- Contains source objects instead of strings
- Causes Claude Code to crash on startup
- Tests fixMarketplaceConfig()
- Expected fix: Convert source objects to path strings

## Usage in Tests

```go
// Load fixture
data, err := os.ReadFile("testdata/settings-broken-extensions.json")

// Test auto-fix
fixer := NewTier1Fixer("/tmp/test-workspace")
err = fixer.fixExtensionsInFile("/tmp/test/settings.json", data)

// Verify fix applied
fixed, _ := os.ReadFile("/tmp/test/settings.json")
// Assert: fixed should not contain .py extensions
```

## Adding New Fixtures

When adding new test cases:
1. Create fixture file with descriptive name
2. Document in this README
3. Add corresponding test in fix_test.go or checks_test.go
4. Verify fixture is actually used in at least one test
