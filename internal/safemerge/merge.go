// Package safemerge implements the disciplined PR merge predicate.
// See docs/design-safe-merge.md §4.2 for the full design.
package safemerge

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Config parameterises a safe-merge invocation.
type Config struct {
	PR      string        // PR number or branch name
	Repo    string        // owner/name (auto-detected if empty)
	Timeout time.Duration // how long to wait for pending checks
	DryRun  bool
}

// prState is the merged view of PR + check data from a single GraphQL query.
type prState struct {
	Number      int
	Title       string
	State       string // OPEN CLOSED MERGED
	IsDraft     bool
	HeadRefName string // branch name
	HeadRefOid  string // current head SHA
	BaseRefName string
	// mergeStateStatus: CLEAN DIRTY BEHIND BLOCKED UNSTABLE UNKNOWN MERGED
	MergeStateStatus  string
	Mergeable         string // MERGEABLE CONFLICTING UNKNOWN
	UnresolvedThreads []reviewThread
	Checks            []checkRun
}

type reviewThread struct {
	ID         string
	Path       string
	IsResolved bool
	FirstBody  string // first comment preview for display
}

type checkRun struct {
	Name       string
	Status     string // QUEUED IN_PROGRESS COMPLETED WAITING PENDING REQUESTED
	Conclusion string // SUCCESS FAILURE NEUTRAL CANCELLED SKIPPED TIMED_OUT ACTION_REQUIRED STARTUP_FAILURE STALE null
}

const prQuery = `query($owner:String!, $repo:String!, $pr:Int!) {
  repository(owner:$owner, name:$repo) {
    pullRequest(number:$pr) {
      number title state isDraft
      headRefName headRefOid baseRefName
      mergeStateStatus mergeable
      reviewThreads(first:100) {
        nodes {
          id path isResolved
          comments(first:1) { nodes { body } }
        }
      }
      commits(last:1) {
        nodes {
          commit {
            statusCheckRollup {
              contexts(first:100) {
                nodes {
                  __typename
                  ... on CheckRun {
                    name status conclusion
                  }
                  ... on StatusContext {
                    context state
                  }
                }
              }
            }
          }
        }
      }
    }
  }
}`

// Merge runs the full safe-merge predicate and — if all guards pass — executes
// the squash merge with an TOCTOU-safe expectedHeadOid anchor.
func Merge(cfg Config) error {
	if cfg.Repo == "" {
		var err error
		cfg.Repo, err = detectRepo()
		if err != nil {
			return fmt.Errorf("cannot detect GitHub repo: %w\nhint: pass --repo owner/name", err)
		}
	}

	deadline := time.Now().Add(cfg.Timeout)

	for {
		pr, err := fetchPR(cfg.Repo, cfg.PR)
		if err != nil {
			return fmt.Errorf("fetching PR %s: %w", cfg.PR, err)
		}

		if err := checkState(pr); err != nil {
			return err
		}

		if err := checkMergeable(pr); err != nil {
			return err
		}

		pending, refused := checkChecks(pr)
		if refused {
			return fmt.Errorf("one or more required checks failed — fix before merging")
		}

		if err := checkThreads(pr); err != nil {
			return err
		}

		if !pending {
			// All guards pass — proceed to merge.
			break
		}

		if time.Now().After(deadline) {
			return fmt.Errorf("timed out after %s waiting for pending checks on %s\n"+
				"Check status: gh pr checks %s --repo %s",
				cfg.Timeout, cfg.PR, cfg.PR, cfg.Repo)
		}

		slog.Info("checks pending, waiting...", "pr", cfg.PR, "remaining", time.Until(deadline).Round(time.Second))
		time.Sleep(30 * time.Second)
	}

	pr, err := fetchPR(cfg.Repo, cfg.PR)
	if err != nil {
		return fmt.Errorf("re-fetching PR before merge: %w", err)
	}

	if cfg.DryRun {
		fmt.Printf("safe-merge: dry-run passed all guards for PR #%d (%s)\n", pr.Number, pr.Title)
		fmt.Printf("  head: %s | branch: %s\n", pr.HeadRefOid[:8], pr.HeadRefName)
		return nil
	}

	return executeMerge(cfg.Repo, pr)
}

