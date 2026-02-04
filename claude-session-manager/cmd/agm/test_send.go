package main

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"time"

	"github.com/spf13/cobra"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/tmux"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/ui"
)

var (
	sendJSON       bool
	sendPrompt     string
	sendPromptFile string
)

var testSendCmd = &cobra.Command{
	Use:   "send <name> <command>",
	Short: "Send a command to a test session",
	Long: `Send a command to a test session and execute it in the tmux pane.

The command will be sent to the Claude session running in the test tmux session.
This is useful for automation and testing workflows.

SECURITY NOTE: This command executes arbitrary commands in the tmux session.
Only use with trusted input in controlled test environments.

Examples:
  # Send a csm command
  csm test send my-test "csm list"

  # Send a complex command
  csm test send my-test "cd /tmp && ls -la"

  # Get JSON output for automation
  csm test send my-test "csm new test-session" --json`,
	Args: cobra.RangeArgs(1, 2), // name required, command optional if --prompt flags used
	RunE: runTestSend,
}

func init() {
	testSendCmd.Flags().BoolVar(
		&sendJSON,
		"json",
		false,
		"Output as JSON for automation",
	)
	testSendCmd.Flags().StringVar(
		&sendPrompt,
		"prompt",
		"",
		"Prompt to send to session",
	)
	testSendCmd.Flags().StringVar(
		&sendPromptFile,
		"prompt-file",
		"",
		"File containing prompt to send",
	)
	testSendCmd.MarkFlagsMutuallyExclusive("prompt", "prompt-file")

	testCmd.AddCommand(testSendCmd)
}

// SendResult represents the result of sending a command
type SendResult struct {
	Name    string    `json:"name"`
	Command string    `json:"command"`
	SentAt  time.Time `json:"sent_at"`
}

func runTestSend(cmd *cobra.Command, args []string) error {
	name := args[0]
	tmuxName := fmt.Sprintf("csm-test-%s", name)

	// Determine what to send: command argument or --prompt flags
	var command string
	var usePromptAPI bool

	if sendPrompt != "" || sendPromptFile != "" {
		// Using --prompt or --prompt-file flags
		usePromptAPI = true
	} else if len(args) == 2 {
		// Using command argument
		command = args[1]
	} else {
		return fmt.Errorf("must provide either <command> argument or --prompt/--prompt-file flag")
	}

	// Check session exists
	exists, err := tmux.HasSession(tmuxName)
	if err != nil {
		return fmt.Errorf("failed to check tmux session: %w", err)
	}
	if !exists {
		return fmt.Errorf("session '%s' does not exist.\n\nSuggestions:\n  • Create session: csm test create %s\n  • List sessions: tmux ls", name, name)
	}

	// Send command or prompt to tmux session
	if usePromptAPI {
		if sendPrompt != "" {
			if err := tmux.SendPromptLiteral(tmuxName, sendPrompt); err != nil {
				return fmt.Errorf("failed to send prompt: %w", err)
			}
			command = sendPrompt // For output display
		} else if sendPromptFile != "" {
			if err := tmux.SendPromptFromFile(tmuxName, sendPromptFile); err != nil {
				return fmt.Errorf("failed to send prompt from file: %w", err)
			}
			command = fmt.Sprintf("(from file: %s)", sendPromptFile) // For output display
		}
	} else {
		// Legacy: send command with bundled C-m
		sendCmd := exec.Command("tmux", "send-keys", "-t", tmuxName, command, "C-m")
		if err := sendCmd.Run(); err != nil {
			return fmt.Errorf("failed to send command: %w\n\nSuggestions:\n  • Check session is alive: tmux has-session -t %s", err, tmuxName)
		}
	}

	// Format output
	result := &SendResult{
		Name:    name,
		Command: command,
		SentAt:  time.Now(),
	}

	if sendJSON {
		jsonBytes, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal JSON: %w", err)
		}
		fmt.Println(string(jsonBytes))
	} else {
		ui.PrintSuccess(fmt.Sprintf("Command sent to '%s': %s", name, command))
	}

	return nil
}
