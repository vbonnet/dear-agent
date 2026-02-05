package main

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/tmux"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/ui"
)

var (
	selectOptionPrompt string
)

var selectOptionCmd = &cobra.Command{
	Use:   "select-option <session-name> <option-number>",
	Short: "Programmatically select an option in AskUserQuestion prompts",
	Long: `Programmatically answer AskUserQuestion prompts in sessions by selecting an option.

This command navigates to the specified option using arrow keys and submits the selection.
Optionally, it can provide custom text input after selecting the option.

Key Features:
  • Navigates to option N using arrow keys (Down)
  • Submits selection with Enter key
  • Optional: Add custom text with --prompt (uses Tab to access input field)
  • Works with Claude Code's AskUserQuestion UI

Examples:
  # Select option 2 (simple selection)
  agm select-option my-session 2

  # Select option 1 and provide custom text
  agm select-option my-session 1 --prompt "Custom configuration details"

  # Select "Yes, and don't ask again" option (typically option 2)
  agm select-option my-session 2

Use Cases:
  • Orchestrator answering session questions automatically
  • Approving skill permissions programmatically
  • Batch processing sessions with standardized answers
  • Testing question flows without manual intervention

Requirements:
  • Session must be showing an AskUserQuestion prompt
  • Option number must be valid (1-4 typically)
  • Session must be active and responsive

See Also:
  • agm send - Send custom prompts to sessions
  • agm reject - Reject permission prompts`,
	Args: cobra.ExactArgs(2),
	RunE: runSelectOption,
}

func init() {
	selectOptionCmd.Flags().StringVar(
		&selectOptionPrompt,
		"prompt",
		"",
		"Optional custom text to provide after selecting option (sends Tab, types text, Enter)",
	)

	rootCmd.AddCommand(selectOptionCmd)
}

func runSelectOption(cmd *cobra.Command, args []string) error {
	sessionName := args[0]
	optionNumber := args[1]

	// Validate option number (1-4 is typical for AskUserQuestion)
	if optionNumber < "1" || optionNumber > "9" {
		return fmt.Errorf("invalid option number: %s (must be 1-9)", optionNumber)
	}

	// Check session exists
	exists, err := tmux.HasSession(sessionName)
	if err != nil {
		return fmt.Errorf("failed to check session: %w", err)
	}
	if !exists {
		return fmt.Errorf("session '%s' does not exist", sessionName)
	}

	// Strategy: Navigate using arrow keys, then submit
	// Option 1 is the default (already selected), so:
	//   - Option 1: 0 presses
	//   - Option 2: 1 press
	//   - Option 3: 2 presses
	//   - etc.

	numPresses := int(optionNumber[0] - '1') // Convert '1' to 0, '2' to 1, etc.

	// Send Down arrow key presses to navigate to the option
	for i := 0; i < numPresses; i++ {
		if err := tmux.SendKeys(sessionName, "Down"); err != nil {
			return fmt.Errorf("failed to send Down key (press %d/%d): %w", i+1, numPresses, err)
		}
		time.Sleep(100 * time.Millisecond) // Small delay between key presses
	}

	// If --prompt provided, use Tab to access custom input field
	if selectOptionPrompt != "" {
		// Send Tab key to switch to custom input field
		if err := tmux.SendKeys(sessionName, "Tab"); err != nil {
			return fmt.Errorf("failed to send Tab key: %w", err)
		}
		time.Sleep(100 * time.Millisecond)

		// Send the custom text using literal mode (prevents special char interpretation)
		if err := tmux.SendPromptLiteral(sessionName, selectOptionPrompt); err != nil {
			return fmt.Errorf("failed to send custom prompt: %w", err)
		}

		// Note: SendPromptLiteral already sends Enter at the end
	} else {
		// No custom prompt - just submit the selection with Enter
		if err := tmux.SendKeys(sessionName, "Enter"); err != nil {
			return fmt.Errorf("failed to send Enter key: %w", err)
		}
	}

	// Print success message
	successMsg := fmt.Sprintf("Selected option %s in session '%s'", optionNumber, sessionName)
	if selectOptionPrompt != "" {
		successMsg += fmt.Sprintf(" with custom text (%d chars)", len(selectOptionPrompt))
	}
	ui.PrintSuccess(successMsg)

	return nil
}
