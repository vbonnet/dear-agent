package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"time"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/agent"
	"github.com/vbonnet/dear-agent/agm/internal/daemon"
	"github.com/vbonnet/dear-agent/agm/internal/delegation"
	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/messages"
	"github.com/vbonnet/dear-agent/agm/internal/monitoring"
	"github.com/vbonnet/dear-agent/agm/internal/state"
	"github.com/vbonnet/dear-agent/agm/internal/safety"
	"github.com/vbonnet/dear-agent/agm/internal/send"
	"github.com/vbonnet/dear-agent/agm/internal/session"
	"github.com/vbonnet/dear-agent/agm/internal/tmux"
	"github.com/vbonnet/dear-agent/agm/internal/ui"
)

var (
	sessionSendPrompt      string
	sessionSendPromptFile  string
	sessionSendPromptStdin bool
	sessionSendSender      string
	sessionSendReplyTo     string
	sessionSendPriority    string // --priority flag (fyi, background, normal, urgent, critical)
	msgTo                  string // --to flag for explicit recipient list
	msgWorkspace           string // --workspace flag for filtering
	msgAll                 bool   // --all flag for sending to all active sessions
	msgIncludeSelf         bool   // --include-self flag for including sender in --all
	msgDelegate            bool   // --delegate flag to track message as a pending delegation
	msgDelegateSummary     string // --delegate-summary for delegation task summary
)

// Priority levels and their instructions injected into message headers
var priorityInstructions = map[string]string{
	"critical":   "DROP everything. Handle this immediately.",
	"urgent":     "Pause your current work to handle this request.",
	"normal":     "",
	"background": "Handle this when you have a natural pause in your current work.",
	"fyi":        "Informational only. Continue your current work.",
}

// priorityToQueuePriority maps --priority flag values to queue priority constants
var priorityToQueuePriority = map[string]string{
	"critical":   messages.PriorityCritical,
	"urgent":     messages.PriorityHigh,
	"normal":     messages.PriorityMedium,
	"background": messages.PriorityLow,
	"fyi":        messages.PriorityLow,
}

// Sender name validation regex: alphanumeric, dash, underscore only
var senderNameRegex = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

var sendMsgCmd = &cobra.Command{
	Use:   "msg [session-name]",
	Short: "Send a message to one or more sessions",
	Long: `Send a message/prompt to one or more AGM sessions.

Features:
  • Multi-recipient: Send to multiple sessions sequentially
  • Glob patterns: Use wildcards to match session names
  • Workspace filtering: Filter recipients by workspace
  • Message priority: Control urgency with --priority (fyi, background, normal, urgent, critical)
  • Literal mode: Uses tmux -l flag to prevent special character interpretation
  • Reliable execution: Prompt is executed as command, not queued as pasted text
  • Large prompts: Supports up to 10KB prompt files
  • Sender attribution: Messages tagged with sender name, unique ID, and timestamp
  • Message threading: Link related messages with --reply-to
  • Audit trail: All messages logged to ~/.agm/logs/messages/

MULTI-RECIPIENT DELIVERY:
  - Single recipient: agm send msg session1 --prompt "..."
  - Comma-separated: agm send msg --to session1,session2,session3 --prompt "..."
  - Glob pattern: agm send msg --to "*research*" --prompt "..."
  - All active sessions: agm send msg --all --prompt "..."
  - All in workspace: agm send msg --all --workspace oss --prompt "..."

SENDER ATTRIBUTION:
  - If running in a AGM session: sender is auto-detected (tamper-resistant)
  - If NOT in AGM session: --sender flag is REQUIRED
  - Sender name must match: ^[a-zA-Z0-9_-]+$ (no spaces)

MESSAGE THREADING:
  - Each message gets a unique ID for tracking
  - Use --reply-to to link messages in conversation threads

Examples:
  # Send to single session (backward compatible)
  agm send msg my-session --prompt "Please review the code"

  # Send to multiple sessions (comma-separated)
  agm send msg --to session1,session2,session3 --prompt "Status update"

  # Send to all sessions matching pattern
  agm send msg --to "*research*" --prompt "Experiment complete"

  # Send to all active sessions
  agm send msg --all --prompt "System update complete"

  # Send to all sessions in workspace
  agm send msg --all --workspace oss --prompt "Deploy complete"

  # Send from external process (must specify sender)
  agm send msg my-session --sender astrocyte --prompt "Diagnosis complete"

  # Reply to a previous message
  agm send msg my-session --reply-to 1738612345678-sender-001 --prompt "Looks good!"

  # Send a prompt from file
  agm send msg my-session --prompt-file /path/to/prompt.txt

  # Send a prompt from stdin (agent-friendly)
  echo "Please review" | agm send msg my-session --prompt-stdin

Requirements:
  • At least one recipient (positional arg or --to flag)
  • Sessions must be running (active tmux session)
  • Requires either --prompt, --prompt-file, or --prompt-stdin flag

See Also:
  • agm send reject - Reject permission prompts with custom reasons
  • agm session logs - View message audit trail
  • agm admin doctor - Check session health`,
	Args: cobra.MaximumNArgs(1),
	RunE: runSend,
}

