package nochecks

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/vbonnet/dear-agent/internal/safegit"
)

// All gh calls are hard-bounded and prompt-disabled so the scan is safe to run
// from supervisor tooling without ever hanging on auth or network.
const (
	ghListTimeout    = 30 * time.Second
	ghAPITimeout     = 20 * time.Second
	ghTriggerTimeout = 20 * time.Second
)

// ghPR mirrors the gh pr list --json fields we request.
type ghPR struct {
	Number      int    `json:"number"`
	Title       string `json:"title"`
	BaseRefName string `json:"baseRefName"`
	HeadRefName string `json:"headRefName"`
	HeadRefOid  string `json:"headRefOid"`
	IsDraft     *bool  `json:"isDraft"`
}

// retriggerSnapshot is the provider state that must still match the scan
// immediately before RetriggerCI is allowed to mutate the PR head.
type retriggerSnapshot struct {
	Number           int
	State            string
	IsDraft          *bool
	BaseRefName      string
	BaseRepoID       int64
	BaseRepoFullName string
	HeadRefName      string
	HeadSHA          string
	HeadRepoID       int64
	HeadRepoFullName string
}

// validatedRetriggerTarget can only be constructed after a current provider
// snapshot matches the scan. Keeping the mutation helper typed to this value
// prevents future callers from accidentally bypassing the revalidation seam.
type validatedRetriggerTarget struct {
	repo        string
	number      int
	headRefName string
	headSHA     string
	treeSHA     string
}

// RetriggerOutcome describes the current result of one candidate revalidation.
type RetriggerOutcome string

const (
	// Retriggered means the empty commit and non-force ref update succeeded.
	Retriggered RetriggerOutcome = "retriggered"
	// RetriggerWouldRun means a dry-run candidate passed current validation.
	RetriggerWouldRun RetriggerOutcome = "would_retrigger"
	// RetriggerNoLongerNeeded means current checks show that CI self-healed.
	RetriggerNoLongerNeeded RetriggerOutcome = "no_longer_needed"
)

// ListOpenPRs returns up to limit open pull requests in repo (owner/name) via
// the gh CLI. When baseFilter is non-empty, it is an explicit provider-side
// filter; every returned row must still carry that same observed base. It
// never prompts and is bounded by ghListTimeout.
func ListOpenPRs(repo string, limit int, baseFilter string) ([]PR, error) {
	args := []string{
		"pr", "list", "--repo", repo, "--state", "open",
		"--json", "number,title,baseRefName,headRefName,headRefOid,isDraft",
		"--limit", fmt.Sprint(limit),
	}
	if baseFilter != "" {
		args = append(args, "--base", baseFilter)
	}
	out, err := ghJSON(ghListTimeout, args)
	if err != nil {
		return nil, fmt.Errorf("gh pr list failed for %s: %w", repo, err)
	}
	var raw []ghPR
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, fmt.Errorf("parsing gh pr list output: %w", err)
	}
	prs := make([]PR, 0, len(raw))
	for _, r := range raw {
		if r.IsDraft == nil {
			return nil, fmt.Errorf("gh pr list returned PR #%d without a known draft state", r.Number)
		}
		if baseFilter != "" && r.BaseRefName != baseFilter {
			return nil, fmt.Errorf(
				"gh pr list returned PR #%d base %q outside requested base %q",
				r.Number, r.BaseRefName, baseFilter,
			)
		}
		prs = append(prs, PR{
			Number:      r.Number,
			Title:       r.Title,
			BaseRefName: r.BaseRefName,
			HeadRefName: r.HeadRefName,
			HeadSHA:     r.HeadRefOid,
			IsDraft:     *r.IsDraft,
		})
	}
	return prs, nil
}

