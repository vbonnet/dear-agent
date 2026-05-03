// Package llm provides llm functionality.
package llm

import (
	"context"
	"fmt"
	"os"
	"strings"

	openai "github.com/sashabaranov/go-openai"
)

// ProviderType represents the OpenAI provider type
type ProviderType string

const (
	// ProviderOpenAI represents the standard OpenAI API
	ProviderOpenAI ProviderType = "openai"
	// ProviderAzure represents Azure OpenAI Service
	ProviderAzure ProviderType = "azure"
)

// OpenAIConfig holds configuration for both OpenAI and Azure OpenAI
type OpenAIConfig struct {
	// Common fields
	APIKey      string
	Model       string
	Temperature float32
	MaxTokens   int

	// Provider type (auto-detected if not specified)
	Provider ProviderType

	// Azure-specific fields
	AzureEndpoint   string // e.g., "https://your-resource.openai.azure.com"
	AzureDeployment string // Required for Azure: deployment name
	AzureAPIVersion string // e.g., "2024-02-15-preview"
}

// OpenAIClient wraps the go-openai client with provider support
type OpenAIClient struct {
	client   *openai.Client
	config   OpenAIConfig
	provider ProviderType
}

// NewOpenAIClient creates a new OpenAI client with auto-detection of provider
func NewOpenAIClient(config OpenAIConfig) (*OpenAIClient, error) {
	// Auto-detect provider if not specified
	if config.Provider == "" {
		config.Provider = detectProvider()
	}

	// Load environment variables if config fields are empty
	if err := loadEnvironmentConfig(&config); err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	// Validate configuration
	if err := validateConfig(config); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	// Create the underlying client based on provider
	var client *openai.Client
	var err error

	switch config.Provider {
	case ProviderOpenAI:
		client, err = createOpenAIClient(config)
	case ProviderAzure:
		client, err = createAzureClient(config)
	default:
		return nil, fmt.Errorf("unsupported provider: %s", config.Provider)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to create client: %w", err)
	}

	return &OpenAIClient{
		client:   client,
		config:   config,
		provider: config.Provider,
	}, nil
}

// detectProvider auto-detects the provider based on environment variables
func detectProvider() ProviderType {
	// Check for Azure-specific environment variables
	if os.Getenv("AZURE_OPENAI_ENDPOINT") != "" || os.Getenv("AZURE_OPENAI_KEY") != "" {
		return ProviderAzure
	}

	// Default to standard OpenAI
	return ProviderOpenAI
}

// loadEnvironmentConfig loads configuration from environment variables
func loadEnvironmentConfig(config *OpenAIConfig) error {
	switch config.Provider {
	case ProviderOpenAI:
		if config.APIKey == "" {
			config.APIKey = os.Getenv("OPENAI_API_KEY")
		}
		if config.Model == "" {
			// Use environment variable or default
			config.Model = getEnvOrDefault("OPENAI_MODEL", "gpt-4-turbo-preview")
		}

	case ProviderAzure:
		if config.APIKey == "" {
			// Azure supports both AZURE_OPENAI_KEY and OPENAI_API_KEY
			config.APIKey = os.Getenv("AZURE_OPENAI_KEY")
			if config.APIKey == "" {
				config.APIKey = os.Getenv("OPENAI_API_KEY")
			}
		}
		if config.AzureEndpoint == "" {
			config.AzureEndpoint = os.Getenv("AZURE_OPENAI_ENDPOINT")
		}
		if config.AzureDeployment == "" {
			config.AzureDeployment = os.Getenv("AZURE_OPENAI_DEPLOYMENT")
		}
		if config.AzureAPIVersion == "" {
			config.AzureAPIVersion = getEnvOrDefault("AZURE_OPENAI_API_VERSION", "2024-02-15-preview")
		}
	}

	// Set defaults for common fields
	if config.Temperature == 0 {
		config.Temperature = 0.7
	}
	if config.MaxTokens == 0 {
		config.MaxTokens = 1000
	}

	return nil
}

// validateConfig validates the configuration for the selected provider
func validateConfig(config OpenAIConfig) error {
	if config.APIKey == "" {
		return fmt.Errorf("API key is required")
	}

	switch config.Provider {
	case ProviderOpenAI:
		if config.Model == "" {
			return fmt.Errorf("model is required for OpenAI provider")
		}

	case ProviderAzure:
		if config.AzureEndpoint == "" {
			return fmt.Errorf("Azure endpoint is required for Azure provider") //nolint:staticcheck // proper noun
		}
		if config.AzureDeployment == "" {
			return fmt.Errorf("Azure deployment name is required for Azure provider") //nolint:staticcheck // proper noun
		}
		if config.AzureAPIVersion == "" {
			return fmt.Errorf("Azure API version is required for Azure provider") //nolint:staticcheck // proper noun
		}
	}

	return nil
}

