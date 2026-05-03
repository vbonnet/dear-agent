package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/pkg/wikibrain"
)

var (
	wikiLintJSON     bool
	wikiLintNoAppend bool
)

var wikiLintCmd = &cobra.Command{
	Use:   "lint",
	Short: "Scan engram-kb for broken links, orphans, stale pages, and coverage gaps",
	Long: `lint scans every markdown page in the knowledge base and emits a
severity-tiered report:

  🔴 errors   — broken internal links (must fix)
  🟡 warnings — orphan pages and stale content (should fix)
  🔵 info     — missing metadata and sparse cross-references (nice to have)

By default the result is also appended to log.md in the KB root.

Examples:
  agm wiki lint
  agm wiki lint --kb ~/src/engram-kb
  agm wiki lint --json
  agm wiki lint --no-append`,
	RunE: runWikiLint,
}

func init() {
	wikiCmd.AddCommand(wikiLintCmd)
	wikiLintCmd.Flags().BoolVar(&wikiLintJSON, "json", false, "output report as JSON")
	wikiLintCmd.Flags().BoolVar(&wikiLintNoAppend, "no-append", false, "skip appending to log.md")
}

func runWikiLint(cmd *cobra.Command, _ []string) error {
	kbPath, err := resolveKBPath(wikiKBPath)
	if err != nil {
		return err
	}

	report, err := wikibrain.Lint(kbPath)
	if err != nil {
		return fmt.Errorf("lint failed: %w", err)
	}

	if wikiLintJSON {
		return printLintJSON(report)
	}
	printLintText(report, kbPath)

	if !wikiLintNoAppend {
		if appendErr := appendToLog(kbPath, wikibrain.FormatLintLogEntry(report)); appendErr != nil {
			fmt.Fprintf(os.Stderr, "warning: could not append to log.md: %v\n", appendErr)
		}
	}

	// Exit 1 if there are errors so CI can gate on it
	if report.Stats.ErrorCount > 0 {
		os.Exit(1)
	}
	return nil
}

func printLintText(report *wikibrain.LintReport, kbPath string) {
	fmt.Printf("Wiki Lint — %s\n", kbPath)
	fmt.Printf("Run at: %s\n\n", report.RunAt.Format("2006-01-02 15:04:05 MST"))
	fmt.Printf("Pages: %d   Links: %d\n\n",
		report.Stats.TotalPages, report.Stats.TotalLinks)

	if len(report.Issues) == 0 {
		fmt.Println("✅ No issues found.")
		return
	}

	// Group by severity for a clean display
	bySeverity := map[wikibrain.Severity][]wikibrain.LintIssue{}
	for _, iss := range report.Issues {
		bySeverity[iss.Severity] = append(bySeverity[iss.Severity], iss)
	}

	severities := []wikibrain.Severity{
		wikibrain.SeverityError,
		wikibrain.SeverityWarning,
		wikibrain.SeverityInfo,
	}
	for _, sev := range severities {
		issues := bySeverity[sev]
		if len(issues) == 0 {
			continue
		}
		sort.Slice(issues, func(i, j int) bool {
			return issues[i].File < issues[j].File
		})
		fmt.Printf("%s %s (%d)\n", sev.Emoji(), strings.ToUpper(sev.String()), len(issues))
		for _, iss := range issues {
			fmt.Printf("  %-40s  %s\n", iss.File, iss.Message)
		}
		fmt.Println()
	}

	fmt.Printf("Summary: 🔴 %d  🟡 %d  🔵 %d\n",
		report.Stats.ErrorCount,
		report.Stats.WarningCount,
		report.Stats.InfoCount,
	)
}

func printLintJSON(report *wikibrain.LintReport) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(report)
}

// appendToLog appends a single line to KB_ROOT/log.md, creating the file if
// absent. The file must remain append-only; never truncate it.
func appendToLog(kbPath, line string) error {
	logPath := filepath.Join(kbPath, "log.md")
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = fmt.Fprintln(f, line)
	return err
}
