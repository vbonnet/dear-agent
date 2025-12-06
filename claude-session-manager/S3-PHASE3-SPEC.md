# S3 Phase 3: Session Resumption & Advanced Features

**Status**: 📋 PLANNED - Awaiting Approval
**Prerequisite**: Phase 2 (CLI Commands) - ✅ COMPLETE (8.6/10)

---

## Goal

Implement advanced session management features that enable:
1. Easy session resumption by UUID or tmux name
2. Automatic tmux session creation and attachment
3. Session health checks and recovery
4. Enhanced manifest management

---

## Scope

### In Scope for Phase 3
- ✅ `csm resume` command (resume by UUID, tmux name, or fuzzy match)
- ✅ Automatic tmux session creation if not exists
- ✅ Claude CLI integration (auto-run `claude --resume`)
- ✅ `csm doctor` enhancements (health checks for sessions)
- ✅ Interactive session selection (fuzzy finder)
- ✅ Session cleanup utilities

### Out of Scope (Future Phases)
- Multi-machine sync (Phase 4)
- Session analytics/reports (Phase 4)
- Git integration enhancements (Phase 4)
- Advanced tmux scripting (Phase 4)

---

## Key Features

### 1. Session Resumption (`csm resume`)

**Command**: `csm resume [identifier]`

**Identifier Types**:
- UUID (full or partial): `csm resume c4eb298c`
- Tmux session name: `csm resume claude-1`
- Fuzzy match on project: `csm resume workspace-design`
- Interactive (no identifier): `csm resume` → shows picker

**Workflow**:
```
1. Resolve identifier to Claude UUID
2. Check session health (worktree exists, Claude dirs present)
3. Get/create tmux session name from manifest
4. Attach to tmux or create new session
5. Send `cd <worktree>` to tmux pane
6. Send `claude --resume <uuid>` to tmux pane
7. Update manifest last_activity timestamp
```

**Example Usage**:
```bash
# By UUID prefix
csm resume c4eb298c

# By tmux name
csm resume claude-1

# By project path pattern
csm resume workspace-design

# Interactive picker
csm resume
```

**Acceptance Criteria**:
- ✅ Resume time < 5 seconds for all paths
- ✅ Works with existing tmux sessions
- ✅ Creates new tmux session if needed
- ✅ Handles CWD deleted bug gracefully
- ✅ Updates manifest on successful resume

---

### 2. Enhanced `csm doctor`

**Current State**: Basic validation only

**Enhancements**:
- Check for orphaned Claude directories (`~/.claude/session-env/<uuid>` with no history)
- Detect stale tmux sessions (tmux exists but Claude UUID not in recent history)
- Validate manifest consistency (Claude UUID exists in history.jsonl)
- Suggest cleanup actions for broken sessions
- Check disk space usage by session directories

**Example Output**:
```
Session Health Check
══════════════════════════════════════════════════════════════

✓ Claude CLI: Installed at /usr/local/bin/claude
✓ Tmux: Version 3.2a
✓ Sessions directory: ~/sessions (6 sessions)

⚠ Issues Found:

1. Orphaned Claude directories (3)
   - c25b857b-...: No history entries, last used 45 days ago
   - aa1d5b34-...: No history entries, last used 12 days ago
   - 161b25b4-...: No history entries, last used 8 days ago

   Suggested action: csm cleanup --orphaned

2. Stale tmux sessions (1)
   - claude-vpaste: No recent activity, last message 60 days ago

   Suggested action: csm cleanup --stale-tmux

Disk Usage:
  Session directories: 245 MB
  Claude session-env:  1.2 GB
  Claude file-history: 890 MB
```

