# Session Persistence Wayfinder Project

**Project Name**: CSM Session Persistence & Context Tracking
**Created**: December 6, 2025
**Status**: ✅ D4 COMPLETE - Requirements Approved (9.3/10)

---

## Problem Statement

Claude AI sessions are ephemeral and don't survive computer reboots. Users face several challenges:

1. **Lost Session State**: After reboot, Claude sessions disappear
2. **Lost Context**: Can't remember which session was working on which task
3. **Lost Progress**: No reference to previous conversations in sessions
4. **No Recovery Strategy**: Manual recreation required for all sessions

**Current State**:
- `csm resume` works great when tmux session exists
- Manifests track metadata (UUID, tmux name, worktree)
- But sessions don't survive reboots

**Desired State**:
- Sessions persist across reboots (or can be reconstructed)
- Users can track what each session was working on
- Session logs backed up for reference
- Resume workflow context after reboot

---

## User Stories

### Primary Use Cases

1. **As a developer**, after reboot I want to:
   - See all my previous sessions
   - Know which session was working on which task/feature
   - Resume sessions where I left off
   - Access conversation history for reference

2. **As a project manager**, I want to:
   - Track how Claude sessions contributed to different features
   - Reference past conversations for documentation
   - Share session logs with team members

3. **As a researcher**, I want to:
   - Keep session logs as research notes
   - Organize sessions by topic/experiment
   - Search across all session conversations

---

## Key Questions to Answer (D1-D4)

### Architecture Questions

1. **Session Reconstruction**:
   - Can we re-instantiate Claude sessions after reboot?
   - What data do we need to preserve?
   - How does Claude's `--resume` flag work across reboots?

2. **Log Backup Strategy**:
   - Where should session logs be stored?
   - What format? (copy history.jsonl? separate per-session files?)
   - How to handle log rotation/cleanup?

3. **Context Tracking**:
   - How do users describe what a session is for?
   - Labels? Tags? Free-form notes?
   - Automatic context extraction vs manual annotation?

4. **Integration with Workspace Architecture**:
   - Sessions should be configurable (default `~/sessions/`, optional `$DEVLOG_ROOT/sessions/`)
   - How to make CSM independent but workspace-friendly?
   - Should session logs go in workspace or separate?

### Implementation Questions

5. **Recovery Workflow**:
   - After reboot, what does `csm list` show?
   - Can sessions be resumed automatically?
   - What if worktree was deleted?

6. **Backup Timing**:
   - Real-time backup (after each message)?
   - On-demand backup (`csm backup`)?
   - Background sync daemon?

7. **Storage Location**:
   - Sessions directory: `~/sessions/` (default) or `$DEVLOG_ROOT/sessions/` (workspace)
   - Logs subdirectory: `~/sessions/<session-id>/logs/`?
   - Centralized backup: `~/.claude/backups/`?

---

## Initial Thoughts

### Option 1: Session Log Backup
- Copy relevant portions of `~/.claude/history.jsonl` to session-specific files
- Store in `~/sessions/<session-id>/logs/`
- On reboot, manifests show sessions but mark as "stopped"
- User can create new session and reference old logs

**Pros**: Simple, doesn't rely on Claude internals
**Cons**: Doesn't preserve running sessions

### Option 2: Full Session Reconstruction
- Backup everything needed to resume: history.jsonl, session-env, file-history
- On reboot, offer to recreate sessions
- Use Claude's `--resume` to restore state

**Pros**: Sessions truly persist
**Cons**: Complex, depends on Claude's resume behavior

### Option 3: Hybrid Approach
- Backup logs for reference (Option 1)
- Track session purpose/context in manifest
- After reboot, show "archived" sessions with logs
- User decides whether to create new session or just reference old logs

**Pros**: Flexible, doesn't force recreation
**Cons**: UX might be confusing (what's the difference between stopped and archived?)

---

## Success Criteria

After this project, users should be able to:

1. ✅ **After Reboot**: See all previous sessions with their purpose/context
2. ✅ **Log Access**: Read conversation history from past sessions
3. ✅ **Context Tracking**: Know which session was working on which feature/task
4. ✅ **Recovery**: Either resume sessions or start fresh with context
5. ✅ **Workspace Integration**: CSM works standalone but integrates with workspace architecture

**Performance Targets**:
- Log backup: < 100ms per message (if real-time)
- Session list: < 1s (even with 100+ archived sessions)
- Log search: < 2s across all sessions

**UX Targets**:
- Zero-config for basic use (backup enabled by default)
- One command to see session context (`csm list --context` or similar)
- Resuming after reboot feels natural (not confusing)

---

## Out of Scope (For Now)

- Multi-machine sync (different problem)
- Session analytics/reports (Phase 4+)
- Search across all session logs (Phase 4+)
- Sharing sessions with other users (Phase 4+)

---

## Risks & Unknowns

1. **Claude Resume Behavior**: Does `--resume` work after reboot if session-env exists?
2. **Log Size**: history.jsonl could be huge - do we backup everything?
3. **User Expectations**: Will users expect sessions to "just work" after reboot?
4. **Storage Growth**: Session logs could accumulate - need cleanup strategy

---

## Next Steps (Wayfinder D1)

1. Research how Claude's `--resume` works
2. Experiment: Does session resume after reboot if files exist?
3. Define architecture (backup vs reconstruction vs hybrid)
4. Identify critical technical risks
5. Create detailed requirements

---

## References

- **Phase 3 Implementation**: `../S3-PHASE3-IMPLEMENTATION.md`
- **Phase 3 Review**: `../S3-PHASE3-REVIEW.md`
- **Current Manifest Schema**: `../internal/manifest/manifest.go`
- **Claude History Format**: `~/.claude/history.jsonl`

---

**End of Project Context**
