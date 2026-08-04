// Package safegit merge.go provides the safe-merge gate that enforces AGENTS.md
// principle 9: an atomic, vetted wrapper for PR merges that cannot be bypassed.
//
// Required before any merge:
//   - All effective required CI checks pass (discovery failure gates every
//     reported check rather than weakening the boundary)
//   - No unresolved review threads exist
//   - Minimum soak: head commit is at least 5 minutes old
//
// After merge: worktree + branch are cleaned up automatically.
package safegit

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
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

const (
	// cleanupCommandTimeout bounds each best-effort local Git cleanup operation.
	// Cleanup runs after provider merge success, so it must never hold the caller
	// indefinitely even when a local worktree or Git process is unhealthy.
	cleanupCommandTimeout = 30 * time.Second
	// cleanupCommandWaitDelay bounds pipe draining when a Git descendant keeps
	// stdout or stderr open after the direct Git process exits.
	cleanupCommandWaitDelay = time.Second
)

// DefaultWatchTimeout is the default time limit for watch mode.
const DefaultWatchTimeout = 45 * time.Minute

// MergeConfig holds options for a safe merge.
type MergeConfig struct {
	PRNumber      int
	Repo          string        // "owner/repo"
	Now           time.Time     // injectable for tests; zero → time.Now()
	DryRun        bool          // check gates but do not execute merge
	Watch         bool          // poll until gates pass or WatchTimeout elapses
	WatchTimeout  time.Duration // zero → DefaultWatchTimeout
	WatchInterval time.Duration // poll interval in watch mode; zero → 30s

	// SkipReviewCheck bypasses the unresolved-thread gate. Its use is audited.
	// Intended only for emergency merges where manual thread resolution is not
	// possible. Prefer resolving threads via resolve-review-threads instead.
	SkipReviewCheck bool

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

	// Gate 1: every provider-effective required CI check must pass.
	if err := runGate(ctx, "ci", func() error { return checkAllCIContext(ctx, cfg.PRNumber, cfg.Repo) }); err != nil {
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
	fmt.Fprintln(os.Stderr, "safe-merge: ✓ all provider-required CI checks pass")

	// Gate 2: no unresolved review threads.
	if cfg.SkipReviewCheck {
		appendAuditEntry(cfg.Repo, cfg.PRNumber, "gate_check", "threads: SKIPPED via --skip-review-check")
		fmt.Fprintln(os.Stderr, "safe-merge: ⚠ review-thread gate skipped (--skip-review-check)")
	} else {
		if err := runGate(ctx, "threads", func() error { return checkReviewThreads(cfg.PRNumber, cfg.Repo) }); err != nil {
			appendAuditEntry(cfg.Repo, cfg.PRNumber, "gate_check", "threads: "+err.Error())
			parts := strings.SplitN(cfg.Repo, "/", 2)
			owner, repoName := parts[0], parts[1]
			fmt.Fprintf(os.Stderr,
				"safe-merge: guidance: resolve threads, then re-run safe-merge, or use:\n  resolve-review-threads resolve-all %s %s %d [author]\n",
				owner, repoName, cfg.PRNumber)
			return fmt.Errorf("review-thread gate: %w", err)
		}
		fmt.Fprintln(os.Stderr, "safe-merge: ✓ no unresolved review threads")
	}

	// Gate 3: minimum soak time + bot review.
	if err := runGate(ctx, "soak", func() error { return checkSoak(cfg.PRNumber, cfg.Repo, now) }); err != nil {
		appendAuditEntry(cfg.Repo, cfg.PRNumber, "gate_check", "soak: "+err.Error())
		return fmt.Errorf("soak gate: %w", err)
	}
	fmt.Fprintln(os.Stderr, "safe-merge: ✓ soak time OK")

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
	confirm := func() error { return confirmMerged(cfg.PRNumber, cfg.Repo, headInfo.SHA) }
	var confirmationErr error
	if cfg.Watch {
		// watchMerge owns retry timing and the overall deadline.
		confirmationErr = confirm()
	} else {
		// A one-shot invocation must still wait for an accepted auto-merge to
		// finish; success from `gh pr merge --auto` only means it was queued.
		confirmationErr = waitForMergeCompletion(ctx, cfg.WatchTimeout, cfg.WatchInterval, confirm)
	}
	if confirmationErr != nil {
		mergeSpan.RecordError(confirmationErr)
		mergeSpan.SetStatus(codes.Error, confirmationErr.Error())
		appendAuditEntry(cfg.Repo, cfg.PRNumber, "merge_pending", confirmationErr.Error())
		return confirmationErr
	}
	appendAuditEntry(cfg.Repo, cfg.PRNumber, "merged",
		fmt.Sprintf("squash merge complete (head=%s)", headInfo.SHA))
	fmt.Fprintln(os.Stderr, "safe-merge: ✓ merge complete")

	// Post-merge cleanup (best-effort, non-fatal).
	if headInfo.Branch != "" {
		cleanupWorktree(ctx, headInfo.Branch)
	}
	return nil
}

type mergeResult struct {
	State      string `json:"state"`
	HeadRefOid string `json:"headRefOid"`
}

var errMergePending = errors.New("merge is pending")

// confirmMerged prevents a successful `gh pr merge --auto` invocation from
// being mistaken for a completed merge. GitHub exits zero when auto-merge is
// merely enabled, so cleanup is safe only after the PR reports MERGED at the
// exact head SHA that passed the gates.
func confirmMerged(prNum int, repo, expectedHeadSHA string) error {
	out, err := runCommand(exec.Command("gh", "pr", "view",
		fmt.Sprintf("%d", prNum), "--repo", repo,
		"--json", "state,headRefOid"))
	if err != nil {
		return fmt.Errorf("confirming merge completion: %w", err)
	}

	var result mergeResult
	if err := json.Unmarshal(out, &result); err != nil {
		return fmt.Errorf("parsing merge completion state: %w", err)
	}
	return validateMergeResult(result, expectedHeadSHA)
}

func validateMergeResult(result mergeResult, expectedHeadSHA string) error {
	if result.HeadRefOid != expectedHeadSHA {
		return fmt.Errorf("merge completion head changed: expected %s, got %s",
			expectedHeadSHA, result.HeadRefOid)
	}
	if result.State != "MERGED" {
		return fmt.Errorf("%w: auto-merge accepted but PR remains %s; waiting for GitHub to complete it",
			errMergePending,
			strings.ToLower(result.State))
	}
	return nil
}

func waitForMergeCompletion(ctx context.Context, timeout, interval time.Duration, check func() error) error {
	if timeout == 0 {
		timeout = DefaultWatchTimeout
	}
	if interval == 0 {
		interval = 30 * time.Second
	}
	deadline := time.Now().Add(timeout)
	for {
		err := check()
		if err == nil {
			return nil
		}
		if !errors.Is(err, errMergePending) {
			return err
		}
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return fmt.Errorf("merge completion timeout after %s: %w", timeout, err)
		}
		wait := min(interval, remaining)
		select {
		case <-ctx.Done():
			return fmt.Errorf("waiting for merge completion: %w", ctx.Err())
		case <-time.After(wait):
		}
	}
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
	return runCommandAllowExitCodes(cmd, nil)
}

