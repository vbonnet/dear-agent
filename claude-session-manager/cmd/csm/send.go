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
	Short: "Send a message to a CSM session",
	Long: `Send a message/prompt to a running CSM session.

This uses tmux literal mode (-l flag) to send text reliably without
special character interpretation, followed by a separate Enter key.

This solves the common issue where multi-line prompts sent via paste-buffer
are interpreted as "pasted text" instead of being executed.

Examples:
  # Send a prompt from command line
  csm send my-session --prompt "Please review the code"

  # Send a prompt from file (for large multi-line prompts)
  csm send my-session --prompt-file /path/to/prompt.txt

IMPORTANT: Requires either --prompt or --prompt-file flag.`,
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
