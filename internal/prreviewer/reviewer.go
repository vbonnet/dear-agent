// Package prreviewer detects updated GitHub pull requests, dispatches external
// provider reviews, and posts the combined result through gh.
package prreviewer

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// ReviewEvent is the GitHub pull request review action to submit.
type ReviewEvent string

const (
	// ReviewComment posts a non-blocking PR review comment.
	ReviewComment ReviewEvent = "COMMENT"
	// ReviewRequestChanges posts a blocking request-changes review.
	ReviewRequestChanges ReviewEvent = "REQUEST_CHANGES"
	// ReviewApprove posts an approval review.
	ReviewApprove ReviewEvent = "APPROVE"
)

// Config controls one external reviewer polling pass.
type Config struct {
	Repos           []string
	Limit           int
	DryRun          bool
	StatePath       string
	ReviewEvent     ReviewEvent
	CodexCmd        []string
	GeminiCmd       []string
	GeminiTries     int
	ProviderTimeout time.Duration
	RetryDelay      time.Duration
	Now             func() time.Time
}

// PR is the subset of gh pull request JSON needed by the reviewer.
type PR struct {
	Number     int       `json:"number"`
	Title      string    `json:"title"`
	HeadRefOID string    `json:"headRefOid"`
	UpdatedAt  time.Time `json:"updatedAt"`
	IsDraft    bool      `json:"isDraft"`
}

// Runner executes external commands behind a testable boundary.
type Runner interface {
	Run(ctx context.Context, name string, args []string, stdin string) (string, error)
}

// ExecRunner executes commands on the local host.
type ExecRunner struct{}

// Run executes name with args, sends stdin, and returns stdout.
func (ExecRunner) Run(ctx context.Context, name string, args []string, stdin string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdin = strings.NewReader(stdin)
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	err := cmd.Run()
	if err != nil {
		msg := strings.TrimSpace(errb.String())
		if msg == "" {
			msg = strings.TrimSpace(out.String())
		}
		if msg != "" {
			return out.String(), fmt.Errorf("%s: %w: %s", name, err, msg)
		}
		return out.String(), fmt.Errorf("%s: %w", name, err)
	}
	return out.String(), nil
}

// Result reports what happened for one inspected pull request.
type Result struct {
	Repo     string
	PRNumber int
	HeadSHA  string
	Posted   bool
	Skipped  bool
	Reason   string
}

type state map[string]string

// RunOnce inspects configured repositories once and reviews each unseen PR head.
func RunOnce(ctx context.Context, cfg Config, runner Runner, stdout io.Writer) ([]Result, error) {
	if err := cfg.setDefaults(); err != nil {
		return nil, err
	}
	st, err := loadState(cfg.StatePath)
	if err != nil {
		return nil, err
	}
	// Every review that was actually posted must survive a later failure,
	// otherwise the next pass re-reviews it and posts a duplicate.
	var posted bool
	persist := func() error {
		if cfg.DryRun || !posted {
			return nil
		}
		return saveState(cfg.StatePath, st)
	}
	var results []Result
	for _, repo := range cfg.Repos {
		prs, err := listPRs(ctx, runner, repo, cfg.Limit)
		if err != nil {
			return results, errors.Join(fmt.Errorf("list PRs for %s: %w", repo, err), persist())
		}
		for _, pr := range prs {
			res, err := handlePR(ctx, cfg, runner, st, repo, pr, stdout)
			results = append(results, res)
			posted = posted || res.Posted
			if err != nil {
				return results, errors.Join(err, persist())
			}
		}
	}
	return results, persist()
}

func (cfg *Config) setDefaults() error {
	if len(cfg.Repos) == 0 {
		return errors.New("at least one --repo is required")
	}
	if cfg.Limit <= 0 {
		cfg.Limit = 50
	}
	if cfg.ReviewEvent == "" {
		cfg.ReviewEvent = ReviewComment
	}
	switch cfg.ReviewEvent {
	case ReviewComment, ReviewRequestChanges, ReviewApprove:
	default:
		return fmt.Errorf("unsupported review event %q", cfg.ReviewEvent)
	}
	if len(cfg.CodexCmd) == 0 {
		cfg.CodexCmd = []string{"codex", "exec", "-"}
	}
	if len(cfg.GeminiCmd) == 0 {
		cfg.GeminiCmd = []string{"agy", "run", "-"}
	}
	if cfg.GeminiTries <= 0 {
		cfg.GeminiTries = 2
	}
	if cfg.ProviderTimeout <= 0 {
		cfg.ProviderTimeout = 10 * time.Minute
	}
	if cfg.RetryDelay < 0 {
		cfg.RetryDelay = 0
	}
	if cfg.StatePath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("resolve home for state path: %w", err)
		}
		cfg.StatePath = filepath.Join(home, ".local", "state", "dear-agent", "external-pr-reviewer.json")
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return nil
}

