package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/cleanup"
	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	gitpkg "github.com/vbonnet/dear-agent/agm/internal/git"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
)

var sessionResourceCleanupCmd = &cobra.Command{
	Use:   "cleanup [session-name]",
	Short: "Clean up git worktrees and branches created by a session",
	Long: `Removes git worktrees and branches that were created during a session.

Cleanup sources (in priority order):
  1. Session manifest resources field (explicit tracking by the session)
  2. Dolt worktree database (tracked via agm worktree commands)

When called without arguments, uses the current session from AGM_SESSION_NAME.

Examples:
  # Clean up the current session's resources
  agm session cleanup

  # Clean up a specific session
  agm session cleanup great-jackson-294857

  # Preview what would be removed
  agm session cleanup --dry-run

  # Force remove even with uncommitted changes
  agm session cleanup --force`,
	Args: cobra.MaximumNArgs(1),
	RunE: runSessionResourceCleanup,
}

var (
	srcDryRun bool
	srcForce  bool
)

func init() {
	sessionCmd.AddCommand(sessionResourceCleanupCmd)
	sessionResourceCleanupCmd.Flags().BoolVar(&srcDryRun, "dry-run", false,
		"Show what would be removed without actually removing anything")
	sessionResourceCleanupCmd.Flags().BoolVar(&srcForce, "force", false,
		"Force remove worktrees even with uncommitted changes")
}

func runSessionResourceCleanup(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	sessionName, err := resolveSessionName(args)
	if err != nil {
		return err
	}

	if srcDryRun {
		fmt.Printf("[dry-run] Checking resources for session: %s\n\n", sessionName)
	} else {
		fmt.Printf("Cleaning up resources for session: %s\n\n", sessionName)
	}

	removed, deleted, errs := collectCleanupResults(ctx, sessionName)
	printCleanupSummary(removed, deleted, errs)
	return nil
}

// resolveSessionName returns the session name from args or AGM_SESSION_NAME env.
func resolveSessionName(args []string) (string, error) {
	if len(args) > 0 {
		return args[0], nil
	}
	name := os.Getenv("AGM_SESSION_NAME")
	if name == "" {
		return "", fmt.Errorf("session name required: provide as argument or set AGM_SESSION_NAME")
	}
	return name, nil
}

// collectCleanupResults runs both cleanup phases and aggregates results.
func collectCleanupResults(ctx context.Context, sessionName string) (removedWorktrees, deletedBranches int, errs []string) {
	// Phase 1: manifest-declared resources (most precise)
	if rw, rb, rerrs := cleanupFromManifest(sessionName); len(rerrs) > 0 || rw > 0 || rb > 0 {
		removedWorktrees += rw
		deletedBranches += rb
		errs = append(errs, rerrs...)
	}

	// Phase 2: Dolt-tracked worktrees (catches anything tracked via agm commands)
	rw, rb, rerrs := cleanupFromDolt(ctx, sessionName)
	removedWorktrees += rw
	deletedBranches += rb
	errs = append(errs, rerrs...)
	return
}

// cleanupFromManifest reads the session manifest and removes declared resources.
func cleanupFromManifest(sessionName string) (worktreesRemoved, branchesDeleted int, errs []string) {
	res, err := loadManifestResources(sessionName)
	if err != nil {
		// Not fatal — manifest may not exist yet or session may not be in Dolt
		return
	}
	if res == nil || (len(res.Worktrees) == 0 && len(res.Branches) == 0) {
		return
	}
	return cleanupManifestResources(res, srcDryRun, srcForce)
}

