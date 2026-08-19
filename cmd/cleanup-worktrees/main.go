// Command cleanup-worktrees reports stale git worktrees for one repository
// and, with --fix, removes only those that positively prove reapable.
//
// The safety model is an allowlist, inherited from the canonical sweep in
// agm/internal/ops/worktree_sweep.go: a worktree is removed only when it is
// clean, unowned by any live session, unlocked, and carries no commits
// beyond the target ref. Every other verdict, including every failed probe,
// keeps the checkout. Remote branches are never deleted.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

func main() {
	if err := run(context.Background(), os.Args[1:]); err != nil {
		var exit exitError
		if msg := err.Error(); msg != "" {
			fmt.Fprintln(os.Stderr, msg)
		}
		if errors.As(err, &exit) {
			os.Exit(exit.code)
		}
		os.Exit(1)
	}
}

// errNoTargetRef reports that neither origin/HEAD nor origin/main resolves.
var errNoTargetRef = errors.New("error: target ref not found: origin/main")

type exitError struct {
	code int
	err  error
}

func (e exitError) Error() string { return e.err.Error() }
func (e exitError) Unwrap() error { return e.err }

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

func run(ctx context.Context, args []string) error {
	cfg, err := parse(args)
	if err != nil {
		return err
	}
	if err := gitOK(ctx, cfg.repo, "rev-parse", "--git-dir"); err != nil {
		return exitError{2, fmt.Errorf("error: not a git directory: %s", cfg.repo)}
	}
	// Resolve the supplied path to its checkout root before any comparison
	// with porcelain output, which is always absolute.
	repoRoot, err := topLevel(ctx, cfg.repo)
	if err != nil {
		return exitError{2, fmt.Errorf("error: cannot resolve worktree root for %s: %w", cfg.repo, err)}
	}
	cfg.repo = repoRoot

	// Fetch before resolving the target: a fresh clone that has never
	// fetched has no origin/HEAD or origin/main to resolve, and refusing
	// there would reject a repository a single fetch makes workable.
	if err := gitOK(ctx, cfg.repo, "fetch", "--quiet", "origin"); err != nil {
		logf("warning: fetch failed; using cached refs")
	}
	target, err := targetRef(ctx, cfg.repo)
	if err != nil {
		return exitError{2, err}
	}
	env := newScanEnv(ctx, cfg, target)
	banner(cfg, env)

	worktrees, err := listWorktrees(ctx, cfg.repo)
	if err != nil {
		return exitError{2, err}
	}
	return report(tally(ctx, cfg, env, worktrees))
}

// newScanEnv resolves the run-wide facts every classification depends on.
func newScanEnv(ctx context.Context, cfg config, target string) scanEnv {
	active, activeKnown := activeSessions(ctx)
	env := scanEnv{
		target:       target,
		targetBranch: strings.TrimPrefix(target, "origin/"),
		now:          time.Now(),
		active:       active,
		activeKnown:  activeKnown,
		protected:    map[string]bool{canonical(cfg.repo): true},
	}
	// The caller's own checkout is protected independently of --repo. Git
	// will happily unlink a worktree containing the caller's cwd, which
	// leaves the invoking shell in a directory that no longer exists.
	if cwd, err := os.Getwd(); err == nil {
		if root, err := topLevel(ctx, cwd); err == nil {
			env.protected[root] = true
		}
	}
	return env
}

func banner(cfg config, env scanEnv) {
	logf("repo: %s", cfg.repo)
	logf("target ref: %s", env.target)
	logf("max-age: %d days (report-only)", cfg.maxAge)
	if cfg.fix {
		logf("mode: FIX (removes MERGED worktrees only)")
	} else {
		logf("mode: DRY-RUN")
	}
	if !env.activeKnown {
		logf("warning: active-session probe failed; no worktree will be removed this run")
	}
}

// counts is the per-class tally reported at the end of a scan.
type counts struct {
	byClass map[classification]int
	removed int
	failed  int
}

func tally(ctx context.Context, cfg config, env scanEnv, worktrees []worktree) counts {
	c := counts{byClass: map[classification]int{}}
	for _, wt := range worktrees {
		v := inspect(ctx, cfg, env, wt)
		c.byClass[v.class]++
		if v.removed {
			c.removed++
		}
		if v.failed {
			c.failed++
		}
	}
	return c
}

func report(c counts) error {
	logf("summary: merged=%d orphaned=%d dirty=%d active=%d protected=%d unknown=%d removed=%d failed=%d",
		c.byClass[classMerged], c.byClass[classOrphaned], c.byClass[classDirty],
		c.byClass[classActive], c.byClass[classProtected], c.byClass[classUnknown],
		c.removed, c.failed)
	if c.failed > 0 {
		return exitError{3, fmt.Errorf("cleanup-worktrees: %d removal(s) failed", c.failed)}
	}
	return nil
}

func parse(args []string) (config, error) {
	cfg := config{maxAge: 14, preserve: preserveFlag{}}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-h", "--help":
			usage()
			return cfg, exitError{0, errors.New("")}
		case "--fix":
			cfg.fix = true
		case "--max-age":
			if i+1 >= len(args) {
				return cfg, exitError{1, errors.New("error: --max-age needs a value")}
			}
			n, err := strconv.Atoi(args[i+1])
			if err != nil {
				return cfg, exitError{1, fmt.Errorf("error: invalid --max-age %q", args[i+1])}
			}
			cfg.maxAge = n
			i++
		case "--preserve":
			if i+1 >= len(args) {
				return cfg, exitError{1, errors.New("error: --preserve needs a value")}
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
		return cfg, exitError{1, errors.New("error: expected one repo path")}
	}
	return cfg, nil
}

func usage() {
	fmt.Fprintln(os.Stderr, "Usage: cleanup-worktrees <repo-path> [--fix] [--max-age DAYS] [--preserve NAME ...]")
}

func logf(format string, args ...any) {
	fmt.Printf("[cleanup-worktrees] "+format+"\n", args...)
}
