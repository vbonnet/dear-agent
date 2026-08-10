// Command external-pr-reviewer polls GitHub pull requests and posts automated
// Codex plus best-effort Gemini reviews using the operator's gh credentials.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/vbonnet/dear-agent/internal/prreviewer"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	fs := flag.NewFlagSet("external-pr-reviewer", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	var (
		repos       multiFlag
		interval    = fs.Duration("interval", 5*time.Minute, "poll interval when --watch is set")
		watch       = fs.Bool("watch", false, "keep polling instead of running one pass")
		limit       = fs.Int("limit", 50, "maximum open PRs to inspect per repo")
		statePath   = fs.String("state", "", "path to JSON state file")
		dryRun      = fs.Bool("dry-run", false, "run providers and print actions without posting or updating state")
		reviewEvent = fs.String("event", string(prreviewer.ReviewComment), "review event: COMMENT, REQUEST_CHANGES, or APPROVE")
		codexCmd    = fs.String("codex-cmd", "codex exec -", "Codex command; prompt is sent on stdin")
		geminiCmd   = fs.String("gemini-cmd", "agy run -", "Gemini/AGY command; prompt is sent on stdin")
		geminiTries = fs.Int("gemini-tries", 2, "Gemini attempts before skipping the secondary review")
	)
	fs.Var(&repos, "repo", "target GitHub repo in owner/name form; repeat for multiple repos")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `usage: external-pr-reviewer --repo owner/name [flags]

Polls open pull requests, detects unseen head SHAs, asks Codex for the primary
review and Gemini/AGY for a best-effort secondary review, then posts via gh pr
review as the current operator. Default event is COMMENT.

flags:
`)
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintf(os.Stderr, "external-pr-reviewer: unexpected argument %q\n", fs.Arg(0))
		return 2
	}
	cfg := prreviewer.Config{
		Repos:       repos,
		Limit:       *limit,
		DryRun:      *dryRun,
		StatePath:   *statePath,
		ReviewEvent: prreviewer.ReviewEvent(strings.ToUpper(strings.TrimSpace(*reviewEvent))),
		CodexCmd:    splitCommand(*codexCmd),
		GeminiCmd:   splitCommand(*geminiCmd),
		GeminiTries: *geminiTries,
	}
	runner := prreviewer.ExecRunner{}
	for {
		if _, err := prreviewer.RunOnce(context.Background(), cfg, runner, os.Stdout); err != nil {
			fmt.Fprintf(os.Stderr, "external-pr-reviewer: %v\n", err)
			return 1
		}
		if !*watch {
			return 0
		}
		time.Sleep(*interval)
	}
}

type multiFlag []string

func (m *multiFlag) String() string {
	return strings.Join(*m, ",")
}

func (m *multiFlag) Set(value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return fmt.Errorf("--repo cannot be empty")
	}
	*m = append(*m, value)
	return nil
}

func splitCommand(s string) []string {
	return strings.Fields(strings.TrimSpace(s))
}