// fetchRequiredChecks reads one branch through SafeGit's shared layered-policy
// owner. The multi-base constructor owns the total deadline and is the only
// exported way to obtain policy for a scan.
func fetchRequiredChecks(ctx context.Context, repo, branch string) (map[string]bool, error) {
	required, err := safegit.RequiredCheckNamesForBranch(ctx, repo, branch)
	if err != nil {
		return nil, fmt.Errorf("reading effective required checks for %s branch %s: %w", repo, branch, err)
	}
	return required, nil
}

// CheckRunNamesForRef returns every page of check-runs reported against sha in
// repo. An empty slice (the stuck case) is a valid, non-error result; a non-nil
// error means the complete read failed. It satisfies CheckRunsFunc once bound
// to a repo.
func CheckRunNamesForRef(repo, sha string) ([]CheckRun, error) {
	return checkRunNamesForRef(context.Background(), repo, sha)
}

func checkRunNamesForRef(ctx context.Context, repo, sha string) ([]CheckRun, error) {
	out, err := ghJSONContext(ctx, ghAPITimeout, []string{
		"api", fmt.Sprintf("repos/%s/commits/%s/check-runs?per_page=100", repo, sha), "--paginate",
		"--jq", ".check_runs[].name",
	})
	if err != nil {
		return nil, err
	}
	var runs []CheckRun
	for line := range strings.SplitSeq(strings.TrimSpace(string(out)), "\n") {
		if name := strings.TrimSpace(line); name != "" {
			runs = append(runs, CheckRun{Name: name})
		}
	}
	return runs, nil
}

// CheckRunsFor binds repo to a CheckRunsFunc for Scan.
func CheckRunsFor(repo string) CheckRunsFunc {
	return func(sha string) ([]CheckRun, error) { return CheckRunNamesForRef(repo, sha) }
}

// RetriggerCI re-reads the current pull request and, only if its eligibility and
// base/head identity still match the scan snapshot, pushes an empty commit onto
// its head branch via the GitHub API. That fires the pull_request:synchronize
// event that restarts CI. It reuses the head commit's tree, so there are no file
// changes — only a fresh commit SHA. No local checkout is required, which lets
// a supervisor run it against any open PR.
//
// workflow_dispatch is deliberately not used: it would run CI against the branch
// ref, not the pull_request context, so its check-runs would not satisfy the
// PR's required checks. Only a new commit on the PR head branch produces
// check-runs in the right context.
//
// This only works when the PR head branch lives in the target repo (the agent
// workflow's same-repo branches). Forks and any observed state/base/head drift
// fail the preflight before a commit or ref mutation is attempted. A dry run
// crosses the same preflight and returns before any Git-object read or write.
// The final provider read narrows, but cannot atomically close, the interval
// before the later GitHub mutations.
func RetriggerCI(
	ctx context.Context,
	repo string,
	pr StuckPR,
	dryRun bool,
) (RetriggerOutcome, error) {
	if pr.requiredChecks == nil {
		return "", fmt.Errorf("retrigger preflight for PR #%d: required-check policy is missing", pr.Number)
	}
	if err := ctx.Err(); err != nil {
		return "", fmt.Errorf("retrigger preflight for PR #%d: caller context ended: %w", pr.Number, err)
	}

	var tree string
	if !dryRun {
		var err error
		tree, err = readHeadTree(ctx, repo, pr.HeadSHA)
		if err != nil {
			return "", err
		}
	}

	runs, err := checkRunNamesForRef(ctx, repo, pr.HeadSHA)
	if err != nil {
		return "", fmt.Errorf("retrigger preflight for PR #%d: re-reading check-runs: %w", pr.Number, err)
	}
	current, err := readRetriggerSnapshot(ctx, repo, pr.Number)
	if err != nil {
		return "", fmt.Errorf("retrigger preflight for PR #%d: reading current pull request: %w", pr.Number, err)
	}
	target, err := newValidatedRetriggerTarget(repo, pr, current)
	if err != nil {
		return "", err
	}
	if !NeedsRetrigger(PR{}, runs, pr.requiredChecks) {
		return RetriggerNoLongerNeeded, nil
	}
	if dryRun {
		return RetriggerWouldRun, nil
	}
	target.treeSHA = tree
	if err := retriggerValidatedCI(ctx, target); err != nil {
		return "", err
	}
	return Retriggered, nil
}

