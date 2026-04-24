package main

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/lock"
	"github.com/vbonnet/dear-agent/agm/internal/ui"
)

var unlockCmd = &cobra.Command{
	Use:   "unlock",
	Short: "Remove stale lock files",
	Long: `Check for and remove stale agm lock files.

This command checks if the lock is held by a process that is still running.
If the process has exited, the lock is considered stale and will be removed.

Examples:
  agm admin unlock              # Check lock status and remove if stale`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Get lock path
		lockPath, err := lock.DefaultLockPath()
		if err != nil {
			return fmt.Errorf("failed to get lock path: %w", err)
		}

		// Check lock status
		info, err := lock.CheckLock(lockPath)
		if err != nil {
			return fmt.Errorf("failed to check lock: %w", err)
		}

		// Display status
		if !info.Exists {
			ui.PrintSuccess("No lock file found.")
			fmt.Printf("   Lock path: %s\n", lockPath)
			return nil
		}

		if info.IsStale {
			// Lock is stale - safe to remove
			fmt.Printf("🔓 Lock is stale (process %d no longer running)\n", info.PID)
			fmt.Printf("   Lock path: %s\n", lockPath)

			if err := lock.ForceUnlock(lockPath); err != nil {
				ui.PrintError(err,
					"Failed to remove stale lock",
					"  • Check lock file permissions: ls -l "+lockPath+"\n"+
						"  • Verify file is not owned by another user: ls -l "+lockPath+"\n"+
						"  • Try manual removal: rm "+lockPath)
				return err
			}

			ui.PrintSuccess("Lock removed successfully")
			return nil
		}

		// Lock is held by active process
		ui.PrintError(
			fmt.Errorf("lock is held by active process %d", info.PID),
			"Lock is currently in use",
			fmt.Sprintf("  • Wait for the process to finish\n"+
				"  • Kill the process: kill %d", info.PID),
		)
		return fmt.Errorf("lock is active")
	},
}

func init() {
	adminCmd.AddCommand(unlockCmd)
}
