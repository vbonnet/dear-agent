---
allowed-tools: Bash(wayfinder:*), Bash(pytest:*), Bash(npm:*), Bash(go:*), Bash(cargo:*), Bash(mvn:*), Bash(gradle:*), Bash(python:*), Read, Write, Edit, Glob, Grep, AskUserQuestion
description: Auto-detect and run test framework, validate phase requirements before advancement
timeout: 10m
---

# /wayfinder:validate-phase - Automated Phase Gate Validation

**Purpose**: Automatically detect project test framework, run test suite, and validate phase requirements before advancing to next Wayfinder phase.

**Eliminates**: 42+ manual "Do Phase validation testing" prompts per workflow.

**Invocation**: `/wayfinder:validate-phase` or `/engram:wayfinder-validate-phase`

---

## Step 1: Detect Active Wayfinder Project

**1.1: Locate project directory**

Follow the same detection logic as `/wayfinder-next-phase`:

- Check conversation history for recent `/wayfinder-start` output showing worktree path
- If not found, search for `wf-worktrees/*/wf/*/WAYFINDER-STATUS.md` using Glob
- Fall back to main branch: search `**/WAYFINDER-STATUS.md` in current directory
- Remember project directory path for all subsequent steps

**1.2: Read WAYFINDER-STATUS.md**

- Extract `current_phase` (e.g., "W0", "D1", "S8")
- Extract `project_path`
- Verify file exists and is valid YAML

**1.3: Error handling**

If no project found:
```
Error: No active Wayfinder project found
Usage: Run /wayfinder-start first to create a project
```

---

## Step 2: Auto-Detect Test Framework

**Purpose**: Identify which test framework to use without user configuration.

**2.1: Check for configuration override**

Look for `.validate-phase.yaml` in project directory:
```yaml
test_command: "pytest --cov --cov-report=term"
framework: pytest
# allow_skip_tests: REMOVED — tests are always required
test_timeout: 600
```

If `test_command` is specified, use it and skip to Step 3.

**2.2: Framework detection (priority order)**

Check for framework-specific files in this order:

### Python Frameworks

**pytest** (priority 1):
- Detection signals:
  - `pytest.ini` file exists
  - `pyproject.toml` contains `[tool.pytest]` section
  - `setup.cfg` contains `[tool:pytest]` section
- Test command: `pytest -v`
- Confidence: 0.95

**unittest** (priority 2):
- Detection signals:
  - `tests/test_*.py` files exist
  - No pytest configuration found
- Test command: `python -m unittest discover`
- Confidence: 0.75

**nose** (priority 3):
- Detection signals:
  - `.noserc` or `nose.cfg` exists
  - `setup.cfg` contains `[nosetests]` section
- Test command: `nosetests`
- Confidence: 0.80

### JavaScript/TypeScript Frameworks

**npm test** (priority 1):
- Detection signals:
  - `package.json` exists
  - `package.json` contains `"scripts": {"test": "..."}` field
- Test command: `npm test`
- Confidence: 0.90

**jest** (priority 2):
- Detection signals:
  - `jest.config.js` or `jest.config.ts` exists
  - `package.json` contains `"jest"` field
- Test command: `npx jest`
- Confidence: 0.85

**mocha** (priority 3):
- Detection signals:
  - `.mocharc.json` or `.mocharc.js` exists
  - `mocha.opts` file exists
- Test command: `npx mocha`
- Confidence: 0.80

**vitest** (priority 4):
- Detection signals:
  - `vitest.config.js` or `vitest.config.ts` exists
  - `vite.config.js` contains vitest configuration
- Test command: `npx vitest run`
- Confidence: 0.85

### Go Framework

**go test** (priority 1):
- Detection signals:
  - `go.mod` file exists
  - `*_test.go` files exist
- Test command: `go test ./...`
- Confidence: 0.95

### Rust Framework

**cargo test** (priority 1):
- Detection signals:
  - `Cargo.toml` file exists
- Test command: `cargo test`
- Confidence: 0.95

### Java Frameworks

**maven** (priority 1):
- Detection signals:
  - `pom.xml` file exists
- Test command: `mvn test`
- Confidence: 0.90

**gradle** (priority 2):
- Detection signals:
  - `build.gradle` or `build.gradle.kts` exists
- Test command: `gradle test`
- Confidence: 0.90

**2.3: Detection algorithm**

For each framework in priority order:
1. Check if all detection signals are present
2. If found, return framework with name, command, and confidence score
3. If multiple frameworks detected, use highest priority (lowest priority number)
4. If no framework detected, proceed to Step 2.4

**2.4: No framework detected fallback**

Display warning message:
```
⚠️  No test framework detected

Cannot automatically run tests. You MUST configure a test command:

1. Add .validate-phase.yaml with:
   test_command: "your-test-command-here"

2. Use --framework flag:
   /wayfinder:validate-phase --framework pytest

Tests are REQUIRED. Configure a test command and re-run validation.
```

