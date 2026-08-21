// Command crap-lint scores the functions a pull request changed by joining
// cyclomatic complexity with test coverage, so the deterministic review signal
// can ask for tests on the branchy code a diff actually touched.
//
// Usage:
//
//	crap-lint -base <rev> -head <rev> [-repo <dir>] [-threshold <n>] [-github-output]
//
// It is advisory by construction: a flagged diff exits 0 and prints a Markdown
// body for the workflow to fold into its existing comment. Only a usage or
// operational failure exits non-zero, so this can never become the reason a PR
// cannot merge.
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

	"github.com/vbonnet/dear-agent/internal/craplens"
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
	flags := flag.NewFlagSet("crap-lint", flag.ContinueOnError)
	flags.SetOutput(stderr)
	var base, head, repo string
	var threshold float64
	var githubOutput bool
	flags.StringVar(&base, "base", "", "base revision (the PR's merge target)")
	flags.StringVar(&head, "head", "", "head revision (the PR's tip)")
	flags.StringVar(&repo, "repo", "", "repository directory (defaults to the working directory)")
	flags.Float64Var(&threshold, "threshold", craplens.DefaultThreshold,
		"CRAP score above which a changed function is reported individually")
	flags.BoolVar(&githubOutput, "github-output", false,
		"emit crap_flagged= and crap_report= in GITHUB_OUTPUT form instead of prose")
	if err := flags.Parse(args); err != nil {
		return exitUsage
	}
	if base == "" || head == "" {
		fmt.Fprintln(stderr, "crap-lint: both -base and -head are required")
		flags.Usage()
		return exitUsage
	}

	report, err := craplens.Analyze(ctx, repo, base, head, threshold)
	if err != nil {
		fmt.Fprintf(stderr, "crap-lint: %v\n", err)
		return exitUsage
	}

	if githubOutput {
		writeGitHubOutput(stdout, report)
		return exitOK
	}
	if !report.Flagged() {
		fmt.Fprint(stdout, unflaggedSummary(report))
		return exitOK
	}
	fmt.Fprint(stdout, report.Render())
	return exitOK
}

// unflaggedSummary describes a run that produced no findings.
//
// "Clean" is only honest when something was actually measured. A checkout
// mismatch, or a run where coverage failed for every touched package, produces
// no findings for the same reason an empty diff does, and reporting those the
// same way would make an unusable measurement indistinguishable from a healthy
// diff.
func unflaggedSummary(r craplens.Report) string {
	if r.CheckoutMismatch {
		return fmt.Sprintf("not measured: the working tree is not at the head revision or has uncommitted changes; %d changed function(s) were left unscored\n", r.Changed)
	}
	if r.Scored == 0 && len(r.Unknown) > 0 {
		return fmt.Sprintf("not measured: coverage could not be collected for any of the %d touched package(s) (%s); %d changed function(s) were left unscored\n",
			len(r.Unknown), strings.Join(r.Unknown, ", "), r.Changed)
	}
	summary := fmt.Sprintf("clean: %d scored changed function(s), %d at or under %.0f, none over %.0f",
		r.Scored, r.WithinAgentTarget, craplens.AgentTarget, r.Threshold)
	if len(r.Unknown) > 0 {
		summary += fmt.Sprintf("; %d package(s) unmeasured (%s)", len(r.Unknown), strings.Join(r.Unknown, ", "))
	}
	if r.Unmeasured > 0 {
		summary += fmt.Sprintf("; %d function(s) unmeasured on this platform", r.Unmeasured)
	}
	return summary + "\n"
}

// writeGitHubOutput emits the two values the workflow consumes. The report uses
// a fixed heredoc delimiter that cannot appear in a Go identifier or a git
// path, and any line equal to it is stripped, so a crafted branch or filename
// cannot terminate the block early and inject workflow outputs.
func writeGitHubOutput(w io.Writer, r craplens.Report) {
	fmt.Fprintf(w, "crap_flagged=%t\n", r.Flagged())
	body := r.Render()
	if body == "" {
		fmt.Fprintln(w, "crap_report=")
		return
	}
	const delim = "CRAP_LINT_REPORT_EOF"
	// The report ends with a newline; splitting it as-is yields a trailing
	// empty element and so an extra blank line before the closing delimiter.
	var kept []string
	for line := range strings.SplitSeq(strings.TrimRight(body, "\n"), "\n") {
		if strings.TrimSpace(line) == delim {
			continue
		}
		kept = append(kept, line)
	}
	fmt.Fprintf(w, "crap_report<<%s\n%s\n%s\n", delim, strings.Join(kept, "\n"), delim)
}
