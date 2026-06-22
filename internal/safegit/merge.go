// Package safegit merge.go provides the safe-merge gate that enforces CLAUDE.md
// principle 9: an atomic, vetted wrapper for PR merges that cannot be bypassed.
//
// Required before any merge:
//   - ALL CI checks pass (no red, no pending — not just required checks)
//   - No unresolved review threads exist
//   - Minimum soak: head commit is at least 5 minutes old
//   - Review bot (gemini-code-assist[bot]) has posted
//
// After merge: worktree + branch are cleaned up automatically.
package safegit

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// tracerName is the OTel tracer name for the safe-merge gate. Spans started
// under it are no-ops unless a collector is configured via
// OTEL_EXPORTER_OTLP_ENDPOINT (see pkg/otelsetup); cmd/safe-merge calls
// InitTracer to wire one up.
const tracerName = "safe-merge"

// MinSoak is the minimum age the head commit must be before merging.
const MinSoak = 5 * time.Minute

// DefaultWatchTimeout is the default time limit for watch mode.
const DefaultWatchTimeout = 45 * time.Minute

// ReviewBot is the GitHub login of the required code-review bot. GitHub appends
// a "[bot]" suffix to GitHub App identities, so the login that actually appears
// in the reviews API is "gemini-code-assist[bot]" — the bare "gemini-code-assist"
// never shows up there. The earlier bare constant caused gate 4's exact-equality
// check to never match, blocking every merge ("review bot has not posted")
// indefinitely (ce-l8by). Matching is done via isReviewBot, which tolerates the
// suffix on either side so a future schema/config drift can't re-break the gate.
const ReviewBot = "gemini-code-assist[bot]"

// isReviewBot reports whether a PR review author login is the required
// code-review bot. It compares with the "[bot]" App suffix stripped from both
// sides, so both "gemini-code-assist" and "gemini-code-assist[bot]" match — the
// suffix is a GitHub presentation detail, not part of the bot's identity.
func isReviewBot(login string) bool {
	return strings.TrimSuffix(login, "[bot]") == strings.TrimSuffix(ReviewBot, "[bot]")
}

// reviewStateCounts reports whether a review in the given state satisfies gate
// 4. A submitted review in any substantive state counts: gemini-code-assist
// posts COMMENTED reviews rather than APPROVED, so requiring APPROVED would
// deadlock the gate. Only PENDING (an unsubmitted draft) and DISMISSED reviews
// are ignored.
func reviewStateCounts(state string) bool {
	switch strings.ToUpper(strings.TrimSpace(state)) {
	case "PENDING", "DISMISSED":
		return false
	default:
		// APPROVED, COMMENTED, CHANGES_REQUESTED, or an empty/unknown state
		// (older gh JSON omitted it) all count as "the bot has posted".
		return true
	}
}

// MergeConfig holds options for a safe merge.
type MergeConfig struct {
	PRNumber      int
	Repo          string        // "owner/repo"
	Now           time.Time     // injectable for tests; zero → time.Now()
	DryRun        bool          // check gates but do not execute merge
	Watch         bool          // poll until gates pass or WatchTimeout elapses
	WatchTimeout  time.Duration // zero → DefaultWatchTimeout
	WatchInterval time.Duration // poll interval in watch mode; zero → 30s

	// ConfigPath overrides where the P4 .safe-merge.yml is read from. Empty →
	// look for ConfigFileName at the repo root; absent there → P4 gates skipped.
	ConfigPath string

	// config is the loaded P4 config (nil/empty → P4 gates skipped). flakeState
	// tracks per-check reruns across watch attempts so max_retries is honoured.
	// Both are populated by SafeMergeContext; flakeState is a reference type so
	// it survives the by-value copy into watchMerge/attemptMerge.
	config     *Config
	flakeState map[string]int
}

// AuditEntry is written to the JSONL audit log on each merge attempt.
type AuditEntry struct {
	Timestamp string `json:"timestamp"`
	PR        int    `json:"pr"`
	Repo      string `json:"repo"`
	Event     string `json:"event"` // "gate_check", "merged", "dry_run", "timeout", "error"
	Detail    string `json:"detail,omitempty"`
}