func init() {
	sendMsgCmd.Flags().StringVar(
		&sessionSendPrompt,
		"prompt",
		"",
		"Prompt text to send to session",
	)
	sendMsgCmd.Flags().StringVar(
		&sessionSendPromptFile,
		"prompt-file",
		"",
		"File containing prompt to send (max 10KB)",
	)
	sendMsgCmd.Flags().BoolVar(
		&sessionSendPromptStdin,
		"prompt-stdin",
		false,
		"Read prompt from stdin",
	)
	sendMsgCmd.Flags().StringVar(
		&sessionSendSender,
		"sender",
		"",
		"Sender identifier (required if not in AGM session)",
	)
	sendMsgCmd.Flags().StringVar(
		&sessionSendReplyTo,
		"reply-to",
		"",
		"Message ID to reply to (creates conversation thread)",
	)
	sendMsgCmd.Flags().StringVar(
		&sessionSendPriority,
		"priority",
		"normal",
		"Message priority: fyi, background, normal (default), urgent, critical",
	)
	sendMsgCmd.Flags().StringVar(
		&msgTo,
		"to",
		"",
		"Recipient specification (comma-separated or glob)",
	)
	sendMsgCmd.Flags().StringVar(
		&msgWorkspace,
		"workspace",
		"",
		"Filter recipients by workspace",
	)
	sendMsgCmd.Flags().BoolVar(
		&msgAll,
		"all",
		false,
		"Send to all active sessions (excludes archived and sender)",
	)
	sendMsgCmd.Flags().BoolVar(
		&msgIncludeSelf,
		"include-self",
		false,
		"Include sender session in --all recipients (default: excluded)",
	)
	sendMsgCmd.Flags().BoolVar(
		&msgDelegate,
		"delegate",
		false,
		"Track this message as a pending delegation (blocks archive until resolved)",
	)
	sendMsgCmd.Flags().StringVar(
		&msgDelegateSummary,
		"delegate-summary",
		"",
		"Task summary for the delegation (used with --delegate)",
	)

	sendMsgCmd.MarkFlagsMutuallyExclusive("prompt", "prompt-file", "prompt-stdin")
	sendMsgCmd.MarkFlagsOneRequired("prompt", "prompt-file", "prompt-stdin")
	sendMsgCmd.MarkFlagsMutuallyExclusive("to", "all")

	// Deprecated --force flag: kept as hidden no-op for backward compatibility
	var forceDeprecated bool
	sendMsgCmd.Flags().BoolVar(&forceDeprecated, "force", false, "deprecated: safety checks always run")
	_ = sendMsgCmd.Flags().MarkHidden("force")
	_ = sendMsgCmd.Flags().MarkDeprecated("force", "safety checks always run; --force is no longer needed")

	sendGroupCmd.AddCommand(sendMsgCmd)

	// Set default delivery function for sequential delivery
	send.SetDefaultDeliveryFunc(deliveryFunc)
}

func runSend(cmd *cobra.Command, args []string) error {
	// Validate priority flag
	if _, ok := priorityInstructions[sessionSendPriority]; !ok {
		return fmt.Errorf("invalid priority '%s': must be one of fyi, background, normal, urgent, critical", sessionSendPriority)
	}

	// Parse recipients (supports single, comma-separated, glob patterns, --all)
	spec, err := send.ParseRecipients(args, msgTo, msgWorkspace, msgAll)
	if err != nil {
		return err
	}

	// For backward compatibility: if we have a single direct recipient, use the original fast path
	// This preserves all existing behavior and ensures zero regression
	if spec.Type == "direct" && len(spec.Recipients) == 1 {
		recipientSession := spec.Recipients[0]
		return runSendSingle(recipientSession)
	}

	// Multi-recipient path: resolve and deliver in parallel
	return runSendMulti(spec)
}

// runSendSingle handles single-recipient sends (original behavior, backward compatible)
func runSendSingle(recipientSession string) (retErr error) {
	defer func() {
		logCommandAudit("send.msg", recipientSession, sendSingleAuditArgs(recipientSession), retErr)
	}()

	adapter, _ := getStorage()
	if adapter != nil {
		defer adapter.Close()
	}

	senderName, err := determineSender(adapter)
	if err != nil {
		return err
	}
	if err := validateSenderAndReplyTo(senderName); err != nil {
		return err
	}
	if err := enforceSendRateLimit(senderName); err != nil {
		return err
	}
	if err := ensureRecipientReady(recipientSession, adapter); err != nil {
		return err
	}

	message, err := readSendMessageContent()
	if err != nil {
		return err
	}

	messageID, formattedMessage, err := buildAndLogMessage(senderName, recipientSession, message)
	if err != nil {
		return err
	}

	currentState, tmuxName := resolveRecipientState(recipientSession, adapter)
	canReceive := session.CheckSessionDelivery(tmuxName)
	return dispatchSendByCanReceive(recipientSession, tmuxName, senderName, messageID, formattedMessage, message, currentState, canReceive, adapter)
}

