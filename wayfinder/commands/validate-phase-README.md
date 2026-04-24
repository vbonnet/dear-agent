# /wayfinder:validate-phase - Usage Guide

## Overview

Automated phase gate validation for Wayfinder workflows. Eliminates 42+ manual validation prompts by automatically detecting test frameworks and running test suites before phase advancement.

## Quick Start

```bash
# Basic usage - auto-detect framework and validate current phase
/wayfinder:validate-phase

# Validate specific phase
/wayfinder:validate-phase --phase S8

# Only run tests, skip other checks
/wayfinder:validate-phase --tests-only
```

## Supported Test Frameworks

### Automatically Detected

- **Python**: pytest, unittest, nose
- **JavaScript/TypeScript**: npm test, jest, mocha, vitest
- **Go**: go test
- **Rust**: cargo test
- **Java**: maven (mvn test), gradle (gradle test)

Total: 11 frameworks across 6 languages

## How It Works

### 1. Project Detection

Locates active Wayfinder project by checking:
- Recent `/wayfinder-start` output in conversation
- `wf-worktrees/*/wf/*/WAYFINDER-STATUS.md` files
- Main branch `**/WAYFINDER-STATUS.md` files

### 2. Framework Detection

Priority order:
1. Configuration override (`.validate-phase.yaml`)
2. Framework-specific files (pytest.ini, package.json, go.mod, etc.)
3. Language-specific defaults

### 3. Test Execution

Runs framework-appropriate command:
- `pytest -v` for pytest
- `npm test` for Node.js
- `go test ./...` for Go
- `cargo test` for Rust
- etc.

### 4. Result Validation

Checks for:
- Test failures (any failure blocks advancement)
- Skipped tests (treated as failures by default)
- Timeout (5 minute default)
- Exit codes (0=success, non-zero=failure)

### 5. Blocking Behavior

**Blocks advancement when**:
- Tests fail (any failures)
- Tests skipped (unless explicitly allowed)
- Test command times out
- Test command returns non-zero exit code