Exit with error code 2. Do NOT allow skipping tests.

---

## Step 3: Run Test Suite

**3.1: Execute test command**

Run the detected test command in project directory:
- Working directory: `{project-dir}`
- Timeout: 300 seconds (5 minutes) default, or from `.validate-phase.yaml`
- Capture: stdout and stderr
- Command: Use detected command from Step 2

Example:
```bash
cd {project-dir} && pytest -v
```

**3.2: Parse test output**

Framework-specific parsing:

### pytest output
Pattern: `(\d+) passed(?:, (\d+) failed)?(?:, (\d+) skipped)?`

Example:
```
======================== 142 passed, 3 failed, 2 skipped in 12.34s ========================
```

Extract:
- `passed`: 142
- `failed`: 3
- `skipped`: 2

### npm test / jest output
Pattern: `Test Suites:.*(\d+) passed.*Tests:.*(\d+) passed(?:, (\d+) failed)?(?:, (\d+) skipped)?`

Example:
```
Test Suites: 3 passed, 3 total
Tests:       47 passed, 2 failed, 47 total
```

Extract:
- `passed`: 47
- `failed`: 2
- `skipped`: 0

### go test output
Pattern: Count `ok  ` vs `FAIL` in output

Example:
```
ok      github.com/user/pkg/auth    0.123s
ok      github.com/user/pkg/config  0.089s
FAIL    github.com/user/pkg/util    0.234s
```

Extract:
- `passed`: 2 (count of "ok")
- `failed`: 1 (count of "FAIL")
- `skipped`: 0

### cargo test output
Pattern: `test result: (ok|FAILED). (\d+) passed; (\d+) failed`

Example:
```
test result: FAILED. 87 passed; 3 failed; 0 ignored; 0 measured; 0 filtered out
```

Extract:
- `passed`: 87
- `failed`: 3
- `skipped`: 0 (ignored count)

**3.3: Validate test results**

Check validation criteria:
- **PASS**: `failed == 0 AND skipped == 0`
- **FAIL**: `failed > 0 OR skipped > 0`

If validation fails, format error message:

```
═══════════════════════════════════════════════════
  TEST SUITE VALIDATION - {framework}
═══════════════════════════════════════════════════

Running: {test_command}

Results:
✅ {passed} tests passed
❌ {failed} tests failed
⏭️  {skipped} tests skipped

Failures:
  1. test_auth.py::test_invalid_token
     AssertionError: Expected 401, got 500

  2. test_auth.py::test_expired_token
     AssertionError: Token validation failed

  3. test_integration.py::test_redis_connection
     ConnectionError: Redis server not running

───────────────────────────────────────────────────
❌ TEST SUITE: FAILED ({failed} failures, {skipped} skipped)

Fix failures before advancing:

1. Run specific test:
   {framework_specific_run_single_test_command}

2. Run failed tests only:
   {framework_specific_run_failed_command}

3. Debug with verbose output:
   {framework_specific_debug_command}

Skipped tests must be fixed before advancing. All tests must pass.
```

Framework-specific fix commands:

- **pytest**:
  - Run single: `pytest -v test_auth.py::test_invalid_token`
  - Run failed: `pytest --lf`
  - Debug: `pytest -vv --tb=long test_auth.py`

- **npm test**:
  - Run single: `npm test -- test_auth.test.js`
  - Run failed: `npm test -- --onlyFailures`
  - Debug: `npm test -- --verbose`

- **go test**:
  - Run single: `go test -v -run TestInvalidToken ./pkg/auth`
  - Run failed: `go test -v ./...`
  - Debug: `go test -v -run TestInvalidToken ./pkg/auth`

- **cargo test**:
  - Run single: `cargo test test_invalid_token`
  - Run failed: `cargo test`
  - Debug: `cargo test -- --nocapture test_invalid_token`

**3.4: Handle errors**

**Test command not found** (exit code 127):
```
❌ ERROR: Test command failed

Command: {test_command}
Exit code: 127
Error: {framework}: command not found

Solutions:
1. Install {framework}:
   {framework_install_command}
2. Check virtual environment is activated
3. Override test command in .validate-phase.yaml:
   test_command: "python -m pytest -v"

Exit code: 3
```

Framework install commands:
- pytest: `pip install pytest`
- npm test: `npm install` (if package.json exists)
- go test: (built-in, check `go version`)
- cargo test: (built-in, check `cargo --version`)

**Timeout error** (timeout exceeded):
```
❌ ERROR: Test execution timeout

Command: {test_command}
Timeout: {timeout} seconds

The test suite took longer than expected.

Solutions:
1. Increase timeout in .validate-phase.yaml:
   test_timeout: 600  # 10 minutes
2. Run specific tests:
   /wayfinder:validate-phase --tests-only
3. Investigate slow tests:
   {framework_duration_command}

Exit code: 3
```

