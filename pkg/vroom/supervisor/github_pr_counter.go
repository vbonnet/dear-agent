package supervisor

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"time"
)

// GitHubOpenPRCounter implements OpenPRCounter by shelling out to the GitHub
// CLI (`gh pr list`). It is the production counter behind the open-PR
// backpressure valve (ce-qpg9): the Overseer queries it each burndown tick to
// decide whether the PR queue is too deep to keep dispatching workers.
//
// Draft PRs are excluded from the count — they are not in the merge pipeline
// and so do not contribute to the firehose. This matches cmd/babysit-prs,
// which also skips drafts when deciding what to merge.
type GitHubOpenPRCounter struct {
	// Repo is the "owner/name" slug passed to `gh --repo`. When empty, gh
	// infers the repo from the current directory's git remote.
	Repo string

	// Limit caps how many PRs gh enumerates. It must comfortably exceed the
	// expected open-PR count or the count saturates and the cap can never
	// trip. Defaults to 200 when non-positive.
	Limit int

	// Timeout bounds each `gh pr list` invocation. Defaults to 30s when zero.
	Timeout time.Duration
}

// ghPR is the minimal PR record decoded from `gh pr list --json`.
type ghPR struct {
	Number  int  `json:"number"`
	IsDraft bool `json:"isDraft"`
}

// OpenPRs implements OpenPRCounter by listing open, non-draft PRs via gh.
func (c *GitHubOpenPRCounter) OpenPRs(ctx context.Context) (int, error) {
	limit := c.Limit
	if limit <= 0 {
		limit = 200
	}
	timeout := c.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}

	args := []string{
		"pr", "list",
		"--state", "open",
		"--json", "number,isDraft",
		"--limit", fmt.Sprintf("%d", limit),
	}
	if c.Repo != "" {
		args = append(args, "--repo", c.Repo)
	}

	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(cctx, "gh", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if cctx.Err() != nil {
			return 0, fmt.Errorf("gh pr list timed out after %s", timeout)
		}
		return 0, fmt.Errorf("gh pr list: %w (stderr: %s)", err, stderr.String())
	}

	return countOpenNonDraft(stdout.Bytes())
}

// countOpenNonDraft decodes `gh pr list --json number,isDraft` output and
// returns the number of non-draft PRs. Split out from OpenPRs so the
// draft-exclusion contract is unit-testable without invoking gh.
func countOpenNonDraft(jsonOut []byte) (int, error) {
	var prs []ghPR
	if err := json.Unmarshal(jsonOut, &prs); err != nil {
		return 0, fmt.Errorf("parse gh output: %w", err)
	}
	count := 0
	for _, pr := range prs {
		if !pr.IsDraft {
			count++
		}
	}
	return count, nil
}