// sendSingleAuditArgs builds the audit arg map for runSendSingle.
func sendSingleAuditArgs(recipientSession string) map[string]string {
	auditArgs := map[string]string{
		"recipient": recipientSession,
		"sender":    sessionSendSender,
		"priority":  sessionSendPriority,
	}
	if sessionSendReplyTo != "" {
		auditArgs["reply_to"] = sessionSendReplyTo
	}
	if msgDelegate {
		auditArgs["delegate"] = "true"
	}
	return auditArgs
}

// validateSenderAndReplyTo enforces format/length checks on senderName and
// the optional --reply-to message ID.
func validateSenderAndReplyTo(senderName string) error {
	if !senderNameRegex.MatchString(senderName) {
		return fmt.Errorf("invalid sender name '%s': must match pattern ^[a-zA-Z0-9_-]+$ (alphanumeric, dash, underscore only)", senderName)
	}
	if len(senderName) < 1 || len(senderName) > 64 {
		return fmt.Errorf("invalid sender name '%s': must be 1-64 characters", senderName)
	}
	if sessionSendReplyTo != "" && !messages.ValidateMessageID(sessionSendReplyTo) {
		return fmt.Errorf("invalid --reply-to message ID format: '%s'\n\nExpected format: {timestamp}-{sender}-{seq}\nExample: 1738612345678-sender-001", sessionSendReplyTo)
	}
	return nil
}

// enforceSendRateLimit applies the per-sender rate limiter (10/min).
func enforceSendRateLimit(senderName string) error {
	rateLimiter := messages.GetRateLimiter(senderName)
	allowed, _, err := rateLimiter.Allow()
	if !allowed {
		return fmt.Errorf("rate limit exceeded: %w\n\nLimit: 10 messages per minute\nTry again in a few seconds", err)
	}
	return nil
}

// ensureRecipientReady verifies the recipient tmux session exists, runs the
// safety guard, and wakes any stale monitors.
func ensureRecipientReady(recipientSession string, adapter *dolt.Adapter) error {
	exists, err := tmux.HasSession(recipientSession)
	if err != nil {
		return fmt.Errorf("failed to check tmux session: %w", err)
	}
	if !exists {
		return fmt.Errorf("session '%s' does not exist in tmux.\n\nSuggestions:\n  • List sessions: agm session list\n  • Create session: agm session new %s", recipientSession, recipientSession)
	}
	guardResult := safety.Check(recipientSession, safety.GuardOptions{SkipMidResponse: true})
	if !guardResult.Safe {
		return fmt.Errorf("safety guard blocked send on session '%s':\n\n%s",
			recipientSession, guardResult.Error())
	}
	checkAndWakeMonitors(recipientSession, adapter)
	return nil
}

// readSendMessageContent reads the message body from --prompt, --prompt-file,
// or stdin (whichever is set).
func readSendMessageContent() (string, error) {
	switch {
	case sessionSendPrompt != "":
		return sessionSendPrompt, nil
	case sessionSendPromptFile != "":
		fileContent, err := os.ReadFile(sessionSendPromptFile)
		if err != nil {
			return "", fmt.Errorf("failed to read prompt file: %w", err)
		}
		return string(fileContent), nil
	case sessionSendPromptStdin:
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return "", fmt.Errorf("failed to read from stdin: %w", err)
		}
		return string(data), nil
	}
	return "", nil
}

// buildAndLogMessage generates the unique message ID, formats the body with
// metadata, and writes a log entry. Returns (messageID, formattedMessage, err).
func buildAndLogMessage(senderName, recipientSession, message string) (string, string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", "", fmt.Errorf("failed to get home directory: %w", err)
	}
	stateDir := filepath.Join(homeDir, ".agm", "state")
	idGen, err := messages.NewMessageIDGenerator(senderName, stateDir)
	if err != nil {
		return "", "", fmt.Errorf("failed to create message ID generator: %w", err)
	}
	messageID, err := idGen.Next()
	if err != nil {
		return "", "", fmt.Errorf("failed to generate message ID: %w", err)
	}
	formattedMessage := formatMessageWithMetadata(senderName, messageID, sessionSendReplyTo, message)
	logsDir := filepath.Join(homeDir, ".agm", "logs", "messages")
	logger, err := messages.NewMessageLogger(logsDir)
	if err != nil {
		return "", "", fmt.Errorf("failed to create logger: %w", err)
	}
	logEntry := messages.CreateLogEntry(messageID, senderName, recipientSession, message, sessionSendReplyTo)
	if err := logger.LogMessage(logEntry); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to log message: %v\n", err)
	}
	return messageID, formattedMessage, nil
}

