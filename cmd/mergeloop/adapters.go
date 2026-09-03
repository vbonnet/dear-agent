package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/vbonnet/dear-agent/internal/mergeloop"
	"github.com/vbonnet/dear-agent/internal/safegit"
)

// ---- gh-backed PR lister ----

type requiredCheckProjector func(context.Context, int, string) ([]safegit.RequiredCheck, error)

type ghLister struct {
	project requiredCheckProjector
	// reviewClock attaches the label ages the agentic review gate needs. It is
	// nil when the gate is not configured, which keeps the loop's per-pull-
	// request GitHub cost unchanged for repositories that have not adopted it.
	reviewClock func(context.Context, string, *mergeloop.PR, time.Time) error
	// now is injected so tests can drive the review clock deterministically.
	now func() time.Time
}

// ghPR mirrors the gh pr list --json schema we request.
type ghPR struct {
	Number           int    `json:"number"`
	Title            string `json:"title"`
	HeadRefName      string `json:"headRefName"`
	HeadRefOid       string `json:"headRefOid"`
	IsDraft          bool   `json:"isDraft"`
	Mergeable        string `json:"mergeable"`
	MergeStateStatus string `json:"mergeStateStatus"`
	ReviewDecision   string `json:"reviewDecision"`
	CreatedAt        string `json:"createdAt"`
	Labels           []struct {
		Name string `json:"name"`
	} `json:"labels"`
	Files []struct {
		Path string `json:"path"`
	} `json:"files"`
}

// ghChangedFilePageSize is the page size gh hard-codes for a pull request's
// `files` connection. A response at exactly this size may be truncated.
const ghChangedFilePageSize = 100

func (g *ghLister) ListOpen(ctx context.Context, repo string, maxOpen int) ([]mergeloop.PR, error) {
	args := []string{
		"pr", "list", "--state", "open",
		"--json", "number,title,headRefName,headRefOid,isDraft,mergeable,mergeStateStatus,reviewDecision,labels,files,createdAt",
		"--limit", strconv.Itoa(maxOpen + 1),
	}
	if repo != "" {
		args = append(args, "--repo", repo)
	}
	out, err := ghJSON(ctx, 60*time.Second, args)
	if err != nil {
		return nil, err
	}
	var raw []ghPR
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, fmt.Errorf("parsing gh pr list: %w", err)
	}
	prs := make([]mergeloop.PR, 0, len(raw))
	if len(raw) > maxOpen {
		for _, r := range raw {
			prs = append(prs, toPR(r, nil))
		}
		return prs, nil
	}
	project := g.project
	if project == nil {
		project = safegit.ProjectRequiredChecks
	}
	for _, r := range raw {
		pr := toPR(r, nil)
		projectionCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
		required, err := project(projectionCtx, r.Number, repo)
		cancel()
		if err != nil {
			if ctx.Err() != nil {
				return nil, fmt.Errorf("resolving effective required checks for PR #%d: %w", r.Number, ctx.Err())
			}
			pr.CheckProjectionError = fmt.Sprintf("resolving effective required checks: %v", err)
			prs = append(prs, pr)
			continue
		}
		checks, err := mergeLoopChecks(required)
		if err != nil {
			pr.CheckProjectionError = fmt.Sprintf("normalizing effective required checks: %v", err)
			prs = append(prs, pr)
			continue
		}
		pr.Checks = checks
		g.attachReviewClock(ctx, repo, &pr)
		prs = append(prs, pr)
	}
	return prs, nil
}

// attachReviewClock records the review timing on pr, or leaves it deliberately
// blank when it cannot be read. A blank clock keeps the gate pending rather
// than merging: a GitHub read that failed is not evidence a reviewer approved.
func (g *ghLister) attachReviewClock(ctx context.Context, repo string, pr *mergeloop.PR) {
	if g.reviewClock == nil {
		return
	}
	now := time.Now
	if g.now != nil {
		now = g.now
	}
	if err := g.reviewClock(ctx, repo, pr, now().UTC()); err != nil {
		pr.ObservedAt = time.Time{}
		pr.LabelAppliedAt = nil
		pr.ReadyAt = time.Time{}
		fmt.Fprintf(os.Stderr, "mergeloop: PR #%d: agentic review clock unavailable: %v\n", pr.Number, err)
	}
}