// SafeMerge gates and executes a squash-merge of the given PR.
//
// When cfg.Watch is true, it polls the gates until they pass or cfg.WatchTimeout
// elapses. When cfg.DryRun is true, it checks gates but does not execute the merge.
// Results are appended to the JSONL audit log in ~/.local/state/dear-agent/.
//
// It is a thin wrapper over SafeMergeContext with a background context, kept for
// callers that do not carry a trace context.
func SafeMerge(cfg MergeConfig) error {
	return SafeMergeContext(context.Background(), cfg)
}

// SafeMergeContext is SafeMerge with an explicit context for trace propagation.
// It opens a parent "safemerge.attempt"/"safemerge.watch" span; per-gate child
// spans (ci, threads, soak, merge) hang off it so a Jaeger trace shows exactly
// which gate blocked a merge and how long each took.
func SafeMergeContext(ctx context.Context, cfg MergeConfig) error {
	if cfg.PRNumber <= 0 {
		return fmt.Errorf("--pr must be a positive integer")
	}
	if cfg.Repo == "" {
		return fmt.Errorf("--repo is required (owner/repo format)")
	}
	if !strings.Contains(cfg.Repo, "/") {
		return fmt.Errorf("--repo must be in owner/repo format, got %q", cfg.Repo)
	}

	// Load the optional P4 config once, up front, so a malformed .safe-merge.yml
	// fails loudly before any gate runs (and isn't re-read every watch attempt).
	loaded, err := LoadConfig(repoRoot(), cfg.ConfigPath)
	if err != nil {
		return fmt.Errorf("loading %s: %w", ConfigFileName, err)
	}
	cfg.config = loaded
	cfg.flakeState = map[string]int{}

	if cfg.Watch {
		return watchMerge(ctx, cfg)
	}
	return attemptMerge(ctx, cfg)
}

// watchMerge polls the merge gates until they pass or the timeout elapses.
func watchMerge(ctx context.Context, cfg MergeConfig) error {
	timeout := cfg.WatchTimeout
	if timeout == 0 {
		timeout = DefaultWatchTimeout
	}
	interval := cfg.WatchInterval
	if interval == 0 {
		interval = 30 * time.Second
	}

	ctx, span := otel.Tracer(tracerName).Start(ctx, "safemerge.watch",
		trace.WithAttributes(
			attribute.Int("pr.number", cfg.PRNumber),
			attribute.String("pr.repo", cfg.Repo),
		))
	defer span.End()

	deadline := time.Now().Add(timeout)
	attempt := 0
	for {
		attempt++
		fmt.Fprintf(os.Stderr, "safe-merge: watch attempt %d (timeout in %s)…\n",
			attempt, time.Until(deadline).Round(time.Second))

		err := attemptMerge(ctx, cfg)
		if err == nil {
			span.SetAttributes(attribute.Int("watch.attempts", attempt))
			return nil
		}

		// If all gates passed but it was a dry-run, return immediately.
		if cfg.DryRun {
			return err
		}

		if time.Now().After(deadline) {
			appendAuditEntry(cfg.Repo, cfg.PRNumber, "timeout", err.Error())
			return fmt.Errorf("watch timeout after %s: last error: %w", timeout, err)
		}

		fmt.Fprintf(os.Stderr, "safe-merge: gates not ready (%v); retrying in %s\n", err, interval)
		time.Sleep(interval)
	}
}

// runGate executes a named merge gate inside a child span so a trace shows
// which gate ran, how long it took, and whether it blocked the merge.
func runGate(ctx context.Context, name string, fn func() error) error {
	_, span := otel.Tracer(tracerName).Start(ctx, "safemerge.gate."+name)
	defer span.End()
	err := fn()
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	return err
}

