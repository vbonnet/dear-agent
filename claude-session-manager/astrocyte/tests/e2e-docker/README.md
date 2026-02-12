# Astrocyte E2E Docker Tests

End-to-end Docker-based tests for astrocyte that simulate real tmux sessions and validate the complete recovery workflow: detection → diagnosis → recovery.

## Overview

These tests run astrocyte in isolated Docker containers with simulated CSM sessions in tmux. They verify that astrocyte correctly detects stuck sessions, performs recovery, and logs incidents.

## Test Architecture

```
tests/e2e-docker/
├── Dockerfile                    # Test environment with tmux, Python, CSM
├── docker-compose.yml            # Orchestration for test containers
├── README.md                     # This file
├── scripts/
│   ├── setup-test-env.sh        # Initialize test environment
│   ├── create-stuck-session.sh  # Create tmux session with stuck state
│   ├── run-astrocyte-test.sh    # Run astrocyte daemon in test mode
│   └── verify-recovery.sh       # Validate recovery results
├── fixtures/
│   ├── stuck-mustering.txt      # Pane content for mustering timeout
│   ├── stuck-zero-token.txt     # Pane content for zero-token waiting
│   ├── stuck-permission.txt     # Permission prompt with violations
│   ├── normal-session.txt       # Normal session content
│   └── manifest-template.yaml   # CSM manifest template
├── mocks/
│   └── csm                       # Mock CSM binary for testing
└── tests/
    ├── test_mustering_detection.sh   # Test mustering timeout
    ├── test_zero_token_recovery.sh   # Test zero-token waiting
    ├── test_permission_rejection.sh  # Test permission prompt rejection
    ├── test_multi_session.sh         # Test multi-session monitoring
    ├── test_incident_logging.sh      # Test JSONL logging
    └── run-all-tests.sh              # Main test runner
```

## Test Scenarios

### 1. Mustering Timeout Detection (`test_mustering_detection.sh`)
- **Purpose**: Verify detection of sessions stuck in "✻ Mustering..." state
- **Setup**: Create tmux session with mustering pattern
- **Expected**: Detection within 2 check cycles, ESC recovery, incident logged

### 2. Zero-Token Waiting Recovery (`test_zero_token_recovery.sh`)
- **Purpose**: Verify detection of sessions with "↓ 0 tokens" and no progress
- **Setup**: Create session with zero-token pattern and thinking spinner
- **Expected**: Detection, ESC recovery, diagnosis prompt sent via `csm send`

### 3. Permission Prompt Rejection (`test_permission_rejection.sh`)
- **Purpose**: Verify immediate detection and rejection of permission prompts
- **Setup**: Create session with permission prompt containing tool violations
- **Expected**: Immediate detection, rejection via `csm reject`, violation prompt sent

### 4. Multi-Session Monitoring (`test_multi_session.sh`)
- **Purpose**: Verify astrocyte monitors multiple sessions correctly
- **Setup**: Create 3 sessions (1 stuck, 2 normal)
- **Expected**: Only stuck session recovered, no false positives on normal sessions

### 5. Incident Logging Validation (`test_incident_logging.sh`)
- **Purpose**: Verify JSONL logging format and required fields
- **Setup**: Trigger incident and examine log file
- **Expected**: Valid JSONL format, all required fields present, crash-safe append

## Prerequisites

- Docker installed and running
- Access to astrocyte source code
- 2GB free disk space
- 512MB free RAM

## Quick Start

### Build and Run All Tests

From the repository root (`claude-session-manager/`):

```bash
# Build test image
docker build -t astrocyte-e2e-test -f astrocyte/tests/e2e-docker/Dockerfile .

# Run full test suite
docker run --rm astrocyte-e2e-test

# Or use docker-compose
cd astrocyte/tests/e2e-docker/
docker-compose up --abort-on-container-exit
```

### Run Individual Tests

```bash
# Build image first
docker build -t astrocyte-e2e-test -f astrocyte/tests/e2e-docker/Dockerfile .

# Run specific test
docker run --rm astrocyte-e2e-test /tests/test_mustering_detection.sh

# Interactive debugging
docker run --rm -it astrocyte-e2e-test /bin/bash
```

### Debug Mode

For interactive debugging:

```bash
cd astrocyte/tests/e2e-docker/
docker-compose --profile debug run --rm astrocyte-debug

# Inside container:
/tests/scripts/setup-test-env.sh
/tests/scripts/create-stuck-session.sh test-session stuck-mustering.txt
python3 /home/testuser/astrocyte/astrocyte.py
```

## Expected Output

### Successful Test Run

```
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
Astrocyte E2E Test Suite
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
Starting test execution...
Timestamp: 2026-02-03T15:30:00Z

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
Running: test_mustering_detection
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

╔════════════════════════════════════════════════════════════╗
║  Test: Mustering Timeout Detection                        ║
╚════════════════════════════════════════════════════════════╝

[TEST] Setting up test environment...
[INFO] Setting up directory structure...
[INFO] Starting tmux server...
[INFO] Creating astrocyte configuration...
[TEST] Creating stuck session with mustering pattern...
[TEST] Running astrocyte daemon (15 seconds)...
[TEST] Verifying recovery...

[✓] Incident logged for session: test-mustering
[✓] Correct symptom detected: stuck_mustering
[✓] Recovery was attempted
[✓] All incidents are valid JSON

[PASS] ✓ test_mustering_detection PASSED

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
Test Summary
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

Total Tests:  5
Passed:       5
Failed:       0

[PASS] ╔═══════════════════════════════════════════════════════════╗
[PASS] ║          ALL TESTS PASSED ✓                               ║
[PASS] ╚═══════════════════════════════════════════════════════════╝
```

