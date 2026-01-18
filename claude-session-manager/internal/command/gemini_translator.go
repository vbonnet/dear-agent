package command

import (
	"context"
	"fmt"
)

// GeminiTranslator implements CommandTranslator for Google Gemini agent.
//
// Translates generic AGM commands to Gemini API calls using dependency injection.
// Safe for concurrent use by multiple goroutines (client is immutable reference).
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
		return fmt.Errorf("%w: %v", ErrAPIFailure, err)
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
		return fmt.Errorf("%w: %v", ErrAPIFailure, err)
	}
	return nil
}

// RunHook implements CommandTranslator.RunHook.
//
// Gemini has no terminal access and cannot execute hooks.
// Always returns ErrNotSupported.
func (t *GeminiTranslator) RunHook(ctx context.Context, sessionID string, hook string) error {
	// Gemini has no terminal access, cannot execute hooks
	return ErrNotSupported
}
