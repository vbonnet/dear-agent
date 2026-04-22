package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/messages"
	"github.com/vbonnet/dear-agent/agm/internal/ui"
)

var (
	logsCleanOlderThan int
	logsQuerySender    string
	logsQuerySince     string
)

var logsCmd = &cobra.Command{
	Use:   "logs",
	Short: "Manage AGM message logs",
	Long: `Manage AGM message logs (query, stats, thread tracking, cleanup).

Message logs are stored in ~/.agm/logs/messages/ as daily JSONL files.
Each log file contains all messages sent on that day.

Log format: YYYY-MM-DD.jsonl
Example: 2026-02-03.jsonl

Subcommands:
  clean  - Remove old log files
  stats  - Show log statistics
  thread - Show conversation thread
  query  - Search message logs`,
	Args: cobra.ArbitraryArgs,
	RunE: groupRunE,
}

var logsCleanCmd = &cobra.Command{
	Use:   "clean",
	Short: "Remove old message log files",
	Long: `Remove message log files older than the retention period.

Default retention: 90 days
Configurable via ~/.config.agm/config.yaml

Examples:
  # Clean logs older than 90 days (default)
  agm session logs clean

  # Clean logs older than 30 days
  agm session logs clean --older-than 30

  # Dry run (show what would be deleted)
  agm session logs clean --dry-run --older-than 30`,
	RunE: runLogsClean,
}

var logsStatsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Show message log statistics",
	Long: `Display statistics about message logs.

Shows:
  - Total log files
  - Total messages logged
  - Date range (oldest to newest)
  - Disk usage

Examples:
  # Show log statistics
  agm session logs stats`,
	RunE: runLogsStats,
}

var logsThreadCmd = &cobra.Command{
	Use:   "thread <message-id>",
	Short: "Show conversation thread for a message",
	Long: `Display the conversation thread containing a specific message.

Follows reply-to links to reconstruct the conversation flow.

Examples:
  # Show thread for a message
  agm session logs thread 1738612345678-sender-001`,
	Args: cobra.ExactArgs(1),
	RunE: runLogsThread,
}

var logsQueryCmd = &cobra.Command{
	Use:   "query",
	Short: "Search message logs",
	Long: `Search message logs by sender, recipient, date, or content.

Examples:
  # Find all messages from a sender
  agm session logs query --sender astrocyte

  # Find messages since a date
  agm session logs query --since 2026-02-01

  # Combine filters
  agm session logs query --sender agm-send --since 2026-02-03`,
	RunE: runLogsQuery,
}

func init() {
	logsCleanCmd.Flags().IntVar(
		&logsCleanOlderThan,
		"older-than",
		90,
		"Delete logs older than this many days",
	)

	logsQueryCmd.Flags().StringVar(
		&logsQuerySender,
		"sender",
		"",
		"Filter by sender name",
	)
	logsQueryCmd.Flags().StringVar(
		&logsQuerySince,
		"since",
		"",
		"Filter by date (YYYY-MM-DD)",
	)

	logsCmd.AddCommand(logsCleanCmd)
	logsCmd.AddCommand(logsStatsCmd)
	logsCmd.AddCommand(logsThreadCmd)
	logsCmd.AddCommand(logsQueryCmd)
	sessionCmd.AddCommand(logsCmd)
}

func runLogsClean(cmd *cobra.Command, args []string) error {
	// Get logs directory
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}
	logsDir := filepath.Join(homeDir, ".agm", "logs", "messages")

	// Create logger
	logger, err := messages.NewMessageLogger(logsDir)
	if err != nil {
		return fmt.Errorf("failed to create logger: %w", err)
	}

	// Clean up old logs
	deletedCount, err := logger.CleanupOldLogs(logsCleanOlderThan)
	if err != nil {
		return fmt.Errorf("failed to clean up logs: %w", err)
	}

	if deletedCount == 0 {
		ui.PrintSuccess(fmt.Sprintf("No log files older than %d days found", logsCleanOlderThan))
	} else {
		ui.PrintSuccess(fmt.Sprintf("Deleted %d log file(s) older than %d days", deletedCount, logsCleanOlderThan))
	}

	return nil
}

func runLogsStats(cmd *cobra.Command, args []string) error {
	// Get logs directory
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}
	logsDir := filepath.Join(homeDir, ".agm", "logs", "messages")

	// Create logger
	logger, err := messages.NewMessageLogger(logsDir)
	if err != nil {
		return fmt.Errorf("failed to create logger: %w", err)
	}

	// Get statistics
	stats, err := logger.GetStats()
	if err != nil {
		return fmt.Errorf("failed to get log stats: %w", err)
	}

	// Display statistics
	fmt.Println("Message Log Statistics")
	fmt.Println("======================")
	fmt.Printf("Total log files:    %d\n", stats.TotalFiles)
	fmt.Printf("Total messages:     %d\n", stats.TotalMessages)

	if stats.OldestDate != "" {
		fmt.Printf("Oldest log:         %s\n", stats.OldestDate)
		fmt.Printf("Newest log:         %s\n", stats.NewestDate)
	}

	fmt.Printf("\nLog directory:      %s\n", logsDir)

	// Calculate disk usage
	var totalSize int64
	entries, _ := os.ReadDir(logsDir)
	for _, entry := range entries {
		if entry.Type().IsRegular() && filepath.Ext(entry.Name()) == ".jsonl" {
			info, err := entry.Info()
			if err == nil {
				totalSize += info.Size()
			}
		}
	}

	// Format size
	sizeStr := formatBytes(totalSize)
	fmt.Printf("Disk usage:         %s\n", sizeStr)

	return nil
}

func runLogsThread(cmd *cobra.Command, args []string) error {
	messageID := args[0]

	// Validate message ID format
	if !messages.ValidateMessageID(messageID) {
		return fmt.Errorf("invalid message ID format: '%s'\n\nExpected format: {timestamp}-{sender}-{seq}\nExample: 1738612345678-sender-001", messageID)
	}

	// Get logs directory
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}
	logsDir := filepath.Join(homeDir, ".agm", "logs", "messages")

	fmt.Printf("Thread view for message: %s\n", messageID)
	fmt.Println("(Note: Full thread tracking implementation pending)")
	fmt.Printf("\nLog directory: %s\n", logsDir)
	fmt.Println("\nTo manually search logs:")
	fmt.Printf("  grep '%s' %s/*.jsonl\n", messageID, logsDir)

	return nil
}

func runLogsQuery(cmd *cobra.Command, args []string) error {
	// Get logs directory
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}
	logsDir := filepath.Join(homeDir, ".agm", "logs", "messages")

	fmt.Println("Message Log Query")
	fmt.Println("=================")

	if logsQuerySender != "" {
		fmt.Printf("Filter by sender: %s\n", logsQuerySender)
	}
	if logsQuerySince != "" {
		fmt.Printf("Filter by date:   %s\n", logsQuerySince)
	}

	fmt.Println("\n(Note: Full query implementation pending)")
	fmt.Printf("\nLog directory: %s\n", logsDir)
	fmt.Println("\nTo manually query logs:")
	if logsQuerySender != "" {
		fmt.Printf("  grep '\"sender\":\"%s\"' %s/*.jsonl\n", logsQuerySender, logsDir)
	} else {
		fmt.Printf("  cat %s/*.jsonl | jq .\n", logsDir)
	}

	return nil
}

// formatBytes formats bytes as human-readable string
func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}

	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}

	return fmt.Sprintf("%.1f %ciB", float64(bytes)/float64(div), "KMGTPE"[exp])
}
