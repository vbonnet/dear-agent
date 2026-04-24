package sandbox

import (
	"context"
	"sync"
	"time"
)

// MockProvider is a fake implementation for testing.
// It doesn't actually create sandboxes, just tracks state in memory.
type MockProvider struct {
	mu         sync.Mutex
	sandboxes  map[string]*Sandbox
	createErr  error // Inject error for Create
	destroyErr error // Inject error for Destroy
}

// NewMockProvider creates a new MockProvider instance.
func NewMockProvider() *MockProvider {
	return &MockProvider{
		sandboxes: make(map[string]*Sandbox),
	}
}

// Create creates a mock sandbox without actually provisioning resources.
func (m *MockProvider) Create(ctx context.Context, req SandboxRequest) (*Sandbox, error) {
	// Check context cancellation before creating
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if m.createErr != nil {
		return nil, m.createErr
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	sb := &Sandbox{
		ID:         req.SessionID,
		MergedPath: req.WorkspaceDir + "/merged",
		UpperPath:  req.WorkspaceDir + "/upper",
		WorkPath:   req.WorkspaceDir + "/work",
		Type:       "mock",
		CreatedAt:  time.Now(),
	}

	m.sandboxes[sb.ID] = sb
	return sb, nil
}

// Destroy removes a mock sandbox from the in-memory registry.
func (m *MockProvider) Destroy(ctx context.Context, id string) error {
	if m.destroyErr != nil {
		return m.destroyErr
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.sandboxes, id)
	return nil
}

// Validate checks if a mock sandbox exists in the registry.
func (m *MockProvider) Validate(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.sandboxes[id]; !exists {
		return NewError(ErrCodeSandboxNotFound, "sandbox not found: "+id)
	}
	return nil
}

// Name returns the provider name.
func (m *MockProvider) Name() string {
	return "mock"
}

// SetCreateError injects an error to be returned by Create.
func (m *MockProvider) SetCreateError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.createErr = err
}

// SetDestroyError injects an error to be returned by Destroy.
func (m *MockProvider) SetDestroyError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.destroyErr = err
}

// GetSandbox retrieves a sandbox from the registry for testing.
func (m *MockProvider) GetSandbox(id string) (*Sandbox, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	sb, exists := m.sandboxes[id]
	return sb, exists
}
