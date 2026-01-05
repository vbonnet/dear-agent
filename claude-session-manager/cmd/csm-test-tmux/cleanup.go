package main

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/testutil/session"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/testutil/tmux"
)

var (
	cleanupSessionsDir string
)

var cleanupCmd = &cobra.Command{
	Use:   "cleanup <name>",
	Short: "Cleanup a test session (kill tmux, remove state)",
	Long: `Cleanup a test session by removing tmux session and state directory.

Cleanup is best-effort and continues even if some steps fail:
1. Kill tmux session
2. Archive CSM session (future)
3. Remove sessions directory

Examples:
  # Cleanup session with default directory
  csm-test-tmux cleanup my-test

  # Cleanup with custom sessions directory
  csm-test-tmux cleanup my-test --sessions-dir /tmp/my-tests

  # Get JSON status of cleanup
  csm-test-tmux cleanup my-test --format json`,
	Args: cobra.ExactArgs(1),
	RunE: runCleanup,
}

func init() {
	cleanupCmd.Flags().StringVar(
		&cleanupSessionsDir,
		"sessions-dir",
		"",
		"Directory for CSM session state (default: /tmp/csm-test-<name>)",
	)

	rootCmd.AddCommand(cleanupCmd)
}

func runCleanup(cmd *cobra.Command, args []string) error {
	name := args[0]

	// Set defaults
	if cleanupSessionsDir == "" {
		cleanupSessionsDir = fmt.Sprintf("/tmp/csm-test-%s", name)
	}

	// Create session manager
	tmuxClient := tmux.New()
	mgr := session.New(tmuxClient)

	// Cleanup session
	opts := session.CleanupOptions{
		SessionsDir: cleanupSessionsDir,
	}

	status, err := mgr.Cleanup(name, opts)
	if err != nil {
		// Even if cleanup returns error, show what was cleaned
		if status != nil {
			_ = printOutput(status)
		}
		return formatError(err)
	}

	return printOutput(status)
}
