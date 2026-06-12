// safe-pr wraps `gh pr create` and `gh pr close` with guards that enforce
// the one-PR-per-bead rule, an open-PR cap, and bead dedup before creation.
//
// Motivation (CLAUDE.md principle 9): PR open/close are high-cost operations
// that trigger Gemini quota, CI minutes, deepsec, and codeql. Raw `gh pr create`
// bypasses all guards — agents under pressure will emit duplicate PRs, exceed
// the open-PR cap, or create PRs for beads that already have one open.
//
// Subcommands:
//
//	safe-pr create [--bead <id>] [--repo owner/name] [-- gh pr create args...]
//	safe-pr close  <number>      [--repo owner/name]
//	safe-pr status               [--repo owner/name]
//
// ALWAYS_ALLOW this binary; deny raw `gh pr create/close` via PreToolUse hook
// (see .claude/hooks/pretool-bash-write-guard or equivalent).
package main

import (
	"fmt"
	"os"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr *os.File) int {
	if len(args) == 0 {
		printUsage(stderr)
		return 2
	}

	switch args[0] {
	case "create":
		return runCreate(args[1:], stdout, stderr)
	case "close":
		return runClose(args[1:], stdout, stderr)
	case "status":
		return runStatus(args[1:], stdout, stderr)
	case "--help", "-h", "help":
		printUsage(stdout)
		return 0
	default:
		fmt.Fprintf(stderr, "safe-pr: unknown subcommand %q\n", args[0])
		printUsage(stderr)
		return 2
	}
}

func printUsage(w *os.File) {
	fmt.Fprintf(w, `safe-pr — guarded gh pr create / close

Usage:
  safe-pr create [--bead <id>] [--repo owner/name] [--cap N] [-- <gh pr create args...>]
  safe-pr close  <number>      [--repo owner/name]
  safe-pr status               [--repo owner/name]

Guards enforced by 'create':
  1. One-PR-per-bead: if an open PR already cites the bead, abort (exit 2).
  2. Open-PR cap: if total open PRs >= cap (default %d), abort (exit 2).

Exit codes:
  0   Success (or status output).
  2   Guard triggered or bad arguments.
  1   Unexpected error (gh unavailable, network failure, etc.).
`, defaultCap)
}
