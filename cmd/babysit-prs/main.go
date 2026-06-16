// Command babysit-prs serially updates-branch and merges all open PRs that
// pass the safe-merge predicate, working around the "every merge makes
// remaining PRs BEHIND" problem imposed by requiresLinearHistory=true.
//
// Workflow per PR:
//
//  1. List all open, non-draft PRs.
//  2. If open PR count exceeds --cap, exit early (backpressure).
//  3. For each PR up to --limit:
//     a. If BEHIND: gh pr update-branch --rebase (brings it up to date).
//     b. Call `safe-merge <pr>` (handles check polling, thread guard, TOCTOU merge).
//     c. If merge fails (head mismatch after race), skip and continue.
//  4. After each merge all remaining PRs fall BEHIND; the next iteration
//     re-runs update-branch for them.
//
// Usage:
//
//	babysit-prs [--repo owner/name] [--limit N] [--cap N] [--timeout dur] [--dry-run]
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
)

const usage = `babysit-prs — serial PR updater + merger

Usage:
  babysit-prs [flags]

Flags:
  --repo <owner/name>             GitHub repo (auto-detected if omitted)
  --limit <n>                     Max PRs to merge per run (default: 10)
  --cap <n>                       Abort if open-PR count > cap (backpressure, default: 50)
  --timeout <dur>                 Timeout per PR for pending checks passed to safe-merge (default: 45m)
  --dry-run                       Update-branch and print checks without merging (passed to safe-merge)
  --skip-bot-review               Bypass Gemini bot-review gate (requires --skip-bot-review-reason)
  --skip-bot-review-reason <s>    Justification for skipping; passed to safe-merge and recorded in audit log

babysit-prs lists every open non-draft PR, rebases any that are BEHIND, then
delegates each merge to the safe-merge binary (which enforces all guards and
performs a TOCTOU-safe squash merge).

See docs/design-safe-merge.md for the merge predicate details.
`

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "\nbabysit-prs: %v\n", err)
		os.Exit(1)
	}
}

type config struct {
	Repo                string
	Limit               int
	Cap                 int
	Timeout             time.Duration
	DryRun              bool
	SkipBotReview       bool
	SkipBotReviewReason string
}

func run(argv []string) error {
	if len(argv) > 0 && (argv[0] == "-h" || argv[0] == "--help") {
		fmt.Print(usage)
		return nil
	}
	cfg, err := parseFlags(argv)
	if err != nil {
		return err
	}
	if cfg.SkipBotReview && strings.TrimSpace(cfg.SkipBotReviewReason) == "" {
		return fmt.Errorf("--skip-bot-review requires --skip-bot-review-reason")
	}
	if cfg.Repo == "" {
		cfg.Repo, err = detectRepo()
		if err != nil {
			return fmt.Errorf("cannot detect GitHub repo: %w\nhint: pass --repo owner/name", err)
		}
	}
	return babysit(cfg)
}

// parseStringFlag returns the value at argv[i+1] for a named flag.
func parseStringFlag(argv []string, i int, flag string) (string, error) {
	if i+1 >= len(argv) {
		return "", fmt.Errorf("%s requires an argument", flag)
	}
	return argv[i+1], nil
}

// parseIntFlag returns a positive integer value for a named flag.
func parseIntFlag(argv []string, i int, flag string) (int, error) {
	val, err := parseStringFlag(argv, i, flag)
	if err != nil {
		return 0, err
	}
	n, err := strconv.Atoi(val)
	if err != nil || n < 1 {
		return 0, fmt.Errorf("%s must be a positive integer, got %q", flag, val)
	}
	return n, nil
}

// parseDurationFlag returns a time.Duration value for a named flag.
func parseDurationFlag(argv []string, i int, flag string) (time.Duration, error) {
	val, err := parseStringFlag(argv, i, flag)
	if err != nil {
		return 0, err
	}
	d, err := time.ParseDuration(val)
	if err != nil {
		return 0, fmt.Errorf("invalid %s %q: %w", flag, val, err)
	}
	return d, nil
}

func parseFlags(argv []string) (config, error) {
	cfg := config{
		Limit:   10,
		Cap:     50,
		Timeout: 45 * time.Minute,
	}
	for i := 0; i < len(argv); i++ {
		arg := argv[i]
		switch arg {
		case "-h", "--help":
			fmt.Print(usage)
			return cfg, nil
		case "--repo":
			val, err := parseStringFlag(argv, i, "--repo")
			if err != nil {
				return cfg, err
			}
			cfg.Repo = val
			i++
		case "--limit":
			n, err := parseIntFlag(argv, i, "--limit")
			if err != nil {
				return cfg, err
			}
			cfg.Limit = n
			i++
		case "--cap":
			n, err := parseIntFlag(argv, i, "--cap")
			if err != nil {
				return cfg, err
			}
			cfg.Cap = n
			i++
		case "--timeout":
			d, err := parseDurationFlag(argv, i, "--timeout")
			if err != nil {
				return cfg, err
			}
			cfg.Timeout = d
			i++
		case "--dry-run":
			cfg.DryRun = true
		case "--skip-bot-review":
			cfg.SkipBotReview = true
		case "--skip-bot-review-reason":
			val, err := parseStringFlag(argv, i, "--skip-bot-review-reason")
			if err != nil {
				return cfg, err
			}
			cfg.SkipBotReviewReason = val
			i++
		default:
			if len(arg) > 0 && arg[0] == '-' {
				return cfg, fmt.Errorf("unknown flag %q\n\n%s", arg, usage)
			}
			return cfg, fmt.Errorf("unexpected argument %q\n\n%s", arg, usage)
		}
	}
	return cfg, nil
}