// attemptMerge checks all gates and, unless DryRun is set, executes the merge.
func attemptMerge(ctx context.Context, cfg MergeConfig) (retErr error) {
	now := cfg.Now
	if now.IsZero() {
		now = time.Now()
	}

	ctx, span := otel.Tracer(tracerName).Start(ctx, "safemerge.attempt",
		trace.WithAttributes(
			attribute.Int("pr.number", cfg.PRNumber),
			attribute.String("pr.repo", cfg.Repo),
			attribute.Bool("dry_run", cfg.DryRun),
		))
	defer func() {
		if retErr != nil {
			span.SetStatus(codes.Error, retErr.Error())
		}
		span.End()
	}()

	fmt.Fprintf(os.Stderr, "safe-merge: checking PR #%d in %s\n", cfg.PRNumber, cfg.Repo)

	// Gate 1: ALL CI checks must pass (not just required ones).
	if err := runGate(ctx, "ci", func() error { return checkAllCI(cfg.PRNumber, cfg.Repo) }); err != nil {
		// P4 flake valve: when flaky_checks are configured, give a failing
		// flaky check one sanctioned rerun before treating it as a real block.
		// The valve has the final say only when it actually finds a failing
		// check; if CI failed for another reason (e.g. still-pending checks),
		// the valve returns nil and we surface the original CI error.
		if cfg.config != nil && len(cfg.config.FlakyChecks) > 0 {
			if fvErr := runGate(ctx, "flake", func() error {
				return checkFlakeValve(cfg.PRNumber, cfg.Repo, cfg.config, cfg.flakeState)
			}); fvErr != nil {
				appendAuditEntry(cfg.Repo, cfg.PRNumber, "gate_check", "flake-valve: "+fvErr.Error())
				fmt.Fprintln(os.Stderr, "safe-merge: flake valve: "+fvErr.Error())
				return fmt.Errorf("flake-valve gate: %w", fvErr)
			}
		}
		appendAuditEntry(cfg.Repo, cfg.PRNumber, "gate_check", "CI: "+err.Error())
		fmt.Fprintf(os.Stderr, "safe-merge: guidance: run `gh pr checks %d --repo %s --watch`\n", cfg.PRNumber, cfg.Repo)
		return fmt.Errorf("CI gate: %w", err)
	}
	fmt.Fprintln(os.Stderr, "safe-merge: ✓ all CI checks pass")

	// Gate 2: no unresolved review threads.
	if err := runGate(ctx, "threads", func() error { return checkReviewThreads(cfg.PRNumber, cfg.Repo) }); err != nil {
		appendAuditEntry(cfg.Repo, cfg.PRNumber, "gate_check", "threads: "+err.Error())
		fmt.Fprintln(os.Stderr, "safe-merge: guidance: resolve open review threads; security-* threads need a written triage verdict")
		return fmt.Errorf("review-thread gate: %w", err)
	}
	fmt.Fprintln(os.Stderr, "safe-merge: ✓ no unresolved review threads")

	// Gate 3: minimum soak time + bot review.
	if err := runGate(ctx, "soak", func() error { return checkSoak(cfg.PRNumber, cfg.Repo, now) }); err != nil {
		appendAuditEntry(cfg.Repo, cfg.PRNumber, "gate_check", "soak: "+err.Error())
		return fmt.Errorf("soak gate: %w", err)
	}
	fmt.Fprintln(os.Stderr, "safe-merge: ✓ soak time and bot review OK")

	// Gate 4 (P4): expected reviewers must have a review newer than the head
	// push. Runs only when .safe-merge.yml lists expected_reviewers; otherwise
	// the built-in soak gate's bot check is the only reviewer requirement.
	if cfg.config != nil && len(cfg.config.ExpectedReviewers) > 0 {
		if err := runGate(ctx, "reviewers", func() error {
			return checkExpectedReviewers(cfg.PRNumber, cfg.Repo, cfg.config.ExpectedReviewers, now)
		}); err != nil {
			appendAuditEntry(cfg.Repo, cfg.PRNumber, "gate_check", "reviewers: "+err.Error())
			fmt.Fprintf(os.Stderr, "safe-merge: guidance: expected reviewers (%s) must review the latest push\n",
				reviewerLogins(cfg.config.ExpectedReviewers))
			return fmt.Errorf("expected-reviewer gate: %w", err)
		}
		fmt.Fprintln(os.Stderr, "safe-merge: ✓ expected reviewers have fresh reviews")
	}

	if cfg.DryRun {
		appendAuditEntry(cfg.Repo, cfg.PRNumber, "dry_run", "all gates passed; merge skipped")
		fmt.Fprintln(os.Stderr, "safe-merge: dry-run — all gates passed; no merge executed")
		return nil
	}

	// Execute the merge, inside its own span.
	_, mergeSpan := otel.Tracer(tracerName).Start(ctx, "safemerge.merge")
	defer mergeSpan.End()

	fmt.Fprintf(os.Stderr, "safe-merge: merging PR #%d (squash)…\n", cfg.PRNumber)
	headInfo, err := prHeadInfo(cfg.PRNumber, cfg.Repo)
	if err != nil {
		mergeSpan.RecordError(err)
		mergeSpan.SetStatus(codes.Error, err.Error())
		appendAuditEntry(cfg.Repo, cfg.PRNumber, "error", "cannot read PR head: "+err.Error())
		return fmt.Errorf("cannot read PR head info (needed for --match-head-commit): %w", err)
	}
	mergeSpan.SetAttributes(attribute.String("pr.head_sha", headInfo.SHA))

	mergeArgs := BuildMergeArgs(cfg.PRNumber, cfg.Repo, headInfo.SHA)
	mergeCmd := exec.Command(mergeArgs[0], mergeArgs[1:]...)
	mergeCmd.Stdout = os.Stdout
	mergeCmd.Stderr = os.Stderr
	if err := mergeCmd.Run(); err != nil {
		mergeSpan.RecordError(err)
		mergeSpan.SetStatus(codes.Error, err.Error())
		appendAuditEntry(cfg.Repo, cfg.PRNumber, "error", "merge failed: "+err.Error())
		return fmt.Errorf("gh pr merge failed: %w", err)
	}
	appendAuditEntry(cfg.Repo, cfg.PRNumber, "merged",
		fmt.Sprintf("squash merge complete (head=%s)", headInfo.SHA))
	fmt.Fprintln(os.Stderr, "safe-merge: ✓ merge complete")

	// Post-merge cleanup (best-effort, non-fatal).
	if headInfo.Branch != "" {
		cleanupWorktree(headInfo.Branch)
	}
	return nil
}

