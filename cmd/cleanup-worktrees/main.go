// Command cleanup-worktrees finds and removes stale git worktrees.
package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func main() {
	os.Setenv("GIT_TERMINAL_PROMPT", "0")
	if err := run(os.Args[1:]); err != nil {
		var exit exitError
		fmt.Fprintln(os.Stderr, err)
		if errors.As(err, &exit) {
			os.Exit(exit.code)
		}
		os.Exit(1)
	}
}

type exitError struct {
	code int
	err  error
}

func (e exitError) Error() string { return e.err.Error() }

type preserveFlag map[string]bool

func (p preserveFlag) String() string { return "" }
func (p preserveFlag) Set(v string) error {
	if v == "" {
		return fmt.Errorf("--preserve needs a value")
	}
	p[v] = true
	return nil
}

type config struct {
	repo     string
	fix      bool
	maxAge   int
	preserve preserveFlag
}

type worktree struct {
	path     string
	branch   string
	head     string
	bare     bool
	detached bool
}

func run(args []string) error {
	cfg, err := parse(args)
	if err != nil {
		return err
	}
	if err := gitOK(cfg.repo, "rev-parse", "--git-dir"); err != nil {
		return exitError{2, fmt.Errorf("error: not a git directory: %s", cfg.repo)}
	}
	target, err := targetRef(cfg.repo)
	if err != nil {
		return exitError{2, err}
	}
	logf("repo: %s", cfg.repo)
	logf("target ref: %s", target)
	logf("max-age: %d days", cfg.maxAge)
	if cfg.fix {
		logf("mode: FIX")
	} else {
		logf("mode: DRY-RUN")
	}
	if err := gitOK(cfg.repo, "fetch", "--quiet", "origin"); err != nil {
		logf("warning: fetch failed; using cached refs")
	}
	worktrees, err := listWorktrees(cfg.repo)
	if err != nil {
		return err
	}
	failed := 0
	stale, kept, preserved := 0, 0, 0
	now := time.Now()
	for _, wt := range worktrees {
		if samePath(wt.path, cfg.repo) {
			continue
		}
		outcome := inspect(cfg, target, now, wt)
		stale += outcome.stale
		kept += outcome.kept
		preserved += outcome.preserved
		failed += outcome.failed
	}
	logf("summary: stale=%d kept=%d preserved=%d failed=%d", stale, kept, preserved, failed)
	if failed > 0 {
		return exitError{3, fmt.Errorf("cleanup-worktrees: %d removal(s) failed", failed)}
	}
	return nil
}

func parse(args []string) (config, error) {
	cfg := config{maxAge: 14, preserve: preserveFlag{}}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-h", "--help":
			usage()
			return cfg, exitError{0, fmt.Errorf("")}
		case "--fix":
			cfg.fix = true
		case "--max-age":
			if i+1 >= len(args) {
				return cfg, exitError{1, fmt.Errorf("error: --max-age needs a value")}
			}
			n, err := strconv.Atoi(args[i+1])
			if err != nil {
				return cfg, exitError{1, fmt.Errorf("error: invalid --max-age %q", args[i+1])}
			}
			cfg.maxAge = n
			i++
		case "--preserve":
			if i+1 >= len(args) {
				return cfg, exitError{1, fmt.Errorf("error: --preserve needs a value")}
			}
			cfg.preserve[args[i+1]] = true
			i++
		default:
			if strings.HasPrefix(args[i], "-") {
				usage()
				return cfg, exitError{1, fmt.Errorf("error: unknown flag: %s", args[i])}
			}
			if cfg.repo != "" {
				return cfg, exitError{1, fmt.Errorf("error: unexpected argument: %s", args[i])}
			}
			cfg.repo = args[i]
		}
	}
	if cfg.repo == "" {
		usage()
		return cfg, exitError{1, fmt.Errorf("error: expected one repo path")}
	}
	return cfg, nil
}

func usage() {
	fmt.Fprintln(os.Stderr, "Usage: cleanup-worktrees <repo-path> [--fix] [--max-age DAYS] [--preserve NAME ...]")
}

type outcome struct{ stale, kept, preserved, failed int }

