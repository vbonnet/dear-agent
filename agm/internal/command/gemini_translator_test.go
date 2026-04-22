package command

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

// TestGeminiTranslator_RenameSession tests all paths for RenameSession method
func TestGeminiTranslator_RenameSession(t *testing.T) {
	tests := []struct {
		name        string
		sessionID   string
		newName     string
		clientErr   error
		wantErr     error
		wantCallLog string
	}{
		{
			name:        "success",
			sessionID:   "conv-123",
			newName:     "new-name",
			clientErr:   nil,
			wantErr:     nil,
			wantCallLog: "UpdateTitle(conv-123, new-name)",
		},
		{
			name:        "client error",
			sessionID:   "conv-123",
			newName:     "new-name",
			clientErr:   errors.New("api error"),
			wantErr:     ErrAPIFailure,
			wantCallLog: "UpdateTitle(conv-123, new-name)",
		},
		{
			name:        "empty name",
			sessionID:   "conv-123",
			newName:     "",
			clientErr:   nil,
			wantErr:     nil,
			wantCallLog: "UpdateTitle(conv-123, )",
		},
		{
			name:        "special characters",
			sessionID:   "conv-456",
			newName:     "my-new-name_123",
			clientErr:   nil,
			wantErr:     nil,
			wantCallLog: "UpdateTitle(conv-456, my-new-name_123)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &MockGeminiClient{
				UpdateTitleFunc: func(ctx context.Context, id, title string) error {
					return tt.clientErr
				},
			}
			translator := NewGeminiTranslator(mock)

			ctx := context.Background()
			err := translator.RenameSession(ctx, tt.sessionID, tt.newName)

			// Check error
			if tt.wantErr == nil {
				if err != nil {
					t.Errorf("expected no error, got %v", err)
				}
			} else {
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("expected error %v, got %v", tt.wantErr, err)
				}
			}

			// Verify client was called correctly
			if len(mock.CallLog) != 1 {
				t.Errorf("expected 1 call, got %d", len(mock.CallLog))
			} else if mock.CallLog[0] != tt.wantCallLog {
				t.Errorf("expected call %s, got %s", tt.wantCallLog, mock.CallLog[0])
			}
		})
	}
}

// TestGeminiTranslator_RenameSession_ContextCancel tests context cancellation
func TestGeminiTranslator_RenameSession_ContextCancel(t *testing.T) {
	mock := &MockGeminiClient{
		UpdateTitleFunc: func(ctx context.Context, id, title string) error {
			// Simulate waiting for context cancellation
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(100 * time.Millisecond):
				return nil
			}
		},
	}
	translator := NewGeminiTranslator(mock)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	err := translator.RenameSession(ctx, "conv-123", "new-name")
	if err == nil {
		t.Error("expected error, got nil")
	}
	// Error should be wrapped, but original context.Canceled should be detectable
	if !strings.Contains(err.Error(), "context canceled") {
		t.Errorf("expected context canceled error, got %v", err)
	}
}

// TestGeminiTranslator_RenameSession_ContextTimeout tests context timeout
func TestGeminiTranslator_RenameSession_ContextTimeout(t *testing.T) {
	mock := &MockGeminiClient{
		UpdateTitleFunc: func(ctx context.Context, id, title string) error {
			// Simulate slow API call
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(100 * time.Millisecond):
				return nil
			}
		},
	}
	translator := NewGeminiTranslator(mock)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	err := translator.RenameSession(ctx, "conv-123", "new-name")
	if err == nil {
		t.Error("expected timeout error, got nil")
	}
	// Error should contain deadline exceeded
	if !strings.Contains(err.Error(), "deadline exceeded") && !strings.Contains(err.Error(), "context deadline exceeded") {
		t.Errorf("expected deadline exceeded error, got %v", err)
	}
}

