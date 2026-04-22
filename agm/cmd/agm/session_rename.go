package main

import (
	"context"
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
	"github.com/vbonnet/dear-agent/agm/internal/messages"
	"github.com/vbonnet/dear-agent/agm/internal/monitoring"
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
		defer adapter.Close()

		// Resolve old session
		m, manifestPath, err := session.ResolveIdentifier(oldName, sessionsDir, adapter)
		if err != nil {
			ui.PrintError(err, "Session not found",
				fmt.Sprintf("  • Check session name: agm session list\n"+
					"  • Session '%s' does not exist or is archived", oldName))
			return err
		}

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

		fmt.Printf("Renaming session: %s → %s\n", oldName, newName)

		// Track what we updated for the summary
		var updated []string

		// 1. Rename tmux session
		if err := renameTmuxSession(oldName, newName); err != nil {
			fmt.Printf("  ⚠  tmux rename skipped: %v\n", err)
		} else {
			updated = append(updated, "tmux session")
		}

		// 2. Update manifest fields
		m.Name = newName
		m.Tmux.SessionName = newName
		m.UpdatedAt = time.Now()
		updated = append(updated, "manifest")

		// 3. Update Dolt database record
		if err := adapter.UpdateSession(m); err != nil {
			ui.PrintError(err, "Failed to update Dolt record", "")
			return err
		}
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
	},
}

func init() {
	sessionCmd.AddCommand(renameCmd)
}

// renameTmuxSession renames the tmux session from oldName to newName.
func renameTmuxSession(oldName, newName string) error {
	normalizedOld := tmux.NormalizeTmuxSessionName(oldName)

	// Check if old tmux session exists
	exists, err := tmux.HasSession(normalizedOld)
	if err != nil {
		return fmt.Errorf("failed to check tmux session: %w", err)
	}
	if !exists {
		return fmt.Errorf("tmux session '%s' not found", normalizedOld)
	}

	// Check if new name already exists in tmux
	normalizedNew := tmux.NormalizeTmuxSessionName(newName)
	newExists, err := tmux.HasSession(normalizedNew)
	if err != nil {
		return fmt.Errorf("failed to check tmux session: %w", err)
	}
	if newExists && normalizedOld != normalizedNew {
		return fmt.Errorf("tmux session '%s' already exists", normalizedNew)
	}

	// Rename via tmux command
	socketPath := tmux.GetSocketPath()
	ctx := context.Background()
	_, err = tmux.RunWithTimeout(ctx, 5*time.Second,
		"tmux", "-S", socketPath, "rename-session",
		"-t", tmux.FormatSessionTarget(normalizedOld), normalizedNew)
	if err != nil {
		return fmt.Errorf("tmux rename-session failed: %w", err)
	}

	return nil
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
	os.Remove(oldPath)
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
	defer queue.Close()

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
