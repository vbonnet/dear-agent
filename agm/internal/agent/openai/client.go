package openai

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/sashabaranov/go-openai"
)

// ErrorType represents specific error categories for OpenAI API calls.
type ErrorType string

const (
	// ErrorTypeAPIKeyMissing indicates that the API key is not configured.
	ErrorTypeAPIKeyMissing ErrorType = "API_KEY_MISSING"

	// ErrorTypeAPIError indicates a general API error.
	ErrorTypeAPIError ErrorType = "API_ERROR"

	// ErrorTypeRateLimit indicates that rate limits have been exceeded.
	ErrorTypeRateLimit ErrorType = "RATE_LIMIT"

	// ErrorTypeInvalidRequest indicates malformed request parameters.
	ErrorTypeInvalidRequest ErrorType = "INVALID_REQUEST"

	// ErrorTypeAuthError indicates authentication failure.
	ErrorTypeAuthError ErrorType = "AUTH_ERROR"

	// ErrorTypeInvalidModel indicates an unsupported model was specified.
	ErrorTypeInvalidModel ErrorType = "INVALID_MODEL"
)

// Supported OpenAI models
var supportedModels = map[string]bool{
	"gpt-4":               true,
	"gpt-4-32k":           true,
	"gpt-4-turbo":         true,
	"gpt-4-turbo-preview": true,
	"gpt-4.1":             true,
	"gpt-4.1-mini":        true,
	"gpt-4o":              true,
	"gpt-4o-mini":         true,
	"o3":                  true,
	"o4-mini":             true,
	"gpt-3.5-turbo":       true,
}

// Models that do not support streaming
var nonStreamingModels = map[string]bool{
	"o3":      true,
	"o4-mini": true,
}

// ClientError represents a structured error from OpenAI API operations.
type ClientError struct {
	Type    ErrorType
	Message string
	Err     error
}

func (e *ClientError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %s: %v", e.Type, e.Message, e.Err)
	}
	return fmt.Sprintf("%s: %s", e.Type, e.Message)
}

func (e *ClientError) Unwrap() error {
	return e.Err
}

// Config holds the configuration for the OpenAI client.
type Config struct {
	// APIKey is the OpenAI API key.
	// If empty, will be read from OPENAI_API_KEY environment variable.
	APIKey string

	// BaseURL is the OpenAI API base URL.
	// Defaults to https://api.openai.com/v1 if empty.
	// For Azure OpenAI, set to your Azure endpoint.
	BaseURL string

	// Model is the model to use for chat completions.
	// Defaults to gpt-4-turbo-preview if empty.
	// Can be set via OPENAI_MODEL environment variable.
	// Supported models: gpt-4, gpt-4-turbo, gpt-4-turbo-preview,
	// gpt-4.1, gpt-4.1-mini, gpt-4o, gpt-4o-mini, o3, o4-mini, gpt-3.5-turbo
	Model string

	// Temperature controls randomness in responses (0.0-2.0).
	// Defaults to 0.7 if not set.
	Temperature float32

	// MaxTokens is the maximum tokens to generate.
	// Defaults to 1000 if not set.
	MaxTokens int

	// IsAzure indicates if this is an Azure OpenAI endpoint.
	// When true, uses Azure-specific authentication.
	IsAzure bool

	// AzureAPIVersion is the API version for Azure OpenAI.
	// Only used when IsAzure is true.
	// Defaults to "2024-02-15-preview" if empty.
	AzureAPIVersion string
}

// DefaultConfig returns a default configuration.
// API key is read from OPENAI_API_KEY environment variable.
// Model is read from OPENAI_MODEL environment variable or defaults to gpt-4-turbo-preview.
func DefaultConfig() Config {
	model := os.Getenv("OPENAI_MODEL")
	if model == "" {
		model = "gpt-4-turbo-preview"
	}

	return Config{
		APIKey:      os.Getenv("OPENAI_API_KEY"),
		BaseURL:     "",
		Model:       model,
		Temperature: 0.7,
		MaxTokens:   1000,
		IsAzure:     false,
	}
}

// ValidateModel checks if a model is supported.
func ValidateModel(model string) error {
	if model == "" {
		return &ClientError{
			Type:    ErrorTypeInvalidModel,
			Message: "model cannot be empty",
		}
	}

	if !supportedModels[model] {
		return &ClientError{
			Type:    ErrorTypeInvalidModel,
			Message: fmt.Sprintf("unsupported model: %s. Supported models: gpt-4, gpt-4-32k, gpt-4-turbo, gpt-4-turbo-preview, gpt-4.1, gpt-4.1-mini, gpt-4o, gpt-4o-mini, o3, o4-mini, gpt-3.5-turbo", model),
		}
	}

	return nil
}

// SupportsStreaming returns true if the model supports streaming responses.
func SupportsStreaming(model string) bool {
	return !nonStreamingModels[model]
}

// Client wraps the OpenAI API client with conversation history support.
type Client struct {
	config Config
	client *openai.Client
}