// resolveRecipientState resolves the recipient's display state for persistence
// and returns (currentState, tmuxName).
func resolveRecipientState(recipientSession string, adapter *dolt.Adapter) (string, string) {
	var currentState string
	tmuxName := recipientSession
	m, manifestPath, resolveErr := session.ResolveIdentifier(recipientSession, cfg.SessionsDir, adapter)
	if resolveErr != nil {
		return currentState, tmuxName
	}
	if m.Tmux.SessionName != "" {
		tmuxName = m.Tmux.SessionName
	}
	currentState = session.ResolveSessionState(tmuxName, m.State, m.Claude.UUID, m.StateUpdatedAt)
	if currentState != m.State {
		if err := session.UpdateSessionState(manifestPath, currentState, "hybrid", m.SessionID, adapter); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to persist session state: %v\n", err)
		}
	}
	return currentState, tmuxName
}

// dispatchSendByCanReceive routes the formatted message to the appropriate
// delivery path based on the CanReceive state read from the recipient pane.
func dispatchSendByCanReceive(recipientSession, tmuxName, senderName, messageID, formattedMessage, message, currentState string, canReceive state.CanReceive, adapter *dolt.Adapter) error {
	switch canReceive {
	case state.CanReceiveYes:
		if err := sendDirectly(recipientSession, senderName, messageID, formattedMessage, sessionSendPromptFile, adapter); err != nil {
			return err
		}
		recordDelegation(senderName, recipientSession, messageID, message)
		return nil
	case state.CanReceiveNotFound:
		return fmt.Errorf("session '%s' tmux session disappeared during delivery", recipientSession)
	case state.CanReceiveQueue:
		if err := queueMessage(recipientSession, senderName, messageID, formattedMessage, currentState); err != nil {
			return err
		}
		recordDelegation(senderName, recipientSession, messageID, message)
		return nil
	case state.CanReceiveOverlay:
		fmt.Fprintf(os.Stderr, "⚠ Session '%s' has active overlay (Background Tasks) — attempting auto-recovery\n", recipientSession)
		if err := dismissOverlayAndDeliver(tmuxName, recipientSession, senderName, messageID, formattedMessage, sessionSendPromptFile, adapter); err != nil {
			return err
		}
		recordDelegation(senderName, recipientSession, messageID, message)
		return nil
	case state.CanReceiveNo:
		fmt.Fprintf(os.Stderr, "⚠ Session '%s' has active permission prompt — message queued for delivery after resolution\n", recipientSession)
		return queueMessage(recipientSession, senderName, messageID, formattedMessage, currentState)
	default:
		fmt.Fprintf(os.Stderr, "Warning: unknown CanReceive state '%s', queueing\n", canReceive)
		if err := queueMessage(recipientSession, senderName, messageID, formattedMessage, currentState); err != nil {
			return err
		}
		recordDelegation(senderName, recipientSession, messageID, message)
		return nil
	}
}

// queueMessage queues a message for later delivery (non-disruptive default)
func queueMessage(recipientSession, senderName, messageID, formattedMessage, currentState string) error {
	// Create message queue
	queue, err := messages.NewMessageQueue()
	if err != nil {
		// Queue creation failed - fall back to direct send with warning
		fmt.Fprintf(os.Stderr, "Warning: failed to create message queue: %v\n", err)
		fallbackAdapter, _ := getStorage()
		return sendDirectly(recipientSession, senderName, messageID, formattedMessage, "", fallbackAdapter)
	}
	defer queue.Close()

	// Check if daemon is running before queueing
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}
	pidFile := filepath.Join(homeDir, ".agm", "daemon.pid")
	daemonRunning := daemon.IsRunning(pidFile)

	// If daemon is not running, fall back to direct tmux delivery
	// instead of refusing — the message is better delivered directly than not at all
	if !daemonRunning {
		fmt.Fprintf(os.Stderr, "⚠ Daemon not running — falling back to direct tmux delivery for '%s'\n", recipientSession)
		fallbackAdapter, _ := getStorage()
		if fallbackAdapter != nil {
			defer fallbackAdapter.Close()
		}
		return sendDirectly(recipientSession, senderName, messageID, formattedMessage, "", fallbackAdapter)
	}

	// Create queue entry with mapped priority
	queuePriority := priorityToQueuePriority[sessionSendPriority]
	if queuePriority == "" {
		queuePriority = messages.PriorityMedium
	}
	entry := &messages.QueueEntry{
		MessageID: messageID,
		From:      senderName,
		To:        recipientSession,
		Message:   formattedMessage,
		Priority:  queuePriority,
		QueuedAt:  time.Now(),
	}

	if err := queue.Enqueue(entry); err != nil {
		return fmt.Errorf("failed to queue message: %w", err)
	}

	// Write pending file for hook-based delivery (best-effort)
	if err := messages.WritePendingFile(recipientSession, messageID, formattedMessage); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to write pending file: %v\n", err)
	}

	fmt.Printf("⏳ Queued to '%s' (session %s) [ID: %s]\n", recipientSession, currentState, messageID)
	fmt.Printf("   Message will be delivered when session becomes READY.\n")
	fmt.Printf("   View queue: agm session queue list\n")

	return nil
}

