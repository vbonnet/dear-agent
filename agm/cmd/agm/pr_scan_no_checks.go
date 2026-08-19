package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/nochecks"
)

var (
	noCheckRepo    string
	noCheckBranch  string
	noCheckLimit   int
	noCheckTrigger bool
	noCheckDryRun  bool
)

var prScanNoChecksCmd = &cobra.Command{
	Use:   "scan-no-checks",
	Short: "Detect open PRs whose head SHA has zero required CI check-runs",
	Long: `Scan open pull requests and flag any whose head commit is missing CI entirely.

This closes the inverse-direction process gap to scan-orphaned. A push-then-PR-open
race can drop the CI trigger, leaving a PR's head SHA with zero check-runs. The
required checks never report, so the safe-merge babysit loop skips the PR as
"pending" on every pass — no red, no green, no signal. That stranded PRs
#579/#581/#582 for 8+ hours with no CI.

For each open, non-draft PR it reads the check-runs on the head SHA and compares
them against the branch-protection required set. A PR is flagged only when NONE of
the required checks has a run on the head SHA — a partial set means CI fired and is
merely still in progress, so the PR is left alone. When the required set can't be
read, it falls back to "any check-run at all", which can only ever under-flag.

With --trigger it re-triggers CI on each stuck PR by pushing an empty commit (same
tree, no file changes) to the head branch via the GitHub API. This is headless — no
checkout — and fires the pull_request:synchronize event CI listens on.
workflow_dispatch is not used: it would run CI against the branch ref, not the
pull_request context, so its check-runs could not satisfy the PR's required checks.

Exit codes:
  0 - No stuck PRs found, or every stuck PR was re-triggered successfully
  1 - Stuck PRs found and left un-triggered, a dry-run match, or a trigger failed
  2 - Error scanning (gh unavailable, parse failure, etc.)

Examples:
  agm pr scan-no-checks
  agm pr scan-no-checks --repo vbonnet/dear-agent --output json
  agm pr scan-no-checks --trigger
  agm pr scan-no-checks --trigger --dry-run`,
	RunE: runPRScanNoChecks,
}

func init() {
	prCmd.AddCommand(prScanNoChecksCmd)

	prScanNoChecksCmd.Flags().StringVar(&noCheckRepo, "repo", "vbonnet/dear-agent",
		"GitHub repository (owner/name) to scan")
	prScanNoChecksCmd.Flags().StringVar(&noCheckBranch, "branch", "main",
		"protected branch whose required-check set defines 'missing CI'")
	prScanNoChecksCmd.Flags().IntVar(&noCheckLimit, "limit", 100,
		"maximum number of open PRs to scan")
	prScanNoChecksCmd.Flags().BoolVar(&noCheckTrigger, "trigger", false,
		"re-trigger CI on each stuck PR via an empty commit to its head branch")
	prScanNoChecksCmd.Flags().BoolVar(&noCheckDryRun, "dry-run", false,
		"with --trigger, report what would be re-triggered without pushing")
}

// noCheckItem is one stuck PR and the outcome of any re-trigger attempt.
type noCheckItem struct {
	nochecks.StuckPR
	Triggered bool   `json:"triggered"`
	Error     string `json:"error,omitempty"`
}

// NoChecksScanResult is the structured output of a no-checks scan.
type NoChecksScanResult struct {
	Repo    string        `json:"repo"`
	OpenPRs int           `json:"open_prs"`
	Trigger bool          `json:"trigger"`
	DryRun  bool          `json:"dry_run"`
	Stuck   []noCheckItem `json:"stuck"`
}

func runPRScanNoChecks(cmd *cobra.Command, args []string) error {
	if noCheckLimit < 1 {
		cmd.SilenceUsage = true
		return fmt.Errorf("--limit must be a positive integer, got %d", noCheckLimit)
	}

	prs, err := nochecks.ListOpenPRs(noCheckRepo, noCheckLimit)
	if err != nil {
		cmd.SilenceUsage = true
		return err
	}

	required := nochecks.FetchRequiredChecks(noCheckRepo, noCheckBranch)
	stuck, readErrs := nochecks.Scan(prs, required, nochecks.CheckRunsFor(noCheckRepo))

	// A per-PR check-runs read failure must not blind the whole scan; surface it
	// and keep going. Such PRs are never flagged (conservative).
	for num, rerr := range readErrs {
		fmt.Fprintf(os.Stderr, "scan-no-checks: PR #%d: reading check-runs: %v\n", num, rerr)
	}

	items := make([]noCheckItem, 0, len(stuck))
	for _, s := range stuck {
		items = append(items, noCheckItem{StuckPR: s})
	}

	failures := 0
	if noCheckTrigger && !noCheckDryRun {
		for i := range items {
			if err := nochecks.RetriggerCI(noCheckRepo, items[i].StuckPR); err != nil {
				items[i].Error = err.Error()
				failures++
				continue
			}
			items[i].Triggered = true
		}
	}

	result := &NoChecksScanResult{
		Repo:    noCheckRepo,
		OpenPRs: len(prs),
		Trigger: noCheckTrigger,
		DryRun:  noCheckDryRun,
		Stuck:   items,
	}

	if err := printResult(result, func() { printNoChecksScanText(result) }); err != nil {
		return err
	}

	// Exit-gate semantics (mirrors `agm pr scan-orphaned`): a non-zero exit lets
	// supervisor tooling treat stranded PRs as an alarm without parsing stdout.
	switch {
	case len(items) == 0:
		return nil
	case !noCheckTrigger:
		cmd.SilenceUsage = true
		return fmt.Errorf("found %d open PR(s) with zero required CI check-runs — re-run with --trigger to restart CI", len(items))
	case noCheckDryRun:
		cmd.SilenceUsage = true
		return fmt.Errorf("%d open PR(s) would be re-triggered (dry-run)", len(items))
	case failures > 0:
		cmd.SilenceUsage = true
		return fmt.Errorf("re-trigger failed for %d of %d stuck PR(s)", failures, len(items))
	default:
		return nil
	}
}

func printNoChecksScanText(r *NoChecksScanResult) {
	fmt.Printf("\n=== No-Checks PR Scan — %s ===\n\n", r.Repo)
	fmt.Printf("Open PRs scanned: %d\n", r.OpenPRs)
	fmt.Printf("Stuck (head SHA has 0 required CI check-runs): %d\n", len(r.Stuck))

	if len(r.Stuck) == 0 {
		fmt.Println("\n  (none — every open PR has required CI check-runs)")
		fmt.Println()
		return
	}

	for _, it := range r.Stuck {
		status := "needs --trigger"
		switch {
		case it.Error != "":
			status = "re-trigger FAILED: " + it.Error
		case it.Triggered:
			status = "re-triggered"
		case r.DryRun:
			status = "would re-trigger"
		}
		fmt.Printf("\n  ✗ #%d %s\n", it.Number, it.Title)
		fmt.Printf("    head %s (%s) [%s]\n", nochecks.ShortSHA(it.HeadSHA), it.HeadRefName, status)
	}
	fmt.Println()
}