func listPRs(ctx context.Context, runner Runner, repo string, limit int) ([]PR, error) {
	args := []string{"pr", "list", "--repo", repo, "--state", "open", "--limit", strconv.Itoa(limit), "--json", "number,title,headRefOid,updatedAt,isDraft"}
	out, err := runner.Run(ctx, "gh", args, "")
	if err != nil {
		return nil, err
	}
	var prs []PR
	if err := json.Unmarshal([]byte(out), &prs); err != nil {
		return nil, fmt.Errorf("parse gh pr list JSON: %w", err)
	}
	return prs, nil
}

func handlePR(ctx context.Context, cfg Config, runner Runner, st state, repo string, pr PR, stdout io.Writer) (Result, error) {
	key := fmt.Sprintf("%s#%d", repo, pr.Number)
	res := Result{Repo: repo, PRNumber: pr.Number, HeadSHA: pr.HeadRefOID}
	if pr.IsDraft {
		res.Skipped, res.Reason = true, "draft"
		return res, nil
	}
	if st[key] == pr.HeadRefOID {
		res.Skipped, res.Reason = true, "already-reviewed"
		return res, nil
	}
	diff, err := prDiff(ctx, runner, repo, pr.Number)
	if err != nil {
		return res, fmt.Errorf("diff for %s: %w", key, err)
	}
	prompt := reviewPrompt(repo, pr, diff)
	codex, err := runProvider(ctx, runner, cfg.CodexCmd, prompt, cfg.ProviderTimeout)
	if err != nil {
		return res, fmt.Errorf("codex review for %s: %w", key, err)
	}
	gemini, geminiErr := runGemini(ctx, cfg, runner, prompt)
	if geminiErr != nil {
		// Provider stderr can carry credentials or local paths, so the detail
		// stays in the operator's log and never reaches the public review body.
		fmt.Fprintf(stdout, "external-pr-reviewer: secondary review unavailable for %s: %v\n", key, geminiErr)
	}
	body := composeReviewBody(repo, pr, cfg.Now(), codex, gemini, cfg.GeminiTries)
	if cfg.DryRun {
		fmt.Fprintf(stdout, "external-pr-reviewer: dry-run would post %s review to %s PR #%d (%s)\n", cfg.ReviewEvent, repo, pr.Number, pr.HeadRefOID)
		res.Posted = false
		return res, nil
	}
	// gh pr review always targets the PR's current head, so a head that moved
	// while the providers were running would attach a review of the old diff.
	head, err := currentHeadSHA(ctx, runner, repo, pr.Number)
	if err != nil {
		return res, fmt.Errorf("confirm head for %s: %w", key, err)
	}
	if head != pr.HeadRefOID {
		res.Skipped, res.Reason = true, "head-changed"
		return res, nil
	}
	if err := postReview(ctx, runner, repo, pr.Number, cfg.ReviewEvent, body); err != nil {
		return res, fmt.Errorf("post review for %s: %w", key, err)
	}
	st[key] = pr.HeadRefOID
	res.Posted = true
	return res, nil
}

func prDiff(ctx context.Context, runner Runner, repo string, number int) (string, error) {
	return runner.Run(ctx, "gh", []string{"pr", "diff", strconv.Itoa(number), "--repo", repo}, "")
}

func currentHeadSHA(ctx context.Context, runner Runner, repo string, number int) (string, error) {
	out, err := runner.Run(ctx, "gh", []string{"pr", "view", strconv.Itoa(number), "--repo", repo, "--json", "headRefOid"}, "")
	if err != nil {
		return "", err
	}
	var view struct {
		HeadRefOID string `json:"headRefOid"`
	}
	if err := json.Unmarshal([]byte(out), &view); err != nil {
		return "", fmt.Errorf("parse gh pr view JSON: %w", err)
	}
	if view.HeadRefOID == "" {
		return "", errors.New("gh pr view returned an empty head SHA")
	}
	return view.HeadRefOID, nil
}

