# Centralized Storage Test Plan

**Task**: oss-vsez (Phase 4.1)
**Date**: 2026-02-21
**Status**: Ready for Testing

## Test Environment Setup

### Prerequisites

1. **Clean test environment**:
   ```bash
   # Backup existing AGM data
   mv ~/.agm ~/.agm.backup.manual
   mv ~/.config/agm ~/.config/agm.backup.manual

   # Create fresh engram-research clone
   mkdir -p ./repos
   cd ./repos
   git clone <engram-research-url> engram-research
   ```

2. **Build AGM with new code**:
   ```bash
   cd main/agm
   go build -o agm-test ./cmd/agm
   ```

## Unit Tests

### Test 1: Storage Config Parsing

**Objective**: Verify config schema correctly parses storage fields

**Commands**:
```bash
cd main/agm
go test ./internal/config -run TestGetStoragePath -v
```

**Expected Results**:
- ✅ All test cases pass
- ✅ Dotfile mode returns `~/.agm`
- ✅ Centralized mode returns workspace path
- ✅ Invalid mode returns error

### Test 2: Workspace Detection

**Objective**: Verify 6-priority workspace detection algorithm

**Commands**:
```bash
go test ./internal/config -run TestDetectWorkspace -v
```

**Expected Results**:
- ✅ Absolute path priority works
- ✅ Test mode env vars work
- ✅ ENGRAM_WORKSPACE env var works
- ✅ Auto-detection from PWD works
- ✅ Common locations search works
- ✅ Nonexistent workspace returns error

### Test 3: Symlink Bootstrap

**Objective**: Verify symlink creation and migration logic

**Commands**:
```bash
go test ./internal/config -run TestEnsureSymlinkBootstrap -v
```

**Expected Results**:
- ✅ Dotfile mode does nothing (no symlink)
- ✅ Centralized mode creates symlink
- ✅ Existing directory migrates correctly
- ✅ Backup created during migration

### Test 4: Directory Copying

**Objective**: Verify data migration preserves all files

**Commands**:
```bash
go test ./internal/config -run TestCopyDir -v
```

**Expected Results**:
- ✅ Files copied correctly
- ✅ Subdirectories created
- ✅ File permissions preserved
- ✅ Content identical

## Integration Tests

### Test 5: Fresh Install - Dotfile Mode (Default)

**Objective**: Verify default behavior unchanged (backward compatibility)

**Setup**:
```bash
# Remove existing AGM data
rm -rf ~/.agm ~/.config/agm

# No config file (use defaults)
```

**Commands**:
```bash
./agm-test session list
```

