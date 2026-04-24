# AGM Critical Bug Fix Report

**Date:** 2025-12-17
**Investigator:** Claude Sonnet 4.5
**Session:** wayfinder

## Executive Summary

Fixed critical bugs in Agent Session Manager (AGM) that caused:
1. Multiple sessions sharing the same Claude UUID (12 sessions affected)
2. Duplicate session directories (old vs new naming format)
3. Broken UUID assignment logic in `agm admin sync`

## Bug #1: Shared Claude UUID

### Symptoms
12 sessions were sharing the SAME Claude UUID (`70875f41-e829-47e3-81a7-9670422f9c36`):
- session-claude-1 through session-claude-5
- session-wayfinder (current conversation)
- session-agm-resilience
- session-agent-to-agent
- session-tool-call
- session-pr-commits
- session-grouper
- session-claude-2-fix

### Root Cause
Found in `~/src/repos/ai-tools/main/agm/cmd/csm/sync.go`:

```go
// Get the most recent Claude UUID from history
var latestUUID string
if len(historyEntries) > 0 {
    latestUUID = historyEntries[len(historyEntries)-1].SessionID
}

// Later, when creating manifests:
Claude: manifest.Claude{
    UUID: latestUUID, // BUG: Uses same UUID for ALL sessions
}
```

**The Problem:**
- When `agm admin sync` ran, it found multiple active tmux sessions without Claude UUIDs
- Instead of leaving them empty or generating unique UUIDs, it assigned the SAME "latest UUID from history" to ALL of them
- This happened on Dec 16, 19:39:54 when session-claude-{1-5} were all created within 1 second

**Why This Is Wrong:**
- Each tmux session running Claude has its own unique Claude conversation
- There's no reliable way to know which tmux session corresponds to which Claude UUID
- The "latest UUID" might not even belong to any of the active sessions

### Fix Applied
Modified `syncActiveTmuxSessions()` in `~/src/repos/ai-tools/main/agm/cmd/csm/sync.go`:

**Before:**
- Auto-assigned latest UUID from history to all sessions with empty UUIDs
- Created manifests with potentially incorrect UUIDs

**After:**
- Creates manifests with EMPTY Claude UUIDs
- Prompts user to manually associate each session using `agm session associate <session-name>`
- Only the user knows which tmux session corresponds to which Claude conversation

### Files Changed
- `~/src/repos/ai-tools/main/agm/cmd/csm/sync.go` (lines 178-301)

## Bug #2: Duplicate Session Directories

### Symptoms
Sessions existed in both old and new naming formats:
- Old format: `claude-1-session`, `claude-2-session`, etc.
- New format: `session-claude-1`, `session-claude-2`, etc.

This caused duplicate entries in `agm session list` and confusion about which directory was active.

### Root Cause
AGM changed its directory naming convention from `<name>-session` to `session-<name>`, but old directories were not cleaned up or migrated.

### Fix Applied
1. Created archive directory: `~/src/sessions/.archive-old-format/`
2. Moved all old format directories to archive:
   - `claude-1-session`
   - `claude-2-session`
   - `claude-3-session`
   - `claude-4-session`
   - `claude-demo-session`
   - `acme-mcp-session`

3. Enhanced `agm admin doctor` to detect this pattern automatically

### Files Changed
- `~/src/repos/ai-tools/main/agm/cmd/csm/doctor.go` (added `detectDuplicateSessionDirs()`)

## Enhancement: agm admin doctor

### New Diagnostics Added
The `agm admin doctor` command now detects:

1. **Duplicate session directories** (old vs new format)
   - Identifies sessions with both `<name>-session` and `session-<name>` directories
   - Provides archive commands to clean up

2. **Sessions sharing the same Claude UUID**
   - Critical issue that breaks `agm session resume`
   - Shows which sessions share which UUIDs
   - Recommends using `agm session associate` to fix

3. **Sessions with empty Claude UUIDs**
   - Sessions that haven't been associated with a Claude conversation
   - Lists affected sessions
   - Provides association instructions

