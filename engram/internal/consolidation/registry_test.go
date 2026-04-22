package consolidation

import (
	"context"
	"errors"
	"testing"
)

// Mock provider for testing
type mockProvider struct {
	name string
}

func (m *mockProvider) GetWorkingContext(ctx context.Context, sessionID string) (*WorkingContext, error) {
	return nil, nil
}
func (m *mockProvider) UpdateWorkingContext(ctx context.Context, sessionID string, updates ContextUpdate) error {
	return nil
}
func (m *mockProvider) GetSessionHistory(ctx context.Context, sessionID string) (*SessionHistory, error) {
	return nil, nil
}
func (m *mockProvider) AppendSessionEvent(ctx context.Context, sessionID string, event SessionEvent) error {
	return nil
}
func (m *mockProvider) PersistSession(ctx context.Context, sessionID string) error {
	return nil
}
func (m *mockProvider) StoreMemory(ctx context.Context, namespace []string, memory Memory) error {
	return nil
}
func (m *mockProvider) RetrieveMemory(ctx context.Context, namespace []string, query Query) ([]Memory, error) {
	return nil, nil
}
func (m *mockProvider) UpdateMemory(ctx context.Context, namespace []string, memoryID string, updates MemoryUpdate) error {
	return nil
}
func (m *mockProvider) DeleteMemory(ctx context.Context, namespace []string, memoryID string) error {
	return nil
}
func (m *mockProvider) StoreArtifact(ctx context.Context, artifactID string, data []byte) error {
	return nil
}
func (m *mockProvider) GetArtifact(ctx context.Context, artifactID string) ([]byte, error) {
	return nil, nil
}
func (m *mockProvider) DeleteArtifact(ctx context.Context, artifactID string) error {
	return nil
}
func (m *mockProvider) Initialize(ctx context.Context, config Config) error {
	return nil
}
func (m *mockProvider) Close(ctx context.Context) error {
	return nil
}
func (m *mockProvider) HealthCheck(ctx context.Context) error {
	return nil
}

func newMockProvider(config Config) (Provider, error) {
	name, ok := config.Options["name"].(string)
	if !ok {
		return nil, ErrInvalidConfig
	}
	return &mockProvider{name: name}, nil
}

func TestRegister(t *testing.T) {
	// Save original providers map and restore after test
	original := providers
	defer func() { providers = original }()

	// Reset providers map
	providers = make(map[string]ProviderFactory)

	Register("test", newMockProvider)

	if len(providers) != 1 {
		t.Errorf("Expected 1 provider registered, got %d", len(providers))
	}

	if _, ok := providers["test"]; !ok {
		t.Error("Provider 'test' not found in registry")
	}
}

func TestLoad(t *testing.T) {
	// Save and restore providers map
	original := providers
	defer func() { providers = original }()

	providers = make(map[string]ProviderFactory)
	Register("mock", newMockProvider)

	tests := []struct {
		name    string
		config  Config
		wantErr error
	}{
		{
			name: "load registered provider",
			config: Config{
				ProviderType: "mock",
				Options: map[string]interface{}{
					"name": "test-provider",
				},
			},
			wantErr: nil,
		},
		{
			name: "load unregistered provider",
			config: Config{
				ProviderType: "nonexistent",
				Options:      map[string]interface{}{},
			},
			wantErr: ErrProviderNotFound,
		},
		{
			name: "load provider with invalid config",
			config: Config{
				ProviderType: "mock",
				Options:      map[string]interface{}{}, // Missing "name"
			},
			wantErr: ErrInvalidConfig,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider, err := Load(tt.config)

			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("Load() error = %v, want %v", err, tt.wantErr)
				}
				if provider != nil {
					t.Error("Expected nil provider on error")
				}
			} else {
				if err != nil {
					t.Errorf("Load() unexpected error: %v", err)
				}
				if provider == nil {
					t.Error("Expected non-nil provider")
				}
			}
		})
	}
}

func TestListProviders(t *testing.T) {
	// Save and restore providers map
	original := providers
	defer func() { providers = original }()

	providers = make(map[string]ProviderFactory)

	// Empty registry
	list := ListProviders()
	if len(list) != 0 {
		t.Errorf("Expected empty list, got %d providers", len(list))
	}

	// Register multiple providers
	Register("provider1", newMockProvider)
	Register("provider2", newMockProvider)
	Register("provider3", newMockProvider)

	list = ListProviders()
	if len(list) != 3 {
		t.Errorf("Expected 3 providers, got %d", len(list))
	}

	// Check all providers present
	found := make(map[string]bool)
	for _, name := range list {
		found[name] = true
	}

	for _, expected := range []string{"provider1", "provider2", "provider3"} {
		if !found[expected] {
			t.Errorf("Provider %s not found in list", expected)
		}
	}
}