// openPR is the minimal PR record we need from gh pr list.
type openPR struct {
	Number      int    `json:"number"`
	Title       string `json:"title"`
	HeadRefName string `json:"headRefName"`
	IsDraft     bool   `json:"isDraft"`
}

func babysit(cfg config) error {
	// Fetch cap+1 so the backpressure check is accurate even when open PR
	// count exactly equals cap.
	prs, err := listOpenPRs(cfg.Repo, cfg.Cap+1)
	if err != nil {
		return fmt.Errorf("listing open PRs: %w", err)
	}

	// Backpressure: abort if too many PRs are open to avoid triggering a
	// merge storm that would cascade CI re-runs across all open PRs.
	if len(prs) > cfg.Cap {
		fmt.Printf("babysit-prs: %d open PRs exceed cap %d — skipping (backpressure)\n",
			len(prs), cfg.Cap)
		return nil
	}

	fmt.Printf("babysit-prs: %d open PRs (limit=%d, cap=%d)\n", len(prs), cfg.Limit, cfg.Cap)

	merged := 0
	for _, pr := range prs {
		if merged >= cfg.Limit {
			fmt.Printf("babysit-prs: reached limit %d, stopping\n", cfg.Limit)
			break
		}

		fmt.Printf("\n=== PR #%d: %s ===\n", pr.Number, pr.Title)

		// Attempt to bring the PR up to date with main. Silently tolerate
		// "already up to date" responses — safe-merge will catch any issues.
		if err := updateBranch(cfg.Repo, pr.Number); err != nil {
			fmt.Printf("  update-branch: %v (proceeding anyway)\n", err)
		}

		// Delegate the full merge predicate + execution to safe-merge.
		if err := callSafeMerge(cfg, pr.Number); err != nil {
			fmt.Printf("  skip PR #%d: %v\n", pr.Number, err)
			continue
		}

		if !cfg.DryRun {
			merged++
		}
	}

	if cfg.DryRun {
		fmt.Printf("\nbabysit-prs: dry-run complete (%d PRs inspected)\n", len(prs))
	} else {
		fmt.Printf("\nbabysit-prs: merged %d PR(s)\n", merged)
	}
	return nil
}

// callSafeMerge invokes the safe-merge binary for a single PR.
func callSafeMerge(cfg config, prNum int) error {
	args := []string{"--pr", strconv.Itoa(prNum), "--repo", cfg.Repo}
	if cfg.DryRun {
		args = append(args, "--dry-run")
	}
	if cfg.SkipBotReview {
		args = append(args, "--skip-bot-review", "--skip-bot-review-reason", cfg.SkipBotReviewReason)
	}

	// Context timeout kills a stuck safe-merge. We do not pass --watch to
	// safe-merge here: babysit-prs is a serial loop that skips unready PRs
	// and retries them on the next run, rather than waiting per-PR.
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 5 * time.Minute
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "safe-merge", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return fmt.Errorf("safe-merge timed out after %s", timeout)
		}
		return fmt.Errorf("safe-merge exited non-zero: %w", err)
	}
	return nil
}

// updateBranch calls `gh pr update-branch --rebase` to rebase a PR onto main.
func updateBranch(repo string, prNum int) error {
	args := []string{"pr", "update-branch", "--rebase", strconv.Itoa(prNum)}
	if repo != "" {
		args = append(args, "--repo", repo)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "gh", args...)
	var stderr bytes.Buffer
	cmd.Stdout = os.Stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return fmt.Errorf("update-branch timed out")
		}
		msg := strings.TrimSpace(stderr.String())
		if msg != "" {
			return fmt.Errorf("%w: %s", err, msg)
		}
		return err
	}
	return nil
}

// listOpenPRs returns open, non-draft PRs up to limit. Pass cap+1 to ensure
// the backpressure check is accurate when open-PR count equals cap exactly.
func listOpenPRs(repo string, limit int) ([]openPR, error) {
	args := []string{
		"pr", "list",
		"--state", "open",
		"--json", "number,title,headRefName,isDraft",
		"--limit", strconv.Itoa(limit),
	}
	if repo != "" {
		args = append(args, "--repo", repo)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "gh", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("gh pr list timed out")
		}
		return nil, fmt.Errorf("gh pr list: %w (stderr: %s)", err, stderr.String())
	}
	var all []openPR
	if err := json.Unmarshal(stdout.Bytes(), &all); err != nil {
		return nil, fmt.Errorf("parse gh output: %w", err)
	}
	var ready []openPR
	for _, pr := range all {
		if !pr.IsDraft {
			ready = append(ready, pr)
		}
	}
	return ready, nil
}

// parseRepoFromURL extracts "owner/repo" from a GitHub remote URL.
// It handles both HTTPS (github.com/owner/repo) and SSH (github.com:owner/repo)
// forms, with or without a trailing ".git" suffix.
func parseRepoFromURL(rawURL string) (string, error) {
	rawURL = strings.TrimSuffix(rawURL, ".git")
	for _, prefix := range []string{"github.com/", "github.com:"} {
		if idx := strings.LastIndex(rawURL, prefix); idx >= 0 {
			return rawURL[idx+len(prefix):], nil
		}
	}
	return "", fmt.Errorf("cannot parse GitHub repo from remote URL: %q", rawURL)
}

// detectRepo auto-detects the GitHub repo from the current git remote.
func detectRepo() (string, error) {
	cmd := exec.Command("git", "remote", "get-url", "origin")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return parseRepoFromURL(strings.TrimSpace(string(out)))
}
