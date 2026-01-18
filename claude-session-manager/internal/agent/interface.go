package agent

import "context"

// Agent represents a unified interface for AI agent interactions.
// Implementations must provide message sending capabilities with both
// blocking and streaming modes.
type Agent interface {
	// Name returns the agent identifier (e.g., "claude", "gemini", "gpt").
	Name() string

	// SendMessage sends a message and waits for the complete response.
	// Returns the full response text or an error if the request fails.
	SendMessage(ctx context.Context, msg string) (string, error)

	// StreamMessage sends a message and returns a channel for streaming responses.
	// The channel emits StreamChunk items until Done is true or an error occurs.
	// The channel is closed when streaming completes or fails.
	StreamMessage(ctx context.Context, msg string) (<-chan StreamChunk, error)
}

// StreamChunk represents a piece of a streamed response.
type StreamChunk struct {
	Content string // Incremental content received
	Done    bool   // True if this is the final chunk
	Error   error  // Error that occurred during streaming (if any)
}
