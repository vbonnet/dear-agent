// Command safe-merge is the only approved path for merging a GitHub PR.
//
// It enforces the merge predicate from docs/design-safe-merge.md §4.2:
//   - PR is OPEN and not a draft
//   - No merge conflicts
//   - Every check run on the head SHA is COMPLETED SUCCESS/NEUTRAL/SKIPPED
//   - No unresolved review threads
//   - Atomic merge with expectedHeadOid (TOCTOU-safe)
//   - Post-merge worktree + branch cleanup
//
// Raw "gh pr merge" is blocked by the pretool-bash-write-guard hook; this
// wrapper is ALWAYS_ALLOW'd because its safety is guaranteed by construction.
//
// Usage:
//
//	safe-merge <pr-number> [--repo owner/name] [--timeout 45m] [--dry-run]
package main

import (
	"fmt"
	"os"
	"time"

	"github.com/vbonnet/dear-agent/internal/safemerge"
)

const usage = `safe-merge — disciplined PR merge wrapper

Usage:
  safe-merge <pr-number> [flags]

Flags:
  --repo <owner/name>   GitHub repo (auto-detected from remote if omitted)
  --timeout <duration>  How long to wait for pending checks (default: 45m)
  --dry-run             Check all guards without merging

safe-merge refuses to merge if any of the following hold:
  · PR is not OPEN or is a draft
  · PR has merge conflicts
  · Any check run on the head SHA is not SUCCESS/NEUTRAL/SKIPPED
  · Any review thread is unresolved

Post-merge: removes the local worktree (if checked out) and deletes the
local branch. The remote branch is deleted by --delete-branch.

See docs/design-safe-merge.md for the full design rationale.
`

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "\nsafe-merge: %v\n", err)
		os.Exit(1)
	}
}

func run(argv []string) error {
	cfg := safemerge.Config{
		Timeout: 45 * time.Minute,
	}

	var positional []string
	for i := 0; i < len(argv); i++ {
		arg := argv[i]
		switch arg {
		case "-h", "--help":
			fmt.Print(usage)
			return nil
		case "--repo":
			if i+1 >= len(argv) {
				return fmt.Errorf("--repo requires an argument")
			}
			cfg.Repo = argv[i+1]
			i++
		case "--timeout":
			if i+1 >= len(argv) {
				return fmt.Errorf("--timeout requires a duration (e.g. 45m)")
			}
			d, err := time.ParseDuration(argv[i+1])
			if err != nil {
				return fmt.Errorf("invalid --timeout %q: %w", argv[i+1], err)
			}
			cfg.Timeout = d
			i++
		case "--dry-run":
			cfg.DryRun = true
		default:
			if len(arg) > 0 && arg[0] == '-' {
				return fmt.Errorf("unknown flag %q\n\nDid you mean to use 'safe-merge <pr> --repo ...'?\n\n%s", arg, usage)
			}
			positional = append(positional, arg)
		}
	}

	if len(positional) < 1 {
		return fmt.Errorf("PR number is required\n\n%s", usage)
	}
	if len(positional) > 1 {
		return fmt.Errorf("unexpected arguments: %v", positional[1:])
	}
	cfg.PR = positional[0]

	return safemerge.Merge(cfg)
}
