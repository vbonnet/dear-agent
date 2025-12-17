# D2: Design & Architecture - Archive Command

**Project**: Add `csm archive` command to Claude Session Manager
**Date**: 2025-12-17
**Phase**: D2 - Design & Architecture

---

## Design Overview

This phase defines the detailed architecture and design for the `csm archive` command, building on requirements from D1.

**Key Design Decisions:**
1. Command structure (Cobra framework)
2. Error message catalog (exact text for all scenarios)
3. Help text and documentation
4. Active session checking implementation
5. --force flag behavior
6. Confirmation prompt format
7. Test strategy and coverage

---

## 1. Command Structure

### File: `cmd/csm/archive.go`

```go
package main

import (
    "fmt"
    "github.com/spf13/cobra"
    "github.com/vbonnet/ai-tools/claude-session-manager/internal/discovery"
    "github.com/vbonnet/ai-tools/claude-session-manager/internal/manifest"
    "github.com/vbonnet/ai-tools/claude-session-manager/internal/session"
    "github.com/vbonnet/ai-tools/claude-session-manager/internal/ui"
)

var (
    forceArchive bool
)

var archiveCmd = &cobra.Command{
    Use:   "archive <session-name>",
    Short: "Archive a Claude session",
    Long:  `[help text - see section 3]`,
    Args:  cobra.ExactArgs(1),
    RunE:  archiveSession,
    ValidArgsFunction: sessionNameCompletion,
}

func init() {
    archiveCmd.Flags().BoolVarP(&forceArchive, "force", "f", false,
        "Skip confirmation prompt")
    rootCmd.AddCommand(archiveCmd)
}
```

**Estimated size:** ~110 lines total

---

## 2. Implementation Logic Flow

### Main Function: `archiveSession(cmd *cobra.Command, args []string) error`

```
1. Extract session name from args[0]
2. Get sessions directory from cfg.SessionsDir

3. Resolve session identifier
   ├─ Call: session.ResolveIdentifier(sessionName, sessionsDir)
   ├─ Returns: (*manifest.Manifest, string, error)
   └─ Error → Show "session not found" error (EXIT 1)

4. Check if already archived
   ├─ If m.Lifecycle == manifest.LifecycleArchived
   ├─ Show warning message with restore instructions
   └─ EXIT 0 (success - idempotent)

5. Check if session is active (tmux running)
   ├─ Create: tmux := session.NewRealTmux()
   ├─ Call: tmux.HasSession(m.Tmux.SessionName)
   ├─ If active AND !forceArchive
   │   ├─ Show error: "Cannot archive active session"
   │   ├─ Guidance: How to stop tmux session
   │   └─ EXIT 1
   └─ If active AND forceArchive
       └─ Continue (force bypasses active check)

6. Show confirmation prompt (unless --force)
   ├─ If !forceArchive
   ├─ Display session info (name, location, project, status)
   ├─ Call: ui.Confirm("Archive this session?")
   ├─ If declined → Print "Cancelled." → EXIT 0
   └─ If --force → Skip prompt entirely

7. Update manifest
   ├─ Set: m.Lifecycle = manifest.LifecycleArchived
   └─ No other changes

8. Write manifest
   ├─ Call: manifest.Write(manifestPath, m)
   ├─ Automatic: backup creation, UpdatedAt timestamp, validation
   └─ Error → Show write error message (EXIT 1)

9. Show success message
   ├─ Print: "Archived session: <name>"
   ├─ Print: "Manifest: <path>"
   ├─ Print: "Use 'csm list --all' to see archived sessions"
   └─ EXIT 0
```

---

## 3. Help Text Design

### `csm archive --help` Output