func inspect(cfg config, target string, now time.Time, wt worktree) outcome {
	if wt.bare || wt.detached {
		return outcome{}
	}
	name := filepath.Base(wt.path)
	if cfg.preserve[name] {
		logf("preserve: %s (--preserve)", name)
		return outcome{preserved: 1}
	}
	ahead := gitInt(cfg.repo, "rev-list", "--count", target+".."+wt.head)
	last := gitInt(cfg.repo, "log", "-1", "--format=%ct", wt.head)
	ageDays := int(now.Sub(time.Unix(int64(last), 0)).Hours() / 24)
	reason := ""
	if ahead == 0 {
		reason = fmt.Sprintf("merged-or-empty (0 commits ahead of %s)", target)
	} else if ageDays >= cfg.maxAge {
		reason = fmt.Sprintf("idle (%d days since last commit, ahead=%d)", ageDays, ahead)
	}
	if reason == "" {
		return outcome{kept: 1}
	}
	logf("stale: %s [%s] - %s", name, wt.branch, reason)
	if !cfg.fix {
		logf("  would: git -C %s worktree remove --force %s", cfg.repo, wt.path)
		logf("  would: git -C %s branch -D %s", cfg.repo, wt.branch)
		logf("  would: git -C %s push origin --delete %s", cfg.repo, wt.branch)
		return outcome{stale: 1}
	}
	if err := runGitPassthrough(cfg.repo, "worktree", "remove", "--force", wt.path); err != nil {
		logf("  FAILED: worktree remove %s", wt.path)
		logf("  skipped branch cleanup for %s because worktree removal failed", strings.TrimPrefix(wt.branch, "refs/heads/"))
		return outcome{stale: 1, failed: 1}
	}
	branch := strings.TrimPrefix(wt.branch, "refs/heads/")
	if branch != "" && gitOK(cfg.repo, "rev-parse", "--verify", "--quiet", branch) == nil {
		if err := runGitPassthrough(cfg.repo, "branch", "-D", branch); err != nil {
			logf("  FAILED: branch -D %s", branch)
			return outcome{stale: 1, failed: 1}
		}
	}
	if branch != "" {
		if err := runGitPassthrough(cfg.repo, "push", "origin", "--delete", branch); err != nil {
			logf("  note: remote branch %s already gone or push failed", branch)
		}
	}
	return outcome{stale: 1}
}

func targetRef(repo string) (string, error) {
	if gitOK(repo, "rev-parse", "--verify", "--quiet", "origin/HEAD") == nil {
		if out, err := git(repo, "symbolic-ref", "--short", "refs/remotes/origin/HEAD"); err == nil && strings.TrimSpace(out) != "" {
			return strings.TrimSpace(out), nil
		}
	}
	if gitOK(repo, "rev-parse", "--verify", "--quiet", "origin/main") != nil {
		return "", fmt.Errorf("error: target ref not found: origin/main")
	}
	return "origin/main", nil
}

func listWorktrees(repo string) ([]worktree, error) {
	out, err := git(repo, "worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}
	var result []worktree
	var cur worktree
	flush := func() {
		if cur.path != "" {
			result = append(result, cur)
			cur = worktree{}
		}
	}
	for line := range strings.SplitSeq(out, "\n") {
		if line == "" {
			flush()
			continue
		}
		key, val, _ := strings.Cut(line, " ")
		switch key {
		case "worktree":
			flush()
			cur.path = val
		case "HEAD":
			cur.head = val
		case "branch":
			cur.branch = val
		case "bare":
			cur.bare = true
		case "detached":
			cur.detached = true
		}
	}
	flush()
	return result, nil
}

func gitInt(repo string, args ...string) int {
	out, err := git(repo, args...)
	if err != nil {
		return 0
	}
	n, _ := strconv.Atoi(strings.TrimSpace(out))
	return n
}

func gitOK(repo string, args ...string) error {
	_, err := git(repo, args...)
	return err
}

func git(repo string, args ...string) (string, error) {
	all := append([]string{"-C", repo}, args...)
	cmd := exec.Command("git", all...)
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(errb.String())
		if msg != "" {
			return out.String(), fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, msg)
		}
		return out.String(), err
	}
	return out.String(), nil
}

func runGitPassthrough(repo string, args ...string) error {
	all := append([]string{"-C", repo}, args...)
	cmd := exec.Command("git", all...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func logf(format string, args ...any) {
	fmt.Printf("[cleanup-worktrees] "+format+"\n", args...)
}

func samePath(a, b string) bool {
	aa, errA := filepath.EvalSymlinks(a)
	bb, errB := filepath.EvalSymlinks(b)
	if errA == nil {
		a = aa
	}
	if errB == nil {
		b = bb
	}
	return filepath.Clean(a) == filepath.Clean(b)
}