// TestGeminiTranslator_SetDirectory tests all paths for SetDirectory method
func TestGeminiTranslator_SetDirectory(t *testing.T) {
	tests := []struct {
		name      string
		sessionID string
		path      string
		clientErr error
		wantErr   error
	}{
		{
			name:      "success",
			sessionID: "conv-123",
			path:      "~/project",
			clientErr: nil,
			wantErr:   nil,
		},
		{
			name:      "client error",
			sessionID: "conv-123",
			path:      "~/project",
			clientErr: errors.New("metadata not supported"),
			wantErr:   ErrAPIFailure,
		},
		{
			name:      "empty path",
			sessionID: "conv-123",
			path:      "",
			clientErr: nil,
			wantErr:   nil,
		},
		{
			name:      "relative path",
			sessionID: "conv-456",
			path:      "./relative",
			clientErr: nil,
			wantErr:   nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &MockGeminiClient{
				UpdateMetadataFunc: func(ctx context.Context, id string, metadata map[string]string) error {
					// Verify metadata structure
					if val, ok := metadata["workingDirectory"]; !ok {
						t.Errorf("expected workingDirectory key in metadata")
					} else if val != tt.path {
						t.Errorf("expected path %s, got %s", tt.path, val)
					}
					return tt.clientErr
				},
			}
			translator := NewGeminiTranslator(mock)

			ctx := context.Background()
			err := translator.SetDirectory(ctx, tt.sessionID, tt.path)

			// Check error
			if tt.wantErr == nil {
				if err != nil {
					t.Errorf("expected no error, got %v", err)
				}
			} else {
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("expected error %v, got %v", tt.wantErr, err)
				}
			}

			// Verify client was called
			if len(mock.CallLog) != 1 {
				t.Errorf("expected 1 call, got %d", len(mock.CallLog))
			}
		})
	}
}

// TestGeminiTranslator_SetDirectory_ContextCancel tests context cancellation
func TestGeminiTranslator_SetDirectory_ContextCancel(t *testing.T) {
	mock := &MockGeminiClient{
		UpdateMetadataFunc: func(ctx context.Context, id string, metadata map[string]string) error {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(100 * time.Millisecond):
				return nil
			}
		},
	}
	translator := NewGeminiTranslator(mock)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	err := translator.SetDirectory(ctx, "conv-123", "/path")
	if err == nil {
		t.Error("expected error, got nil")
	}
}

// TestGeminiTranslator_RunHook tests RunHook delegates to client
func TestGeminiTranslator_RunHook(t *testing.T) {
	tests := []struct {
		name      string
		sessionID string
		hook      string
	}{
		{
			name:      "agm-assoc hook",
			sessionID: "conv-123",
			hook:      "/agm:agm-assoc",
		},
		{
			name:      "rename hook",
			sessionID: "conv-456",
			hook:      "/rename session-name",
		},
		{
			name:      "empty hook",
			sessionID: "conv-789",
			hook:      "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Default mock client returns ErrNotSupported for hooks (API clients)
			mock := &MockGeminiClient{}
			translator := NewGeminiTranslator(mock)

			ctx := context.Background()
			err := translator.RunHook(ctx, tt.sessionID, tt.hook)

			// Should return ErrNotSupported for API-based clients
			if !errors.Is(err, ErrNotSupported) {
				t.Errorf("expected ErrNotSupported, got %v", err)
			}

			// Should call client's RunHook method
			if len(mock.CallLog) != 1 {
				t.Errorf("expected 1 call to RunHook, got %d", len(mock.CallLog))
			}
		})
	}
}

// TestErrorWrapping tests error wrapping and unwrapping
func TestErrorWrapping(t *testing.T) {
	t.Run("ErrAPIFailure wrapping", func(t *testing.T) {
		originalErr := errors.New("network timeout")
		mock := &MockGeminiClient{
			UpdateTitleFunc: func(ctx context.Context, id, title string) error {
				return originalErr
			},
		}
		translator := NewGeminiTranslator(mock)

		err := translator.RenameSession(context.Background(), "conv-123", "new-name")

		// Should be detectable with errors.Is()
		if !errors.Is(err, ErrAPIFailure) {
			t.Errorf("expected errors.Is(err, ErrAPIFailure) to be true")
		}

		// Should contain original error message
		if !strings.Contains(err.Error(), "network timeout") {
			t.Errorf("expected error to contain original message, got %v", err)
		}

		// Should contain the original error via errors.Is (multi-wrap)
		if !errors.Is(err, originalErr) {
			t.Error("expected errors.Is(err, originalErr) to be true")
		}
	})

	t.Run("ErrNotSupported not wrapped", func(t *testing.T) {
		translator := NewGeminiTranslator(&MockGeminiClient{})
		err := translator.RunHook(context.Background(), "conv-123", "/hook")

		// Should be exact match
		if !errors.Is(err, ErrNotSupported) {
			t.Errorf("expected errors.Is(err, ErrNotSupported) to be true")
		}

		// Should not wrap another error
		unwrapped := errors.Unwrap(err)
		if unwrapped != nil {
			t.Errorf("expected unwrapped error to be nil, got %v", unwrapped)
		}
	})
}

