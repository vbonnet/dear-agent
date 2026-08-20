// Command pr-concern-lint reports whether a pull request mixes a mechanical
// refactor with net-new logic, so the deterministic size-and-scope signal can
// ask for a split on diff shape and not only on diff size.
//
// Usage:
//
//	pr-concern-lint -base <rev> -head <rev> [-repo <dir>] [-min-new-logic <n>] [-github-output]
//
// It is advisory by construction: a mixed diff exits 0 and prints a reason for
// the workflow to fold into its existing split-suggestion comment. Only a usage
// or operational failure exits non-zero, so this can never become the reason a
// PR cannot merge.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/vbonnet/dear-agent/internal/prconcern"
)

func main() {
	os.Exit(mainExitCode())
}

func mainExitCode() int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return run(ctx, os.Args[1:], os.Stdout, os.Stderr)
}

const (
	exitOK    = 0
	exitUsage = 2
)

func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("pr-concern-lint", flag.ContinueOnError)
	flags.SetOutput(stderr)
	var base, head, repo string
	var minNewLogic int
	var githubOutput bool
	flags.StringVar(&base, "base", "", "base revision (the PR's merge target)")
	flags.StringVar(&head, "head", "", "head revision (the PR's tip)")
	flags.StringVar(&repo, "repo", "", "repository directory (defaults to the working directory)")
	flags.IntVar(&minNewLogic, "min-new-logic", prconcern.DefaultNewLogicLines,
		"added non-test source lines that must accompany a move before the diff counts as mixed")
	flags.BoolVar(&githubOutput, "github-output", false,
		"emit mixed_concern= and reason= in GITHUB_OUTPUT form instead of prose")
	if err := flags.Parse(args); err != nil {
		return exitUsage
	}
	if base == "" || head == "" {
		fmt.Fprintln(stderr, "pr-concern-lint: both -base and -head are required")
		flags.Usage()
		return exitUsage
	}

	changes, err := prconcern.Collect(ctx, repo, base, head)
	if err != nil {
		fmt.Fprintf(stderr, "pr-concern-lint: %v\n", err)
		return exitUsage
	}
	analysis := prconcern.Analyze(changes, minNewLogic)

	if githubOutput {
		writeGitHubOutput(stdout, analysis)
		return exitOK
	}
	if !analysis.Mixed {
		fmt.Fprintf(stdout, "single-concern: %d move-only record(s), %d added line(s) of new logic\n",
			len(analysis.MoveOnly), analysis.NewLogicLines)
		return exitOK
	}
	fmt.Fprintln(stdout, analysis.Reason())
	return exitOK
}

// writeGitHubOutput emits the two values the workflow consumes. The reason uses
// a randomless heredoc delimiter that cannot appear in a git path or a rendered
// reason, so a crafted branch or filename cannot terminate the block early and
// inject workflow outputs.
func writeGitHubOutput(w io.Writer, a prconcern.Analysis) {
	fmt.Fprintf(w, "mixed_concern=%t\n", a.Mixed)
	reason := a.Reason()
	if reason == "" {
		fmt.Fprintln(w, "reason=")
		return
	}
	const delim = "PR_CONCERN_REASON_EOF"
	// Defence in depth: strip any line that would close the block early.
	var kept []string
	for line := range strings.SplitSeq(reason, "\n") {
		if strings.TrimSpace(line) == delim {
			continue
		}
		kept = append(kept, line)
	}
	fmt.Fprintf(w, "reason<<%s\n%s\n%s\n", delim, strings.Join(kept, "\n"), delim)
}
