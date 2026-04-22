# Reaper Test Plan

**Purpose**: Ensure high confidence that the async archive and `/agm:agm-exit` workflows are robust and won't break with future changes.

**Last Updated**: 2026-02-12
**Status**: Implementation in progress

---

## Test Pyramid

```
        /\
       /E2E\        Docker E2E Tests (1-2 tests)
      /------\
     /        \
    /Integration\ Integration Tests (5-10 tests)
   /------------\
  /              \
 /  Unit Tests    \ Unit Tests (15-20 tests)
/------------------\
```

---

## 1. Unit Tests

### 1.1 Archive Command Tests (`cmd/agm/archive_test.go`)

**Existing Coverage** ✅:
- Synchronous archive flow
- Archive validation
- Error handling
- Permission edge cases

**Missing Coverage** (to add):

#### Test: `TestSpawnReaper_Success`
- Setup: Create valid session with manifest
- Action: Call `spawnReaper(sessionName)`
- Assert:
  - Returns nil error
  - agm-reaper binary found
  - Process spawned successfully
  - Log file path created
  - Process detached (doesn't block)

#### Test: `TestSpawnReaper_BinaryNotFound`
- Setup: Mock `os.Executable()` to return path without agm-reaper
- Action: Call `spawnReaper(sessionName)`
- Assert:
  - Returns error containing "agm-reaper binary not found"
  - Error message includes expected path
  - Error message includes build instructions

#### Test: `TestSpawnReaper_BinaryNotExecutable`
- Setup: Create agm-reaper file with 0644 permissions
- Action: Call `spawnReaper(sessionName)`
- Assert:
  - Returns error about permissions
  - Error message suggests chmod +x

#### Test: `TestSpawnReaper_SessionNameSanitization`
- Setup: Create session with path traversal attempt: `../../../evil-session`
- Action: Call `spawnReaper(sessionName)`
- Assert:
  - Log file path uses sanitized name (no directory traversal)
  - Log file created in /tmp only
  - Process spawned with correct session name

#### Test: `TestArchiveSession_AsyncFlag`
- Setup: Create valid session, set asyncArchive=true
- Action: Call `archiveSession(cmd, []string{"session-name"})`
- Assert:
  - Calls spawnReaper() instead of synchronous archive
  - Session lifecycle remains unchanged (not archived yet)
  - Success message about async archive
  - Log file path shown

#### Test: `TestArchiveSession_AsyncIncompatibleWithAll`
- Setup: Set asyncArchive=true and archiveAll=true
- Action: Call `archiveSession(cmd, []string{})`
- Assert:
  - Returns error "--async flag is not compatible with --all"
  - No reaper spawned
  - No archive operation performed

---

### 1.2 Reaper Logic Tests (`internal/reaper/reaper_test.go`)

**Existing Coverage** ✅:
- Reaper struct construction
- Basic field validation

**Missing Coverage** (to add):

#### Test: `TestReaper_Run_MockedTmux`
- Setup: Mock tmux client to simulate:
  - Prompt detection succeeds after 5s
  - Pane closes successfully
  - Session manifest exists
- Action: Call `r.Run()`
- Assert:
  - waitForPrompt called
  - sendExit called after prompt detected
  - waitForPaneClose called
  - archiveSession called
  - Manifest lifecycle = archived
  - All steps complete in < 10s

#### Test: `TestReaper_Run_PromptTimeoutFallback`
- Setup: Mock tmux client to simulate:
  - Prompt detection times out (90s)
  - Falls back to fixed 60s wait
  - Pane closes successfully
- Action: Call `r.Run()`
- Assert:
  - waitForPrompt returns timeout error
  - Fallback sleep triggered (check logs)
  - sendExit still called
  - Archive completes successfully

#### Test: `TestReaper_Run_PaneCloseTimeout`
- Setup: Mock tmux client to simulate:
  - Prompt detected
  - /exit sent
  - Pane never closes (timeout after 30s)
- Action: Call `r.Run()`
- Assert:
  - Returns error "pane did not close within 30s"
  - Archive not performed
  - Error logged

#### Test: `TestReaper_Run_SessionNotFound`
- Setup: Reaper with non-existent session name
- Action: Call `r.Run()`
- Assert:
  - Returns error "session not found"
  - No tmux commands sent
  - No archive attempted

#### Test: `TestReaper_Run_AlreadyArchived`
- Setup: Session manifest with lifecycle=archived
- Action: Call `r.Run()`
- Assert:
  - Completes successfully (idempotent)
  - Log message "Session already archived, skipping"
  - No duplicate archive operations

---

## 2. Integration Tests

### 2.1 Reaper E2E Flow (`test/integration/reaper/reaper_e2e_test.go`)

**Purpose**: Test full reaper flow with real tmux sessions and Claude mocks.

#### Test: `TestReaper_FullFlow_WithMockClaude`
- Setup:
  - Create real tmux session
  - Launch mock Claude process (script that responds to /exit)
  - Create AGM session manifest
- Action:
  - Spawn agm-reaper with session name
  - Wait for completion (max 3 minutes)
- Assert:
  - Reaper log shows prompt detection
  - Reaper log shows /exit sent
  - Reaper log shows pane closed
  - Reaper log shows archive complete
  - Session manifest lifecycle = archived
  - Tmux session no longer exists

#### Test: `TestReaper_FullFlow_StuckSession`
- Setup:
  - Create tmux session with process that never returns prompt
- Action:
  - Spawn reaper with 10s prompt timeout
- Assert:
  - Reaper falls back to fixed wait after timeout
  - Reaper eventually sends /exit anyway
  - Log shows fallback triggered

#### Test: `TestReaper_FullFlow_ProcessDies`
- Setup:
  - Create tmux session
  - Kill process before reaper runs
- Action:
  - Spawn reaper
- Assert:
  - Reaper detects pane already closed
  - Archive completes successfully
  - No errors (graceful handling)

---

### 2.2 Archive Command Integration (`test/integration/lifecycle/archive_test.go`)

**Existing Coverage** ✅:
- Synchronous archive with tmux
- Archive validation
- List filtering

**Missing Coverage** (to add):

#### Test: `TestArchive_AsyncFlag_Integration`
- Setup:
  - Create real AGM session with tmux
  - Mock Claude ready state
- Action:
  - Run: `agm session archive SESSION --async`
- Assert:
  - Command returns immediately (< 1s)
  - Reaper process spawned (check ps output)
  - Log file created
  - Within 2 minutes: session archived
  - Reaper process no longer running

---

## 3. Docker E2E Tests

### 3.1 Docker Test Infrastructure

**Location**: `test/e2e/docker/`

**Dockerfile**:
```dockerfile
FROM ubuntu:22.04

# Install dependencies
RUN apt-get update && apt-get install -y \
    tmux \
    golang-1.21 \
    git \
    python3 \
    curl

# Setup test user
RUN useradd -m -s /bin/bash testuser
USER testuser
WORKDIR /home/testuser

# Copy AGM binaries
COPY --chown=testuser:testuser bin/agm /usr/local/bin/agm
COPY --chown=testuser:testuser bin/agm-reaper /usr/local/bin/agm-reaper

# Copy test scripts
COPY test/e2e/docker/scripts/ /tests/

# Setup test environment
RUN mkdir -p /home/testuser/sessions /home/testuser/.config/agm

ENTRYPOINT ["/bin/bash"]
```

**docker-compose.yml**:
```yaml
version: '3.8'

services:
  reaper-e2e-test:
    build:
      context: ../../../
      dockerfile: test/e2e/docker/Dockerfile
    image: agm-reaper-e2e:latest
    container_name: agm-reaper-e2e
    environment:
      - SESSIONS_DIR=/home/testuser/sessions
      - AGM_TEST_MODE=true
    volumes:
      - ./test-results:/home/testuser/test-results
    command: /tests/run-reaper-tests.sh
```

---

### 3.2 Docker Test Cases

#### Test: `test_reaper_happy_path.sh`
```bash
#!/bin/bash
set -e

echo "=== Reaper Happy Path E2E Test ==="

# Create tmux session
tmux new-session -d -s test-session

# Launch mock Claude (Python script that waits for /exit)
tmux send-keys -t test-session "python3 /tests/mock_claude.py" C-m

# Create AGM manifest
agm session associate test-session --create

# Wait for Claude ready signal
sleep 2

# Spawn async archive
agm session archive test-session --async

# Monitor reaper log
REAPER_LOG="/tmp/agm-reaper-test-session.log"
timeout 120 bash -c "while ! grep -q 'Session archived successfully' $REAPER_LOG; do sleep 1; done"

# Verify session archived
if agm session list --all | grep -q "test-session.*archived"; then
    echo "✓ Session archived successfully"
    exit 0
else
    echo "✗ Session not archived"
    cat $REAPER_LOG
    exit 1
fi
```

#### Test: `test_reaper_prompt_timeout.sh`
- Mock Claude never returns prompt
- Reaper falls back to fixed wait
- Archive completes anyway

#### Test: `test_reaper_binary_missing.sh`
- Remove agm-reaper binary
- Run `agm session archive --async`
- Verify error message shows installation instructions

#### Test: `test_agm_exit_workflow.sh`
- Create AGM session
- Send /agm:agm-exit command
- Verify reaper spawned
- Verify session exits automatically
- Verify session archived

---

## 4. BDD Tests (Ginkgo)

### 4.1 Feature: Async Archive

**Location**: `test/bdd/features/async_archive.feature`

```gherkin
Feature: Async Archive with Reaper
  As an AGM user
  I want to archive sessions asynchronously
  So that I don't have to wait for the archive to complete

  Scenario: Archive session with async flag
    Given I have an active AGM session "my-session"
    When I run "agm session archive my-session --async"
    Then the command completes immediately
    And a reaper process is spawned
    And a log file is created at "/tmp/agm-reaper-my-session.log"
    And within 2 minutes the session is archived

  Scenario: Reaper detects prompt and exits cleanly
    Given I have a Claude session responding normally
    When the reaper starts monitoring
    Then it waits for the prompt to appear
    And sends /exit command
    And waits for the pane to close
    And archives the session

  Scenario: Reaper handles stuck session
    Given I have a Claude session that never returns prompt
    When the reaper starts monitoring
    Then it times out after 90 seconds
    And falls back to 60 second fixed wait
    And still sends /exit command
    And completes the archive

  Scenario: Cannot use async with bulk archive
    When I run "agm session archive --all --async"
    Then the command fails
    And shows error "--async flag is not compatible with --all"
```

### 4.2 Feature: /agm:agm-exit Command

**Location**: `test/bdd/features/agm_exit.feature`

```gherkin
Feature: /agm:agm-exit Slash Command
  As a Claude Code user
  I want to exit and archive my session automatically
  So that I don't have to manually run two commands

  Scenario: Exit from AGM-associated session
    Given I am in a tmux session "test-agm-session"
    And the session is associated with AGM
    When I run /agm:agm-exit
    Then Claude spawns an async reaper
    And shows "✓ Async archive started"
    And shows the reaper PID
    And shows the log file path
    And the reaper monitors my session

  Scenario: Exit from non-AGM session
    Given I am in a tmux session "non-agm-session"
    And the session is NOT associated with AGM
    When I run /agm:agm-exit
    Then Claude shows error "❌ Session not associated with AGM"
    And shows message "Run /agm:agm-assoc first to associate this session"
    And no reaper is spawned

  Scenario: Exit from non-tmux environment
    Given I am NOT in a tmux session
    When I run /agm:agm-exit
    Then Claude shows error "❌ Not running in tmux session"
    And shows message "agm-exit requires tmux. Use /exit manually to exit Claude"
    And no reaper is spawned
```

---

## 5. Test Execution Strategy

### 5.1 Local Development

```bash
# Run all unit tests
make test-unit

# Run integration tests (requires tmux)
make test-integration

# Run Docker E2E tests
make test-e2e-docker

# Run all tests
make test-all
```

### 5.2 CI/CD Pipeline

```yaml
# .github/workflows/reaper-tests.yml
name: Reaper Tests

on: [push, pull_request]

jobs:
  unit-tests:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - uses: actions/setup-go@v4
        with:
          go-version: '1.21'
      - run: make test-unit

  integration-tests:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - uses: actions/setup-go@v4
        with:
          go-version: '1.21'
      - name: Install tmux
        run: sudo apt-get install -y tmux
      - run: make test-integration

  docker-e2e-tests:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - name: Run Docker E2E tests
        run: make test-e2e-docker
```

---

## 6. Success Criteria

✅ **High Confidence Metrics**:

1. **Code Coverage**: ≥80% for reaper and archive code
2. **Test Count**: ≥25 tests total (unit + integration + E2E)
3. **CI/CD**: All tests pass in automated pipeline
4. **Docker**: E2E tests run in isolation (no external dependencies)
5. **BDD**: Behavior specs match documentation
6. **Regression**: Tests catch the bug that removed --async flag

**Acceptance**: All tests pass, coverage ≥80%, Docker tests run in <5 minutes.

---

## 7. Implementation Timeline

- **Phase 1** (1 hour): Unit tests for spawnReaper() and --async flag
- **Phase 2** (2 hours): Integration tests for reaper flow
- **Phase 3** (3 hours): Docker E2E infrastructure and tests
- **Phase 4** (1 hour): BDD feature specs and implementation
- **Phase 5** (1 hour): CI/CD integration and documentation

**Total**: ~8 hours of work

---

## 8. Maintenance

**When to Update Tests**:
- Before changing reaper logic
- Before modifying archive command
- When adding new async features
- After any tmux integration changes

**Test Ownership**: agm-core-team
**Review Required**: All PR changes affecting reaper must update tests