func readHeadTree(ctx context.Context, repo, headSHA string) (string, error) {
	treeOut, err := ghJSONContext(ctx, ghTriggerTimeout, []string{
		"api", fmt.Sprintf("repos/%s/git/commits/%s", repo, headSHA), "--jq", ".tree.sha",
	})
	if err != nil {
		return "", fmt.Errorf("resolving head tree: %w", err)
	}
	tree := strings.TrimSpace(string(treeOut))
	if tree == "" {
		return "", fmt.Errorf("head commit %s has no tree sha", ShortSHA(headSHA))
	}
	return tree, nil
}

func readRetriggerSnapshot(ctx context.Context, repo string, number int) (retriggerSnapshot, error) {
	out, err := ghJSONContext(ctx, ghTriggerTimeout, []string{
		"api", fmt.Sprintf("repos/%s/pulls/%d", repo, number),
	})
	if err != nil {
		return retriggerSnapshot{}, err
	}

	var raw struct {
		Number int    `json:"number"`
		State  string `json:"state"`
		Draft  *bool  `json:"draft"`
		Base   struct {
			Ref  string `json:"ref"`
			Repo *struct {
				ID       int64  `json:"id"`
				FullName string `json:"full_name"`
			} `json:"repo"`
		} `json:"base"`
		Head struct {
			Ref  string `json:"ref"`
			SHA  string `json:"sha"`
			Repo *struct {
				ID       int64  `json:"id"`
				FullName string `json:"full_name"`
			} `json:"repo"`
		} `json:"head"`
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		return retriggerSnapshot{}, fmt.Errorf("parsing current pull request: %w", err)
	}

	current := retriggerSnapshot{
		Number:      raw.Number,
		State:       raw.State,
		IsDraft:     raw.Draft,
		BaseRefName: raw.Base.Ref,
		HeadRefName: raw.Head.Ref,
		HeadSHA:     raw.Head.SHA,
	}
	if raw.Base.Repo != nil {
		current.BaseRepoID = raw.Base.Repo.ID
		current.BaseRepoFullName = raw.Base.Repo.FullName
	}
	if raw.Head.Repo != nil {
		current.HeadRepoID = raw.Head.Repo.ID
		current.HeadRepoFullName = raw.Head.Repo.FullName
	}
	return current, nil
}

func validateRetriggerSnapshot(repo string, scanned StuckPR, current retriggerSnapshot) error {
	_, err := newValidatedRetriggerTarget(repo, scanned, current)
	return err
}

func newValidatedRetriggerTarget(
	repo string,
	scanned StuckPR,
	current retriggerSnapshot,
) (validatedRetriggerTarget, error) {
	preflight := func(format string, args ...any) (validatedRetriggerTarget, error) {
		return validatedRetriggerTarget{}, fmt.Errorf(
			"retrigger preflight for PR #%d: "+format,
			append([]any{scanned.Number}, args...)...,
		)
	}

	if current.Number != scanned.Number {
		return preflight("provider returned PR #%d", current.Number)
	}
	if current.State != "open" {
		return preflight("state is %q, expected open", current.State)
	}
	if current.IsDraft == nil {
		return preflight("draft state is missing")
	}
	if *current.IsDraft {
		return preflight("pull request is now a draft")
	}
	if current.BaseRefName != scanned.BaseRefName {
		return preflight("base changed from %q to %q", scanned.BaseRefName, current.BaseRefName)
	}
	if current.HeadRefName != scanned.HeadRefName {
		return preflight("head ref changed from %q to %q", scanned.HeadRefName, current.HeadRefName)
	}
	if current.HeadSHA != scanned.HeadSHA {
		return preflight("head SHA changed from %q to %q", scanned.HeadSHA, current.HeadSHA)
	}
	if current.HeadRefName == "" || current.HeadSHA == "" {
		return preflight("head ref or SHA is missing")
	}
	if current.BaseRepoID <= 0 || current.HeadRepoID <= 0 {
		return preflight(
			"base/head repository identity is missing (base=%d, head=%d)",
			current.BaseRepoID,
			current.HeadRepoID,
		)
	}
	if current.BaseRepoID != current.HeadRepoID {
		return preflight(
			"head repository id %d differs from base repository id %d (fork)",
			current.HeadRepoID,
			current.BaseRepoID,
		)
	}
	if !strings.EqualFold(current.BaseRepoFullName, repo) {
		return preflight(
			"base repository %q is not target repository %q",
			current.BaseRepoFullName,
			repo,
		)
	}
	if !strings.EqualFold(current.HeadRepoFullName, repo) {
		return preflight(
			"head repository %q is not target repository %q (fork)",
			current.HeadRepoFullName,
			repo,
		)
	}

	return validatedRetriggerTarget{
		repo:        repo,
		number:      scanned.Number,
		headRefName: scanned.HeadRefName,
		headSHA:     scanned.HeadSHA,
	}, nil
}

