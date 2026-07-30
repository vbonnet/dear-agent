package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/delegation"
	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	"github.com/vbonnet/dear-agent/agm/internal/git"
	"github.com/vbonnet/dear-agent/agm/internal/interrupt"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/messages"
	"github.com/vbonnet/dear-agent/agm/internal/monitoring"
	"github.com/vbonnet/dear-agent/agm/internal/ops"
	"github.com/vbonnet/dear-agent/agm/internal/session"
	"github.com/vbonnet/dear-agent/agm/internal/tmux"
	"github.com/vbonnet/dear-agent/agm/internal/ui"
)

// validNamePattern allows alphanumeric, dash, underscore
var validNamePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]*$`)

var renameCmd = &cobra.Command{
	Use:   "rename <old-name> <new-name>",
	Short: "Rename an AGM session and update all references",
	Long: `Rename an AGM session, updating all state that references the session name:

  1. Tmux session name
  2. Manifest fields (Name, Tmux.SessionName)
  3. Dolt database record
  4. Heartbeat files (~/.agm/heartbeats/)
  5. Interrupt flag files (~/.agm/interrupts/)
  6. Message queue entries (SQLite from/to fields)
  7. Pending message directories (~/.agm/pending/)
  8. Delegation records (~/.agm/delegations/)
  9. Monitor references in other sessions