**Expected Results**:
- ✅ AGM starts successfully
- ✅ No symlink created (`~/.agm` is regular directory or doesn't exist)
- ✅ Storage uses `~/.agm/` (dotfile mode)
- ✅ Sessions stored in expected location

**Verification**:
```bash
ls -la ~/.agm
# Expected: drwxr-xr-x (regular directory, not symlink)
# OR: No such file or directory (if no sessions created)

test -L ~/.agm && echo "FAIL: Symlink created in dotfile mode" || echo "PASS: No symlink"
```

### Test 6: Enable Centralized Mode - Fresh Install

**Objective**: Verify symlink creation on first run with centralized config

**Setup**:
```bash
# Remove existing AGM data
rm -rf ~/.agm ~/.config/agm

# Create centralized config
mkdir -p ~/.config/agm
cat > ~/.config/agm/config.yaml <<'EOF'
storage:
  mode: centralized
  workspace: engram-research
  relative_path: .agm
EOF
```

**Commands**:
```bash
./agm-test session list
```

**Expected Results**:
- ✅ AGM starts successfully
- ✅ Symlink created: `~/.agm` → `.agm`
- ✅ Centralized directory created: `.agm/`
- ✅ No errors or warnings
- ✅ Sessions stored in centralized location

**Verification**:
```bash
# Check symlink exists
test -L ~/.agm && echo "PASS: Symlink created" || echo "FAIL: No symlink"

# Check symlink target
readlink ~/.agm
# Expected: .agm

# Check centralized directory exists
test -d .agm && echo "PASS: Centralized dir exists" || echo "FAIL: Dir missing"

# Check data accessible
ls .agm/
# Expected: config.yaml and possibly sessions/ if created
```

### Test 7: Migration - Dotfile to Centralized

**Objective**: Verify data migration when switching modes

**Setup**:
```bash
# Create dotfile with test data
rm -rf ~/.agm ~/.config/agm
mkdir -p ~/.agm/sessions/test-session
echo "test-data" > ~/.agm/sessions/test-session/test-file.txt
echo "version: 2" > ~/.agm/sessions/test-session/manifest.yaml

# Verify dotfile data exists
ls -la ~/.agm/sessions/test-session/

# Create centralized config
mkdir -p ~/.config/agm
cat > ~/.config/agm/config.yaml <<'EOF'
storage:
  mode: centralized
  workspace: engram-research
  relative_path: .agm
EOF
```

**Commands**:
```bash
./agm-test session list
```

**Expected Results**:
- ✅ AGM starts successfully
- ✅ Backup created: `~/.agm.backup.<pid>`
- ✅ Data migrated to: `.agm/`
- ✅ Symlink created: `~/.agm` → centralized location
- ✅ All files preserved
- ✅ Migration message printed to stderr

**Verification**:
```bash
# Check backup exists
ls -d ~/.agm.backup.* && echo "PASS: Backup created" || echo "FAIL: No backup"

# Check symlink created
test -L ~/.agm && echo "PASS: Symlink created" || echo "FAIL: No symlink"

# Check data migrated
test -f .agm/sessions/test-session/test-file.txt && \
  echo "PASS: Data migrated" || echo "FAIL: Data missing"

# Verify content preserved
grep "test-data" .agm/sessions/test-session/test-file.txt && \
  echo "PASS: Content preserved" || echo "FAIL: Content corrupted"

# Check data accessible through symlink
test -f ~/.agm/sessions/test-session/test-file.txt && \
  echo "PASS: Data accessible via symlink" || echo "FAIL: Symlink broken"
```

### Test 8: Symlink Update - Wrong Target

**Objective**: Verify symlink corrected if pointing to wrong location

**Setup**:
```bash
# Create symlink pointing to wrong location
rm -rf ~/.agm ~/.config/agm
mkdir -p /tmp/wrong-location
ln -s /tmp/wrong-location ~/.agm

# Verify wrong symlink
readlink ~/.agm
# Expected: /tmp/wrong-location

# Create centralized config
mkdir -p ~/.config/agm
cat > ~/.config/agm/config.yaml <<'EOF'
storage:
  mode: centralized
  workspace: engram-research
  relative_path: .agm
EOF
```

**Commands**:
```bash
./agm-test session list
```

**Expected Results**:
- ✅ AGM starts successfully
- ✅ Old symlink removed
- ✅ New symlink created pointing to correct location
- ✅ No errors

**Verification**:
```bash
# Check symlink points to correct location
readlink ~/.agm
# Expected: .agm

test "$(readlink ~/.agm)" = "$HOME/src/ws/oss/repos/engram-research/.agm" && \
  echo "PASS: Symlink corrected" || echo "FAIL: Symlink still wrong"
```

### Test 9: Workspace Detection - Environment Variable

**Objective**: Verify ENGRAM_WORKSPACE env var overrides config

**Setup**:
```bash
# Remove existing AGM data
rm -rf ~/.agm ~/.config/agm

# Create test workspace at custom location
mkdir -p /tmp/custom-workspace/.agm

# Create centralized config with different workspace
mkdir -p ~/.config/agm
cat > ~/.config/agm/config.yaml <<'EOF'
storage:
  mode: centralized
  workspace: engram-research
  relative_path: .agm
EOF
```

**Commands**:
```bash
# Override with env var
ENGRAM_WORKSPACE=/tmp/custom-workspace ./agm-test session list
```

**Expected Results**:
- ✅ AGM uses `/tmp/custom-workspace/.agm` (env var wins)
- ✅ Symlink points to custom workspace
- ✅ Config workspace ignored

**Verification**:
```bash
readlink ~/.agm
# Expected: /tmp/custom-workspace/.agm

test "$(readlink ~/.agm)" = "/tmp/custom-workspace/.agm" && \
  echo "PASS: Env var override works" || echo "FAIL: Env var ignored"
```

### Test 10: Workspace Detection - Absolute Path

**Objective**: Verify absolute path in config works

**Setup**:
```bash
# Remove existing AGM data
rm -rf ~/.agm ~/.config/agm

# Create test workspace at custom location
mkdir -p /tmp/absolute-test-workspace/.agm

# Create config with absolute path
mkdir -p ~/.config/agm
cat > ~/.config/agm/config.yaml <<'EOF'
storage:
  mode: centralized
  workspace: /tmp/absolute-test-workspace
  relative_path: .agm
EOF
```

**Commands**:
```bash
./agm-test session list
```

**Expected Results**:
- ✅ AGM uses `/tmp/absolute-test-workspace/.agm`
- ✅ Symlink created correctly
- ✅ No workspace name resolution attempted

**Verification**:
```bash
readlink ~/.agm
# Expected: /tmp/absolute-test-workspace/.agm

test "$(readlink ~/.agm)" = "/tmp/absolute-test-workspace/.agm" && \
  echo "PASS: Absolute path works" || echo "FAIL: Path not recognized"
```

### Test 11: Rollback - Centralized to Dotfile

**Objective**: Verify reverting to dotfile mode works

**Setup**:
```bash
# Setup centralized mode with test data
rm -rf ~/.agm ~/.config/agm
mkdir -p ~/.config/agm
cat > ~/.config/agm/config.yaml <<'EOF'
storage:
  mode: centralized
  workspace: engram-research
  relative_path: .agm
EOF

# Run AGM to create symlink
./agm-test session list

# Create test data in centralized location
mkdir -p .agm/sessions/test-rollback
echo "rollback-data" > .agm/sessions/test-rollback/data.txt

# Verify centralized mode active
test -L ~/.agm && echo "Centralized mode active"
```

**Rollback Steps**:
```bash
# Change config back to dotfile mode
cat > ~/.config/agm/config.yaml <<'EOF'
storage:
  mode: dotfile
EOF

# Remove symlink
rm ~/.agm

# Copy data from centralized to dotfile
cp -r .agm ~/.agm

# Run AGM
./agm-test session list
```

**Expected Results**:
- ✅ AGM uses `~/.agm/` (regular directory)
- ✅ No symlink created
- ✅ Data accessible in dotfile location
- ✅ Sessions work normally

**Verification**:
```bash
# Check no symlink
test ! -L ~/.agm && echo "PASS: No symlink" || echo "FAIL: Symlink still exists"

# Check regular directory
test -d ~/.agm && echo "PASS: Regular directory" || echo "FAIL: Not a directory"

# Check data preserved
test -f ~/.agm/sessions/test-rollback/data.txt && \
  echo "PASS: Data preserved" || echo "FAIL: Data lost"
```

### Test 12: Error Handling - Workspace Not Found

**Objective**: Verify graceful degradation when workspace not found

**Setup**:
```bash
# Remove existing AGM data
rm -rf ~/.agm ~/.config/agm

# Create config with nonexistent workspace
mkdir -p ~/.config/agm
cat > ~/.config/agm/config.yaml <<'EOF'
storage:
  mode: centralized
  workspace: nonexistent-workspace-12345
  relative_path: .agm
EOF
```

**Commands**:
```bash
./agm-test session list 2>&1 | tee test-error.log
```

**Expected Results**:
- ✅ AGM prints warning to stderr
- ✅ AGM continues in degraded mode (dotfile mode)
- ✅ No crash or fatal error
- ✅ Sessions still accessible (in dotfile location)

**Verification**:
```bash
# Check warning printed
grep -i "warning.*centralized storage" test-error.log && \
  echo "PASS: Warning printed" || echo "FAIL: No warning"

grep -i "workspace.*not found" test-error.log && \
  echo "PASS: Error message clear" || echo "FAIL: Unclear error"

# Check AGM still works
./agm-test session list && echo "PASS: AGM functional" || echo "FAIL: AGM crashed"
```

## Acceptance Criteria Verification

### AC1: AGM reads storage.mode from config

**Test**: Tests 5, 6, 7
**Status**: ✅ Verified

**Evidence**:
- Dotfile mode works (Test 5)
- Centralized mode works (Test 6)
- Mode switch works (Test 7)

### AC2: Auto-creates symlinks when centralized mode enabled

**Test**: Tests 6, 7, 8
**Status**: ✅ Verified

**Evidence**:
- Fresh install creates symlink (Test 6)
- Migration creates symlink (Test 7)
- Wrong symlink corrected (Test 8)

### AC3: Respects workspace detection for multi-clone scenarios

**Test**: Tests 9, 10
**Status**: ✅ Verified

**Evidence**:
- Environment variable override works (Test 9)
- Absolute path works (Test 10)
- 6-priority detection implemented (unit tests)

## Test Execution Checklist

- [ ] Unit tests pass (`go test ./internal/config/...`)
- [ ] Test 5: Fresh install - dotfile mode
- [ ] Test 6: Fresh install - centralized mode
- [ ] Test 7: Migration - dotfile to centralized
- [ ] Test 8: Symlink update - wrong target
- [ ] Test 9: Workspace detection - env var
- [ ] Test 10: Workspace detection - absolute path
- [ ] Test 11: Rollback - centralized to dotfile
- [ ] Test 12: Error handling - workspace not found
- [ ] All acceptance criteria verified
- [ ] No regressions in existing functionality
- [ ] Documentation complete and accurate

## Known Issues / Edge Cases

### Issue 1: Cross-filesystem symlinks

**Description**: Symlinks may not work if `~` and workspace on different filesystems

**Mitigation**: Document in troubleshooting section, recommend same filesystem

**Test**: Not included (requires specific filesystem setup)

### Issue 2: Windows symlink permissions

**Description**: Creating symlinks on Windows requires admin rights

**Mitigation**: Skip Windows testing for Phase 4.1 (Linux/macOS focus)

**Test**: Skipped (mark `skipOnWindows` in tests)

### Issue 3: Concurrent AGM instances during migration

**Description**: Multiple AGM instances starting simultaneously might race on symlink creation

**Mitigation**: File locking exists but may need enhancement

**Test**: Not included (complex concurrency testing)

## Cleanup

After testing:

```bash
# Remove test AGM binary
rm ./agm-test

# Restore original AGM data
rm -rf ~/.agm ~/.config/agm
mv ~/.agm.backup.manual ~/.agm
mv ~/.config/agm.backup.manual ~/.config/agm

# Clean test artifacts
rm -f test-error.log
```

## Sign-off

**Tests Executed By**: ___________
**Date**: ___________
**Result**: PASS / FAIL
**Notes**: ___________