// runCheckCommand accepts gh's documented check-status exits while still
// requiring a valid JSON response. gh pr checks exits 1 for failed checks and
// 8 for pending checks; neither means the query itself failed.
func runCheckCommand(cmd *exec.Cmd) ([]byte, error) {
	return runCommandAllowExitCodes(cmd, map[int]bool{1: true, 8: true})
}

func runCommandAllowExitCodes(cmd *exec.Cmd, allowed map[int]bool) ([]byte, error) {
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && allowed[exitErr.ExitCode()] && json.Valid(out) {
			return out, nil
		}
		if se := strings.TrimSpace(stderr.String()); se != "" {
			return nil, fmt.Errorf("%w\nstderr: %s", err, se)
		}
		return nil, err
	}
	return out, nil
}

// BuildMergeArgs returns the full argv for the gh pr merge command, including
// the TOCTOU-preventing --match-head-commit flag and the policy-compatible
// auto/merge-queue path. Exported so tests can verify the safety flags are
// always present — removing the head anchor in PR #460 caused a P1 regression.
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
		"--auto",
		"--delete-branch",
		"--match-head-commit", headSHA,
	}
}

// --- gate implementations ---

type checkRun struct {
	Name  string `json:"name"`
	State string `json:"state"`
}

// RequiredCheckStatus is the normalized state of a provider-required check.
type RequiredCheckStatus uint8

