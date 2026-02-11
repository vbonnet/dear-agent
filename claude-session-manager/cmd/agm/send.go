package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"github.com/spf13/cobra"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/messages"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/session"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/tmux"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/ui"
)

var (
	sessionSendPrompt     string
	sessionSendPromptFile string
	sessionSendSender     string
	sessionSendReplyTo    string
)

// Sender name validation regex: alphanumeric, dash, underscore only
var senderNameRegex = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

var sendCmd = &cobra.Command{
	Use:   "send <session-name>",
	Short: "Send a message to a running session",
	Long: `Send a message/prompt to a running AGM session, interrupting any active thinking state.

Features:
  • Auto-interrupt: Sends ESC to interrupt thinking before sending prompt
  • Literal mode: Uses tmux -l flag to prevent special character interpretation
  • Reliable execution: Prompt is executed as command, not queued as pasted text
  • Large prompts: Supports up to 10KB prompt files
  • Sender attribution: Messages tagged with sender name and unique ID
  • Message threading: Link related messages with --reply-to
  • Audit trail: All messages logged to ~/.agm/logs/messages/

SENDER ATTRIBUTION:
  - If running in a AGM session: sender is auto-detected (tamper-resistant)
  - If NOT in AGM session: --sender flag is REQUIRED
  - Sender name must match: ^[a-zA-Z0-9_-]+$ (no spaces)

MESSAGE THREADING:
  - Each message gets a unique ID for tracking
  - Use --reply-to to link messages in conversation threads

Examples:
  # Send from within a AGM session (auto-detects sender)
  agm session send my-session --prompt "Please review the code"

  # Send from external process (must specify sender)
  agm session send my-session --sender astrocyte --prompt "Diagnosis complete"

  # Reply to a previous message
  agm session send my-session --reply-to 1738612345678-sender-001 --prompt "Looks good!"

  # Send from cron job
  agm session send my-session --sender cron-backup --prompt "Backup finished"

  # Send a prompt from file
  agm session send my-session --prompt-file /path/to/prompt.txt

  # Send diagnosis request to stuck session
  agm session send gemini-research --sender astrocyte --prompt "⚠️ Your session was stuck. Please analyze what caused the hang."

Requirements:
  • Session must be running (active tmux session)
  • Requires either --prompt or --prompt-file flag

See Also:
  • agm session reject - Reject permission prompts with custom reasons
  • agm session logs - View message audit trail
  • agm admin doctor - Check session health`,
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
	sendCmd.Flags().StringVar(
		&sessionSendSender,
		"sender",
		"",
		"Sender identifier (required if not in AGM session)",
	)
	sendCmd.Flags().StringVar(
		&sessionSendReplyTo,
		"reply-to",
		"",
		"Message ID to reply to (creates conversation thread)",
	)
	sendCmd.MarkFlagsMutuallyExclusive("prompt", "prompt-file")
	sendCmd.MarkFlagsOneRequired("prompt", "prompt-file")

	sessionCmd.AddCommand(sendCmd)
}

