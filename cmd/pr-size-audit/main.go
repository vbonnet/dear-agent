// Command pr-size-audit sweeps recently merged pull requests and reports the
// ones that exceeded the repository's PR size budget or mixed a mechanical
// refactor with net-new logic.
//
// # Why this exists
//
// `.github/workflows/pr-size-scope.yml` flags an oversized PR while it is open,
// and the split-request job asks for a split. Neither records what happened
// next. On 2026-08-19 a sweep of merged history found that every oversized PR
// merged since that gate shipped had received its comment and merged unsplit
// anyway — five for five — and nothing in the system knew. An advisory gate
// with no audit leg is a notification, not a gate: declining costs nothing and
// the decline rate stays invisible.
//
// This is that missing leg. It reads merged history rather than open PRs, so it
// measures outcomes instead of intentions.
//
// # Detect and report only
//
// It opens no PR, reverts nothing, and fails nothing. Following
// `cmd/merge-audit`, remediation is a human decision; this command's whole job
// is to make the trend impossible to not see.
//
// Usage:
//
//	pr-size-audit [-repo <dir>] [-base <rev>] [-limit <n>]
//	              [-max-lines <n>] [-max-files <n>] [-format markdown|text]
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"regexp"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/vbonnet/dear-agent/internal/prconcern"
)

const (
	exitOK    = 0
	exitUsage = 2

	// Defaults mirror CONTRIBUTING.md's "Size budget". They are the TARGET, not
	// the CI ceiling: an audit against the ceiling would report that everything
	// is fine while the median drifts upward underneath it.
	defaultMaxLines = 400
	defaultMaxFiles = 15

	sweepTimeout = 10 * time.Minute
)

// prNumber pulls the PR number out of a squash-merge subject, e.g.
// "fix(disk): alarm on a stale reaper (#1160)". Merges made another way simply
// report no number rather than being dropped from the sweep.
var prNumber = regexp.MustCompile(`\(#(\d+)\)\s*$`)

// merge is one merged change with its measured shape.
type merge struct {
	SHA     string
	Subject string
	PR      string
	Lines   int
	Files   int
	Mixed   bool
}

// OverBudget reports whether the merge broke either budget dimension.
func (m merge) OverBudget(maxLines, maxFiles int) bool {
	return m.Lines > maxLines || m.Files > maxFiles
}

func main() { os.Exit(mainExitCode()) }

func mainExitCode() int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return run(ctx, os.Args[1:], os.Stdout, os.Stderr)
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("pr-size-audit", flag.ContinueOnError)
	flags.SetOutput(stderr)
	var repo, base, format string
	var limit, maxLines, maxFiles int
	flags.StringVar(&repo, "repo", "", "repository directory (defaults to the working directory)")
	flags.StringVar(&base, "base", "origin/main", "branch whose merge history is swept")
	flags.IntVar(&limit, "limit", 50, "number of most recent merges to sweep")
	flags.IntVar(&maxLines, "max-lines", defaultMaxLines, "changed-line budget per PR")
	flags.IntVar(&maxFiles, "max-files", defaultMaxFiles, "changed-file budget per PR")
	flags.StringVar(&format, "format", "markdown", "output format: markdown or text")
	if err := flags.Parse(args); err != nil {
		return exitUsage
	}
	if limit <= 0 {
		fmt.Fprintln(stderr, "pr-size-audit: -limit must be positive")
		return exitUsage
	}
	if format != "markdown" && format != "text" {
		fmt.Fprintf(stderr, "pr-size-audit: unknown -format %q (want markdown or text)\n", format)
		return exitUsage
	}

	ctx, cancel := context.WithTimeout(ctx, sweepTimeout)
	defer cancel()

	merges, err := sweep(ctx, stderr, repo, base, limit)
	if err != nil {
		fmt.Fprintf(stderr, "pr-size-audit: %v\n", err)
		return exitUsage
	}
	writeReport(stdout, merges, maxLines, maxFiles, format)
	return exitOK
}

// sweep measures the most recent `limit` commits on base. A commit that
// can't be read or diffed (e.g. the repository's root commit, which has no
// parent) is skipped rather than voiding the whole sweep, but noted on
// stderr: a partial report still shows the trend, and a silent zero would
// read as "no offenders" instead of "something went wrong".
func sweep(ctx context.Context, stderr io.Writer, repo, base string, limit int) ([]merge, error) {
	shas, err := revList(ctx, repo, base, limit)
	if err != nil {
		return nil, err
	}
	out := make([]merge, 0, len(shas))
	for _, sha := range shas {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		subject, err := commitSubject(ctx, repo, sha)
		if err != nil {
			fmt.Fprintf(stderr, "pr-size-audit: skipping %s: read commit subject: %v\n", sha, err)
			continue
		}
		changes, err := prconcern.Collect(ctx, repo, sha+"^1", sha)
		if err != nil {
			fmt.Fprintf(stderr, "pr-size-audit: skipping %s %q: %v\n", sha, subject, err)
			continue
		}
		m := merge{SHA: sha, Subject: subject}
		if g := prNumber.FindStringSubmatch(subject); g != nil {
			m.PR = g[1]
		}
		for _, c := range changes {
			m.Files++
			if c.Added > 0 {
				m.Lines += c.Added
			}
			if c.Deleted > 0 {
				m.Lines += c.Deleted
			}
		}
		m.Mixed = prconcern.Analyze(changes, 0).Mixed
		out = append(out, m)
	}
	return out, nil
}

