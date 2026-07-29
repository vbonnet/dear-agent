package main

import (
	"context"
	"errors"
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
	"github.com/vbonnet/dear-agent/agm/internal/ops"
	"github.com/vbonnet/dear-agent/agm/internal/safety"
	"github.com/vbonnet/dear-agent/agm/internal/send"
	"github.com/vbonnet/dear-agent/agm/internal/session"
	"github.com/vbonnet/dear-agent/agm/internal/tmux"
	"github.com/vbonnet/dear-agent/agm/internal/ui"
	"github.com/vbonnet/dear-agent/internal/telemetry"
	"go.opentelemetry.io/otel/codes"
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
	msgForce               bool   // --force flag: force through queued-input/cooldown checks (ForceDelivery)
	msgForceReason         string // --reason: deprecated no-op (human_typing no longer blocks; kept for compat)
	msgAutonomous          bool   // --autonomous flag: session is unattended, skip human_typing detection
)

var sendMultiLinePromptSafeForHarnessContext = tmux.SendMultiLinePromptSafeForHarnessContext
var resolveSendRecipientHarness = bestEffortSendRecipientHarness

func sendStructuredPrompt(ctx context.Context, recipient, message string, shouldInterrupt bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return sendMultiLinePromptSafeForHarnessContext(ctx, recipient, message, shouldInterrupt, resolveSendRecipientHarness(recipient))
}

func bestEffortSendRecipientHarness(recipient string) string {
	adapter, err := getStorage()
	if err != nil {
		return ""
	}
	defer func() { _ = adapter.Close() }()
	sessionsDir := ""
	if cfg != nil {
		sessionsDir = cfg.SessionsDir
	}
	m, _, err := session.ResolveIdentifier(recipient, sessionsDir, adapter)
	if err != nil {
		return ""
	}
	return m.Harness
}

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

	sendMsgCmd.Flags().BoolVar(&msgForce, "force", false, "Replace only positively identified queued AGM input after exact harness and pane verification")
	sendMsgCmd.Flags().StringVar(&msgForceReason, "reason", "", "Deprecated no-op: human_typing no longer blocks, so --force needs no audited reason (accepted for compatibility)")
	sendMsgCmd.Flags().BoolVar(&msgAutonomous, "autonomous", false, "Mark the session unattended and allow the same narrow queued-AGM recovery as --force")

	sendGroupCmd.AddCommand(sendMsgCmd)

	// Set default delivery function for sequential delivery
	send.SetDefaultDeliveryFunc(deliveryFunc)
}

func runSend(cmd *cobra.Command, args []string) error {
	// Validate priority flag
	if _, ok := priorityInstructions[sessionSendPriority]; !ok {
		return fmt.Errorf("invalid priority '%s': must be one of fyi, background, normal, urgent, critical", sessionSendPriority)
	}

	// ce-v9in: in autonomous mode the recipient is unattended, so the tmux send
	// path stashes its own stale input (C-s) instead of blocking on it as if a
	// human were typing. Set the process-global flag for every delivery path.
	// AGM_AUTONOMOUS=1 lets the mesh spawner mark a whole session tree as
	// unattended, so peer-to-peer sends auto-clear without each call passing
	// --autonomous explicitly.
	tmux.SetAutonomousMode(msgAutonomous || os.Getenv("AGM_AUTONOMOUS") == "1")

	// Parse recipients (supports single, comma-separated, glob patterns, --all)
	spec, err := send.ParseRecipients(args, msgTo, msgWorkspace, msgAll)
	if err != nil {
		return err
	}

	// For backward compatibility: if we have a single direct recipient, use the original fast path
	// This preserves all existing behavior and ensures zero regression
	if spec.Type == "direct" && len(spec.Recipients) == 1 {
		recipientSession := spec.Recipients[0]
		return runSendSingle(cmd.Context(), recipientSession)
	}

	// Multi-recipient path: resolve and deliver sequentially under shared tmux safety.
	return runSendMulti(cmd.Context(), spec)
}