const (
	// RequiredCheckPending means a required check has not reached a terminal state.
	RequiredCheckPending RequiredCheckStatus = iota
	// RequiredCheckPassing means a required check completed acceptably.
	RequiredCheckPassing
	// RequiredCheckFailing means a required check completed unsuccessfully.
	RequiredCheckFailing
)

// RequiredCheck is a provider check after shared effective-policy
// reconciliation. It intentionally excludes advisory rollup history.
type RequiredCheck struct {
	Name   string
	Status RequiredCheckStatus
}

type requiredCheckProjection struct {
	Runs               []checkRun
	ProviderRuns       []checkRun
	AuthoritativeEmpty bool
}

type requiredStatusChecksResponse struct {
	Contexts []string `json:"contexts"`
	Checks   []struct {
		Context string `json:"context"`
		AppID   *int64 `json:"app_id"`
	} `json:"checks"`
}

type appliedBranchRule struct {
	Type       string `json:"type"`
	Parameters struct {
		RequiredStatusChecks []struct {
			Context       string `json:"context"`
			IntegrationID *int64 `json:"integration_id"`
		} `json:"required_status_checks"`
	} `json:"parameters"`
}

type requiredCheckIdentity struct {
	Context       string
	IntegrationID int64
	Scoped        bool
}

type requiredCheckPolicy struct {
	Identities           map[requiredCheckIdentity]bool
	HasRequiredWorkflows bool
}

type prBaseResult struct {
	BaseRefName string `json:"baseRefName"`
}

// discoverRequiredChecks returns the complete layered CI policy for the PR's
// base branch. Applied rulesets and classic branch protection are independent
// sources that GitHub layers, so both must be fetched successfully before any
// subset of checks can be trusted. A classic 404 is authoritative known-empty;
// every other discovery or parse failure blocks the merge.
func discoverRequiredChecks(repo, branch string) (requiredCheckPolicy, error) {
	return discoverRequiredChecksContext(context.Background(), repo, branch)
}

func discoverRequiredChecksContext(ctx context.Context, repo, branch string) (requiredCheckPolicy, error) {
	parts := strings.SplitN(repo, "/", 2)
	if len(parts) != 2 || branch == "" {
		return requiredCheckPolicy{}, fmt.Errorf("invalid required-check target %q branch %q", repo, branch)
	}

	// --paginate --slurp makes omission impossible when more than one API page
	// of rules applies. The parser expects one nested array per returned page.
	rulesOut, rulesErr := runCommand(exec.CommandContext(ctx, "gh", "api", "--paginate", "--slurp",
		rulesBranchEndpoint(repo, branch),
	))
	var rulesPolicy requiredCheckPolicy
	if rulesErr == nil {
		var parseErr error
		rulesPolicy, parseErr = parseAppliedRulesRequiredChecks(rulesOut)
		if parseErr != nil {
			rulesErr = parseErr
		}
	}

	classicOut, classicErr := runCommand(exec.CommandContext(ctx, "gh", "api",
		fmt.Sprintf("repos/%s/%s/branches/%s/protection/required_status_checks", parts[0], parts[1], url.PathEscape(branch)),
	))
	var classicPolicy requiredCheckPolicy
	if classicErr != nil {
		if isHTTPNotFound(classicErr) {
			classicErr = nil
		}
	} else {
		classicPolicy, classicErr = parseClassicRequiredChecks(classicOut)
	}
	if err := errors.Join(rulesErr, classicErr); err != nil {
		return requiredCheckPolicy{}, fmt.Errorf("discovering effective required checks: %w", err)
	}
	return mergeRequiredCheckPolicies(rulesPolicy, classicPolicy), nil
}

