# Build and Verify Guide - Archive Dolt Migration

## Quick Start

This guide walks you through building AGM with the Dolt archive fix and verifying it works correctly.

**Estimated Time**: 15 minutes

---

## Step 1: Prerequisites

### 1.1 Check Environment

```bash
# Verify Go installation
go version
# Expected: go version go1.21.x or higher

# Verify Dolt installation
dolt version
# Expected: dolt version 1.x.x

# Check workspace is set
echo $WORKSPACE
# Expected: oss (or your workspace name)

# If not set:
export WORKSPACE=oss
```

### 1.2 Verify Dolt Server is Running

```bash
# Check if Dolt SQL server is running
ps aux | grep "dolt sql-server" | grep -v grep

# If not running, start it:
cd ~/src/ws/${WORKSPACE}/.dolt/dolt-db
dolt sql-server -H 127.0.0.1 -P 3307 &

# Test connection
dolt sql -q "SELECT 1 as test"
# Expected:
# +------+
# | test |
# +------+
# | 1    |
# +------+
```

---

## Step 2: Build AGM

### 2.1 Navigate to Project

```bash
cd main/agm
```

### 2.2 Verify Code Changes

```bash
# Check that the fix files exist
ls -la internal/dolt/sessions.go
ls -la cmd/agm/archive.go

# Verify ResolveIdentifier method exists
grep -n "func (a \*Adapter) ResolveIdentifier" internal/dolt/sessions.go
# Expected: Line number showing the function definition
```

### 2.3 Build

```bash
# Clean build
go clean

# Build AGM binary
go build -o ~/go/bin/agm ./cmd/agm

# Verify build success
echo $?
# Expected: 0 (success)

# Check binary exists
ls -lh ~/go/bin/agm
# Expected: -rwxr-xr-x ... agm
```

### 2.4 Verify AGM is Usable

```bash
# Check version
agm version

# Test basic command
agm session list
# Should show sessions (or "No active sessions found")
```

---

## Step 3: Run Tests

### 3.1 Unit Tests

```bash
# Run Dolt adapter tests
go test ./internal/dolt/... -v

# Expected output should include:
# === RUN   TestResolveIdentifier
# --- PASS: TestResolveIdentifier
# === RUN   TestResolveIdentifierExcludesArchived
# --- PASS: TestResolveIdentifierExcludesArchived
# === RUN   TestResolveIdentifierWithDuplicateNames
# --- PASS: TestResolveIdentifierWithDuplicateNames
# PASS
# ok      github.com/vbonnet/dear-agent/agm/internal/dolt
```

### 3.2 Integration Tests (Optional)

```bash
# Enable integration tests
export DOLT_TEST_INTEGRATION=1

# Run archive integration tests
go test ./test/integration/lifecycle/... -tags=integration -v -run Archive

# Expected: All tests pass
# PASS
# ok      github.com/vbonnet/dear-agent/agm/test/integration/lifecycle
```

### 3.3 Check for Errors

```bash
# Verify no compilation errors
go build ./...

# Run linter (if installed)
golangci-lint run ./... 2>&1 | head -20
```

---

## Step 4: Manual Verification

### 4.1 Create Test Session (if needed)

```bash
# Check if you have any STOPPED sessions
agm session list --json | jq -r '.[] | select(.status=="STOPPED") | .name'

# If no STOPPED sessions, you can create one for testing:
# (Skip if you have existing STOPPED sessions to test with)

# Note: Creating a test session requires tmux. For verification purposes,
# it's better to use existing STOPPED sessions from the production data.
```

### 4.2 Test Archive by Session ID

```bash
# Get a STOPPED session
SESSION_JSON=$(agm session list --json | jq -r '.[0] | select(.status=="STOPPED")')
SESSION_ID=$(echo "$SESSION_JSON" | jq -r '.session_id')
SESSION_NAME=$(echo "$SESSION_JSON" | jq -r '.name')

echo "Testing with session ID: $SESSION_ID"
echo "Session name: $SESSION_NAME"

# Archive by session ID
agm session archive "$SESSION_ID"

# Expected output:
# ✓ Archived session: <session-id>
#
# The session is now hidden from 'agm session list'.
# Use 'agm session list --all' to see archived sessions.
```

### 4.3 Verify Archive Success

```bash
# Check session is NOT in default list
agm session list | grep "$SESSION_NAME"
# Expected: No output (empty)

# Check session IS in --all list
agm session list --all --json | jq -r ".[] | select(.session_id==\"$SESSION_ID\") | .lifecycle"
# Expected: "archived"

# Verify in Dolt directly
dolt sql -q "SELECT id, name, status FROM agm_sessions WHERE id='$SESSION_ID'"
# Expected: Shows status='archived'
```

### 4.4 Test Archive by Tmux Name

```bash
# Get another STOPPED session
TMUX_SESSION=$(agm session list --json | jq -r '.[1] | select(.status=="STOPPED")')
TMUX_NAME=$(echo "$TMUX_SESSION" | jq -r '.tmux.session_name')

echo "Testing with tmux name: $TMUX_NAME"

# Archive by tmux name
agm session archive "$TMUX_NAME"

# Expected: ✓ Archived session: <tmux-name>

# Verify
agm session list --all | grep "$TMUX_NAME"
# Should show in archived list
```

### 4.5 Test Error Handling

```bash
# Try to archive non-existent session
agm session archive non-existent-session-12345

# Expected output:
# ✗ Session not found: non-existent-session-12345
#   Storage: Dolt storage
#
# Session 'non-existent-session-12345' not found.
```

---

## Step 5: Verification Checklist

Complete this checklist to confirm the fix is working:

### Build & Tests
- [ ] AGM builds without errors: `go build ./...`
- [ ] Unit tests pass: `go test ./internal/dolt/... -v`
- [ ] Integration tests pass (if enabled): `DOLT_TEST_INTEGRATION=1 go test ./test/integration/lifecycle/... -tags=integration`
- [ ] No lint errors: `golangci-lint run ./...`

### Functionality
- [ ] Can archive by session ID: `agm session archive <session-id>`
- [ ] Can archive by tmux name: `agm session archive <tmux-name>`
- [ ] Can archive by manifest name: `agm session archive <manifest-name>`
- [ ] Archived sessions hidden from default list: `agm session list`
- [ ] Archived sessions shown with `--all`: `agm session list --all`
- [ ] Clear error for non-existent session: `agm session archive fake-session`

### Database Verification
- [ ] Dolt shows status='archived' after archiving
- [ ] Lifecycle field updates in agm_sessions table
- [ ] No duplicate or orphaned records

### Regression
- [ ] Original bug fixed: Can archive STOPPED sessions that exist in Dolt
- [ ] Cannot re-archive already archived sessions

---

## Step 6: Performance Check (Optional)

```bash
# Measure archive operation time
time agm session archive <session-name>

# Expected: < 1 second for single archive

# Bulk operation performance
time for session in $(agm session list --json | jq -r '.[].name' | head -5); do
    agm session archive "$session" 2>/dev/null || true
done

# Expected: < 5 seconds for 5 sessions
```

---

## Troubleshooting

### Issue: "failed to connect to Dolt"

**Solution**:
```bash
# Check Dolt server status
ps aux | grep "dolt sql-server"

# Restart Dolt server
pkill -f "dolt sql-server"
cd ~/src/ws/${WORKSPACE}/.dolt/dolt-db
dolt sql-server -H 127.0.0.1 -P 3307 &

# Wait for startup
sleep 3

# Test connection
dolt sql -q "SELECT 1"
```

### Issue: "WORKSPACE environment variable not set"

**Solution**:
```bash
# Set workspace
export WORKSPACE=oss

# Make permanent (add to ~/.bashrc or ~/.zshrc)
echo 'export WORKSPACE=oss' >> ~/.bashrc
source ~/.bashrc
```

### Issue: Build fails with import errors

**Solution**:
```bash
# Update dependencies
go mod tidy

# Download dependencies
go mod download

# Verify go.mod
go mod verify

# Rebuild
go clean
go build -o ~/go/bin/agm ./cmd/agm
```

### Issue: Tests fail with database errors

**Solution**:
```bash
# Apply Dolt migrations
cd ~/src/ws/${WORKSPACE}/.dolt/dolt-db
dolt sql < main/agm/internal/dolt/migrations/*.sql

# Or use AGM to apply migrations
agm doctor --fix-migrations
```

### Issue: "session not found" for existing session

**Debug Steps**:
```bash
# 1. Verify session exists in Dolt
dolt sql -q "SELECT id, name, tmux_session_name, status FROM agm_sessions WHERE name='<session-name>'"

# 2. Check workspace matches
echo $WORKSPACE
dolt sql -q "SELECT DISTINCT workspace FROM agm_sessions"

# 3. Verify session is not archived
dolt sql -q "SELECT status FROM agm_sessions WHERE name='<session-name>'"

# 4. Check AGM can connect to Dolt
agm session list
```

---

## Rollback (If Needed)

If you need to rollback to the previous version:

```bash
# 1. Stash current changes
cd main/agm
git stash

# 2. Checkout previous commit
git checkout HEAD~1

# 3. Rebuild
go build -o ~/go/bin/agm ./cmd/agm

# 4. Or restore from backup (if you made one)
cp ~/go/bin/agm.backup ~/go/bin/agm
```

---

## Success Criteria

The fix is successfully deployed when:

✅ **All tests pass**
- Unit tests: `go test ./internal/dolt/... -v`
- Integration tests: `DOLT_TEST_INTEGRATION=1 go test ./test/integration/lifecycle/... -tags=integration`

✅ **Manual verification complete**
- Can archive by session ID
- Can archive by tmux name
- Can archive by manifest name
- Archived sessions properly hidden/shown

✅ **Original bug fixed**
- STOPPED sessions that exist in Dolt can be archived
- No more "session not found" errors for valid sessions

✅ **No regressions**
- Existing functionality still works
- No new errors or warnings

---

## Next Steps

After successful verification:

1. **Deploy to Production**
   ```bash
   # Copy binary to production path
   sudo cp ~/go/bin/agm /usr/local/bin/agm

   # Or install system-wide
   cd main/agm
   sudo make install
   ```

2. **Update Documentation**
   - Mark verification checklist as complete
   - Update CHANGELOG.md
   - Add release notes

3. **Monitor**
   - Check logs for Dolt connection errors
   - Monitor archive command usage
   - Track any reported issues

4. **Plan Future Work**
   - Migrate other commands to Dolt (resume, kill, unarchive)
   - Remove legacy filesystem code
   - Complete Dolt migration

---

## References

- **Migration Guide**: `docs/ARCHIVE-DOLT-MIGRATION.md`
- **Testing Runbook**: `docs/testing/ARCHIVE-DOLT-RUNBOOK.md`
- **Testing Overview**: `docs/testing/README.md`
- **Dolt Documentation**: https://docs.dolthub.com/

---

## Support

If you encounter issues:

1. Check **Troubleshooting** section above
2. Review **docs/ARCHIVE-DOLT-MIGRATION.md**
3. Run **agm doctor** for automated diagnostics
4. File issue in GitHub repository
5. Open a GitHub issue

---

**Last Updated**: 2026-03-12
**Version**: 1.0.0
**Status**: Ready for verification