// appendAuditEntry writes a single JSON line to the safe-merge audit log.
// Failures are silently ignored so audit-log issues never block merges.
func appendAuditEntry(repo string, prNum int, event, detail string) {
	dir := auditLogDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return
	}
	f, err := os.OpenFile(filepath.Join(dir, "safe-merge-audit.jsonl"),
		os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return
	}
	// Closing a writable handle can surface deferred write errors (flush
	// failures, full disk). The audit log is non-fatal — never block a
	// merge on it — but a dropped close error means a silently corrupt log,
	// so log it via slog rather than discarding it.
	defer func() {
		if cerr := f.Close(); cerr != nil {
			slog.Warn("safe-merge: failed to close audit log", "error", cerr)
		}
	}()

	entry := AuditEntry{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		PR:        prNum,
		Repo:      repo,
		Event:     event,
		Detail:    detail,
	}
	b, _ := json.Marshal(entry)
	if _, werr := fmt.Fprintln(f, string(b)); werr != nil {
		slog.Warn("safe-merge: failed to write audit log entry", "error", werr)
	}
}

// auditLogDir returns the directory for the safe-merge audit log.
// Overridable via SAFE_MERGE_AUDIT_DIR environment variable for testing.
func auditLogDir() string {
	if d := os.Getenv("SAFE_MERGE_AUDIT_DIR"); d != "" {
		return d
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "/tmp"
	}
	return filepath.Join(home, ".local", "state", "dear-agent")
}

// runCommand executes cmd, capturing stdout. On failure, stderr is appended to
// the error so callers get actionable context instead of a bare exit-code.
func runCommand(cmd *exec.Cmd) ([]byte, error) {
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		if se := strings.TrimSpace(stderr.String()); se != "" {
			return nil, fmt.Errorf("%w\nstderr: %s", err, se)
		}
		return nil, err
	}
	return out, nil
}

