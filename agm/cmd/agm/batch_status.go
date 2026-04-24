package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/ops"
	"github.com/vbonnet/dear-agent/agm/internal/ui"
)

var (
	batchStatusSessions string
)

var batchStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show status of all active workers",
	Long: `Show the status of all active workers in a single view.

Displays each worker's state, duration, commit count, and branch.

Examples:
  agm batch status                              # All active workers
  agm batch status --sessions "w1,w2"           # Specific workers only`,
	RunE: runBatchStatus,
}

func init() {
	batchStatusCmd.Flags().StringVar(&batchStatusSessions, "sessions", "", "Comma-separated list of session names to check")
	batchCmd.AddCommand(batchStatusCmd)
}

func runBatchStatus(cmd *cobra.Command, args []string) error {
	opCtx, cleanup, err := newOpContextWithStorage()
	if err != nil {
		return fmt.Errorf("failed to connect to storage: %w", err)
	}
	defer cleanup()

	req := &ops.BatchStatusRequest{}
	if batchStatusSessions != "" {
		for _, s := range strings.Split(batchStatusSessions, ",") {
			s = strings.TrimSpace(s)
			if s != "" {
				req.Sessions = append(req.Sessions, s)
			}
		}
	}

	result, err := ops.BatchStatus(opCtx, req)
	if err != nil {
		return handleError(err)
	}

	return printResult(result, func() {
		printBatchStatusTable(result)
	})
}

func printBatchStatusTable(result *ops.BatchStatusResult) {
	if len(result.Workers) == 0 {
		fmt.Println("No active workers found.")
		return
	}

	fmt.Println()
	fmt.Println(strings.Repeat("=", 90))
	fmt.Printf("Active Workers (%d)\n", result.Total)
	fmt.Println(strings.Repeat("=", 90))

	// Header
	fmt.Printf("  %-20s %-18s %-15s %-8s %-10s %s\n",
		"NAME", "STATE", "DURATION", "COMMITS", "COST", "BRANCH")
	fmt.Println("  " + strings.Repeat("-", 88))

	for _, w := range result.Workers {
		stateIcon := stateIndicator(w.State)
		costStr := ""
		if w.Cost > 0 {
			costStr = fmt.Sprintf("$%.2f", w.Cost)
		}
		fmt.Printf("  %-20s %s %-15s %-15s %-8d %-10s %s\n",
			truncate(w.Name, 20),
			stateIcon,
			truncate(w.State, 15),
			w.Duration,
			w.Commits,
			costStr,
			w.Branch,
		)
	}

	fmt.Println()
	ui.PrintSuccess(fmt.Sprintf("%d worker(s) shown", result.Total))
}

func stateIndicator(state string) string {
	switch state {
	case "DONE":
		return "[DONE]"
	case "WORKING":
		return "[WORK]"
	case "USER_PROMPT":
		return "[WAIT]"
	case "PERMISSION_PROMPT":
		return "[PERM]"
	case "OFFLINE":
		return "[OFF] "
	case "COMPACTING":
		return "[COMP]"
	default:
		return "[    ]"
	}
}

// truncateStr is like truncate in status.go but avoids redeclaration.
// Both are in package main so we reuse the existing truncate function instead.
// This file uses truncate() from status.go directly.