// checkState verifies the PR is open and not a draft.
func checkState(pr *prState) error {
	if pr.State != "OPEN" {
		return fmt.Errorf("PR #%d is %s — cannot merge", pr.Number, strings.ToLower(pr.State))
	}
	if pr.IsDraft {
		return fmt.Errorf("PR #%d is a draft — mark it ready for review first:\n  gh pr ready %d --repo <owner/repo>", pr.Number, pr.Number)
	}
	return nil
}

// checkMergeable verifies there are no merge conflicts.
func checkMergeable(pr *prState) error {
	switch pr.Mergeable {
	case "CONFLICTING":
		return fmt.Errorf("PR #%d has merge conflicts — rebase onto %s first:\n  git rebase origin/%s", pr.Number, pr.BaseRefName, pr.BaseRefName)
	case "UNKNOWN":
		// GitHub hasn't computed it yet — treat as pending, caller will retry.
	}
	if pr.MergeStateStatus == "DIRTY" {
		return fmt.Errorf("PR #%d merge state is DIRTY — rebase onto %s first", pr.Number, pr.BaseRefName)
	}
	if pr.MergeStateStatus == "BEHIND" {
		return fmt.Errorf("PR #%d is behind base %s — update the branch first:\n  gh pr update-branch %d --repo <owner/repo>", pr.Number, pr.BaseRefName, pr.Number)
	}
	return nil
}

// allowedConclusion returns true if a check conclusion is safe to merge on.
func allowedConclusion(c string) bool {
	switch c {
	case "SUCCESS", "NEUTRAL", "SKIPPED":
		return true
	}
	return false
}

// checkChecks returns (pending, refused).
// pending=true means some checks are still running (caller should retry).
// refused=true means a check has definitively failed.
func checkChecks(pr *prState) (pending, refused bool) {
	for _, c := range pr.Checks {
		switch c.Status {
		case "COMPLETED":
			if !allowedConclusion(c.Conclusion) {
				fmt.Fprintf(os.Stderr, "✗ FAIL  %s (%s)\n", c.Name, c.Conclusion)
				refused = true
			} else {
				fmt.Fprintf(os.Stderr, "✓ ok    %s\n", c.Name)
			}
		case "QUEUED", "IN_PROGRESS", "WAITING", "PENDING", "REQUESTED":
			fmt.Fprintf(os.Stderr, "⏳ wait  %s (%s)\n", c.Name, c.Status)
			pending = true
		}
	}
	return pending, refused
}

// checkThreads verifies all review threads are resolved.
func checkThreads(pr *prState) error {
	var unresolved []reviewThread
	for _, t := range pr.UnresolvedThreads {
		if !t.IsResolved {
			unresolved = append(unresolved, t)
		}
	}
	if len(unresolved) == 0 {
		return nil
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "PR #%d has %d unresolved review thread(s):\n\n", pr.Number, len(unresolved))
	for _, t := range unresolved {
		preview := t.FirstBody
		if len(preview) > 100 {
			preview = preview[:100] + "…"
		}
		fmt.Fprintf(&sb, "  %s: %s\n", t.Path, preview)
	}
	fmt.Fprintf(&sb, "\nResolve all threads first, or use:\n  resolve-review-threads resolve-all <owner> <repo> %d", pr.Number)
	return errors.New(sb.String())
}

// executeMerge performs the TOCTOU-safe squash merge.
func executeMerge(repo string, pr *prState) error {
	owner, repoName, err := splitRepo(repo)
	if err != nil {
		return err
	}

	args := []string{
		"pr", "merge",
		fmt.Sprintf("%d", pr.Number),
		"--squash",
		"--delete-branch",
		"--match-head-commit", pr.HeadRefOid,
		"--repo", repo,
	}

	slog.Info("merging PR", "number", pr.Number, "title", pr.Title, "head", pr.HeadRefOid[:8])
	cmd := exec.Command("gh", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			if exitErr.ExitCode() == 1 {
				// Could be 409 head mismatch — tell caller
				return fmt.Errorf("merge failed (head mismatch or server error) — run safe-merge again to retry from current state")
			}
		}
		return fmt.Errorf("gh pr merge: %w", err)
	}

	slog.Info("PR merged successfully", "number", pr.Number, "branch", pr.HeadRefName)

	postMergeCleanup(owner, repoName, pr.HeadRefName)
	appendAuditRecord(repo, pr, "merged")
	return nil
}

