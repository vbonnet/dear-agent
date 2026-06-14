// Command safe-rebase rebases a feature branch onto the latest base branch
// with safety checks: refuses to operate on protected branches, aborts cleanly
// on conflicts, and optionally force-pushes + runs preflight in --auto mode.
//
// Usage:
//
//	safe-rebase [-C <repo-dir>] [--base <branch>] [--auto] [--dry-run]
//
// Examples:
//
//	safe-rebase                          # rebase current branch onto main
//	safe-rebase --base develop           # rebase onto develop
//	safe-rebase --auto                   # rebase + force-push + preflight
//	safe-rebase -C ~/worktrees/foo/bar   # operate on a specific worktree
package main

import (
	"fmt"
	"os"
	"time"

	"github.com/vbonnet/dear-agent/internal/safegit"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "\nsafe-rebase: %v\n", err)
		os.Exit(1)
	}
}

func parseRebaseArgs(argv []string) (*safegit.RebaseConfig, bool, error) {
	cfg := safegit.RebaseConfig{}
	for i := 0; i < len(argv); i++ {
		arg := argv[i]
		switch arg {
		case "-h", "--help":
			fmt.Print(usage)
			return nil, true, nil
		case "-C":
			if i+1 >= len(argv) {
				return nil, false, fmt.Errorf("-C requires a repository directory argument")
			}
			cfg.RepoDir = argv[i+1]
			i++
		case "--base":
			if i+1 >= len(argv) {
				return nil, false, fmt.Errorf("--base requires a branch name argument")
			}
			cfg.BaseBranch = argv[i+1]
			i++
		case "--auto":
			cfg.Auto = true
		case "--dry-run":
			cfg.DryRun = true
		case "--timeout":
			if i+1 >= len(argv) {
				return nil, false, fmt.Errorf("--timeout requires a duration argument (e.g. 30s)")
			}
			d, err := time.ParseDuration(argv[i+1])
			if err != nil {
				return nil, false, fmt.Errorf("invalid --timeout %q: %w", argv[i+1], err)
			}
			cfg.Timeout = d
			i++
		default:
			return nil, false, fmt.Errorf("unknown flag %q — see safe-rebase --help", arg)
		}
	}
	return &cfg, false, nil
}

func run(argv []string) error {
	cfg, help, err := parseRebaseArgs(argv)
	if err != nil {
		return err
	}
	if help {
		return nil
	}

	result, err := safegit.SafeRebase(*cfg)
	if err != nil {
		if result != nil && len(result.Conflicts) > 0 {
			fmt.Fprintf(os.Stderr, "\nConflicting files:\n")
			for _, f := range result.Conflicts {
				fmt.Fprintf(os.Stderr, "  %s\n", f)
			}
		}
		return err
	}

	fmt.Fprintf(os.Stderr, "\nsafe-rebase: done — %s rebased onto %s (%d commit(s))\n",
		result.Branch, result.BaseBranch, result.CommitsAfter)
	if result.Pushed {
		fmt.Fprintln(os.Stderr, "safe-rebase: branch force-pushed to origin")
	}
	if result.Preflight {
		fmt.Fprintln(os.Stderr, "safe-rebase: preflight passed")
	}
	return nil
}

const usage = `safe-rebase — rebase a feature branch onto main with safety checks.

This is the approved merge strategy for agents handling stale PRs (CLAUDE.md).
Force-push is legitimate ONLY on feature branches during rebase — this wrapper
enforces that invariant by refusing to operate on protected branches.

Usage:
  safe-rebase [-C <repo-dir>] [--base <branch>] [--auto] [--dry-run]

Flags:
  -C <repo-dir>     operate on this repository / worktree
  --base <branch>   branch to rebase onto (default: main)
  --auto            after clean rebase: force-push + run make preflight
  --dry-run         report what would happen without doing it
  --timeout <dur>   timeout for network ops (default 30s)
  -h, --help        show this help

Behavior:
  1. Fetches origin/<base> to get the latest state
  2. Runs git rebase origin/<base> on the current branch
  3. On conflict: aborts the rebase, reports conflicting files, exits non-zero
  4. On clean rebase: reports success
  5. With --auto: force-pushes (--force-with-lease) + runs make preflight

Safety:
  - REFUSES to operate on protected branches (main, master, develop, release)
  - Uses --force-with-lease (not --force) to protect against upstream changes
  - Network ops bounded by timeout + GIT_TERMINAL_PROMPT=0
  - Conflicts are NEVER auto-resolved — agents must report them for human review

Audit log:
  Every operation is logged to ~/.local/state/dear-agent/safe-rebase-audit.jsonl
  (override with SAFE_MERGE_AUDIT_DIR).
`