func rulesBranchEndpoint(repo, branch string) string {
	return fmt.Sprintf("repos/%s/rules/branches/%s?per_page=100", repo, url.PathEscape(branch))
}

func isHTTPNotFound(err error) bool {
	return err != nil && strings.Contains(err.Error(), "HTTP 404")
}

func parseClassicRequiredChecks(data []byte) (requiredCheckPolicy, error) {
	var resp requiredStatusChecksResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return requiredCheckPolicy{}, fmt.Errorf("parsing classic required checks: %w", err)
	}
	policy := newRequiredCheckPolicy()
	canonicalContexts := make(map[string]bool, len(resp.Checks))
	for _, c := range resp.Checks {
		policy.add(c.Context, c.AppID)
		if c.Context != "" {
			canonicalContexts[c.Context] = true
		}
	}
	for _, context := range resp.Contexts {
		if !canonicalContexts[context] {
			policy.add(context, nil)
		}
	}
	return policy, nil
}

func parseAppliedRulesRequiredChecks(data []byte) (requiredCheckPolicy, error) {
	var pages [][]appliedBranchRule
	if err := json.Unmarshal(data, &pages); err != nil {
		return requiredCheckPolicy{}, fmt.Errorf("parsing applied branch rules: %w", err)
	}
	policy := newRequiredCheckPolicy()
	for _, rules := range pages {
		for _, rule := range rules {
			switch rule.Type {
			case "required_status_checks":
				for _, check := range rule.Parameters.RequiredStatusChecks {
					policy.add(check.Context, check.IntegrationID)
				}
			case "workflows", "required_workflows":
				policy.HasRequiredWorkflows = true
			}
		}
	}
	return policy, nil
}

func newRequiredCheckPolicy() requiredCheckPolicy {
	return requiredCheckPolicy{
		Identities: make(map[requiredCheckIdentity]bool),
	}
}

func (p *requiredCheckPolicy) add(context string, integrationID *int64) {
	if context == "" {
		return
	}
	identity := requiredCheckIdentity{Context: context}
	if integrationID != nil {
		identity.Scoped = true
		identity.IntegrationID = *integrationID
	}
	p.Identities[identity] = true
}

func (p requiredCheckPolicy) contexts() map[string]bool {
	contexts := make(map[string]bool, len(p.Identities))
	for identity := range p.Identities {
		contexts[identity.Context] = true
	}
	return contexts
}

func (p requiredCheckPolicy) ambiguousContexts() []string {
	counts := make(map[string]int, len(p.Identities))
	for identity := range p.Identities {
		counts[identity.Context]++
	}
	var ambiguous []string
	for context, count := range counts {
		if count > 1 {
			ambiguous = append(ambiguous, context)
		}
	}
	sort.Strings(ambiguous)
	return ambiguous
}

func mergeRequiredCheckPolicies(policies ...requiredCheckPolicy) requiredCheckPolicy {
	merged := newRequiredCheckPolicy()
	for _, policy := range policies {
		merged.HasRequiredWorkflows = merged.HasRequiredWorkflows || policy.HasRequiredWorkflows
		for identity := range policy.Identities {
			merged.Identities[identity] = true
		}
	}
	return merged
}