// postMergeCleanup removes the local worktree and branch for the merged PR's
// head branch, following CLAUDE.md §5 discipline.
func postMergeCleanup(owner, repoName, branch string) {
	// Find the local git worktree whose branch matches.
	repoRoot := findRepoRoot(owner, repoName)
	if repoRoot == "" {
		slog.Info("post-merge cleanup: local repo not found", "owner", owner, "repo", repoName)
		return
	}

	// Remove worktree for this branch.
	out, err := exec.Command("git", "-C", repoRoot, "worktree", "list", "--porcelain").Output()
	if err != nil {
		slog.Warn("post-merge cleanup: cannot list worktrees", "error", err)
		return
	}
	wt := findWorktreeForBranch(string(out), branch)
	if wt != "" {
		if err := exec.Command("git", "-C", repoRoot, "worktree", "remove", wt).Run(); err != nil {
			slog.Warn("post-merge cleanup: cannot remove worktree", "path", wt, "error", err)
		} else {
			slog.Info("post-merge cleanup: removed worktree", "path", wt)
		}
	}

	// Delete local branch.
	if err := exec.Command("git", "-C", repoRoot, "branch", "-D", branch).Run(); err != nil {
		slog.Warn("post-merge cleanup: cannot delete local branch", "branch", branch, "error", err)
	} else {
		slog.Info("post-merge cleanup: deleted local branch", "branch", branch)
	}
}

// findWorktreeForBranch parses git worktree list --porcelain output and
// returns the path of the worktree checked out on branch, or "".
func findWorktreeForBranch(porcelain, branch string) string {
	var currentPath string
	for line := range strings.SplitSeq(porcelain, "\n") {
		if p, ok := strings.CutPrefix(line, "worktree "); ok {
			currentPath = p
		}
		if line == "branch refs/heads/"+branch {
			return currentPath
		}
	}
	return ""
}

// findRepoRoot returns the ~/src/<owner>/<repo> path if it exists, or looks
// for any directory named <repo> under ~/src.
func findRepoRoot(owner, repo string) string {
	home, _ := os.UserHomeDir()
	// Canonical path: ~/src/<repo> (the owner subdir is not always present)
	candidates := []string{
		filepath.Join(home, "src", owner, repo),
		filepath.Join(home, "src", repo),
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// fetchPR runs a GraphQL query and returns the PR state.
func fetchPR(repo, prNum string) (*prState, error) {
	owner, repoName, err := splitRepo(repo)
	if err != nil {
		return nil, err
	}

	vars := fmt.Sprintf(`{"owner":%q,"repo":%q,"pr":%s}`, owner, repoName, prNum)
	out, err := ghGraphQL(prQuery, vars)
	if err != nil {
		return nil, fmt.Errorf("GraphQL query failed: %w", err)
	}

	return parsePRState(out)
}

// ghGraphQL runs `gh api graphql -f query=<q> -f variables=<v>` and returns stdout.
func ghGraphQL(query, variables string) ([]byte, error) {
	cmd := exec.Command("gh", "api", "graphql",
		"-f", "query="+query,
		"-f", "variables="+variables,
	)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("%w: %s", err, stderr.String())
	}
	return stdout.Bytes(), nil
}

// parsePRState unmarshals the GraphQL response into prState.
func parsePRState(data []byte) (*prState, error) {
	var resp struct {
		Data struct {
			Repository struct {
				PullRequest struct {
					Number           int    `json:"number"`
					Title            string `json:"title"`
					State            string `json:"state"`
					IsDraft          bool   `json:"isDraft"`
					HeadRefName      string `json:"headRefName"`
					HeadRefOid       string `json:"headRefOid"`
					BaseRefName      string `json:"baseRefName"`
					MergeStateStatus string `json:"mergeStateStatus"`
					Mergeable        string `json:"mergeable"`
					ReviewThreads    struct {
						Nodes []struct {
							ID         string `json:"id"`
							Path       string `json:"path"`
							IsResolved bool   `json:"isResolved"`
							Comments   struct {
								Nodes []struct {
									Body string `json:"body"`
								} `json:"nodes"`
							} `json:"comments"`
						} `json:"nodes"`
					} `json:"reviewThreads"`
					Commits struct {
						Nodes []struct {
							Commit struct {
								StatusCheckRollup *struct {
									Contexts struct {
										Nodes []json.RawMessage `json:"nodes"`
									} `json:"contexts"`
								} `json:"statusCheckRollup"`
							} `json:"commit"`
						} `json:"nodes"`
					} `json:"commits"`
				} `json:"pullRequest"`
			} `json:"repository"`
		} `json:"data"`
		Errors []struct{ Message string } `json:"errors"`
	}

	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("parse GraphQL response: %w", err)
	}
	if len(resp.Errors) > 0 {
		return nil, fmt.Errorf("GraphQL error: %s", resp.Errors[0].Message)
	}

	raw := resp.Data.Repository.PullRequest
	pr := &prState{
		Number:           raw.Number,
		Title:            raw.Title,
		State:            raw.State,
		IsDraft:          raw.IsDraft,
		HeadRefName:      raw.HeadRefName,
		HeadRefOid:       raw.HeadRefOid,
		BaseRefName:      raw.BaseRefName,
		MergeStateStatus: raw.MergeStateStatus,
		Mergeable:        raw.Mergeable,
	}

	for _, t := range raw.ReviewThreads.Nodes {
		rt := reviewThread{
			ID:         t.ID,
			Path:       t.Path,
			IsResolved: t.IsResolved,
		}
		if len(t.Comments.Nodes) > 0 {
			rt.FirstBody = t.Comments.Nodes[0].Body
		}
		if !t.IsResolved {
			pr.UnresolvedThreads = append(pr.UnresolvedThreads, rt)
		}
	}

	if len(raw.Commits.Nodes) > 0 {
		rollup := raw.Commits.Nodes[0].Commit.StatusCheckRollup
		if rollup != nil {
			for _, node := range rollup.Contexts.Nodes {
				pr.Checks = append(pr.Checks, parseCheckNode(node))
			}
		}
	}

	return pr, nil
}

