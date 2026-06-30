// src-recovery restores a golden ~/src/<repo> checkout to a clean, current
// default branch — and can do nothing else.
//
// Usage:
//
//	src-recovery [--dry-run] [--timeout <dur>] <repo-path-under-~/src>
//
// Examples:
//
//	src-recovery ~/src/dear-agent
//	src-recovery --dry-run ~/src/brain-v2
//	src-recovery --timeout 60s /Users/me/src/dear-agent
//
// It validates the path is strictly under ~/src/, then runs exactly:
//
//	git stash --include-untracked   (only when the tree is dirty)
//	git checkout <default-branch>
//	git pull --ff-only
//
// It takes no pass-through git arguments, refuses any other git verb by
// construction (see internal/safesrc), and appends a line-per-step audit record
// to ~/.local/state/dear-agent/src-recovery.log. It is the only sanctioned way
// for an agent to write to ~/src/**, which is otherwise read-only
// (see AGENTS.md and vbonnet/engram-research
// retrospectives/2026-06-11-src-violations-and-burndown.md).
//
// The `unlock` subcommand clears a stale .git/index.lock left behind by a killed
// git, which otherwise wedges the daily archive sync's commit step:
//
//	src-recovery unlock [--dry-run] [--min-age <dur>] <repo-path-under-~/src>
//
// It removes the lock only when it is older than --min-age (default 60s) and no
// process holds it open, so it can never race a live index update.
package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/vbonnet/dear-agent/internal/safesrc"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "\nsrc-recovery: %v\n", err)
		os.Exit(1)
	}
}

func run(argv []string) error {
	// Subcommand dispatch: `unlock` clears a stale .git/index.lock; the bare
	// form (no subcommand) runs the recovery sequence. Keeping recovery as the
	// default preserves the original `src-recovery <repo>` invocation.
	if len(argv) > 0 && argv[0] == "unlock" {
		return runUnlock(argv[1:])
	}

	dryRun := false
	timeout := safesrc.DefaultTimeout
	var repoArg string

	for i := 0; i < len(argv); i++ {
		arg := argv[i]
		switch arg {
		case "-h", "--help":
			fmt.Print(usage)
			return nil
		case "--dry-run", "-n":
			dryRun = true
		case "--timeout":
			rest := argv[i+1:]
			if len(rest) == 0 {
				return fmt.Errorf("--timeout requires a duration argument (e.g. 30s)")
			}
			val := rest[0]
			d, err := time.ParseDuration(val)
			if err != nil {
				return fmt.Errorf("invalid --timeout %q: %w", val, err)
			}
			timeout = d
			i++
		default:
			if len(arg) > 0 && arg[0] == '-' {
				return fmt.Errorf("unknown flag %q (src-recovery takes no pass-through git arguments; see --help)", arg)
			}
			if repoArg != "" {
				return fmt.Errorf("expected exactly one repository path, got a second: %q", arg)
			}
			repoArg = arg
		}
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("cannot determine home directory: %w", err)
	}

	repo, err := safesrc.ValidateRepo(home, repoArg)
	if err != nil {
		return err
	}

	logW, closeLog, err := openAuditLog(home)
	if err != nil {
		return err
	}
	defer closeLog()

	ctx := safesrc.WithStamp(context.Background(), time.Now().UTC().Format(time.RFC3339))
	rec := &safesrc.Recoverer{
		Repo:    repo,
		DryRun:  dryRun,
		Timeout: timeout,
		Log:     logW,
	}
	plan, err := rec.Recover(ctx)
	if err != nil {
		return err
	}

	if dryRun {
		fmt.Printf("src-recovery (dry-run): %s — would recover from %s onto %s%s\n",
			repo, plan.StartBranch, plan.DefaultBranch, dirtyNote(plan.Dirty))
		return nil
	}
	fmt.Printf("src-recovery: %s recovered onto %s%s\n", repo, plan.DefaultBranch, stashNote(plan, repo))
	return nil
}

// runUnlock implements `src-recovery unlock [--dry-run] [--min-age <dur>] <repo>`.
// It validates the path through the same ~/src boundary check as recovery, then
// removes the repo's .git/index.lock only if it is provably stale.
func runUnlock(argv []string) error {
	opts, showedHelp, err := parseUnlockArgs(argv)
	if err != nil {
		return err
	}
	if showedHelp {
		return nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("cannot determine home directory: %w", err)
	}

	repo, err := safesrc.ValidateRepo(home, opts.repoArg)
	if err != nil {
		return err
	}

	logW, closeLog, err := openAuditLog(home)
	if err != nil {
		return err
	}
	defer closeLog()

	u := &safesrc.Unlocker{
		Repo:   repo,
		DryRun: opts.dryRun,
		MinAge: opts.minAge,
		Now:    time.Now(),
		Log:    logW,
	}
	res, err := u.Unlock()
	if err != nil {
		return err
	}

	switch {
	case !res.Existed:
		fmt.Printf("src-recovery unlock: %s — no index.lock present, nothing to do\n", repo)
	case opts.dryRun:
		fmt.Printf("src-recovery unlock (dry-run): %s — would remove stale lock %s (age %s)\n",
			repo, res.LockPath, res.Age.Round(time.Second))
	default:
		fmt.Printf("src-recovery unlock: %s — removed stale lock %s (age %s)\n",
			repo, res.LockPath, res.Age.Round(time.Second))
	}
	return nil
}

// unlockOpts holds the parsed `unlock` flags.
type unlockOpts struct {
	dryRun  bool
	minAge  time.Duration
	repoArg string
}

