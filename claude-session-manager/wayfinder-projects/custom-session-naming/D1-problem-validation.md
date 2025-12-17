# D1: Problem Validation - Custom Session Naming Integration for CSM

**Date:** 2025-12-10
**Project:** Custom Session Naming Integration
**Status:** Discovery Phase 1

---

## Executive Summary

This document validates the problem of integrating custom session naming support into CSM (claude-session-manager) to enable user-defined, synchronized session names between Claude Code and tmux sessions.

**Current State:** CSM auto-generates session names using pattern `claude-<directory-basename>` (e.g., `claude-myproject`, `claude-myapp`).

**Desired State:** Users can specify custom session names (e.g., `research-deep-dive`, `feature-auth-refactor`) that are synchronized between Claude and tmux.

**Problem:** While Claude Code has active feature requests for custom session naming (Issues #2112, #6006), there appears to be "unofficial" support mentioned in community discussions that we need to investigate and potentially leverage in CSM.

---

## Problem Statement

### Current Pain Points

1. **Limited Naming Control**
   - CSM auto-generates session names from directory basename
   - Pattern: `claude-<directory>` (e.g., `claude-base`, `claude-myapp`)
   - No way to specify meaningful, context-rich names
   - Multiple projects in same directory require numeric suffixes (`claude-base-2`, `claude-base-3`)

2. **Poor Session Discoverability**
   - Directory-based names don't convey session purpose
   - Example: `claude-base` could be anything (research, implementation, debugging)
   - Hard to identify correct session from `csm list` output
   - Requires checking context/notes after resuming

3. **Manual Name Management**
   - Users can rename tmux sessions manually with `tmux rename-session`
   - This breaks CSM's manifest tracking (NAME ≠ TMUX columns)
   - Results in "orphaned" sessions that csm can't resume

4. **Session Clearing Challenge**
   - `/clear` command in Claude creates **brand new Claude session UUID**
   - CSM manifest still tracks old UUID
   - Tmux session continues with same name
   - CSM loses ability to sync/discover the new Claude session
   - Workaround: Manual `csm sync` after `/clear`, but still creates confusion

### Real-World Example

**Scenario:** User working on authentication refactor in `~/src/repos/myapp`:

```bash
# Current behavior
$ cd ~/src/repos/myapp
$ csm new
Generated tmux session name: claude-myapp
Created tmux session: claude-myapp

# User wants to work on different aspect
$ csm new ~/src/repos/myapp
Generated tmux session name: claude-myapp-2  # Not descriptive!
```

**What user actually wants:**
```bash
$ csm new --name "auth-refactor"
$ csm new --name "performance-profiling"
$ csm new --name "bug-investigation-issue-4532"
```

**After `/clear` in session:**
```
User: /clear
Claude: Starting fresh conversation...

# Problem:
# - Claude UUID changed from abc-123 to xyz-789
# - tmux session still named "auth-refactor"
# - CSM manifest still references abc-123
# - `csm sync` discovers xyz-789 but doesn't know it's the same session
```

---

## Validation of Problem Scope

### Evidence from CSM Source Code

**File:** `~/src/repos/ai-tools/base/claude-session-manager/cmd/csm/new.go:74`

```go
// Generate unique tmux name
tmuxName := generateTmuxName(workDir, existingSessions)
```

**File:** `~/src/repos/ai-tools/base/claude-session-manager/cmd/csm/resume.go:427-468`

```go
func generateTmuxName(project string, existingSessions []string) string {
	base := filepath.Base(project)
	base = sanitizeTmuxName(base)

	if base == "" {
		base = "session"
	}

	name := fmt.Sprintf("claude-%s", base)
	// ... conflict detection and numeric suffix logic
}
```

**Current Behavior:**
- Name derived solely from `filepath.Base(workDir)`
- No user input mechanism for custom names
- Fixed prefix `claude-` hardcoded
- Numeric suffixes for conflicts (`-2`, `--3`, etc.)

### Evidence from Manifest Structure

**File:** `~/src/ws/sessions/claude-1-session/manifest.yaml`

```yaml
schema_version: "2.0"
session_id: claude-1-session
name: claude-1
created_at: 2025-12-04T07:54:16Z
updated_at: 2025-12-10T03:29:28.099884276Z
lifecycle: ""
context:
    project: /home/user
claude: {}
tmux:
    session_name: claude-1
```

**Key Fields:**
- `name`: Session display name (currently matches tmux name)
- `tmux.session_name`: Actual tmux session identifier
- `claude.uuid`: Claude session UUID (not shown when empty)

**Observation:** Manifest already supports independent `name` and `tmux.session_name` fields, suggesting architecture is ready for custom naming.

### Evidence from `csm list` Output

```
NAME         TMUX         STATUS  UPDATED  PROJECT
claude-1     claude-1     active  12h ago  /home/user
claude-2     claude-2     active  19h ago  /home/user
claude-demo  claude-demo  active  16h ago  /home/user
```

**Analysis:**
- NAME and TMUX columns always match in auto-generated sessions
- No evidence of custom-named sessions in practice
- Session purpose unclear from names alone

---

## Research: Claude Code Custom Session Naming

### Community References

**Source:** Reddit post "claude code now unofficially supports custom session names"
**URL:** https://www.reddit.com/r/ClaudeAI/comments/1ozge01/

**Note:** Reddit URL was inaccessible during initial research, but title suggests "unofficial" support exists.

### Feature Request Status

**GitHub Issue #2112:**
- Feature request for `--session-name` flag
- Status: Open (as of research date)
- Proposed syntax: `claude --session-name "my-custom-name"`

**GitHub Issue #6006:**
- Feature request for conversation session renaming
- Includes `/session-name` command proposal
- Status: Open (as of research date)

### Investigation Required

**Questions to Answer in D2:**
1. Does Claude Code have undocumented/experimental support for custom session names?
2. Is there an environment variable (e.g., `CLAUDE_SESSION_NAME`) that works?
3. Are there CLI flags or config options not in official docs?
4. What does "unofficially supports" mean in the Reddit post context?

**Investigation Methods:**
- Examine Claude Code binary/source for session naming hooks
- Test potential environment variables (`CLAUDE_SESSION_NAME`, `CLAUDE_SESSION_ID`, etc.)
- Review Claude Code config files for session-related settings
- Check recent Claude Code releases/changelogs for session features

---

## Impact Analysis

### User Impact

**Who is affected:**
- CSM users managing multiple concurrent Claude sessions
- Users working on long-running projects with descriptive session needs
- Teams sharing session workflows (need standardized naming conventions)

**Frequency:**
- High: Daily for power users with 3+ concurrent sessions
- Medium: Weekly for users with 1-2 sessions, occasional multi-session work
- Low: Rare for single-session users

**Pain Level:**
- **High:** Users relying on `csm list` to choose between 5+ sessions
- **Medium:** Users with 2-3 sessions needing mental mapping (claude-2 = auth work)
- **Low:** Single-session users (current auto-naming sufficient)

### Technical Impact

**Components Affected:**
1. **CSM `new` Command**
   - Add `--name` flag
   - Validate custom name input
   - Pass name to manifest creation

2. **CSM Manifest**
   - Already supports independent `name` field
   - May need to track "user-defined" vs "auto-generated" metadata

3. **CSM `sync` Command**
   - Handle `/clear` scenario: new Claude UUID, same session name
   - Detect UUID changes in existing sessions
   - Update manifest without creating duplicate

4. **CSM `list` Command**
   - Display works without changes (already shows NAME column)

5. **Claude Code Integration**
   - If Claude supports custom names: Pass name via env var or flag
   - If Claude doesn't support: Track name only in CSM/tmux

**Complexity:**
- **Low Complexity:** Adding `--name` flag to CSM (Go CLI framework handles this easily)
- **Medium Complexity:** Handling `/clear` UUID changes gracefully
- **High Complexity (if needed):** Integrating with Claude Code if custom naming not officially supported

---

## Success Criteria

### Must-Have (P0)

1. **Custom Name Support**
   - `csm new --name "my-session"` creates session with custom name
   - Tmux session named `my-session` (not `claude-myproject`)
   - Manifest correctly tracks custom name

2. **Name Validation**
   - Reject invalid tmux session names (spaces, special chars)
   - Prevent conflicts with existing tmux sessions
   - Clear error messages for invalid names

3. **Manifest Integrity**
   - Custom names persist across `csm resume`
   - `csm list` correctly displays custom names
   - `csm sync` doesn't overwrite custom names

4. **Backward Compatibility**
   - `csm new` (no `--name` flag) still works with auto-generated names
   - Existing sessions with auto-generated names unaffected
   - No migration required for existing manifests

### Should-Have (P1)

5. **`/clear` Handling**
   - Detect when Claude UUID changes within a session
   - Update manifest with new UUID without creating new session entry
   - Preserve session name, context, and history metadata

6. **Rename Support**
   - `csm rename <old-name> <new-name>` to update existing session
   - Updates both manifest and tmux session atomically
   - Handles conflicts gracefully

### Nice-to-Have (P2)

7. **Claude Code Integration**
   - If Claude supports custom names: Set Claude's internal name to match
   - Environment variable or flag to pass name to Claude
   - Synchronized naming across CSM, tmux, and Claude UI

8. **Naming Conventions**
   - Suggest naming patterns (project-purpose, feature-bugfix-123)
   - Validation hints for best practices
   - Optional auto-prefixing (e.g., `claude-` prefix configurable)

---

## Out of Scope

### Explicitly Not Included

1. **Session Tags/Categories**
   - Example: Tagging sessions as "research", "implementation", "debugging"
   - Reason: Different feature, could be separate project
   - Note: Manifest already has `context.tags` field for this

2. **Multi-User Session Sharing**
   - Example: Team-wide session registry
   - Reason: CSM is single-user tool, no server component

3. **Auto-Naming Heuristics**
   - Example: Analyze git branch name or commit messages for auto-naming
   - Reason: Adds complexity, user knows best context

4. **Session Templates**
   - Example: Pre-configured session names based on project type
   - Reason: Can be added later if demand exists

---

## Constraints and Assumptions

### Constraints

1. **Tmux Name Restrictions**
   - Must be valid tmux session names (alphanumeric, hyphens, underscores)
   - Must be unique across all tmux sessions (not just Claude sessions)
   - Maximum length ~256 characters (tmux limit)

2. **CSM Architecture**
   - Must maintain manifest v2.0 schema compatibility
   - Must work with existing CSM commands (resume, list, sync)
   - Cannot break existing sessions or manifests

3. **Claude Code Limitations**
   - Claude session UUID is opaque (generated by Anthropic servers)
   - No API to set/change Claude UUID from client side
   - `/clear` creates entirely new session (UUID cannot be preserved)

### Assumptions

1. **User Behavior**
   - Assume users will choose meaningful names (not validated for "meaningfulness")
   - Assume users understand tmux name restrictions (clear error messages help)
   - Assume `/clear` is infrequent operation (not optimizing for rapid clearing)

2. **Claude Code Support**
   - Assume Claude Code may or may not support custom session naming
   - Assume CSM can provide value even if Claude doesn't support custom names
   - Assume feature requests (#2112, #6006) may be implemented in future

3. **Manifest Structure**
   - Assume `name` and `tmux.session_name` can differ if needed
   - Assume `claude.uuid` changes are detectable via `csm sync`
   - Assume manifest locking prevents concurrent modification issues

---

## Risks

### Technical Risks

| Risk | Probability | Impact | Mitigation |
|------|-------------|--------|------------|
| Claude doesn't support custom names | High | Medium | CSM tracks names independently in manifest/tmux |
| `/clear` detection fails | Medium | High | Require explicit `csm sync --force` after `/clear` |
| Name conflicts with system sessions | Low | Medium | Check all tmux sessions, not just CSM-managed |
| Manifest corruption on rename | Low | High | Atomic file updates with backups (already implemented) |

### User Experience Risks

| Risk | Probability | Impact | Mitigation |
|------|-------------|--------|------------|
| Users confused by `/clear` behavior | Medium | Medium | Document clearly, suggest `csm sync` after clearing |
| Invalid name input frustration | Medium | Low | Clear validation errors with examples |
| Existing workflow disruption | Low | Medium | Maintain backward compatibility, make `--name` optional |

---

## Next Steps (D2)

### Investigation Tasks

1. **Claude Code Session Naming Research**
   - Test for undocumented environment variables
   - Examine Claude config files for session options
   - Review recent release notes for session features
   - Attempt to reproduce "unofficial support" from Reddit post

2. **CSM `/clear` Detection**
   - Test `/clear` command in active session
   - Observe UUID change in `~/.config/claude/code/history.jsonl`
   - Design algorithm to detect UUID changes for same tmux session
   - Prototype `csm sync` update logic

3. **Name Validation Requirements**
   - Define allowed characters (tmux + CSM constraints)
   - Define max/min length
   - Define reserved names (if any)
   - Create validation function specification

### Deliverables for D2

- **D2-solution-exploration.md**: Detailed technical investigation results
- Proof-of-concept code for:
  - Custom name flag parsing
  - Name validation function
  - `/clear` detection algorithm
- Decision matrix: Claude integration vs CSM-only implementation

---

## References

### CSM Source Files

- `~/src/repos/ai-tools/base/claude-session-manager/cmd/csm/new.go:74` - Name generation
- `~/src/repos/ai-tools/base/claude-session-manager/cmd/csm/resume.go:427-468` - `generateTmuxName()`
- `~/src/repos/ai-tools/base/claude-session-manager/internal/manifest/manifest.go` - Manifest schema

### Manifest Examples

- `~/src/ws/sessions/claude-1-session/manifest.yaml` - Current manifest structure
- Schema version: 2.0

### Community References

- Reddit: "claude code now unofficially supports custom session names" (URL inaccessible)
- GitHub Issue #2112: `--session-name` flag feature request
- GitHub Issue #6006: Conversation session renaming

---

## Problem Validation: ✅ **CONFIRMED**

**Validation Summary:**
- **Problem exists:** Current auto-naming insufficient for multi-session workflows
- **User need validated:** Power users require descriptive, meaningful session names
- **Technical feasibility:** CSM architecture supports custom naming (manifest has `name` field)
- **Scope defined:** Clear P0/P1/P2 criteria, out-of-scope boundaries set

**Proceed to D2:** Solution exploration and Claude Code integration research.

---

**Created:** 2025-12-10
**Author:** Claude Sonnet 4.5
**Status:** ✅ COMPLETE