func toPR(r ghPR, required []mergeloop.Check) mergeloop.PR {
	pr := mergeloop.PR{
		Number:           r.Number,
		Title:            r.Title,
		HeadRefName:      r.HeadRefName,
		HeadSHA:          r.HeadRefOid,
		IsDraft:          r.IsDraft,
		Mergeable:        r.Mergeable,
		MergeStateStatus: r.MergeStateStatus,
		ReviewDecision:   r.ReviewDecision,
		Checks:           append([]mergeloop.Check(nil), required...),
	}
	for _, l := range r.Labels {
		pr.Labels = append(pr.Labels, l.Name)
	}
	for _, f := range r.Files {
		pr.ChangedFiles = append(pr.ChangedFiles, f.Path)
	}
	// gh generates `files(first: 100)` for this connection and never paginates
	// it, so a full page means the list may be incomplete. Report that rather
	// than letting classification treat a partial list as exhaustive.
	pr.ChangedFilesTruncated = len(r.Files) >= ghChangedFilePageSize
	if r.CreatedAt != "" {
		if at, err := time.Parse(time.RFC3339, r.CreatedAt); err == nil {
			pr.CreatedAt = at
		}
	}
	return pr
}

func mergeLoopChecks(effective []safegit.RequiredCheck) ([]mergeloop.Check, error) {
	checks := make([]mergeloop.Check, 0, len(effective))
	for _, check := range effective {
		var verdict mergeloop.CheckVerdict
		switch check.Status {
		case safegit.RequiredCheckPassing:
			verdict = mergeloop.CheckPass
		case safegit.RequiredCheckPending:
			verdict = mergeloop.CheckPending
		case safegit.RequiredCheckFailing:
			verdict = mergeloop.CheckFail
		default:
			return nil, fmt.Errorf("unknown safegit required-check status %d for %q", check.Status, check.Name)
		}
		checks = append(checks, mergeloop.Check{Name: check.Name, Verdict: verdict, Required: true})
	}
	return checks, nil
}

func ghJSON(ctx context.Context, timeout time.Duration, args []string) ([]byte, error) {
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(cctx, "gh", args...)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		// Build a label from at most the first two args; slicing args[:2]
		// unconditionally would panic when fewer than two were passed.
		label := strings.Join(args, " ")
		if len(args) > 2 {
			label = strings.Join(args[:2], " ")
		}
		if cctx.Err() != nil {
			return nil, fmt.Errorf("gh %s timed out", label)
		}
		return nil, fmt.Errorf("gh %s: %w (%s)", label, err, strings.TrimSpace(stderr.String()))
	}
	return stdout.Bytes(), nil
}

// ---- safe-rebase backed rebaser ----

type safeRebaser struct{ dryRun bool }

func (s *safeRebaser) Rebase(ctx context.Context, repo string, pr int) error {
	if s.dryRun {
		fmt.Printf("  [dry-run] would rebase PR #%d\n", pr)
		return nil
	}
	// Prefer the deterministic safe-rebase wrapper once it lands on main
	// (PR #465); fall back to gh's update-branch --rebase until then.
	if path, err := exec.LookPath("safe-rebase"); err == nil {
		return runStreaming(ctx, 90*time.Second, path, "--pr", strconv.Itoa(pr), "--repo", repo)
	}
	return runStreaming(ctx, 90*time.Second, "gh", "pr", "update-branch", "--rebase", strconv.Itoa(pr), "--repo", repo)
}

// ---- safe-merge backed merger ----

type safeMerger struct{ dryRun bool }

