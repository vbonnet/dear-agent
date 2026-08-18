// Command external-pr-reviewer polls GitHub pull requests and posts automated
// Codex plus best-effort Gemini reviews using the operator's gh credentials.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
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
		repos           multiFlag
		interval        = fs.Duration("interval", 5*time.Minute, "poll interval when --watch is set")
		watch           = fs.Bool("watch", false, "keep polling instead of running one pass")
		limit           = fs.Int("limit", 50, "maximum open PRs to inspect per repo")
		statePath       = fs.String("state", "", "path to JSON state file")
		dryRun          = fs.Bool("dry-run", false, "run providers and print actions without posting or updating state")
		reviewEvent     = fs.String("event", string(prreviewer.ReviewComment), "review event: COMMENT, REQUEST_CHANGES, or APPROVE")
		codexCmd        = fs.String("codex-cmd", "codex exec -", "Codex command; prompt is sent on stdin")
		geminiCmd       = fs.String("gemini-cmd", "agy run -", "Gemini/AGY command; prompt is sent on stdin")
		geminiTries     = fs.Int("gemini-tries", 2, "Gemini attempts before skipping the secondary review")
		providerTimeout = fs.Duration("provider-timeout", 10*time.Minute, "maximum run time for a single provider invocation")
		retryDelay      = fs.Duration("retry-delay", 5*time.Second, "delay between secondary provider attempts")
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
	if *watch && *interval <= 0 {
		fmt.Fprintf(os.Stderr, "external-pr-reviewer: --interval must be positive when --watch is set, got %s\n", *interval)
		return 2
	}
	codexArgv, err := splitCommand(*codexCmd)
	if err != nil {
		fmt.Fprintf(os.Stderr, "external-pr-reviewer: --codex-cmd: %v\n", err)
		return 2
	}
	geminiArgv, err := splitCommand(*geminiCmd)
	if err != nil {
		fmt.Fprintf(os.Stderr, "external-pr-reviewer: --gemini-cmd: %v\n", err)
		return 2
	}
	cfg := prreviewer.Config{
		Repos:           repos,
		Limit:           *limit,
		DryRun:          *dryRun,
		StatePath:       *statePath,
		ReviewEvent:     prreviewer.ReviewEvent(strings.ToUpper(strings.TrimSpace(*reviewEvent))),
		CodexCmd:        codexArgv,
		GeminiCmd:       geminiArgv,
		GeminiTries:     *geminiTries,
		ProviderTimeout: *providerTimeout,
		RetryDelay:      *retryDelay,
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	runner := prreviewer.ExecRunner{}
	for {
		if _, err := prreviewer.RunOnce(ctx, cfg, runner, os.Stdout); err != nil {
			if errors.Is(err, context.Canceled) && ctx.Err() != nil {
				return 0
			}
			fmt.Fprintf(os.Stderr, "external-pr-reviewer: %v\n", err)
			return 1
		}
		if !*watch {
			return 0
		}
		timer := time.NewTimer(*interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return 0
		case <-timer.C:
		}
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

// splitCommand splits a provider command into an argument vector, honouring
// single and double quotes so an executable path containing spaces survives.
func splitCommand(s string) ([]string, error) {
	var (
		argv    []string
		current strings.Builder
		quote   rune
		open    bool
	)
	for _, r := range s {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
				continue
			}
			current.WriteRune(r)
		case r == '\'' || r == '"':
			quote = r
			open = true
		case r == ' ' || r == '\t' || r == '\n' || r == '\r':
			if current.Len() > 0 || open {
				argv = append(argv, current.String())
				current.Reset()
				open = false
			}
		default:
			current.WriteRune(r)
		}
	}
	if quote != 0 {
		return nil, fmt.Errorf("unbalanced %q quote in %q", string(quote), s)
	}
	if current.Len() > 0 || open {
		argv = append(argv, current.String())
	}
	return argv, nil
}