func fetchPRBaseBranchContext(ctx context.Context, prNum int, repo string) (string, error) {
	out, err := runCommand(exec.CommandContext(ctx, "gh", "pr", "view",
		fmt.Sprintf("%d", prNum),
		"--repo", repo,
		"--json", "baseRefName",
	))
	if err != nil {
		return "", fmt.Errorf("gh pr view base branch failed: %w", err)
	}
	var result prBaseResult
	if err := json.Unmarshal(out, &result); err != nil {
		return "", fmt.Errorf("parsing PR base branch: %w", err)
	}
	if result.BaseRefName == "" {
		return "", fmt.Errorf("PR #%d has no baseRefName", prNum)
	}
	return result.BaseRefName, nil
}

func classifyProviderRequiredChecks(providerRequired []checkRun, policy requiredCheckPolicy) ([]checkRun, error) {
	if ambiguous := policy.ambiguousContexts(); len(ambiguous) > 0 {
		return nil, fmt.Errorf("multiple required integration identities share context text and cannot be proven independently: %s", strings.Join(ambiguous, ", "))
	}
	expected := policy.contexts()
	classified := append([]checkRun(nil), providerRequired...)
	seen := make(map[string]bool, len(providerRequired))
	for _, check := range providerRequired {
		if !expected[check.Name] {
			return nil, fmt.Errorf("provider reports required check %q absent from discovered branch policy", check.Name)
		}
		seen[check.Name] = true
	}
	for context := range expected {
		if !seen[context] {
			classified = append(classified, checkRun{Name: context, State: "pending"})
		}
	}
	return classified, nil
}

func projectRequiredCheckRuns(ctx context.Context, prNum int, repo string) (requiredCheckProjection, error) {
	baseBranch, err := fetchPRBaseBranchContext(ctx, prNum, repo)
	if err != nil {
		return requiredCheckProjection{}, err
	}
	policy, err := discoverRequiredChecksContext(ctx, repo, baseBranch)
	if err != nil {
		return requiredCheckProjection{}, err
	}
	if policy.HasRequiredWorkflows {
		return requiredCheckProjection{}, fmt.Errorf("required workflow rules apply to %s; safe-merge cannot yet prove missing workflow runs", baseBranch)
	}

	requiredOut, requiredErr := runCheckCommand(exec.CommandContext(ctx, "gh", "pr", "checks",
		fmt.Sprintf("%d", prNum),
		"--repo", repo,
		"--required",
		"--json", "name,state",
	))
	if requiredErr != nil {
		if len(policy.Identities) == 0 && isNoRequiredChecksReported(requiredErr) {
			return requiredCheckProjection{AuthoritativeEmpty: true}, nil
		}
		return requiredCheckProjection{}, fmt.Errorf("provider required-check projection unavailable for %d configured identity requirement(s): %w", len(policy.Identities), requiredErr)
	}

	var providerRequired []checkRun
	if err := json.Unmarshal(requiredOut, &providerRequired); err != nil {
		return requiredCheckProjection{}, fmt.Errorf("parsing provider-required check output: %w", err)
	}
	if len(providerRequired) == 0 && len(policy.Identities) == 0 {
		return requiredCheckProjection{AuthoritativeEmpty: true}, nil
	}
	requiredOnly, err := classifyProviderRequiredChecks(providerRequired, policy)
	if err != nil {
		return requiredCheckProjection{}, fmt.Errorf("reconciling provider-required checks with discovered policy: %w", err)
	}
	return requiredCheckProjection{Runs: requiredOnly, ProviderRuns: providerRequired}, nil
}

func isNoRequiredChecksReported(err error) bool {
	if err == nil {
		return false
	}
	for line := range strings.SplitSeq(err.Error(), "\n") {
		line = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "stderr:"))
		if strings.HasSuffix(line, "' branch") &&
			(strings.HasPrefix(line, "no checks reported on the '") ||
				strings.HasPrefix(line, "no required checks reported on the '")) {
			return true
		}
	}
	return false
}

// ProjectRequiredChecks resolves the complete layered branch policy, reconciles
// it with GitHub's integration-aware PR projection, and returns only the
// normalized checks that mergeloop may use for repair classification.
func ProjectRequiredChecks(ctx context.Context, prNum int, repo string) ([]RequiredCheck, error) {
	projection, err := projectRequiredCheckRuns(ctx, prNum, repo)
	if err != nil {
		return nil, err
	}
	return normalizeRequiredChecks(projection.Runs), nil
}

