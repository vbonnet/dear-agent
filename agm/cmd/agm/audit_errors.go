package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/ops"
)

// parseDuration is defined in archive.go — reused here via same package.

var (
	auditErrorsSince    string
	auditErrorsCategory string
	auditErrorsLimit    int
)

var auditTopCmd = &cobra.Command{
	Use:   "audit",
	Short: "Audit and diagnostics commands",
	Long:  `Commands for auditing error logs, session health, and system diagnostics.`,
	Args:  cobra.ArbitraryArgs,
	RunE:  groupRunE,
}

var auditErrorsCmd = &cobra.Command{
	Use:   "errors",
	Short: "Read error log, group by category, and report top issues",
	Long: `Read the persistent error log (~/.agm/hook-errors.jsonl), group entries
by category, and display a summary of the most common issues.

Categories:
  hook-error          Pre/post hook execution failures
  permission-blocked  Permission-denied tool calls
  test-failure        Test suite failures
  build-failure       Compilation or build errors
  merge-conflict      Git merge conflicts

Examples:
  agm audit errors                          # Show all errors
  agm audit errors --since 24h             # Errors from the last 24 hours
  agm audit errors --category test-failure  # Only test failures
  agm audit errors --limit 10              # Show at most 10 entries
  agm audit errors --since 7d --category hook-error`,
	RunE: runAuditErrors,
}

func init() {
	rootCmd.AddCommand(auditTopCmd)
	auditTopCmd.AddCommand(auditErrorsCmd)

	auditErrorsCmd.Flags().StringVar(&auditErrorsSince, "since", "",
		"Show errors since duration ago (e.g., 1h, 24h, 7d)")
	auditErrorsCmd.Flags().StringVar(&auditErrorsCategory, "category", "",
		"Filter by error category")
	auditErrorsCmd.Flags().IntVar(&auditErrorsLimit, "limit", 0,
		"Maximum number of entries to display")
}

func runAuditErrors(_ *cobra.Command, _ []string) error {
	// Validate category flag
	if auditErrorsCategory != "" && !ops.IsValidCategory(auditErrorsCategory) {
		valid := make([]string, 0, len(ops.ValidCategories()))
		for _, c := range ops.ValidCategories() {
			valid = append(valid, string(c))
		}
		return fmt.Errorf("invalid category %q; valid categories: %s",
			auditErrorsCategory, strings.Join(valid, ", "))
	}

	// Parse --since
	var since time.Time
	if auditErrorsSince != "" {
		d, err := parseDuration(auditErrorsSince)
		if err != nil {
			return fmt.Errorf("invalid --since value: %w", err)
		}
		since = time.Now().Add(-d)
	}

	// Read errors
	el := ops.NewErrorLog(ops.DefaultErrorLogPath())

	var entries []ops.ErrorEntry
	var err error
	if auditErrorsCategory != "" {
		entries, err = el.ReadErrorsByCategory(auditErrorsCategory, since)
	} else {
		entries, err = el.ReadErrors(since)
	}
	if err != nil {
		return fmt.Errorf("read error log: %w", err)
	}

	// Apply limit
	if auditErrorsLimit > 0 && len(entries) > auditErrorsLimit {
		entries = entries[len(entries)-auditErrorsLimit:]
	}

	if len(entries) == 0 {
		fmt.Println("No errors found.")
		return nil
	}

	// JSON output mode
	if outputFormat == "json" {
		report := buildErrorReport(entries)
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(report)
	}

	printErrorAuditText(entries, since)
	return nil
}

// printErrorAuditText prints the human-readable error audit grouped by
// category, showing up to 5 most recent entries per category.
func printErrorAuditText(entries []ops.ErrorEntry, since time.Time) {
	grouped := groupByCategory(entries)

	fmt.Printf("Error Log Summary (%d entries)\n", len(entries))
	if !since.IsZero() {
		fmt.Printf("Since: %s\n", since.Format(time.RFC3339))
	}
	fmt.Println(strings.Repeat("─", 60))

	type catCount struct {
		category string
		entries  []ops.ErrorEntry
	}
	var sorted []catCount
	for cat, ents := range grouped {
		sorted = append(sorted, catCount{cat, ents})
	}
	sort.Slice(sorted, func(i, j int) bool {
		return len(sorted[i].entries) > len(sorted[j].entries)
	})

	for _, cc := range sorted {
		fmt.Printf("\n[%s] — %d error(s)\n", cc.category, len(cc.entries))
		start := 0
		if len(cc.entries) > 5 {
			start = len(cc.entries) - 5
			fmt.Printf("  ... (%d earlier entries omitted)\n", start)
		}
		for _, e := range cc.entries[start:] {
			ts := e.Timestamp.Format("2006-01-02 15:04:05")
			sess := e.SessionName
			if sess == "" {
				sess = "-"
			}
			fmt.Printf("  %s  [%s]  %s  (source: %s)\n", ts, sess, e.Message, e.Source)
		}
	}

	fmt.Println()
}

// errorReport is the JSON representation of the audit errors output.
type errorReport struct {
	TotalErrors int                      `json:"total_errors"`
	Since       string                   `json:"since,omitempty"`
	ByCategory  map[string]int           `json:"by_category"`
	Entries     []ops.ErrorEntry         `json:"entries"`
	TopSources  []sourceCount            `json:"top_sources"`
}

type sourceCount struct {
	Source string `json:"source"`
	Count  int    `json:"count"`
}

func buildErrorReport(entries []ops.ErrorEntry) errorReport {
	r := errorReport{
		TotalErrors: len(entries),
		ByCategory:  make(map[string]int),
		Entries:     entries,
	}

	if !entries[0].Timestamp.IsZero() {
		r.Since = entries[0].Timestamp.Format(time.RFC3339)
	}

	sourceCounts := make(map[string]int)
	for _, e := range entries {
		r.ByCategory[string(e.Category)]++
		sourceCounts[e.Source]++
	}

	for src, cnt := range sourceCounts {
		r.TopSources = append(r.TopSources, sourceCount{Source: src, Count: cnt})
	}
	sort.Slice(r.TopSources, func(i, j int) bool {
		return r.TopSources[i].Count > r.TopSources[j].Count
	})

	return r
}

func groupByCategory(entries []ops.ErrorEntry) map[string][]ops.ErrorEntry {
	grouped := make(map[string][]ops.ErrorEntry)
	for _, e := range entries {
		grouped[string(e.Category)] = append(grouped[string(e.Category)], e)
	}
	return grouped
}