// runSendSingle handles single-recipient sends (original behavior, backward compatible)
func runSendSingle(ctx context.Context, recipientSession string) (retErr error) {
	// Telemetry: agm.session.execute span covering message dispatch.
	_, span := telemetry.SessionExecute(ctx, recipientSession)
	defer func() {
		if retErr != nil {
			span.RecordError(retErr)
			span.SetStatus(codes.Error, retErr.Error())
		}
		span.End()
	}()

	defer func() {
		logCommandAudit("send.msg", recipientSession, sendSingleAuditArgs(recipientSession), retErr)
	}()

	adapter, _ := getStorage()
	if adapter != nil {
		defer func() { _ = adapter.Close() }()
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
	prepareRecipientDelivery(recipientSession, adapter)

	message, err := readSendMessageContent()
	if err != nil {
		return err
	}

	messageID, formattedMessage, err := buildAndLogMessage(senderName, recipientSession, message)
	if err != nil {
		return err
	}

	currentState, tmuxName, _ := resolveRecipientState(recipientSession, adapter)
	return dispatchSend(ctx, recipientSession, tmuxName, senderName, messageID, formattedMessage, message, currentState, adapter)
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

// prepareRecipientDelivery records advisory telemetry and caller policy without
// deciding whether a send is safe. ops.SendMessage is the sole direct-delivery
// readiness authority for both API and tmux sessions.
func prepareRecipientDelivery(recipientSession string, adapter *dolt.Adapter) {
	harnessType := ""
	sessionsDir := ""
	if cfg != nil {
		sessionsDir = cfg.SessionsDir
	}
	if m, _, resolveErr := session.ResolveIdentifier(recipientSession, sessionsDir, adapter); resolveErr == nil {
		harnessType = m.Harness
		if isAutonomousRole(m.Context.Tags) {
			tmux.SetAutonomousMode(true)
		}
		if isAPIBasedAgent(harnessType) {
			checkAndWakeMonitors(recipientSession, adapter)
			return
		}
	}

	// Record the recipient harness so the non-blocking human_typing stash in the
	// tmux send path picks the right stash key (ce-subs).
	tmux.SetStashHarness(harnessType)

	guardOpts := safety.GuardOptions{
		SkipUninitialized: true,
		SkipMidResponse:   true,
		AutonomousMode:    tmux.AutonomousMode(),
		Harness:           harnessType,
	}
	if msgForce {
		tmux.SetForceDelivery(true)
	}

	guardResult := safety.Check(recipientSession, guardOpts)
	for _, adv := range guardResult.Advisories {
		if adv.Guard == safety.ViolationHumanTyping {
			telemetry.RecordHumanTypingDetected(context.Background(), harnessType)
		}
	}
	checkAndWakeMonitors(recipientSession, adapter)
}

// isAutonomousRole reports whether the session tags include a non-human role.
// Autonomous roles (worker/orchestrator/overseer/meta-orchestrator) never have a
// human at the keyboard, so the human_typing guard is noise for sends to them.
func isAutonomousRole(tags []string) bool {
	for _, t := range tags {
		switch t {
		case "role:worker", "role:orchestrator", "role:overseer", "role:meta-orchestrator":
			return true
		}
	}
	return false
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

// resolveRecipientState resolves the recipient's display state and delivery
// surface for persistence, returning (currentState, tmuxName, harnessType).
func resolveRecipientState(recipientSession string, adapter *dolt.Adapter) (string, string, string) {
	return resolveRecipientStateWithDependencies(recipientSession, adapter, session.ResolveSessionState, session.UpdateSessionState)
}

func resolveRecipientStateWithDependencies(
	recipientSession string,
	adapter *dolt.Adapter,
	resolveState func(string, string, string, time.Time) string,
	updateState func(string, string, string, string, *dolt.Adapter) error,
) (string, string, string) {
	var currentState string
	tmuxName := recipientSession
	m, manifestPath, resolveErr := session.ResolveIdentifier(recipientSession, cfg.SessionsDir, adapter)
	if resolveErr != nil {
		return currentState, tmuxName, ""
	}
	if m.Tmux.SessionName != "" {
		tmuxName = m.Tmux.SessionName
	}
	harnessType := m.Harness
	if harnessType == "" {
		harnessType = "claude-code"
	}
	harnessType = agent.NormalizeHarnessName(harnessType)
	// Pure API sessions intentionally have no tmux state. Their adapter status
	// is checked inside direct delivery, so tmux-only state resolution must not
	// persist OFFLINE after a successful provider send.
	if isAPIBasedAgent(harnessType) {
		return m.State, tmuxName, harnessType
	}
	currentState = resolveState(tmuxName, m.State, m.Claude.UUID, m.StateUpdatedAt)
	if currentState != m.State {
		if err := updateState(manifestPath, currentState, "hybrid", m.SessionID, adapter); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to persist session state: %v\n", err)
		}
	}
	return currentState, tmuxName, harnessType
}

type cliInputDeliveryPolicy struct {
	Force      bool
	Autonomous bool
}

func currentCLIInputDeliveryPolicy() cliInputDeliveryPolicy {
	return cliInputDeliveryPolicy{
		Force:      msgForce,
		Autonomous: tmux.AutonomousMode(),
	}
}

// dispatchSend attempts the shared direct-delivery transaction first, then
// translates its typed outcome into CLI-owned queue or overlay policy.
func dispatchSend(ctx context.Context, recipientSession, tmuxName, senderName, messageID, formattedMessage, message, currentState string, adapter *dolt.Adapter) error {
	directDelivery := func() error {
		return sendDirectly(ctx, recipientSession, senderName, messageID, formattedMessage, sessionSendPromptFile, adapter)
	}
	queueDelivery := func(readiness string) error {
		queueState := currentState
		if queueState == "" {
			queueState = readiness
		}
		if readiness == "PERMISSION" || readiness == "NO" {
			fmt.Fprintf(os.Stderr, "⚠ Session '%s' has an active permission prompt — message queued for delivery after resolution\n", recipientSession)
		}
		return queueMessage(ctx, recipientSession, senderName, messageID, formattedMessage, queueState)
	}
	overlayRecovery := func() error {
		fmt.Fprintf(os.Stderr, "⚠ Session '%s' has an active dismissible overlay — attempting auto-recovery\n", recipientSession)
		return dismissOverlayAndDeliver(tmuxName, recipientSession, directDelivery, queueDelivery)
	}
	if err := dispatchSendByOperationOutcome(directDelivery, queueDelivery, overlayRecovery); err != nil {
		return err
	}
	recordDelegation(senderName, recipientSession, messageID, message)
	return nil
}

func dispatchSendByOperationOutcome(directDelivery func() error, queueDelivery func(string) error, overlayRecovery func() error) error {
	err := directDelivery()
	if err == nil {
		return nil
	}
	readiness, ok := operationReadiness(err)
	if !ok {
		return err
	}
	switch readiness {
	case "QUEUE", "QUEUED_AGM", "PERMISSION", "ONBOARDING", "NO", "UNKNOWN":
		return queueDelivery(readiness)
	case "OVERLAY":
		return overlayRecovery()
	default:
		return err
	}
}

func operationReadiness(err error) (string, bool) {
	var opErr *ops.OpError
	if !errors.As(err, &opErr) || opErr.Code != ops.ErrCodeSessionNotReady {
		return "", false
	}
	readiness := opErr.Parameters["readiness"]
	if readiness == "" {
		readiness = "UNKNOWN"
	}
	return readiness, true
}

// queueMessage queues a message for later delivery (non-disruptive default)
func queueMessage(ctx context.Context, recipientSession, senderName, messageID, formattedMessage, currentState string) error {
	// Create message queue
	queue, err := messages.NewMessageQueue()
	if err != nil {
		// CLI fallback re-enters shared atomic exact-pane readiness.
		fmt.Fprintf(os.Stderr, "Warning: failed to create message queue: %v\n", err)
		fallbackAdapter, _ := getStorage()
		return sendDirectly(ctx, recipientSession, senderName, messageID, formattedMessage, "", fallbackAdapter)
	}
	defer func() { _ = queue.Close() }()

	// Check if daemon is running before queueing
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}
	pidFile := filepath.Join(homeDir, ".agm", "daemon.pid")
	daemonRunning := daemon.IsRunning(pidFile)

	// Only CLI delivery reaches this queue path; daemon-absent fallback re-enters
	// shared atomic readiness before sending any replacement input.
	if !daemonRunning {
		fmt.Fprintf(os.Stderr, "⚠ Daemon not running — falling back to direct tmux delivery for '%s'\n", recipientSession)
		fallbackAdapter, _ := getStorage()
		if fallbackAdapter != nil {
			defer func() { _ = fallbackAdapter.Close() }()
		}
		return sendDirectly(ctx, recipientSession, senderName, messageID, formattedMessage, "", fallbackAdapter)
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

// sendDirectly sends a message directly to a registered session without
// queuing. CLI harnesses use the shared operations layer so their readiness
// proof and exact-pane delivery cannot drift from MCP or Skills behavior.
func sendDirectly(ctx context.Context, recipientSession, senderName, messageID, formattedMessage, promptFile string, adapter *dolt.Adapter) error {
	return sendDirectlyWithTmux(ctx, recipientSession, senderName, messageID, formattedMessage, promptFile, adapter, session.NewRealTmux())
}

func sendDirectlyWithTmux(ctx context.Context, recipientSession, senderName, messageID, formattedMessage, promptFile string, adapter *dolt.Adapter, tmuxClient session.TmuxInterface) error {
	return sendDirectlyWithDependencies(ctx, recipientSession, senderName, messageID, formattedMessage, promptFile, adapter, tmuxClient, newAPIHarnessAdapter)
}

type apiAgentFactory = ops.APISessionAgentFactory

func sendDirectlyWithDependencies(ctx context.Context, recipientSession, senderName, messageID, formattedMessage, promptFile string, adapter *dolt.Adapter, tmuxClient session.TmuxInterface, newAPIAgent apiAgentFactory) error {
	if adapter == nil {
		return fmt.Errorf("verified delivery requires session storage")
	}
	policy := currentCLIInputDeliveryPolicy()
	result, err := sendViaSharedOperationsWithFactory(ctx, recipientSession, senderName, messageID, formattedMessage, promptFile, policy.Force, policy.Autonomous, adapter, tmuxClient, newAPIAgent)
	if err != nil {
		return err
	}

	// AGY metadata association is caller-specific post-delivery work. Reload by
	// the stable ID returned by the operation; this lookup does not participate
	// in transport selection or readiness.
	m, loadErr := adapter.GetSession(result.SessionID)
	if loadErr == nil && m != nil && agent.NormalizeHarnessName(m.Harness) == "agy" && (m.Agy == nil || m.Agy.ConversationID == "") {
		agyTmuxName := m.Tmux.SessionName
		if agyTmuxName == "" {
			agyTmuxName = result.Recipient
		}
		if err := waitForAgyMetadataBackfill(ctx, agyTmuxName, tmux.WaitForAgyPromptAfterInput); err != nil {
			return err
		}
		if err := associateSpawnedAgySessionWithRetry(ctx, m.Name, 20, 500*time.Millisecond); err != nil {
			return err
		}
	}
	return nil
}

func newAPIHarnessAdapter(ctx context.Context, m *manifest.Manifest) (agent.Agent, error) {
	return ops.NewAPISessionAgent(ctx, m)
}

func waitForAgyMetadataBackfill(ctx context.Context, sessionName string, wait func(context.Context, string, time.Duration) error) error {
	if err := wait(ctx, sessionName, 60*time.Second); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		fmt.Fprintf(os.Stderr, "Warning: AGY metadata backfill wait failed: %v\n", err)
	}
	return nil
}

// sendViaSharedOperations routes CLI delivery through ops.SendMessage. The
// supplied tmux capability must atomically prove harness ownership and send to
// the exact verified pane; weaker transports fail closed inside shared ops.
func sendViaSharedOperations(ctx context.Context, recipientSession, senderName, messageID, formattedMessage, promptFile string, force, autonomous bool, storage dolt.Storage, tmuxClient session.TmuxInterface) error {
	_, err := sendViaSharedOperationsWithFactory(ctx, recipientSession, senderName, messageID, formattedMessage, promptFile, force, autonomous, storage, tmuxClient, newAPIHarnessAdapter)
	return err
}

func sendViaSharedOperationsWithFactory(ctx context.Context, recipientSession, senderName, messageID, formattedMessage, promptFile string, force, autonomous bool, storage dolt.Storage, tmuxClient session.TmuxInterface, newAPIAgent apiAgentFactory) (*ops.SendMessageResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if storage == nil {
		return nil, fmt.Errorf("verified CLI delivery requires session storage")
	}

	result, err := ops.SendMessage(&ops.OpContext{
		Context:         ctx,
		Storage:         storage,
		Tmux:            tmuxClient,
		APIAgentFactory: newAPIAgent,
	}, &ops.SendMessageRequest{
		Recipient:  recipientSession,
		Message:    formattedMessage,
		Force:      force,
		Autonomous: autonomous,
	})
	if err != nil {
		return result, fmt.Errorf("shared CLI send: %w", err)
	}
	if result == nil || !result.Delivered {
		return result, fmt.Errorf("shared CLI send did not deliver to %q", recipientSession)
	}

	// Preserve the existing hook-visible audit artifact only after verified
	// delivery. A failed readiness check must not create an alternate path that
	// can inject the message later.
	if err := messages.WritePendingFile(result.Recipient, messageID, formattedMessage); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to write pending file: %v\n", err)
	}

	via := "tmux"
	if current, getErr := storage.GetSession(result.SessionID); getErr == nil && current != nil && isAPIBasedAgent(current.Harness) {
		via = current.Harness + " API"
	}
	successMsg := fmt.Sprintf("✓ Sent to '%s' from '%s' (%d chars) [ID: %s] [via: %s]", result.Recipient, senderName, len(formattedMessage), messageID, via)
	if promptFile != "" {
		successMsg += fmt.Sprintf(" [file: %s]", promptFile)
	}
	ui.PrintSuccess(successMsg)

	return result, nil
}

// isAPIBasedAgent returns true if the harness type uses API-based communication
// as opposed to tmux-based CLI communication.
func isAPIBasedAgent(harnessType string) bool {
	switch harnessType {
	case "openai", "gpt":
		return true
	case "claude-code", "gemini-cli", "codex-cli", "opencode-cli", "agy", "pi-cli":
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
func runSendMulti(ctx context.Context, spec *send.RecipientSpec) (retErr error) {
	defer func() {
		logCommandAudit("send.msg.multi", "", multiAuditArgs(spec), retErr)
	}()

	adapter, err := getStorage()
	if err != nil {
		return fmt.Errorf("failed to connect to Dolt storage: %w", err)
	}
	defer func() { _ = adapter.Close() }()

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

	tmuxClient := session.NewRealTmux()
	results := deliverMultiRecipientJobs(ctx, jobs, agent.OpenAIDeliveryTimeout, func(ctx context.Context, job *send.DeliveryJob) error {
		return deliveryFuncWithDependencies(ctx, job, adapter, tmuxClient)
	})

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

// deliverMultiRecipientJobs gives every sequential recipient its own bounded
// transaction. A batch-wide deadline would let one valid slow API completion
// consume the budget of every later recipient.
func deliverMultiRecipientJobs(ctx context.Context, jobs []*send.DeliveryJob, timeout time.Duration, deliver send.DeliveryFunc) []*send.DeliveryResult {
	return send.SequentialDeliver(ctx, jobs, func(ctx context.Context, job *send.DeliveryJob) error {
		deliveryCtx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		return deliver(deliveryCtx, job)
	})
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
func deliveryFunc(ctx context.Context, job *send.DeliveryJob) error {
	adapter, err := getStorage()
	if err != nil {
		return fmt.Errorf("open storage for verified delivery: %w", err)
	}
	defer func() { _ = adapter.Close() }()

	return deliveryFuncWithDependencies(ctx, job, adapter, session.NewRealTmux())
}

func deliveryFuncWithDependencies(ctx context.Context, job *send.DeliveryJob, adapter *dolt.Adapter, tmuxClient session.TmuxInterface) error {
	return deliveryFuncWithAgentFactory(ctx, job, adapter, tmuxClient, newAPIHarnessAdapter)
}

func deliveryFuncWithAgentFactory(ctx context.Context, job *send.DeliveryJob, adapter *dolt.Adapter, tmuxClient session.TmuxInterface, newAPIAgent apiAgentFactory) error {
	// Multi-recipient sends intentionally use the same manifest resolution and
	// final readiness boundary as single-recipient sends.
	return sendDirectlyWithDependencies(ctx, job.Recipient, job.Sender, job.MessageID, job.FormattedMessage, job.PromptFile, adapter, tmuxClient, newAPIAgent)
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

// dismissOverlayAndDeliver preserves CLI-owned overlay recovery while every
// readiness retry re-enters the shared direct-delivery transaction.
func dismissOverlayAndDeliver(tmuxName, recipientSession string, directDelivery func() error, queueDelivery func(string) error) error {
	if paneContent, err := tmux.CapturePaneOutput(tmuxName, 30); err == nil {
		if dismissed, dismissErr := tmux.DismissAgySurveyIfPresent(tmuxName, paneContent); dismissErr != nil {
			return dismissErr
		} else if dismissed {
			time.Sleep(200 * time.Millisecond)
			deliveryErr := directDelivery()
			if deliveryErr == nil {
				fmt.Fprintf(os.Stderr, "✓ AGY feedback survey skipped on '%s' — delivering message\n", recipientSession)
				return nil
			}
			if readiness, ok := operationReadiness(deliveryErr); !ok || readiness != "OVERLAY" {
				return queueAfterOverlay(deliveryErr, queueDelivery)
			}
		}
	}

	return retryOverlayDelivery(
		recipientSession,
		func() error {
			if err := tmux.SendKeys(tmuxName, "Left"); err != nil {
				return fmt.Errorf("failed to send Left key to dismiss overlay: %w", err)
			}
			return nil
		},
		func() error {
			if err := tmux.SendKeys(tmuxName, "Escape"); err != nil {
				return fmt.Errorf("failed to send Escape key to dismiss overlay: %w", err)
			}
			return nil
		},
		func() { time.Sleep(200 * time.Millisecond) },
		directDelivery,
		queueDelivery,
	)
}

func retryOverlayDelivery(recipientSession string, dismissLeft, dismissEscape func() error, pause func(), directDelivery func() error, queueDelivery func(string) error) error {
	if err := dismissLeft(); err != nil {
		return err
	}
	pause()

	deliveryErr := directDelivery()
	if deliveryErr == nil {
		fmt.Fprintf(os.Stderr, "✓ Overlay dismissed on '%s' — delivering message\n", recipientSession)
		return nil
	}
	if readiness, ok := operationReadiness(deliveryErr); !ok || readiness != "OVERLAY" {
		return queueAfterOverlay(deliveryErr, queueDelivery)
	}

	fmt.Fprintln(os.Stderr, "⚠ Overlay still active, trying Escape key...")
	if err := dismissEscape(); err != nil {
		return err
	}
	pause()
	deliveryErr = directDelivery()
	if deliveryErr == nil {
		fmt.Fprintf(os.Stderr, "✓ Overlay dismissed with Escape on '%s' — delivering message\n", recipientSession)
		return nil
	}
	return queueAfterOverlay(deliveryErr, queueDelivery)
}

func queueAfterOverlay(err error, queueDelivery func(string) error) error {
	readiness, ok := operationReadiness(err)
	if !ok {
		return err
	}
	switch readiness {
	case "OVERLAY", "QUEUE", "QUEUED_AGM", "PERMISSION", "ONBOARDING", "NO", "UNKNOWN":
		return queueDelivery(readiness)
	default:
		return err
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
