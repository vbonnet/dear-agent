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
	approveReason       string
	approveReasonFile   string
	approveAutoContinue bool
)

var sendApproveCmd = &cobra.Command{
	Use:   "approve <session-name>",
	Short: "Approve a permission prompt with optional reason",
	Long: `Approve a permission prompt with optional reason, automating the Enter flow.

Features:
  • Automated navigation: Selects "Yes" option (usually default)
  • Custom reasoning: Adds approval reason as additional instructions (optional)
  • Smart extraction: Extracts "## Standard Prompt (Recommended)" from markdown files
  • Literal mode: Uses tmux -l flag for reliable text transmission

This automates the flow:
  1. Detect prompt type (2 or 3 options) and ensure "Yes" is selected
  2. Optionally press Tab to add additional instructions
  3. Send approval reason in literal mode (if provided)
  4. Submit with Enter

Use Cases:
  • Approving bash commands with additional context
  • Providing approval notes for audit trail
  • Automated approval with conditional instructions

Examples:
  # Simple approval (no reason)
  agm send approve my-session

  # Approve with inline reason
  agm send approve my-session --reason "LGTM, approved for testing"

  # Approve with approval notes from file
  agm send approve my-session --reason-file ~/prompts/APPROVAL-NOTES.md

  # Approve and auto-continue
  agm send approve my-session --auto-continue

  # Approve with custom feedback
  agm send approve task --reason "Approved - please use error handling patterns from ~/docs/patterns.md"

Workflow Executed:
  1. Verify "Yes" option is selected (default position)
  2. If reason provided: Send Tab key to add additional instructions
  3. Send approval reason text in literal mode (if provided)
  4. Send Enter to submit
  5. Optionally send another Enter to auto-continue (--auto-continue flag)

Requirements:
  • Session must be showing a permission prompt with a "Yes" option
  • Reason is optional - can approve without providing reason

See Also:
  • agm send reject - Reject permissions with reasons
  • agm send msg - Send messages to running sessions
  • agm admin doctor - Check session health`,
	Args: cobra.ExactArgs(1),
	RunE: runApprove,
}

func init() {
	sendApproveCmd.Flags().StringVarP(
		&approveReason,
		"reason",
		"r",
		"",
		"Approval reason to send (optional)",
	)
	sendApproveCmd.Flags().StringVar(
		&approveReasonFile,
		"reason-file",
		"",
		"File containing approval reason (max 10KB)",
	)
	sendApproveCmd.Flags().BoolVar(
		&approveAutoContinue,
		"auto-continue",
		false,
		"Automatically press Enter after approval",
	)
	sendApproveCmd.MarkFlagsMutuallyExclusive("reason", "reason-file")

	sendGroupCmd.AddCommand(sendApproveCmd)
}

