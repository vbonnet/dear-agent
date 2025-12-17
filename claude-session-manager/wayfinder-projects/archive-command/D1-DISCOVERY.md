# D1: Discovery - Archive Command

**Project**: Add `csm archive` command to Claude Session Manager
**Date**: 2025-12-17
**Phase**: D1 - Discovery & Requirements Gathering

---

## Problem Statement

Users need a way to hide old/inactive sessions from `csm list` without deleting the session files. Currently, sessions accumulate over time and clutter the session list, making it hard to find active sessions.

The manifest schema already supports a `Lifecycle` field that can be set to "archived", and `csm list --all` shows archived sessions, but there's no CLI command to mark sessions as archived.

---

## User Requirements

From user interview:

1. **Command name**: `csm archive <session-name>`
2. **Behavior**: Mark session as archived (set `Lifecycle: "archived"` in manifest)
3. **Safety**: Confirmation prompt before archiving (with `--force` flag to bypass)
4. **Active session protection**: Block archiving active tmux sessions (show error with guidance)
5. **Data preservation**: DO NOT delete files - only update metadata
6. **Discoverability**: Tab auto-completion for session names (like `csm resume`)
7. **Restore documentation**: Help text must clearly explain how to restore archived sessions

---

## Existing Infrastructure Analysis

### What Already Exists

1. **Manifest Schema** (`internal/manifest/manifest.go`):
   - `Lifecycle` field (string) - can be "" or "archived"
   - Constant `LifecycleArchived = "archived"` defined

2. **Filtering Logic** (`cmd/csm/list.go:54-62`):
   - `csm list` filters out archived sessions by default
   - `csm list --all` shows all sessions including archived

3. **Session Resolution** (`internal/session/session.go:13-46`):
   - `ResolveIdentifier(name, dir)` finds sessions by:
     - Session ID (directory name)
     - Tmux session name
     - Manifest Name field

4. **Manifest Updates** (`internal/manifest/write.go`):
   - `Write(path, manifest)` function:
     - Creates automatic backups before overwriting
     - Updates `UpdatedAt` timestamp
     - Validates manifest schema
     - Atomic write with proper permissions (0600)

5. **UI Patterns** (`internal/ui/`):
   - `Confirm(question)` - y/n prompts
   - `PrintSuccess(msg)` - success with checkmark
   - `PrintWarning(msg)` - warning with yellow icon
   - `PrintError(err, cause, solution)` - formatted errors

6. **Tab Completion** (`cmd/csm/resume.go:109-147`):
   - `ValidArgsFunction` for Cobra auto-completion
   - Lists sessions from manifests
   - Gets tmux mapping for session names

---

## Scope

### In Scope

- Create `cmd/csm/archive.go` command file
- Implement archive functionality:
  - Find session by name/ID
  - Validate session exists
  - Check if already archived
  - Prompt for confirmation (with `--force` bypass)
  - Update `Lifecycle` field
  - Save manifest (automatic backup)
- Add tab auto-completion
- Manual testing plan
- Write automated tests (`cmd/csm/archive_test.go`)

### Out of Scope

- **Unarchive/restore command** (deferred to future enhancement - manual restore documented in help text)
- Bulk archive operations
- Archive by age/date
- Archive cleanup/deletion
- Changes to manifest schema (already supports archiving)

**Note on Restore**: Users can restore archived sessions by:
1. `csm list --all` to find session ID
2. Edit `~/sessions/session-X/manifest.yaml`
3. Change `lifecycle: "archived"` to `lifecycle: ""`
4. Session appears in `csm list` again

This will be documented clearly in help text and error messages.

---

## Success Criteria

1. **Functional**:
   - `csm archive <name>` marks session as archived
   - Confirmation prompt shown by default
   - `--force` flag skips confirmation
   - Session hidden from `csm list` after archiving
   - Session visible in `csm list --all`
   - Tab completion suggests session names

2. **Error Handling**:
   - Session not found → helpful error with suggestion to run `csm list`
   - Already archived → warning (idempotent) with restore instructions
   - Active session → error with guidance on stopping tmux session
   - User cancels → exit gracefully (no changes made)

3. **Safety**:
   - Automatic manifest backup created
   - No file deletion
   - Validation before write

4. **Testing**:
   - All manual tests pass
   - Automated test suite passes
   - No regressions in existing commands

5. **Documentation**:
   - Help text explains behavior
   - Examples in `--help` output
   - All wayfinder phases documented

---

## Risks & Mitigations

| Risk | Impact | Mitigation |
|------|--------|------------|
| Accidentally archive active session | Medium | Show session info before confirmation |
| Manifest corruption during write | High | Atomic writes + automatic backups (existing) |
| User confusion about restore | Low | Help text explains restore process |
| Breaking existing commands | High | Automated tests + manual regression testing |

---

## Decisions Made (Post-Review)

1. **Active Session Blocking**: YES - prevent archiving active sessions
   - Show error: "Cannot archive active session 'X'"
   - Guidance: "Stop session with: tmux kill-session -t X"
   - OR: "Use --force to archive anyway" (if --force bypasses active check)

2. **Unarchive Command**: DEFERRED to future enhancement
   - Manual restore documented in help text
   - User decision: Skip unarchive for now
   - Future: Add `csm unarchive <name>` command

3. **--force Flag Scope**: To be defined in D2
   - Does it skip: (a) confirmation only, (b) active session check only, or (c) both?

## Questions for D2 (Design Phase)

1. What should all error messages look like exactly?
2. Should --force bypass active session check, or only confirmation?
3. Should confirmation prompt show session status (active/stopped)?

---

## Next Steps

**Ready for D2**: Architecture & Design phase

**Decision Point**: ✅ PROCEED TO D2 (approved with avg score 9.1/10)
