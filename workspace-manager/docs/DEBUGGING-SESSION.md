# Debugging Session - December 4, 2025

Post-implementation debugging session to fix critical bugs and improve the session sync tool.

## Context

After completing implementation (Phases 0-5), installed the tool on the workstation and encountered several issues during testing with 5 active tmux+Claude sessions.

## Issues Found & Fixed

### 1. Arithmetic Increment Bug with `set -e`

**Issue**: Scripts exiting after processing only 1 session instead of all 5.

**Root Cause**: `((count++))` returns the OLD value (0), which is falsy, causing `set -e` to exit the script.

```bash
# BROKEN:
count=0
((count++))  # Returns 0, exits with code 1 under set -e

# FIXED:
count=0
((count++)) || true  # Prevent exit on count=0
```

**Files Fixed**:
- `commands/list.sh` (2 instances)
- `commands/sync.sh` (4 instances)
- `commands/cleanup.sh` (4 instances)
- `lib/recovery-utils.sh` (1 instance)

**Impact**: Critical - prevented all list/sync/cleanup operations from working.

### 2. Grep Exit Codes with `set -euo pipefail`

**Issue**: Scripts exiting when grep finds no matches.

**Root Cause**: `grep` returns exit code 1 when no matches found, `set -euo pipefail` causes script to exit.

**Fix**: Added `|| true` to all grep commands searching for manifests:
```bash
grep -l "pattern" files 2>/dev/null | head -1 || true
```

**Files Fixed**:
- `lib/claude-discovery.sh` (multiple grep instances)
- `commands/sync.sh` (grep for manifest matching)

### 3. Session Deduplication

**Issue**: Raw parsing of `history.jsonl` showed 381 "sessions" but only 15 unique UUIDs.

**Discovery**:
- Each line in `history.jsonl` is a user message, not a session
- Multiple messages belong to the same session UUID
- UUIDs remain constant across compaction/continuation

**Analysis Results**:
- 15 unique session UUIDs total
- 6 are active/long-running (>20 messages, >24h duration)
- 9 are short/test sessions (1-5 messages, <1h duration)

**Fix**: Implemented deduplication in sync command (keeps most recent activity per UUID).

### 4. Filtering Heuristics

**Problem**: Original `--days 7` filter breaks after vacation (1.5 weeks away).

**Better Heuristic Discovered**: Active sessions have:
- \>20 messages
- \>24 hours duration
- Survives vacation gaps (duration-based, not time-based)

**Implementation**:
- Added `--active-only` filter
- Made it the default behavior
- Added `get_session_stats()` function to analyze session activity

**Results**:
- Default: 6 active sessions (perfect match for tmux sessions)
- `--all`: 15 sessions (all unique sessions, deduplicated)
- `--days N`: Still available for cleanup scenarios

## Key Learnings

### 1. Claude Session Persistence

**Question**: Do Claude UUIDs change with compaction/continuation?

**Answer**: ✅ **NO** - UUIDs remain constant!

**Evidence**: Session `c86ffd41-cbcc-4bfa-8b1f-4da7c83fc3d2` has:
- 124 messages
- 71.6 hours duration
- Same UUID from Dec 1 to Dec 4

**Implication**: Tmux → Claude UUID mapping is stable and reliable.

### 2. Session-Env Directories

**Discovery**: All `~/.claude/session-env/{uuid}/` directories are empty in this environment.

**Conclusion**:
- Claude Code doesn't populate session-env in this environment
- Feature may be planned but not yet implemented
- Removed `--valid-only` filter (returns 0 sessions)

### 3. Manual Session Mapping

**Challenge**: Auto-detection of tmux → Claude UUID mapping is unreliable.

**Solutions Attempted**:
1. Process inspection (`/proc/{pid}/environ`) - No UUID found
2. Working directory matching - Ambiguous (all sessions in `/home/user`)
3. Recent activity matching - Works but not deterministic

**Best Practice**:
- Use `session sync --active-only` to identify candidates
- Manually verify mapping using last messages from `history.jsonl`
- Created `claude-auto-detect.sh` library for future improvements

## Documentation Updates

### Updated Files

1. **`SESSION-CLI-README.md`** - Updated sync command examples to use `--active-only` default
2. **`DEVELOPMENT.md`** - Added notes about arithmetic increment bug pattern
3. **This file** - Complete debugging session record

### Simplified sync Command

**Before** (5 options):
```
--verbose, --auto, --days <N>, --active-only, --valid-only
```

**After** (3 options):
```
--verbose, --all, --days <N>
```

**Default Behavior**: Shows only active sessions (>20 msgs, >24h duration)

**Rationale**:
- `--valid-only`: Removed (all session-env dirs empty)
- `--auto`: Removed (dangerous, already discouraged)
- `--active-only`: Made default (best signal-to-noise)
- `--all`: Added to override default

## Manifests Created

Successfully created manifests for 5 active sessions:

| Tmux Session | Claude UUID | Messages | Duration | Project |
|--------------|-------------|----------|----------|---------|
| claude-1 | c4eb298c-8c89-4f75 | 97 | 80.2h | Task management |
| claude-2 | e6121188-6c34-4b94 | 101 | 67.6h | Questions plugin |
| claude-3 | c86ffd41-cbcc-4bfa | 124 | 71.6h | Workspace tools (this session) |
| claude-4 | c25b857b-541f-44c6 | 22 | 34.7h | Metacontext consolidation |
| claude-demo | 81caeba9-dc4b-4c56 | 5 | 0.8h | Demo/testing |

Plus one non-tmux session:
- `[REDACTED_EMPLOYER]-mcp-session` (aa1d5b34-42d6-4b7e) - Started outside tmux

## Testing Results

✅ **`session list --claude`**: Shows all 6 sessions correctly
✅ **`session sync`**: Default shows 6 active sessions (filtered from 15 total)
✅ **`session sync --all`**: Shows all 15 unique sessions (deduplicated from 408 messages)
✅ **`session resume claude-1 --verbose`**: Correctly detects empty session-env and offers recovery

## Performance

- **`session sync`** (default): ~90 seconds (calls `get_session_stats()` for filtering)
- **`session sync --all`**: <1 second (no stats needed)
- **`session list`**: <1 second

## Next Steps

1. ✅ Commit all fixes to engram-research
2. ⏳ Test actual resume workflow (needs active Claude session with valid session-env)
3. ⏳ Consider optimizing `get_session_stats()` performance
4. ⏳ Improve auto-detection heuristics in `claude-auto-detect.sh`

## Files Modified

```
M commands/cleanup.sh      # Fixed 4 arithmetic increment bugs
M commands/list.sh         # Fixed 2 arithmetic increment bugs
M commands/sync.sh         # Fixed 4 bugs, added filtering, simplified API
M lib/claude-discovery.sh  # Fixed grep exit codes, added get_session_stats()
M lib/recovery-utils.sh    # Fixed 1 arithmetic increment bug
A lib/claude-auto-detect.sh # New: Experimental auto-detection library
```

## Retrospective

**What Went Well**:
- Systematic debugging approach identified root cause quickly
- Pattern recognition (arithmetic increment bug) allowed fixing across all files
- Session analysis revealed better heuristics (active-only vs days filter)

**What Could Be Improved**:
- Should have tested with `set -euo pipefail` earlier
- Auto-detection needs more work (process inspection insufficient)
- Performance of `get_session_stats()` could be optimized

**Key Insight**: Message count + duration is a much better signal for "active session" than recency alone. This heuristic survives vacation gaps and filters out test sessions perfectly.