// parseUnlockArgs parses the `unlock` subcommand's flags. showedHelp is true when
// -h/--help was handled, signalling the caller to return without doing work.
func parseUnlockArgs(argv []string) (opts unlockOpts, showedHelp bool, err error) {
	opts.minAge = safesrc.DefaultMinLockAge
	for i := 0; i < len(argv); i++ {
		arg := argv[i]
		switch arg {
		case "-h", "--help":
			fmt.Print(unlockUsage)
			return opts, true, nil
		case "--dry-run", "-n":
			opts.dryRun = true
		case "--min-age":
			rest := argv[i+1:]
			if len(rest) == 0 {
				return opts, false, fmt.Errorf("--min-age requires a duration argument (e.g. 60s)")
			}
			d, perr := time.ParseDuration(rest[0])
			if perr != nil {
				return opts, false, fmt.Errorf("invalid --min-age %q: %w", rest[0], perr)
			}
			opts.minAge = d
			i++
		default:
			if len(arg) > 0 && arg[0] == '-' {
				return opts, false, fmt.Errorf("unknown flag %q (see: src-recovery unlock --help)", arg)
			}
			if opts.repoArg != "" {
				return opts, false, fmt.Errorf("expected exactly one repository path, got a second: %q", arg)
			}
			opts.repoArg = arg
		}
	}
	return opts, false, nil
}

func dirtyNote(dirty bool) string {
	if dirty {
		return " (working tree would be stashed)"
	}
	return ""
}

func stashNote(p safesrc.Plan, repo string) string {
	if p.Stashed {
		return fmt.Sprintf(" — prior changes stashed (recover with: git -C %s stash pop)", repo)
	}
	return ""
}

// openAuditLog appends to ~/.local/state/dear-agent/src-recovery.log AND echoes
// to stderr, so every recovery leaves a durable audit trail and is visible live.
func openAuditLog(home string) (io.Writer, func(), error) {
	dir := filepath.Join(home, ".local", "state", "dear-agent")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, nil, fmt.Errorf("cannot create audit log dir %s: %w", dir, err)
	}
	path := filepath.Join(dir, "src-recovery.log")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, nil, fmt.Errorf("cannot open audit log %s: %w", path, err)
	}
	w := io.MultiWriter(f, os.Stderr)
	return w, func() { _ = f.Close() }, nil
}

const usage = `src-recovery — restore a golden ~/src/<repo> checkout to a clean, current default branch.

Usage:
  src-recovery [--dry-run] [--timeout <dur>] <repo-path-under-~/src>

Flags:
  --dry-run, -n     print the recovery plan; run no mutating git verb
  --timeout <dur>   kill the pull after this long (default 30s; e.g. 60s, 2m)
  -h, --help        show this help

What it does (and the ONLY things it does):
  1. validate the path is strictly under ~/src/
  2. git stash --include-untracked   (only if the tree is dirty; nothing is lost)
  3. git checkout <default-branch>    (origin/HEAD, usually main)
  4. git pull --ff-only               (never a merge/rebase that entangles the tree)

It refuses any other git operation by construction, refuses a tree with
unresolved conflicts (resolve those by hand), and appends an audit line per step
to ~/.local/state/dear-agent/src-recovery.log.

Why this exists:
  ~/src/** is the read-only golden tree; all work belongs in ~/worktrees/**.
  When a checkout drifts (dirty, wrong branch, behind origin), recovering it by
  hand means raw git writes into ~/src — the exact thing the guards forbid. This
  wrapper is the one vetted, narrowly-scoped path to clean it up, so the safe
  action is the only action. See vbonnet/engram-research retrospectives/2026-06-11-src-violations-and-burndown.md.

Examples:
  src-recovery ~/src/dear-agent
  src-recovery --dry-run ~/src/brain-v2
  src-recovery --timeout 60s ~/src/dear-agent

Subcommands:
  unlock   remove a stale .git/index.lock (see: src-recovery unlock --help)
`

const unlockUsage = `src-recovery unlock — remove a stale .git/index.lock from a golden ~/src/<repo>.

Usage:
  src-recovery unlock [--dry-run] [--min-age <dur>] <repo-path-under-~/src>

Flags:
  --dry-run, -n     report what would be removed; delete nothing
  --min-age <dur>   only remove a lock older than this (default 60s; e.g. 5m)
  -h, --help        show this help

What it does (and the ONLY thing it does):
  1. validate the path is strictly under ~/src/
  2. locate <gitdir>/index.lock
  3. remove it iff it is older than --min-age AND no process holds it open

A real index.lock lives for milliseconds while git swaps the index in place; one
older than --min-age with no holder is debris a crashed/killed git left behind.
The lock path is computed, never taken from an argument, so no other file can be
named for deletion. Every decision is appended to
~/.local/state/dear-agent/src-recovery.log.

Why this exists:
  A killed git leaves a zero-byte index.lock that makes every later commit fail
  with exit 128. The daily archive sync then rsyncs new logs but cannot commit
  them — a silent, compounding backlog. Clearing the lock means a raw rm into
  ~/src, which the guards forbid; this is the one vetted path to do it safely.
  See vbonnet/engram-research retrospectives/2026-06-11-src-violations-and-burndown.md.

Examples:
  src-recovery unlock ~/src/ai-conversation-logs
  src-recovery unlock --dry-run ~/src/ai-conversation-logs
  src-recovery unlock --min-age 5m ~/src/dear-agent
`