Examples:
  agm session rename my-old-session my-new-session
  agm session rename monorepo-investigation research-monorepo`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		oldName := args[0]
		newName := args[1]
		sessionsDir := getSessionsDir()

		// Validate new name format
		if !validNamePattern.MatchString(newName) {
			ui.PrintError(fmt.Errorf("invalid name: %q", newName),
				"New session name must be alphanumeric with dashes and underscores",
				"  • Must start with an alphanumeric character\n"+
					"  • Allowed characters: a-z, A-Z, 0-9, dash (-), underscore (_)")
			return fmt.Errorf("invalid session name: %s", newName)
		}

		// Get Dolt adapter
		adapter, err := getStorage()
		if err != nil {
			ui.PrintError(err, "Failed to connect to Dolt storage", "")
			return err
		}
		defer func() { _ = adapter.Close() }()

		// Resolve old session
		m, manifestPath, err := session.ResolveIdentifier(oldName, sessionsDir, adapter)
		if err != nil {
			ui.PrintError(err, "Session not found",
				fmt.Sprintf("  • Check session name: agm session list\n"+
					"  • Session '%s' does not exist or is archived", oldName))
			return err
		}
		stableSessionID := m.SessionID
		var updated []string
		if err := runSessionRenameTransactionWithLock(stableSessionID, ops.WithSessionLock, func() (retErr error) {
			// Resolution by user-visible name happens before the lock only to find
			// the stable identity. Reload that identity while holding the same lock
			// used by resume so every tmux and storage decision is current.
			m, manifestPath, err = session.ResolveIdentifier(stableSessionID, sessionsDir, adapter)
			if err != nil {
				return fmt.Errorf("reload session %s for rename: %w", stableSessionID, err)
			}
			oldName = m.Name

			// Check new name doesn't already exist
			existingByName, _ := adapter.GetSessionByName(newName)
			if existingByName != nil && existingByName.SessionID != m.SessionID {
				ui.PrintError(fmt.Errorf("session '%s' already exists", newName),
					"Cannot rename: target name is already in use",
					fmt.Sprintf("  • Existing session ID: %s", existingByName.SessionID))
				return fmt.Errorf("session name already exists: %s", newName)
			}

			// Also check by tmux name resolution
			existingByTmux, _, _ := session.ResolveIdentifier(newName, sessionsDir, adapter)
			if existingByTmux != nil && existingByTmux.SessionID != m.SessionID {
				ui.PrintError(fmt.Errorf("session '%s' already exists", newName),
					"Cannot rename: target name resolves to a different session",
					fmt.Sprintf("  • Existing session ID: %s", existingByTmux.SessionID))
				return fmt.Errorf("session name already exists: %s", newName)
			}

			if newName != oldName {
				if err := adapter.ReserveSessionName(stableSessionID, newName); err != nil {
					var uncertain *dolt.SessionNameReservationCommitUncertainError
					if errors.As(err, &uncertain) {
						err = errors.Join(err, adapter.ReleaseSessionNameReservation(stableSessionID))
					}
					ui.PrintError(err,
						"Cannot rename: target name could not be reserved",
						"  • Another create or rename may already own this name")
					return err
				}
				defer func() {
					if err := adapter.ReleaseSessionNameReservation(stableSessionID); err != nil {
						retErr = errors.Join(retErr, fmt.Errorf("release rename target-name reservation: %w", err))
					}
				}()
			}

			fmt.Printf("Renaming session: %s → %s\n", oldName, newName)

			// 1. Rename tmux session
			tmuxRename, err := moveTmuxSessionForRename(cmd.Context(), m.Tmux.SessionName, newName)
			if tmuxRename.Identity.Valid() {
				defer func() {
					cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(cmd.Context()), 10*time.Second)
					defer cancel()
					if cleanupErr := tmux.ClearSessionRenameIdentityContext(cleanupCtx, tmuxRename.Identity); cleanupErr != nil {
						ui.PrintWarning(fmt.Sprintf("Failed to clear tmux rename identity marker: %v", cleanupErr))
					}
				}()
			}
			if err != nil && tmuxRename.SourceAbsent {
				fmt.Printf("  ⚠  tmux rename skipped: %v\n", err)
			} else if err != nil {
				ui.PrintError(err, "Failed to rename tmux session", "")
				return err
			} else if tmuxRename.Moved {
				updated = append(updated, "tmux session")
			}

			// 2-3. Atomically persist the exact identity snapshot. If a concurrent
			// writer advanced it after resolution, restore the live tmux name and
			// report the conflict rather than storing a dead identity.
			restoreTmux := func(ctx context.Context, currentName, previousName string) error {
				return restoreTmuxSessionNameAfterRename(ctx, tmuxRename.Identity, currentName, previousName)
			}
			if err := persistRenamedSessionIdentity(cmd.Context(), adapter, m, newName, tmuxRename.Moved, restoreTmux); err != nil {
				ui.PrintError(err, "Failed to update Dolt record", "")
				return err
			}
			updated = append(updated, "manifest")
			updated = append(updated, "dolt record")

			// 4. Rename heartbeat file
			if renamed, err := renameHeartbeatFile(oldName, newName); err != nil {
				fmt.Printf("  ⚠  heartbeat rename failed: %v\n", err)
			} else if renamed {
				updated = append(updated, "heartbeat file")
			}

			// 5. Rename interrupt flag file
			if renamed, err := renameInterruptFile(oldName, newName); err != nil {
				fmt.Printf("  ⚠  interrupt rename failed: %v\n", err)
			} else if renamed {
				updated = append(updated, "interrupt file")
			}

			// 6. Update message queue entries (SQLite)
			if count, err := renameMessageQueueEntries(oldName, newName); err != nil {
				fmt.Printf("  ⚠  message queue update failed: %v\n", err)
			} else if count > 0 {
				updated = append(updated, fmt.Sprintf("message queue (%d entries)", count))
			}

			// 7. Rename pending message directory
			if renamed, err := renamePendingDir(oldName, newName); err != nil {
				fmt.Printf("  ⚠  pending dir rename failed: %v\n", err)
			} else if renamed {
				updated = append(updated, "pending messages dir")
			}

			// 8. Rename delegation file
			if renamed, err := renameDelegationFile(oldName, newName); err != nil {
				fmt.Printf("  ⚠  delegation rename failed: %v\n", err)
			} else if renamed {
				updated = append(updated, "delegation file")
			}

			// 9. Update monitor references in other sessions
			if count, err := updateMonitorReferences(oldName, newName, adapter); err != nil {
				fmt.Printf("  ⚠  monitor references update failed: %v\n", err)
			} else if count > 0 {
				updated = append(updated, fmt.Sprintf("monitor refs (%d sessions)", count))
			}

			// 10. Update ready-file signal
			if renamed, err := renameReadyFile(oldName, newName); err != nil {
				fmt.Printf("  ⚠  ready-file rename failed: %v\n", err)
			} else if renamed {
				updated = append(updated, "ready-file")
			}

			// Auto-commit manifest change
			_ = git.CommitManifest(manifestPath, "rename", newName)

			// Print summary
			fmt.Printf("\n✓ Session renamed successfully: %s → %s\n", oldName, newName)
			fmt.Printf("  Updated: %s\n", formatList(updated))
			fmt.Printf("\n  To send messages: agm send msg %s --prompt \"...\"\n", newName)
			fmt.Printf("  To view session:  agm session get %s\n", newName)

			return nil
		}); err != nil {
			return err
		}
		return nil
	},
}

func init() {
	sessionCmd.AddCommand(renameCmd)
}

type sessionIdentityRenamer interface {
	RenameSessionIdentity(ctx context.Context, sessionID, previousName, previousTmuxName, observedRevision, newName string) (dolt.RenameSessionIdentityResult, error)
}

func runSessionRenameTransactionWithLock(sessionID string, withLock func(string, func() error) error, transaction func() error) error {
	if sessionID == "" {
		return fmt.Errorf("rename session ID is required")
	}
	if withLock == nil {
		return fmt.Errorf("rename session lock dependency is missing")
	}
	if transaction == nil {
		return fmt.Errorf("rename session transaction dependency is missing")
	}
	return withLock(sessionID, transaction)
}

func persistRenamedSessionIdentity(ctx context.Context, store sessionIdentityRenamer, m *manifest.Manifest, newName string, tmuxRenamed bool, renameTmux func(context.Context, string, string) error) error {
	previousName := m.Name
	previousTmuxName := m.Tmux.SessionName
	result, err := store.RenameSessionIdentity(ctx, m.SessionID, previousName, previousTmuxName, m.Tmux.SessionRevision, newName)
	if err != nil {
		if result.StorageCommitted {
			m.Name = newName
			m.Tmux.SessionName = newName
			m.UpdatedAt = time.Now()
			return err
		}
		if tmuxRenamed && result.TmuxRollbackSafe {
			rollbackCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
			defer cancel()
			if rollbackErr := renameTmux(rollbackCtx, newName, previousTmuxName); rollbackErr != nil {
				return errors.Join(err, fmt.Errorf("restore tmux session name after storage conflict: %w", rollbackErr))
			}
		} else if tmuxRenamed {
			return errors.Join(err, fmt.Errorf("tmux rollback skipped because storage identity could not be proven unchanged"))
		}
		return err
	}
	m.Name = newName
	m.Tmux.SessionName = newName
	m.UpdatedAt = time.Now()
	return nil
}

type tmuxRenameOutcome struct {
	Moved        bool
	SourceAbsent bool
	Identity     tmux.RenameSessionIdentity
}

func moveTmuxSessionForRename(ctx context.Context, oldName, newName string) (tmuxRenameOutcome, error) {
	normalizedOld := tmux.NormalizeTmuxSessionName(oldName)
	normalizedNew := tmux.NormalizeTmuxSessionName(newName)
	oldExists, err := tmux.HasSessionStrictContext(ctx, normalizedOld)
	if err != nil {
		return tmuxRenameOutcome{}, fmt.Errorf("check source tmux session: %w", err)
	}
	if !oldExists {
		return tmuxRenameOutcome{SourceAbsent: true}, fmt.Errorf("tmux session %q not found", normalizedOld)
	}
	newExists, err := tmux.HasSessionStrictContext(ctx, normalizedNew)
	if err != nil {
		return tmuxRenameOutcome{}, fmt.Errorf("check target tmux session: %w", err)
	}
	if newExists && normalizedOld != normalizedNew {
		return tmuxRenameOutcome{}, fmt.Errorf("tmux session %q already exists", normalizedNew)
	}
	identity, err := tmux.ClaimSessionRenameIdentityContext(ctx, normalizedOld)
	if err != nil {
		return tmuxRenameOutcome{}, fmt.Errorf("claim source tmux session identity: %w", err)
	}
	outcome := tmuxRenameOutcome{Identity: identity}
	renameErr := tmux.RenameClaimedSessionContext(ctx, identity, normalizedNew)

	inspectCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	currentName, owned, inspectErr := tmux.InspectSessionRenameIdentityContext(inspectCtx, identity)
	if inspectErr != nil {
		return outcome, errors.Join(renameErr, inspectErr)
	}
	return classifyTmuxRenameResult(outcome, normalizedOld, normalizedNew, currentName, owned, renameErr)
}

func classifyTmuxRenameResult(outcome tmuxRenameOutcome, oldName, newName, currentName string, owned bool, renameErr error) (tmuxRenameOutcome, error) {
	if owned && currentName == newName {
		outcome.Moved = true
		return outcome, nil
	}
	if owned && currentName == oldName && renameErr != nil {
		return outcome, renameErr
	}
	if renameErr == nil {
		renameErr = fmt.Errorf("tmux rename reported success without preserving its claimed identity")
	}
	return outcome, errors.Join(renameErr, fmt.Errorf("tmux rename outcome is ambiguous: identity owned=%v current name=%q source=%q target=%q", owned, currentName, oldName, newName))
}

func restoreTmuxSessionNameAfterRename(ctx context.Context, identity tmux.RenameSessionIdentity, currentName, previousName string) error {
	normalizedPrevious := tmux.NormalizeTmuxSessionName(previousName)
	renameErr := tmux.RenameClaimedSessionContext(ctx, identity, normalizedPrevious)
	actualName, owned, inspectErr := tmux.InspectSessionRenameIdentityContext(ctx, identity)
	if inspectErr != nil {
		return errors.Join(renameErr, inspectErr)
	}
	if owned && actualName == normalizedPrevious {
		return nil
	}
	if renameErr == nil {
		renameErr = fmt.Errorf("tmux rename rollback reported success without preserving its claimed identity")
	}
	return errors.Join(renameErr, fmt.Errorf("tmux rename rollback is unconfirmed: identity owned=%v current name=%q previous=%q renamed=%q", owned, actualName, normalizedPrevious, currentName))
}

// renameHeartbeatFile renames the heartbeat file from old to new session name.
func renameHeartbeatFile(oldName, newName string) (bool, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return false, err
	}
	dir := filepath.Join(homeDir, ".agm", "heartbeats")

	oldPath := filepath.Join(dir, "loop-"+oldName+".json")

	if _, err := os.Stat(oldPath); os.IsNotExist(err) {
		return false, nil // No heartbeat file to rename
	}

	// Read, update session name inside, write to new path
	hb, err := monitoring.ReadHeartbeat(dir, oldName)
	if err != nil {
		return false, err
	}
	hb.Session = newName

	// Write to new path via the writer
	writer, err := monitoring.NewHeartbeatWriter(dir)
	if err != nil {
		return false, err
	}
	if err := writer.Write(newName, hb.IntervalSecs, hb.CycleNumber, hb.OK); err != nil {
		return false, err
	}

	// Remove old file
	_ = os.Remove(oldPath)
	return true, nil
}

// renameInterruptFile renames the interrupt flag file from old to new session name.
func renameInterruptFile(oldName, newName string) (bool, error) {
	dir := interrupt.DefaultDir()
	oldPath := interrupt.FlagPath(dir, oldName)
	newPath := interrupt.FlagPath(dir, newName)

	if _, err := os.Stat(oldPath); os.IsNotExist(err) {
		return false, nil // No interrupt file to rename
	}

	if err := os.Rename(oldPath, newPath); err != nil {
		return false, err
	}
	return true, nil
}

// renameMessageQueueEntries updates from_session and to_session in the SQLite message queue.
func renameMessageQueueEntries(oldName, newName string) (int, error) {
	queue, err := messages.NewMessageQueue()
	if err != nil {
		return 0, err
	}
	defer func() { _ = queue.Close() }()

	count, err := queue.RenameSession(oldName, newName)
	if err != nil {
		return 0, err
	}
	return count, nil
}

// renamePendingDir renames the pending messages directory.
func renamePendingDir(oldName, newName string) (bool, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return false, err
	}

	oldDir := filepath.Join(homeDir, ".agm", "pending", oldName)
	newDir := filepath.Join(homeDir, ".agm", "pending", newName)

	if _, err := os.Stat(oldDir); os.IsNotExist(err) {
		return false, nil
	}

	if err := os.Rename(oldDir, newDir); err != nil {
		return false, err
	}
	return true, nil
}

// renameDelegationFile renames the delegation JSONL file and updates internal references.
func renameDelegationFile(oldName, newName string) (bool, error) {
	delegDir, err := delegation.DefaultDir()
	if err != nil {
		return false, err
	}

	oldPath := filepath.Join(delegDir, oldName+".jsonl")
	newPath := filepath.Join(delegDir, newName+".jsonl")

	if _, err := os.Stat(oldPath); os.IsNotExist(err) {
		return false, nil
	}

	if err := os.Rename(oldPath, newPath); err != nil {
		return false, err
	}
	return true, nil
}

// updateMonitorReferences updates other sessions' Monitors arrays that reference the old name.
func updateMonitorReferences(oldName, newName string, adapter *dolt.Adapter) (int, error) {
	allSessions, err := adapter.ListSessions(&dolt.SessionFilter{})
	if err != nil {
		return 0, err
	}

	count := 0
	for _, s := range allSessions {
		modified := false
		for i, mon := range s.Monitors {
			if mon == oldName {
				s.Monitors[i] = newName
				modified = true
			}
		}
		if modified {
			if err := adapter.UpdateSession(s); err != nil {
				return count, fmt.Errorf("failed to update monitors for session %s: %w", s.SessionID, err)
			}
			count++
		}
	}
	return count, nil
}

// renameReadyFile renames the ready-file signal.
func renameReadyFile(oldName, newName string) (bool, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return false, err
	}

	oldPath := filepath.Join(homeDir, ".agm", "ready-"+oldName)
	newPath := filepath.Join(homeDir, ".agm", "ready-"+newName)

	if _, err := os.Stat(oldPath); os.IsNotExist(err) {
		return false, nil
	}

	if err := os.Rename(oldPath, newPath); err != nil {
		return false, err
	}
	return true, nil
}

// formatList joins a string slice with commas.
func formatList(items []string) string {
	if len(items) == 0 {
		return "none"
	}
	result := items[0]
	for i := 1; i < len(items); i++ {
		result += ", " + items[i]
	}
	return result
}