func runSend(cmd *cobra.Command, args []string) error {
	recipientSession := args[0]

	// Determine sender (auto-detect or use --sender flag)
	senderName, err := determineSender()
	if err != nil {
		return err
	}

	// Validate sender name format
	if !senderNameRegex.MatchString(senderName) {
		return fmt.Errorf("invalid sender name '%s': must match pattern ^[a-zA-Z0-9_-]+$ (alphanumeric, dash, underscore only)", senderName)
	}

	// Validate sender name length
	if len(senderName) < 1 || len(senderName) > 64 {
		return fmt.Errorf("invalid sender name '%s': must be 1-64 characters", senderName)
	}

	// Validate --reply-to if provided
	if sessionSendReplyTo != "" {
		if !messages.ValidateMessageID(sessionSendReplyTo) {
			return fmt.Errorf("invalid --reply-to message ID format: '%s'\n\nExpected format: {timestamp}-{sender}-{seq}\nExample: 1738612345678-sender-001", sessionSendReplyTo)
		}
	}

	// Check rate limit
	rateLimiter := messages.GetRateLimiter(senderName)
	allowed, remaining, err := rateLimiter.Allow()
	if !allowed {
		return fmt.Errorf("rate limit exceeded: %w\n\nLimit: 10 messages per minute\nTry again in a few seconds", err)
	}
	_ = remaining // Will be used in logging metadata

	// Check recipient session exists in tmux
	exists, err := tmux.HasSession(recipientSession)
	if err != nil {
		return fmt.Errorf("failed to check tmux session: %w", err)
	}
	if !exists {
		return fmt.Errorf("session '%s' does not exist in tmux.\n\nSuggestions:\n  • List sessions: agm session list\n  • Create session: agm session new %s", recipientSession, recipientSession)
	}

	// Get message content
	var message string
	if sessionSendPrompt != "" {
		message = sessionSendPrompt
	} else if sessionSendPromptFile != "" {
		// Read file content (max 10KB checked in SendPromptFileSafe)
		fileContent, err := os.ReadFile(sessionSendPromptFile)
		if err != nil {
			return fmt.Errorf("failed to read prompt file: %w", err)
		}
		message = string(fileContent)
	}

	// Generate unique message ID
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}
	stateDir := filepath.Join(homeDir, ".agm", "state")
	idGen, err := messages.NewMessageIDGenerator(senderName, stateDir)
	if err != nil {
		return fmt.Errorf("failed to create message ID generator: %w", err)
	}
	messageID, err := idGen.Next()
	if err != nil {
		return fmt.Errorf("failed to generate message ID: %w", err)
	}

	// Format message with sender prefix and message ID
	formattedMessage := formatMessageWithMetadata(senderName, messageID, sessionSendReplyTo, message)

	// Log the message before sending
	logsDir := filepath.Join(homeDir, ".agm", "logs", "messages")
	logger, err := messages.NewMessageLogger(logsDir)
	if err != nil {
		return fmt.Errorf("failed to create logger: %w", err)
	}

	logEntry := messages.CreateLogEntry(messageID, senderName, recipientSession, message, sessionSendReplyTo)
	if err := logger.LogMessage(logEntry); err != nil {
		// Log error but don't fail the send operation
		fmt.Fprintf(os.Stderr, "Warning: failed to log message: %v\n", err)
	}

	// Send using SAFE method (waits for prompt, handles interrupts)
	// This is the key difference from ace0ea5 - we use SendMultiLinePromptSafe
	// which calls WaitForPromptSimple before sending (fix from 76d3053)
	if err := tmux.SendMultiLinePromptSafe(recipientSession, formattedMessage); err != nil {
		return fmt.Errorf("failed to send prompt: %w", err)
	}

	// Print success message with message ID
	successMsg := fmt.Sprintf("Sent to '%s' from '%s' (%d chars) [ID: %s]", recipientSession, senderName, len(formattedMessage), messageID)
	if sessionSendPromptFile != "" {
		successMsg += fmt.Sprintf(" [file: %s]", sessionSendPromptFile)
	}
	ui.PrintSuccess(successMsg)

	return nil
}

// determineSender returns the sender name either from auto-detection or --sender flag
func determineSender() (string, error) {
	// If --sender flag provided, use it
	if sessionSendSender != "" {
		return sessionSendSender, nil
	}

	// Try auto-detection (only works in AGM sessions)
	detectedName, err := session.GetCurrentSessionName(cfg.SessionsDir)
	if err != nil {
		return "", fmt.Errorf("--sender flag is required when not in a AGM session.\n\nError: %w\n\nExamples:\n  • From daemon: agm session send session --sender astrocyte --prompt \"...\"\n  • From script: agm session send session --sender my-script --prompt \"...\"", err)
	}

	return detectedName, nil
}

// formatMessageWithMetadata prefixes the message with sender, ID, and optional reply-to
func formatMessageWithMetadata(sender, messageID, replyTo, message string) string {
	header := fmt.Sprintf("[From: %s | ID: %s", sender, messageID)
	if replyTo != "" {
		header += fmt.Sprintf(" | Reply-To: %s", replyTo)
	}
	header += "]"
	return fmt.Sprintf("%s\n%s", header, message)
}
