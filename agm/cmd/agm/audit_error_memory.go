package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/engram/errormemory"
)

var (
	auditEMSince    string
	auditEMCategory string
	auditEMLimit    int
	auditEMSource   string
)

var auditErrorMemoryCmd = &cobra.Command{
	Use:   "error-memory",
	Short: "Query the persistent error memory store",
	Long: `Query the error memory store (~/.agm/error-memory.jsonl) which tracks
deduplicated error patterns across sessions. Unlike 'audit errors' which shows
raw hook error events, error-memory shows consolidated patterns with counts.

Examples:
  agm audit error-memory                              # Show all patterns
  agm audit error-memory --since 7d                   # Patterns active in last 7 days
  agm audit error-memory --category stall             # Filter by category
  agm audit error-memory --source agm-cross-check     # Filter by source
  agm audit error-memory --limit 10                   # Top 10 patterns`,
	RunE: runAuditErrorMemory,
}

func init() {
	auditTopCmd.AddCommand(auditErrorMemoryCmd)

	auditErrorMemoryCmd.Flags().StringVar(&auditEMSince, "since", "",
		"Show patterns active since duration ago (e.g., 1h, 24h, 7d)")
	auditErrorMemoryCmd.Flags().StringVar(&auditEMCategory, "category", "",
		"Filter by error category (e.g., stall, false-completion, quality-gate)")
	auditErrorMemoryCmd.Flags().IntVar(&auditEMLimit, "limit", 20,
		"Maximum number of patterns to display")
	auditErrorMemoryCmd.Flags().StringVar(&auditEMSource, "source", "",
		"Filter by source subsystem (e.g., agm-stall-detector, agm-cross-check)")
}

func runAuditErrorMemory(_ *cobra.Command, _ []string) error {
	store := errormemory.NewStore(errormemory.DefaultDBPath)
	records, err := store.Load()
	if err != nil {
		return fmt.Errorf("load error memory: %w", err)
	}

	// Prune expired records from display
	records = errormemory.PruneExpired(records)

	// Filter by --since
	if auditEMSince != "" {
		d, err := parseDuration(auditEMSince)
		if err != nil {
			return fmt.Errorf("invalid --since value: %w", err)
		}
		since := time.Now().Add(-d)
		var filtered []errormemory.ErrorRecord
		for _, r := range records {
			if r.LastSeen.After(since) {
				filtered = append(filtered, r)
			}
		}
		records = filtered
	}

	// Filter by --category
	if auditEMCategory != "" {
		var filtered []errormemory.ErrorRecord
		for _, r := range records {
			if r.ErrorCategory == auditEMCategory {
				filtered = append(filtered, r)
			}
		}
		records = filtered
	}

	// Filter by --source
	if auditEMSource != "" {
		var filtered []errormemory.ErrorRecord
		for _, r := range records {
			if r.Source == auditEMSource {
				filtered = append(filtered, r)
			}
		}
		records = filtered
	}

	if len(records) == 0 {
		fmt.Println("No error memory patterns found.")
		return nil
	}

	// Sort by count descending (most frequent first)
	sort.Slice(records, func(i, j int) bool {
		return records[i].Count > records[j].Count
	})

	// Apply limit
	if auditEMLimit > 0 && len(records) > auditEMLimit {
		records = records[:auditEMLimit]
	}

	// JSON output
	if outputFormat == "json" {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(records)
	}

	// Text output
	fmt.Printf("Error Memory Patterns (%d entries)\n", len(records))
	fmt.Println(strings.Repeat("─", 72))

	for _, r := range records {
		ago := time.Since(r.LastSeen).Round(time.Minute)
		sessStr := "-"
		if len(r.SessionIDs) > 0 {
			sessStr = strings.Join(r.SessionIDs, ", ")
		}
		fmt.Printf("\n  [%s] %s  (%dx, last %v ago)\n", r.ErrorCategory, r.Pattern, r.Count, ago)
		if r.Remediation != "" {
			fmt.Printf("    Fix: %s\n", r.Remediation)
		}
		fmt.Printf("    Source: %s  Sessions: %s\n", r.Source, sessStr)
	}

	fmt.Println()
	return nil
}
