package devlog

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/vbonnet/ai-tools/devlog/internal/git"
	"github.com/vbonnet/ai-tools/devlog/internal/output"
	"github.com/vbonnet/ai-tools/devlog/internal/workspace"
)

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Clone repositories and create worktrees from config",
	Long: `Sync reads the workspace configuration and:
  1. Clones missing bare repositories
  2. Creates configured worktrees
  3. Checks out specified branches

The sync command is idempotent - it only creates missing repos and worktrees,
skipping any that already exist.

Use --dry-run to see what would be done without making changes.
Use --verbose to see detailed progress information.`,
	RunE: runSync,
}

func runSync(cmd *cobra.Command, args []string) error {
	// Load workspace configuration
	ws, err := workspace.LoadWorkspace(".")
	if err != nil {
		return fmt.Errorf("failed to load workspace: %w", err)
	}

	// Create output writer
	out := output.NewStdoutWriter(IsVerbose())

	dryRun := IsDryRun()
	if dryRun {
		out.Info("DRY RUN MODE - No changes will be made")
	}

	// Track summary statistics
	reposCloned := 0
	reposSkipped := 0
	worktreesCreated := 0
	worktreesSkipped := 0

	// Process each repository
	for _, repo := range ws.Config.Repos {
		repoPath := ws.GetRepoPath(&repo)
		gitRepo := git.NewLocalRepository(repoPath)

		// Clone repository if it doesn't exist
		if !gitRepo.Exists() {
			if dryRun {
				out.Progress(fmt.Sprintf("Would clone %s from %s", repo.Name, repo.URL))
				reposCloned++
			} else {
				out.Progress(fmt.Sprintf("Cloning %s...", repo.Name))
				if err := gitRepo.Clone(repo.URL, repoPath); err != nil {
					out.Error(fmt.Sprintf("Failed to clone %s: %v", repo.Name, err))
					return err
				}
				out.Success(fmt.Sprintf("Cloned %s", repo.Name))
				reposCloned++
			}
		} else {
			out.Progress(fmt.Sprintf("Repository %s already exists, skipping clone", repo.Name))
			reposSkipped++
		}

		// Create worktrees
		if len(repo.Worktrees) > 0 {
			out.Progress(fmt.Sprintf("Processing worktrees for %s...", repo.Name))

			// List existing worktrees to avoid duplicates
			var existingWorktrees map[string]bool
			if gitRepo.Exists() && !dryRun {
				existing, err := gitRepo.ListWorktrees()
				if err != nil {
					out.Error(fmt.Sprintf("Failed to list worktrees for %s: %v", repo.Name, err))
					// Continue anyway, attempt to create worktrees
				} else {
					existingWorktrees = make(map[string]bool)
					for _, wt := range existing {
						existingWorktrees[wt.Name] = true
					}
				}
			}

			for _, wt := range repo.Worktrees {
				// Check if worktree already exists
				if existingWorktrees != nil && existingWorktrees[wt.Name] {
					out.Progress(fmt.Sprintf("  Worktree %s already exists, skipping", wt.Name))
					worktreesSkipped++
					continue
				}

				if dryRun {
					out.Progress(fmt.Sprintf("  Would create worktree %s on branch %s", wt.Name, wt.Branch))
					worktreesCreated++
				} else {
					out.Progress(fmt.Sprintf("  Creating worktree %s...", wt.Name))
					if err := gitRepo.CreateWorktree(wt.Name, wt.Branch); err != nil {
						// Non-fatal: log error but continue with other worktrees
						out.Error(fmt.Sprintf("  Failed to create worktree %s: %v", wt.Name, err))
					} else {
						out.Success(fmt.Sprintf("  Created worktree %s on branch %s", wt.Name, wt.Branch))
						worktreesCreated++
					}
				}
			}
		}
	}

	// Print summary
	out.Info("")
	out.Info("Sync Summary:")
	out.Info(fmt.Sprintf("  Repositories cloned: %d", reposCloned))
	out.Info(fmt.Sprintf("  Repositories skipped (already exist): %d", reposSkipped))
	out.Info(fmt.Sprintf("  Worktrees created: %d", worktreesCreated))
	out.Info(fmt.Sprintf("  Worktrees skipped (already exist): %d", worktreesSkipped))

	if dryRun {
		out.Info("")
		out.Info("DRY RUN COMPLETE - No changes were made")
	} else {
		out.Success("Sync complete!")
	}

	return nil
}

func init() {
	rootCmd.AddCommand(syncCmd)
}
