// safe-push pushes a git branch without ever hanging on the macOS keychain
// credential helper and without force-pushing protected/default branches.
//
// Usage:
//
//	safe-push [-C <repo-dir>] [--timeout <dur>] [git push args...]
//
// Examples:
//
//	safe-push -u origin my-branch
//	safe-push -C ~/worktrees/dear-agent/foo -u origin foo
//	safe-push --timeout 60s origin HEAD:main
//
// Every push runs with the credential helper chain reset to the GitHub CLI
// helper only (see internal/safegit), GIT_TERMINAL_PROMPT=0, and a hard
// timeout, so a wedged osxkeychain helper turns into a fast, clear failure
// instead of an indefinite block. Force-push flags are rejected.
package main

import (
	"fmt"
	"os"
	"time"

	"github.com/vbonnet/dear-agent/internal/safegit"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "\nsafe-push: %v\n", err)
		os.Exit(1)
	}
}

func run(argv []string) error {
	repoDir := ""
	timeout := safegit.DefaultTimeout
	var pushArgs []string

	for i := 0; i < len(argv); i++ {
		arg := argv[i]
		switch arg {
		case "-h", "--help":
			fmt.Print(usage)
			return nil
		case "-C":
			if i+1 >= len(argv) {
				return fmt.Errorf("-C requires a repository directory argument")
			}
			repoDir = argv[i+1]
			i++
		case "--timeout":
			if i+1 >= len(argv) {
				return fmt.Errorf("--timeout requires a duration argument (e.g. 30s)")
			}
			d, err := time.ParseDuration(argv[i+1])
			if err != nil {
				return fmt.Errorf("invalid --timeout %q: %w", argv[i+1], err)
			}
			timeout = d
			i++
		default:
			// Everything else is forwarded verbatim to `git push`.
			pushArgs = append(pushArgs, arg)
		}
	}

	return safegit.Push(repoDir, pushArgs, timeout)
}

const usage = `safe-push — push git without hanging on the keychain helper.

Usage:
  safe-push [-C <repo-dir>] [--timeout <dur>] [git push args...]

Flags:
  -C <repo-dir>     run the push in this repository (like git -C)
  --timeout <dur>   kill the push after this long (default 30s; e.g. 60s, 2m)
  -h, --help        show this help

All other arguments are passed straight through to ` + "`git push`" + `.

Examples:
  safe-push -u origin my-branch
  safe-push -C ~/worktrees/dear-agent/foo -u origin foo
  safe-push -C ~/worktrees/dear-agent/foo --force-with-lease origin foo
  safe-push --timeout 60s origin HEAD:main

Force-push policy:
  safe-push allows force-pushes only to non-default PR branches, preferably via
  --force-with-lease. It refuses force-pushes to main, master, and the repo's
  configured default branch. --mirror remains refused.

Why this exists:
  The macOS osxkeychain credential helper sits first in git's generic helper
  chain and can block on a GUI auth dialog in a headless session, hanging the
  push forever. safe-push resets the helper chain to the GitHub CLI helper only
  (gh auth git-credential) for every host, sets GIT_TERMINAL_PROMPT=0, and
  bounds the push with a timeout. Force-push is rejected by construction.
`