// createOpenAIClient creates a standard OpenAI client
func createOpenAIClient(config OpenAIConfig) (*openai.Client, error) {
	clientConfig := openai.DefaultConfig(config.APIKey)
	return openai.NewClientWithConfig(clientConfig), nil
}

// createAzureClient creates an Azure OpenAI client
func createAzureClient(config OpenAIConfig) (*openai.Client, error) {
	// Azure OpenAI requires special configuration
	azureConfig := openai.DefaultAzureConfig(config.APIKey, config.AzureEndpoint)

	// Set API version
	azureConfig.APIVersion = config.AzureAPIVersion

	// Note: Azure uses deployment names instead of model names
	// The deployment name will be set in the request, not here

	return openai.NewClientWithConfig(azureConfig), nil
}

// CreateChatCompletion creates a chat completion using the configured provider
func (c *OpenAIClient) CreateChatCompletion(ctx context.Context, messages []openai.ChatCompletionMessage) (*openai.ChatCompletionResponse, error) {
	// Build the request
	req := openai.ChatCompletionRequest{
		Messages:    messages,
		Temperature: c.config.Temperature,
		MaxTokens:   c.config.MaxTokens,
	}

	// Set model or deployment name based on provider
	switch c.provider {
	case ProviderOpenAI:
		req.Model = c.config.Model
	case ProviderAzure:
		// For Azure, use the deployment name as the model
		req.Model = c.config.AzureDeployment
	}

	// Make the API call
	resp, err := c.client.CreateChatCompletion(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to create chat completion: %w", err)
	}

	return &resp, nil
}

// CreateChatCompletionWithFunctions creates a chat completion with function calling support
func (c *OpenAIClient) CreateChatCompletionWithFunctions(
	ctx context.Context,
	messages []openai.ChatCompletionMessage,
	functions []openai.FunctionDefinition,
) (*openai.ChatCompletionResponse, error) {
	req := openai.ChatCompletionRequest{
		Messages:    messages,
		Temperature: c.config.Temperature,
		MaxTokens:   c.config.MaxTokens,
		Tools:       make([]openai.Tool, len(functions)),
	}

	// Convert functions to tools format
	for i, fn := range functions {
		req.Tools[i] = openai.Tool{
			Type:     openai.ToolTypeFunction,
			Function: &fn,
		}
	}

	// Set model or deployment name based on provider
	switch c.provider {
	case ProviderOpenAI:
		req.Model = c.config.Model
	case ProviderAzure:
		req.Model = c.config.AzureDeployment
	}

	resp, err := c.client.CreateChatCompletion(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to create chat completion with functions: %w", err)
	}

	return &resp, nil
}

// GetProvider returns the current provider type
func (c *OpenAIClient) GetProvider() ProviderType {
	return c.provider
}

// GetModel returns the model name (or deployment name for Azure)
func (c *OpenAIClient) GetModel() string {
	switch c.provider {
	case ProviderOpenAI:
		return c.config.Model
	case ProviderAzure:
		return c.config.AzureDeployment
	default:
		return ""
	}
}

// GetEndpoint returns the API endpoint being used
func (c *OpenAIClient) GetEndpoint() string {
	switch c.provider {
	case ProviderOpenAI:
		return "https://api.openai.com/v1"
	case ProviderAzure:
		return c.config.AzureEndpoint
	default:
		return ""
	}
}

// getEnvOrDefault returns environment variable value or default
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// ValidateConnection tests the connection to the OpenAI API
func (c *OpenAIClient) ValidateConnection(ctx context.Context) error {
	// Make a simple request to validate connectivity
	messages := []openai.ChatCompletionMessage{
		{
			Role:    openai.ChatMessageRoleUser,
			Content: "Hello",
		},
	}

	req := openai.ChatCompletionRequest{
		Messages:    messages,
		MaxTokens:   5,
		Temperature: 0,
	}

	// Set model based on provider
	switch c.provider {
	case ProviderOpenAI:
		req.Model = c.config.Model
	case ProviderAzure:
		req.Model = c.config.AzureDeployment
	}

	_, err := c.client.CreateChatCompletion(ctx, req)
	if err != nil {
		// Check for common error types
		if strings.Contains(err.Error(), "401") {
			return fmt.Errorf("authentication failed: invalid API key")
		}
		if strings.Contains(err.Error(), "404") {
			if c.provider == ProviderAzure {
				return fmt.Errorf("deployment not found: check AZURE_OPENAI_DEPLOYMENT")
			}
			return fmt.Errorf("model not found: check model configuration")
		}
		return fmt.Errorf("connection validation failed: %w", err)
	}

	return nil
}
