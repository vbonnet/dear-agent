package devlog

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/pkg/cliframe"
	"github.com/vbonnet/dear-agent/tools/devlog/internal/git"
	"github.com/vbonnet/dear-agent/tools/devlog/internal/workspace"
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

//nolint:gocyclo // CLI sync orchestration requires sequential branching
func runSync(cmd *cobra.Command, args []string) error {
	flags := GetCommonFlags()
	writer := cliframe.NewWriter(cmd.OutOrStdout(), cmd.ErrOrStderr())
	writer.SetColorEnabled(!flags.NoColor)

	// Load workspace configuration
	ws, err := workspace.LoadWorkspace(".")
	if err != nil {
		return cliframe.NewError("load_workspace_failed",
			"Failed to load workspace configuration").
			WithCause(err).
			AddSuggestion("Run 'devlog init' to initialize a workspace").
			WithExitCode(cliframe.ExitNoInput)
	}

	dryRun := flags.DryRun
	if dryRun && !flags.Quiet {
		writer.Info("DRY RUN MODE - No changes will be made")
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
				if flags.Verbose {
					writer.Info(fmt.Sprintf("Would clone %s from %s", repo.Name, repo.URL))
				}
				reposCloned++
			} else {
				if flags.Verbose {
					writer.Info(fmt.Sprintf("Cloning %s...", repo.Name))
				}
				if err := gitRepo.Clone(repo.URL, repoPath); err != nil {
					return cliframe.NewError("clone_failed",
						fmt.Sprintf("Failed to clone %s", repo.Name)).
						WithCause(err).
						AddSuggestion("Check your network connection and repository URL").
						WithExitCode(cliframe.ExitTempFail).
						MarkRetryable(30)
				}
				writer.Success(fmt.Sprintf("Cloned %s", repo.Name))
				reposCloned++
			}
		} else {
			if flags.Verbose {
				writer.Info(fmt.Sprintf("Repository %s already exists, skipping clone", repo.Name))
			}
			reposSkipped++
		}

		// Create worktrees
		if len(repo.Worktrees) > 0 {
			if flags.Verbose {
				writer.Info(fmt.Sprintf("Processing worktrees for %s...", repo.Name))
			}

			// List existing worktrees to avoid duplicates
			var existingWorktrees map[string]bool
			if gitRepo.Exists() && !dryRun {
				existing, err := gitRepo.ListWorktrees()
				if err != nil {
					writer.Error(fmt.Sprintf("Failed to list worktrees for %s: %v", repo.Name, err))
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
					if flags.Verbose {
						writer.Info(fmt.Sprintf("  Worktree %s already exists, skipping", wt.Name))
					}
					worktreesSkipped++
					continue
				}

				if dryRun {
					if flags.Verbose {
						writer.Info(fmt.Sprintf("  Would create worktree %s on branch %s", wt.Name, wt.Branch))
					}
					worktreesCreated++
				} else {
					if flags.Verbose {
						writer.Info(fmt.Sprintf("  Creating worktree %s...", wt.Name))
					}
					if err := gitRepo.CreateWorktree(wt.Name, wt.Branch); err != nil {
						// Non-fatal: log error but continue with other worktrees
						writer.Error(fmt.Sprintf("  Failed to create worktree %s: %v", wt.Name, err))
					} else {
						writer.Success(fmt.Sprintf("  Created worktree %s on branch %s", wt.Name, wt.Branch))
						worktreesCreated++
					}
				}
			}
		}
	}

	// Print summary
	if !flags.Quiet {
		writer.Info("")
		writer.Info("Sync Summary:")
		writer.Info(fmt.Sprintf("  Repositories cloned: %d", reposCloned))
		writer.Info(fmt.Sprintf("  Repositories skipped (already exist): %d", reposSkipped))
		writer.Info(fmt.Sprintf("  Worktrees created: %d", worktreesCreated))
		writer.Info(fmt.Sprintf("  Worktrees skipped (already exist): %d", worktreesSkipped))

		if dryRun {
			writer.Info("")
			writer.Info("DRY RUN COMPLETE - No changes were made")
		} else {
			writer.Success("Sync complete!")
		}
	}

	return nil
}

func init() {
	rootCmd.AddCommand(syncCmd)
}