func normalizeRequiredChecks(checks []checkRun) []RequiredCheck {
	normalized := make([]RequiredCheck, 0, len(checks))
	for _, check := range checks {
		normalized = append(normalized, RequiredCheck{
			Name:   check.Name,
			Status: requiredCheckStatus(check.State),
		})
	}
	return normalized
}

func informationalCheckRuns(allChecks, providerRequired []checkRun) []checkRun {
	requiredOccurrences := make(map[string]int, len(providerRequired))
	for _, check := range providerRequired {
		requiredOccurrences[check.Name+"\x00"+strings.ToUpper(check.State)]++
	}
	var informational []checkRun
	for _, check := range allChecks {
		key := check.Name + "\x00" + strings.ToUpper(check.State)
		if requiredOccurrences[key] > 0 {
			requiredOccurrences[key]--
			continue
		}
		informational = append(informational, check)
	}
	return informational
}

func requiredCheckStatus(state string) RequiredCheckStatus {
	switch strings.ToLower(state) {
	case "success", "pass", "neutral", "skipping", "skipped":
		return RequiredCheckPassing
	case "pending", "queued", "in_progress", "waiting", "requested", "expected":
		return RequiredCheckPending
	default:
		return RequiredCheckFailing
	}
}

// checkAllCI verifies that every effective required CI check has passed.
func checkAllCI(prNum int, repo string) error {
	return checkAllCIContext(context.Background(), prNum, repo)
}

func checkAllCIContext(ctx context.Context, prNum int, repo string) error {
	projection, err := projectRequiredCheckRuns(ctx, prNum, repo)
	if err != nil {
		return err
	}
	allOut, err := runCheckCommand(exec.CommandContext(ctx, "gh", "pr", "checks",
		fmt.Sprintf("%d", prNum),
		"--repo", repo,
		"--json", "name,state",
	))
	if err != nil {
		if projection.AuthoritativeEmpty && isNoRequiredChecksReported(err) {
			allOut = []byte("[]")
		} else {
			return fmt.Errorf("gh pr checks failed: %w", err)
		}
	}
	var allChecks []checkRun
	if err := json.Unmarshal(allOut, &allChecks); err != nil {
		return fmt.Errorf("parsing all-check output: %w", err)
	}

	required := projection.Runs
	if projection.AuthoritativeEmpty {
		required = allChecks
		fmt.Fprintln(os.Stderr, "safe-merge: base branch has no required CI contexts; validating all reported checks (conservative policy)")
	} else {
		requiredNames := make([]string, 0, len(required))
		for _, check := range required {
			requiredNames = append(requiredNames, check.Name)
		}
		informationalRuns := informationalCheckRuns(allChecks, projection.ProviderRuns)
		informational := make([]string, 0, len(informationalRuns))
		for _, check := range informationalRuns {
			informational = append(informational, fmt.Sprintf("%s (%s)", check.Name, check.State))
		}
		sort.Strings(requiredNames)
		sort.Strings(informational)
		fmt.Fprintf(os.Stderr, "safe-merge: provider-effective required checks: %s\n", strings.Join(requiredNames, ", "))
		if len(informational) > 0 {
			fmt.Fprintf(os.Stderr, "safe-merge: informational checks (not merge-gating): %s\n", strings.Join(informational, ", "))
		}
	}
	return validateCheckRuns(required)
}

