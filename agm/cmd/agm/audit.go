package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/ops"
)

var auditTrailCmd = &cobra.Command{
	Use:   "audit-trail",
	Short: "View and search the audit trail",
	Long: `Audit trail commands provide access to the AGM command execution log.

The audit trail records every AGM command execution with timestamps,
session context, result status, and duration.

Examples:
  agm audit-trail log                              # Show recent audit events
  agm audit-trail log -n 50                        # Show last 50 events
  agm audit-trail search --type session.new        # Search by command type
  agm audit-trail search --session worker-1        # Search by session name`,
	Args: cobra.ArbitraryArgs,
	RunE: groupRunE,
}

var auditLogCmd = &cobra.Command{
	Use:   "log",
	Short: "View recent audit events",
	Long:  `Display recent audit events from the AGM audit trail (newest last).`,
	RunE:  runAuditLog,
}

var auditSearchCmd = &cobra.Command{
	Use:   "search",
	Short: "Search audit events by type or session",
	Long:  `Search the AGM audit trail with filters for command type and session name.`,
	RunE:  runAuditSearch,
}

var (
	auditLogLimit     int
	auditSearchType   string
	auditSearchSess   string
	auditSearchLimit  int
	auditOutputFormat string
)

func init() {
	rootCmd.AddCommand(auditTrailCmd)
	auditTrailCmd.AddCommand(auditLogCmd)
	auditTrailCmd.AddCommand(auditSearchCmd)

	auditLogCmd.Flags().IntVarP(&auditLogLimit, "lines", "n", 20, "number of recent events to show")
	auditLogCmd.Flags().StringVarP(&auditOutputFormat, "output", "o", "", "output format: text (default), json")

	auditSearchCmd.Flags().StringVar(&auditSearchType, "type", "", "filter by command type (substring match)")
	auditSearchCmd.Flags().StringVar(&auditSearchSess, "session", "", "filter by session name (substring match)")
	auditSearchCmd.Flags().IntVarP(&auditSearchLimit, "limit", "n", 50, "max results to return")
}

func auditLogPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".agm", "logs", "audit.jsonl")
}

func runAuditLog(_ *cobra.Command, _ []string) error {
	path := auditLogPath()
	events, err := ops.ReadRecentEvents(path, auditLogLimit)
	if err != nil {
		return fmt.Errorf("failed to read audit log: %w", err)
	}

	if len(events) == 0 {
		fmt.Println("No audit events found.")
		return nil
	}

	format := auditOutputFormat
	if format == "" {
		format = outputFormat // fall back to global --output flag
	}

	if format == "json" {
		return printAuditJSON(events)
	}
	return printAuditTable(events)
}

func runAuditSearch(_ *cobra.Command, _ []string) error {
	path := auditLogPath()
	events, err := ops.SearchEvents(path, ops.AuditSearchParams{
		Command: auditSearchType,
		Session: auditSearchSess,
		Limit:   auditSearchLimit,
	})
	if err != nil {
		return fmt.Errorf("failed to search audit log: %w", err)
	}

	if len(events) == 0 {
		fmt.Println("No matching audit events found.")
		return nil
	}

	format := auditOutputFormat
	if format == "" {
		format = outputFormat
	}

	if format == "json" {
		return printAuditJSON(events)
	}
	return printAuditTable(events)
}

func printAuditJSON(events []ops.AuditEvent) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(events)
}

func printAuditTable(events []ops.AuditEvent) error {
	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "TIMESTAMP\tCOMMAND\tSESSION\tRESULT\tDURATION")
	for _, ev := range events {
		ts := ev.Timestamp.Format(time.RFC3339)
		sess := ev.Session
		if sess == "" {
			sess = "-"
		}
		dur := fmt.Sprintf("%dms", ev.DurationMs)
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", ts, ev.Command, sess, ev.Result, dur)
	}
	return w.Flush()
}