func runApprove(cmd *cobra.Command, args []string) error {
	sessionName := args[0]

	// Check session exists in tmux
	exists, err := tmux.HasSession(sessionName)
	if err != nil {
		return fmt.Errorf("failed to check tmux session: %w", err)
	}
	if !exists {
		return fmt.Errorf("session '%s' does not exist in tmux.\n\nSuggestions:\n  • List sessions: agm session list\n  • Create session: agm session new %s", sessionName, sessionName)
	}

	// Safety guard check
	guardResult := safety.Check(sessionName, safety.GuardOptions{
		SkipUninitialized: true, // approve only makes sense on initialized sessions
		SkipMidResponse:   true, // approve is for permission prompts, not mid-response
	})
	if !guardResult.Safe {
		return fmt.Errorf("safety guard blocked approve on session '%s':\n\n%s",
			sessionName, guardResult.Error())
	}

	reason, err := loadApprovalReason()
	if err != nil {
		return err
	}

	// Get AGM socket path for all tmux commands
	socketPath := tmux.GetSocketPath()

	// Step 1: Verify "Yes" is selected (typically default, no navigation needed)
	// We still detect the prompt to ensure we're on a permission screen
	if err := detectYesOptionPresent(sessionName); err != nil {
		return fmt.Errorf("failed to detect Yes option: %w", err)
	}

	// Step 2: If reason provided, press Tab to add instructions
	if reason != "" {
		if err := exec.Command("tmux", "-S", socketPath, "send-keys", "-t", sessionName, "Tab").Run(); err != nil {
			return fmt.Errorf("failed to press Tab: %w", err)
		}
		time.Sleep(300 * time.Millisecond)

		// Step 3: Send approval reason in literal mode
		if err := exec.Command("tmux", "-S", socketPath, "send-keys", "-t", sessionName, "-l", reason).Run(); err != nil {
			return fmt.Errorf("failed to send approval reason: %w", err)
		}
		time.Sleep(300 * time.Millisecond)
	}

	// Step 4: Send Enter to submit
	if err := exec.Command("tmux", "-S", socketPath, "send-keys", "-t", sessionName, "C-m").Run(); err != nil {
		return fmt.Errorf("failed to send Enter: %w", err)
	}

	// Step 5: Optionally auto-continue
	if approveAutoContinue {
		time.Sleep(500 * time.Millisecond)
		if err := exec.Command("tmux", "-S", socketPath, "send-keys", "-t", sessionName, "C-m").Run(); err != nil {
			ui.PrintWarning(fmt.Sprintf("Failed to auto-continue: %v", err))
		}
	}

	if reason != "" {
		ui.PrintSuccess(fmt.Sprintf("Approved permission prompt in '%s' with reason (%d chars)", sessionName, len(reason)))
	} else {
		ui.PrintSuccess(fmt.Sprintf("Approved permission prompt in '%s'", sessionName))
	}
	return nil
}

// loadApprovalReason reads the approval reason from --reason or --reason-file.
// For .md reason files it tries to extract the "## Standard Prompt (Recommended)"
// section first, falling back to the raw file contents.
func loadApprovalReason() (string, error) {
	if approveReason != "" {
		return approveReason, nil
	}
	if approveReasonFile == "" {
		return "", nil
	}
	content, err := os.ReadFile(approveReasonFile)
	if err != nil {
		return "", fmt.Errorf("failed to read reason file: %w", err)
	}
	if len(approveReasonFile) > 3 && approveReasonFile[len(approveReasonFile)-3:] == ".md" {
		if extracted := extractApprovalPrompt(string(content)); extracted != "" {
			return extracted, nil
		}
	}
	return string(content), nil
}

// detectYesOptionPresent verifies that a permission prompt with "Yes" option is present
func detectYesOptionPresent(sessionName string) error {
	// Capture pane content using AGM socket
	socketPath := tmux.GetSocketPath()
	out, err := exec.Command("tmux", "-S", socketPath, "capture-pane", "-t", sessionName, "-p").Output()
	if err != nil {
		return fmt.Errorf("failed to capture pane: %w", err)
	}

	content := string(out)
	lines := splitApproveLines(content)

	// Look for numbered options with "Yes"
	// Patterns: "   1. Yes" (with optional whitespace and selection marker)
	yesFound := false
	for _, line := range lines {
		if containsApprove(line, "1. Yes") || containsApprove(line, "1.Yes") {
			yesFound = true
			break
		}
	}

	if !yesFound {
		return fmt.Errorf("could not find Yes option in permission prompt")
	}

	return nil
}

// extractApprovalPrompt extracts the "## Standard Prompt (Recommended)" section from markdown
func extractApprovalPrompt(content string) string {
	// Look for pattern: ## Standard Prompt (Recommended)\n```\n...\n```
	// This matches the format in approval notes files

	start := -1
	end := -1

	// Find "## Standard Prompt"
	lines := splitApproveLines(content)
	for i, line := range lines {
		if containsApprove(line, "## Standard Prompt") {
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

func splitApproveLines(s string) []string {
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

func containsApprove(s, substr string) bool {
	return len(s) >= len(substr) && indexOfApprove(s, substr) != -1
}

func indexOfApprove(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
