package main

import (
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/safety"
	"github.com/vbonnet/dear-agent/agm/internal/tmux"
	"github.com/vbonnet/dear-agent/agm/internal/ui"
)

var (
	rejectReason     string
	rejectReasonFile string
)

var sendRejectCmd = &cobra.Command{
	Use:   "reject <session-name>",
	Short: "Reject a permission prompt with custom reason",
	Long: `Reject a permission prompt with a custom reason, automating the Down → Tab → paste → Enter flow.

Features:
  • Automated navigation: Navigates to "No" option using arrow keys
  • Custom reasoning: Adds rejection reason as additional instructions
  • Smart extraction: Extracts "## Standard Prompt (Recommended)" from markdown files
  • Literal mode: Uses tmux -l flag for reliable text transmission

This automates the flow:
  1. Detect prompt type (2 or 3 options) and navigate to "No"
  2. Press Tab to add additional instructions
  3. Send rejection reason in literal mode
  4. Submit with Enter

Use Cases:
  • Rejecting bash commands that violate tool usage guidelines
  • Providing feedback on why a permission was denied
  • Automated enforcement of coding standards

Examples:
  # Reject with inline reason
  agm send reject my-session --reason "Use Read tool instead of cat"

  # Reject with violation prompt from file
  agm send reject my-session --reason-file ~/prompts/VIOLATION-PROMPTS.md

  # Reject with custom feedback
  agm send reject task --reason "Please use absolute paths and separate tool calls. Read the bash tool guidance at ~/docs/bash-rules.md"

Workflow Executed:
  1. Send Down key(s) to navigate to "No" option (auto-detects 2 or 3 option prompts)
  2. Send Tab key to add additional instructions
  3. Send rejection reason text in literal mode
  4. Send Enter to submit

Requirements:
  • Session must be showing a permission prompt with a "No" option
  • Requires either --reason or --reason-file flag

See Also:
  • agm send msg - Send messages to running sessions
  • agm admin doctor - Check session health`,
	Args: cobra.ExactArgs(1),
	RunE: runReject,
}

func init() {
	sendRejectCmd.Flags().StringVar(
		&rejectReason,
		"reason",
		"",
		"Rejection reason to send",
	)
	sendRejectCmd.Flags().StringVar(
		&rejectReasonFile,
		"reason-file",
		"",
		"File containing rejection reason (max 10KB)",
	)
	sendRejectCmd.MarkFlagsMutuallyExclusive("reason", "reason-file")
	sendRejectCmd.MarkFlagsOneRequired("reason", "reason-file")

	sendGroupCmd.AddCommand(sendRejectCmd)
}

func runReject(cmd *cobra.Command, args []string) error {
	sessionName := args[0]

	// Check session exists in tmux
	exists, err := tmux.HasSession(sessionName)
	if err != nil {
		return fmt.Errorf("failed to check tmux session: %w", err)
	}
	if !exists {
		return fmt.Errorf("session '%s' does not exist in tmux.\\n\\nSuggestions:\\n  • List sessions: agm session list\\n  • Create session: agm session new %s", sessionName, sessionName)
	}

	// Safety guard check
	guardResult := safety.Check(sessionName, safety.GuardOptions{
		SkipUninitialized: true, // reject only makes sense on initialized sessions
		SkipMidResponse:   true, // reject is for permission prompts
	})
	if !guardResult.Safe {
		return fmt.Errorf("safety guard blocked reject on session '%s':\n\n%s",
			sessionName, guardResult.Error())
	}

	// Get rejection reason
	var reason string
	if rejectReason != "" {
		reason = rejectReason
	} else if rejectReasonFile != "" {
		// Read from file
		content, err := os.ReadFile(rejectReasonFile)
		if err != nil {
			return fmt.Errorf("failed to read reason file: %w", err)
		}

		// For .md files, extract the standard prompt
		if len(rejectReasonFile) > 3 && rejectReasonFile[len(rejectReasonFile)-3:] == ".md" {
			// Try to extract "## Standard Prompt (Recommended)" section
			extracted := extractStandardPrompt(string(content))
			if extracted != "" {
				reason = extracted
			} else {
				reason = string(content)
			}
		} else {
			reason = string(content)
		}
	}

	// Get AGM socket path for all tmux commands
	socketPath := tmux.GetSocketPath()

	// Step 1: Detect number of options and navigate to "No"
	// Permission prompts have either:
	//   2 options: 1. Yes, 2. No              (press Down once)
	//   3 options: 1. Yes, 2. Don't ask, 3. No (press Down twice)
	downPresses, err := detectNoOptionPosition(sessionName)
	if err != nil {
		return fmt.Errorf("failed to detect No option position: %w", err)
	}

	// Navigate to "No" option
	for i := 0; i < downPresses; i++ {
		if err := exec.Command("tmux", "-S", socketPath, "send-keys", "-t", sessionName, "Down").Run(); err != nil {
			return fmt.Errorf("failed to navigate to No option: %w", err)
		}
		time.Sleep(100 * time.Millisecond)
	}
	time.Sleep(200 * time.Millisecond)

	// Step 2: Press Tab to add instructions
	if err := exec.Command("tmux", "-S", socketPath, "send-keys", "-t", sessionName, "Tab").Run(); err != nil {
		return fmt.Errorf("failed to press Tab: %w", err)
	}
	time.Sleep(300 * time.Millisecond)

	// Step 3: Send rejection reason in literal mode
	if err := exec.Command("tmux", "-S", socketPath, "send-keys", "-t", sessionName, "-l", reason).Run(); err != nil {
		return fmt.Errorf("failed to send rejection reason: %w", err)
	}
	time.Sleep(300 * time.Millisecond)

	// Step 4: Send Enter to submit
	if err := exec.Command("tmux", "-S", socketPath, "send-keys", "-t", sessionName, "C-m").Run(); err != nil {
		return fmt.Errorf("failed to send Enter: %w", err)
	}

	ui.PrintSuccess(fmt.Sprintf("Rejected permission prompt in '%s' with reason (%d chars)", sessionName, len(reason)))
	return nil
}

