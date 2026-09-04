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
	HeadRefName string `json:"headRefName"`
	HeadRefOid  string `json:"headRefOid"`
	IsDraft     bool   `json:"isDraft"`
}

// ListOpenPRs returns up to limit open pull requests in repo (owner/name) via
// the gh CLI. It never prompts and is bounded by ghListTimeout.
func ListOpenPRs(repo string, limit int) ([]PR, error) {
	out, err := ghJSON(ghListTimeout, []string{
		"pr", "list", "--repo", repo, "--state", "open",
		"--json", "number,title,headRefName,headRefOid,isDraft",
		"--limit", fmt.Sprint(limit),
	})
	if err != nil {
		return nil, fmt.Errorf("gh pr list failed for %s: %w", repo, err)
	}
	var raw []ghPR
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, fmt.Errorf("parsing gh pr list output: %w", err)
	}
	prs := make([]PR, 0, len(raw))
	for _, r := range raw {
		prs = append(prs, PR{
			Number:      r.Number,
			Title:       r.Title,
			HeadRefName: r.HeadRefName,
			HeadSHA:     r.HeadRefOid,
			IsDraft:     r.IsDraft,
		})
	}
	return prs, nil
}

// FetchRequiredChecks returns the complete effective required-check context set
// for branch through SafeGit's shared layered-policy owner. A non-nil empty set
// means the policy is authoritatively empty; every incomplete or unsupported
// policy returns an error and no partial set. The whole two-source read shares
// one supervisor-owned deadline.
func FetchRequiredChecks(ctx context.Context, repo, branch string) (map[string]bool, error) {
	ctx, cancel := context.WithTimeout(ctx, ghAPITimeout)
	defer cancel()

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
	out, err := ghJSON(ghAPITimeout, []string{
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

// RetriggerCI pushes an empty commit onto pr's head branch via the GitHub API,
// firing the pull_request:synchronize event that restarts CI. It reuses the head
// commit's tree, so there are no file changes — only a fresh commit SHA. No
// local checkout is required, which lets a supervisor run it against any open
// PR.
//
// workflow_dispatch is deliberately not used: it would run CI against the branch
// ref, not the pull_request context, so its check-runs would not satisfy the
// PR's required checks. Only a new commit on the PR head branch produces
// check-runs in the right context.
//
// This only works when the PR head branch lives in the base repo (the agent
// workflow's same-repo branches); a fork head ref is not writable here and the
// ref PATCH fails loudly for that PR.
func RetriggerCI(repo string, pr StuckPR) error {
	treeOut, err := ghJSON(ghTriggerTimeout, []string{
		"api", fmt.Sprintf("repos/%s/git/commits/%s", repo, pr.HeadSHA), "--jq", ".tree.sha",
	})
	if err != nil {
		return fmt.Errorf("resolving head tree: %w", err)
	}
	tree := strings.TrimSpace(string(treeOut))
	if tree == "" {
		return fmt.Errorf("head commit %s has no tree sha", ShortSHA(pr.HeadSHA))
	}

	msg := fmt.Sprintf("ci: re-trigger checks for #%d (ce-np2s)", pr.Number)
	commitOut, err := ghJSON(ghTriggerTimeout, []string{
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

	if _, err := ghJSON(ghTriggerTimeout, []string{
		"api", "-X", "PATCH",
		fmt.Sprintf("repos/%s/git/refs/heads/%s", repo, pr.HeadRefName),
		"-f", "sha=" + newSHA,
	}); err != nil {
		return fmt.Errorf("fast-forwarding %s to %s: %w", pr.HeadRefName, ShortSHA(newSHA), err)
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
	if _, err := exec.LookPath("gh"); err != nil {
		return nil, fmt.Errorf("gh CLI not found on PATH: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
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
		if ctx.Err() != nil {
			return nil, fmt.Errorf("gh %s timed out after %s", label, timeout)
		}
		return nil, fmt.Errorf("gh %s: %w (%s)", label, err, strings.TrimSpace(stderr.String()))
	}
	return stdout.Bytes(), nil
}
