// bead-pr-sync reconciles bead status against GitHub PR merge state.
//
// A bead is a DoD violation when it is CLOSED but its referenced PR is not
// yet merged. This tool finds those cases and reopens the bead to in_progress.
//
// Usage:
//
//	bead-pr-sync [--dry-run] [--repo owner/name] [--beads-dir /path]
package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr *os.File) int {
	fs := flag.NewFlagSet("bead-pr-sync", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var (
		dryRun   = fs.Bool("dry-run", false, "report mismatches without reopening beads")
		repo     = fs.String("repo", "", "GitHub repo in owner/name form (defaults to git remote)")
		beadsDir = fs.String("beads-dir", "", "path to the .beads directory (overrides BEADS_DIR)")
		limit    = fs.Int("limit", 200, "maximum number of closed beads to scan")
	)
	fs.Usage = func() {
		fmt.Fprintf(stderr, "usage: bead-pr-sync [flags]\n\n"+
			"Finds CLOSED beads whose referenced PR is not yet merged and reopens them.\n\n"+
			"flags:\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return 2
	}

	if *repo == "" {
		detected, err := detectRepo()
		if err != nil {
			fmt.Fprintf(stderr, "error: cannot detect GitHub repo: %v\n", err)
			fmt.Fprintf(stderr, "hint: run inside a git repo or pass --repo owner/name\n")
			return 1
		}
		*repo = detected
	}

	cfg := Config{
		DryRun:   *dryRun,
		Repo:     *repo,
		BeadsDir: *beadsDir,
		Limit:    *limit,
	}

	stats, err := Reconcile(cfg, stdout, stderr)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}

	// In dry-run nothing is reopened, so report the count that *would* be
	// fixed (every violation) rather than the always-zero Fixed counter.
	mode, count := "fixed", stats.Fixed
	if cfg.DryRun {
		mode, count = "would fix", stats.Violations
	}
	fmt.Fprintf(stdout, "\nscanned %d closed beads: %d violations found, %d %s\n",
		stats.Scanned, stats.Violations, count, mode)
	return 0
}
