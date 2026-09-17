package sandbox

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// MockProvider is a fake implementation for testing. It materializes the
// provider-owned directory shape but does not mount, clone, or isolate any
// repository content.
type MockProvider struct {
	mu          sync.Mutex
	sandboxes   map[string]*Sandbox
	directories map[string][]string
	createErr   error // Inject error for Create
	destroyErr  error // Inject error for Destroy
}

// NewMockProvider creates a new MockProvider instance.
func NewMockProvider() *MockProvider {
	return &MockProvider{
		sandboxes:   make(map[string]*Sandbox),
		directories: make(map[string][]string),
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
	if req.SessionID == "" {
		return nil, NewInvalidConfigError("SessionID", "must not be empty")
	}
	if len(req.LowerDirs) == 0 {
		return nil, NewInvalidConfigError("LowerDirs", "at least one lower directory is required")
	}
	if req.WorkspaceDir == "" {
		return nil, NewInvalidConfigError("WorkspaceDir", "must not be empty")
	}
	if !filepath.IsAbs(req.WorkspaceDir) || filepath.Clean(req.WorkspaceDir) != req.WorkspaceDir {
		return nil, NewInvalidConfigError("WorkspaceDir", "must be a clean absolute path")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	mergedDir := req.WorkspaceDir + "/merged"
	workingDir, _, err := MapFlatWorkingDir(req.WorkingDir, req.LowerDirs, mergedDir)
	if err != nil {
		return nil, err
	}

	sb := &Sandbox{
		ID:         req.SessionID,
		MergedPath: mergedDir,
		WorkingDir: workingDir,
		UpperPath:  req.WorkspaceDir + "/upper",
		WorkPath:   req.WorkspaceDir + "/work",
		Type:       "mock",
		CreatedAt:  time.Now(),
	}
	for _, dir := range []string{sb.MergedPath, sb.WorkingDir, sb.UpperPath, sb.WorkPath} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, fmt.Errorf("materialize mock sandbox directory %s: %w", dir, err)
		}
	}

	m.sandboxes[sb.ID] = sb
	m.directories[sb.ID] = []string{sb.MergedPath, sb.UpperPath, sb.WorkPath}
	return sb, nil
}

// Destroy removes a mock sandbox from the in-memory registry.
func (m *MockProvider) Destroy(ctx context.Context, id string) error {
	if m.destroyErr != nil {
		return m.destroyErr
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	_, exists := m.sandboxes[id]
	if !exists {
		return nil
	}
	seen := make(map[string]struct{})
	for _, dir := range m.directories[id] {
		clean := filepath.Clean(dir)
		if _, duplicate := seen[clean]; duplicate {
			continue
		}
		seen[clean] = struct{}{}
		if err := os.RemoveAll(clean); err != nil {
			return fmt.Errorf("remove mock sandbox directory %s: %w", clean, err)
		}
	}
	delete(m.sandboxes, id)
	delete(m.directories, id)
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
