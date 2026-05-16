package main

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/cleanup"
	gitpkg "github.com/vbonnet/dear-agent/agm/internal/git"
)

var sessionReapWorktreesCmd = &cobra.Command{
	Use:   "reap-worktrees",
	Short: "Remove agent worktrees that are provably safe to drop",
	Long: `Removes git worktrees created by agent sessions that hold no work.

This is the filesystem/git-based safety net for worktree sprawl. Unlike
'agm session cleanup' (which only knows about manifest/Dolt-tracked
worktrees and therefore misses anything created by a raw 'git worktree
add'), this command enumerates the actual worktrees on disk and removes
one only when ALL of the following hold:

  • it lives under the worktrees base dir (default: ~/worktrees)
  • its branch starts with 'claude/' (agent session marker)
  • its working tree is clean (git status --porcelain is empty)
  • it has zero commits ahead of the repo's base ref (origin/HEAD)
  • it is not the worktree this command is running in

Anything that fails a check — or whose state cannot be positively
determined — is kept. A squash-merged branch reports commits-ahead > 0
and is therefore kept: reclaiming those is the job of
'agm audit resources' / scripts/cleanup-worktrees.sh, not this path.

It is invoked automatically by the Stop lifecycle hook so sessions clean
up after themselves. It is best-effort and never blocks session exit.

Examples:
  agm session reap-worktrees --dry-run
  agm session reap-worktrees
  agm session reap-worktrees --worktrees-dir ~/worktrees --repo ~/src/dear-agent`,
	Args: cobra.NoArgs,
	RunE: runSessionReapWorktrees,
}

var (
	reapDryRun       bool
	reapWorktreesDir string
	reapRepoPath     string
)

func init() {
	sessionCmd.AddCommand(sessionReapWorktreesCmd)
	sessionReapWorktreesCmd.Flags().BoolVar(&reapDryRun, "dry-run", false,
		"Show what would be removed without removing anything")
	sessionReapWorktreesCmd.Flags().StringVar(&reapWorktreesDir, "worktrees-dir", "",
		"Base directory containing agent worktrees (default: ~/worktrees)")
	sessionReapWorktreesCmd.Flags().StringVar(&reapRepoPath, "repo", "",
		"Repository path to enumerate worktrees for (default: current directory)")
}

func runSessionReapWorktrees(cmd *cobra.Command, args []string) error {
	opts := buildReaperOptions()

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	res := cleanup.ReapStaleWorktrees(opts, cleanup.RealReaperGitOps{}, logger)

	printReapSummary(cmd.OutOrStdout(), res, opts.DryRun)
	return nil
}

// buildReaperOptions resolves CLI flags and environment into ReaperOptions,
// defaulting the repo to the current directory and self-worktree to the
// git toplevel of the current directory so a live session never reaps itself.
func buildReaperOptions() cleanup.ReaperOptions {
	repoPath := reapRepoPath
	if repoPath == "" {
		if wd, err := os.Getwd(); err == nil {
			repoPath = wd
		}
	}

	worktreesBase := reapWorktreesDir
	if worktreesBase == "" {
		if home, err := os.UserHomeDir(); err == nil {
			worktreesBase = filepath.Join(home, "worktrees")
		}
	}

	return cleanup.ReaperOptions{
		RepoPath:      repoPath,
		WorktreesBase: worktreesBase,
		SelfPath:      currentWorktreeTopLevel(repoPath),
		DryRun:        reapDryRun,
	}
}

// currentWorktreeTopLevel returns the absolute path of the worktree the
// command is running inside, so ReapStaleWorktrees can exclude it. Best
// effort: an empty result simply disables the self-exclusion (the other
// allowlist checks still apply).
func currentWorktreeTopLevel(repoPath string) string {
	wd, err := os.Getwd()
	if err != nil {
		wd = repoPath
	}
	// Pick the most specific (longest-path) worktree that contains wd, so a
	// nested .worktrees/<uuid> wins over its parent.
	best := ""
	for _, wt := range listWorktreesQuiet(repoPath) {
		rel, err := filepath.Rel(wt.Path, wd)
		if err != nil || rel == ".." || isParentEscape(rel) {
			continue
		}
		if len(wt.Path) > len(best) {
			best = wt.Path
		}
	}
	return best
}

func listWorktreesQuiet(repoPath string) []gitpkg.Worktree {
	wts, err := gitpkg.ListWorktrees(repoPath)
	if err != nil {
		return nil
	}
	return wts
}

func isParentEscape(rel string) bool {
	return rel == ".." || filepath.IsAbs(rel) ||
		(len(rel) >= 3 && rel[0] == '.' && rel[1] == '.' && os.IsPathSeparator(rel[2]))
}

func printReapSummary(w io.Writer, res *cleanup.ReapResult, dryRun bool) {
	verb := "Removed"
	if dryRun {
		verb = "Would remove"
	}

	if len(res.Removed) == 0 {
		fmt.Fprintln(w, "No stale agent worktrees to reap.")
	} else {
		sort.Strings(res.Removed)
		for _, p := range res.Removed {
			fmt.Fprintf(w, "%s: %s\n", verb, p)
		}
		fmt.Fprintf(w, "\n%s %d worktree(s).\n", verb, len(res.Removed))
	}

	if len(res.Kept) > 0 {
		paths := make([]string, 0, len(res.Kept))
		for p := range res.Kept {
			paths = append(paths, p)
		}
		sort.Strings(paths)
		fmt.Fprintf(w, "\nKept %d worktree(s):\n", len(paths))
		for _, p := range paths {
			fmt.Fprintf(w, "  %s (%s)\n", p, res.Kept[p])
		}
	}
}