// sendDirectly sends a message directly to a session without queuing.
// Supports both tmux-based (Claude, Gemini) and API-based (OpenAI) sessions.
//
// For DONE-state sends, the underlying tmux send does not emit an ESC keystroke
// because the session is already at the prompt — sending ESC is redundant and
// can exit plan mode.
func sendDirectly(recipientSession, senderName, messageID, formattedMessage, promptFile string, adapter *dolt.Adapter) error {
	// Try to load manifest to determine agent type
	m, _, err := session.ResolveIdentifier(recipientSession, cfg.SessionsDir, adapter)
	if err != nil {
		// No manifest found - fall back to tmux-based send for legacy sessions
		return sendViaTmux(recipientSession, senderName, messageID, formattedMessage, promptFile, false)
	}

	// Determine delivery method based on harness type
	harnessType := m.Harness
	if harnessType == "" {
		harnessType = "claude-code" // Default to Claude Code for backward compatibility
	}

	// Check if this is an API-based harness (OpenAI, etc.)
	if isAPIBasedAgent(harnessType) {
		// Use Agent interface for API-based sessions
		return sendViaAgent(m, senderName, messageID, formattedMessage, promptFile)
	}

	// Fall back to tmux for CLI-based harnesses (Claude Code, Gemini CLI)
	return sendViaTmux(recipientSession, senderName, messageID, formattedMessage, promptFile, false)
}

// sendViaTmux sends a message via tmux (for CLI-based agents like Claude, Gemini)
// Bug fix (2026-03-14): Added shouldInterrupt parameter to control ESC behavior
func sendViaTmux(recipientSession, senderName, messageID, formattedMessage, promptFile string, shouldInterrupt bool) error {
	// Write pending file for hook-based delivery (best-effort, in addition to tmux)
	if err := messages.WritePendingFile(recipientSession, messageID, formattedMessage); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to write pending file: %v\n", err)
	}

	// Send using SAFE method (waits for prompt, with conditional interrupt)
	if err := tmux.SendMultiLinePromptSafe(recipientSession, formattedMessage, shouldInterrupt); err != nil {
		return fmt.Errorf("failed to send prompt: %w", err)
	}

	// Print success message with message ID
	successMsg := fmt.Sprintf("✓ Sent to '%s' from '%s' (%d chars) [ID: %s] [via: tmux]", recipientSession, senderName, len(formattedMessage), messageID)
	if promptFile != "" {
		successMsg += fmt.Sprintf(" [file: %s]", promptFile)
	}
	ui.PrintSuccess(successMsg)

	return nil
}

// sendViaAgent sends a message via Agent interface (for API-based harnesses like OpenAI)
func sendViaAgent(m *manifest.Manifest, senderName, messageID, formattedMessage, promptFile string) error {
	// Get harness type from manifest
	harnessType := m.Harness
	if harnessType == "" {
		return fmt.Errorf("manifest missing harness type")
	}

	// Create harness adapter via factory
	agentAdapter, err := agent.GetHarness(harnessType)
	if err != nil {
		return fmt.Errorf("failed to create harness adapter for type '%s': %w", harnessType, err)
	}

	// Create message
	msg := agent.Message{
		ID:        messageID,
		Role:      agent.RoleUser,
		Content:   formattedMessage,
		Timestamp: time.Now(),
		Metadata: map[string]interface{}{
			"sender":    senderName,
			"source":    "agm_send",
			"file_path": promptFile,
		},
	}

	// Write pending file for hook-based delivery (best-effort, in addition to API)
	if err := messages.WritePendingFile(m.Name, messageID, formattedMessage); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to write pending file: %v\n", err)
	}

	// Send message via Agent interface
	sessionID := agent.SessionID(m.SessionID)
	if err := agentAdapter.SendMessage(sessionID, msg); err != nil {
		return fmt.Errorf("failed to send message via harness: %w", err)
	}

	// Print success message with message ID
	successMsg := fmt.Sprintf("✓ Sent to '%s' from '%s' (%d chars) [ID: %s] [via: %s API]", m.Name, senderName, len(formattedMessage), messageID, m.Harness)
	if promptFile != "" {
		successMsg += fmt.Sprintf(" [file: %s]", promptFile)
	}
	ui.PrintSuccess(successMsg)

	return nil
}

