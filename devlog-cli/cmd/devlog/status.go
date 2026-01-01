package devlog

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/vbonnet/ai-tools/devlog/internal/git"
	"github.com/vbonnet/ai-tools/devlog/internal/output"
	"github.com/vbonnet/ai-tools/devlog/internal/workspace"
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

func runStatus(cmd *cobra.Command, args []string) error {
	// Load workspace configuration
	ws, err := workspace.LoadWorkspace(".")
	if err != nil {
		return fmt.Errorf("failed to load workspace: %w", err)
	}

	out := output.NewStdoutWriter(IsVerbose())

	out.Info(fmt.Sprintf("Workspace: %s", ws.Config.Name))
	if ws.Config.Description != "" {
		out.Info(fmt.Sprintf("Description: %s", ws.Config.Description))
	}
	out.Info("")

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
			out.Success(fmt.Sprintf("✓ %s (cloned)", repo.Name))
			reposCloned++
		} else {
			out.Error(fmt.Sprintf("✗ %s (not cloned)", repo.Name))
		}

		// Count configured worktrees
		worktreesConfigured += len(repo.Worktrees)

		// List worktrees if repo exists
		if exists {
			// Get actual worktrees from git
			actualWorktrees, err := gitRepo.ListWorktrees()
			if err != nil {
				out.Error(fmt.Sprintf("  Failed to list worktrees: %v", err))
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
					out.Success(fmt.Sprintf("  ✓ %s → %s", wt.Name, branch))
					worktreesCreated++

					// Warn if on different branch than configured
					if branch != wt.Branch && branch != "(detached)" {
						out.Info(fmt.Sprintf("    ⚠ configured branch: %s, actual: %s", wt.Branch, branch))
					}
				} else {
					// Worktree doesn't exist
					out.Error(fmt.Sprintf("  ✗ %s (not created)", wt.Name))
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
					out.Info(fmt.Sprintf("  → %s (not in config) → %s", actual.Name, branch))
				}
			}
		} else {
			// Repo doesn't exist, show what would be created
			for _, wt := range repo.Worktrees {
				out.Progress(fmt.Sprintf("  - %s → %s (pending)", wt.Name, wt.Branch))
			}
		}

		out.Info("") // Blank line between repos
	}

	// Print summary
	out.Info("Summary:")
	out.Info(fmt.Sprintf("  Repositories: %d configured, %d cloned", reposConfigured, reposCloned))
	out.Info(fmt.Sprintf("  Worktrees: %d configured, %d created", worktreesConfigured, worktreesCreated))

	if reposCloned < reposConfigured || worktreesCreated < worktreesConfigured {
		out.Info("")
		out.Info("Run 'devlog sync' to create missing repos and worktrees")
	}

	return nil
}

func init() {
	rootCmd.AddCommand(statusCmd)
}
