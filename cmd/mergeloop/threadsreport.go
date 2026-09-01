package main

import (
	"context"
	"flag"
	"fmt"
	"time"

	"github.com/vbonnet/dear-agent/internal/mergeloop"
)

// runThreadsReport implements `mergeloop threads --pr N`, a read-only view of
// how one pull request's review threads classify.
//
// It exists because ce-lr7j's definition of done requires confirming on a LIVE
// pull request that a P1 bot thread blocks rather than auto-resolves, and the
// merge gate is otherwise only reachable when a PR happens to be green. It also
// gives the ce-pcz7 triage a way to enumerate the findings that were resolved
// unread. It performs no mutations.
func runThreadsReport(argv []string) error {
	fs := flag.NewFlagSet("threads", flag.ContinueOnError)
	var (
		repo = fs.String("repo", "", "GitHub repo owner/name (auto-detected if empty)")
		pr   = fs.Int("pr", 0, "pull request number")
	)
	if err := fs.Parse(argv); err != nil {
		return err
	}
	if *pr <= 0 {
		return fmt.Errorf("--pr is required")
	}
	target := *repo
	if target == "" {
		var err error
		target, err = detectRepo()
		if err != nil {
			return fmt.Errorf("cannot detect repo: %w (pass --repo owner/name)", err)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	owner, name, ok := splitOwnerRepo(target)
	if !ok {
		return fmt.Errorf("invalid repo %q (want owner/name)", target)
	}
	// dryRun so nothing can mutate even if this grows a resolve path later.
	r := &ghThreadResolver{dryRun: true}
	threads, err := r.listThreads(ctx, owner, name, *pr)
	if err != nil {
		return err
	}

	resolvable, withheld := partitionResolvable(threads)
	findings := blockingFindingsIn(threads)

	fmt.Printf("%s PR #%d: %d review thread(s)\n\n", target, *pr, len(threads))
	for _, t := range threads {
		state := "unresolved"
		if t.isResolved {
			state = "resolved"
		}
		author := "(none)"
		if len(t.comments) > 0 {
			author = t.comments[0].author
		}
		fmt.Printf("  %-12s %-26s severity=%-9s human_reply=%-5t %s\n",
			state, author, mergeloop.ThreadSeverityOf(t.bodies()), t.hasHumanComment(), t.id)
	}

	fmt.Printf("\nauto-resolve: %d resolvable, %d withheld (blocking or unrecognised severity)\n",
		len(resolvable), withheld)
	if len(findings) == 0 {
		fmt.Printf("merge gate:   PASS, no unaddressed blocking bot findings\n")
		return nil
	}
	fmt.Printf("merge gate:   REFUSE, %d unaddressed blocking bot finding(s)\n", len(findings))
	for _, f := range findings {
		fmt.Printf("  - %s/%s: %s\n", f.Author, f.Severity, f.Excerpt)
	}
	return nil
}