// TestMockGeminiClient tests the mock client behavior
func TestMockGeminiClient(t *testing.T) {
	t.Run("default behavior (success)", func(t *testing.T) {
		mock := &MockGeminiClient{}

		err := mock.UpdateConversationTitle(context.Background(), "conv-123", "title")
		if err != nil {
			t.Errorf("expected nil error from default behavior, got %v", err)
		}

		err = mock.UpdateConversationMetadata(context.Background(), "conv-123", map[string]string{"key": "value"})
		if err != nil {
			t.Errorf("expected nil error from default behavior, got %v", err)
		}
	})

	t.Run("custom behavior (error)", func(t *testing.T) {
		customErr := errors.New("custom error")
		mock := &MockGeminiClient{
			UpdateTitleFunc: func(ctx context.Context, id, title string) error {
				return customErr
			},
		}

		err := mock.UpdateConversationTitle(context.Background(), "conv-123", "title")
		if !errors.Is(err, customErr) {
			t.Errorf("expected custom error, got %v", err)
		}
	})

	t.Run("call logging", func(t *testing.T) {
		mock := &MockGeminiClient{}

		_ = mock.UpdateConversationTitle(context.Background(), "conv-123", "title-1")
		_ = mock.UpdateConversationTitle(context.Background(), "conv-456", "title-2")
		_ = mock.UpdateConversationMetadata(context.Background(), "conv-789", map[string]string{"key": "value"})

		if len(mock.CallLog) != 3 {
			t.Errorf("expected 3 calls logged, got %d", len(mock.CallLog))
		}

		expectedCalls := []string{
			"UpdateTitle(conv-123, title-1)",
			"UpdateTitle(conv-456, title-2)",
			"UpdateMetadata(conv-789, map[key:value])",
		}

		for i, expected := range expectedCalls {
			if i >= len(mock.CallLog) {
				t.Errorf("missing call log entry %d", i)
				continue
			}
			if mock.CallLog[i] != expected {
				t.Errorf("call %d: expected %s, got %s", i, expected, mock.CallLog[i])
			}
		}
	})
}

// BenchmarkGeminiTranslator_RenameSession benchmarks RenameSession overhead
func BenchmarkGeminiTranslator_RenameSession(b *testing.B) {
	mock := &MockGeminiClient{}
	translator := NewGeminiTranslator(mock)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = translator.RenameSession(ctx, "conv-123", "new-name")
	}
}

// BenchmarkGeminiTranslator_SetDirectory benchmarks SetDirectory overhead
func BenchmarkGeminiTranslator_SetDirectory(b *testing.B) {
	mock := &MockGeminiClient{}
	translator := NewGeminiTranslator(mock)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = translator.SetDirectory(ctx, "conv-123", "~/project")
	}
}

// BenchmarkGeminiTranslator_RunHook benchmarks RunHook overhead
func BenchmarkGeminiTranslator_RunHook(b *testing.B) {
	mock := &MockGeminiClient{}
	translator := NewGeminiTranslator(mock)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = translator.RunHook(ctx, "conv-123", "/hook")
	}
}

// Example demonstrates basic usage of GeminiTranslator
func Example() {
	// Create mock client for demonstration
	client := &MockGeminiClient{}

	// Create translator
	translator := NewGeminiTranslator(client)

	// Execute commands with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Rename session
	if err := translator.RenameSession(ctx, "conv-123", "my-session"); err != nil {
		if errors.Is(err, ErrNotSupported) {
			fmt.Println("Rename not supported")
		} else if errors.Is(err, ErrAPIFailure) {
			fmt.Println("API call failed:", err)
		}
		return
	}

	// Set working directory
	if err := translator.SetDirectory(ctx, "conv-123", "~/project"); err != nil {
		fmt.Println("Set directory failed:", err)
		return
	}

	// Run hook (returns ErrNotSupported for Gemini)
	if err := translator.RunHook(ctx, "conv-123", "/agm:agm-assoc"); err != nil {
		if errors.Is(err, ErrNotSupported) {
			fmt.Println("Hook not supported by Gemini")
		}
		return
	}

	// Output:
	// Hook not supported by Gemini
}
