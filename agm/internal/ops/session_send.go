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

	// Use manager backend for delivery if available
	if ctx.Manager != nil {
		tmuxName := m.Tmux.SessionName
		if tmuxName == "" {
			tmuxName = m.Name
		}
		result, sendErr := ctx.Manager.SendMessage(context.Background(), manager.SessionID(tmuxName), req.Message)
		delivered := sendErr == nil && result.Delivered
		return &SendMessageResult{
			Operation:     "send_message",
			Recipient:     m.Name,
			MessageLength: len(req.Message),
			Delivered:     delivered,
		}, sendErr
	}

	// Legacy stub: no backend available for actual delivery
	return &SendMessageResult{
		Operation:     "send_message",
		Recipient:     m.Name,
		MessageLength: len(req.Message),
		Delivered:     false,
	}, nil
}