// BuildMergeArgs returns the full argv for the gh pr merge command, including
// the TOCTOU-preventing --match-head-commit flag. Exported so tests can verify
// the flag is always present — its removal in PR #460 caused a P1 regression.
// Panics if headSHA is empty — the caller must resolve it first.
func BuildMergeArgs(prNum int, repo, headSHA string) []string {
	if headSHA == "" {
		panic("BuildMergeArgs: headSHA must not be empty — TOCTOU anchor requires a resolved SHA")
	}
	return []string{
		"gh", "pr", "merge",
		fmt.Sprintf("%d", prNum),
		"--repo", repo,
		"--squash",
		"--delete-branch",
		"--match-head-commit", headSHA,
	}
}

// --- gate implementations ---

type checkRun struct {
	Name  string `json:"name"`
	State string `json:"state"`
}

type requiredStatusChecksResponse struct {
	Checks []struct {
		Context string `json:"context"`
	} `json:"checks"`
}

// fetchRequiredChecks returns the set of required status check names for the
// main branch. If the API call fails, an empty set is returned so the caller
// falls back to validating all checks (fail-strict).
func fetchRequiredChecks(repo string) map[string]bool {
	parts := strings.SplitN(repo, "/", 2)
	if len(parts) != 2 {
		return nil
	}
	out, err := runCommand(exec.Command("gh", "api",
		fmt.Sprintf("repos/%s/%s/branches/main/protection/required_status_checks", parts[0], parts[1]),
	))
	if err != nil {
		return nil
	}
	var resp requiredStatusChecksResponse
	if err := json.Unmarshal(out, &resp); err != nil {
		return nil
	}
	required := make(map[string]bool, len(resp.Checks))
	for _, c := range resp.Checks {
		required[c.Context] = true
	}
	return required
}

// checkAllCI verifies that all required CI checks on the PR have passed.
// It fetches the branch-protection required check list and only gates on those,
// so non-required checks that fail fleet-wide (e.g. Doc Proximity Check) do
// not block merges. If the required check list cannot be fetched, all checks
// are validated (fail-strict fallback).
func checkAllCI(prNum int, repo string) error {
	out, err := runCommand(exec.Command("gh", "pr", "checks",
		fmt.Sprintf("%d", prNum),
		"--repo", repo,
		"--json", "name,state",
	))
	if err != nil {
		return fmt.Errorf("gh pr checks failed: %w", err)
	}

	required := fetchRequiredChecks(repo)
	if len(required) > 0 {
		// Filter to only required checks before validating.
		var allChecks []checkRun
		if err := json.Unmarshal(out, &allChecks); err != nil {
			return fmt.Errorf("parsing check output: %w", err)
		}
		var requiredOnly []checkRun
		seen := make(map[string]bool)
		for _, c := range allChecks {
			if required[c.Name] {
				requiredOnly = append(requiredOnly, c)
				seen[c.Name] = true
			}
		}
		// Required checks not yet reported count as pending (fail-safe).
		for req := range required {
			if !seen[req] {
				requiredOnly = append(requiredOnly, checkRun{Name: req, State: "pending"})
			}
		}
		b, _ := json.Marshal(requiredOnly)
		return parseCheckRuns(b)
	}
	return parseCheckRuns(out)
}

// parseCheckRuns validates that every check in the JSON input has passed.
// The input is expected to already be filtered to the desired set (required
// checks only in production; all checks in unit tests). Exported for testing.
func parseCheckRuns(data []byte) error {
	var checks []checkRun
	if err := json.Unmarshal(data, &checks); err != nil {
		return fmt.Errorf("parsing check output: %w", err)
	}

	var failing []string
	var pending []string
	for _, c := range checks {
		switch strings.ToLower(c.State) {
		case "success", "pass", "neutral", "skipping", "skipped":
			// acceptable
		case "pending", "queued", "in_progress", "waiting", "requested":
			pending = append(pending, c.Name)
		default:
			failing = append(failing, fmt.Sprintf("%s (%s)", c.Name, c.State))
		}
	}
	if len(failing) > 0 {
		return fmt.Errorf("checks failed: %s", strings.Join(failing, ", "))
	}
	if len(pending) > 0 {
		return fmt.Errorf("checks still pending: %s", strings.Join(pending, ", "))
	}
	return nil
}

