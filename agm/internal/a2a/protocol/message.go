// Package protocol provides protocol-related functionality.
package protocol

import (
	"fmt"
	"strings"
	"time"
)

// Message represents a single A2A protocol message
type Message struct {
	// AgentID is the unique identifier of the agent sending the message
	AgentID string

	// Timestamp is when the message was created
	Timestamp time.Time

	// Status is the current status of the message
	Status Status

	// MessageNumber is the sequential number within the channel
	MessageNumber int

	// Context describes what the agent is working on (2-3 sentences max)
	Context string

	// Proposal is the main content (proposal, answer, or analysis)
	Proposal string

	// Questions is a list of specific questions for the other agent
	Questions []string

	// Blockers lists what blocks the agent from proceeding
	Blockers []string

	// NextSteps lists proposed next steps
	NextSteps []string
}

// NewMessage creates a new message with default values
func NewMessage(agentID string, status Status) *Message {
	return &Message{
		AgentID:   agentID,
		Timestamp: time.Now(),
		Status:    status,
		Questions: make([]string, 0),
		Blockers:  make([]string, 0),
		NextSteps: make([]string, 0),
	}
}

// Validate checks if the message is valid
func (m *Message) Validate() error {
	if m.AgentID == "" {
		return fmt.Errorf("agent ID is required")
	}

	if !m.Status.IsValid() {
		return fmt.Errorf("invalid status: %s", m.Status)
	}

	if m.MessageNumber < 1 {
		return fmt.Errorf("message number must be >= 1")
	}

	if m.Context == "" {
		return fmt.Errorf("context is required")
	}

	if m.Proposal == "" {
		return fmt.Errorf("proposal/response is required")
	}

	return nil
}

// Format returns the message as formatted markdown
func (m *Message) Format() string {
	var sb strings.Builder

	// Header
	sb.WriteString("---\n")
	fmt.Fprintf(&sb, "**Agent ID**: %s\n", m.AgentID)
	fmt.Fprintf(&sb, "**Timestamp**: %s\n", m.Timestamp.Format("2006-01-02 15:04"))
	fmt.Fprintf(&sb, "**Status**: %s\n", m.Status)
	fmt.Fprintf(&sb, "**Message #**: %d\n", m.MessageNumber)
	sb.WriteString("---\n\n")

	// Context
	sb.WriteString("### Context (What I'm working on)\n\n")
	sb.WriteString(m.Context)
	sb.WriteString("\n\n")

	// Proposal/Response
	sb.WriteString("### Proposal/Response\n\n")
	sb.WriteString(m.Proposal)
	sb.WriteString("\n\n")

	// Questions
	sb.WriteString("### Questions for Other Agent\n\n")
	if len(m.Questions) == 0 {
		sb.WriteString("None - all questions answered.\n")
	} else {
		for i, q := range m.Questions {
			fmt.Fprintf(&sb, "%d. %s\n", i+1, q)
		}
	}
	sb.WriteString("\n")

	// Blockers
	sb.WriteString("### Blockers/Dependencies\n\n")
	if len(m.Blockers) == 0 {
		sb.WriteString("None - ready to proceed.\n")
	} else {
		for _, b := range m.Blockers {
			fmt.Fprintf(&sb, "- %s\n", b)
		}
	}
	sb.WriteString("\n")

	// Next Steps
	sb.WriteString("### Proposed Next Steps\n\n")
	if len(m.NextSteps) == 0 {
		sb.WriteString("1. Await response\n")
	} else {
		for i, s := range m.NextSteps {
			fmt.Fprintf(&sb, "%d. %s\n", i+1, s)
		}
	}
	sb.WriteString("\n")

	sb.WriteString("---\n\n")

	return sb.String()
}

// EstimateTokens returns an approximate token count (chars / 4)
func (m *Message) EstimateTokens() int {
	content := m.Format()
	return len([]rune(content)) / 4
}
