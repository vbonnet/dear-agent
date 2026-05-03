package ranking_test

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vbonnet/dear-agent/engram/ecphory/ranking"
)

func TestFactory_NewFactory(t *testing.T) {
	config := ranking.DefaultConfig()
	factory, err := ranking.NewFactory(config)

	require.NoError(t, err)
	assert.NotNil(t, factory)

	// Local provider should always be available
	providers := factory.ListProviders()
	assert.Contains(t, providers, "local")
}

func TestFactory_NewFactory_NilConfig(t *testing.T) {
	// Should use default config when nil passed
	factory, err := ranking.NewFactory(nil)

	require.NoError(t, err)
	assert.NotNil(t, factory)

	// Local provider should still be available
	providers := factory.ListProviders()
	assert.Contains(t, providers, "local")
}

func TestFactory_AutoDetect_NoCredentials(t *testing.T) {
	// Clear all credentials
	t.Setenv("ANTHROPIC_API_KEY", "") // restored on test cleanup
	os.Unsetenv("ANTHROPIC_API_KEY")
	t.Setenv("GOOGLE_CLOUD_PROJECT", "") // restored on test cleanup
	os.Unsetenv("GOOGLE_CLOUD_PROJECT")
	t.Setenv("USE_VERTEX_GEMINI", "") // restored on test cleanup
	os.Unsetenv("USE_VERTEX_GEMINI")
	t.Setenv("VERTEX_LOCATION", "") // restored on test cleanup
	os.Unsetenv("VERTEX_LOCATION")

	factory, err := ranking.NewFactory(nil)
	require.NoError(t, err)

	provider, err := factory.AutoDetect()
	require.NoError(t, err)
	assert.Equal(t, "local", provider.Name())
}

func TestFactory_AutoDetect_Anthropic(t *testing.T) {
	// Clear other credentials
	t.Setenv("GOOGLE_CLOUD_PROJECT", "") // restored on test cleanup
	os.Unsetenv("GOOGLE_CLOUD_PROJECT")
	t.Setenv("USE_VERTEX_GEMINI", "") // restored on test cleanup
	os.Unsetenv("USE_VERTEX_GEMINI")
	t.Setenv("VERTEX_LOCATION", "") // restored on test cleanup
	os.Unsetenv("VERTEX_LOCATION")

	// Set Anthropic credentials
	t.Setenv("ANTHROPIC_API_KEY", "sk-ant-test")
	t.Cleanup(func() { os.Unsetenv("ANTHROPIC_API_KEY") })

	factory, err := ranking.NewFactory(nil)
	require.NoError(t, err)

	provider, err := factory.AutoDetect()
	require.NoError(t, err)

	// S8.2 complete: Anthropic provider should be detected
	assert.Equal(t, "anthropic", provider.Name())
}

func TestFactory_AutoDetect_VertexGemini(t *testing.T) {
	// Clear other credentials
	t.Setenv("ANTHROPIC_API_KEY", "") // restored on test cleanup
	os.Unsetenv("ANTHROPIC_API_KEY")

	// Set Google Cloud credentials
	t.Setenv("GOOGLE_CLOUD_PROJECT", "test-project")
	t.Setenv("USE_VERTEX_GEMINI", "true")
	t.Cleanup(func() {
		os.Unsetenv("GOOGLE_CLOUD_PROJECT")
		os.Unsetenv("USE_VERTEX_GEMINI")
	})

	factory, err := ranking.NewFactory(nil)
	require.NoError(t, err)

	provider, err := factory.AutoDetect()
	require.NoError(t, err)

	// S8.4 complete: Vertex Claude has higher precedence than Vertex Gemini
	// The default config sets Vertex Claude location to us-east5, so when
	// GOOGLE_CLOUD_PROJECT is set, Vertex Claude is registered and selected
	// (higher precedence than Vertex Gemini in the registration order)
	assert.Equal(t, "vertexai-claude", provider.Name())
}

