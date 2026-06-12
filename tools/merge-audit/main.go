// merge-audit checks recent merges on a GitHub repo for CI bypass or
// non-squash strategy. It fetches PRs merged within the lookback window,
// verifies all required status checks were green before merge, and flags
// merge commits with more than one parent (non-squash strategy).
//
// Usage:
//
//	merge-audit [--repo owner/repo] [--branch main] [--lookback 24h]
//
// Exit codes: 0=clean, 1=bypassed merges found, 2=usage/setup error.
//
// Reads GITHUB_TOKEN from the environment.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"time"
)

var (
	repoRe   = regexp.MustCompile(`^[A-Za-z0-9_.\-]+/[A-Za-z0-9_.\-]+$`)
	branchRe = regexp.MustCompile(`^[A-Za-z0-9_./\-]+$`)
	shaRe    = regexp.MustCompile(`^[0-9a-fA-F]{7,64}$`)
)

func main() {
	os.Exit(run())
}

func run() int {
	repoFlag := flag.String("repo", "", "GitHub repo as owner/repo (default: GITHUB_REPOSITORY env var)")
	branch := flag.String("branch", "main", "branch to audit merges on")
	lookback := flag.String("lookback", "24h", "how far back to look for merges")
	flag.Parse()

	repo, token, since, code := parseArgs(*repoFlag, *branch, *lookback)
	if code != 0 {
		return code
	}

	ctx := context.Background()

	required, err := fetchRequiredChecks(ctx, token, repo, *branch)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error fetching branch protection: %v\n", err)
		return 2
	}

	prs, err := fetchMergedPRs(ctx, token, repo, since)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error fetching merged PRs: %v\n", err)
		return 2
	}

	if len(prs) == 0 {
		fmt.Printf("ok: no PRs merged on %s/%s since %s\n", repo, *branch, since.Format(time.RFC3339))
		return 0
	}

	findings := auditPRs(ctx, token, repo, required, prs)

	if len(findings) == 0 {
		fmt.Printf("ok: %d PR(s) merged on %s/%s since %s — all clean\n",
			len(prs), repo, *branch, since.Format(time.RFC3339))
		return 0
	}

	fmt.Printf("BYPASS DETECTED: %d issue(s) on %s/%s since %s\n",
		len(findings), repo, *branch, since.Format(time.RFC3339))
	for _, f := range findings {
		fmt.Printf("\n  PR #%d: %s\n", f.PR, f.Title)
		for _, issue := range f.Issues {
			fmt.Printf("    - %s\n", issue)
		}
	}
	return 1
}

// parseArgs validates flags and environment, returning repo, token, since, exitCode.
func parseArgs(repoFlag, branch, lookback string) (repo, token string, since time.Time, code int) {
	repo = repoFlag
	if repo == "" {
		repo = os.Getenv("GITHUB_REPOSITORY")
	}
	if repo == "" {
		fmt.Fprintln(os.Stderr, "error: --repo or GITHUB_REPOSITORY must be set")
		return "", "", time.Time{}, 2
	}
	if !repoRe.MatchString(repo) {
		fmt.Fprintf(os.Stderr, "error: invalid repo %q\n", repo)
		return "", "", time.Time{}, 2
	}
	if !branchRe.MatchString(branch) {
		fmt.Fprintf(os.Stderr, "error: invalid branch %q\n", branch)
		return "", "", time.Time{}, 2
	}
	dur, err := time.ParseDuration(lookback)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: invalid lookback %q: %v\n", lookback, err)
		return "", "", time.Time{}, 2
	}
	token = os.Getenv("GITHUB_TOKEN")
	if token == "" {
		fmt.Fprintln(os.Stderr, "error: GITHUB_TOKEN must be set")
		return "", "", time.Time{}, 2
	}
	return repo, token, time.Now().UTC().Add(-dur), 0
}

// auditPRs checks each PR for CI bypass or non-squash strategy.
func auditPRs(ctx context.Context, token, repo string, required []string, prs []PullRequest) []Finding {
	var findings []Finding
	for _, pr := range prs {
		mergedAt, err := time.Parse(time.RFC3339, pr.MergedAt)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: PR #%d has unparseable merged_at %q; skipping\n", pr.Number, pr.MergedAt)
			continue
		}

		checkRuns, parentCount := fetchPRData(ctx, token, repo, pr)
		f := classifyPR(pr, required, checkRuns, parentCount, mergedAt)
		if f != nil {
			findings = append(findings, *f)
		}
	}
	return findings
}