// isAPIBasedAgent returns true if the harness type uses API-based communication
// (as opposed to tmux-based CLI communication)
func isAPIBasedAgent(harnessType string) bool {
	switch harnessType {
	case "codex-cli":
		return true
	case "claude-code", "gemini-cli", "opencode-cli":
		return false
	default:
		// Unknown harnesses default to tmux-based for backward compatibility
		return false
	}
}

// determineSender returns the sender name either from auto-detection or --sender flag
func determineSender(adapter *dolt.Adapter) (string, error) {
	// If --sender flag provided, use it
	if sessionSendSender != "" {
		return sessionSendSender, nil
	}

	// Try auto-detection (only works in AGM sessions)
	detectedName, err := session.GetCurrentSessionName(cfg.SessionsDir, adapter)
	if err != nil {
		return "", fmt.Errorf("--sender flag is required when not in a AGM session.\n\nError: %w\n\nExamples:\n  • From daemon: agm send msg session --sender astrocyte --prompt \"...\"\n  • From script: agm send msg session --sender my-script --prompt \"...\"", err)
	}

	return detectedName, nil
}

// formatMessageWithMetadata prefixes the message with sender, ID, priority, timestamp, and optional reply-to
func formatMessageWithMetadata(sender, messageID, replyTo, message string) string {
	now := time.Now().UTC().Format(time.RFC3339)
	header := fmt.Sprintf("[From: %s | ID: %s | Sent: %s", sender, messageID, now)
	if replyTo != "" {
		header += fmt.Sprintf(" | Reply-To: %s", replyTo)
	}
	header += "]"

	// Add priority instruction line if not normal
	instruction := priorityInstructions[sessionSendPriority]
	if instruction != "" {
		return fmt.Sprintf("%s\n[Priority: %s] %s\n%s", header, sessionSendPriority, instruction, message)
	}
	return fmt.Sprintf("%s\n%s", header, message)
}

// runSendMulti handles multi-recipient message delivery with sequential execution
func runSendMulti(spec *send.RecipientSpec) (retErr error) {
	defer func() {
		logCommandAudit("send.msg.multi", "", multiAuditArgs(spec), retErr)
	}()

	adapter, err := getStorage()
	if err != nil {
		return fmt.Errorf("failed to connect to Dolt storage: %w", err)
	}
	defer adapter.Close()

	senderName, err := determineSender(adapter)
	if err != nil {
		return err
	}
	if err := validateSenderAndReplyTo(senderName); err != nil {
		return err
	}

	if !msgIncludeSelf {
		spec.ExcludeSender = senderName
	}
	resolver := &doltSessionResolver{adapter: adapter}
	resolvedSpec, err := send.ResolveRecipients(spec, resolver)
	if err != nil {
		return err
	}

	message, err := readMultiSendMessageContent()
	if err != nil {
		return err
	}
	if err := enforceSendRateLimit(senderName); err != nil {
		return err
	}

	jobs, homeDir, err := buildMultiDeliveryJobs(senderName, message, resolvedSpec.Recipients)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	results := send.SequentialDeliver(ctx, jobs, deliveryFunc)

	report := send.GenerateReport(results)
	report.PrintReport()

	logMultiResults(homeDir, senderName, message, jobs, results)
	if msgDelegate {
		for _, result := range results {
			if result.Success {
				recordDelegation(senderName, result.Recipient, result.MessageID, message)
			}
		}
	}
	if report.HasFailures() {
		return fmt.Errorf("some deliveries failed (see report above)")
	}
	return nil
}

// multiAuditArgs builds the audit map for runSendMulti.
func multiAuditArgs(spec *send.RecipientSpec) map[string]string {
	auditArgs := map[string]string{
		"recipient_count": fmt.Sprintf("%d", len(spec.Recipients)),
		"priority":        sessionSendPriority,
		"type":            spec.Type,
	}
	if msgAll {
		auditArgs["all"] = "true"
	}
	if msgWorkspace != "" {
		auditArgs["workspace"] = msgWorkspace
	}
	if msgDelegate {
		auditArgs["delegate"] = "true"
	}
	return auditArgs
}

