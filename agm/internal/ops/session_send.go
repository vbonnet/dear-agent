package ops

import (
	"time"

	"github.com/google/uuid"
	"github.com/vbonnet/dear-agent/agm/internal/agent"
	"github.com/vbonnet/dear-agent/agm/internal/manager"
	"github.com/vbonnet/dear-agent/agm/internal/session"
)

// SendMessageRequest defines the input for sending a message to a session.
type SendMessageRequest struct {
	// Recipient is a session ID, name, or UUID prefix.
	Recipient string `json:"recipient"`

	// Message is the text to send.
	Message string `json:"message"`

	// Force permits delivery only when the verified harness owns the exact pane
	// and a queued-input marker plus complete AGM header identify a stuck AGM
	// paste. It does not bypass human drafts or any other fail-closed state.
	Force bool `json:"force,omitempty"`

	// Autonomous permits the same narrowly scoped stuck-AGM recovery for an
	// unattended session. It never bypasses human drafts, generic busy states,
	// permission, overlays, onboarding, harness identity, target existence, or
	// backend-error checks.
	Autonomous bool `json:"autonomous,omitempty"`
}

// SendMessageResult is the output of SendMessage.
type SendMessageResult struct {
	Operation     string `json:"operation"`
	Recipient     string `json:"recipient"`
	MessageLength int    `json:"message_length"`
	Delivered     bool   `json:"delivered"`
}

// SendMessage sends a message to a session.
// When a manager.Backend is available on OpContext, it delivers the message
// through the backend abstraction. Otherwise falls back to the legacy stub.
func SendMessage(ctx *OpContext, req *SendMessageRequest) (*SendMessageResult, error) {
	if req == nil || req.Recipient == "" {
		return nil, ErrInvalidInput("recipient", "Recipient session identifier is required.")
	}
	if req.Message == "" {
		return nil, ErrInvalidInput("message", "Message text is required.")
	}

	// Validate that the recipient session exists
	m, err := ctx.Storage.GetSession(req.Recipient)
	if err != nil {
		m, err = findByName(ctx, req.Recipient)
		if err != nil {
			return nil, err
		}
	}
	if m == nil {
		return nil, ErrSessionNotFound(req.Recipient)
	}

	// Check if session is archived
	if m.Lifecycle == "archived" {
		return nil, ErrSessionArchived(m.Name)
	}

	// Resolve the tmux session name once; both delivery paths target it.
	tmuxName := m.Tmux.SessionName
	if tmuxName == "" {
		tmuxName = m.Name
	}
	harness := m.Harness
	if harness == "" {
		harness = "claude-code"
	}
	harness = agent.NormalizeHarnessName(harness)

	newResult := func(delivered bool) *SendMessageResult {
		return &SendMessageResult{
			Operation:     "send_message",
			Recipient:     m.Name,
			MessageLength: len(req.Message),
			Delivered:     delivered,
		}
	}

	callCtx := requestContext(ctx)
	if err := callCtx.Err(); err != nil {
		return newResult(false), ErrStorageError("send_message context", err)
	}

	// Pure API sessions have no pane. Route them through the shared stable-ID
	// lifecycle/readiness/provider transaction before considering any tmux
	// capability that a surface (notably MCP) may also have wired.
	if isAPISessionManifest(m) {
		message := agent.Message{
			ID:        uuid.NewString(),
			Role:      agent.RoleUser,
			Content:   req.Message,
			Timestamp: time.Now(),
			Metadata: map[string]any{
				"source": "ops_send_message",
			},
		}
		if _, err := DeliverAPISessionMessage(callCtx, ctx.Storage, m, message, ctx.APIAgentFactory); err != nil {
			return newResult(false), err
		}
		return newResult(true), nil
	}

	// Tmux readiness and delivery are one atomic capability. The implementation
	// holds the same mutation lock across composer observation and exact-pane
	// input so a concurrent AGM sender cannot invalidate the readiness proof.
	if ctx.Tmux != nil {
		sender, ok := ctx.Tmux.(session.AtomicInputSender)
		if !ok {
			return newResult(false), ErrSessionNotReady(m.Name, "ATOMIC_DELIVERY_UNAVAILABLE")
		}
		allowQueuedAGM := req.Force || req.Autonomous
		readiness, readinessErr := sender.SendKeysIfInputReady(callCtx, tmuxName, harness, req.Message, session.InputDeliveryOptions{
			AllowQueuedAGM: allowQueuedAGM,
		})
		if readinessErr != nil {
			return newResult(false), ErrStorageError("tmux.SendKeysIfInputReady", readinessErr)
		}
		if !readiness.Ready {
			return newResult(false), ErrSessionNotReady(m.Name, readiness.State)
		}
		if readiness.Forced && (!allowQueuedAGM || readiness.State != "YES") {
			return newResult(false), ErrSessionNotReady(m.Name, "INVALID_QUEUED_AGM_DELIVERY")
		}
		if readiness.PaneID == "" {
			return newResult(false), ErrSessionNotReady(m.Name, "UNVERIFIED_PANE")
		}
		return newResult(true), nil
	}

	// Preferred delivery path: manager.Backend. It may represent tmux or a
	// structured backend; do not repeat a weaker generic check after exact tmux
	// readiness has already succeeded.
	if ctx.Manager != nil {
		readiness, readinessErr := ctx.Manager.CheckDelivery(callCtx, manager.SessionID(tmuxName))
		if readinessErr != nil {
			return newResult(false), ErrStorageError("manager.CheckDelivery", readinessErr)
		}
		if readiness != manager.CanReceiveYes {
			return newResult(false), ErrSessionNotReady(m.Name, managerReadinessName(readiness))
		}
		if err := callCtx.Err(); err != nil {
			return newResult(false), ErrStorageError("send_message context", err)
		}
		result, sendErr := ctx.Manager.SendMessage(callCtx, manager.SessionID(tmuxName), req.Message)
		return newResult(sendErr == nil && result.Delivered), sendErr
	}

	// Legacy path: no manager backend was wired in. Deliver directly through
	// the tmux abstraction when one is available — this is the same underlying
	// send-keys mechanism the manager backend uses, so callers constructed with
	// only a Tmux client (rather than the newer Backend) still reach the
	// recipient instead of silently dropping the message. This closes the
	// long-standing "AGM message delivery is undeliverable" gap (ce-6as.36).
	if ctx.Tmux != nil {
		return newResult(false), ErrSessionNotReady(m.Name, "UNVERIFIED")
	}

	// No delivery mechanism configured at all (neither a manager Backend nor a
	// Tmux client). Report non-delivery without an error: best-effort callers
	// such as stall recovery rely on this to surface "could not send" via the
	// Delivered flag rather than failing the whole operation.
	return newResult(false), nil
}

func managerReadinessName(readiness manager.CanReceive) string {
	switch readiness {
	case manager.CanReceiveYes:
		return "YES"
	case manager.CanReceiveNo:
		return "NO"
	case manager.CanReceiveQueue:
		return "QUEUE"
	case manager.CanReceiveNotFound:
		return "NOT_FOUND"
	default:
		return "UNKNOWN"
	}
}
