// Package devlog provides devlog functionality.
package devlog

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/pkg/cliframe"
	"github.com/vbonnet/dear-agent/tools/devlog/internal/git"
	"github.com/vbonnet/dear-agent/tools/devlog/internal/workspace"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show workspace status",
	Long: `Status displays the current state of your devlog workspace:
  - Configured repositories and their clone status
  - Configured worktrees and their creation status
  - Current branches for each worktree

This command shows what exists versus what is configured, helping you
identify missing repositories or worktrees that need to be created.

Use 'devlog sync' to create missing repos and worktrees.`,
	RunE: runStatus,
}

//nolint:gocyclo // CLI status display requires sequential branching
func runStatus(cmd *cobra.Command, args []string) error {
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

	if !flags.Quiet {
		writer.Info(fmt.Sprintf("Workspace: %s", ws.Config.Name))
		if ws.Config.Description != "" {
			writer.Info(fmt.Sprintf("Description: %s", ws.Config.Description))
		}
		writer.Info("")
	}

	// Track summary statistics
	reposConfigured := len(ws.Config.Repos)
	reposCloned := 0
	worktreesConfigured := 0
	worktreesCreated := 0

	// Process each repository
	for _, repo := range ws.Config.Repos {
		repoPath := ws.GetRepoPath(&repo)
		gitRepo := git.NewLocalRepository(repoPath)

		// Check if repository exists
		exists := gitRepo.Exists()
		if exists {
			writer.Success(fmt.Sprintf("✓ %s (cloned)", repo.Name))
			reposCloned++
		} else {
			writer.Error(fmt.Sprintf("✗ %s (not cloned)", repo.Name))
		}

		// Count configured worktrees
		worktreesConfigured += len(repo.Worktrees)

		// List worktrees if repo exists
		if exists {
			// Get actual worktrees from git
			actualWorktrees, err := gitRepo.ListWorktrees()
			if err != nil {
				writer.Error(fmt.Sprintf("  Failed to list worktrees: %v", err))
				continue
			}

			// Create map for quick lookup
			actualMap := make(map[string]git.WorktreeInfo)
			for _, wt := range actualWorktrees {
				actualMap[wt.Name] = wt
			}

			// Check each configured worktree
			for _, wt := range repo.Worktrees {
				if actual, found := actualMap[wt.Name]; found {
					// Worktree exists
					branch := actual.Branch
					if branch == "" {
						branch = "(detached)"
					}
					writer.Success(fmt.Sprintf("  ✓ %s → %s", wt.Name, branch))
					worktreesCreated++

					// Warn if on different branch than configured
					if branch != wt.Branch && branch != "(detached)" {
						writer.Info(fmt.Sprintf("    ⚠ configured branch: %s, actual: %s", wt.Branch, branch))
					}
				} else {
					// Worktree doesn't exist
					writer.Error(fmt.Sprintf("  ✗ %s (not created)", wt.Name))
				}
			}

			// List any extra worktrees not in config
			for _, actual := range actualWorktrees {
				configured := false
				for _, wt := range repo.Worktrees {
					if wt.Name == actual.Name {
						configured = true
						break
					}
				}

				if !configured {
					branch := actual.Branch
					if branch == "" {
						branch = "(detached)"
					}
					writer.Info(fmt.Sprintf("  → %s (not in config) → %s", actual.Name, branch))
				}
			}
		} else if flags.Verbose {
			// Repo doesn't exist, show what would be created
			for _, wt := range repo.Worktrees {
				writer.Info(fmt.Sprintf("  - %s → %s (pending)", wt.Name, wt.Branch))
			}
		}

		if !flags.Quiet {
			writer.Info("") // Blank line between repos
		}
	}

	// Print summary
	if !flags.Quiet {
		writer.Info("Summary:")
		writer.Info(fmt.Sprintf("  Repositories: %d configured, %d cloned", reposConfigured, reposCloned))
		writer.Info(fmt.Sprintf("  Worktrees: %d configured, %d created", worktreesConfigured, worktreesCreated))

		if reposCloned < reposConfigured || worktreesCreated < worktreesConfigured {
			writer.Info("")
			writer.Info("Run 'devlog sync' to create missing repos and worktrees")
		}
	}

	return nil
}

func init() {
	rootCmd.AddCommand(statusCmd)
}