func retriggerValidatedCI(ctx context.Context, target validatedRetriggerTarget) error {
	if target.treeSHA == "" {
		return fmt.Errorf("validated retrigger target for PR #%d has no tree sha", target.number)
	}

	msg := fmt.Sprintf("ci: re-trigger checks for #%d (ce-np2s)", target.number)
	commitOut, err := ghJSONContext(ctx, ghTriggerTimeout, []string{
		"api", fmt.Sprintf("repos/%s/git/commits", target.repo),
		"-f", "message=" + msg,
		"-f", "tree=" + target.treeSHA,
		"-f", "parents[]=" + target.headSHA,
		"--jq", ".sha",
	})
	if err != nil {
		return fmt.Errorf("creating empty commit: %w", err)
	}
	newSHA := strings.TrimSpace(string(commitOut))
	if newSHA == "" {
		return fmt.Errorf("empty-commit creation returned no sha")
	}

	if _, err := ghJSONContext(ctx, ghTriggerTimeout, []string{
		"api", "-X", "PATCH",
		fmt.Sprintf("repos/%s/git/refs/heads/%s", target.repo, target.headRefName),
		"-f", "sha=" + newSHA,
		"-F", "force=false",
	}); err != nil {
		return fmt.Errorf("fast-forwarding %s to %s: %w", target.headRefName, ShortSHA(newSHA), err)
	}
	return nil
}

// ShortSHA truncates a commit SHA to its 7-char prefix for display.
func ShortSHA(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}

// ghJSON runs a bounded, prompt-disabled gh command and returns stdout.
func ghJSON(timeout time.Duration, args []string) ([]byte, error) {
	return ghJSONContext(context.Background(), timeout, args)
}

func ghJSONContext(parent context.Context, timeout time.Duration, args []string) ([]byte, error) {
	if _, err := exec.LookPath("gh"); err != nil {
		return nil, fmt.Errorf("gh CLI not found on PATH: %w", err)
	}
	if parent == nil {
		return nil, fmt.Errorf("gh caller context is nil")
	}

	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "gh", args...)
	cmd.Env = append(os.Environ(),
		"GIT_TERMINAL_PROMPT=0",
		"GH_PROMPT_DISABLED=1",
		"GH_NO_UPDATE_NOTIFIER=1",
	)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		label := strings.Join(args, " ")
		if len(args) > 2 {
			label = strings.Join(args[:2], " ")
		}
		if parent.Err() != nil {
			return nil, fmt.Errorf("gh %s stopped by caller context: %w", label, parent.Err())
		}
		if ctx.Err() != nil {
			return nil, fmt.Errorf("gh %s timed out after %s", label, timeout)
		}
		return nil, fmt.Errorf("gh %s: %w (%s)", label, err, strings.TrimSpace(stderr.String()))
	}
	return stdout.Bytes(), nil
}
