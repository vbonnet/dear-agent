// Package openai provides openai functionality.
package openai

import "context"

// ClientInterface defines the interface for OpenAI API operations.
// This allows for mocking in tests.
type ClientInterface interface {
	// CreateChatCompletion sends a chat completion request.
	CreateChatCompletion(ctx context.Context, messages []Message) (*ChatCompletionResponse, error)

	// Model returns the configured model name.
	Model() string

	// IsAzure returns true if this client is configured for Azure OpenAI.
	IsAzure() bool
}

// Ensure Client implements ClientInterface
var _ ClientInterface = (*Client)(nil)
