package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/ops"
	"github.com/vbonnet/dear-agent/agm/internal/ui"
)

var (
	batchMergeSessions    string
	batchMergeRepoDir     string
	batchMergeTargetBranch string
	batchMergeDryRun      bool
)

var batchMergeCmd = &cobra.Command{
	Use:   "merge",
	Short: "Cherry-pick commits from verified workers to target branch",
	Long: `Cherry-pick commits from worker session branches into the target branch.

By default, merges from all DONE/OFFLINE workers. Use --sessions to limit
to specific workers. Use --dry-run to preview without making changes.

Examples:
  agm batch merge --repo-dir /path/to/repo
  agm batch merge --repo-dir /path/to/repo --sessions "w1,w2"
  agm batch merge --repo-dir /path/to/repo --target-branch main
  agm batch merge --repo-dir /path/to/repo --dry-run`,
	RunE: runBatchMerge,
}

func init() {
	batchMergeCmd.Flags().StringVar(&batchMergeSessions, "sessions", "", "Comma-separated list of session names to merge")
	batchMergeCmd.Flags().StringVar(&batchMergeRepoDir, "repo-dir", "", "Repository directory (required)")
	batchMergeCmd.Flags().StringVar(&batchMergeTargetBranch, "target-branch", "", "Target branch to merge into (default: current HEAD)")
	batchMergeCmd.Flags().BoolVar(&batchMergeDryRun, "dry-run", false, "Preview what would be merged without making changes")
	_ = batchMergeCmd.MarkFlagRequired("repo-dir")
	batchCmd.AddCommand(batchMergeCmd)
}

func runBatchMerge(cmd *cobra.Command, args []string) error {
	opCtx, cleanup, err := newOpContextWithStorage()
	if err != nil {
		return fmt.Errorf("failed to connect to storage: %w", err)
	}
	defer cleanup()

	req := &ops.BatchMergeRequest{
		RepoDir:      batchMergeRepoDir,
		TargetBranch: batchMergeTargetBranch,
		DryRun:       batchMergeDryRun,
	}

	if batchMergeSessions != "" {
		for _, s := range strings.Split(batchMergeSessions, ",") {
			s = strings.TrimSpace(s)
			if s != "" {
				req.Sessions = append(req.Sessions, s)
			}
		}
	}

	result, err := ops.BatchMerge(opCtx, req)
	if err != nil {
		return handleError(err)
	}

	return printResult(result, func() {
		printBatchMergeSummary(result)
	})
}

func printBatchMergeSummary(result *ops.BatchMergeResult) {
	if result.DryRun {
		fmt.Println("\n[DRY RUN] No changes were made.")
	}

	fmt.Println()
	fmt.Println(strings.Repeat("=", 60))
	fmt.Printf("Batch Merge Summary (%d workers)\n", result.Summary.Total)
	fmt.Println(strings.Repeat("=", 60))

	for _, m := range result.Merged {
		fmt.Printf("  [OK] %s (%d commits from %s)\n", m.Name, len(m.Commits), m.Branch)
		for _, c := range m.Commits {
			hash := c
			if len(hash) > 8 {
				hash = hash[:8]
			}
			fmt.Printf("       %s\n", hash)
		}
	}
	for _, s := range result.Skipped {
		fmt.Printf("  [SKIP] %s: %s\n", s.Name, s.Reason)
	}

	fmt.Println()
	if result.Summary.Merged > 0 {
		ui.PrintSuccess(fmt.Sprintf("%d worker(s) merged", result.Summary.Merged))
	}
	if result.Summary.Skipped > 0 {
		ui.PrintWarning(fmt.Sprintf("%d worker(s) skipped", result.Summary.Skipped))
	}
}