// detectNoOptionPosition detects how many Down presses are needed to reach "No" option
// Returns:
//
//	1 for 2-option prompts (1. Yes, 2. No)
//	2 for 3-option prompts (1. Yes, 2. Don't ask, 3. No)
func detectNoOptionPosition(sessionName string) (int, error) {
	// Capture pane content using AGM socket
	socketPath := tmux.GetSocketPath()
	out, err := exec.Command("tmux", "-S", socketPath, "capture-pane", "-t", sessionName, "-p").Output()
	if err != nil {
		return 0, fmt.Errorf("failed to capture pane: %w", err)
	}

	content := string(out)
	lines := splitRejectLines(content)

	// Look for numbered options in reverse (No is always last)
	// Patterns: "   2. No" or "   3. No"
	noOptionNum := 0
	for i := len(lines) - 1; i >= 0; i-- {
		line := lines[i]
		// Check for "2. No" or "3. No" (with optional whitespace and selection marker)
		if containsReject(line, "2. No") || containsReject(line, "2.No") {
			noOptionNum = 2
			break
		}
		if containsReject(line, "3. No") || containsReject(line, "3.No") {
			noOptionNum = 3
			break
		}
	}

	if noOptionNum == 0 {
		return 0, fmt.Errorf("could not find No option in permission prompt")
	}

	// Return number of Down presses needed (option number - 1)
	// Option 2 = 1 Down press, Option 3 = 2 Down presses
	return noOptionNum - 1, nil
}

// extractStandardPrompt extracts the "## Standard Prompt (Recommended)" section from markdown
func extractStandardPrompt(content string) string {
	// Look for pattern: ## Standard Prompt (Recommended)\n```\n...\n```
	// This matches the format in VIOLATION-PROMPTS.md

	start := -1
	end := -1

	// Find "## Standard Prompt"
	lines := splitRejectLines(content)
	for i, line := range lines {
		if containsReject(line, "## Standard Prompt") {
			// Found section header, look for opening ```
			for j := i + 1; j < len(lines); j++ {
				if lines[j] == "```" {
					start = j + 1
					break
				}
			}
			break
		}
	}

	if start == -1 {
		return "" // Didn't find standard prompt section
	}

	// Find closing ```
	for i := start; i < len(lines); i++ {
		if lines[i] == "```" {
			end = i
			break
		}
	}

	if end == -1 {
		return "" // Didn't find closing fence
	}

	// Extract lines between fences
	extracted := ""
	for i := start; i < end; i++ {
		extracted += lines[i] + "\n"
	}

	return extracted
}

func splitRejectLines(s string) []string {
	result := []string{}
	current := ""
	for _, c := range s {
		if c == '\n' {
			result = append(result, current)
			current = ""
		} else {
			current += string(c)
		}
	}
	if current != "" {
		result = append(result, current)
	}
	return result
}

func containsReject(s, substr string) bool {
	return len(s) >= len(substr) && indexOfReject(s, substr) != -1
}

func indexOfReject(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