func TestFactory_AutoDetect_Precedence(t *testing.T) {
	// Set both Anthropic and Google Cloud credentials
	t.Setenv("ANTHROPIC_API_KEY", "sk-ant-test")
	t.Setenv("GOOGLE_CLOUD_PROJECT", "test-project")
	t.Cleanup(func() {
		os.Unsetenv("ANTHROPIC_API_KEY")
		os.Unsetenv("GOOGLE_CLOUD_PROJECT")
	})

	factory, err := ranking.NewFactory(nil)
	require.NoError(t, err)

	// Anthropic should win (higher precedence)
	// Note: After S8.2, this will be "anthropic"
	provider, err := factory.AutoDetect()
	require.NoError(t, err)
	assert.NotNil(t, provider)
}

func TestFactory_GetProvider(t *testing.T) {
	factory, err := ranking.NewFactory(nil)
	require.NoError(t, err)

	// Get local provider (always available)
	provider, err := factory.GetProvider("local")
	require.NoError(t, err)
	assert.NotNil(t, provider)
	assert.Equal(t, "local", provider.Name())
}

func TestFactory_GetProvider_NotFound(t *testing.T) {
	factory, err := ranking.NewFactory(nil)
	require.NoError(t, err)

	_, err = factory.GetProvider("nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "provider not found")
}

func TestFactory_Register(t *testing.T) {
	factory, err := ranking.NewFactory(nil)
	require.NoError(t, err)

	// Create a mock provider
	mockProvider := &mockProvider{name: "mock", model: "mock-v1"}

	err = factory.Register(mockProvider)
	require.NoError(t, err)

	// Verify it was registered
	provider, err := factory.GetProvider("mock")
	require.NoError(t, err)
	assert.Equal(t, "mock", provider.Name())
}

func TestFactory_Register_Nil(t *testing.T) {
	factory, err := ranking.NewFactory(nil)
	require.NoError(t, err)

	err = factory.Register(nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "provider cannot be nil")
}

func TestFactory_ListProviders(t *testing.T) {
	factory, err := ranking.NewFactory(nil)
	require.NoError(t, err)

	providers := factory.ListProviders()
	assert.NotEmpty(t, providers)
	assert.Contains(t, providers, "local")
}

func TestFactory_Detect(t *testing.T) {
	// Clear all credentials (standard GCP + Claude Code variables)
	t.Setenv("ANTHROPIC_API_KEY", "") // restored on test cleanup
	os.Unsetenv("ANTHROPIC_API_KEY")
	t.Setenv("GOOGLE_CLOUD_PROJECT", "") // restored on test cleanup
	os.Unsetenv("GOOGLE_CLOUD_PROJECT")
	t.Setenv("ANTHROPIC_VERTEX_PROJECT_ID", "") // restored on test cleanup
	os.Unsetenv("ANTHROPIC_VERTEX_PROJECT_ID")
	t.Setenv("VERTEX_LOCATION", "") // restored on test cleanup
	os.Unsetenv("VERTEX_LOCATION")
	t.Setenv("CLOUD_ML_REGION", "") // restored on test cleanup
	os.Unsetenv("CLOUD_ML_REGION")
	t.Setenv("USE_VERTEX_GEMINI", "") // restored on test cleanup
	os.Unsetenv("USE_VERTEX_GEMINI")
	t.Setenv("GEMINI_API_KEY", "") // restored on test cleanup
	os.Unsetenv("GEMINI_API_KEY")

	factory, err := ranking.NewFactory(nil)
	require.NoError(t, err)

	result := factory.Detect()
	assert.NotNil(t, result)
	assert.Equal(t, "local", result.Provider)
	assert.Contains(t, result.Reason, "No API credentials")
	assert.NotEmpty(t, result.Available)
}

func TestFactory_Detect_Anthropic(t *testing.T) {
	// Set Anthropic credentials
	t.Setenv("ANTHROPIC_API_KEY", "sk-ant-test")
	t.Cleanup(func() { os.Unsetenv("ANTHROPIC_API_KEY") })

	factory, err := ranking.NewFactory(nil)
	require.NoError(t, err)

	result := factory.Detect()
	assert.NotNil(t, result)
	assert.Equal(t, "anthropic", result.Provider)
	assert.Contains(t, result.Reason, "ANTHROPIC_API_KEY")
}

func TestFactory_Detect_VertexClaude(t *testing.T) {
	// Clear Anthropic
	os.Unsetenv("ANTHROPIC_API_KEY")

	// Set Google Cloud with us-east5
	t.Setenv("GOOGLE_CLOUD_PROJECT", "test-project")
	t.Setenv("VERTEX_LOCATION", "us-east5")
	t.Cleanup(func() {
		t.Setenv("GOOGLE_CLOUD_PROJECT", "") // restored on test cleanup
		os.Unsetenv("GOOGLE_CLOUD_PROJECT")
		os.Unsetenv("VERTEX_LOCATION")
	})

	factory, err := ranking.NewFactory(nil)
	require.NoError(t, err)

	result := factory.Detect()
	assert.NotNil(t, result)
	assert.Equal(t, "vertexai-claude", result.Provider)
	assert.Contains(t, result.Reason, "us-east5")
}

func TestFactory_MustAutoDetect(t *testing.T) {
	// Clear credentials to ensure local provider
	t.Setenv("ANTHROPIC_API_KEY", "") // restored on test cleanup
	os.Unsetenv("ANTHROPIC_API_KEY")
	t.Setenv("GOOGLE_CLOUD_PROJECT", "") // restored on test cleanup
	os.Unsetenv("GOOGLE_CLOUD_PROJECT")

	factory, err := ranking.NewFactory(nil)
	require.NoError(t, err)

	// Should not panic (local always available)
	provider := factory.MustAutoDetect()
	assert.NotNil(t, provider)
	assert.Equal(t, "local", provider.Name())
}

func TestDefaultConfig(t *testing.T) {
	config := ranking.DefaultConfig()
	assert.NotNil(t, config)

	assert.Equal(t, "auto", config.Ecphory.Ranking.Provider)
	assert.Equal(t, "local", config.Ecphory.Ranking.Fallback)

	assert.Equal(t, "ANTHROPIC_API_KEY", config.Ecphory.Providers.Anthropic.APIKeyEnv)
	assert.Equal(t, "claude-3-5-haiku-20241022", config.Ecphory.Providers.Anthropic.Model)

	assert.Equal(t, "GOOGLE_CLOUD_PROJECT", config.Ecphory.Providers.VertexAI.ProjectIDEnv)
	assert.Equal(t, "gemini-2.0-flash-exp", config.Ecphory.Providers.VertexAI.Model)

	assert.Equal(t, "us-east5", config.Ecphory.Providers.VertexAIClaude.Location)
	assert.Equal(t, "claude-sonnet-4-5@20250929", config.Ecphory.Providers.VertexAIClaude.Model)
}

func TestLoadConfig_NotFound(t *testing.T) {
	// Load from non-existent path should return defaults
	config, err := ranking.LoadConfig("/nonexistent/path/config.yaml")
	require.NoError(t, err)
	assert.NotNil(t, config)

	// Should have default values
	assert.Equal(t, "auto", config.Ecphory.Ranking.Provider)
}

func TestLoadConfig_EmptyPath(t *testing.T) {
	// Empty path should check ~/.engram/config.yaml and fall back to defaults
	config, err := ranking.LoadConfig("")
	require.NoError(t, err)
	assert.NotNil(t, config)

	// Should have default values (since ~/.engram/config.yaml likely doesn't exist yet)
	assert.Equal(t, "auto", config.Ecphory.Ranking.Provider)
}

// Mock provider for testing
type mockProvider struct {
	name  string
	model string
}

func (m *mockProvider) Name() string {
	return m.name
}

func (m *mockProvider) Model() string {
	return m.model
}

func (m *mockProvider) Rank(ctx context.Context, query string, candidates []ranking.Candidate) ([]ranking.RankedResult, error) {
	return nil, nil
}

func (m *mockProvider) Capabilities() ranking.Capabilities {
	return ranking.Capabilities{}
}
