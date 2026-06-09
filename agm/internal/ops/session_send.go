package ops

import (
	"context"

	"github.com/vbonnet/dear-agent/agm/internal/manager"
)

// SendMessageRequest defines the input for sending a message to a session.
type SendMessageRequest struct {
	// Recipient is a session ID, name, or UUID prefix.
	Recipient string `json:"recipient"`

	// Message is the text to send.
	Message string `json:"message"`
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

	newResult := func(delivered bool) *SendMessageResult {
		return &SendMessageResult{
			Operation:     "send_message",
			Recipient:     m.Name,
			MessageLength: len(req.Message),
			Delivered:     delivered,
		}
	}

	// Preferred path: deliver through the manager.Backend abstraction.
	if ctx.Manager != nil {
		result, sendErr := ctx.Manager.SendMessage(context.Background(), manager.SessionID(tmuxName), req.Message)
		return newResult(sendErr == nil && result.Delivered), sendErr
	}

	// Legacy path: no manager backend was wired in. Deliver directly through
	// the tmux abstraction when one is available — this is the same underlying
	// send-keys mechanism the manager backend uses, so callers constructed with
	// only a Tmux client (rather than the newer Backend) still reach the
	// recipient instead of silently dropping the message. This closes the
	// long-standing "AGM message delivery is undeliverable" gap (ce-6as.36).
	if ctx.Tmux != nil {
		if err := ctx.Tmux.SendKeys(tmuxName, req.Message); err != nil {
			return newResult(false), err
		}
		return newResult(true), nil
	}

	// No delivery mechanism configured at all (neither a manager Backend nor a
	// Tmux client). Report non-delivery without an error: best-effort callers
	// such as stall recovery rely on this to surface "could not send" via the
	// Delivered flag rather than failing the whole operation.
	return newResult(false), nil
}
