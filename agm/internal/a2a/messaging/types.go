// Package messaging provides structured agent-to-agent message types
// with routing support for direct, broadcast, and role-based delivery.
package messaging

import (
	"fmt"
	"time"

	"github.com/google/uuid"
)

// MessageType identifies the kind of A2A message.
type MessageType string

const (
	TypeRequest      MessageType = "request"      // Ask another agent to do something
	TypeResponse     MessageType = "response"     // Reply to a request
	TypeNotification MessageType = "notification" // Inform without expecting a reply
	TypeDelegation   MessageType = "delegation"   // Delegate a task to another agent
)

// RoutingMode determines how a message is delivered.
type RoutingMode string

const (
	RouteDirect    RoutingMode = "direct"    // To a specific agent by ID
	RouteBroadcast RoutingMode = "broadcast" // To all agents
	RouteRole      RoutingMode = "role"      // To all agents with a specific role
)

// Priority indicates message urgency.
type Priority string

const (
	PriorityLow    Priority = "low"
	PriorityNormal Priority = "normal"
	PriorityHigh   Priority = "high"
)

// Message is a structured A2A protocol message with routing metadata.
type Message struct {
	// Identity
	ID            string      `json:"id"`
	CorrelationID string      `json:"correlation_id,omitempty"` // Links request-response pairs
	Type          MessageType `json:"type"`

	// Routing
	Sender      string      `json:"sender"`               // Agent ID of sender
	Recipient   string      `json:"recipient,omitempty"`   // Agent ID for direct routing
	TargetRole  string      `json:"target_role,omitempty"` // Role for role-based routing
	RoutingMode RoutingMode `json:"routing_mode"`

	// Content
	Subject string `json:"subject"`          // Brief description
	Body    string `json:"body"`             // Main content
	Action  string `json:"action,omitempty"` // Requested action (for request/delegation)

	// Metadata
	Priority  Priority  `json:"priority"`
	Timestamp time.Time `json:"timestamp"`
	ExpiresAt time.Time `json:"expires_at,omitempty"` // Optional TTL
}

// NewRequest creates a direct request message to a specific agent.
func NewRequest(sender, recipient, subject, body, action string) *Message {
	id := uuid.New().String()
	return &Message{
		ID:            id,
		CorrelationID: id, // Requests start a new correlation chain
		Type:          TypeRequest,
		Sender:        sender,
		Recipient:     recipient,
		RoutingMode:   RouteDirect,
		Subject:       subject,
		Body:          body,
		Action:        action,
		Priority:      PriorityNormal,
		Timestamp:     time.Now(),
	}
}

// NewResponse creates a response to a previous message.
func NewResponse(sender string, inReplyTo *Message, body string) *Message {
	return &Message{
		ID:            uuid.New().String(),
		CorrelationID: inReplyTo.CorrelationID,
		Type:          TypeResponse,
		Sender:        sender,
		Recipient:     inReplyTo.Sender,
		RoutingMode:   RouteDirect,
		Subject:       "Re: " + inReplyTo.Subject,
		Body:          body,
		Priority:      inReplyTo.Priority,
		Timestamp:     time.Now(),
	}
}

// NewNotification creates a broadcast notification.
func NewNotification(sender, subject, body string) *Message {
	return &Message{
		ID:          uuid.New().String(),
		Type:        TypeNotification,
		Sender:      sender,
		RoutingMode: RouteBroadcast,
		Subject:     subject,
		Body:        body,
		Priority:    PriorityNormal,
		Timestamp:   time.Now(),
	}
}

// NewDelegation creates a task delegation message to a specific agent.
func NewDelegation(sender, recipient, subject, body, action string) *Message {
	id := uuid.New().String()
	return &Message{
		ID:            id,
		CorrelationID: id,
		Type:          TypeDelegation,
		Sender:        sender,
		Recipient:     recipient,
		RoutingMode:   RouteDirect,
		Subject:       subject,
		Body:          body,
		Action:        action,
		Priority:      PriorityNormal,
		Timestamp:     time.Now(),
	}
}

// NewRoleMessage creates a message targeted at all agents with a given role.
func NewRoleMessage(sender, targetRole string, msgType MessageType, subject, body string) *Message {
	return &Message{
		ID:          uuid.New().String(),
		Type:        msgType,
		Sender:      sender,
		TargetRole:  targetRole,
		RoutingMode: RouteRole,
		Subject:     subject,
		Body:        body,
		Priority:    PriorityNormal,
		Timestamp:   time.Now(),
	}
}

// Validate checks that the message has all required fields.
func (m *Message) Validate() error {
	if m.ID == "" {
		return fmt.Errorf("messaging: id is required")
	}
	if m.Sender == "" {
		return fmt.Errorf("messaging: sender is required")
	}
	if m.Subject == "" {
		return fmt.Errorf("messaging: subject is required")
	}
	if m.Body == "" {
		return fmt.Errorf("messaging: body is required")
	}

	switch m.Type {
	case TypeRequest, TypeResponse, TypeNotification, TypeDelegation:
		// valid
	default:
		return fmt.Errorf("messaging: invalid type %q", m.Type)
	}

	switch m.RoutingMode {
	case RouteDirect:
		if m.Recipient == "" {
			return fmt.Errorf("messaging: recipient required for direct routing")
		}
	case RouteRole:
		if m.TargetRole == "" {
			return fmt.Errorf("messaging: target_role required for role-based routing")
		}
	case RouteBroadcast:
		// no additional validation needed
	default:
		return fmt.Errorf("messaging: invalid routing_mode %q", m.RoutingMode)
	}

	if m.Type == TypeResponse && m.CorrelationID == "" {
		return fmt.Errorf("messaging: correlation_id required for responses")
	}

	return nil
}

// IsExpired returns true if the message has a TTL and it has elapsed.
func (m *Message) IsExpired() bool {
	if m.ExpiresAt.IsZero() {
		return false
	}
	return time.Now().After(m.ExpiresAt)
}