const reviewThreadsQuery = `
query($owner: String!, $name: String!, $number: Int!) {
  repository(owner: $owner, name: $name) {
    pullRequest(number: $number) {
      reviewThreads(first: 100) {
        nodes {
          isResolved
          isOutdated
          comments(first: 1) {
            nodes {
              body
            }
          }
        }
      }
    }
  }
}`

type gqlReviewResult struct {
	Data struct {
		Repository struct {
			PullRequest struct {
				ReviewThreads struct {
					Nodes []struct {
						IsResolved bool `json:"isResolved"`
						IsOutdated bool `json:"isOutdated"`
						Comments   struct {
							Nodes []struct {
								Body string `json:"body"`
							} `json:"nodes"`
						} `json:"comments"`
					} `json:"nodes"`
				} `json:"reviewThreads"`
			} `json:"pullRequest"`
		} `json:"repository"`
	} `json:"data"`
}

func checkReviewThreads(prNum int, repo string) error {
	parts := strings.SplitN(repo, "/", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid repo %q", repo)
	}
	owner, name := parts[0], parts[1]

	out, err := runCommand(exec.Command("gh", "api", "graphql",
		"--field", fmt.Sprintf("owner=%s", owner),
		"--field", fmt.Sprintf("name=%s", name),
		"--field", fmt.Sprintf("number=%d", prNum),
		"--field", fmt.Sprintf("query=%s", reviewThreadsQuery),
	))
	if err != nil {
		return fmt.Errorf("GraphQL query failed: %w", err)
	}
	return parseReviewThreads(out)
}

// parseReviewThreads returns an error if any unresolved, non-outdated review
// threads exist in the GraphQL response. Exported for testing.
func parseReviewThreads(data []byte) error {
	var result gqlReviewResult
	if err := json.Unmarshal(data, &result); err != nil {
		return fmt.Errorf("parsing GraphQL response: %w", err)
	}

	var unresolved []string
	for i, t := range result.Data.Repository.PullRequest.ReviewThreads.Nodes {
		if t.IsResolved || t.IsOutdated {
			continue
		}
		label := fmt.Sprintf("thread #%d", i+1)
		if len(t.Comments.Nodes) > 0 {
			body := t.Comments.Nodes[0].Body
			if len(body) > 60 {
				body = body[:60] + "…"
			}
			label = fmt.Sprintf("thread #%d: %q", i+1, body)
		}
		unresolved = append(unresolved, label)
	}

	if len(unresolved) > 0 {
		return fmt.Errorf("%d unresolved review thread(s):\n  %s",
			len(unresolved), strings.Join(unresolved, "\n  "))
	}
	return nil
}

type prViewResult struct {
	HeadRefOid string `json:"headRefOid"`
	Commits    []struct {
		CommittedDate time.Time `json:"committedDate"`
	} `json:"commits"`
	Reviews []struct {
		Author struct {
			Login string `json:"login"`
		} `json:"author"`
		State string `json:"state"`
	} `json:"reviews"`
}

func checkSoak(prNum int, repo string, now time.Time) error {
	out, err := runCommand(exec.Command("gh", "pr", "view",
		fmt.Sprintf("%d", prNum),
		"--repo", repo,
		"--json", "headRefOid,commits,reviews",
	))
	if err != nil {
		return fmt.Errorf("gh pr view failed: %w", err)
	}
	return parseSoak(out, now)
}

