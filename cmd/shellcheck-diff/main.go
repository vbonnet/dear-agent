// Command shellcheck-diff reports only the ShellCheck findings a change
// introduced.
//
// A repository-wide ShellCheck gate at anything stricter than -S error fails
// on pre-existing debt, so it can never be turned on. A gate scoped to whole
// changed files fails a one-line edit to a legacy script for findings the
// author did not write. Both outcomes teach contributors to route around the
// gate.
//
// This command applies the rule the Go linter already uses in this repository
// (.golangci.yml new-from-merge-base): a finding blocks only when it sits on a
// line the change added or rewrote. Legacy debt stays visible in the nightly
// full sweep without blocking unrelated work.
//
// Inputs are a unified diff and a ShellCheck JSON1 document, so the decision
// logic is a pure function over two files and is unit tested rather than
// buried in a workflow's run block.
//
//	shellcheck -f json1 -S style $(changed scripts) > findings.json
//	git diff -U0 "$base...$head" -- '*.sh' > changed.patch
//	shellcheck-diff --diff changed.patch --findings findings.json --min-severity warning
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
)

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "shellcheck-diff: %v\n", err)
		os.Exit(2)
	}
}

func run(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("shellcheck-diff", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	diffPath := fs.String("diff", "", "path to a unified diff produced with `git diff -U0` (required)")
	findingsPath := fs.String("findings", "", "path to a ShellCheck JSON1 document (required)")
	minSeverity := fs.String("min-severity", "warning", "lowest severity that fails: error, warning, info or style")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *diffPath == "" || *findingsPath == "" {
		return errors.New("both --diff and --findings are required")
	}
	threshold, err := severityRank(*minSeverity)
	if err != nil {
		return err
	}

	diffBytes, err := os.ReadFile(*diffPath)
	if err != nil {
		return fmt.Errorf("read diff: %w", err)
	}
	findingsBytes, err := os.ReadFile(*findingsPath)
	if err != nil {
		return fmt.Errorf("read findings: %w", err)
	}

	touched, err := parseTouchedLines(string(diffBytes))
	if err != nil {
		return fmt.Errorf("parse diff: %w", err)
	}
	findings, err := parseFindings(findingsBytes)
	if err != nil {
		return fmt.Errorf("parse findings: %w", err)
	}

	blocking := selectBlocking(findings, touched, threshold)
	for _, f := range blocking {
		fmt.Fprintf(stdout, "%s:%d:%d: %s: %s [SC%d]\n", f.File, f.Line, f.Column, f.Level, f.Message, f.Code)
	}
	if len(blocking) > 0 {
		return fmt.Errorf("%d ShellCheck finding(s) on changed lines at or above %s", len(blocking), *minSeverity)
	}
	fmt.Fprintf(stdout, "No ShellCheck findings at or above %s on changed lines (%d finding(s) inspected).\n",
		*minSeverity, len(findings))
	return nil
}

// selectBlocking keeps the findings that are both severe enough and anchored to
// a line the change touched, ordered for stable output.
func selectBlocking(findings []Finding, touched TouchedLines, threshold int) []Finding {
	var blocking []Finding
	for _, f := range findings {
		rank, err := severityRank(f.Level)
		if err != nil || rank > threshold {
			continue
		}
		if touched.Contains(f.File, f.Line) {
			blocking = append(blocking, f)
		}
	}
	sort.SliceStable(blocking, func(i, j int) bool {
		if blocking[i].File != blocking[j].File {
			return blocking[i].File < blocking[j].File
		}
		if blocking[i].Line != blocking[j].Line {
			return blocking[i].Line < blocking[j].Line
		}
		return blocking[i].Column < blocking[j].Column
	})
	return blocking
}

// severityRank orders ShellCheck levels from most to least severe so a
// threshold comparison is a single integer comparison.
func severityRank(level string) (int, error) {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "error":
		return 0, nil
	case "warning":
		return 1, nil
	case "info":
		return 2, nil
	case "style":
		return 3, nil
	default:
		return 0, fmt.Errorf("unknown ShellCheck severity %q", level)
	}
}
