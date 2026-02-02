package main

import (
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/spf13/cobra"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/tmux"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/ui"
)

var (
	rejectReason     string
	rejectReasonFile string
)

var rejectCmd = &cobra.Command{
	Use:   "reject <session-name>",
	Short: "Reject a permission prompt with a reason",
	Long: `Reject a permission prompt in a CSM session by navigating to "No" and providing a rejection reason.

This automates the flow:
1. Navigate to "No" option using arrow keys
2. Press Tab to add additional instructions
3. Paste rejection reason (e.g., tool usage violation prompt)
4. Send Enter to submit

Common use case: Rejecting bash commands that should use Claude Code tools instead.

Examples:
  # Reject with inline reason
  csm reject my-session --reason "Use Read tool instead of cat"

  # Reject with violation prompt from file
  csm reject my-session --reason-file ~/src/ws/oss/tool-usage-analysis/prompts/VIOLATION-PROMPTS.md

IMPORTANT: Session must be showing a permission prompt with "No" option.`,
	Args: cobra.ExactArgs(1),
	RunE: runReject,
}

func init() {
	rejectCmd.Flags().StringVar(
		&rejectReason,
		"reason",
		"",
		"Rejection reason to send",
	)
	rejectCmd.Flags().StringVar(
		&rejectReasonFile,
		"reason-file",
		"",
		"File containing rejection reason (max 10KB)",
	)
	rejectCmd.MarkFlagsMutuallyExclusive("reason", "reason-file")
	rejectCmd.MarkFlagsOneRequired("reason", "reason-file")

	rootCmd.AddCommand(rejectCmd)
}

func runReject(cmd *cobra.Command, args []string) error {
	sessionName := args[0]

	// Check session exists in tmux
	exists, err := tmux.HasSession(sessionName)
	if err != nil {
		return fmt.Errorf("failed to check tmux session: %w", err)
	}
	if !exists {
		return fmt.Errorf("session '%s' does not exist in tmux.\\n\\nSuggestions:\\n  • List sessions: csm list\\n  • Create session: csm new %s", sessionName, sessionName)
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
		if err := exec.Command("tmux", "send-keys", "-t", sessionName, "Down").Run(); err != nil {
			return fmt.Errorf("failed to navigate to No option: %w", err)
		}
		time.Sleep(100 * time.Millisecond)
	}
	time.Sleep(200 * time.Millisecond)

	// Step 2: Press Tab to add instructions
	if err := exec.Command("tmux", "send-keys", "-t", sessionName, "Tab").Run(); err != nil {
		return fmt.Errorf("failed to press Tab: %w", err)
	}
	time.Sleep(300 * time.Millisecond)

	// Step 3: Send rejection reason in literal mode
	if err := exec.Command("tmux", "send-keys", "-t", sessionName, "-l", reason).Run(); err != nil {
		return fmt.Errorf("failed to send rejection reason: %w", err)
	}
	time.Sleep(300 * time.Millisecond)

	// Step 4: Send Enter to submit
	if err := exec.Command("tmux", "send-keys", "-t", sessionName, "C-m").Run(); err != nil {
		return fmt.Errorf("failed to send Enter: %w", err)
	}

	ui.PrintSuccess(fmt.Sprintf("Rejected permission prompt in '%s' with reason (%d chars)", sessionName, len(reason)))
	return nil
}

// detectNoOptionPosition detects how many Down presses are needed to reach "No" option
// Returns:
//   1 for 2-option prompts (1. Yes, 2. No)
//   2 for 3-option prompts (1. Yes, 2. Don't ask, 3. No)
func detectNoOptionPosition(sessionName string) (int, error) {
	// Capture pane content
	out, err := exec.Command("tmux", "capture-pane", "-t", sessionName, "-p").Output()
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
		if (containsReject(line, "2. No") || containsReject(line, "2.No")) {
			noOptionNum = 2
			break
		}
		if (containsReject(line, "3. No") || containsReject(line, "3.No")) {
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
