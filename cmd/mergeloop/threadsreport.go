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
	remaining := threadsRemainingUnresolved(threads, resolvable)

	fmt.Printf("%s PR #%d: %d review thread(s)\n\n", target, *pr, len(threads))
	printThreadRows(threads)
	fmt.Printf("\nauto-resolve:      %d resolvable, %d withheld (blocking or unrecognised severity)\n",
		len(resolvable), withheld)
	printGateVerdicts(findings, remaining)
	return nil
}

// printThreadRows renders one line per review thread: its resolution state,
// opening author, classified severity, and whether a person replied.
func printThreadRows(threads []reviewThread) {
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
}

// printGateVerdicts reports the two INDEPENDENT gates separately, then the
// overall verdict. Conflating them is what let this command print a bare
// "merge gate: PASS" for a PR safe-merge would refuse.
func printGateVerdicts(findings []mergeloop.BlockingFinding, remaining []string) {
	if len(findings) == 0 {
		fmt.Printf("bot-finding gate:  PASS, no unaddressed blocking bot findings\n")
	} else {
		fmt.Printf("bot-finding gate:  REFUSE, %d unaddressed blocking bot finding(s)\n", len(findings))
		for _, f := range findings {
			fmt.Printf("  - %s/%s: %s\n", f.Author, f.Severity, f.Excerpt)
		}
	}

	// GitHub's required_conversation_resolution, which safe-merge also enforces.
	if len(remaining) == 0 {
		fmt.Printf("conversation gate: PASS, every thread is resolved or auto-resolvable\n")
	} else {
		fmt.Printf("conversation gate: REFUSE, %d thread(s) stay unresolved after auto-resolve\n", len(remaining))
		for _, id := range remaining {
			fmt.Printf("  - %s\n", id)
		}
	}

	if len(findings) == 0 && len(remaining) == 0 {
		fmt.Printf("merge gate:        PASS\n")
	} else {
		fmt.Printf("merge gate:        REFUSE\n")
	}
}

// threadsRemainingUnresolved returns the IDs of threads that will still be
// unresolved after the auto-resolve step runs, and so will still be held by
// GitHub's required_conversation_resolution.
//
// This is deliberately separate from blockingFindingsIn, which is only the
// custom bot-finding gate. A human thread, or a bot thread whose severity this
// code does not recognise, yields no blocking FINDING but is still left open,
// and safe-merge's mandatory unresolved-thread check refuses the merge on it.
// Reporting the bot-finding gate alone as "merge gate: PASS" told an operator
// the merge would proceed when it would not.
func threadsRemainingUnresolved(threads []reviewThread, resolvable []botThread) []string {
	willResolve := make(map[string]bool, len(resolvable))
	for _, b := range resolvable {
		willResolve[b.id] = true
	}
	var out []string
	for _, t := range threads {
		if t.isResolved || willResolve[t.id] {
			continue
		}
		out = append(out, t.id)
	}
	return out
}
