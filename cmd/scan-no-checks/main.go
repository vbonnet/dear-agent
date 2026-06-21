// Command scan-no-checks finds open pull requests whose head commit has zero
// required CI check-runs and, optionally, re-triggers CI on them.
//
// # Why this exists
//
// safe-pr arms squash auto-merge on every PR it creates, on the assumption that
// CI will run, report required checks, and let the PR merge itself once green.
// A push-then-PR-open race can drop the CI trigger: the head SHA ends up with
// no check-runs at all, so the required checks never report. Auto-merge then
// waits forever and the safe-merge babysit loop skips the PR as "pending" on
// every pass — there is no red, no green, and no signal. This is exactly how
// PRs #579/#581/#582 aged 8+ hours with zero CI before anyone noticed (bead
// ce-np2s). This command closes that detection gap.
//
// # What it does
//
//  1. Lists open, non-draft PRs.
//  2. Reads the branch-protection required check set once.
//  3. For each PR, fetches the check-runs on its head SHA and flags the PR when
//     none of the required checks has a run (see internal/prchecks).
//  4. Reports the stuck PRs (table or --json).
//  5. With --trigger, re-triggers CI on each stuck PR by pushing an empty commit
//     to its head branch via the GitHub API (no local checkout required), which
//     fires the pull_request:synchronize event that CI listens on.
//
// Re-trigger is headless on purpose: workflow_dispatch would run CI against the
// branch ref, not the pull_request context, so its check-runs would not satisfy
// the PR's required checks. Only a new commit on the PR head branch produces
// check-runs in the right context. The empty commit reuses the head commit's
// tree, so it changes nothing about the code.
//
// Exit status: 0 when no PR is stuck, or when every stuck PR was re-triggered
// successfully under --trigger. Non-zero when stuck PRs are found and left
// un-triggered (detector/alarm mode) or when a re-trigger fails — so a cron can
// treat a non-zero exit as "stranded PRs need attention".
//
// Usage:
//
//	scan-no-checks [--repo owner/name] [--limit N] [--trigger] [--dry-run] [--json]
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/vbonnet/dear-agent/internal/prchecks"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "\nscan-no-checks: %v\n", err)
		os.Exit(1)
	}
}

type config struct {
	repo    string
	limit   int
	trigger bool
	dryRun  bool
	jsonOut bool
}

func parseFlags(argv []string) (config, error) {
	var cfg config
	fs := flag.NewFlagSet("scan-no-checks", flag.ContinueOnError)
	fs.StringVar(&cfg.repo, "repo", "", "GitHub repo owner/name (auto-detected from git remote if omitted)")
	fs.IntVar(&cfg.limit, "limit", 100, "maximum number of open PRs to scan")
	fs.BoolVar(&cfg.trigger, "trigger", false, "re-trigger CI on each stuck PR via an empty commit")
	fs.BoolVar(&cfg.dryRun, "dry-run", false, "with --trigger, report what would be re-triggered without pushing")
	fs.BoolVar(&cfg.jsonOut, "json", false, "emit machine-readable JSON instead of a table")
	if err := fs.Parse(argv); err != nil {
		return cfg, err
	}
	if cfg.limit < 1 {
		return cfg, fmt.Errorf("--limit must be a positive integer, got %d", cfg.limit)
	}
	if fs.NArg() > 0 {
		return cfg, fmt.Errorf("unexpected argument %q", fs.Arg(0))
	}
	return cfg, nil
}

func run(argv []string) error {
	cfg, err := parseFlags(argv)
	if err != nil {
		return err
	}
	if cfg.repo == "" {
		cfg.repo, err = detectRepo()
		if err != nil {
			return fmt.Errorf("cannot detect GitHub repo: %w\nhint: pass --repo owner/name", err)
		}
	}
	return scan(context.Background(), cfg, os.Stdout)
}

// result is one stuck PR and the outcome of any re-trigger attempt.
type result struct {
	Number      int    `json:"number"`
	Title       string `json:"title"`
	HeadRefName string `json:"headRefName"`
	HeadSHA     string `json:"headSHA"`
	Triggered   bool   `json:"triggered"`
	Error       string `json:"error,omitempty"`
}

