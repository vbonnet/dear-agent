package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"text/tabwriter"

	"github.com/vbonnet/dear-agent/pkg/aggregator"
	"github.com/vbonnet/dear-agent/pkg/aggregator/collectors"
)

func runCollect(ctx context.Context, args []string) int {
	fs := flag.NewFlagSet("collect", flag.ContinueOnError)
	dbPath := fs.String("db", "signals.db", "path to signals.db")
	repoPath := fs.String("repo", ".", "path to the repo / Go module root")
	covFile := fs.String("coverage-file", "",
		"path to a Go coverage profile (e.g. cover.out)")
	lintFile := fs.String("lint-file", "",
		"path to a precomputed golangci-lint JSON output")
	secFile := fs.String("security-file", "",
		"path to a precomputed govulncheck JSON output")
	lookback := fs.Int("lookback-days", 7,
		"git activity lookback window in days")
	asJSON := fs.Bool("json", false, "emit machine-readable JSON")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	store, err := aggregator.OpenSQLiteStore(*dbPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer func() { _ = store.Close() }()

	cs := buildCollectors(*repoPath, *covFile, *lintFile, *secFile, *lookback)
	agg := aggregator.Aggregator{Store: store, Collectors: cs}
	report, err := agg.Run(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(report); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		return collectExitCode(report)
	}

	tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "COLLECTOR\tSIGNALS\tSTATUS")
	names := make([]string, 0, len(report.Collected))
	for name := range report.Collected {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		status := "ok"
		if msg := report.ErrorMsgs[name]; msg != "" {
			status = msg
		}
		fmt.Fprintf(tw, "%s\t%d\t%s\n", name, report.Collected[name], status)
	}
	_ = tw.Flush()
	fmt.Printf("\nfinished in %s\n",
		report.FinishedAt.Sub(report.StartedAt).Round(1e6))
	return collectExitCode(report)
}

// collectExitCode returns 0 if every collector succeeded, 1 if any
// failed. The store insert succeeded — we just want CI to notice
// missing tools or misconfiguration.
func collectExitCode(r aggregator.Report) int {
	for _, e := range r.Errors {
		if e != nil {
			return 1
		}
	}
	return 0
}

func buildCollectors(
	repo, covFile, lintFile, secFile string,
	lookbackDays int,
) []aggregator.Collector {
	cs := []aggregator.Collector{
		&collectors.GitActivity{Repo: repo, LookbackDays: lookbackDays},
		&collectors.LintTrend{Repo: repo, InputFile: lintFile},
		&collectors.DepFreshness{Repo: repo},
		&collectors.SecurityAlerts{Repo: repo, InputFile: secFile},
	}
	// Coverage is opt-in: without a profile we skip rather than try
	// to invoke `go test -cover ./...` from inside a CLI that may
	// not have permission to run tests.
	if covFile != "" {
		cs = append(cs, &collectors.TestCoverage{ProfilePath: covFile})
	}
	return cs
}
