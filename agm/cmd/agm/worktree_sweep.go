package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/ops"
)

// worktreeCmd is the parent for cross-repo worktree maintenance. Unlike
// `agm session reap-worktrees` (on-Stop, single-repo, fires only when a
// session exits cleanly) and `agm admin cleanup-worktrees` (Dolt-tracked
// worktrees only), this group operates by filesystem discovery across every
// repo under ~/worktrees — the backfill path the on-Stop reaper structurally
// cannot cover (pre-existing / killed-session / non-AGM husks).
var worktreeCmd = &cobra.Command{
	Use:   "worktree",
	Short: "Cross-repo git worktree maintenance",
	Long: `Worktree commands operate on the actual worktrees on disk under
~/worktrees/<repo>/<name>, across every repo — independent of session
manifests or the on-Stop reaper.

Examples:
  agm worktree sweep            # classify everything (read-only)
  agm worktree sweep --execute  # also reap provably-merged husks`,
	Args: cobra.ArbitraryArgs,
	RunE: groupRunE,
}

var worktreeSweepCmd = &cobra.Command{
	Use:   "sweep",
	Short: "Classify all worktrees and reap the provably-merged ones",
	Long: `Scans every git worktree under the worktrees base dir (default:
~/worktrees), classifies each, and — only with --execute — removes the ones
that are provably MERGED and clean, deleting their orphaned local branch.

This is the backfill the #120 on-Stop reaper cannot do: it runs over the
whole pre-existing population regardless of which session (if any) created a
worktree or whether that session ever fired its Stop hook.

Each worktree is classified as exactly one of:

  ACTIVE         a live AGM/tmux session (or a running session's nested
                 sandbox, or this sweep's own worktree) owns it — untouched
  AWAITING_INPUT the session transcript ends with the assistant waiting on
                 the user (AskUserQuestion, or a trailing question) — kept
                 even if it looks merged, so the user can return and answer
  MERGED         branch is an ancestor of the base ref OR its PR is MERGED,
                 and the tree is clean — the ONLY class reaped by --execute
  DIRTY          uncommitted or untracked changes — never touched
  ORPHANED       clean but not provably merged: a husk (no PR / open PR /
                 closed-unmerged / unpushed commits). Kept; 'unpushed-commits'
                 is real data-loss risk and is surfaced loudly
  UNKNOWN        safety could not be established (probe failed / detached
                 HEAD) — kept, fail-safe

The merge test is squash-safe: it never uses ahead/behind (which a
squash-merge inflates to "ahead>0", the bug that lets husks accumulate). It
proves a merge only via ancestor-of-base OR an authoritative 'gh' PR state of
MERGED. Worktrees sharing an identical HEAD commit subject within a repo are
flagged as a fan-out duplicate group (report-only).

Dry-run is the default; nothing is removed without --execute. Even with
--execute only MERGED worktrees are removed — everything else is reported and
left exactly as found (allowlist semantics).

Examples:
  agm worktree sweep
  agm worktree sweep --execute
  agm worktree sweep --no-pr-check
  agm worktree sweep -o json > audit.json
  agm worktree sweep --worktrees-dir ~/worktrees --execute
  agm worktree sweep --orphan-only   # daily audit: branches with commits above main but no PR`,
	Args: cobra.NoArgs,
	RunE: runWorktreeSweep,
}

var (
	sweepExecute      bool
	sweepWorktreesDir string
	sweepNoPRCheck    bool
	sweepOrphanOnly   bool
)

func init() {
	rootCmd.AddCommand(worktreeCmd)
	worktreeCmd.AddCommand(worktreeSweepCmd)
	worktreeSweepCmd.Flags().BoolVar(&sweepExecute, "execute", false,
		"Actually remove provably-merged worktrees (default: dry-run only)")
	worktreeSweepCmd.Flags().StringVar(&sweepWorktreesDir, "worktrees-dir", "",
		"Base directory containing worktrees (default: ~/worktrees)")
	worktreeSweepCmd.Flags().BoolVar(&sweepNoPRCheck, "no-pr-check", false,
		"Disable the gh PR-state oracle; squash-merged husks kept conservatively")
	worktreeSweepCmd.Flags().BoolVar(&sweepOrphanOnly, "orphan-only", false,
		"Print only orphan branches (commits above main merge-base, no open or merged PR)")
}

// sweepActiveSessions is the seam the live-set lookup goes through so the
// fail-closed path can be exercised without a Dolt or tmux host.
var sweepActiveSessions = getActiveSessions