func scan(ctx context.Context, cfg config, out io.Writer) error {
	prs, err := listOpenPRs(ctx, cfg.repo, cfg.limit)
	if err != nil {
		return fmt.Errorf("listing open PRs: %w", err)
	}
	required := fetchRequiredChecks(ctx, cfg.repo)

	var stuck []result
	for _, pr := range prs {
		runs, err := fetchCheckRuns(ctx, cfg.repo, pr.HeadSHA)
		if err != nil {
			// A check-runs read failure for one PR must not blind the whole
			// scan; surface it and move on.
			fmt.Fprintf(os.Stderr, "scan-no-checks: PR #%d: reading check-runs: %v\n", pr.Number, err)
			continue
		}
		if !prchecks.NeedsRetrigger(pr, runs, required) {
			continue
		}
		stuck = append(stuck, result{
			Number: pr.Number, Title: pr.Title,
			HeadRefName: pr.HeadRefName, HeadSHA: pr.HeadSHA,
		})
	}

	failures := 0
	if cfg.trigger {
		for i := range stuck {
			r := &stuck[i]
			pr := prchecks.PR{Number: r.Number, HeadRefName: r.HeadRefName, HeadSHA: r.HeadSHA}
			if cfg.dryRun {
				continue
			}
			if err := retriggerCI(ctx, cfg.repo, pr); err != nil {
				r.Error = err.Error()
				failures++
				continue
			}
			r.Triggered = true
		}
	}

	if err := report(out, cfg, stuck); err != nil {
		return err
	}

	switch {
	case len(stuck) == 0:
		return nil
	case !cfg.trigger:
		return fmt.Errorf("%d open PR(s) have zero required CI check-runs; re-run with --trigger to restart CI", len(stuck))
	case cfg.dryRun:
		return fmt.Errorf("%d open PR(s) would be re-triggered (dry-run)", len(stuck))
	case failures > 0:
		return fmt.Errorf("re-trigger failed for %d of %d stuck PR(s)", failures, len(stuck))
	default:
		return nil
	}
}

// report writes the stuck-PR findings to out as a table or, with --json, as a
// JSON array.
func report(out io.Writer, cfg config, stuck []result) error {
	if cfg.jsonOut {
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		if stuck == nil {
			stuck = []result{}
		}
		return enc.Encode(stuck)
	}
	if len(stuck) == 0 {
		fmt.Fprintln(out, "scan-no-checks: no stuck PRs — every open PR has required CI check-runs")
		return nil
	}
	fmt.Fprintf(out, "scan-no-checks: %d PR(s) with zero required CI check-runs:\n", len(stuck))
	for _, r := range stuck {
		status := "needs --trigger"
		switch {
		case r.Error != "":
			status = "re-trigger FAILED: " + r.Error
		case r.Triggered:
			status = "re-triggered"
		case cfg.dryRun:
			status = "would re-trigger"
		}
		fmt.Fprintf(out, "  #%-5d %-40.40s %s [%s]\n", r.Number, r.Title, shortSHA(r.HeadSHA), status)
	}
	return nil
}

func shortSHA(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}

// retriggerCI pushes an empty commit onto pr's head branch via the GitHub API,
// firing the pull_request:synchronize event that restarts CI. It reuses the
// head commit's tree, so no file changes — only a fresh commit SHA. No local
// checkout is required, which lets a cron run it against any open PR.
//
// This only works when the PR head branch lives in the base repo (the agent
// workflow's same-repo branches); a fork head ref is not writable here and the
// ref PATCH fails loudly for that PR without aborting the scan.
func retriggerCI(ctx context.Context, repo string, pr prchecks.PR) error {
	treeOut, err := ghJSON(ctx, 20*time.Second, []string{
		"api", fmt.Sprintf("repos/%s/git/commits/%s", repo, pr.HeadSHA), "--jq", ".tree.sha",
	})
	if err != nil {
		return fmt.Errorf("resolving head tree: %w", err)
	}
	tree := strings.TrimSpace(string(treeOut))
	if tree == "" {
		return fmt.Errorf("head commit %s has no tree sha", shortSHA(pr.HeadSHA))
	}

	msg := fmt.Sprintf("ci: re-trigger checks for #%d (ce-np2s)", pr.Number)
	commitOut, err := ghJSON(ctx, 20*time.Second, []string{
		"api", fmt.Sprintf("repos/%s/git/commits", repo),
		"-f", "message=" + msg,
		"-f", "tree=" + tree,
		"-f", "parents[]=" + pr.HeadSHA,
		"--jq", ".sha",
	})
	if err != nil {
		return fmt.Errorf("creating empty commit: %w", err)
	}
	newSHA := strings.TrimSpace(string(commitOut))
	if newSHA == "" {
		return fmt.Errorf("empty-commit creation returned no sha")
	}

	if _, err := ghJSON(ctx, 20*time.Second, []string{
		"api", "-X", "PATCH",
		fmt.Sprintf("repos/%s/git/refs/heads/%s", repo, pr.HeadRefName),
		"-f", "sha=" + newSHA,
	}); err != nil {
		return fmt.Errorf("fast-forwarding %s to %s: %w", pr.HeadRefName, shortSHA(newSHA), err)
	}
	return nil
}

