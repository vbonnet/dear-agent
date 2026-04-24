package llm

import (
	"context"
	"os"
	"testing"

	openai "github.com/sashabaranov/go-openai"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDetectProvider(t *testing.T) {
	tests := []struct {
		name     string
		envVars  map[string]string
		expected ProviderType
	}{
		{
			name:     "no env vars defaults to OpenAI",
			envVars:  map[string]string{},
			expected: ProviderOpenAI,
		},
		{
			name: "AZURE_OPENAI_ENDPOINT detects Azure",
			envVars: map[string]string{
				"AZURE_OPENAI_ENDPOINT": "https://test.openai.azure.com",
			},
			expected: ProviderAzure,
		},
		{
			name: "AZURE_OPENAI_KEY detects Azure",
			envVars: map[string]string{
				"AZURE_OPENAI_KEY": "test-key",
			},
			expected: ProviderAzure,
		},
		{
			name: "both Azure vars present detects Azure",
			envVars: map[string]string{
				"AZURE_OPENAI_ENDPOINT": "https://test.openai.azure.com",
				"AZURE_OPENAI_KEY":      "test-key",
			},
			expected: ProviderAzure,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear all relevant env vars
			clearEnvVars(t)

			// Set test env vars
			for key, value := range tt.envVars {
				os.Setenv(key, value)
			}

			result := detectProvider()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestLoadEnvironmentConfig(t *testing.T) {
	tests := []struct {
		name        string
		provider    ProviderType
		envVars     map[string]string
		initialCfg  OpenAIConfig
		expectedCfg OpenAIConfig
	}{
		{
			name:     "OpenAI loads from env",
			provider: ProviderOpenAI,
			envVars: map[string]string{
				"OPENAI_API_KEY": "sk-test123",
				"OPENAI_MODEL":   "gpt-4",
			},
			initialCfg: OpenAIConfig{
				Provider: ProviderOpenAI,
			},
			expectedCfg: OpenAIConfig{
				Provider:    ProviderOpenAI,
				APIKey:      "sk-test123",
				Model:       "gpt-4",
				Temperature: 0.7,
				MaxTokens:   1000,
			},
		},
		{
			name:     "OpenAI uses defaults when env empty",
			provider: ProviderOpenAI,
			envVars:  map[string]string{},
			initialCfg: OpenAIConfig{
				Provider: ProviderOpenAI,
				APIKey:   "sk-configured",
			},
			expectedCfg: OpenAIConfig{
				Provider:    ProviderOpenAI,
				APIKey:      "sk-configured",
				Model:       "gpt-4-turbo-preview",
				Temperature: 0.7,
				MaxTokens:   1000,
			},
		},
		{
			name:     "Azure loads from env",
			provider: ProviderAzure,
			envVars: map[string]string{
				"AZURE_OPENAI_KEY":         "azure-key-123",
				"AZURE_OPENAI_ENDPOINT":    "https://test.openai.azure.com",
				"AZURE_OPENAI_DEPLOYMENT":  "gpt-4-deployment",
				"AZURE_OPENAI_API_VERSION": "2024-03-01-preview",
			},
			initialCfg: OpenAIConfig{
				Provider: ProviderAzure,
			},
			expectedCfg: OpenAIConfig{
				Provider:        ProviderAzure,
				APIKey:          "azure-key-123",
				AzureEndpoint:   "https://test.openai.azure.com",
				AzureDeployment: "gpt-4-deployment",
				AzureAPIVersion: "2024-03-01-preview",
				Temperature:     0.7,
				MaxTokens:       1000,
			},
		},
		{
			name:     "Azure falls back to OPENAI_API_KEY",
			provider: ProviderAzure,
			envVars: map[string]string{
				"OPENAI_API_KEY":          "fallback-key",
				"AZURE_OPENAI_ENDPOINT":   "https://test.openai.azure.com",
				"AZURE_OPENAI_DEPLOYMENT": "gpt-4-deployment",
			},
			initialCfg: OpenAIConfig{
				Provider: ProviderAzure,
			},
			expectedCfg: OpenAIConfig{
				Provider:        ProviderAzure,
				APIKey:          "fallback-key",
				AzureEndpoint:   "https://test.openai.azure.com",
				AzureDeployment: "gpt-4-deployment",
				AzureAPIVersion: "2024-02-15-preview", // default
				Temperature:     0.7,
				MaxTokens:       1000,
			},
		},
		{
			name:     "Azure prefers AZURE_OPENAI_KEY over OPENAI_API_KEY",
			provider: ProviderAzure,
			envVars: map[string]string{
				"AZURE_OPENAI_KEY":        "azure-specific-key",
				"OPENAI_API_KEY":          "generic-key",
				"AZURE_OPENAI_ENDPOINT":   "https://test.openai.azure.com",
				"AZURE_OPENAI_DEPLOYMENT": "gpt-4-deployment",
			},
			initialCfg: OpenAIConfig{
				Provider: ProviderAzure,
			},
			expectedCfg: OpenAIConfig{
				Provider:        ProviderAzure,
				APIKey:          "azure-specific-key",
				AzureEndpoint:   "https://test.openai.azure.com",
				AzureDeployment: "gpt-4-deployment",
				AzureAPIVersion: "2024-02-15-preview",
				Temperature:     0.7,
				MaxTokens:       1000,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear all relevant env vars
			clearEnvVars(t)

			// Set test env vars
			for key, value := range tt.envVars {
				os.Setenv(key, value)
			}

			cfg := tt.initialCfg
			err := loadEnvironmentConfig(&cfg)
			require.NoError(t, err)

			assert.Equal(t, tt.expectedCfg.Provider, cfg.Provider)
			assert.Equal(t, tt.expectedCfg.APIKey, cfg.APIKey)
			assert.Equal(t, tt.expectedCfg.Model, cfg.Model)
			assert.Equal(t, tt.expectedCfg.AzureEndpoint, cfg.AzureEndpoint)
			assert.Equal(t, tt.expectedCfg.AzureDeployment, cfg.AzureDeployment)
			assert.Equal(t, tt.expectedCfg.AzureAPIVersion, cfg.AzureAPIVersion)
			assert.Equal(t, tt.expectedCfg.Temperature, cfg.Temperature)
			assert.Equal(t, tt.expectedCfg.MaxTokens, cfg.MaxTokens)
		})
	}
}

func TestValidateConfig(t *testing.T) {
	tests := []struct {
		name      string
		config    OpenAIConfig
		expectErr bool
		errMsg    string
	}{
		{
			name: "valid OpenAI config",
			config: OpenAIConfig{
				Provider: ProviderOpenAI,
				APIKey:   "sk-test",
				Model:    "gpt-4",
			},
			expectErr: false,
		},
		{
			name: "OpenAI missing API key",
			config: OpenAIConfig{
				Provider: ProviderOpenAI,
				Model:    "gpt-4",
			},
			expectErr: true,
			errMsg:    "API key is required",
		},
		{
			name: "OpenAI missing model",
			config: OpenAIConfig{
				Provider: ProviderOpenAI,
				APIKey:   "sk-test",
			},
			expectErr: true,
			errMsg:    "model is required",
		},
		{
			name: "valid Azure config",
			config: OpenAIConfig{
				Provider:        ProviderAzure,
				APIKey:          "azure-key",
				AzureEndpoint:   "https://test.openai.azure.com",
				AzureDeployment: "gpt-4-deployment",
				AzureAPIVersion: "2024-02-15-preview",
			},
			expectErr: false,
		},
		{
			name: "Azure missing endpoint",
			config: OpenAIConfig{
				Provider:        ProviderAzure,
				APIKey:          "azure-key",
				AzureDeployment: "gpt-4-deployment",
				AzureAPIVersion: "2024-02-15-preview",
			},
			expectErr: true,
			errMsg:    "Azure endpoint is required",
		},
		{
			name: "Azure missing deployment",
			config: OpenAIConfig{
				Provider:        ProviderAzure,
				APIKey:          "azure-key",
				AzureEndpoint:   "https://test.openai.azure.com",
				AzureAPIVersion: "2024-02-15-preview",
			},
			expectErr: true,
			errMsg:    "Azure deployment name is required",
		},
		{
			name: "Azure missing API version",
			config: OpenAIConfig{
				Provider:        ProviderAzure,
				APIKey:          "azure-key",
				AzureEndpoint:   "https://test.openai.azure.com",
				AzureDeployment: "gpt-4-deployment",
			},
			expectErr: true,
			errMsg:    "Azure API version is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateConfig(tt.config)

			if tt.expectErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestNewOpenAIClient(t *testing.T) {
	clearEnvVars(t)

	tests := []struct {
		name      string
		envVars   map[string]string
		config    OpenAIConfig
		expectErr bool
	}{
		{
			name: "creates OpenAI client successfully",
			envVars: map[string]string{
				"OPENAI_API_KEY": "sk-test123",
			},
			config: OpenAIConfig{
				Provider: ProviderOpenAI,
				Model:    "gpt-4",
			},
			expectErr: false,
		},
		{
			name: "creates Azure client successfully",
			envVars: map[string]string{
				"AZURE_OPENAI_KEY": "azure-test",
			},
			config: OpenAIConfig{
				Provider:        ProviderAzure,
				AzureEndpoint:   "https://test.openai.azure.com",
				AzureDeployment: "gpt-4-deployment",
				AzureAPIVersion: "2024-02-15-preview",
			},
			expectErr: false,
		},
		{
			name: "auto-detects OpenAI provider",
			envVars: map[string]string{
				"OPENAI_API_KEY": "sk-test123",
			},
			config: OpenAIConfig{
				Model: "gpt-4",
			},
			expectErr: false,
		},
		{
			name: "auto-detects Azure provider",
			envVars: map[string]string{
				"AZURE_OPENAI_KEY":        "azure-test",
				"AZURE_OPENAI_ENDPOINT":   "https://test.openai.azure.com",
				"AZURE_OPENAI_DEPLOYMENT": "gpt-4-deployment",
			},
			config:    OpenAIConfig{},
			expectErr: false,
		},
		{
			name:    "fails with missing config",
			envVars: map[string]string{},
			config: OpenAIConfig{
				Provider: ProviderOpenAI,
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear and set env vars
			clearEnvVars(t)
			for key, value := range tt.envVars {
				os.Setenv(key, value)
			}

			client, err := NewOpenAIClient(tt.config)

			if tt.expectErr {
				require.Error(t, err)
				assert.Nil(t, client)
			} else {
				require.NoError(t, err)
				require.NotNil(t, client)
				assert.NotNil(t, client.client)
			}
		})
	}
}

func TestGetProvider(t *testing.T) {
	clearEnvVars(t)

	// Test OpenAI
	os.Setenv("OPENAI_API_KEY", "sk-test")
	client, err := NewOpenAIClient(OpenAIConfig{
		Provider: ProviderOpenAI,
		Model:    "gpt-4",
	})
	require.NoError(t, err)
	assert.Equal(t, ProviderOpenAI, client.GetProvider())

	// Test Azure
	clearEnvVars(t)
	os.Setenv("AZURE_OPENAI_KEY", "azure-test")
	azureClient, err := NewOpenAIClient(OpenAIConfig{
		Provider:        ProviderAzure,
		AzureEndpoint:   "https://test.openai.azure.com",
		AzureDeployment: "gpt-4-deployment",
		AzureAPIVersion: "2024-02-15-preview",
	})
	require.NoError(t, err)
	assert.Equal(t, ProviderAzure, azureClient.GetProvider())
}

func TestGetModel(t *testing.T) {
	clearEnvVars(t)

	tests := []struct {
		name          string
		config        OpenAIConfig
		envVars       map[string]string
		expectedModel string
	}{
		{
			name: "OpenAI returns model name",
			config: OpenAIConfig{
				Provider: ProviderOpenAI,
				Model:    "gpt-4-turbo",
			},
			envVars: map[string]string{
				"OPENAI_API_KEY": "sk-test",
			},
			expectedModel: "gpt-4-turbo",
		},
		{
			name: "Azure returns deployment name",
			config: OpenAIConfig{
				Provider:        ProviderAzure,
				AzureEndpoint:   "https://test.openai.azure.com",
				AzureDeployment: "my-gpt4-deployment",
				AzureAPIVersion: "2024-02-15-preview",
			},
			envVars: map[string]string{
				"AZURE_OPENAI_KEY": "azure-test",
			},
			expectedModel: "my-gpt4-deployment",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearEnvVars(t)
			for key, value := range tt.envVars {
				os.Setenv(key, value)
			}

			client, err := NewOpenAIClient(tt.config)
			require.NoError(t, err)
			assert.Equal(t, tt.expectedModel, client.GetModel())
		})
	}
}

func TestGetEndpoint(t *testing.T) {
	clearEnvVars(t)

	tests := []struct {
		name             string
		config           OpenAIConfig
		envVars          map[string]string
		expectedEndpoint string
	}{
		{
			name: "OpenAI returns standard endpoint",
			config: OpenAIConfig{
				Provider: ProviderOpenAI,
				Model:    "gpt-4",
			},
			envVars: map[string]string{
				"OPENAI_API_KEY": "sk-test",
			},
			expectedEndpoint: "https://api.openai.com/v1",
		},
		{
			name: "Azure returns custom endpoint",
			config: OpenAIConfig{
				Provider:        ProviderAzure,
				AzureEndpoint:   "https://my-resource.openai.azure.com",
				AzureDeployment: "gpt-4-deployment",
				AzureAPIVersion: "2024-02-15-preview",
			},
			envVars: map[string]string{
				"AZURE_OPENAI_KEY": "azure-test",
			},
			expectedEndpoint: "https://my-resource.openai.azure.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearEnvVars(t)
			for key, value := range tt.envVars {
				os.Setenv(key, value)
			}

			client, err := NewOpenAIClient(tt.config)
			require.NoError(t, err)
			assert.Equal(t, tt.expectedEndpoint, client.GetEndpoint())
		})
	}
}

func TestCreateChatCompletionRequest(t *testing.T) {
	clearEnvVars(t)

	tests := []struct {
		name          string
		config        OpenAIConfig
		envVars       map[string]string
		expectedModel string
	}{
		{
			name: "OpenAI uses model name",
			config: OpenAIConfig{
				Provider:    ProviderOpenAI,
				Model:       "gpt-4",
				Temperature: 0.5,
				MaxTokens:   100,
			},
			envVars: map[string]string{
				"OPENAI_API_KEY": "sk-test",
			},
			expectedModel: "gpt-4",
		},
		{
			name: "Azure uses deployment name",
			config: OpenAIConfig{
				Provider:        ProviderAzure,
				AzureEndpoint:   "https://test.openai.azure.com",
				AzureDeployment: "gpt-4-deployment",
				AzureAPIVersion: "2024-02-15-preview",
				Temperature:     0.5,
				MaxTokens:       100,
			},
			envVars: map[string]string{
				"AZURE_OPENAI_KEY": "azure-test",
			},
			expectedModel: "gpt-4-deployment",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearEnvVars(t)
			for key, value := range tt.envVars {
				os.Setenv(key, value)
			}

			client, err := NewOpenAIClient(tt.config)
			require.NoError(t, err)

			// Verify the model/deployment is set correctly
			assert.Equal(t, tt.expectedModel, client.GetModel())
		})
	}
}

// clearEnvVars clears all OpenAI-related environment variables
func clearEnvVars(t *testing.T) {
	t.Helper()
	vars := []string{
		"OPENAI_API_KEY",
		"OPENAI_MODEL",
		"AZURE_OPENAI_KEY",
		"AZURE_OPENAI_ENDPOINT",
		"AZURE_OPENAI_DEPLOYMENT",
		"AZURE_OPENAI_API_VERSION",
	}
	for _, v := range vars {
		os.Unsetenv(v)
	}
}

// Integration test (skipped by default, requires real API key)
func TestValidateConnection_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		t.Skip("OPENAI_API_KEY not set")
	}

	client, err := NewOpenAIClient(OpenAIConfig{
		Provider: ProviderOpenAI,
		APIKey:   apiKey,
		Model:    "gpt-4",
	})
	require.NoError(t, err)

	ctx := context.Background()
	err = client.ValidateConnection(ctx)
	require.NoError(t, err)
}

// Example usage test
func ExampleNewOpenAIClient_openAI() {
	// Standard OpenAI setup
	client, err := NewOpenAIClient(OpenAIConfig{
		Provider: ProviderOpenAI,
		APIKey:   "sk-your-api-key",
		Model:    "gpt-4",
	})
	if err != nil {
		panic(err)
	}

	messages := []openai.ChatCompletionMessage{
		{
			Role:    openai.ChatMessageRoleUser,
			Content: "Hello!",
		},
	}

	ctx := context.Background()
	_, _ = client.CreateChatCompletion(ctx, messages)
}

func ExampleNewOpenAIClient_azure() {
	// Azure OpenAI setup
	client, err := NewOpenAIClient(OpenAIConfig{
		Provider:        ProviderAzure,
		APIKey:          "your-azure-key",
		AzureEndpoint:   "https://your-resource.openai.azure.com",
		AzureDeployment: "gpt-4-deployment",
		AzureAPIVersion: "2024-02-15-preview",
	})
	if err != nil {
		panic(err)
	}

	messages := []openai.ChatCompletionMessage{
		{
			Role:    openai.ChatMessageRoleUser,
			Content: "Hello!",
		},
	}

	ctx := context.Background()
	_, _ = client.CreateChatCompletion(ctx, messages)
}
