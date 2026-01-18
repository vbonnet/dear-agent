package gpt

import (
	"time"

	"github.com/vbonnet/ai-tools/claude-session-manager/internal/agent"
)

// Session represents a GPT conversation session.
// Sessions are stored in-memory and contain conversation history and metadata.
type Session struct {
	// ID is the unique session identifier (UUID).
	ID agent.SessionID

	// Context contains session metadata (name, working directory, etc).
	Context agent.SessionContext

	// Messages is the conversation history (user + assistant messages).
	Messages []agent.Message

	// Status is the session status (active, terminated).
	Status agent.Status

	// CreatedAt is when the session was created.
	CreatedAt time.Time

	// UpdatedAt is when the session was last modified.
	UpdatedAt time.Time
}