// ghPR mirrors the gh pr list --json fields we request.
type ghPR struct {
	Number      int    `json:"number"`
	Title       string `json:"title"`
	HeadRefName string `json:"headRefName"`
	HeadRefOid  string `json:"headRefOid"`
	IsDraft     bool   `json:"isDraft"`
}

func listOpenPRs(ctx context.Context, repo string, limit int) ([]prchecks.PR, error) {
	args := []string{
		"pr", "list", "--state", "open",
		"--json", "number,title,headRefName,headRefOid,isDraft",
		"--limit", fmt.Sprint(limit),
	}
	if repo != "" {
		args = append(args, "--repo", repo)
	}
	out, err := ghJSON(ctx, 30*time.Second, args)
	if err != nil {
		return nil, err
	}
	var raw []ghPR
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, fmt.Errorf("parsing gh pr list: %w", err)
	}
	prs := make([]prchecks.PR, 0, len(raw))
	for _, r := range raw {
		prs = append(prs, prchecks.PR{
			Number: r.Number, Title: r.Title,
			HeadRefName: r.HeadRefName, HeadSHA: r.HeadRefOid, IsDraft: r.IsDraft,
		})
	}
	return prs, nil
}

// fetchCheckRuns returns the check-runs reported against sha. An empty slice
// (the stuck case) is a valid, non-error result.
func fetchCheckRuns(ctx context.Context, repo, sha string) ([]prchecks.CheckRun, error) {
	out, err := ghJSON(ctx, 20*time.Second, []string{
		"api", fmt.Sprintf("repos/%s/commits/%s/check-runs", repo, sha),
		"--jq", ".check_runs[].name",
	})
	if err != nil {
		return nil, err
	}
	var runs []prchecks.CheckRun
	for line := range strings.SplitSeq(strings.TrimSpace(string(out)), "\n") {
		name := strings.TrimSpace(line)
		if name == "" {
			continue
		}
		runs = append(runs, prchecks.CheckRun{Name: name})
	}
	return runs, nil
}

// fetchRequiredChecks returns the branch-protection required check names for
// main. On any error it returns nil, which prchecks.NeedsRetrigger treats as
// "fall back to any-check-run" — conservative, never over-flagging.
func fetchRequiredChecks(ctx context.Context, repo string) map[string]bool {
	if repo == "" {
		return nil
	}
	out, err := ghJSON(ctx, 20*time.Second, []string{
		"api", fmt.Sprintf("repos/%s/branches/main/protection/required_status_checks", repo),
		"--jq", ".contexts[]",
	})
	if err != nil {
		return nil
	}
	set := map[string]bool{}
	for line := range strings.SplitSeq(strings.TrimSpace(string(out)), "\n") {
		name := strings.TrimSpace(line)
		if name != "" {
			set[name] = true
		}
	}
	if len(set) == 0 {
		return nil
	}
	return set
}

func ghJSON(ctx context.Context, timeout time.Duration, args []string) ([]byte, error) {
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(cctx, "gh", args...)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GH_PROMPT_DISABLED=1")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		label := strings.Join(args, " ")
		if len(args) > 2 {
			label = strings.Join(args[:2], " ")
		}
		if cctx.Err() != nil {
			return nil, fmt.Errorf("gh %s timed out after %s", label, timeout)
		}
		return nil, fmt.Errorf("gh %s: %w (%s)", label, err, strings.TrimSpace(stderr.String()))
	}
	return stdout.Bytes(), nil
}

// parseRepoFromURL extracts "owner/repo" from a GitHub remote URL, handling
// both HTTPS and SSH forms with or without a trailing ".git".
func parseRepoFromURL(rawURL string) (string, error) {
	rawURL = strings.TrimSuffix(strings.TrimSpace(rawURL), ".git")
	for _, prefix := range []string{"github.com/", "github.com:"} {
		if idx := strings.LastIndex(rawURL, prefix); idx >= 0 {
			return rawURL[idx+len(prefix):], nil
		}
	}
	return "", fmt.Errorf("cannot parse GitHub repo from remote URL: %q", rawURL)
}

func detectRepo() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "git", "remote", "get-url", "origin").Output()
	if err != nil {
		return "", err
	}
	return parseRepoFromURL(string(out))
}
