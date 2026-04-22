package main

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/ops"
	"github.com/vbonnet/dear-agent/agm/internal/ui"
)

var sessionRetryCmd = &cobra.Command{
	Use:   "retry <session-name>",
	Short: "Manually retry a stalled session with error context",
	Long: `Manually retry a session that has stalled, including error context from previous failure.

Retries use bounded retry logic with exponential backoff (1m, 3m, 10m).
Max retries: 3 attempts before escalation to orchestrator.

The command loads the last error from the previous failure attempt and includes
it in the recovery message to help diagnose the issue.

Examples:
  agm session retry my-worker           # Retry session by name
  agm session retry worker-abc123       # Retry session by ID`,
	Args: cobra.ExactArgs(1),
	RunE: runSessionRetry,
}

func init() {
	sessionCmd.AddCommand(sessionRetryCmd)
}

func runSessionRetry(cmd *cobra.Command, args []string) error {
	sessionName := args[0]

	// Construct OpContext with storage
	opCtx, cleanup, err := newOpContextWithStorage()
	if err != nil {
		return fmt.Errorf("failed to connect to Dolt storage: %w", err)
	}
	defer cleanup()

	// Call ops layer
	result, opErr := ops.RetrySession(opCtx, &ops.RetrySessionRequest{
		SessionName: sessionName,
	})
	if opErr != nil {
		return handleError(opErr)
	}

	// Output based on format
	return printResult(result, func() {
		fmt.Printf("Session Retry: %s\n", ui.Bold(sessionName))
		fmt.Printf("  Attempt:      %d\n", result.RetryState.AttemptCount)
		if result.RetryState.LastError != "" {
			fmt.Printf("  Last Error:   %s\n", result.RetryState.LastError)
		}
		fmt.Printf("  Last Attempt: %s\n", result.RetryState.LastAttempt)
		if !result.RetryState.NextRetryAt.IsZero() {
			fmt.Printf("  Next Retry:   %s\n", result.RetryState.NextRetryAt)
		}
		fmt.Printf("  Status:       %s\n", result.Status)
		if result.Description != "" {
			fmt.Printf("  Description:  %s\n", result.Description)
		}
	})
}