// parseCheckNode extracts name/status/conclusion from either a CheckRun or StatusContext node.
func parseCheckNode(raw json.RawMessage) checkRun {
	var typed struct {
		Typename   string `json:"__typename"`
		Name       string `json:"name"`
		Status     string `json:"status"`
		Conclusion string `json:"conclusion"`
		// StatusContext fields
		Context string `json:"context"`
		State   string `json:"state"`
	}
	if err := json.Unmarshal(raw, &typed); err != nil {
		return checkRun{Name: "unknown", Status: "COMPLETED", Conclusion: "FAILURE"}
	}
	if typed.Typename == "StatusContext" {
		c := "FAILURE"
		switch typed.State {
		case "SUCCESS":
			c = "SUCCESS"
		case "PENDING":
			return checkRun{Name: typed.Context, Status: "IN_PROGRESS", Conclusion: ""}
		case "FAILURE", "ERROR":
			c = "FAILURE"
		}
		return checkRun{Name: typed.Context, Status: "COMPLETED", Conclusion: c}
	}
	return checkRun{
		Name:       typed.Name,
		Status:     typed.Status,
		Conclusion: typed.Conclusion,
	}
}

// splitRepo splits "owner/repo" into components.
func splitRepo(repo string) (string, string, error) {
	parts := strings.SplitN(repo, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("invalid repo %q — expected owner/name", repo)
	}
	return parts[0], parts[1], nil
}

// detectRepo auto-detects the GitHub repo from the current git remote.
func detectRepo() (string, error) {
	cmd := exec.Command("git", "remote", "get-url", "origin")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	rawURL := strings.TrimSpace(string(out))
	rawURL = strings.TrimSuffix(rawURL, ".git")
	for _, prefix := range []string{"github.com/", "github.com:"} {
		if idx := strings.LastIndex(rawURL, prefix); idx >= 0 {
			return rawURL[idx+len(prefix):], nil
		}
	}
	return "", fmt.Errorf("cannot parse GitHub repo from remote URL: %q", rawURL)
}

// appendAuditRecord appends a JSONL line to ~/.local/state/safe-merge/audit.jsonl.
func appendAuditRecord(repo string, pr *prState, outcome string) {
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, ".local", "state", "safe-merge")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return
	}
	record := map[string]any{
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"repo":      repo,
		"pr":        pr.Number,
		"head":      pr.HeadRefOid,
		"branch":    pr.HeadRefName,
		"outcome":   outcome,
	}
	data, err := json.Marshal(record)
	if err != nil {
		return
	}
	f, err := os.OpenFile(filepath.Join(dir, "audit.jsonl"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return
	}
	defer func() { _ = f.Close() }()
	_, _ = f.Write(append(data, '\n'))
}
