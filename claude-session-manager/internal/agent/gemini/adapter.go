package gemini

import (
	"context"

	"github.com/vbonnet/ai-tools/claude-session-manager/internal/agent"
)

// GeminiAdapter is a stub implementation of the Agent interface for Gemini.
// All methods currently return agent.ErrNotImplemented.
type GeminiAdapter struct{}

// Name returns the agent identifier.
func (a *GeminiAdapter) Name() string {
	return "gemini"
}

// SendMessage is not yet implemented.
func (a *GeminiAdapter) SendMessage(ctx context.Context, msg string) (string, error) {
	return "", agent.ErrNotImplemented
}

// StreamMessage is not yet implemented.
func (a *GeminiAdapter) StreamMessage(ctx context.Context, msg string) (<-chan agent.StreamChunk, error) {
	return nil, agent.ErrNotImplemented
}

func init() {
	agent.Register("gemini", &GeminiAdapter{})
}