// parseSoak checks the soak constraints from the gh pr view JSON output.
// Exported for testing.
func parseSoak(data []byte, now time.Time) error {
	var pr prViewResult
	if err := json.Unmarshal(data, &pr); err != nil {
		return fmt.Errorf("parsing pr view: %w", err)
	}

	// Find the most recent commit time.
	var newest time.Time
	for _, c := range pr.Commits {
		if c.CommittedDate.After(newest) {
			newest = c.CommittedDate
		}
	}

	if !newest.IsZero() {
		age := now.Sub(newest)
		if age < MinSoak {
			remaining := MinSoak - age
			return fmt.Errorf("head commit is only %s old (need ≥%s); retry in %s",
				age.Round(time.Second), MinSoak, remaining.Round(time.Second))
		}
	}

	// Check review bot posted. Any submitted review counts — gemini-code-assist
	// posts COMMENTED reviews (it does not APPROVE), so gating on state ==
	// "APPROVED" would never pass; we only require that a review from the bot
	// exists. COMMENTED, APPROVED, and CHANGES_REQUESTED all satisfy gate 4.
	botPosted := false
	for _, r := range pr.Reviews {
		// isReviewBot tolerates the "[bot]" App suffix on either side, and
		// reviewStateCounts ignores PENDING/DISMISSED so only a submitted bot
		// review (COMMENTED/APPROVED/CHANGES_REQUESTED) satisfies gate 4.
		if isReviewBot(r.Author.Login) && reviewStateCounts(r.State) {
			botPosted = true
			break
		}
	}
	if !botPosted {
		return fmt.Errorf("review bot (%s) has not posted a review yet", ReviewBot)
	}

	return nil
}

type prHeadResult struct {
	SHA    string
	Branch string
}

func prHeadInfo(prNum int, repo string) (prHeadResult, error) {
	out, err := runCommand(exec.Command("gh", "pr", "view",
		fmt.Sprintf("%d", prNum),
		"--repo", repo,
		"--json", "headRefName,headRefOid",
	))
	if err != nil {
		return prHeadResult{}, err
	}
	var raw struct {
		HeadRefName string `json:"headRefName"`
		HeadRefOid  string `json:"headRefOid"`
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		return prHeadResult{}, err
	}
	if raw.HeadRefOid == "" {
		return prHeadResult{}, fmt.Errorf("PR #%d has no headRefOid — cannot anchor merge", prNum)
	}
	return prHeadResult{SHA: raw.HeadRefOid, Branch: raw.HeadRefName}, nil
}

// cleanupWorktree removes any local worktrees tracking the given branch and
// then deletes the local branch. This mirrors the post-merge cleanup in
// CLAUDE.md §5. Failures are printed as warnings, not returned as errors.
func cleanupWorktree(branch string) {
	// List worktrees in JSON format.
	out, err := exec.Command("git", "worktree", "list", "--porcelain").Output()
	if err != nil {
		fmt.Fprintf(os.Stderr, "safe-merge: cleanup: git worktree list: %v\n", err)
		return
	}

	// Parse porcelain output: blocks separated by blank lines.
	// Each block has "worktree <path>", optionally "branch refs/heads/<branch>".
	// The very first worktree block is always the main worktree — never remove it.
	var toRemove []string
	var mainWorktree, currentPath string
	for line := range strings.SplitSeq(string(out), "\n") {
		line = strings.TrimSpace(line)
		if after, ok := strings.CutPrefix(line, "worktree "); ok {
			currentPath = after
			if mainWorktree == "" {
				mainWorktree = currentPath
			}
		} else if line == "branch refs/heads/"+branch && currentPath != "" {
			if currentPath != mainWorktree {
				toRemove = append(toRemove, currentPath)
			}
			currentPath = ""
		}
	}

	for _, path := range toRemove {
		fmt.Fprintf(os.Stderr, "safe-merge: cleanup: removing worktree %s\n", path)
		if out, err := exec.Command("git", "worktree", "remove", path).CombinedOutput(); err != nil {
			fmt.Fprintf(os.Stderr, "safe-merge: cleanup: worktree remove %s: %v: %s\n", path, err, out)
		}
	}

	// Delete the local branch if it exists.
	if out, err := exec.Command("git", "branch", "-d", branch).CombinedOutput(); err != nil {
		// -d refuses to delete if unmerged; that's fine — we already merged.
		// Suppress the error if it's just "branch not found".
		if !strings.Contains(string(out), "not found") {
			fmt.Fprintf(os.Stderr, "safe-merge: cleanup: branch -d %s: %s\n", branch, strings.TrimSpace(string(out)))
		}
	} else {
		fmt.Fprintf(os.Stderr, "safe-merge: cleanup: removed local branch %s\n", branch)
	}
}