func revList(ctx context.Context, repo, base string, limit int) ([]string, error) {
	if strings.HasPrefix(base, "-") {
		return nil, fmt.Errorf("invalid base revision: %q", base)
	}
	args := []string{}
	if repo != "" {
		args = append(args, "-C", repo)
	}
	args = append(args, "rev-list", "--first-parent", fmt.Sprintf("--max-count=%d", limit), base)
	out, err := exec.CommandContext(ctx, "git", args...).Output()
	if err != nil {
		return nil, fmt.Errorf("cannot list merges on %s: %w", base, err)
	}
	return strings.Fields(string(out)), nil
}

func commitSubject(ctx context.Context, repo, sha string) (string, error) {
	args := []string{}
	if repo != "" {
		args = append(args, "-C", repo)
	}
	args = append(args, "log", "-1", "--format=%s", sha)
	out, err := exec.CommandContext(ctx, "git", args...).Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// percentile returns the p-th percentile of a sorted-in-place copy of values.
func percentile(values []int, p float64) int {
	if len(values) == 0 {
		return 0
	}
	s := append([]int(nil), values...)
	sort.Ints(s)
	i := int(float64(len(s)-1) * p)
	return s[i]
}

func writeReport(w io.Writer, merges []merge, maxLines, maxFiles int, format string) {
	if len(merges) == 0 {
		fmt.Fprintln(w, "No merges swept.")
		return
	}
	lines := make([]int, 0, len(merges))
	var over, mixed int
	offenders := make([]merge, 0, len(merges))
	for _, m := range merges {
		lines = append(lines, m.Lines)
		bad := m.OverBudget(maxLines, maxFiles)
		if bad {
			over++
		}
		if m.Mixed {
			mixed++
		}
		if bad || m.Mixed {
			offenders = append(offenders, m)
		}
	}
	// Worst first: the top of the list is where a reader's attention goes.
	sort.SliceStable(offenders, func(i, j int) bool { return offenders[i].Lines > offenders[j].Lines })

	pct := func(n int) float64 { return float64(n) * 100 / float64(len(merges)) }

	if format == "text" {
		fmt.Fprintf(w, "swept=%d over_budget=%d (%.0f%%) mixed_concern=%d median=%d p90=%d max=%d\n",
			len(merges), over, pct(over), mixed,
			percentile(lines, 0.5), percentile(lines, 0.9), percentile(lines, 1))
		for _, m := range offenders {
			fmt.Fprintf(w, "%s\t%d lines\t%d files\tmixed=%t\t%s\n", short(m.SHA), m.Lines, m.Files, m.Mixed, m.Subject)
		}
		return
	}

	fmt.Fprintln(w, "<!-- pr-size-audit -->")
	fmt.Fprintln(w, "## PR size and concern-mixing audit")
	fmt.Fprintf(w, "\nSwept the %d most recent merges against the budget of **%d changed lines / %d changed files** "+
		"(CONTRIBUTING.md — Small, stacked PRs).\n\n", len(merges), maxLines, maxFiles)
	fmt.Fprintf(w, "| Metric | Value |\n|---|---|\n")
	fmt.Fprintf(w, "| Over budget | **%d of %d (%.0f%%)** |\n", over, len(merges), pct(over))
	fmt.Fprintf(w, "| Mixed refactor + new logic | %d (%.0f%%) |\n", mixed, pct(mixed))
	fmt.Fprintf(w, "| Median changed lines | %d |\n", percentile(lines, 0.5))
	fmt.Fprintf(w, "| p90 changed lines | %d |\n", percentile(lines, 0.9))
	fmt.Fprintf(w, "| Largest | %d |\n", percentile(lines, 1))

	if len(offenders) == 0 {
		fmt.Fprintln(w, "\nNo offenders in this window.")
		return
	}
	fmt.Fprintf(w, "\n### Offenders (%d)\n\n| PR | Lines | Files | Mixed | Subject |\n|---|---|---|---|---|\n", len(offenders))
	for _, m := range offenders {
		ref := short(m.SHA)
		if m.PR != "" {
			ref = "#" + m.PR
		}
		flag := ""
		if m.Mixed {
			flag = "yes"
		}
		fmt.Fprintf(w, "| %s | %d | %d | %s | %s |\n", ref, m.Lines, m.Files, flag, sanitize(m.Subject))
	}
	fmt.Fprintln(w, "\nThis report is informational. It reverts nothing and blocks nothing — "+
		"it exists so the trend cannot go unnoticed the way it did before 2026-08-19.")
}

func short(sha string) string {
	if len(sha) > 9 {
		return sha[:9]
	}
	return sha
}

// sanitize keeps a commit subject from breaking the Markdown table it lands in.
func sanitize(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "|", "\\|"), "\n", " ")
}
