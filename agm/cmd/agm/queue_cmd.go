package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/messages"
)

var queueCmd = &cobra.Command{
	Use:   "queue",
	Short: "Manage the message queue",
	Long:  `Inspect and manage the AGM message delivery queue.`,
	Args:  cobra.ArbitraryArgs,
	RunE:  groupRunE,
}

var queueListCmd = &cobra.Command{
	Use:   "list",
	Short: "Show queue status and messages",
	Long: `Display pending, failed, and delivered message counts with optional
detailed message listing.

Examples:
  agm session queue list              # Show counts + recent messages
  agm session queue list --status queued   # Show only queued messages
  agm session queue list --limit 50   # Show more messages`,
	RunE: runQueueList,
}

func init() {
	sessionCmd.AddCommand(queueCmd)
	queueCmd.AddCommand(queueListCmd)

	queueListCmd.Flags().StringP("status", "s", "", "Filter by status: queued, delivered, failed")
	queueListCmd.Flags().IntP("limit", "n", 20, "Maximum number of messages to show")
}

func runQueueList(cmd *cobra.Command, args []string) error {
	queue, err := messages.NewMessageQueue()
	if err != nil {
		return fmt.Errorf("failed to open message queue: %w", err)
	}
	defer queue.Close()

	// Show summary counts
	stats, err := queue.GetStats()
	if err != nil {
		return fmt.Errorf("failed to get queue stats: %w", err)
	}

	queued := stats[messages.StatusQueued]
	delivered := stats[messages.StatusDelivered]
	failed := stats[messages.StatusFailed]
	total := queued + delivered + failed

	fmt.Printf("=== Message Queue Summary ===\n\n")
	fmt.Printf("  Pending:   %d\n", queued)
	fmt.Printf("  Delivered: %d\n", delivered)
	fmt.Printf("  Failed:    %d\n", failed)
	fmt.Printf("  Total:     %d\n", total)

	// Get filter and limit
	statusFilter, _ := cmd.Flags().GetString("status")
	limit, _ := cmd.Flags().GetInt("limit")

	if statusFilter != "" {
		// Validate status filter
		switch statusFilter {
		case messages.StatusQueued, messages.StatusDelivered, messages.StatusFailed:
			// valid
		default:
			return fmt.Errorf("invalid status filter %q: must be queued, delivered, or failed", statusFilter)
		}
	}

	// List messages
	entries, err := queue.GetQueueList(statusFilter, limit)
	if err != nil {
		return fmt.Errorf("failed to list messages: %w", err)
	}

	if len(entries) == 0 {
		if statusFilter != "" {
			fmt.Printf("\nNo %s messages found.\n", statusFilter)
		} else {
			fmt.Printf("\nNo messages in queue.\n")
		}
		return nil
	}

	header := "Recent Messages"
	if statusFilter != "" {
		header = fmt.Sprintf("Messages (%s)", statusFilter)
	}
	fmt.Printf("\n=== %s ===\n\n", header)

	for _, e := range entries {
		statusIcon := statusIcon(e.Status)
		age := time.Since(e.QueuedAt).Truncate(time.Second)
		msgPreview := truncateMsg(e.Message, 60)

		fmt.Printf("%s %-8s  %s → %s  [%s]  %s ago\n",
			statusIcon, e.MessageID, e.From, e.To, e.Priority, age)
		fmt.Printf("           %s\n", msgPreview)

		if e.AttemptCount > 0 {
			fmt.Printf("           attempts: %d", e.AttemptCount)
			if e.LastAttempt != nil {
				fmt.Printf("  last: %s ago", time.Since(*e.LastAttempt).Truncate(time.Second))
			}
			fmt.Println()
		}
		fmt.Println()
	}

	return nil
}

func statusIcon(status string) string {
	switch status {
	case messages.StatusQueued:
		return "⏳"
	case messages.StatusDelivered:
		return "✓"
	case messages.StatusFailed:
		return "✗"
	default:
		return "?"
	}
}

func truncateMsg(msg string, maxLen int) string {
	// Take first line only
	if idx := strings.IndexByte(msg, '\n'); idx >= 0 {
		msg = msg[:idx]
	}
	if len(msg) <= maxLen {
		return msg
	}
	return msg[:maxLen] + "..."
}