func (s *safeMerger) Merge(_ context.Context, repo string, pr int) error {
	err := safegit.SafeMerge(safegit.MergeConfig{
		PRNumber: pr,
		Repo:     repo,
		DryRun:   s.dryRun,
	})
	if err != nil && isSoftMergeError(err) {
		return fmt.Errorf("%w: %w", mergeloop.ErrNotReady, err)
	}
	return err
}

// isSoftMergeError reports whether a safe-merge failure is a transient gate
// (soak window, bot review, unresolved threads, pending checks) — meaning
// "wait and retry" rather than a hard error to surface.
func isSoftMergeError(err error) bool {
	msg := strings.ToLower(err.Error())
	for _, soft := range []string{"soak", "review bot", "unresolved", "pending", "not posted", "still running", "not ready"} {
		if strings.Contains(msg, soft) {
			return true
		}
	}
	return false
}

// ---- AGM-backed agent spawner ----

type agmSpawner struct {
	dryRun  bool
	enabled bool
	harness string
	model   string
}

func (a *agmSpawner) ActiveSession(ctx context.Context, _ string, pr int) (bool, error) {
	// Best-effort: query agm for an active session with our deterministic name.
	cctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(cctx, "agm", "session", "list", "--all", "--output", "json")
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return false, nil // agm unavailable → assume no active session
	}
	var sessions []struct {
		Name   string `json:"name"`
		Status string `json:"status"`
	}
	if err := json.Unmarshal(bytes.TrimSpace(stdout.Bytes()), &sessions); err != nil {
		return false, nil
	}
	want := mergeloop.SessionName(pr)
	for _, s := range sessions {
		if s.Name == want && !isTerminalStatus(s.Status) {
			return true, nil
		}
	}
	return false, nil
}

func isTerminalStatus(s string) bool {
	switch strings.ToLower(s) {
	case "stopped", "archived", "completed", "failed", "killed":
		return true
	}
	return false
}

func (a *agmSpawner) Spawn(ctx context.Context, req mergeloop.AgentRequest) (string, error) {
	if a.dryRun {
		fmt.Printf("  [dry-run] would spawn agent %s (%s) for PR #%d\n", req.SessionName, req.Kind, req.PRNumber)
		return req.SessionName, nil
	}
	if !a.enabled {
		// Defer-don't-block: the agent-fix path is wired but gated until the
		// host dispatch substrate + auth (ce-cd14 / ce-m3ya) land.
		return "", fmt.Errorf("agent spawning disabled (pass --enable-agents): %w", mergeloop.ErrSpawnUnavailable)
	}
	workspace := os.Getenv("WORKSPACE")
	if workspace == "" {
		workspace = "oss"
	}
	cctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	args := mergeloop.BuildSessionNewArgs(req, workspace, mergeloop.SpawnRoute{
		Harness: a.harness,
		Model:   a.model,
	})
	// #nosec G204 G702 -- arguments are passed as exec argv, never interpreted
	// by a shell; the agm binary name is a constant.
	cmd := exec.CommandContext(cctx, "agm", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("agm session new: %w (%s)", err, strings.TrimSpace(stderr.String()))
	}
	return req.SessionName, nil
}

// ---- shared helpers ----

func runStreaming(ctx context.Context, timeout time.Duration, name string, args ...string) error {
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(cctx, name, args...)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		if cctx.Err() != nil {
			return fmt.Errorf("%s timed out after %s", name, timeout)
		}
		return fmt.Errorf("%s: %w", name, err)
	}
	return nil
}

func detectRepo() (string, error) {
	// Bound the git call: a misconfigured credential helper can make
	// `git remote get-url` hang indefinitely, which would stall mergeloop
	// startup before the loop ever begins.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "git", "remote", "get-url", "origin").Output()
	if err != nil {
		return "", err
	}
	raw := strings.TrimSuffix(strings.TrimSpace(string(out)), ".git")
	for _, prefix := range []string{"github.com/", "github.com:"} {
		if idx := strings.LastIndex(raw, prefix); idx >= 0 {
			return raw[idx+len(prefix):], nil
		}
	}
	return "", fmt.Errorf("cannot parse repo from %q", raw)
}
