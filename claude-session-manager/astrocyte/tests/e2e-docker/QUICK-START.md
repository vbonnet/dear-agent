# Astrocyte E2E Tests - Quick Start Guide

## One-Command Test Run

```bash
# From claude-session-manager root directory
docker build -t astrocyte-e2e-test -f astrocyte/tests/e2e-docker/Dockerfile . && \
docker run --rm astrocyte-e2e-test
```

**Expected**: "ALL TESTS PASSED ✓" in ~2-3 minutes

## Using Docker Compose

```bash
cd astrocyte/tests/e2e-docker/

# Run tests
docker-compose up --abort-on-container-exit

# Clean up
docker-compose down
```

## Individual Tests

```bash
# Build once
docker build -t astrocyte-e2e-test -f astrocyte/tests/e2e-docker/Dockerfile .

# Run specific test
docker run --rm astrocyte-e2e-test /tests/test_mustering_detection.sh
docker run --rm astrocyte-e2e-test /tests/test_zero_token_recovery.sh
docker run --rm astrocyte-e2e-test /tests/test_permission_rejection.sh
docker run --rm astrocyte-e2e-test /tests/test_multi_session.sh
docker run --rm astrocyte-e2e-test /tests/test_incident_logging.sh
```

## Debug Mode

```bash
cd astrocyte/tests/e2e-docker/

# Start interactive container
docker-compose --profile debug run --rm astrocyte-debug

# Inside container, run manually:
/tests/scripts/setup-test-env.sh
/tests/scripts/create-stuck-session.sh test-debug stuck-mustering.txt
python3 /home/testuser/astrocyte/astrocyte.py

# Check results:
cat ~/.csm/astrocyte/incidents.jsonl
cat ~/.csm/astrocyte/logs/csm-mock.log
tmux -S /tmp/csm.sock list-sessions
```

## Troubleshooting

### Build fails with "astrocyte.py not found"
**Fix**: Build from `claude-session-manager/` root, not subdirectory

### Tests timeout
**Fix**: Increase Docker memory to 2GB minimum

### "Permission denied" errors
**Fix**: Rebuild image (scripts made executable in Dockerfile)

## Expected Output

```
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

## CI/CD Integration

### GitHub Actions

```yaml
- name: Run Astrocyte E2E Tests
  run: |
    docker build -t astrocyte-e2e-test \
      -f astrocyte/tests/e2e-docker/Dockerfile .
    docker run --rm astrocyte-e2e-test
```

### Exit Codes

- `0` = All tests passed
- `1` = One or more tests failed
- `2` = Build or setup error

## More Information

See [README.md](README.md) for complete documentation.