**Acceptance Criteria**:
- ✅ Detects all common issues
- ✅ Provides actionable remediation steps
- ✅ Non-destructive (only suggests, doesn't auto-delete)
- ✅ Color-coded output (green=good, yellow=warning, red=error)

---

### 3. Session Cleanup Utilities

**Command**: `csm cleanup [options]`

**Cleanup Types**:
- `--orphaned`: Remove Claude dirs with no history entries
- `--stale-tmux`: Remove tmux sessions with no recent activity
- `--old-sessions`: Archive sessions older than N days
- `--dry-run`: Show what would be deleted without deleting
- `--interactive`: Confirm each deletion

**Safety Features**:
- Require explicit confirmation for destructive operations
- Create backups before deletion
- Support undo within 7 days (trash directory)
- Never delete active tmux sessions

**Example**:
```bash
# Preview cleanup
csm cleanup --orphaned --dry-run

# Interactive cleanup
csm cleanup --orphaned --interactive

# Aggressive cleanup (old sessions)
csm cleanup --old-sessions 90 --dry-run
```

---

### 4. Interactive Session Picker

**Use Case**: When user runs `csm resume` without identifier

**Features**:
- Fuzzy search by project path, UUID, tmux name
- Show session metadata (messages, last activity, duration)
- Highlight active tmux sessions
- Preview session info before resuming
- Keyboard navigation (arrow keys, enter to select)

**UI Mockup**:
```
Select a session to resume:

> [claude-1 ✓] Take a look at github.com/vbonnet/engram and...
               c4eb298c | /home/user | 179 msgs | 6h ago

  [claude-2 ✓] Implement wayfinder template for S3 Phase 2...
               e6121188 | /home/user | 173 msgs | 8h ago

  [-]          Fix column alignment in csm list output
               71c4cd0c | /home/user | 10 msgs  | 1d ago

Search: _

↑↓ Navigate | Enter: Resume | Esc: Cancel | /: Search
```

**Note**: Session descriptions come from the `display` field in `history.jsonl` (first user message), providing context without needing an LLM!

**Implementation Options**:
1. **Native Go** (using `github.com/charmbracelet/bubbletea`)
2. **External tool** (fzf integration)
3. **Simple prompt** (numbered list with input)

**Recommended**: Option 1 (bubbletea) for better UX and no external dependencies

---

## Implementation Tasks

### Task 1: Resume Command Core Logic

**File**: `cmd/csm/resume.go`

**Functions to Implement**:
```go
// resolveSessionIdentifier finds the Claude UUID from various identifier types
func resolveSessionIdentifier(identifier string) (string, error)

// checkSessionHealth validates session can be resumed
func checkSessionHealth(uuid string) (*HealthStatus, error)

// ensureTmuxSession gets existing or creates new tmux session
func ensureTmuxSession(uuid, tmuxName string) error

// resumeSession performs the complete resume workflow
func resumeSession(uuid string) error
```

**Dependencies**:
- Existing tmux package
- Existing manifest package
- Existing discovery package
- New: session health validation logic

---

### Task 2: Enhanced Doctor Command

**File**: `cmd/csm/doctor.go`

**Enhancements**:
```go
// checkOrphanedClaude finds session dirs with no history
func checkOrphanedClaude() []OrphanedSession

// checkStaleTmux finds tmux sessions with old activity
func checkStaleTmux() []StaleTmuxSession

// validateManifests checks manifest consistency
func validateManifests() []ManifestIssue

// calculateDiskUsage computes space used by sessions
func calculateDiskUsage() DiskUsageReport
```

---

### Task 3: Cleanup Command

**File**: `cmd/csm/cleanup.go`

**Functions**:
```go
// cleanupOrphaned removes orphaned Claude directories
func cleanupOrphaned(dryRun, interactive bool) error

// cleanupStaleTmux removes stale tmux sessions
func cleanupStaleTmux(dryRun, interactive bool) error

// archiveOldSessions moves old sessions to archive
func archiveOldSessions(days int, dryRun bool) error

// createBackup creates timestamped backup before deletion
func createBackup(path string) error
```

---

### Task 4: Interactive Picker

**File**: `internal/ui/picker.go`

**Using**: `github.com/charmbracelet/bubbletea`

**Components**:
```go
// SessionPicker presents interactive session selection
type SessionPicker struct {
    sessions []SessionItem
    cursor   int
    selected int
    filter   string
}

// Run displays picker and returns selected UUID
func (p *SessionPicker) Run() (string, error)
```

---

## Testing Strategy

### Unit Tests
- Resume identifier resolution (UUID, tmux name, fuzzy match)
- Health check logic (worktree exists, Claude dirs present)
- Cleanup dry-run mode
- Tmux session creation/attachment

### Integration Tests
- Full resume workflow with fixture data
- Doctor command with known issues
- Cleanup with temporary test directories
- Picker with mock terminal

### Manual Tests
1. Resume existing tmux session
2. Resume with no tmux (create new)
3. Resume with deleted worktree
4. Doctor finds real issues
5. Cleanup with dry-run and interactive mode
6. Interactive picker keyboard navigation

---

## Success Criteria

1. **Performance**: Resume < 5 seconds end-to-end
2. **Reliability**: Handle all known error cases (deleted worktree, missing tmux, corrupted manifest)
3. **UX**: Clear feedback at each step, no silent failures
4. **Safety**: All destructive operations require confirmation
5. **Test Coverage**: >80% for new code
6. **Review Score**: ≥8.5/10 in multi-persona review

---

## Dependencies

### External Packages (New)
- `github.com/charmbracelet/bubbletea` - Interactive TUI
- `github.com/charmbracelet/lipgloss` - Terminal styling (optional)

### Internal Packages (Existing)
- `internal/tmux` - Tmux session management
- `internal/manifest` - Manifest read/write
- `internal/discovery` - Session discovery
- `internal/claude` - History parsing

---

## Risk Assessment

### Medium Risks
1. **Tmux Compatibility**: Different tmux versions may behave differently
   - **Mitigation**: Test on tmux 2.x and 3.x, document minimum version

2. **Race Conditions**: Multiple `csm resume` calls simultaneously
   - **Mitigation**: Add file locking on manifest updates

3. **Terminal Compatibility**: Interactive picker may not work in all terminals
   - **Mitigation**: Fallback to simple numbered list if bubbletea fails

### Low Risks
1. **Performance**: Resume might be slow on network filesystems
   - **Mitigation**: Add timeout warnings, optimize file reads

---

## Implementation Order

1. **Week 1**: Resume command core logic (no interactive picker)
2. **Week 2**: Enhanced doctor command
3. **Week 3**: Cleanup utilities with dry-run
4. **Week 4**: Interactive picker
5. **Week 5**: Integration tests and review

**Total Estimate**: 5 weeks (assuming part-time development)

---

## Open Questions

1. **Picker Dependency**: Is adding bubbletea acceptable, or prefer fzf integration?
2. **Manifest Locking**: Should we implement file locking for concurrent access?
3. **Backup Strategy**: Where to store backups? `.trash/` in sessions dir or separate location?
4. **Cleanup Retention**: How long to keep deleted sessions in trash before permanent deletion?
5. **Multi-Session Resume**: Should `csm resume` support resuming multiple sessions at once?

---

## Future Enhancements (Phase 4+)

- Multi-machine manifest sync (rsync/cloud storage)
- Session templates (create new sessions from templates)
- Git worktree integration (auto-create worktrees)
- Session analytics (time spent, message patterns)
- Backup/restore workflows
- CI/CD integration (resume sessions in scripts)

---

## Deliverables

1. Fully functional `csm resume` command
2. Enhanced `csm doctor` with comprehensive checks
3. Safe `csm cleanup` utilities
4. Interactive session picker (bubbletea or fzf)
5. Comprehensive test suite (>80% coverage)
6. Updated documentation and examples
7. Multi-persona review (≥8.5/10)

---

## Review Checklist (for Phase 3 completion)

- [ ] Resume works with all identifier types
- [ ] Tmux integration is robust (create/attach/send commands)
- [ ] Health checks detect common issues
- [ ] Cleanup operations are safe (confirmation, dry-run, backups)
- [ ] Interactive picker is responsive and intuitive
- [ ] All tests pass
- [ ] Documentation is complete
- [ ] Multi-persona review passed

---

**Phase 3 Status**: Awaiting approval to begin implementation