```
Archive a Claude session by marking it as archived.

Archived sessions:
  • Hidden from 'csm list' (use --all flag to see them)
  • Files are NOT deleted (only metadata updated)
  • Cannot be resumed until restored
  • Automatic backup created before archiving

This command will:
  1. Find the session by name, tmux name, or session ID
  2. Check if session is currently active in tmux
  3. Prompt for confirmation (unless --force is used)
  4. Update the manifest Lifecycle field to "archived"
  5. Create automatic backup of the manifest

To restore an archived session:
  1. Run: csm list --all
  2. Find session ID
  3. Edit: ~/sessions/session-<ID>/manifest.yaml
  4. Change: lifecycle: "archived" to lifecycle: ""
  5. Save and session will appear in csm list

Examples:
  # Archive with confirmation prompt
  csm archive my-old-session

  # Archive without confirmation (automation/scripts)
  csm archive my-old-session --force

  # List all sessions including archived
  csm list --all

  # Archive by tmux session name
  csm archive claude-5

  # Archive by session ID
  csm archive session-abc123

Usage:
  csm archive <session-name> [flags]

Flags:
  -f, --force   Skip confirmation prompt

Global Flags:
  (standard global flags listed here)
```

---

## 4. Error Message Catalog

### 4.1 Session Not Found

**Trigger:** `session.ResolveIdentifier()` returns error

**Message:**
```
❌ Session 'my-session' not found

Session not found

Try:
  • Check session name with: csm list
  • Available sessions are in: ~/sessions
```

**Exit Code:** 1

---

### 4.2 Already Archived

**Trigger:** `m.Lifecycle == manifest.LifecycleArchived`

**Message:**
```
⚠ Session 'my-session' is already archived

Manifest: ~/sessions/session-abc123/manifest.yaml

To restore this session:
  1. Edit the manifest file above
  2. Change lifecycle: "archived" to lifecycle: ""
  3. Session will appear in csm list
```

**Exit Code:** 0 (success - idempotent operation)

---

### 4.3 Active Session (Cannot Archive)

**Trigger:** `tmux.HasSession()` returns true AND `!forceArchive`

**Message:**
```
❌ Cannot archive active session 'my-session'

The session is currently running in tmux.

To archive this session:
  1. Stop the tmux session first:
     tmux kill-session -t my-session

  2. Then archive:
     csm archive my-session

Or use --force to archive anyway:
  csm archive my-session --force
```

**Exit Code:** 1

---

### 4.4 User Cancelled

**Trigger:** `ui.Confirm()` returns false

**Message:**
```
Cancelled.
```

**Exit Code:** 0 (success - user chose to cancel)

---

### 4.5 Write Failure

**Trigger:** `manifest.Write()` returns error

**Message:**
```
❌ Failed to write manifest: <error details>

Failed to write manifest

Try:
  • Check file permissions
  • Verify disk space
```

**Exit Code:** 1

---

## 5. Confirmation Prompt Format

### When Shown

- Default behavior (no `--force` flag)
- Before making any changes
- After validating session exists and is not already archived

### Format

```
Archive session: my-session
  Location: ~/sessions/session-abc123/manifest.yaml
  Project: /home/user/my-project
  Status: stopped

This will mark the session as archived.
Files will NOT be deleted.

Archive this session? (y/n): _
```

**If user enters 'y':** Continue with archiving
**If user enters 'n':** Print "Cancelled." and exit 0
**If user enters anything else:** Re-prompt

---

## 6. --force Flag Behavior

### Design Decision

**--force skips BOTH:**
1. Confirmation prompt
2. Active session check

**Rationale:**
- Power users/automation need to bypass all interactive checks
- Force means "I know what I'm doing, proceed"
- Active sessions can be archived (just metadata update, no harm)
- Consistent with typical --force semantics (git, rm, etc.)

### Implementation

```go
// Skip confirmation if --force
if !forceArchive {
    // Show confirmation prompt
    confirmed, err := ui.Confirm("Archive this session?")
    if !confirmed {
        fmt.Println("Cancelled.")
        return nil
    }
}

// Check active session (unless --force)
if !forceArchive {
    tmux := session.NewRealTmux()
    if tmux.HasSession(m.Tmux.SessionName) {
        // Show error and exit
    }
}
```

---

## 7. Tab Auto-Completion

### Implementation Pattern

Based on `cmd/csm/resume.go:109-147`:

```go
ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
    // Only complete first argument
    if len(args) != 0 {
        return nil, cobra.ShellCompDirectiveNoFileComp
    }

    // List all manifests
    manifests, err := manifest.List(cfg.SessionsDir)
    if err != nil {
        return []string{}, cobra.ShellCompDirectiveNoFileComp
    }

    // Get tmux mapping
    tmuxMapping, _ := discovery.GetTmuxMapping(cfg.SessionsDir)

    // Build suggestions
    var suggestions []string
    for _, m := range manifests {
        // Include ALL sessions (archived + non-archived)
        // Reason: archiving is idempotent, showing archived is OK

        // Add tmux name
        if tmuxName := tmuxMapping[m.SessionID]; tmuxName != "" {
            suggestions = append(suggestions, tmuxName)
        }

        // Add manifest name (if different)
        if m.Name != "" && m.Name != tmuxMapping[m.SessionID] {
            suggestions = append(suggestions, m.Name)
        }
    }

    return suggestions, cobra.ShellCompDirectiveNoFileComp
},
```

**Note:** Include archived sessions in completion because:
- Archiving is idempotent (warning shown if already archived)
- User may want to re-archive to update timestamp
- Prevents confusion ("why doesn't tab completion show my session?")

---

## 8. Test Strategy

### 8.1 Unit Tests: `cmd/csm/archive_test.go`

**Test Functions:**

1. **TestArchiveSession_Success**
   - Given: Non-archived, stopped session
   - When: Archive with force flag
   - Then: Lifecycle set to "archived", success message shown

2. **TestArchiveSession_AlreadyArchived**
   - Given: Already archived session
   - When: Archive attempt
   - Then: Warning shown, exit 0, no changes

3. **TestArchiveSession_NotFound**
   - Given: Non-existent session name
   - When: Archive attempt
   - Then: Error shown, exit 1

4. **TestArchiveSession_ActiveSession**
   - Given: Active tmux session, no --force
   - When: Archive attempt
   - Then: Error shown with guidance, exit 1

5. **TestArchiveSession_ActiveSessionWithForce**
   - Given: Active tmux session, --force flag
   - When: Archive attempt
   - Then: Archives successfully (force bypasses check)

6. **TestArchiveSession_UserCancels**
   - Given: User responds 'n' to confirmation
   - When: Archive attempt without --force
   - Then: "Cancelled." shown, exit 0, no changes

7. **TestArchiveSession_UserConfirms**
   - Given: User responds 'y' to confirmation
   - When: Archive attempt without --force
   - Then: Archives successfully

8. **TestArchiveSession_WriteFailure**
   - Given: Manifest write fails (mock error)
   - When: Archive attempt
   - Then: Error shown, exit 1

9. **TestValidArgsFunction_IncludesArchivedSessions**
   - Given: Mix of archived and non-archived sessions
   - When: Tab completion called
   - Then: All sessions returned (archived + non-archived)

10. **TestValidArgsFunction_IncludesTmuxAndManifestNames**
    - Given: Sessions with tmux names and manifest names
    - When: Tab completion called
    - Then: Both tmux names and manifest names returned

**Mock Requirements:**
- Mock tmux interface (HasSession returns configurable value)
- Mock ui.Confirm (returns configurable bool)
- Mock manifest.Write (can return error)
- Use existing test patterns from other csm commands

---

### 8.2 Integration Tests

**Manual Test Plan:**

```bash
# Setup
csm new test-archive-1
csm new test-archive-2

# Test 1: Normal archive with confirmation
echo "y" | csm archive test-archive-1
csm list | grep test-archive-1  # Should NOT appear
csm list --all | grep test-archive-1  # SHOULD appear as archived

# Test 2: Already archived
csm archive test-archive-1  # Should show warning

# Test 3: User cancellation
echo "n" | csm archive test-archive-2  # Should show "Cancelled."
csm list | grep test-archive-2  # SHOULD still appear (not archived)

# Test 4: Force flag
csm archive test-archive-2 --force  # No prompt, immediate archive

# Test 5: Active session blocking
csm resume test-archive-1  # Start tmux session
csm archive test-archive-1  # Should error (active session)
csm archive test-archive-1 --force  # Should work (force bypasses)

# Test 6: Tab completion
csm archive <TAB>  # Should show all session names

# Test 7: Not found
csm archive nonexistent  # Should error

# Test 8: Backup verification
ls ~/sessions/session-test-archive-1/manifest.yaml.*  # Should see backup

# Cleanup
tmux kill-session -t test-archive-1 2>/dev/null
rm -rf ~/sessions/session-test-archive-*
```