// NewClient creates a new OpenAI client with the given configuration.
// Returns an error if the API key is missing or model is invalid.
func NewClient(ctx context.Context, config Config) (*Client, error) {
	// Validate API key
	if config.APIKey == "" {
		return nil, &ClientError{
			Type:    ErrorTypeAPIKeyMissing,
			Message: "OpenAI API key is required. Set OPENAI_API_KEY environment variable or provide in config",
		}
	}

	// Set defaults
	if config.Model == "" {
		config.Model = os.Getenv("OPENAI_MODEL")
		if config.Model == "" {
			config.Model = "gpt-4-turbo-preview"
		}
	}

	// Validate model
	if err := ValidateModel(config.Model); err != nil {
		return nil, err
	}

	if config.Temperature == 0 {
		config.Temperature = 0.7
	}
	if config.MaxTokens == 0 {
		config.MaxTokens = 1000
	}

	// Create OpenAI client configuration
	var client *openai.Client
	if config.IsAzure {
		// Azure OpenAI configuration
		if config.AzureAPIVersion == "" {
			config.AzureAPIVersion = "2024-02-15-preview"
		}
		azureConfig := openai.DefaultAzureConfig(config.APIKey, config.BaseURL)
		azureConfig.APIVersion = config.AzureAPIVersion
		client = openai.NewClientWithConfig(azureConfig)
	} else {
		// Standard OpenAI configuration
		clientConfig := openai.DefaultConfig(config.APIKey)
		if config.BaseURL != "" {
			clientConfig.BaseURL = config.BaseURL
		}
		client = openai.NewClientWithConfig(clientConfig)
	}

	return &Client{
		config: config,
		client: client,
	}, nil
}

// Message represents a chat message with role and content.
type Message struct {
	Role      string // "system", "user", or "assistant"
	Content   string
	Timestamp time.Time `json:"timestamp,omitempty"` // Optional timestamp for conversation history
}

// ChatCompletionResponse contains the response from a chat completion request.
type ChatCompletionResponse struct {
	// Content is the assistant's response text.
	Content string

	// Model is the model that generated the response.
	Model string

	// FinishReason indicates why the response ended.
	// Values: "stop", "length", "content_filter", "function_call"
	FinishReason string

	// Usage contains token usage statistics.
	Usage TokenUsage
}

// TokenUsage contains token usage statistics for a completion.
type TokenUsage struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
}

// CreateChatCompletion sends a chat completion request with conversation history.
// The messages parameter should include the full conversation history.
// Returns the assistant's response or an error.
func (c *Client) CreateChatCompletion(ctx context.Context, messages []Message) (*ChatCompletionResponse, error) {
	if len(messages) == 0 {
		return nil, &ClientError{
			Type:    ErrorTypeInvalidRequest,
			Message: "messages cannot be empty",
		}
	}

	// Convert messages to OpenAI format
	openaiMessages := make([]openai.ChatCompletionMessage, len(messages))
	for i, msg := range messages {
		openaiMessages[i] = openai.ChatCompletionMessage{
			Role:    msg.Role,
			Content: msg.Content,
		}
	}

	// Create chat completion request
	req := openai.ChatCompletionRequest{
		Model:       c.config.Model,
		Messages:    openaiMessages,
		Temperature: c.config.Temperature,
		MaxTokens:   c.config.MaxTokens,
	}

	// Send request
	resp, err := c.client.CreateChatCompletion(ctx, req)
	if err != nil {
		return nil, c.classifyError(err)
	}

	// Validate response
	if len(resp.Choices) == 0 {
		return nil, &ClientError{
			Type:    ErrorTypeAPIError,
			Message: "no response choices returned from API",
		}
	}

	// Extract response
	choice := resp.Choices[0]
	return &ChatCompletionResponse{
		Content:      choice.Message.Content,
		Model:        resp.Model,
		FinishReason: string(choice.FinishReason),
		Usage: TokenUsage{
			PromptTokens:     resp.Usage.PromptTokens,
			CompletionTokens: resp.Usage.CompletionTokens,
			TotalTokens:      resp.Usage.TotalTokens,
		},
	}, nil
}

// classifyError converts OpenAI SDK errors into structured ClientError types.
func (c *Client) classifyError(err error) error {
	if err == nil {
		return nil
	}

	// Check for specific OpenAI API errors
	var apiErr *openai.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.HTTPStatusCode {
		case 401:
			return &ClientError{
				Type:    ErrorTypeAuthError,
				Message: "authentication failed",
				Err:     err,
			}
		case 429:
			return &ClientError{
				Type:    ErrorTypeRateLimit,
				Message: "rate limit exceeded",
				Err:     err,
			}
		case 400:
			return &ClientError{
				Type:    ErrorTypeInvalidRequest,
				Message: "invalid request parameters",
				Err:     err,
			}
		default:
			return &ClientError{
				Type:    ErrorTypeAPIError,
				Message: fmt.Sprintf("API error (status %d)", apiErr.HTTPStatusCode),
				Err:     err,
			}
		}
	}

	// Generic API error
	return &ClientError{
		Type:    ErrorTypeAPIError,
		Message: "API request failed",
		Err:     err,
	}
}

// Model returns the configured model name.
func (c *Client) Model() string {
	return c.config.Model
}

// IsAzure returns true if this client is configured for Azure OpenAI.
func (c *Client) IsAzure() bool {
	return c.config.IsAzure
}
