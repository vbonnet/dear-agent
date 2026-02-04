package main

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/lock"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/ui"
)

var force bool

var unlockCmd = &cobra.Command{
	Use:   "unlock",
	Short: "Remove stale lock files",
	Long: `Check for and remove stale csm lock files.

This command checks if the lock is held by a process that is still running.
If the process has exited, the lock is considered stale and will be removed.

Use --force to remove the lock even if the process is still running (DANGEROUS).

Examples:
  csm unlock              # Check lock status and remove if stale
  csm unlock --force      # Force remove lock (even if process is running)`,
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
		if force {
			ui.PrintWarning(fmt.Sprintf("WARNING: Forcing unlock of active process %d", info.PID))
			fmt.Printf("   Lock path: %s\n", lockPath)
			fmt.Println("   This may cause race conditions if the process is actually running!")

			if err := lock.ForceUnlock(lockPath); err != nil {
				ui.PrintError(err,
					"Failed to force unlock active lock",
					"  • Check lock file permissions: ls -l "+lockPath+"\n"+
						"  • Verify file is not owned by another user: ls -l "+lockPath+"\n"+
						"  • Kill holding process first: kill "+fmt.Sprintf("%d", info.PID)+"\n"+
						"  • Try manual removal: rm "+lockPath)
				return err
			}

			ui.PrintSuccess("Lock forcefully removed")
			return nil
		}

		// Lock is active, force not specified
		ui.PrintError(
			fmt.Errorf("lock is held by active process %d", info.PID),
			"Lock is currently in use",
			fmt.Sprintf("  • Wait for the process to finish\n"+
				"  • Kill the process: kill %d\n"+
				"  • Force unlock: csm unlock --force (DANGEROUS)", info.PID),
		)
		return fmt.Errorf("lock is active")
	},
}

func init() {
	unlockCmd.Flags().BoolVar(&force, "force", false, "force unlock even if process is running (DANGEROUS)")
	rootCmd.AddCommand(unlockCmd)
}
