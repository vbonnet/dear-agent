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
// (see .claude/CLAUDE.md and docs/retros/2026-06-11-src-violations-and-burndown.md).
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
  action is the only action. See docs/retros/2026-06-11-src-violations-and-burndown.md.

Examples:
  src-recovery ~/src/dear-agent
  src-recovery --dry-run ~/src/brain-v2
  src-recovery --timeout 60s ~/src/dear-agent
`
