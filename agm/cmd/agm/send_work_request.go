package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/messages"
	"github.com/vbonnet/dear-agent/agm/internal/tmux"
	"github.com/vbonnet/dear-agent/agm/internal/ui"
)

var (
	wrTitle              string
	wrDescription        string
	wrPriority           string
	wrScope              string
	wrAcceptanceCriteria []string
	wrChecks             []string
	wrDryRun             bool
)

// WorkRequestMessage is the JSON structure for a work-request message
type WorkRequestMessage struct {
	Type         string            `json:"type"`
	ID           string            `json:"id"`
	From         string            `json:"from"`
	Title        string            `json:"title"`
	Description  string            `json:"description"`
	Priority     string            `json:"priority"`
	Scope        string            `json:"scope"`
	Acceptance   []string          `json:"acceptance_criteria,omitempty"`
	Verification WorkRequestVerify `json:"verification"`
}

// WorkRequestVerify holds verification info for work requests
type WorkRequestVerify struct {
	Originator string   `json:"originator"`
	Checks     []string `json:"checks,omitempty"`
}

var sendWorkRequestCmd = &cobra.Command{
	Use:   "work-request <recipient>",
	Short: "Send a structured work request to a session",
	Long: `Send a structured WorkRequest JSON message to a session.

The work-request command generates a properly formatted WorkRequest JSON payload
and sends it to the specified recipient session via agm send msg.

Auto-populated fields:
  • type: "work-request"
  • from: current session name (auto-detected)
  • id: auto-generated unique message ID
  • verification.originator: sender session name

Examples:
  # Basic work request
  agm send work-request worker-1 --title "Fix login bug" --description "Users can't log in"

  # With acceptance criteria and checks
  agm send work-request worker-1 \
    --title "Add retry logic" \
    --description "Add exponential backoff to API calls" \
    --priority P1 --scope S \
    --acceptance-criteria "Retries up to 3 times" \
    --acceptance-criteria "Uses exponential backoff" \
    --check "go test ./..."

  # Preview without sending
  agm send work-request worker-1 --dry-run \
    --title "test" --description "test desc"

Required flags:
  --title         Work request title
  --description   What needs to be done`,
	Args: cobra.ExactArgs(1),
	RunE: runSendWorkRequest,
}

func init() {
	sendWorkRequestCmd.Flags().StringVar(&wrTitle, "title", "", "Work request title (required)")
	sendWorkRequestCmd.Flags().StringVar(&wrDescription, "description", "", "What needs to be done (required)")
	sendWorkRequestCmd.Flags().StringVar(&wrPriority, "priority", "P2", "Priority: P0/P1/P2/P3")
	sendWorkRequestCmd.Flags().StringVar(&wrScope, "scope", "M", "Scope: XS/S/M/L/XL")
	sendWorkRequestCmd.Flags().StringSliceVar(&wrAcceptanceCriteria, "acceptance-criteria", nil, "Acceptance criteria (repeatable)")
	sendWorkRequestCmd.Flags().StringSliceVar(&wrChecks, "check", nil, "Verification check commands (repeatable)")
	sendWorkRequestCmd.Flags().BoolVar(&wrDryRun, "dry-run", false, "Print JSON without sending")

	_ = sendWorkRequestCmd.MarkFlagRequired("title")
	_ = sendWorkRequestCmd.MarkFlagRequired("description")

	sendGroupCmd.AddCommand(sendWorkRequestCmd)
}

func runSendWorkRequest(cmd *cobra.Command, args []string) error {
	recipient := args[0]

	// Validate priority
	validPriorities := map[string]bool{"P0": true, "P1": true, "P2": true, "P3": true}
	if !validPriorities[wrPriority] {
		return fmt.Errorf("invalid priority '%s': must be one of P0, P1, P2, P3", wrPriority)
	}

	// Validate scope
	validScopes := map[string]bool{"XS": true, "S": true, "M": true, "L": true, "XL": true}
	if !validScopes[wrScope] {
		return fmt.Errorf("invalid scope '%s': must be one of XS, S, M, L, XL", wrScope)
	}

	// Get adapter and determine sender
	adapter, _ := getStorage()
	if adapter != nil {
		defer adapter.Close()
	}

	senderName, err := determineSender(adapter)
	if err != nil {
		return err
	}

	// Generate message ID
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

	// Build work request
	msg := WorkRequestMessage{
		Type:        "work-request",
		ID:          messageID,
		From:        senderName,
		Title:       wrTitle,
		Description: wrDescription,
		Priority:    wrPriority,
		Scope:       wrScope,
		Acceptance:  wrAcceptanceCriteria,
		Verification: WorkRequestVerify{
			Originator: senderName,
			Checks:     wrChecks,
		},
	}

	jsonBytes, err := json.MarshalIndent(msg, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal work request: %w", err)
	}
	jsonStr := string(jsonBytes)

	// Dry run: print and exit
	if wrDryRun {
		fmt.Println(jsonStr)
		return nil
	}

	// Send via tmux (same pattern as sendDirectly)
	return sendWorkRequestMessage(recipient, senderName, messageID, jsonStr)
}

// sendWorkRequestMessage delivers a work-request JSON message to a recipient
func sendWorkRequestMessage(recipient, sender, messageID, jsonPayload string) error {
	// Check recipient exists in tmux
	exists, err := tmux.HasSession(recipient)
	if err != nil {
		return fmt.Errorf("failed to check tmux session: %w", err)
	}
	if !exists {
		return fmt.Errorf("session '%s' does not exist in tmux.\n\nSuggestions:\n  • List sessions: agm session list\n  • Create session: agm session new %s", recipient, recipient)
	}

	// Format with metadata header
	formattedMessage := formatMessageWithMetadata(sender, messageID, "", jsonPayload)

	// Log the message
	homeDir, err := os.UserHomeDir()
	if err == nil {
		logsDir := filepath.Join(homeDir, ".agm", "logs", "messages")
		logger, logErr := messages.NewMessageLogger(logsDir)
		if logErr == nil {
			logEntry := messages.CreateLogEntry(messageID, sender, recipient, jsonPayload, "")
			if err := logger.LogMessage(logEntry); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to log message: %v\n", err)
			}
		}
	}

	// Write pending file and send via tmux
	if err := messages.WritePendingFile(recipient, messageID, formattedMessage); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to write pending file: %v\n", err)
	}

	if err := tmux.SendMultiLinePromptSafe(recipient, formattedMessage, true); err != nil {
		return fmt.Errorf("failed to send work request: %w", err)
	}

	ui.PrintSuccess(fmt.Sprintf("Sent work-request to '%s' from '%s' [ID: %s]", recipient, sender, messageID))
	return nil
}

// BuildWorkRequestJSON creates a WorkRequestMessage and returns JSON (exported for testing)
func BuildWorkRequestJSON(sender, messageID, title, description, priority, scope string, acceptance, checks []string) (string, error) {
	msg := WorkRequestMessage{
		Type:        "work-request",
		ID:          messageID,
		From:        sender,
		Title:       title,
		Description: description,
		Priority:    priority,
		Scope:       scope,
		Acceptance:  acceptance,
		Verification: WorkRequestVerify{
			Originator: sender,
			Checks:     checks,
		},
	}

	jsonBytes, err := json.MarshalIndent(msg, "", "  ")
	if err != nil {
		return "", err
	}
	return string(jsonBytes), nil
}