---

### 8.3 Regression Tests

**Verify no breaking changes:**

```bash
# Run full test suite
cd ~/src/repos/ai-tools/base/claude-session-manager
go test ./...

# Manual smoke tests
csm list              # Should work
csm list --all        # Should show archived sessions
csm new test          # Should create session
csm resume test       # Should resume session
csm associate test    # Should associate session

# Cleanup
rm -rf ~/sessions/session-test
```

---

## 9. Edge Cases & Error Scenarios

| Scenario | Expected Behavior | Exit Code |
|----------|-------------------|-----------|
| Empty session name | Cobra validation error | 1 |
| Session name with spaces | ResolveIdentifier handles | varies |
| Very long session name (>255) | ResolveIdentifier handles | varies |
| Special characters (!@#$%) | ResolveIdentifier handles | varies |
| Manifest locked (lock file) | Wait or timeout (existing behavior) | 1 |
| Concurrent archive attempts | Lock prevents race condition | 1 |
| Corrupted manifest | Read/validation fails | 1 |
| Permission denied (read) | ResolveIdentifier error | 1 |
| Permission denied (write) | manifest.Write error | 1 |
| Disk full | manifest.Write error | 1 |
| Session directory deleted | Not found error | 1 |

**All edge cases handled by existing infrastructure** - no special code needed.

---

## 10. Security Considerations

### Input Validation

**Session name sanitization:** Not required
- ResolveIdentifier does lookup against existing manifests (not file path construction)
- No path traversal risk
- No command injection risk (not passed to shell)

### Path Handling

- Use `filepath.Join()` (existing pattern)
- Never construct paths from user input
- All paths come from ResolveIdentifier (validated)

### File Permissions

- Manifest backups: 0600 (user-only) - existing Write() behavior
- No changes to file permissions needed

### Information Disclosure

- Error messages show paths: `~/sessions/session-X/manifest.yaml`
- Using `~` prefix (not `/home/user`) for cleaner output
- Acceptable: CSM is single-user tool

---

## 11. Performance Analysis

### Complexity

- **Session resolution:** O(n) where n = number of sessions
- **Tmux check:** O(1) - single `tmux has-session` call
- **Manifest update:** O(1) - single file write
- **Total:** O(n) - acceptable for typical session counts (<1000)

### Optimization

**No optimization needed:**
- Archive is infrequent operation (not hot path)
- O(n) session scan is fast even with 1000s of sessions
- Existing ResolveIdentifier is efficient

---

## 12. Design Review Checklist

- ✅ Command structure follows Cobra patterns
- ✅ Error messages are helpful and actionable
- ✅ Help text is comprehensive
- ✅ Active session checking prevents surprises
- ✅ --force flag has clear semantics
- ✅ Confirmation prompt shows relevant info
- ✅ Tab completion works like other commands
- ✅ Test coverage is comprehensive
- ✅ Edge cases identified and handled
- ✅ Security considerations addressed
- ✅ Performance is acceptable
- ✅ Code size estimate is reasonable (~110 lines)

---

## Next Steps

**Ready for D3**: Implementation phase

**Implementation Checklist:**
1. Create `cmd/csm/archive.go` with command structure
2. Implement `archiveSession()` function
3. Add tab completion `ValidArgsFunction`
4. Create `cmd/csm/archive_test.go` with 10 unit tests
5. Run tests: `go test ./cmd/csm/`
6. Build: `make -C ~/src/repos/ai-tools/base/claude-session-manager`
7. Manual testing (follow 8.2 integration test plan)
8. Regression testing (go test ./...)
9. Install: `make install`

**Decision Point**: Proceed to D3? YES / NO / NEEDS_REVISION
