package command

import "context"

// GeminiClient defines the required interface for Gemini API client.
//
// This interface enables dependency injection and testing with mocks.
// Real implementation will be provided by gemini package (future work).
//
// Implementations must be safe for concurrent use by multiple goroutines.
type GeminiClient interface {
	// UpdateConversationTitle updates the title/name of a conversation.
	//
	// Parameters:
	//   - ctx: Context for timeout and cancellation
	//   - conversationID: Gemini conversation identifier
	//   - title: New conversation title
	//
	// Returns:
	//   - nil on success
	//   - error if API call fails (network, auth, rate limit, invalid ID, etc.)
	UpdateConversationTitle(ctx context.Context, conversationID string, title string) error

	// UpdateConversationMetadata updates custom metadata for a conversation.
	//
	// Parameters:
	//   - ctx: Context for timeout and cancellation
	//   - conversationID: Gemini conversation identifier
	//   - metadata: Key-value pairs to set (e.g., {"workingDirectory": "/path"})
	//
	// Returns:
	//   - nil on success
	//   - error if API call fails or metadata not supported
	UpdateConversationMetadata(ctx context.Context, conversationID string, metadata map[string]string) error
}