// fetchPRData retrieves check-runs and parent count for a PR, logging warnings on error.
func fetchPRData(ctx context.Context, token, repo string, pr PullRequest) ([]CheckRun, int) {
	var checkRuns []CheckRun
	if pr.Head.SHA != "" {
		var err error
		checkRuns, err = fetchCheckRuns(ctx, token, repo, pr.Head.SHA)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: PR #%d check-runs fetch failed: %v; skipping checks\n", pr.Number, err)
		}
	}

	parentCount := 1
	if pr.MergeCommitSHA != "" {
		n, err := fetchParentCount(ctx, token, repo, pr.MergeCommitSHA)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: PR #%d parent-count fetch failed: %v; assuming squash\n", pr.Number, err)
		} else {
			parentCount = n
		}
	}
	return checkRuns, parentCount
}

// Finding describes a PR that bypassed required CI or used a non-squash strategy.
type Finding struct {
	PR     int
	Title  string
	Issues []string
}

// classifyPR returns a Finding if the PR bypassed required checks or used a
// non-squash merge strategy; returns nil when everything is in order.
func classifyPR(pr PullRequest, required []string, checkRuns []CheckRun, parentCount int, mergedAt time.Time) *Finding {
	var issues []string

	if parentCount > 1 {
		short := pr.MergeCommitSHA
		if len(short) > 8 {
			short = short[:8]
		}
		issues = append(issues, fmt.Sprintf("non-squash merge (commit %s has %d parents)", short, parentCount))
	}

	checkMap := make(map[string]CheckRun, len(checkRuns))
	for _, cr := range checkRuns {
		checkMap[cr.Name] = cr
	}

	for _, req := range required {
		issues = append(issues, classifyCheck(req, checkMap, mergedAt)...)
	}

	if len(issues) == 0 {
		return nil
	}
	return &Finding{PR: pr.Number, Title: pr.Title, Issues: issues}
}

// classifyCheck returns any findings for a single required check.
func classifyCheck(req string, checkMap map[string]CheckRun, mergedAt time.Time) []string {
	cr, ok := checkMap[req]
	if !ok {
		return []string{fmt.Sprintf("required check %q absent at merge", req)}
	}
	if cr.CompletedAt != "" {
		completedAt, err := time.Parse(time.RFC3339, cr.CompletedAt)
		if err == nil && completedAt.After(mergedAt) {
			return []string{fmt.Sprintf("required check %q completed after merge (completed %s, merged %s)",
				req, cr.CompletedAt, mergedAt.Format(time.RFC3339))}
		}
	}
	if cr.Status != "completed" {
		return []string{fmt.Sprintf("required check %q not completed at merge (status=%s)", req, cr.Status)}
	}
	if cr.Conclusion != "success" {
		return []string{fmt.Sprintf("required check %q non-success at merge (conclusion=%s)", req, cr.Conclusion)}
	}
	return nil
}

// PullRequest is a minimal view of a GitHub PR.
type PullRequest struct {
	Number         int    `json:"number"`
	Title          string `json:"title"`
	MergedAt       string `json:"merged_at"`
	MergeCommitSHA string `json:"merge_commit_sha"`
	Head           struct {
		SHA string `json:"sha"`
	} `json:"head"`
}

// CheckRun is a minimal view of a GitHub check-run.
type CheckRun struct {
	Name        string `json:"name"`
	Status      string `json:"status"`
	Conclusion  string `json:"conclusion"`
	CompletedAt string `json:"completed_at"`
}

type checkRunsResponse struct {
	CheckRuns []CheckRun `json:"check_runs"`
}

type commitResponse struct {
	Parents []struct {
		SHA string `json:"sha"`
	} `json:"parents"`
}

type branchProtectionResponse struct {
	RequiredStatusChecks *struct {
		Contexts []string `json:"contexts"`
		Checks   []struct {
			Context string `json:"context"`
		} `json:"checks"`
	} `json:"required_status_checks"`
}