// readMultiSendMessageContent reads the message body for runSendMulti, with an
// extra 10KB cap on --prompt-file uploads to protect against accidental large
// files in fan-out mode.
func readMultiSendMessageContent() (string, error) {
	switch {
	case sessionSendPrompt != "":
		return sessionSendPrompt, nil
	case sessionSendPromptFile != "":
		fileInfo, err := os.Stat(sessionSendPromptFile)
		if err != nil {
			return "", fmt.Errorf("failed to stat prompt file: %w", err)
		}
		const maxFileSize = 10 * 1024
		if fileInfo.Size() > maxFileSize {
			return "", fmt.Errorf("prompt file too large: %d bytes (max %d bytes)", fileInfo.Size(), maxFileSize)
		}
		fileContent, err := os.ReadFile(sessionSendPromptFile)
		if err != nil {
			return "", fmt.Errorf("failed to read prompt file: %w", err)
		}
		return string(fileContent), nil
	case sessionSendPromptStdin:
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return "", fmt.Errorf("failed to read from stdin: %w", err)
		}
		return string(data), nil
	}
	return "", nil
}

// buildMultiDeliveryJobs creates one DeliveryJob per recipient with a unique
// message ID. Returns the jobs, the resolved homeDir (used by the caller for
// logging), and any error from ID generation.
func buildMultiDeliveryJobs(senderName, message string, recipients []string) ([]*send.DeliveryJob, string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, "", fmt.Errorf("failed to get home directory: %w", err)
	}
	stateDir := filepath.Join(homeDir, ".agm", "state")
	idGen, err := messages.NewMessageIDGenerator(senderName, stateDir)
	if err != nil {
		return nil, homeDir, fmt.Errorf("failed to create message ID generator: %w", err)
	}
	jobs := make([]*send.DeliveryJob, 0, len(recipients))
	for _, recipient := range recipients {
		msgID, err := idGen.Next()
		if err != nil {
			return nil, homeDir, fmt.Errorf("failed to generate message ID: %w", err)
		}
		formattedMsg := formatMessageWithMetadata(senderName, msgID, sessionSendReplyTo, message)
		jobs = append(jobs, &send.DeliveryJob{
			Recipient:        recipient,
			Sender:           senderName,
			MessageID:        msgID,
			FormattedMessage: formattedMsg,
			PromptFile:       sessionSendPromptFile,
			ShouldInterrupt:  false,
			SessionsDir:      cfg.SessionsDir,
		})
	}
	return jobs, homeDir, nil
}

// logMultiResults writes a log entry for each successful delivery.
func logMultiResults(homeDir, senderName, message string, jobs []*send.DeliveryJob, results []*send.DeliveryResult) {
	logsDir := filepath.Join(homeDir, ".agm", "logs", "messages")
	logger, err := messages.NewMessageLogger(logsDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to create logger: %v\n", err)
		return
	}
	for _, result := range results {
		if !result.Success {
			continue
		}
		for _, job := range jobs {
			if job.MessageID != result.MessageID {
				continue
			}
			logEntry := messages.CreateLogEntry(job.MessageID, senderName, job.Recipient, message, sessionSendReplyTo)
			if err := logger.LogMessage(logEntry); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to log message to %s: %v\n", job.Recipient, err)
			}
			break
		}
	}
}

// deliveryFunc implements the actual message delivery for a single recipient
// This is used by SequentialDeliver for sequential message sending
func deliveryFunc(job *send.DeliveryJob) error {
	// Check recipient session exists in tmux
	exists, err := tmux.HasSession(job.Recipient)
	if err != nil {
		return fmt.Errorf("failed to check tmux session: %w", err)
	}
	if !exists {
		return fmt.Errorf("session '%s' does not exist in tmux", job.Recipient)
	}

	// Use the existing sendDirectly logic for actual delivery
	// This ensures consistent behavior with single-recipient sends
	return sendViaTmux(job.Recipient, job.Sender, job.MessageID, job.FormattedMessage, job.PromptFile, job.ShouldInterrupt)
}

// recordDelegation records a delegation if --delegate flag is set.
// Best-effort: logs warnings on failure but does not fail the send.
func recordDelegation(sender, recipient, messageID, message string) {
	if !msgDelegate {
		return
	}

	dir, err := delegation.DefaultDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to get delegation dir: %v\n", err)
		return
	}

	tracker, err := delegation.NewTracker(dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to create delegation tracker: %v\n", err)
		return
	}

	summary := msgDelegateSummary
	if summary == "" {
		// Use first 200 chars of message as summary
		summary = message
		if len(summary) > 200 {
			summary = summary[:200] + "..."
		}
	}

	d := &delegation.Delegation{
		MessageID:   messageID,
		From:        sender,
		To:          recipient,
		TaskSummary: summary,
	}

	if err := tracker.Record(d); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to record delegation: %v\n", err)
		return
	}

	fmt.Fprintf(os.Stderr, "📋 Delegation tracked: %s → %s [ID: %s]\n", sender, recipient, messageID)
	fmt.Fprintf(os.Stderr, "   Resolve with: agm delegation resolve %s %s\n", sender, messageID)
}

