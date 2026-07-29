package ops

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/vbonnet/dear-agent/agm/internal/agent"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
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
	Operation       string `json:"operation"`
	Recipient       string `json:"recipient"`
	SessionID       string `json:"session_id"`
	MessageLength   int    `json:"message_length"`
	Delivered       bool   `json:"delivered"`
	ResponsePending bool   `json:"response_pending"`
}

// SendMessage resolves one stable recipient and performs direct delivery.
// Pure API sessions use their provider transaction. Tmux-backed sessions
// reload lifecycle and delivery identity under the stable-session lock before
// using the injected tmux runtime's atomic readiness-and-exact-pane capability.
func SendMessage(ctx *OpContext, req *SendMessageRequest) (*SendMessageResult, error) {
	if req == nil || req.Recipient == "" {
		return nil, ErrInvalidInput("recipient", "Recipient session identifier is required.")
	}
	if req.Message == "" {
		return nil, ErrInvalidInput("message", "Message text is required.")
	}
	if ctx == nil || ctx.Storage == nil {
		return nil, ErrStorageError("send_message storage", errors.New("session storage is required"))
	}

	// Validate that the recipient session exists
	m, err := ctx.Storage.GetSession(req.Recipient)
	if err != nil {
		m, err = findActiveByName(ctx, req.Recipient)
		if err != nil {
			return nil, err
		}
	}
	if m == nil {
		return nil, ErrSessionNotFound(req.Recipient)
	}
	if m.SessionID == "" {
		return nil, ErrStorageError("send_message", errors.New("resolved session has no stable session ID"))
	}

	callCtx := requestContext(ctx)
	if err := callCtx.Err(); err != nil {
		return newSendMessageResult(m, req, false), ErrStorageError("send_message context", err)
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
		current, err := DeliverAPISessionMessage(callCtx, ctx.Storage, m, message, ctx.APIDeliveryFactory)
		if err != nil {
			return newSendMessageResult(m, req, false), err
		}
		return newSendMessageResult(current, req, true), nil
	}

	var result *SendMessageResult
	err = WithSessionLockContext(callCtx, m.SessionID, func() error {
		current, reloadErr := ctx.Storage.GetSession(m.SessionID)
		if reloadErr != nil {
			return ErrStorageError("send_message_reload", reloadErr)
		}
		if current == nil {
			return ErrSessionNotFound(m.SessionID)
		}
		if err := requireActiveDeliverySession(current, m.Name); err != nil {
			result = newSendMessageResult(current, req, false)
			return err
		}
		if isAPISessionManifest(current) {
			result = newSendMessageResult(current, req, false)
			return ErrSessionNotReady(current.Name, "DELIVERY_SURFACE_CHANGED")
		}
		result, reloadErr = sendResolvedMessage(callCtx, ctx, current, req)
		return reloadErr
	})
	return result, err
}

func newSendMessageResult(m *manifest.Manifest, req *SendMessageRequest, delivered bool) *SendMessageResult {
	result := &SendMessageResult{
		Operation: "send_message",
		Delivered: delivered,
	}
	if m != nil {
		result.Recipient = m.Name
		result.SessionID = m.SessionID
	}
	if req != nil {
		result.MessageLength = len(req.Message)
	}
	return result
}

func requireActiveDeliverySession(current *manifest.Manifest, fallbackName string) error {
	if current.Lifecycle == "" {
		return nil
	}
	currentName := current.Name
	if currentName == "" {
		currentName = fallbackName
	}
	if current.Lifecycle == manifest.LifecycleArchived {
		return ErrSessionArchived(currentName)
	}
	return ErrSessionNotReady(currentName, "LIFECYCLE_"+current.Lifecycle)
}

// sendResolvedMessage delivers to one current non-API manifest while its
// stable-session lifecycle lock is held.
func sendResolvedMessage(callCtx context.Context, opCtx *OpContext, m *manifest.Manifest, req *SendMessageRequest) (*SendMessageResult, error) {
	tmuxName := m.Tmux.SessionName
	if tmuxName == "" {
		tmuxName = m.Name
	}
	harness := m.Harness
	if harness == "" {
		harness = "claude-code"
	}
	harness = agent.NormalizeHarnessName(harness)

	// Tmux readiness and delivery are one atomic capability. The implementation
	// holds the same mutation lock across composer observation and exact-pane
	// input so a concurrent AGM sender cannot invalidate the readiness proof.
	if opCtx.Tmux != nil {
		return sendResolvedTmuxMessage(callCtx, opCtx, m, req, tmuxName, harness)
	}

	// No local runtime was configured. Best-effort callers such as stall
	// recovery surface non-delivery through the result rather than failing the
	// whole operation.
	return newSendMessageResult(m, req, false), nil
}

func sendResolvedTmuxMessage(callCtx context.Context, opCtx *OpContext, m *manifest.Manifest, req *SendMessageRequest, tmuxName, harness string) (*SendMessageResult, error) {
	newResult := func(delivered bool) *SendMessageResult {
		return newSendMessageResult(m, req, delivered)
	}
	sender, ok := opCtx.Tmux.(session.AtomicInputSender)
	if !ok {
		return newResult(false), ErrSessionNotReady(m.Name, "ATOMIC_DELIVERY_UNAVAILABLE")
	}
	allowQueuedAGM := req.Force || req.Autonomous
	readiness, err := sender.SendKeysIfInputReady(callCtx, tmuxName, harness, req.Message, session.InputDeliveryOptions{
		AllowQueuedAGM: allowQueuedAGM,
	})
	if err != nil {
		return newResult(false), ErrStorageError("tmux.SendKeysIfInputReady", err)
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
	result := newResult(true)
	result.ResponsePending = true
	return result, nil
}
