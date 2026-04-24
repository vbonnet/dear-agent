package command

import (
	"context"
	"fmt"
)

// MockGeminiClient is a test double for GeminiClient.
//
// Allows tests to control behavior and simulate errors without real API calls.
// Default behavior (nil function fields) returns success.
//
// Example:
//
//	// Success case (default)
//	mock := &MockGeminiClient{}
//	translator := NewGeminiTranslator(mock)
//	err := translator.RenameSession(ctx, "id", "name")  // Returns nil
//
//	// Error case (custom behavior)
//	mock := &MockGeminiClient{
//	    UpdateTitleFunc: func(...) error {
//	        return errors.New("api error")
//	    },
//	}
//	err := translator.RenameSession(ctx, "id", "name")  // Returns ErrAPIFailure
type MockGeminiClient struct {
	// UpdateTitleFunc is called by UpdateConversationTitle.
	// If nil, default behavior returns nil (success).
	UpdateTitleFunc func(ctx context.Context, id string, title string) error

	// UpdateMetadataFunc is called by UpdateConversationMetadata.
	// If nil, default behavior returns nil (success).
	UpdateMetadataFunc func(ctx context.Context, id string, metadata map[string]string) error

	// RunHookFunc is called by RunHook.
	// If nil, default behavior returns ErrNotSupported (hooks not available for API clients).
	RunHookFunc func(ctx context.Context, sessionID string, hookName string) error

	// CallLog tracks calls made to client methods (for test verification).
	CallLog []string
}

// UpdateConversationTitle implements GeminiClient.UpdateConversationTitle.
func (m *MockGeminiClient) UpdateConversationTitle(ctx context.Context, id string, title string) error {
	m.CallLog = append(m.CallLog, fmt.Sprintf("UpdateTitle(%s, %s)", id, title))
	if m.UpdateTitleFunc != nil {
		return m.UpdateTitleFunc(ctx, id, title)
	}
	return nil // Default: success
}

// UpdateConversationMetadata implements GeminiClient.UpdateConversationMetadata.
func (m *MockGeminiClient) UpdateConversationMetadata(ctx context.Context, id string, metadata map[string]string) error {
	m.CallLog = append(m.CallLog, fmt.Sprintf("UpdateMetadata(%s, %v)", id, metadata))
	if m.UpdateMetadataFunc != nil {
		return m.UpdateMetadataFunc(ctx, id, metadata)
	}
	return nil // Default: success
}

// RunHook implements GeminiClient.RunHook.
func (m *MockGeminiClient) RunHook(ctx context.Context, sessionID string, hookName string) error {
	m.CallLog = append(m.CallLog, fmt.Sprintf("RunHook(%s, %s)", sessionID, hookName))
	if m.RunHookFunc != nil {
		return m.RunHookFunc(ctx, sessionID, hookName)
	}
	return ErrNotSupported // Default: hooks not supported for API clients
}