Framework duration commands:
- pytest: `pytest --durations=10`
- npm test: `npm test -- --verbose`
- go test: `go test -v ./... 2>&1 | grep -E "PASS|FAIL"`
- cargo test: `cargo test -- --show-output`

---

## Step 4: Phase-Specific Validation

**4.1: Determine required validations for current phase**

Based on `current_phase` from Step 1:

| Phase | Tests Required | Blocking |
|-------|----------------|----------|
| W0 | No | - |
| D1-D4 | No | - |
| S4-S5 | Optional | No |
| S6 | **Yes** | **Yes** |
| S7 | **Yes** | **Yes** |
| S8 | **Yes** | **Yes** |
| S9 | **Yes** | **Yes** |
| S10 | **Yes** | **Yes** |
| S11 | Optional | No |

**4.2: Skip validation if not required**

If current phase is W0, D1-D4, S4, S5, or S11:
```
ℹ️  Phase {current_phase}: Test validation not required
    Skipping test suite validation for this phase.
```

Proceed to Step 5 (Summary).

**4.3: Enforce validation for S6-S10**

If current phase is S6, S7, S8, S9, or S10:
- Test suite validation is **mandatory**
- If tests fail, **block phase advancement**
- User must fix failures before proceeding

---

## Step 5: Validation Summary

**5.1: Display summary**

```
═══════════════════════════════════════════════════
  PHASE VALIDATION - {current_phase} ({phase_name})
═══════════════════════════════════════════════════

Detected framework: {framework}
Current phase: {current_phase} ({phase_name})

🧪 Test Suite Validation
   Running: {test_command}
   {test_result_emoji} {passed} tests passed, {failed} failures, {skipped} skipped
   Duration: {duration}s

───────────────────────────────────────────────────
{overall_status}

{action_message}
```

**5.2: Overall status determination**

- If tests PASSED: `✅ VALIDATION PASSED`
- If tests FAILED: `❌ VALIDATION FAILED`
- If tests SKIPPED (not required): `ℹ️  VALIDATION SKIPPED (not required for this phase)`

**5.3: Action message**

Success:
```
✅ VALIDATION PASSED

All quality gates passed. Safe to advance to next phase.
Ready to run: /wayfinder:next-phase
```

Failure:
```
❌ VALIDATION FAILED

Blocking issues:
  - {failed} test failures
  - {skipped} skipped tests (not allowed by default)

Fix before advancing:
{fix_commands}

Use --force to override (not recommended)
```

Skipped:
```
ℹ️  VALIDATION SKIPPED

Test validation not required for {current_phase}.
You can still run tests manually if desired.

Ready to run: /wayfinder:next-phase
```

---

## Step 6: Exit with Appropriate Code

Return exit code based on validation result:

- **0**: Validation passed or not required
- **1**: Validation failed (blocking)
- **2**: Configuration error (no framework detected, invalid config)
- **3**: Runtime error (command execution failed, timeout)

---

## Advanced Options (Future Enhancement)

The following flags are documented in the spec but not yet implemented in this initial version:

**Flags** (parse from `$ARGUMENTS` if provided):
- `--phase <phase>`: Validate specific phase (default: current)
- `--tests-only`: Only run test validation, skip other checks
- `--framework <name>`: Override auto-detection
- `--skip-tests`: REMOVED — tests are mandatory and cannot be skipped
- `--force`: Report violations but don't block
- `--verbose`: Show detailed output

For the initial implementation, these flags can be added incrementally after core functionality is validated.

---

## Integration with /wayfinder:next-phase

**Future enhancement** (Task 1.4):

Before advancing to next phase, `/wayfinder:next-phase` should call this validation:

```
Running /wayfinder:next to advance from S8 to S9...

═══════════════════════════════════════════════════
  VALIDATING PHASE S8 BEFORE ADVANCEMENT
═══════════════════════════════════════════════════

🧪 Tests: ❌ FAILED (3 failures)

───────────────────────────────────────────────────
❌ CANNOT ADVANCE TO S9

Fix violations:
1. Run: pytest -v test_auth.py
2. Fix 3 test failures

Fix now (f) | Skip validation (s) | Cancel (c)? _
```

This integration will be implemented in Task 1.4 after validating the standalone skill.

---

## Error Handling Summary

| Error Type | Exit Code | User Guidance |
|------------|-----------|---------------|
| No framework detected | 2 | Add config or use --framework |
| Test command not found | 3 | Install framework |
| Test execution timeout | 3 | Increase timeout or fix slow tests |
| Test failures | 1 | Fix failing tests with specific commands |
| Skipped tests (not allowed) | 1 | Fix or allow in config |
| No project found | 2 | Run /wayfinder-start first |