// dismissOverlayAndDeliver dismisses a UI overlay (e.g., Background Tasks view)
// by sending Left arrow key, waiting for the overlay to close, re-checking
// delivery readiness, and then delivering the message.
//
// Recovery sequence:
//  1. Send Left arrow key to dismiss the overlay
//  2. Wait 200ms for the overlay to close
//  3. Re-check delivery readiness (pane content)
//  4. If ready, deliver the message directly
//  5. If still blocked, queue for later delivery
func dismissOverlayAndDeliver(tmuxName, recipientSession, senderName, messageID, formattedMessage, promptFile string, adapter *dolt.Adapter) error {
	// Step 1: Send Left arrow to dismiss the overlay
	if err := tmux.SendKeys(tmuxName, "Left"); err != nil {
		return fmt.Errorf("failed to send Left key to dismiss overlay: %w", err)
	}

	// Step 2: Wait for overlay to close
	time.Sleep(200 * time.Millisecond)

	// Step 3: Re-check delivery readiness
	canReceive := session.CheckSessionDelivery(tmuxName)

	//nolint:exhaustive // intentional partial: handles the relevant subset
	switch canReceive {
	case state.CanReceiveYes:
		// Overlay dismissed, prompt visible — deliver directly
		fmt.Fprintf(os.Stderr, "✓ Overlay dismissed on '%s' — delivering message\n", recipientSession)
		return sendDirectly(recipientSession, senderName, messageID, formattedMessage, promptFile, adapter)

	case state.CanReceiveOverlay:
		// Overlay still visible — try Escape as fallback
		fmt.Fprintf(os.Stderr, "⚠ Overlay still active, trying Escape key...\n")
		if err := tmux.SendKeys(tmuxName, "Escape"); err != nil {
			return fmt.Errorf("failed to send Escape key to dismiss overlay: %w", err)
		}
		time.Sleep(200 * time.Millisecond)

		// Final re-check
		canReceive = session.CheckSessionDelivery(tmuxName)
		if canReceive == state.CanReceiveYes {
			fmt.Fprintf(os.Stderr, "✓ Overlay dismissed with Escape on '%s' — delivering message\n", recipientSession)
			return sendDirectly(recipientSession, senderName, messageID, formattedMessage, promptFile, adapter)
		}
		// Give up — queue the message
		fmt.Fprintf(os.Stderr, "⚠ Could not dismiss overlay on '%s' (state: %s) — queueing message\n", recipientSession, canReceive)
		return queueMessage(recipientSession, senderName, messageID, formattedMessage, "BACKGROUND_TASKS")

	default:
		// Overlay dismissed but session is in unexpected state — queue for safety
		fmt.Fprintf(os.Stderr, "⚠ Overlay dismissed but session '%s' is %s — queueing message\n", recipientSession, canReceive)
		return queueMessage(recipientSession, senderName, messageID, formattedMessage, string(canReceive))
	}
}

// doltSessionResolver wraps dolt.Adapter to implement send.SessionResolver
type doltSessionResolver struct {
	adapter *dolt.Adapter
}

func (r *doltSessionResolver) ResolveIdentifier(identifier string) (*manifest.Manifest, error) {
	return r.adapter.ResolveIdentifier(identifier)
}

func (r *doltSessionResolver) ListAllSessions() ([]*manifest.Manifest, error) {
	// List all active sessions (exclude archived)
	filter := &dolt.SessionFilter{
		Lifecycle: "", // Empty means active sessions only
	}
	return r.adapter.ListSessions(filter)
}

// checkAndWakeMonitors checks if a recipient session has monitors with stale
// loop heartbeats, and triggers wakes for any that are stale.
// This is the "fast-path" — when sending a message to session X, we proactively
// check X's monitors so the monitoring loop is awake to handle the message.
func checkAndWakeMonitors(recipientSession string, adapter *dolt.Adapter) {
	if adapter == nil {
		return
	}

	m, err := adapter.ResolveIdentifier(recipientSession)
	if err != nil || m == nil || len(m.Monitors) == 0 {
		return
	}

	for _, monitorSession := range m.Monitors {
		hb, err := monitoring.ReadHeartbeat("", monitorSession)
		if err != nil {
			continue // No heartbeat file — skip
		}

		if monitoring.CheckStaleness(hb) == "stale" {
			fmt.Fprintf(os.Stderr, "Monitor '%s' has stale heartbeat, sending wake...\n", monitorSession)

			// Best-effort wake — don't block message delivery on failure
			output, cmdErr := exec.Command("agm", "send", "wake-loop", monitorSession).CombinedOutput()
			if cmdErr != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to wake monitor '%s': %v (%s)\n",
					monitorSession, cmdErr, string(output))
			}
		}
	}
}