### Example Output
```
--- Checking for duplicate Claude UUIDs ---
⚠ Found 1 Claude UUID(s) shared by multiple sessions
  • UUID 70875f41... is shared by 12 sessions:
    - agent-to-agent
    - claude-1
    - claude-2
    - claude-2-fix
    - claude-3
    - claude-4
    - claude-5
    - csm-resilience
    - grouper
    - pr-commits
    - tool-call
    - wayfinder

  Recommendation: Each session should have a unique Claude UUID
    Use 'agm session associate <session-name>' to assign correct UUIDs
```

### Files Changed
- `~/src/repos/ai-tools/main/agm/cmd/csm/doctor.go` (comprehensive rewrite)

## Current Status

### Fixed
✅ `agm admin sync` no longer assigns the same UUID to multiple sessions
✅ `agm admin doctor` detects duplicate UUIDs and directories
✅ Old format session directories archived
✅ No more duplicate session entries in `agm session list`

### Remaining Issues (Require Manual Intervention)
⚠️ 12 sessions still share UUID `70875f41-e829-47e3-81a7-9670422f9c36`
⚠️ 1 session (agm-close) has empty UUID

### Manual Remediation Required

**For sessions sharing UUID `70875f41...`:**
The user must determine which session should actually keep this UUID by:
1. Checking Claude history: `~/.claude/history.jsonl`
2. Looking at session-env directories: `~/.claude/session-env/70875f41.../`
3. Reviewing recent activity in each session

Then for each session:
- If it's the CORRECT session for UUID `70875f41...`: Keep it
- If it's NOT the correct session: Run `agm session associate <session-name>` to assign the correct UUID
- If the session is no longer needed: Archive it

**For session with empty UUID (agm-close):**
Run: `agm session associate agm-close` to link it to the current Claude conversation

## Testing

### Build
```bash
go build -C ~/src/repos/ai-tools/main/agm \
  -o ~/src/repos/ai-tools/main/agm/agm \
  ./cmd/agm
cp ~/src/repos/ai-tools/main/agm/agm \
   ~/src/repos/ai-tools/main/agm/bin/agm
```

### Test Results
```bash
$ agm admin doctor
=== Agent Session Manager Health Check ===

✓ Claude history found
✓ tmux installed: tmux 3.4
✓ Found 15 session manifests

--- Checking for duplicate session directories ---
✓ No duplicate session directories found

--- Checking for duplicate Claude UUIDs ---
⚠ Found 1 Claude UUID(s) shared by multiple sessions
  • UUID 70875f41... is shared by 12 sessions

--- Checking session health ---
✓ All sessions are healthy

⚠ ⚠ Some issues found - see recommendations above
```

## Recommendations

### Short Term
1. User should review the 12 sessions sharing UUID `70875f41...` and reassociate them correctly
2. Associate the `csm-close` session with its Claude conversation
3. Run `agm admin doctor` regularly to detect future issues

### Long Term
1. Implement a SessionStart hook that automatically captures Claude UUIDs when sessions start
2. Add automated migration for old format directories
3. Consider adding a `csm fix` command that can automatically remediate common issues
4. Add validation in `agm session new` to prevent UUID conflicts

## Design Issue: Missing SessionStart Hook

The code in `new.go` line 317 mentions:
```go
fmt.Println("💡 The session will be automatically associated via SessionStart hook")
```

But this hook doesn't exist! This is why sessions created via `agm session new` have empty UUIDs.

**Solution Options:**
1. Implement the SessionStart hook (preferred)
2. Update `agm session new` to capture UUID immediately after starting Claude
3. Remove the misleading message and document that `agm session associate` is required

## Conclusion

The critical UUID bug has been fixed in the code, preventing future occurrences. However, existing sessions require manual remediation. The enhanced `agm admin doctor` command provides clear diagnostics and recommendations for fixing these issues.

---

**Next Steps:**
1. Commit these changes to the AGM repository
2. User to remediate the 12 sessions with shared UUIDs
3. Consider implementing the SessionStart hook for automatic UUID capture