---

## Examples

### Example 1: Happy Path (pytest)

```
═══════════════════════════════════════════════════
  PHASE VALIDATION - S8 (Implementation)
═══════════════════════════════════════════════════

Detected framework: pytest (confidence: 0.95)
Current phase: S8 (Implementation)

🧪 Test Suite Validation
   Running: pytest -v
   ✅ 142 tests passed, 0 failures, 0 skipped
   Duration: 12.3s

───────────────────────────────────────────────────
✅ VALIDATION PASSED

All quality gates passed. Safe to advance to S9.
Ready to run: /wayfinder:next-phase
```

### Example 2: Test Failures (npm test)

```
═══════════════════════════════════════════════════
  PHASE VALIDATION - S8 (Implementation)
═══════════════════════════════════════════════════

Detected framework: npm test (confidence: 0.90)
Current phase: S8 (Implementation)

🧪 Test Suite Validation
   Running: npm test
   ❌ 47 passed, 3 failed, 0 skipped

   Failures:
     1. auth.test.js::should reject invalid token
        Expected: 401, Received: 500

     2. auth.test.js::should handle expired tokens
        Token validation failed

     3. integration.test.js::should connect to Redis
        Connection refused

───────────────────────────────────────────────────
❌ VALIDATION FAILED

Blocking issues:
  - 3 test failures

Fix before advancing:

1. Run specific test:
   npm test -- auth.test.js

2. Run failed tests only:
   npm test -- --onlyFailures

3. Debug with verbose output:
   npm test -- --verbose

Use --force to override (not recommended)
```

### Example 3: No Framework Detected

```
⚠️  No test framework detected

Searched for:
  - Python: pytest.ini, pyproject.toml, setup.cfg
  - JavaScript: package.json with "test" script
  - Go: go.mod
  - Rust: Cargo.toml
  - Java: pom.xml, build.gradle

Solutions:
1. Add test framework configuration file
2. Override detection:
   /wayfinder:validate-phase --framework pytest
3. Specify custom command in .validate-phase.yaml:
   test_command: "your-test-command"

Exit code: 2
```

### Example 4: Phase W0 (No Validation Required)

```
═══════════════════════════════════════════════════
  PHASE VALIDATION - W0 (Project Charter)
═══════════════════════════════════════════════════

Current phase: W0 (Project Charter)

ℹ️  Test validation not required for this phase
    Skipping test suite validation.

───────────────────────────────────────────────────
ℹ️  VALIDATION SKIPPED

Test validation not required for W0.

Ready to run: /wayfinder:next-phase
```

---

## Configuration File Schema

**`.validate-phase.yaml`** (optional, in project directory):

```yaml
# Test configuration
test_command: "pytest --cov --cov-report=term"  # Override auto-detected command
framework: pytest                                # Override framework detection
# allow_skip_tests: REMOVED — tests are always required
test_timeout: 300                                # Timeout in seconds (default: 300)

# Future enhancements (not yet implemented)
required_docs:
  - SPEC.md
  - ARCHITECTURE.md

quality_checks:
  linter:
    command: "ruff check ."
    blocking: true
  type_checker:
    command: "mypy src/"
    blocking: false

coverage:
  minimum: 80
  exclude_paths:
    - "tests/"
    - "*_test.py"
```

---

## Implementation Notes

**Security Considerations**:
- Never use `shell=True` in subprocess calls
- Validate config file paths (must be within project directory)
- Sanitize user input (test commands, framework names)
- Restrict file access to project directory only

**Cross-Platform Compatibility**:
- Use `pathlib.Path` for path operations
- Platform-specific command adjustments if needed
- Test on Linux, macOS, Windows

**Performance**:
- Default timeout: 5 minutes (configurable)
- Framework detection should be fast (<1 second)
- Parallel execution not needed for initial implementation

---

## References

- **Specification**: WAYFINDER-VALIDATE-PHASE-SPEC.md
- **Wayfinder Workflow ADR**: cortex/docs/adr/wayfinder-workflow.md
- **Related Skills**: `/wayfinder-next-phase`, `/wayfinder-start`

---

## Testing

Before integrating with `/wayfinder:next-phase`:

1. Test with pytest project (Python)
2. Test with npm test project (JavaScript/TypeScript)
3. Test with go test project (Go)
4. Test with cargo test project (Rust)
5. Test with no framework (error handling)
6. Test with .validate-phase.yaml override
7. Test in different phases (W0, S8, S11)

See Task 1.4 in ROADMAP.md for integration testing plan.

---

**Version**: 1.0.0 (Initial Implementation)
**Status**: Ready for testing
**Next Steps**: Test with real projects, integrate with /wayfinder:next-phase