func fetchRequiredChecks(ctx context.Context, token, repo, branch string) ([]string, error) {
	if !repoRe.MatchString(repo) {
		return nil, fmt.Errorf("invalid repo %q: must be owner/repo", repo)
	}
	if !branchRe.MatchString(branch) {
		return nil, fmt.Errorf("invalid branch %q", branch)
	}
	apiURL := "https://api.github.com/repos/" + repo + "/branches/" + branch + "/protection"
	body, err := githubGET(ctx, token, apiURL)
	if err != nil {
		return nil, err
	}

	var prot branchProtectionResponse
	if err := json.Unmarshal(body, &prot); err != nil {
		return nil, fmt.Errorf("parsing response: %w", err)
	}
	if prot.RequiredStatusChecks == nil {
		return nil, nil
	}

	seen := make(map[string]bool)
	var out []string
	for _, c := range prot.RequiredStatusChecks.Checks {
		if !seen[c.Context] {
			out = append(out, c.Context)
			seen[c.Context] = true
		}
	}
	for _, c := range prot.RequiredStatusChecks.Contexts {
		if !seen[c] {
			out = append(out, c)
			seen[c] = true
		}
	}
	return out, nil
}

func fetchMergedPRs(ctx context.Context, token, repo string, since time.Time) ([]PullRequest, error) {
	if !repoRe.MatchString(repo) {
		return nil, fmt.Errorf("invalid repo %q", repo)
	}
	apiURL := "https://api.github.com/repos/" + repo + "/pulls?state=closed&sort=updated&direction=desc&per_page=50"
	body, err := githubGET(ctx, token, apiURL)
	if err != nil {
		return nil, err
	}

	var prs []PullRequest
	if err := json.Unmarshal(body, &prs); err != nil {
		return nil, fmt.Errorf("parsing PRs: %w", err)
	}

	var merged []PullRequest
	for _, pr := range prs {
		if pr.MergedAt == "" {
			continue
		}
		t, err := time.Parse(time.RFC3339, pr.MergedAt)
		if err != nil {
			continue
		}
		if t.Before(since) {
			continue
		}
		merged = append(merged, pr)
	}
	return merged, nil
}

func fetchCheckRuns(ctx context.Context, token, repo, sha string) ([]CheckRun, error) {
	if !repoRe.MatchString(repo) {
		return nil, fmt.Errorf("invalid repo %q", repo)
	}
	if !isSafeSHA(sha) {
		return nil, fmt.Errorf("invalid SHA %q", sha)
	}
	apiURL := "https://api.github.com/repos/" + repo + "/commits/" + sha + "/check-runs?per_page=100"
	body, err := githubGET(ctx, token, apiURL)
	if err != nil {
		return nil, err
	}

	var resp checkRunsResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parsing check-runs: %w", err)
	}
	return resp.CheckRuns, nil
}

func fetchParentCount(ctx context.Context, token, repo, sha string) (int, error) {
	if !repoRe.MatchString(repo) {
		return 0, fmt.Errorf("invalid repo %q", repo)
	}
	if !isSafeSHA(sha) {
		return 0, fmt.Errorf("invalid SHA %q", sha)
	}
	apiURL := "https://api.github.com/repos/" + repo + "/commits/" + sha
	body, err := githubGET(ctx, token, apiURL)
	if err != nil {
		return 0, err
	}

	var commit commitResponse
	if err := json.Unmarshal(body, &commit); err != nil {
		return 0, fmt.Errorf("parsing commit: %w", err)
	}
	return len(commit.Parents), nil
}

// isSafeSHA validates that a string is a plausible git SHA (hex, 7-64 chars).
func isSafeSHA(s string) bool {
	return shaRe.MatchString(s)
}

func githubGET(ctx context.Context, token, apiURL string) ([]byte, error) {
	// All variable URL segments (repo, branch, sha) are allowlist-validated by
	// callers before reaching here. gosec G107/G704 flags the dynamic URL, but
	// the taint analysis cannot see the regex guards as sanitizers.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode == http.StatusNotFound {
		return nil, errors.New("not found (404)")
	}
	if resp.StatusCode != http.StatusOK {
		truncated := string(body)
		if len(truncated) > 200 {
			truncated = truncated[:200] + "…"
		}
		return nil, fmt.Errorf("GitHub API %d: %s", resp.StatusCode, truncated)
	}
	return body, nil
}
