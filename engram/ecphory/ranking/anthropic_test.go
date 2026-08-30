package ranking

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewAnthropicProvider(t *testing.T) {
	tests := []struct {
		name      string
		config    AnthropicConfig
		setAPIKey bool
		apiKey    string
		wantErr   bool
	}{
		{
			name: "valid config with API key",
			config: AnthropicConfig{
				Model: "claude-3-5-haiku-20241022",
			},
			setAPIKey: true,
			apiKey:    "sk-ant-test123456789",
			wantErr:   false,
		},
		{
			name: "missing API key",
			config: AnthropicConfig{
				Model: "claude-3-5-haiku-20241022",
			},
			setAPIKey: false,
			wantErr:   true,
		},
		{
			name: "default model",
			config: AnthropicConfig{
				Model: "", // Should default to claude-3-5-haiku-20241022
			},
			setAPIKey: true,
			apiKey:    "sk-ant-test123456789",
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set/unset API key using standard ANTHROPIC_API_KEY env var
			if tt.setAPIKey {
				t.Setenv("ANTHROPIC_API_KEY", tt.apiKey)
			}

			provider, err := NewAnthropicProvider(tt.config)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, provider)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, provider)

			// Verify provider interface
			assert.Equal(t, "anthropic", provider.Name())

			// Verify model (default or specified)
			if tt.config.Model == "" {
				assert.Equal(t, "claude-3-5-haiku-20241022", provider.Model())
			} else {
				assert.Equal(t, tt.config.Model, provider.Model())
			}
		})
	}
}

func TestAnthropicProvider_Capabilities(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "sk-ant-test123456789")

	provider, err := NewAnthropicProvider(AnthropicConfig{
		Model: "claude-3-5-haiku-20241022",
	})
	require.NoError(t, err)

	caps := provider.Capabilities()

	assert.True(t, caps.SupportsCaching)
	assert.True(t, caps.SupportsStructuredOutput)
	assert.Equal(t, 5, caps.MaxConcurrentRequests)
	assert.Equal(t, 200000, caps.MaxTokensPerRequest)
}
