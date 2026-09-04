package main

import (
	"fmt"
	"os"
	"sort"

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

For each open, non-draft PR it reads the effective required set for that PR's
provider-observed base, then compares the check-runs on the head SHA against it.
A PR is flagged only when NONE of the required checks has a run on the head SHA —
a partial set means CI fired and is merely still in progress, so the PR is left
alone. Each required set is the complete union of applied rulesets and classic
branch protection. If any candidate base policy can't be completely read, or
contains a requirement this name-only scanner can't prove, the scan stops before
classification or re-trigger. Only an authoritatively empty policy uses the
conservative "any check-run at all" fallback. Drafts are excluded before policy
and check-run reads.

By default every provider-returned open PR up to --limit is considered, and
every eligible non-draft PR among them is scanned. --branch is an optional PR
base filter, not the policy source for otherwise unrelated PRs.

Every head check-run page is read before classification. A failed page makes that
PR indeterminate, is included in structured output, and keeps the command
non-successful rather than treating incomplete evidence as healthy.

With --trigger the write path first resolves the existing tree and completely
re-reads check-runs, then uses a fresh PR read as its final provider observation.
It refuses any closed, draft, retargeted, advanced, forked, or self-healed
candidate. It then re-triggers CI by pushing an empty commit (same tree, no file
changes) to the head branch via the GitHub API. Dry-run performs the current
check-run and PR validation without reading or writing Git objects. Caller
cancellation stops the trigger sequence and later candidates.

The final reads are snapshot guards, not an atomic GitHub transaction: PR state
can change or CI can appear after those observations. The empty commit retains
the observed head as parent and the ref update is explicitly non-force, which
rejects ordinary concurrent head advances but is not an expected-old-SHA CAS.
This is headless — no checkout — and fires the pull_request:synchronize event CI listens on.
workflow_dispatch is not used: it would run CI against the branch ref, not the
pull_request context, so its check-runs could not satisfy the PR's required checks.

Exit codes:
  0 - No stuck PRs found, or every stuck PR was re-triggered successfully
  1 - Stuck/indeterminate PRs remain, a dry-run match, or a trigger failed
  2 - Error scanning (gh unavailable, parse failure, etc.)

Examples:
  agm pr scan-no-checks
  agm pr scan-no-checks --repo vbonnet/dear-agent --output json
  agm pr scan-no-checks --branch main
  agm pr scan-no-checks --trigger
  agm pr scan-no-checks --trigger --dry-run`,
	RunE: runPRScanNoChecks,
}

func init() {
	prCmd.AddCommand(prScanNoChecksCmd)

	prScanNoChecksCmd.Flags().StringVar(&noCheckRepo, "repo", "vbonnet/dear-agent",
		"GitHub repository (owner/name) to scan")
	prScanNoChecksCmd.Flags().StringVar(&noCheckBranch, "branch", "",
		"optional pull request base filter (empty scans all observed bases)")
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
	Triggered     bool   `json:"triggered"`
	NoLongerStuck bool   `json:"no_longer_stuck,omitempty"`
	Error         string `json:"error,omitempty"`
}

type noCheckReadError struct {
	Number int    `json:"number"`
	Error  string `json:"error"`
}

// NoChecksScanResult is the structured output of a no-checks scan.
type NoChecksScanResult struct {
	Repo        string             `json:"repo"`
	BaseFilter  string             `json:"base_filter"`
	OpenPRs     int                `json:"open_prs"`
	EligiblePRs int                `json:"eligible_prs"`
	Trigger     bool               `json:"trigger"`
	DryRun      bool               `json:"dry_run"`
	Stuck       []noCheckItem      `json:"stuck"`
	ReadErrors  []noCheckReadError `json:"read_errors"`
}

func runPRScanNoChecks(cmd *cobra.Command, args []string) error {
	if noCheckLimit < 1 {
		cmd.SilenceUsage = true
		return fmt.Errorf("--limit must be a positive integer, got %d", noCheckLimit)
	}
	if noCheckDryRun && !noCheckTrigger {
		cmd.SilenceUsage = true
		return fmt.Errorf("--dry-run requires --trigger")
	}

	prs, err := nochecks.ListOpenPRs(noCheckRepo, noCheckLimit, noCheckBranch)
	if err != nil {
		cmd.SilenceUsage = true
		return err
	}
	eligiblePRs := 0
	for _, pr := range prs {
		if !pr.IsDraft {
			eligiblePRs++
		}
	}

	required, err := nochecks.FetchRequiredChecksByBase(cmd.Context(), noCheckRepo, prs)
	if err != nil {
		cmd.SilenceUsage = true
		return err
	}
	stuck, readErrs, err := nochecks.Scan(prs, required, nochecks.CheckRunsFor(noCheckRepo))
	if err != nil {
		cmd.SilenceUsage = true
		return err
	}

	// A per-PR check-runs read failure must not blind the whole scan; surface it
	// and keep going. Such PRs are never flagged (conservative).
	readErrNumbers := make([]int, 0, len(readErrs))
	for num := range readErrs {
		readErrNumbers = append(readErrNumbers, num)
	}
	sort.Ints(readErrNumbers)
	readErrors := make([]noCheckReadError, 0, len(readErrNumbers))
	for _, num := range readErrNumbers {
		rerr := readErrs[num]
		fmt.Fprintf(os.Stderr, "scan-no-checks: PR #%d: reading check-runs: %v\n", num, rerr)
		readErrors = append(readErrors, noCheckReadError{Number: num, Error: rerr.Error()})
	}

	items := make([]noCheckItem, 0, len(stuck))
	for _, s := range stuck {
		items = append(items, noCheckItem{StuckPR: s})
	}

	failures := 0
	actionable := 0
	if noCheckTrigger {
		for i := range items {
			outcome, err := nochecks.RetriggerCI(
				cmd.Context(), noCheckRepo, items[i].StuckPR, noCheckDryRun,
			)
			if err != nil {
				if ctxErr := cmd.Context().Err(); ctxErr != nil {
					cmd.SilenceUsage = true
					return fmt.Errorf("re-trigger canceled at PR #%d: %w", items[i].Number, ctxErr)
				}
				items[i].Error = err.Error()
				failures++
				continue
			}
			switch outcome {
			case nochecks.Retriggered:
				items[i].Triggered = true
				actionable++
			case nochecks.RetriggerWouldRun:
				actionable++
			case nochecks.RetriggerNoLongerNeeded:
				items[i].NoLongerStuck = true
			default:
				items[i].Error = fmt.Sprintf("unexpected retrigger outcome %q", outcome)
				failures++
			}
		}
	}

	result := &NoChecksScanResult{
		Repo:        noCheckRepo,
		BaseFilter:  noCheckBranch,
		OpenPRs:     len(prs),
		EligiblePRs: eligiblePRs,
		Trigger:     noCheckTrigger,
		DryRun:      noCheckDryRun,
		Stuck:       items,
		ReadErrors:  readErrors,
	}

	if err := printResult(result, func() { printNoChecksScanText(result) }); err != nil {
		return err
	}

	// Exit-gate semantics (mirrors `agm pr scan-orphaned`): a non-zero exit lets
	// supervisor tooling treat stranded PRs as an alarm without parsing stdout.
	switch {
	case len(items) == 0 && len(readErrors) == 0:
		return nil
	case !noCheckTrigger && len(items) > 0:
		cmd.SilenceUsage = true
		return fmt.Errorf("found %d open PR(s) with zero required CI check-runs — re-run with --trigger to restart CI", len(items))
	case failures > 0:
		cmd.SilenceUsage = true
		return fmt.Errorf("re-trigger validation or mutation failed for %d of %d stuck PR(s)", failures, len(items))
	case len(readErrors) > 0:
		cmd.SilenceUsage = true
		return fmt.Errorf("%d open PR(s) remain indeterminate because check-runs could not be read", len(readErrors))
	case noCheckDryRun && actionable > 0:
		cmd.SilenceUsage = true
		return fmt.Errorf("%d open PR(s) remain eligible for re-trigger (dry-run)", actionable)
	default:
		return nil
	}
}

func printNoChecksScanText(r *NoChecksScanResult) {
	fmt.Printf("\n=== No-Checks PR Scan — %s ===\n\n", r.Repo)
	if r.BaseFilter == "" {
		fmt.Println("Base filter: (all observed bases)")
	} else {
		fmt.Printf("Base filter: %s\n", r.BaseFilter)
	}
	fmt.Printf("Open PRs listed: %d\n", r.OpenPRs)
	fmt.Printf("Eligible non-draft PRs scanned: %d\n", r.EligiblePRs)
	fmt.Printf("Stuck (head SHA has 0 required CI check-runs): %d\n", len(r.Stuck))
	fmt.Printf("Indeterminate (check-runs unreadable): %d\n", len(r.ReadErrors))
	for _, readErr := range r.ReadErrors {
		fmt.Printf("  ? #%d %s\n", readErr.Number, readErr.Error)
	}

	if len(r.Stuck) == 0 {
		switch {
		case len(r.ReadErrors) > 0:
			fmt.Println("\n  (no stuck PRs proven; unreadable PRs remain indeterminate)")
		case r.EligiblePRs == 0:
			fmt.Println("\n  (none — no eligible non-draft PRs to scan)")
		default:
			fmt.Println("\n  (none — no eligible non-draft PR needs a CI re-trigger)")
		}
		fmt.Println()
		return
	}

	for _, it := range r.Stuck {
		status := "needs --trigger"
		switch {
		case it.Error != "":
			status = "re-trigger FAILED: " + it.Error
		case it.NoLongerStuck:
			status = "no longer stuck (CI appeared after scan)"
		case it.Triggered:
			status = "re-triggered"
		case r.Trigger && r.DryRun:
			status = "eligible for re-trigger at current snapshot"
		}
		fmt.Printf("\n  ✗ #%d %s\n", it.Number, it.Title)
		fmt.Printf("    base %s; head %s (%s) [%s]\n",
			it.BaseRefName, nochecks.ShortSHA(it.HeadSHA), it.HeadRefName, status)
	}
	fmt.Println()
}
