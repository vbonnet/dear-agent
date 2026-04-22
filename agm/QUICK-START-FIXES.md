# Quick Start: AGM Bug Fixes

## What Was Fixed

### 1. UUID Generation Bug (CRITICAL)
**Problem:** `agm admin sync` was assigning the same Claude UUID to multiple sessions, causing 12 sessions to share UUID `70875f41...`

**Fix:** Modified `agm admin sync` to leave Claude UUIDs empty instead of auto-assigning. Users must now explicitly associate sessions using `agm session associate`.

### 2. Duplicate Session Directories
**Problem:** Sessions existed in both old (`claude-1-session`) and new (`session-claude-1`) formats.

**Fix:** Archived old format directories to `~/src/sessions/.archive-old-format/`

### 3. Enhanced Diagnostics
**Added:** `agm admin doctor` now detects:
- Duplicate session directories
- Sessions sharing the same Claude UUID
- Sessions with empty Claude UUIDs

## Installation

```bash
go build -C ~/src/repos/ai-tools/main/agm \
  -o ~/src/repos/ai-tools/main/agm/agm \
  ./cmd/agm
cp ~/src/repos/ai-tools/main/agm/agm \
   ~/src/repos/ai-tools/main/agm/bin/agm
```

## Current Status

Run `agm admin doctor` to see current issues:

```bash
$ agm admin doctor
```

Expected output:
- ✅ No duplicate directories (fixed)
- ⚠️ 12 sessions sharing UUID `70875f41...` (requires manual fix)
- ⚠️ 1 session with empty UUID (requires manual fix)

## Manual Remediation Required

### For the 12 Sessions Sharing UUID `70875f41...`

These sessions ALL point to the same Claude conversation:
- agent-to-agent
- claude-1, claude-2, claude-3, claude-4, claude-5
- claude-2-fix
- csm-resilience
- grouper
- pr-commits
- tool-call
- wayfinder (current session)

**You must decide which session should keep this UUID by:**

1. Check which session you're currently in:
   ```bash
   echo $TMUX_SESSION_NAME  # Or check tmux status bar
   ```

2. If you're in `wayfinder` session and want to keep it linked to this conversation:
   - Keep `wayfinder` with UUID `70875f41...`
   - Reassociate all other sessions

3. For each other session, either:
   - **Archive it** (if no longer needed):
     ```bash
     agm session archive <session-name>
     ```

   - **Reassociate it** (if still active):
     ```bash
     # Attach to the session
     tmux attach -t <session-name>

     # Start Claude or verify it's running
     claude

     # Send a message to create a new conversation entry
     # Then exit Claude (Ctrl+D)

     # Associate the session (auto-detects latest UUID)
     agm session associate <session-name>
     ```

### For Session with Empty UUID (agm-close)

```bash
# Attach to the session
tmux attach -t agm-close

# Verify Claude is running or start it
claude

# Send a message to create history entry
# Exit Claude

# Associate
agm session associate agm-close
```

## Testing Your Fixes

After fixing sessions, verify:

```bash
# Run diagnostics
agm admin doctor

# Expected: No duplicate UUIDs

# List sessions
agm session list

# Try resuming
agm session resume <session-name>
```

## Prevention: Future Sessions

When creating new sessions:

```bash
# Method 1: Create and auto-associate (recommended)
agm session new my-session
# Send first message in Claude
agm session associate my-session

# Method 2: Create without tmux (manual)
claude
# In separate terminal:
agm session associate my-session
```

## Notes

- The bug is FIXED in the code - future `agm admin sync` won't create this problem
- Existing sessions require manual remediation
- Use `agm admin doctor` regularly to catch issues early
- Each Claude conversation should have exactly ONE AGM session pointing to it

## Need Help?

Check the full report: `AGM-BUG-FIX-REPORT.md`
