package main

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/testutil/session"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/testutil/tmux"
)

var (
	sendSessionsDir  string
	sendAutocomplete bool
	sendDelay        int
)

var sendCmd = &cobra.Command{
	Use:   "send <name> <command>",
	Short: "Send a command to the test session",
	Long: `Send a command to an existing test session.

The command is executed in the Claude session via tmux send-keys.

Examples:
  # Send a simple command
  csm-test-tmux send my-test "git status"

  # Send a slash command with autocomplete
  csm-test-tmux send my-test "/commit" --autocomplete

  # Send with custom delay for autocomplete
  csm-test-tmux send my-test "/help" --autocomplete --delay 200`,
	Args: cobra.ExactArgs(2),
	RunE: runSend,
}

func init() {
	sendCmd.Flags().StringVar(
		&sendSessionsDir,
		"sessions-dir",
		"",
		"Directory for CSM session state (default: /tmp/csm-test-<name>)",
	)
	sendCmd.Flags().BoolVar(
		&sendAutocomplete,
		"autocomplete",
		false,
		"Send an additional Enter after delay (for autocomplete commands)",
	)
	sendCmd.Flags().IntVar(
		&sendDelay,
		"delay",
		100,
		"Delay in milliseconds before sending autocomplete Enter",
	)

	rootCmd.AddCommand(sendCmd)
}

func runSend(cmd *cobra.Command, args []string) error {
	name := args[0]
	command := args[1]

	// Set defaults
	if sendSessionsDir == "" {
		sendSessionsDir = fmt.Sprintf("/tmp/csm-test-%s", name)
	}

	// Create session manager
	tmuxClient := tmux.New()
	mgr := session.New(tmuxClient)

	// Send command
	opts := session.SendOptions{
		Command:      command,
		SessionsDir:  sendSessionsDir,
		Autocomplete: sendAutocomplete,
		Delay:        time.Duration(sendDelay) * time.Millisecond,
	}

	err := mgr.Send(name, opts)
	if err != nil {
		return formatError(err)
	}

	// Success output
	result := map[string]interface{}{
		"status":       "sent",
		"session":      name,
		"command":      command,
		"autocomplete": sendAutocomplete,
	}

	return printOutput(result)
}