func runProvider(ctx context.Context, runner Runner, cmd []string, prompt string, timeout time.Duration) (string, error) {
	if len(cmd) == 0 {
		return "", errors.New("provider command is empty")
	}
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	out, err := runner.Run(ctx, cmd[0], cmd[1:], prompt)
	if err != nil {
		return "", err
	}
	out = strings.TrimSpace(out)
	if out == "" {
		return "", errors.New("provider returned empty review")
	}
	return out, nil
}

func runGemini(ctx context.Context, cfg Config, runner Runner, prompt string) (string, error) {
	var last error
	for attempt := range cfg.GeminiTries {
		if attempt > 0 && cfg.RetryDelay > 0 {
			// An immediate retry re-hits the same rate limit or outage.
			timer := time.NewTimer(cfg.RetryDelay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return "", errors.Join(last, ctx.Err())
			case <-timer.C:
			}
		}
		out, err := runProvider(ctx, runner, cfg.GeminiCmd, prompt, cfg.ProviderTimeout)
		if err == nil {
			return out, nil
		}
		last = err
	}
	return "", last
}

func postReview(ctx context.Context, runner Runner, repo string, number int, event ReviewEvent, body string) error {
	args := []string{"pr", "review", strconv.Itoa(number), "--repo", repo, "-b", body}
	switch event {
	case ReviewComment:
		args = append(args, "--comment")
	case ReviewRequestChanges:
		args = append(args, "--request-changes")
	case ReviewApprove:
		args = append(args, "--approve")
	}
	_, err := runner.Run(ctx, "gh", args, "")
	return err
}

func reviewPrompt(repo string, pr PR, diff string) string {
	return fmt.Sprintf(`You are reviewing a GitHub pull request.

Repository: %s
Pull request: #%d
Title: %s
Head SHA: %s

Review the diff for correctness bugs, regressions, missing tests, privacy/security risk, and operational risk. Lead with concrete findings and file/line references when possible. If there are no issues, say that clearly and mention residual test risk briefly.

Diff:
%s
`, repo, pr.Number, pr.Title, pr.HeadRefOID, diff)
}

func composeReviewBody(repo string, pr PR, now time.Time, codex, gemini string, geminiTries int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Automated external review for `%s` PR #%d at %s.\n\n", repo, pr.Number, now.UTC().Format(time.RFC3339))
	b.WriteString("## Codex\n\n")
	b.WriteString(strings.TrimSpace(codex))
	b.WriteString("\n\n## Gemini\n\n")
	if strings.TrimSpace(gemini) != "" {
		b.WriteString(strings.TrimSpace(gemini))
	} else {
		// The provider's own error text is deliberately withheld: it is
		// attacker- and environment-controlled and this body is public.
		fmt.Fprintf(&b, "Gemini review unavailable after %d attempt(s); skipped best-effort secondary review. See the reviewer log for the failure detail.", geminiTries)
	}
	b.WriteString("\n")
	return b.String()
}

func loadState(path string) (state, error) {
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return state{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read state: %w", err)
	}
	var st state
	if err := json.Unmarshal(raw, &st); err != nil {
		return nil, fmt.Errorf("parse state: %w", err)
	}
	if st == nil {
		st = state{}
	}
	return st, nil
}

func saveState(path string, st state) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create state dir: %w", err)
	}
	raw, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}
	raw = append(raw, '\n')
	// Write through a sibling temp file so an interrupted or short write can
	// never truncate the only record of what has already been reviewed.
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".*.tmp")
	if err != nil {
		return fmt.Errorf("create state temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer func() {
		tmp.Close()
		os.Remove(tmpName)
	}()
	if err := tmp.Chmod(0o600); err != nil {
		return fmt.Errorf("set state permissions: %w", err)
	}
	if _, err := tmp.Write(raw); err != nil {
		return fmt.Errorf("write state: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		return fmt.Errorf("sync state: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close state: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("replace state: %w", err)
	}
	return nil
}
