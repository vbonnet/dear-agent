// Command doc-audit validates freshness markers and the shrink-only debt
// baseline declared in .dear-agent.yml.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/vbonnet/dear-agent/pkg/docaudit"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("doc-audit", flag.ContinueOnError)
	flags.SetOutput(stderr)
	repo := flags.String("repo", ".", "repository root")
	asOfText := flags.String("as-of", time.Now().UTC().Format("2006-01-02"), "audit date in YYYY-MM-DD form")
	baselineRef := flags.String("baseline-ref", "", "Git ref whose baseline may not be expanded")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 0 {
		fmt.Fprintf(stderr, "doc-audit: unexpected arguments: %v\n", flags.Args())
		return 2
	}
	asOf, err := time.Parse("2006-01-02", *asOfText)
	if err != nil {
		fmt.Fprintf(stderr, "doc-audit: invalid -as-of %q (expected YYYY-MM-DD)\n", *asOfText)
		return 2
	}

	report, err := docaudit.CheckRepository(context.Background(), *repo, docaudit.Options{
		AsOf:        asOf,
		BaselineRef: *baselineRef,
	})
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	if !report.Blocking() {
		fmt.Fprintf(stdout, "ok: %d living document(s), %d baselined freshness finding(s), ratchet intact\n", report.Documents, len(report.Findings))
		return 0
	}

	fmt.Fprintf(stderr, "doc-audit: freshness ratchet failed (%d document(s), %d finding(s))\n", report.Documents, len(report.Findings))
	printFindings(stderr, "new findings", report.NewFindings)
	printEntries(stderr, "stale baseline entries", report.StaleBaseline)
	printEntries(stderr, "baseline additions prohibited by base ref", report.AddedBaseline)
	fmt.Fprintln(stderr, "Fix and remove debt entries; do not stamp an audit date without verifying the document.")
	return 1
}

func printFindings(w io.Writer, label string, findings []docaudit.Finding) {
	if len(findings) == 0 {
		return
	}
	fmt.Fprintf(w, "%s (%d):\n", label, len(findings))
	for _, finding := range findings {
		fmt.Fprintf(w, "  %s\n", finding.ID())
	}
}

func printEntries(w io.Writer, label string, entries []docaudit.BaselineEntry) {
	if len(entries) == 0 {
		return
	}
	fmt.Fprintf(w, "%s (%d):\n", label, len(entries))
	for _, entry := range entries {
		fmt.Fprintf(w, "  %s\n", entry.ID())
	}
}