**Does NOT block when**:
- Phase doesn't require tests (W0, D1-D4, S4-S5, S11)
- `--force` flag used (reports but doesn't block)
- `--skip-tests` flag used (skips validation entirely)

## Configuration

### .validate-phase.yaml

Override auto-detection:

```yaml
framework: pytest
test_command: pytest -v --cov
timeout: 600  # seconds
allow_skipped: false
phases:
  S6: { required: true }
  S7: { required: true }
  S8: { required: true }
  S9: { required: true }
  S10: { required: true }
```

## Usage Examples

### Example 1: Basic Validation (Auto-Detect)

```bash
/wayfinder:validate-phase
```

**Output (Success)**:
```
┌─────────────────────────────────────────────┐
│ Wayfinder Phase Validation                  │
├─────────────────────────────────────────────┤
│ Phase: S8 (Execute Build)                   │
│ Framework: pytest (auto-detected)           │
│ Command: pytest -v                          │
└─────────────────────────────────────────────┘

Running tests... (timeout: 5m)

✓ Tests passed: 127/127
✓ Tests failed: 0
✓ Tests skipped: 0

✅ Validation PASSED - Ready to advance
```

### Example 2: Test Failures

```bash
/wayfinder:validate-phase
```

**Output (Failure)**:
```
┌─────────────────────────────────────────────┐
│ Wayfinder Phase Validation                  │
├─────────────────────────────────────────────┤
│ Phase: S8 (Execute Build)                   │
│ Framework: pytest (auto-detected)           │
│ Command: pytest -v                          │
└─────────────────────────────────────────────┘

Running tests... (timeout: 5m)

✗ Tests passed: 125/127
✗ Tests failed: 2
✗ Tests skipped: 0

Failing tests:
  - test_api_auth.py::test_invalid_token
  - test_database.py::test_connection_retry

❌ Validation FAILED - Cannot advance

Fix commands:
  pytest test_api_auth.py::test_invalid_token -v
  pytest test_database.py::test_connection_retry -v

Exit code: 1
```

### Example 3: Framework Override

```bash
/wayfinder:validate-phase --framework jest
```

Useful when:
- Multiple frameworks detected
- Auto-detection picks wrong framework
- Testing alternative framework

### Example 4: No Framework Detected

```bash
/wayfinder:validate-phase
```

**Output**:
```
⚠️  No test framework detected

To proceed:
1. Create .validate-phase.yaml with framework specification
2. Or add test framework to your project
3. Or use --skip-tests flag (not recommended)

Supported frameworks:
  - pytest (pytest.ini, pyproject.toml)
  - npm test (package.json)
  - go test (go.mod)
  - cargo test (Cargo.toml)
  - maven (pom.xml)
  - gradle (build.gradle)

Exit code: 2
```

## Integration with /wayfinder:next-phase

### Automatic Validation (Future)

When integrated with `/wayfinder:next-phase`, validation will run automatically:

```bash
/wayfinder:next-phase
```

**Will execute**:
1. `/wayfinder:validate-phase` (auto-invoked)
2. If validation passes → advance phase
3. If validation fails → block advancement, show errors

### Manual Validation

Run validation separately before advancement:

```bash
# Validate first
/wayfinder:validate-phase

# Then advance (if validation passed)
/wayfinder:next-phase
```

## Troubleshooting

### Issue: "No active Wayfinder project found"

**Cause**: Not in a Wayfinder project directory

**Fix**:
```bash
# Create new project
/wayfinder-start "my-project"

# Or navigate to existing project
cd ~/src/wf-worktrees/my-project/wf/my-project
```

### Issue: "No test framework detected"

**Cause**: Project doesn't have recognizable test framework files

**Fix**:
1. Add test framework configuration:
   - Python: Create `pytest.ini` or add `[tool.pytest]` to `pyproject.toml`
   - Node.js: Add `"test"` script to `package.json`
   - Go: Ensure `go.mod` exists
   - Rust: Ensure `Cargo.toml` exists

2. Or create `.validate-phase.yaml`:
   ```yaml
   framework: pytest
   test_command: pytest -v
   ```

### Issue: Tests timeout after 5 minutes

**Cause**: Test suite takes longer than default timeout

**Fix**: Configure longer timeout in `.validate-phase.yaml`:
```yaml
timeout: 900  # 15 minutes
```

### Issue: Skipped tests blocking advancement

**Cause**: By default, skipped tests = failures

**Fix**: Allow skipped tests in `.validate-phase.yaml`:
```yaml
allow_skipped: true
```

## Phase-Specific Behavior

### Discovery Phases (W0, D1-D4, S4-S5)
- Tests NOT required (informational only)
- Validation runs but doesn't block
- Encourages adding tests early

### Execution Phases (S6-S10)
- Tests REQUIRED (blocking)
- Validation must pass to advance
- Zero failures allowed

### Completion Phase (S11)
- Tests NOT required (assuming already validated in S6-S10)
- Final checklist validation instead

## Exit Codes

| Code | Meaning | Action |
|------|---------|--------|
| 0 | Validation passed | Safe to advance |
| 1 | Validation failed | Fix tests before advancing |
| 2 | Configuration error | Check .validate-phase.yaml |
| 3 | Runtime error | Check logs, retry |
| 130 | User interrupted | Re-run when ready |

## Best Practices

### 1. Run Early and Often

```bash
# After making changes
/wayfinder:validate-phase

# Before committing
/wayfinder:validate-phase

# Before advancing phase
/wayfinder:validate-phase
```

### 2. Configure Per-Project

Create `.validate-phase.yaml` for project-specific needs:

```yaml
framework: jest
test_command: npm test -- --coverage
timeout: 300
phases:
  S8: { required: true, allow_skipped: false }
```

### 3. Use Verbose Mode for Debugging

```bash
/wayfinder:validate-phase --verbose
```

Shows:
- Full test command
- Complete test output
- Detailed parsing results

### 4. Don't Skip Tests in Execution Phases

```bash
# ❌ BAD - skips critical validation
/wayfinder:validate-phase --skip-tests

# ✅ GOOD - fix failing tests
/wayfinder:validate-phase
# (then fix failures before advancing)
```

## Future Enhancements

Planned improvements:

1. **Coverage Validation**
   - Require minimum code coverage (e.g., 80%)
   - Block advancement if coverage drops

2. **Code Quality Checks**
   - Run linters (eslint, golangci-lint, etc.)
   - Check for TODO/FIXME comments

3. **Security Scanning**
   - Run security vulnerability scanners
   - Check for known CVEs in dependencies

4. **Performance Regression**
   - Compare test execution time to baseline
   - Alert if tests suddenly slow down

5. **Parallel Execution**
   - Run tests in parallel for faster validation
   - Support sharding for large test suites

## Related Commands

- `/wayfinder-start` - Create new Wayfinder project
- `/wayfinder-next-phase` - Advance to next phase (will integrate with validation)
- `/wayfinder-stop` - Complete Wayfinder project
- `/engram:bow` - General close-out validation (enhanced in Phase 2)

## Support

**Specification**: WAYFINDER-VALIDATE-PHASE-SPEC.md
**Implementation**: plugins/wayfinder/commands/validate-phase.md
**Swarm**: claude-code-automation-improvements
**Phase**: 1
**Beads**: oss-pz4d, oss-2k7h, oss-qty2, oss-7lbt, oss-3toy

## Version History

- **v1.0** (2026-02-23): Initial implementation
  - 11 test frameworks supported
  - Auto-detection algorithm
  - Phase-aware validation
  - Comprehensive error handling
