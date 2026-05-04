package collectors

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/vbonnet/dear-agent/pkg/aggregator"
)

// GitActivity counts commits in a lookback window and emits a single
// signal whose Subject is the repo path and whose Value is the count.
// Metadata records the lookback window in days and the branch counted
// (HEAD by default).
type GitActivity struct {
	Repo         string         // absolute path to the git work tree
	LookbackDays int            // 0 → 7
	Branch       string         // empty → HEAD
	Exec         Exec           // nil → DefaultExec
	LookPathFn   func(string) (string, error) // nil → LookPath
}

// Name implements aggregator.Collector.
func (c *GitActivity) Name() string { return "dear-agent.git" }

// Kind implements aggregator.Collector.
func (c *GitActivity) Kind() aggregator.Kind { return aggregator.KindGitActivity }

// Collect runs `git log --since=<window> --pretty=format:%H` and counts
// the resulting lines. Git is required; if absent, returns
// *aggregator.ErrToolMissing.
func (c *GitActivity) Collect(ctx context.Context) ([]aggregator.Signal, error) {
	if strings.TrimSpace(c.Repo) == "" {
		return nil, fmt.Errorf("collectors.GitActivity: empty Repo")
	}
	lookPath := c.LookPathFn
	if lookPath == nil {
		lookPath = LookPath
	}
	if _, err := lookPath("git"); err != nil {
		return nil, &aggregator.ErrToolMissing{Collector: c.Name(), Tool: "git"}
	}

	days := c.LookbackDays
	if days <= 0 {
		days = 7
	}
	branch := c.Branch
	if branch == "" {
		branch = "HEAD"
	}

	exec := c.Exec
	if exec == nil {
		exec = DefaultExec
	}
	since := fmt.Sprintf("%d days ago", days)
	out, err := exec(ctx, c.Repo, "git", "log",
		fmt.Sprintf("--since=%s", since),
		"--pretty=format:%H",
		branch)
	if err != nil {
		return nil, fmt.Errorf("collectors.GitActivity: git log: %w", err)
	}
	count := countLines(out)
	md := map[string]any{
		"lookbackDays": days,
		"branch":       branch,
	}
	mdJSON, _ := json.Marshal(md) // map of primitives can't fail to marshal
	return []aggregator.Signal{{
		Kind:     aggregator.KindGitActivity,
		Subject:  c.Repo,
		Value:    float64(count),
		Metadata: string(mdJSON),
	}}, nil
}

// countLines counts non-empty lines in b. Used by collectors that read
// line-oriented external command output.
func countLines(b []byte) int {
	if len(b) == 0 {
		return 0
	}
	s := strings.TrimRight(string(b), "\n")
	if s == "" {
		return 0
	}
	return strings.Count(s, "\n") + 1
}

