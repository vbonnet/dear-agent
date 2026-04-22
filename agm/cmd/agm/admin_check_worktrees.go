package main

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	gitpkg "github.com/vbonnet/dear-agent/agm/internal/git"
)

var checkWorktreesCmd = &cobra.Command{
	Use:   "check-worktrees",
	Short: "Check for orphaned worktrees (exit gate)",
	Long: `Checks if there are any active or orphaned worktrees tracked by AGM.

This command is intended as an exit gate: it returns a non-zero exit code
when orphaned worktrees are found, preventing session exit until they are
cleaned up or acknowledged.

Exit codes:
  0 - No orphaned worktrees found
  1 - Orphaned worktrees detected (blocks exit)
  2 - Error checking worktrees

Examples:
  agm admin check-worktrees
  agm admin check-worktrees --session my-session`,
	RunE: runCheckWorktrees,
}

var checkWorktreesSession string

func init() {
	adminCmd.AddCommand(checkWorktreesCmd)
	checkWorktreesCmd.Flags().StringVar(&checkWorktreesSession, "session", "",
		"Check worktrees for a specific session only")
}

func runCheckWorktrees(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	doltConfig, err := dolt.DefaultConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Dolt not configured (%v), skipping worktree check\n", err)
		return nil
	}

	adapter, err := dolt.New(doltConfig)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Cannot connect to Dolt (%v), skipping worktree check\n", err)
		return nil
	}
	defer adapter.Close()

	var worktrees []dolt.WorktreeRecord
	if checkWorktreesSession != "" {
		worktrees, err = adapter.ListWorktreesBySession(ctx, checkWorktreesSession)
	} else {
		worktrees, err = adapter.ListActiveWorktrees(ctx)
	}
	if err != nil {
		return fmt.Errorf("failed to query worktrees: %w", err)
	}

	if len(worktrees) == 0 {
		fmt.Println("No active worktrees found.")
		return nil
	}

	// Check which worktrees still exist on disk
	var orphans []dolt.WorktreeRecord
	for _, wt := range worktrees {
		// Verify the worktree directory still exists
		if _, statErr := os.Stat(wt.WorktreePath); os.IsNotExist(statErr) {
			// Directory gone but DB says active - mark as removed
			if err := adapter.UntrackWorktree(ctx, wt.WorktreePath); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to untrack worktree %s: %v\n", wt.WorktreePath, err)
			}
			continue
		}

		// Verify it's still a valid git worktree
		listed, listErr := gitpkg.ListWorktrees(wt.RepoPath)
		if listErr != nil {
			orphans = append(orphans, wt)
			continue
		}

		found := false
		for _, lw := range listed {
			if lw.Path == wt.WorktreePath {
				found = true
				break
			}
		}
		if !found {
			// Not in git worktree list but directory exists - stale
			if err := adapter.UntrackWorktree(ctx, wt.WorktreePath); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to untrack stale worktree %s: %v\n", wt.WorktreePath, err)
			}
			continue
		}

		orphans = append(orphans, wt)
	}

	if len(orphans) == 0 {
		fmt.Println("No active worktrees found.")
		return nil
	}

	fmt.Fprintf(os.Stderr, "Found %d active worktree(s):\n\n", len(orphans))
	for _, wt := range orphans {
		fmt.Fprintf(os.Stderr, "  Session: %s\n", wt.SessionName)
		fmt.Fprintf(os.Stderr, "  Path:    %s\n", wt.WorktreePath)
		fmt.Fprintf(os.Stderr, "  Branch:  %s\n", wt.Branch)
		fmt.Fprintf(os.Stderr, "  Created: %s\n\n", wt.CreatedAt.Format("2006-01-02 15:04"))
	}

	fmt.Fprintf(os.Stderr, "Run 'agm admin cleanup-worktrees' to remove them.\n")
	os.Exit(1)
	return nil
}
