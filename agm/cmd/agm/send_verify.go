package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	"github.com/vbonnet/dear-agent/agm/internal/messages"
	"github.com/vbonnet/dear-agent/agm/internal/tmux"
	"github.com/vbonnet/dear-agent/agm/internal/ui"
)

var (
	verifyRequestID    string
	verifyStatus       string
	verifyCheckResults []string
	verifyGaps         []string
	verifyDryRun       bool
)

// VerifyResultMessage is the JSON structure for a verify-result message
type VerifyResultMessage struct {
	Type      string        `json:"type"`
	ID        string        `json:"id"`
	From      string        `json:"from"`
	RequestID string        `json:"request_id"`
	Status    string        `json:"status"`
	Checks    []CheckResult `json:"checks,omitempty"`
	Gaps      []GapEntry    `json:"gaps,omitempty"`
}

// CheckResult represents a single verification check result
type CheckResult struct {
	Description string `json:"description"`
	Command     string `json:"command"`
	Result      string `json:"result"`
	Details     string `json:"details,omitempty"`
}

// GapEntry represents a gap found during verification
type GapEntry struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Criteria    string `json:"criteria,omitempty"`
}

var sendVerifyCmd = &cobra.Command{
	Use:   "verify <recipient>",
	Short: "Send a verification result to a session",
	Long: `Send a structured VerifyResult JSON message to a session.

The verify command generates a properly formatted VerifyResult JSON payload
and sends it to the specified recipient session.

Auto-populated fields:
  • type: "verify-result"
  • from: current session name (auto-detected)
  • id: auto-generated unique message ID

Examples:
  # Verified with all checks passing
  agm send verify orchestrator \
    --request-id 1234567890-orch-001 \
    --status VERIFIED \
    --check-result "tests|go test ./...|PASS|all 42 tests pass"

  # Gaps found
  agm send verify orchestrator \
    --request-id 1234567890-orch-001 \
    --status GAPS_FOUND \
    --check-result "tests|go test ./...|FAIL|3 failures" \
    --gap "Missing error handling|No retry on timeout|Must retry 3 times"

  # Preview without sending
  agm send verify orchestrator --dry-run \
    --request-id 1234567890-orch-001 --status VERIFIED

Check result format (pipe-separated):
  "description|command|PASS or FAIL|details"

Gap format (pipe-separated):
  "title|description|criteria"

Required flags:
  --request-id   Original work request ID
  --status       VERIFIED or GAPS_FOUND`,
	Args: cobra.ExactArgs(1),
	RunE: runSendVerify,
}

func init() {
	sendVerifyCmd.Flags().StringVar(&verifyRequestID, "request-id", "", "Original work request ID (required)")
	sendVerifyCmd.Flags().StringVar(&verifyStatus, "status", "", "Result status: VERIFIED or GAPS_FOUND (required)")
	sendVerifyCmd.Flags().StringSliceVar(&verifyCheckResults, "check-result", nil, "Check result: \"description|command|PASS or FAIL|details\" (repeatable)")
	sendVerifyCmd.Flags().StringSliceVar(&verifyGaps, "gap", nil, "Gap found: \"title|description|criteria\" (repeatable)")
	sendVerifyCmd.Flags().BoolVar(&verifyDryRun, "dry-run", false, "Print JSON without sending")

	_ = sendVerifyCmd.MarkFlagRequired("request-id")
	_ = sendVerifyCmd.MarkFlagRequired("status")

	sendGroupCmd.AddCommand(sendVerifyCmd)
}

func runSendVerify(cmd *cobra.Command, args []string) error {
	recipient := args[0]

	// Validate status
	if verifyStatus != "VERIFIED" && verifyStatus != "GAPS_FOUND" {
		return fmt.Errorf("invalid status '%s': must be VERIFIED or GAPS_FOUND", verifyStatus)
	}

	// Parse check results
	checks, err := parseCheckResults(verifyCheckResults)
	if err != nil {
		return err
	}

	// Parse gaps
	gaps, err := parseGaps(verifyGaps)
	if err != nil {
		return err
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

	// Build verify result
	msg := VerifyResultMessage{
		Type:      "verify-result",
		ID:        messageID,
		From:      senderName,
		RequestID: verifyRequestID,
		Status:    verifyStatus,
		Checks:    checks,
		Gaps:      gaps,
	}

	jsonBytes, err := json.MarshalIndent(msg, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal verify result: %w", err)
	}
	jsonStr := string(jsonBytes)

	// Dry run: print and exit
	if verifyDryRun {
		fmt.Println(jsonStr)
		return nil
	}

	// Send via tmux
	return sendVerifyMessage(recipient, senderName, messageID, jsonStr, adapter)
}

// parseCheckResults parses pipe-separated check result strings
func parseCheckResults(raw []string) ([]CheckResult, error) {
	if len(raw) == 0 {
		return nil, nil
	}

	results := make([]CheckResult, 0, len(raw))
	for _, r := range raw {
		parts := strings.SplitN(r, "|", 4)
		if len(parts) < 3 {
			return nil, fmt.Errorf("invalid --check-result format: '%s'\n\nExpected: \"description|command|PASS or FAIL|details\"\nMinimum 3 pipe-separated fields required", r)
		}

		result := strings.TrimSpace(parts[2])
		if result != "PASS" && result != "FAIL" {
			return nil, fmt.Errorf("invalid check result '%s': third field must be PASS or FAIL", result)
		}

		cr := CheckResult{
			Description: strings.TrimSpace(parts[0]),
			Command:     strings.TrimSpace(parts[1]),
			Result:      result,
		}
		if len(parts) == 4 {
			cr.Details = strings.TrimSpace(parts[3])
		}
		results = append(results, cr)
	}
	return results, nil
}

// parseGaps parses pipe-separated gap strings
func parseGaps(raw []string) ([]GapEntry, error) {
	if len(raw) == 0 {
		return nil, nil
	}

	gaps := make([]GapEntry, 0, len(raw))
	for _, g := range raw {
		parts := strings.SplitN(g, "|", 3)
		if len(parts) < 2 {
			return nil, fmt.Errorf("invalid --gap format: '%s'\n\nExpected: \"title|description|criteria\"\nMinimum 2 pipe-separated fields required", g)
		}

		entry := GapEntry{
			Title:       strings.TrimSpace(parts[0]),
			Description: strings.TrimSpace(parts[1]),
		}
		if len(parts) == 3 {
			entry.Criteria = strings.TrimSpace(parts[2])
		}
		gaps = append(gaps, entry)
	}
	return gaps, nil
}

// sendVerifyMessage delivers a verify-result JSON message to a recipient
func sendVerifyMessage(recipient, sender, messageID, jsonPayload string, adapter *dolt.Adapter) error {
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
		return fmt.Errorf("failed to send verify result: %w", err)
	}

	ui.PrintSuccess(fmt.Sprintf("Sent verify-result to '%s' from '%s' [ID: %s] [status: %s]", recipient, sender, messageID, verifyStatus))
	return nil
}
