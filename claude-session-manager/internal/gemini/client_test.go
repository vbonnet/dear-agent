package gemini

import (
	"context"
	"errors"
	"testing"
)

func TestNewClient(t *testing.T) {
	tests := []struct {
		name    string
		apiKey  string
		wantErr bool
	}{
		{
			name:    "valid API key",
			apiKey:  "test-api-key",
			wantErr: false,
		},
		{
			name:    "empty API key",
			apiKey:  "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := NewClient(tt.apiKey)

			if (err != nil) != tt.wantErr {
				t.Errorf("NewClient() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && client == nil {
				t.Error("NewClient() returned nil client")
			}

			if tt.wantErr {
				var userErr *UserError
				if !errors.As(err, &userErr) {
					t.Error("NewClient() error should be UserError")
				}
			}
		})
	}
}

func TestMockClient_CreateSession(t *testing.T) {
	ctx := context.Background()

	t.Run("default behavior", func(t *testing.T) {
		mockClient := &MockClient{}

		session, err := mockClient.CreateSession(ctx, "gemini-pro")
		if err != nil {
			t.Fatalf("CreateSession() unexpected error: %v", err)
		}

		if session.ID != "mock-session" {
			t.Errorf("CreateSession() ID = %q, want %q", session.ID, "mock-session")
		}

		if session.Model != "gemini-pro" {
			t.Errorf("CreateSession() Model = %q, want %q", session.Model, "gemini-pro")
		}
	})

	t.Run("custom function", func(t *testing.T) {
		mockClient := &MockClient{
			CreateSessionFunc: func(ctx context.Context, model string) (Session, error) {
				return Session{ID: "custom-id", Model: model, History: []*Message{}}, nil
			},
		}

		session, err := mockClient.CreateSession(ctx, "gemini-pro")
		if err != nil {
			t.Fatalf("CreateSession() unexpected error: %v", err)
		}

		if session.ID != "custom-id" {
			t.Errorf("CreateSession() ID = %q, want %q", session.ID, "custom-id")
		}
	})

	t.Run("custom error", func(t *testing.T) {
		expectedErr := errors.New("custom error")
		mockClient := &MockClient{
			CreateSessionFunc: func(ctx context.Context, model string) (Session, error) {
				return Session{}, expectedErr
			},
		}

		_, err := mockClient.CreateSession(ctx, "gemini-pro")
		if err != expectedErr {
			t.Errorf("CreateSession() error = %v, want %v", err, expectedErr)
		}
	})
}

func TestMockClient_SendMessage(t *testing.T) {
	ctx := context.Background()
	session := Session{ID: "test", Model: "gemini-pro", History: []*Message{}}

	t.Run("default behavior", func(t *testing.T) {
		mockClient := &MockClient{}

		response, err := mockClient.SendMessage(ctx, session, "Hello")
		if err != nil {
			t.Fatalf("SendMessage() unexpected error: %v", err)
		}

		expected := "Mock response to: Hello"
		if response != expected {
			t.Errorf("SendMessage() = %q, want %q", response, expected)
		}
	})

	t.Run("custom function", func(t *testing.T) {
		mockClient := &MockClient{
			SendMessageFunc: func(ctx context.Context, session Session, message string) (string, error) {
				if message == "" {
					return "", &UserError{Message: "message cannot be empty"}
				}
				return "Custom response", nil
			},
		}

		// Valid message
		response, err := mockClient.SendMessage(ctx, session, "Hello")
		if err != nil {
			t.Fatalf("SendMessage() unexpected error: %v", err)
		}

		if response != "Custom response" {
			t.Errorf("SendMessage() = %q, want %q", response, "Custom response")
		}

		// Empty message
		_, err = mockClient.SendMessage(ctx, session, "")
		var userErr *UserError
		if !errors.As(err, &userErr) {
			t.Error("SendMessage() with empty message should return UserError")
		}
	})
}

func TestMockClient_Close(t *testing.T) {
	t.Run("default behavior", func(t *testing.T) {
		mockClient := &MockClient{}

		err := mockClient.Close()
		if err != nil {
			t.Errorf("Close() unexpected error: %v", err)
		}
	})

	t.Run("custom function", func(t *testing.T) {
		expectedErr := errors.New("close error")
		mockClient := &MockClient{
			CloseFunc: func() error {
				return expectedErr
			},
		}

		err := mockClient.Close()
		if err != expectedErr {
			t.Errorf("Close() error = %v, want %v", err, expectedErr)
		}
	})
}
