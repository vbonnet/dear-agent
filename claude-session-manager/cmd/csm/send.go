package main

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/tmux"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/ui"
)

var (
	sessionSendPrompt     string
	sessionSendPromptFile string
)

var sendCmd = &cobra.Command{
	Use:   "send <session-name>",
	Short: "Send a message to a running session",
	Long: `Send a message/prompt to a running CSM session, interrupting any active thinking state.

Features:
  • Auto-interrupt: Sends ESC to interrupt thinking before sending prompt
  • Literal mode: Uses tmux -l flag to prevent special character interpretation
  • Reliable execution: Prompt is executed as command, not queued as pasted text
  • Large prompts: Supports up to 10KB prompt files

This solves the common issue where multi-line prompts sent via paste-buffer
are interpreted as "pasted text" instead of being executed.

Use Cases:
  • Automated recovery of stuck sessions (used by astrocyte daemon)
  • Sending diagnosis prompts to investigate hangs
  • Batch message delivery to multiple sessions

Examples:
  # Send inline prompt
  csm send my-session --prompt "Please review the code"

  # Send from file (for large multi-line prompts)
  csm send my-session --prompt-file /path/to/prompt.txt

  # Send diagnosis request to stuck session
  csm send gemini-research --prompt "⚠️ Your session was stuck. Please analyze what caused the hang."

  # Send multi-line prompt
  csm send task --prompt "Review the following:
  1. Authentication logic
  2. Error handling
  3. Security concerns"

Requirements:
  • Session must be running (active tmux session)
  • Requires either --prompt or --prompt-file flag

See Also:
  • csm reject - Reject permission prompts with custom reasons
  • csm doctor - Check session health`,
	Args: cobra.ExactArgs(1),
	RunE: runSend,
}

func init() {
	sendCmd.Flags().StringVar(
		&sessionSendPrompt,
		"prompt",
		"",
		"Prompt text to send to session",
	)
	sendCmd.Flags().StringVar(
		&sessionSendPromptFile,
		"prompt-file",
		"",
		"File containing prompt to send (max 10KB)",
	)
	sendCmd.MarkFlagsMutuallyExclusive("prompt", "prompt-file")
	sendCmd.MarkFlagsOneRequired("prompt", "prompt-file")

	rootCmd.AddCommand(sendCmd)
}

func runSend(cmd *cobra.Command, args []string) error {
	sessionName := args[0]

	// Check session exists in tmux
	exists, err := tmux.HasSession(sessionName)
	if err != nil {
		return fmt.Errorf("failed to check tmux session: %w", err)
	}
	if !exists {
		return fmt.Errorf("session '%s' does not exist in tmux.\n\nSuggestions:\n  • List sessions: csm list\n  • Create session: csm new %s", sessionName, sessionName)
	}

	// Send prompt using literal mode
	if sessionSendPrompt != "" {
		if err := tmux.SendPromptLiteral(sessionName, sessionSendPrompt); err != nil {
			return fmt.Errorf("failed to send prompt: %w", err)
		}
		ui.PrintSuccess(fmt.Sprintf("Prompt sent to '%s' (%d chars)", sessionName, len(sessionSendPrompt)))
	} else if sessionSendPromptFile != "" {
		if err := tmux.SendPromptFromFile(sessionName, sessionSendPromptFile); err != nil {
			return fmt.Errorf("failed to send prompt from file: %w", err)
		}
		ui.PrintSuccess(fmt.Sprintf("Prompt sent to '%s' from file: %s", sessionName, sessionSendPromptFile))
	}

	return nil
}