### Failed Test Run

```
[FAIL] ✗ test_zero_token_recovery FAILED

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
Test Summary
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

Total Tests:  5
Passed:       4
Failed:       1

[FAIL] ╔═══════════════════════════════════════════════════════════╗
[FAIL] ║          SOME TESTS FAILED ✗                              ║
[FAIL] ╚═══════════════════════════════════════════════════════════╝

Failed tests:
  - test_zero_token_recovery
```

## Test Configuration

Tests use accelerated timeouts for fast execution:

```yaml
# ~/.agm/astrocyte/config.yaml (in container)
interval_seconds: 5  # Check every 5 seconds

thresholds:
  mustering_timeout: 1        # 1 minute
  zero_token_waiting: 1       # 1 minute
  cursor_frozen: 2            # 2 minutes
  permission_prompt_duration: 1  # 1 minute
```

This allows tests to complete in ~15 seconds per scenario instead of production timeouts (10+ minutes).

## Mock CSM Commands

The tests use a mock `csm` binary (`mocks/csm`) that:
- Logs all calls to `~/.agm/astrocyte/logs/csm-mock.log`
- Simulates `csm send` (prompt delivery)
- Simulates `csm reject` (permission rejection)
- Returns success (exit 0) for valid calls

This allows testing astrocyte's integration without requiring the full CSM Go binary.

## Troubleshooting

### Build Failures

**Error: `Could not find astrocyte.py`**
- **Cause**: Building from wrong directory
- **Fix**: Build from `claude-session-manager/` root, not subdirectory

**Error: `permission denied` on scripts**
- **Cause**: Scripts not executable
- **Fix**: Already handled in Dockerfile, rebuild image

### Test Failures

**Error: `Incident not logged`**
- **Cause**: Astrocyte didn't detect stuck pattern or threshold not met
- **Fix**: Check astrocyte log: `/home/testuser/.agm/astrocyte/logs/astrocyte-test.log`

**Error: `CSM commands not called`**
- **Cause**: Mock CSM binary not in PATH or not executable
- **Fix**: Verify `/home/testuser/bin/csm` exists and is executable

**Error: `Tmux session not found`**
- **Cause**: Tmux server not started or session creation failed
- **Fix**: Check tmux socket: `/tmp/csm.sock`

### Debugging Tips

1. **View astrocyte logs**:
   ```bash
   docker run --rm astrocyte-e2e-test cat /home/testuser/.agm/astrocyte/logs/astrocyte-test.log
   ```

2. **View incidents log**:
   ```bash
   docker run --rm astrocyte-e2e-test cat /home/testuser/.agm/astrocyte/incidents.jsonl
   ```

3. **Interactive shell**:
   ```bash
   docker run --rm -it astrocyte-e2e-test /bin/bash
   ```

4. **Check tmux sessions**:
   ```bash
   # Inside container
   tmux -S /tmp/csm.sock list-sessions
   tmux -S /tmp/csm.sock capture-pane -t test-session -p
   ```

## CI/CD Integration

### GitHub Actions (Example)

```yaml
name: Astrocyte E2E Tests

on:
  push:
    branches: [main, develop]
  pull_request:
    branches: [main, develop]

jobs:
  e2e-tests:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3

      - name: Build test image
        run: |
          docker build -t astrocyte-e2e-test \
            -f astrocyte/tests/e2e-docker/Dockerfile .

      - name: Run E2E tests
        run: |
          docker run --rm astrocyte-e2e-test

      - name: Upload test results
        if: failure()
        uses: actions/upload-artifact@v3
        with:
          name: test-results
          path: astrocyte/tests/e2e-docker/test-results/
```

## Test Duration

- Individual test: ~15-20 seconds
- Full suite (5 tests): ~2-3 minutes
- Build time: ~1 minute (first build)

## Limitations

1. **Mock CSM**: Tests use mock `csm` binary, not real CSM Go implementation
2. **Fast timeouts**: Tests use 1-minute thresholds vs. production 10+ minutes
3. **No cloud integration**: Remote reporting not tested
4. **No Slack/email**: Notification systems not tested (would require external services)

## Future Enhancements

- [ ] Test diagnosis file creation and content
- [ ] Test remote reporting integration (requires mock collector)
- [ ] Test recovery strategy chains (escape → ctrl_c → restart)
- [ ] Test session restart recovery
- [ ] Test false positive detection and logging
- [ ] Performance testing (100+ sessions)
- [ ] Test systemd service integration

## Related Documentation

- Astrocyte README: `../README.md`
- Astrocyte source: `../astrocyte.py`
- CSM E2E tests: `../../tests/e2e-install/README.md`

## Contributing

When adding new tests:

1. Create test script in `tests/test_*.sh`
2. Follow naming convention: `test_<feature>_<aspect>.sh`
3. Use test helper functions from existing tests
4. Add test to `run-all-tests.sh`
5. Update this README with test description
6. Ensure test runs in <30 seconds

## License

Same as astrocyte and CSM projects.
