package gemini

import "context"

// MockClient implements GeminiClient for testing
type MockClient struct {
	CreateSessionFunc func(ctx context.Context, model string) (Session, error)
	SendMessageFunc   func(ctx context.Context, session Session, message string) (string, error)
	CloseFunc         func() error
}

func (m *MockClient) CreateSession(ctx context.Context, model string) (Session, error) {
	if m.CreateSessionFunc != nil {
		return m.CreateSessionFunc(ctx, model)
	}
	return Session{ID: "mock-session", Model: model, History: []*Message{}}, nil
}

func (m *MockClient) SendMessage(ctx context.Context, session Session, message string) (string, error) {
	if m.SendMessageFunc != nil {
		return m.SendMessageFunc(ctx, session, message)
	}
	return "Mock response to: " + message, nil
}

func (m *MockClient) Close() error {
	if m.CloseFunc != nil {
		return m.CloseFunc()
	}
	return nil
}
