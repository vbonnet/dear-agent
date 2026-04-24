// Package command provides command functionality.
package command

import (
	"context"
	"errors"
	"fmt"
)

// GeminiTranslator implements CommandTranslator for Google Gemini agent.
//
// Translates generic AGM commands to Gemini API calls using dependency injection.
// Safe for concurrent use by multiple goroutines (client is immutable reference).
//
// Note: This translator is for API-based Gemini clients. For CLI-based Gemini
// (which runs in tmux like Claude), use the GeminiCLIAdapter's ExecuteCommand directly.
//
// Example:
//
//	client := gemini.NewClient(apiKey)  // Future: real Gemini client
//	translator := command.NewGeminiTranslator(client)
//	err := translator.RenameSession(ctx, "conv-123", "new-name")
type GeminiTranslator struct {
	client GeminiClient // Injected dependency (interface, not concrete type)
}

// NewGeminiTranslator creates a new Gemini command translator.
//
// Parameters:
//   - client: GeminiClient implementation (can be real or mock)
//
// Returns:
//   - *GeminiTranslator ready to translate commands
//
// Example:
//
//	translator := command.NewGeminiTranslator(client)
func NewGeminiTranslator(client GeminiClient) *GeminiTranslator {
	return &GeminiTranslator{client: client}
}

// RenameSession implements CommandTranslator.RenameSession.
//
// Translates generic rename command to Gemini API call.
func (t *GeminiTranslator) RenameSession(ctx context.Context, sessionID string, newName string) error {
	if err := t.client.UpdateConversationTitle(ctx, sessionID, newName); err != nil {
		return fmt.Errorf("%w: %w", ErrAPIFailure, err)
	}
	return nil
}

// SetDirectory implements CommandTranslator.SetDirectory.
//
// Translates generic set directory command to Gemini metadata update.
func (t *GeminiTranslator) SetDirectory(ctx context.Context, sessionID string, path string) error {
	metadata := map[string]string{
		"workingDirectory": path,
	}
	if err := t.client.UpdateConversationMetadata(ctx, sessionID, metadata); err != nil {
		return fmt.Errorf("%w: %w", ErrAPIFailure, err)
	}
	return nil
}

// RunHook implements CommandTranslator.RunHook.
//
// Delegates hook execution to the underlying GeminiClient.
// For API-based clients, this will return ErrNotSupported.
// For CLI-based clients (via wrapper), this will execute hooks via subprocess.
func (t *GeminiTranslator) RunHook(ctx context.Context, sessionID string, hook string) error {
	if err := t.client.RunHook(ctx, sessionID, hook); err != nil {
		// If the client doesn't support hooks, return ErrNotSupported
		// Otherwise wrap the error as an API failure
		if errors.Is(err, ErrNotSupported) {
			return ErrNotSupported
		}
		return fmt.Errorf("%w: %w", ErrAPIFailure, err)
	}
	return nil
}
