package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/ops"
	"github.com/vbonnet/dear-agent/agm/internal/safety"
	"github.com/vbonnet/dear-agent/agm/internal/tmux"
	"github.com/vbonnet/dear-agent/agm/internal/ui"
)

var (
	selectOptionPrompt string
	selectOptionForce  bool
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
  agm session select-option my-session 2

  # Select option 1 and provide custom text
  agm session select-option my-session 1 --prompt "Custom configuration details"

  # Select "Yes, and don't ask again" option (typically option 2)
  agm session select-option my-session 2

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
	selectOptionCmd.Flags().BoolVar(
		&selectOptionForce,
		"force",
		false,
		"Bypass safety guards (human typing/attached detection)",
	)

	sessionCmd.AddCommand(selectOptionCmd)
}

func runSelectOption(cmd *cobra.Command, args []string) (retErr error) {
	sessionName := args[0]
	optionNumber := args[1]

	// Audit trail: log enriched event on exit (captures both success and failure)
	defer func() {
		auditArgs := map[string]string{
			"option":  optionNumber,
			"force":   fmt.Sprintf("%v", selectOptionForce),
		}
		if selectOptionPrompt != "" {
			auditArgs["has_prompt"] = "true"
		}
		logCommandAudit("session.select-option", sessionName, auditArgs, retErr)
	}()

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

	// Safety guard check (unless --force is set)
	if !selectOptionForce {
		guardResult := safety.Check(sessionName, safety.GuardOptions{
			SkipUninitialized: true, // select-option only makes sense on initialized sessions
			SkipMidResponse:   true, // selecting options is for prompt UIs
		})
		if !guardResult.Safe {
			return fmt.Errorf("safety guard blocked select-option on session '%s':\n\n%sTo bypass: agm session select-option %s %s --force",
				sessionName, guardResult.Error(), sessionName, optionNumber)
		}
	}

	// Verify session state via capture-pane before sending any keys.
	// Bug fix: select-option must confirm a prompt with options is actually
	// visible, otherwise keys go to the wrong UI state.
	paneContent, err := tmux.CapturePaneOutput(sessionName, 30)
	if err != nil {
		return fmt.Errorf("failed to capture pane for state verification: %w", err)
	}
	if !verifyOptionPromptVisible(paneContent) {
		return fmt.Errorf("no option prompt detected in session '%s' — capture-pane shows no permission prompt or numbered options; refusing to send keys blindly", sessionName)
	}

	if err := navigateAndSubmitOption(sessionName, optionNumber); err != nil {
		return err
	}

	// Print success message
	successMsg := fmt.Sprintf("Selected option %s in session '%s'", optionNumber, sessionName)
	if selectOptionPrompt != "" {
		successMsg += fmt.Sprintf(" with custom text (%d chars)", len(selectOptionPrompt))
	}
	ui.PrintSuccess(successMsg)

	return nil
}

// navigateAndSubmitOption sends the necessary Down-arrow presses to land on
// the requested option (1 = no presses, 2 = one Down, etc.), then either
// submits with Enter or — if --prompt is set — Tab into the custom input
// field, types the prompt, and submits it.
func navigateAndSubmitOption(sessionName, optionNumber string) error {
	numPresses := int(optionNumber[0] - '1')
	for i := 0; i < numPresses; i++ {
		if err := tmux.SendKeys(sessionName, "Down"); err != nil {
			return fmt.Errorf("failed to send Down key (press %d/%d): %w", i+1, numPresses, err)
		}
		time.Sleep(100 * time.Millisecond)
	}
	if selectOptionPrompt == "" {
		if err := tmux.SendKeys(sessionName, "Enter"); err != nil {
			return fmt.Errorf("failed to send Enter key: %w", err)
		}
		return nil
	}
	if err := tmux.SendKeys(sessionName, "Tab"); err != nil {
		return fmt.Errorf("failed to send Tab key: %w", err)
	}
	time.Sleep(100 * time.Millisecond)
	if err := tmux.SendPromptLiteral(sessionName, selectOptionPrompt, true); err != nil {
		return fmt.Errorf("failed to send custom prompt: %w", err)
	}
	return nil
}

// verifyOptionPromptVisible checks capture-pane output for evidence of an
// option prompt (permission prompt or AskUserQuestion with numbered options).
func verifyOptionPromptVisible(paneContent string) bool {
	// Check for permission prompt indicators
	for _, indicator := range ops.PermissionPromptIndicators {
		if strings.Contains(paneContent, indicator) {
			return true
		}
	}

	// Check for numbered options (AskUserQuestion pattern: "1. ...", "2. ...")
	if strings.Contains(paneContent, "1. ") && strings.Contains(paneContent, "2. ") {
		return true
	}

	// Check for ❯ selector on numbered options (Claude Code permission UI)
	if strings.Contains(paneContent, "❯") && strings.Contains(paneContent, "1.") {
		return true
	}

	return false
}