func runWorktreeSweep(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	worktreesBase := sweepWorktreesDir
	if worktreesBase == "" {
		if home, err := os.UserHomeDir(); err == nil {
			worktreesBase = filepath.Join(home, "worktrees")
		}
	}

	// Active-session set: reused from the audit-resources path so the sweep
	// shares one definition of "live" (Dolt, tmux fallback). A failed lookup
	// is fatal for --execute: fewer ACTIVE matches is not "more
	// conservative" for a command that deletes — it is exactly how two live
	// worktrees were reaped during the 2026-07-10 audit (ce-3knl.1). The
	// merge/dirty guards do not compensate, because a live worktree sitting
	// clean at origin/main classifies as MERGED (ce-3ch7).
	active, err := sweepActiveSessions(ctx)
	activeKnown := err == nil
	if err != nil {
		if sweepExecute {
			return fmt.Errorf("could not query active sessions (%w): refusing to execute a "+
				"sweep that cannot prove which worktrees are live — re-run without --execute "+
				"to classify only", err)
		}
		fmt.Fprintf(os.Stderr,
			"Warning: could not query active sessions (%v); dry-run classification only, "+
				"--execute would refuse to run\n", err)
	}

	// Resolve self from the cwd's repo (worktreesBase itself is not a git
	// repo, so currentWorktreeTopLevel must look up the repo containing the
	// cwd) so a sweep run from inside a worktree never reaps itself.
	selfRepo := ""
	if wd, wdErr := os.Getwd(); wdErr == nil {
		selfRepo = wd
	}

	opts := ops.SweepOptions{
		WorktreesBase:       worktreesBase,
		Execute:             sweepExecute,
		CheckPR:             !sweepNoPRCheck,
		ActiveSessions:      active,
		ActiveSessionsKnown: activeKnown,
		SelfPath:            currentWorktreeTopLevel(selfRepo),
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	res, err := ops.Sweep(opts, ops.RealSweepDeps{}, logger)
	if err != nil {
		return fmt.Errorf("sweep failed: %w", err)
	}

	if outputFormat == "json" {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(res)
	}
	printSweepReport(cmd.OutOrStdout(), res, sweepExecute, sweepOrphanOnly)
	return nil
}

func printSweepReport(out io.Writer, res *ops.SweepResult, execute, orphanOnly bool) {
	if orphanOnly {
		printOrphanBranchReport(out, res)
		return
	}
	if len(res.Worktrees) == 0 {
		_, _ = fmt.Fprintln(out, "No worktrees found.")
		return
	}
	printWorktreeTable(out, res)
	printReapedSection(out, res, execute)
	printOrphanSection(out, res)
	printFailedSection(out, res)
}

func printWorktreeTable(out io.Writer, res *ops.SweepResult) {
	w := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "CLASS\tREPO\tBRANCH\tAGE\tPR\tREASON\tPATH")
	for _, s := range res.Worktrees {
		branch := s.Branch
		if branch == "" {
			branch = "(detached)"
		}
		reason := s.Reason
		if s.DupCount > 1 {
			reason = fmt.Sprintf("%s [dup x%d]", reason, s.DupCount)
		}
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			s.Class, s.Repo, branch, age(s.LastCommit), prCol(s.PRState), reason, s.Path)
	}
	_ = w.Flush()

	counts := res.Counts()
	classOrder := []ops.Classification{
		ops.ClassActive, ops.ClassAwaitingInput, ops.ClassMerged,
		ops.ClassDirty, ops.ClassOrphaned, ops.ClassUnknown,
	}
	_, _ = fmt.Fprintf(out, "\n%d worktree(s):", len(res.Worktrees))
	for _, c := range classOrder {
		if n := counts[c]; n > 0 {
			_, _ = fmt.Fprintf(out, " %s=%d", c, n)
		}
	}
	_, _ = fmt.Fprintln(out)
}

func printReapedSection(out io.Writer, res *ops.SweepResult, execute bool) {
	if len(res.Removed) == 0 {
		_, _ = fmt.Fprintln(out, "\nNothing is provably reapable.")
		return
	}
	sort.Strings(res.Removed)
	verb := "Would reap"
	if execute {
		verb = "Reaped"
	}
	_, _ = fmt.Fprintf(out, "\n%s %d provably-merged worktree(s):\n", verb, len(res.Removed))
	for _, p := range res.Removed {
		_, _ = fmt.Fprintf(out, "  %s\n", p)
	}
	if !execute {
		_, _ = fmt.Fprintln(out, "\nRe-run with --execute to remove the MERGED worktrees above.")
	}
}

func printFailedSection(out io.Writer, res *ops.SweepResult) {
	if len(res.Failed) == 0 {
		return
	}
	paths := make([]string, 0, len(res.Failed))
	for p := range res.Failed {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	_, _ = fmt.Fprintf(out, "\n%d removal(s) failed:\n", len(paths))
	for _, p := range paths {
		_, _ = fmt.Fprintf(out, "  %s: %s\n", p, res.Failed[p])
	}
}

// printOrphanSection prints the orphan-branch subsection inside the normal
// sweep report. It is a no-op when no orphan branches are detected.
func printOrphanSection(out io.Writer, res *ops.SweepResult) {
	var orphans []ops.WorktreeStatus
	for _, s := range res.Worktrees {
		if s.IsOrphanBranch {
			orphans = append(orphans, s)
		}
	}
	if len(orphans) == 0 {
		return
	}
	_, _ = fmt.Fprintf(out, "\n%d orphan branch(es) — commits above main, no open or merged PR:\n", len(orphans))
	for _, s := range orphans {
		_, _ = fmt.Fprintf(out, "  %s  branch=%s  commits_above_main=%d  path=%s\n",
			s.Repo, s.Branch, s.CommitsAboveMergeBase, s.Path)
	}
	_, _ = fmt.Fprintln(out, "  → open a PR or run `agm worktree sweep --execute` to remove")
}

// printOrphanBranchReport is the --orphan-only mode: prints ONLY orphan-branch
// worktrees, one per line (suitable for piping into daily audit scripts).
func printOrphanBranchReport(out io.Writer, res *ops.SweepResult) {
	found := false
	for _, s := range res.Worktrees {
		if !s.IsOrphanBranch {
			continue
		}
		found = true
		_, _ = fmt.Fprintf(out, "ORPHAN  repo=%-20s  branch=%-40s  commits=%d  path=%s\n",
			s.Repo, s.Branch, s.CommitsAboveMergeBase, s.Path)
	}
	if !found {
		_, _ = fmt.Fprintln(out, "No orphan branches found.")
	}
}

func age(t time.Time) string {
	if t.IsZero() {
		return "?"
	}
	d := time.Since(t)
	switch {
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}

func prCol(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
