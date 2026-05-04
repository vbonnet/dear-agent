package main

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	gitpkg "github.com/vbonnet/dear-agent/agm/internal/git"
)

var cleanupWorktreesCmd = &cobra.Command{
	Use:   "cleanup-worktrees",
	Short: "Remove orphaned worktrees and clean up branches",
	Long: `Removes worktrees that are tracked by AGM but no longer needed.

For each tracked worktree:
  1. Removes the git worktree (git worktree remove)
  2. Optionally deletes the branch (--delete-branches)
  3. Updates the tracking record in the database

By default, only merged worktrees are removed. Use --force to remove
unmerged worktrees as well.

Examples:
  agm admin cleanup-worktrees
  agm admin cleanup-worktrees --force
  agm admin cleanup-worktrees --delete-branches
  agm admin cleanup-worktrees --dry-run`,
	RunE: runCleanupWorktrees,
}

var (
	wtCleanupForce          bool
	wtCleanupDeleteBranches bool
	wtCleanupDryRun         bool
	wtCleanupSession        string
)

func init() {
	adminCmd.AddCommand(cleanupWorktreesCmd)
	cleanupWorktreesCmd.Flags().BoolVar(&wtCleanupForce, "force", false,
		"Force remove worktrees even with uncommitted changes")
	cleanupWorktreesCmd.Flags().BoolVar(&wtCleanupDeleteBranches, "delete-branches", false,
		"Also delete the associated branches after removing worktrees")
	cleanupWorktreesCmd.Flags().BoolVar(&wtCleanupDryRun, "dry-run", false,
		"Show what would be done without actually doing it")
	cleanupWorktreesCmd.Flags().StringVar(&wtCleanupSession, "session", "",
		"Only clean up worktrees for a specific session")
}

func runCleanupWorktrees(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	doltConfig, err := dolt.DefaultConfig()
	if err != nil {
		return fmt.Errorf("dolt not configured: %w", err)
	}

	adapter, err := dolt.New(doltConfig)
	if err != nil {
		return fmt.Errorf("cannot connect to Dolt: %w", err)
	}
	defer adapter.Close()

	var worktrees []dolt.WorktreeRecord
	if wtCleanupSession != "" {
		worktrees, err = adapter.ListWorktreesBySession(ctx, wtCleanupSession)
	} else {
		worktrees, err = adapter.ListActiveWorktrees(ctx)
	}
	if err != nil {
		return fmt.Errorf("failed to query worktrees: %w", err)
	}

	// Also include orphaned worktrees
	orphaned, err := adapter.ListOrphanedWorktrees(ctx)
	if err != nil {
		return fmt.Errorf("failed to query orphaned worktrees: %w", err)
	}
	worktrees = append(worktrees, orphaned...)

	if len(worktrees) == 0 {
		fmt.Println("No worktrees to clean up.")
		return nil
	}

	removedCount := 0
	errorCount := 0

	for _, wt := range worktrees {
		removed, hadErr := cleanupOneWorktree(ctx, adapter, wt)
		if hadErr {
			errorCount++
		}
		if removed {
			removedCount++
		}
	}

	if wtCleanupDryRun {
		fmt.Printf("\n%d worktree(s) would be removed.\n", len(worktrees))
	} else {
		fmt.Printf("\nRemoved %d worktree(s)", removedCount)
		if errorCount > 0 {
			fmt.Printf(", %d error(s)", errorCount)
		}
		fmt.Println(".")
	}

	return nil
}

// cleanupOneWorktree removes a single worktree (or reports it would be removed
// in dry-run mode). Returns (removed, errored): removed counts both successful
// removals and "already gone" entries that we untracked; errored signals a
// non-fatal failure so the caller can keep going through the slice.
func cleanupOneWorktree(ctx context.Context, adapter *dolt.Adapter, wt dolt.WorktreeRecord) (bool, bool) {
	label := fmt.Sprintf("%s (branch: %s, session: %s)", wt.WorktreePath, wt.Branch, wt.SessionName)

	if wtCleanupDryRun {
		fmt.Printf("[dry-run] Would remove: %s\n", label)
		return false, false
	}

	if _, statErr := os.Stat(wt.WorktreePath); os.IsNotExist(statErr) {
		fmt.Printf("Already gone: %s\n", label)
		if err := adapter.UntrackWorktree(ctx, wt.WorktreePath); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to untrack worktree %s: %v\n", wt.WorktreePath, err)
		}
		return true, false
	}

	if removeErr := gitpkg.RemoveWorktree(wt.RepoPath, wt.WorktreePath, wtCleanupForce); removeErr != nil {
		fmt.Fprintf(os.Stderr, "Failed to remove %s: %v\n", label, removeErr)
		return false, true
	}

	fmt.Printf("Removed: %s\n", label)
	if err := adapter.UntrackWorktree(ctx, wt.WorktreePath); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to untrack worktree %s: %v\n", wt.WorktreePath, err)
	}

	if wtCleanupDeleteBranches && wt.Branch != "" {
		if branchErr := gitpkg.DeleteBranch(wt.RepoPath, wt.Branch, wtCleanupForce); branchErr != nil {
			fmt.Fprintf(os.Stderr, "  Warning: failed to delete branch %s: %v\n", wt.Branch, branchErr)
		} else {
			fmt.Printf("  Deleted branch: %s\n", wt.Branch)
		}
	}
	return true, false
}