// cleanupFromDolt removes worktrees tracked in the Dolt database.
func cleanupFromDolt(ctx context.Context, sessionName string) (worktreesRemoved, branchesDeleted int, errs []string) {
	doltConfig, err := dolt.DefaultConfig()
	if err != nil {
		return
	}
	adapter, err := dolt.New(doltConfig)
	if err != nil {
		return
	}
	defer adapter.Close()

	if srcDryRun {
		listDoltWorktreesDryRun(ctx, adapter, sessionName)
		return
	}

	store := &cleanup.DoltWorktreeStore{Adapter: adapter}
	result := cleanup.SessionResources(ctx, sessionName, store, cleanup.RealGitOps{}, slog.Default())
	return result.WorktreesRemoved, result.BranchesDeleted, result.Errors
}

func listDoltWorktreesDryRun(ctx context.Context, adapter *dolt.Adapter, sessionName string) {
	worktrees, err := adapter.ListWorktreesBySession(ctx, sessionName)
	if err != nil {
		return
	}
	for _, wt := range worktrees {
		fmt.Printf("[dry-run] Would remove Dolt-tracked worktree: %s (branch: %s)\n", wt.WorktreePath, wt.Branch)
	}
}

func printCleanupSummary(removedWorktrees, deletedBranches int, errs []string) {
	fmt.Println()
	if srcDryRun {
		fmt.Println("Dry run complete. No changes made.")
		return
	}
	if removedWorktrees == 0 && deletedBranches == 0 && len(errs) == 0 {
		fmt.Println("Nothing to clean up.")
	}
	if removedWorktrees > 0 {
		fmt.Printf("Removed %d worktree(s)\n", removedWorktrees)
	}
	if deletedBranches > 0 {
		fmt.Printf("Deleted %d branch(es)\n", deletedBranches)
	}
	for _, e := range errs {
		fmt.Fprintf(os.Stderr, "Warning: %s\n", e)
	}
}

// loadManifestResources reads the ResourceManifest for a session from the Dolt manifest.
func loadManifestResources(sessionName string) (*manifest.ResourceManifest, error) {
	adapter, err := getStorage()
	if err != nil {
		return nil, err
	}
	defer adapter.Close()

	m, err := adapter.GetSession(sessionName)
	if err != nil {
		return nil, fmt.Errorf("session not found: %w", err)
	}
	if m == nil {
		return nil, fmt.Errorf("session not found: %s", sessionName)
	}
	return m.Resources, nil
}

// cleanupManifestResources removes worktrees and branches declared in the manifest.
func cleanupManifestResources(res *manifest.ResourceManifest, dryRun, force bool) (worktreesRemoved, branchesDeleted int, errs []string) {
	for _, wt := range res.Worktrees {
		path := expandHome(wt.Path)
		label := fmt.Sprintf("%s (branch: %s, repo: %s)", path, wt.Branch, wt.Repo)

		if dryRun {
			fmt.Printf("[dry-run] Would remove manifest worktree: %s\n", label)
			continue
		}

		if _, statErr := os.Stat(path); os.IsNotExist(statErr) {
			fmt.Printf("Already gone: %s\n", path)
			worktreesRemoved++
			continue
		}

		repo := expandHome(wt.Repo)
		if err := gitpkg.RemoveWorktree(repo, path, force); err != nil {
			errs = append(errs, fmt.Sprintf("failed to remove worktree %s: %v", path, err))
			continue
		}
		fmt.Printf("Removed worktree: %s\n", label)
		worktreesRemoved++
	}

	for _, br := range res.Branches {
		if dryRun {
			fmt.Printf("[dry-run] Would delete manifest branch: %s in %s\n", br.Name, br.Repo)
			continue
		}

		repo := expandHome(br.Repo)
		if err := gitpkg.DeleteBranch(repo, br.Name, force); err != nil {
			if !isBranchNotFound(err) {
				errs = append(errs, fmt.Sprintf("failed to delete branch %s: %v", br.Name, err))
			}
			continue
		}
		fmt.Printf("Deleted branch: %s in %s\n", br.Name, br.Repo)
		branchesDeleted++
	}
	return
}

func expandHome(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}

func isBranchNotFound(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "not found") || strings.Contains(msg, "did not match")
}
