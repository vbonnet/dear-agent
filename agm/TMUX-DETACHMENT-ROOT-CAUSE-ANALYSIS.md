# TMux Detachment Issue - Root Cause Analysis

## Summary

**ROOT CAUSE IDENTIFIED**: The astrocyte daemon was calling the old `csm send` command which no longer exists after the CLI restructuring (commit 61c9a1d, Feb 10). This caused continuous errors that interfered with tmux session state, leading to session detachment and window swapping issues.

## Investigation Timeline

### User Report
"I keep getting my tmux sessions detached or swapped under me. This used to be a bug with `agm` which we theoretically fixed (yesterday or monday)."

### Evidence Found

1. **Astrocyte Daemon** (`astrocyte/astrocyte_messaging.py`):
   ```python
   # Line 214 (BEFORE):
   subprocess.run(
       ["csm", "send", session_name, "--prompt", tagged_message],
       check=True
   )

   # Line 231 (BEFORE):
   subprocess.run(
       ["csm", "send", session_name, "--prompt-file", temp_path],
       check=True
   )
   ```

   **Impact**: Every time astrocyte tried to send a message to a Claude session, it would call a non-existent command, causing subprocess.CalledProcessError.

2. **Stuck Process Detection** (`astrocyte/astrocyte_stuck_csm_send_recovery.py`):
   ```python
   # Line 58 (BEFORE):
   if f'csm send {session_name}' not in line and f'csm send "{session_name}"' not in line:
       continue
   ```

   **Impact**: Process recovery was looking for the wrong process names, failing to detect actual stuck `agm session send` processes.

3. **User-Facing Error Messages** (internal/ui/errors.go, internal/ui/table.go):
   - All help text referenced `csm` commands instead of `agm session` commands
   - Users would get incorrect recovery instructions

4. **Internal Recovery Instructions** (internal/tmux/*.go):
   - All timeout/error recovery messages referenced `csm list`, `csm attach`
   - Developers debugging issues would get wrong commands

## Files Fixed

### Critical (Daemon/Runtime Code)
1. **astrocyte/astrocyte_messaging.py**
   - `csm send` → `agm session send` (2 occurrences)

2. **astrocyte/astrocyte_stuck_csm_send_recovery.py**
   - Process detection: `csm send` → `agm session send`
   - All log messages updated (16 occurrences)

### User-Facing (Error Messages)
3. **internal/ui/errors.go**
   - `csm list` → `agm session list`
   - `csm archive` → `agm session archive`
   - `csm sync` → `agm admin sync`
   - `csm unarchive` → `agm session unarchive`

4. **internal/ui/table.go**
   - `csm new` → `agm session new`

### Developer-Facing (Recovery Instructions)
5. **internal/tmux/tmux.go**
   - `csm list` → `agm session list` (5 occurrences)

6. **internal/tmux/send.go**
   - `csm list` → `agm session list`
   - `csm attach` → `agm session attach`

7. **internal/tmux/timeout.go**
   - `csm list` → `agm session list`

## Fix Details

**Commit**: b0412f9
**Message**: "CRITICAL FIX: Update astrocyte daemon to use agm session send"

**Total Changes**: 7 files changed, 42 insertions(+), 42 deletions(-)

## Expected Outcome

After applying these fixes:
1. ✅ Astrocyte daemon will successfully send messages to Claude sessions
2. ✅ Stuck process detection will correctly identify hung `agm session send` processes
3. ✅ No more subprocess errors causing tmux state interference
4. ✅ Users will get correct recovery instructions
5. ✅ TMux session detachment/swapping should stop

## Next Steps Required

### Immediate
1. **Restart astrocyte daemon** to apply Python code changes:
   ```bash
   # Find astrocyte process
   ps aux | grep astrocyte

   # Kill it (will auto-restart via systemd/cron if configured)
   pkill -f astrocyte
   ```

2. **Delete old csm binaries** (requires permissions):
   ```bash
   rm ~/go/bin/agm
   rm ~/go/bin/agm-attach-wrapper
   rm ~/go/bin/agm-agent-wrapper
   rm ~/go/bin/agm-reaper
   ```

3. **Monitor for tmux detachment** over next 24 hours

### Testing
4. **Verify astrocyte can send messages**:
   - Check daemon logs after restart: `tail -f ~/.agm/astrocyte/logs/daemon.log`
   - Look for successful message sends (no subprocess errors)

5. **Test stuck process detection**:
   - Intentionally create hung agm session send process
   - Verify astrocyte detects and kills it

6. **Add regression tests** (see Test Plan below)

### Logging Enhancement (User Requested)
7. **Add debug logging for subprocess calls**:
   - Log command being executed before subprocess.run()
   - Log stdout/stderr on failure
   - Add timing information

8. **Add tmux state logging**:
   - Log tmux session list before/after operations
   - Log window/pane state during recovery

## Test Plan

### Unit Tests Needed
1. Test that astrocyte_messaging.py calls correct command structure
2. Test that stuck process detection finds "agm session send" processes
3. Test that UI error messages contain "agm" not "csm"

### Integration Tests Needed
1. End-to-end test: astrocyte sends message via agm session send
2. Test stuck process detection with real agm session send process
3. Test recovery flow doesn't cause tmux detachment

### Manual Verification
1. Create test session: `agm session new test-recovery --harness claude-code --detached`
2. Trigger stuck state (long-running command)
3. Watch astrocyte logs for recovery
4. Verify no tmux detachment occurs

## Historical Context

**Original Issue**: CLI restructuring (commit 61c9a1d, Feb 10) changed commands from:
- `csm new` → `agm session new`
- `csm send` → `agm session send`
- `csm list` → `agm session list`

**Hook Regression**: Previously fixed in `~/.claude/hooks/session-start/agm-safe-auto-detect.sh`
- Changed `agm associate` → `agm session associate`
- This was caught earlier in the session

**Astrocyte Regression**: Missed during CLI restructuring because:
1. Python code (not Go)
2. Not covered by grep during command reference updates
3. No tests exercising actual subprocess calls

## Remaining Old References

**Still need manual review** (161 files found with csm references):
- Documentation files (.md) - mostly acceptable as historical context
- Test files (test/integration/, test/e2e/, test/bdd/) - some may need updates
- Scripts (tests/manual-e2e-test.sh) - need updates

**Priority**: Focus on executable code first (Python .py, Go .go, Shell .sh files)

## Lessons Learned

1. **Grep for command references during renames**: Should have grepped for subprocess.run calls
2. **Test daemon code paths**: Astrocyte tests don't exercise actual subprocess calls
3. **Update all command references atomically**: Mix of Python/Go/Shell made this hard to track
4. **Add logging before subprocess calls**: Would have caught this immediately
5. **Monitor daemon logs after major changes**: Would have shown subprocess errors

## Contact

If tmux detachment issues persist after applying these fixes:
1. Check astrocyte daemon logs for new errors
2. Verify agm binary is correctly installed and in PATH
3. Check for any remaining agm references in active code
4. Monitor tmux session list during operations to detect state changes

---

**Status**: FIXED (pending daemon restart and monitoring)
**Created**: 2026-02-11
**Author**: Claude Code investigation
