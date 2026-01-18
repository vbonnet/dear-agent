package claude

import (
	"context"

	"github.com/vbonnet/ai-tools/claude-session-manager/internal/agent"
)

// ClaudeAdapter is a stub implementation of the Agent interface for Claude.
// All methods currently return agent.ErrNotImplemented.
type ClaudeAdapter struct{}

// Name returns the agent identifier.
func (a *ClaudeAdapter) Name() string {
	return "claude"
}

// SendMessage is not yet implemented.
func (a *ClaudeAdapter) SendMessage(ctx context.Context, msg string) (string, error) {
	return "", agent.ErrNotImplemented
}

// StreamMessage is not yet implemented.
func (a *ClaudeAdapter) StreamMessage(ctx context.Context, msg string) (<-chan agent.StreamChunk, error) {
	return nil, agent.ErrNotImplemented
}

func init() {
	agent.Register("claude", &ClaudeAdapter{})
}
