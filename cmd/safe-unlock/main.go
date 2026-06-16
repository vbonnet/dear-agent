// Command safe-unlock is the authorised, vetted path for clearing stale git
// lock files from a repository or linked worktree.
//
// It enforces CLAUDE.md principle 9: an atomic wrapper that makes the safe path
// the only path. A raw `rm .git/index.lock` races any git that is genuinely
// live; safe-unlock removes a lock only when it is provably stale — older than
// --min-age AND held open by no process — and refuses (non-zero exit) any lock a
// running git still holds. It generalises src-recovery's ~/src-scoped `unlock`
// to any checkout, most importantly the ~/worktrees/** trees where agents work,
// and to the full family of git lock files (index, refs, config, packed-refs)
// including those inside linked worktrees.
//
// Every decision is appended to ~/.local/state/dear-agent/safe-unlock-audit.jsonl.
//
// Usage:
//
//	safe-unlock [--dry-run] [--min-age <dur>] [--include-worktrees] [<repo-path>]
//
// repo-path defaults to the current directory, so a wedged agent can just run
// `safe-unlock` from inside the affected worktree.
//
// Examples:
//
//	safe-unlock                                  # clean the current checkout
//	safe-unlock ~/worktrees/dear-agent/feat-x    # clean a specific worktree
//	safe-unlock --dry-run ~/src/dear-agent       # report, remove nothing
//	safe-unlock --min-age 30s .                  # treat 30s-old locks as stale
//
// Exit codes:
//
//	0  success — stale locks removed and/or nothing to do
//	1  operational error (not a git repo, unreadable git dir, removal failed)
//	2  a lock is present but ACTIVE (a real git is running); nothing removable
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/vbonnet/dear-agent/internal/safeunlock"
)

const usage = `safe-unlock — remove provably-stale git lock files

USAGE:
    safe-unlock [flags] [<repo-path>]

ARGS:
    <repo-path>    checkout or worktree to clean (default: current directory)

FLAGS:
    -n, --dry-run            report what would be removed; remove nothing
        --min-age <dur>      minimum lock age to treat as stale (default 2m)
        --include-worktrees  also scan linked worktrees' lock files (default true)
    -h, --help               show this help

A lock is removed only when it is BOTH older than --min-age AND held open by no
process (checked with lsof). An active lock is left in place and reported with
exit code 2. Every decision is appended to
~/.local/state/dear-agent/safe-unlock-audit.jsonl.
`

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(argv []string) int {
	fs := flag.NewFlagSet("safe-unlock", flag.ContinueOnError)
	fs.Usage = func() { fmt.Print(usage) }

	var dryRun bool
	fs.BoolVar(&dryRun, "dry-run", false, "report what would be removed; remove nothing")
	fs.BoolVar(&dryRun, "n", false, "report what would be removed; remove nothing (shorthand)")
	minAge := fs.Duration("min-age", safeunlock.DefaultMinLockAge, "minimum lock age to treat as stale")
	includeWorktrees := fs.Bool("include-worktrees", true, "also scan linked worktrees' lock files")

	if err := fs.Parse(argv); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		fmt.Fprintf(os.Stderr, "safe-unlock: %v\n", err)
		return 1
	}

	repo := "."
	switch fs.NArg() {
	case 0:
		// default: current directory
	case 1:
		repo = fs.Arg(0)
	default:
		fmt.Fprintf(os.Stderr, "safe-unlock: expected at most one repo path, got %d\n", fs.NArg())
		return 1
	}

	c := &safeunlock.Cleaner{
		Repo:             repo,
		DryRun:           dryRun,
		MinAge:           *minAge,
		IncludeWorktrees: *includeWorktrees,
		Now:              time.Now(),
		Log:              os.Stdout,
	}

	results, err := c.Clean()
	if err != nil && len(results) == 0 {
		// Could not scan at all (not a git repo, unreadable git dir).
		fmt.Fprintf(os.Stderr, "safe-unlock: %v\n", err)
		return 1
	}

	var removed, active int
	for _, r := range results {
		if r.Removed {
			removed++
		}
		if r.Active {
			active++
		}
	}

	switch {
	case len(results) == 0:
		fmt.Printf("safe-unlock: %s — no git lock files present, nothing to do\n", repo)
	case dryRun:
		fmt.Printf("safe-unlock (dry-run): %s — %d removable stale lock(s), %d active (left in place)\n",
			repo, len(results)-active, active)
	default:
		fmt.Printf("safe-unlock: %s — removed %d stale lock(s), %d active (left in place)\n",
			repo, removed, active)
	}

	// A per-lock failure (e.g. a removal that hit a permission error) does not
	// abort the others, but it is still an operational error: surface it and
	// exit 1, taking precedence over the active-lock signal.
	if err != nil {
		fmt.Fprintf(os.Stderr, "safe-unlock: %v\n", err)
		return 1
	}

	if active > 0 {
		fmt.Fprintf(os.Stderr,
			"safe-unlock: %d lock(s) are still ACTIVE — a git operation is in flight; "+
				"wait for it to finish, then re-run\n", active)
		return 2
	}
	return 0
}