func validateCheckRuns(checks []checkRun) error {
	var failing []string
	var pending []string
	for _, check := range checks {
		switch requiredCheckStatus(check.State) {
		case RequiredCheckPassing:
		case RequiredCheckPending:
			pending = append(pending, check.Name)
		case RequiredCheckFailing:
			failing = append(failing, fmt.Sprintf("%s (%s)", check.Name, check.State))
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

// parseCheckRuns validates that every check in the JSON input has passed.
// The input is expected to already be filtered to the desired set (required
// checks only in production; all checks in unit tests). Exported for testing.
func parseCheckRuns(data []byte) error {
	var checks []checkRun
	if err := json.Unmarshal(data, &checks); err != nil {
		return fmt.Errorf("parsing check output: %w", err)
	}
	return validateCheckRuns(checks)
}

const reviewThreadsQuery = `
query($owner: String!, $name: String!, $number: Int!, $cursor: String) {
  repository(owner: $owner, name: $name) {
    pullRequest(number: $number) {
      reviewThreads(first: 100, after: $cursor) {
        nodes {
          isResolved
          isOutdated
          comments(first: 1) {
            nodes {
              author { login }
              body
            }
          }
        }
        pageInfo { hasNextPage endCursor }
      }
    }
  }
}`

type gqlReviewThread struct {
	IsResolved bool `json:"isResolved"`
	IsOutdated bool `json:"isOutdated"`
	Comments   struct {
		Nodes []struct {
			Author struct {
				Login string `json:"login"`
			} `json:"author"`
			Body string `json:"body"`
		} `json:"nodes"`
	} `json:"comments"`
}

type gqlReviewResult struct {
	Data struct {
		Repository struct {
			PullRequest struct {
				ReviewThreads struct {
					Nodes    []gqlReviewThread `json:"nodes"`
					PageInfo struct {
						HasNextPage bool   `json:"hasNextPage"`
						EndCursor   string `json:"endCursor"`
					} `json:"pageInfo"`
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

	var threads []gqlReviewThread
	cursor := ""
	for {
		args := []string{
			"api", "graphql",
			"--field", fmt.Sprintf("owner=%s", owner),
			"--field", fmt.Sprintf("name=%s", name),
			"--field", fmt.Sprintf("number=%d", prNum),
			"--field", fmt.Sprintf("query=%s", reviewThreadsQuery),
		}
		if cursor != "" {
			args = append(args, "--field", fmt.Sprintf("cursor=%s", cursor))
		}
		out, err := runCommand(exec.Command("gh", args...))
		if err != nil {
			return fmt.Errorf("GraphQL query failed: %w", err)
		}
		var result gqlReviewResult
		if err := json.Unmarshal(out, &result); err != nil {
			return fmt.Errorf("parsing GraphQL response: %w", err)
		}
		page := result.Data.Repository.PullRequest.ReviewThreads
		threads = append(threads, page.Nodes...)
		if !page.PageInfo.HasNextPage {
			break
		}
		if page.PageInfo.EndCursor == "" || page.PageInfo.EndCursor == cursor {
			return fmt.Errorf("GraphQL review-thread pagination did not advance")
		}
		cursor = page.PageInfo.EndCursor
	}
	return reviewThreadError(threads)
}

// parseReviewThreads returns an error if any unresolved, non-outdated review
// threads exist in the GraphQL response. Exported for testing.
func parseReviewThreads(data []byte) error {
	var result gqlReviewResult
	if err := json.Unmarshal(data, &result); err != nil {
		return fmt.Errorf("parsing GraphQL response: %w", err)
	}

	return reviewThreadError(result.Data.Repository.PullRequest.ReviewThreads.Nodes)
}

func reviewThreadError(threads []gqlReviewThread) error {
	var unresolved []string
	for i, t := range threads {
		if t.IsResolved || t.IsOutdated {
			continue
		}
		label := fmt.Sprintf("thread #%d", i+1)
		if len(t.Comments.Nodes) > 0 {
			c := t.Comments.Nodes[0]
			author := c.Author.Login
			if author == "" {
				author = "unknown"
			}
			body := c.Body
			if len([]rune(body)) > 80 {
				body = string([]rune(body)[:80]) + "…"
			}
			label = fmt.Sprintf("  • @%s: %q", author, body)
		}
		unresolved = append(unresolved, label)
	}

	if len(unresolved) > 0 {
		return fmt.Errorf("%d unresolved review thread(s):\n%s",
			len(unresolved), strings.Join(unresolved, "\n"))
	}
	return nil
}

type prViewResult struct {
	HeadRefOid string `json:"headRefOid"`
	Commits    []struct {
		CommittedDate time.Time `json:"committedDate"`
	} `json:"commits"`
}

func checkSoak(prNum int, repo string, now time.Time) error {
	out, err := runCommand(exec.Command("gh", "pr", "view",
		fmt.Sprintf("%d", prNum),
		"--repo", repo,
		"--json", "headRefOid,commits",
	))
	if err != nil {
		return fmt.Errorf("gh pr view failed: %w", err)
	}
	return parseSoak(out, now)
}

// parseSoak checks the soak time constraint from the gh pr view JSON output.
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
// AGENTS.md §5. Failures are printed as warnings, not returned as errors.
func cleanupWorktree(ctx context.Context, branch string) {
	// NUL-delimited porcelain preserves every valid worktree path byte verbatim.
	out, err := runCleanupGit(ctx, "", "worktree", "list", "--porcelain", "-z")
	if err != nil {
		fmt.Fprintf(os.Stderr, "safe-merge: cleanup: git worktree list: %v\n", err)
		return
	}

	// Parse NUL-delimited porcelain fields. Each block has "worktree <path>",
	// optionally "branch refs/heads/<branch>", followed by an empty field.
	// The very first worktree block is always the main worktree — never remove it.
	var toRemove []string
	var mainWorktree, currentPath string
	for field := range bytes.SplitSeq(out, []byte{0}) {
		if after, ok := bytes.CutPrefix(field, []byte("worktree ")); ok {
			currentPath = string(after)
			if mainWorktree == "" {
				mainWorktree = currentPath
			}
		} else if bytes.Equal(field, []byte("branch refs/heads/"+branch)) && currentPath != "" {
			if currentPath != mainWorktree {
				toRemove = append(toRemove, currentPath)
			}
			currentPath = ""
		}
	}
	if mainWorktree == "" {
		fmt.Fprintln(os.Stderr, "safe-merge: cleanup: git worktree list returned no primary worktree")
		return
	}

	for _, path := range toRemove {
		fmt.Fprintf(os.Stderr, "safe-merge: cleanup: removing worktree %s\n", path)
		if out, err := runCleanupGit(ctx, mainWorktree, "worktree", "remove", "--", path); err != nil {
			fmt.Fprintf(os.Stderr, "safe-merge: cleanup: worktree remove %s: %v: %s\n", path, err, out)
		}
	}

	// Delete the local branch if it exists.
	if out, err := runCleanupGit(ctx, mainWorktree, "branch", "-d", "--", branch); err != nil {
		// -d refuses to delete if unmerged; that's fine — we already merged.
		// Suppress the error if it's just "branch not found".
		if !strings.Contains(string(out), "not found") {
			fmt.Fprintf(os.Stderr, "safe-merge: cleanup: branch -d %s: %s\n", branch, strings.TrimSpace(string(out)))
		}
	} else {
		fmt.Fprintf(os.Stderr, "safe-merge: cleanup: removed local branch %s\n", branch)
	}
}

func runCleanupGit(ctx context.Context, dir string, args ...string) ([]byte, error) {
	commandCtx, cancel := context.WithTimeout(ctx, cleanupCommandTimeout)
	defer cancel()

	// #nosec G204,G702 -- executable name is fixed; internal Git argv uses
	// explicit option terminators before provider- or repository-derived values.
	cmd := exec.CommandContext(commandCtx, "git", args...)
	cmd.Dir = dir
	cmd.WaitDelay = cleanupCommandWaitDelay
	out, err := cmd.CombinedOutput()
	if err != nil && commandCtx.Err() != nil {
		return out, fmt.Errorf("git cleanup command: %w", commandCtx.Err())
	}
	return out, err
}
