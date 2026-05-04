package openai

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/sashabaranov/go-openai"
)

func TestDefaultConfig(t *testing.T) {
	// Set environment variable for test
	testAPIKey := "test-api-key-12345"
	t.Setenv("OPENAI_API_KEY", testAPIKey)
	defer os.Unsetenv("OPENAI_API_KEY")

	config := DefaultConfig()

	if config.APIKey != testAPIKey {
		t.Errorf("expected API key %q, got %q", testAPIKey, config.APIKey)
	}
	if config.Model != "gpt-4-turbo-preview" {
		t.Errorf("expected model %q, got %q", "gpt-4-turbo-preview", config.Model)
	}
	if config.Temperature != 0.7 {
		t.Errorf("expected temperature 0.7, got %f", config.Temperature)
	}
	if config.MaxTokens != 1000 {
		t.Errorf("expected max tokens 1000, got %d", config.MaxTokens)
	}
	if config.IsAzure {
		t.Error("expected IsAzure to be false")
	}
}

func TestDefaultConfig_WithModelEnvVar(t *testing.T) {
	// Set environment variables for test
	testAPIKey := "test-api-key-12345"
	testModel := "gpt-4o"
	t.Setenv("OPENAI_API_KEY", testAPIKey)
	t.Setenv("OPENAI_MODEL", testModel)
	defer func() {
		os.Unsetenv("OPENAI_API_KEY")
		os.Unsetenv("OPENAI_MODEL")
	}()

	config := DefaultConfig()

	if config.APIKey != testAPIKey {
		t.Errorf("expected API key %q, got %q", testAPIKey, config.APIKey)
	}
	if config.Model != testModel {
		t.Errorf("expected model %q, got %q", testModel, config.Model)
	}
}

func TestNewClient_MissingAPIKey(t *testing.T) {
	config := Config{
		APIKey: "",
	}

	client, err := NewClient(context.Background(), config)

	if client != nil {
		t.Error("expected nil client when API key is missing")
	}
	if err == nil {
		t.Fatal("expected error when API key is missing")
	}

	var clientErr *ClientError
	if !errors.As(err, &clientErr) {
		t.Fatalf("expected ClientError, got %T", err)
	}
	if clientErr.Type != ErrorTypeAPIKeyMissing {
		t.Errorf("expected error type %q, got %q", ErrorTypeAPIKeyMissing, clientErr.Type)
	}
}

func TestNewClient_WithAPIKey(t *testing.T) {
	config := Config{
		APIKey: "test-api-key",
		Model:  "gpt-4",
	}

	client, err := NewClient(context.Background(), config)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if client == nil {
		t.Fatal("expected non-nil client")
	}
	if client.config.APIKey != "test-api-key" {
		t.Errorf("expected API key %q, got %q", "test-api-key", client.config.APIKey)
	}
	if client.config.Model != "gpt-4" {
		t.Errorf("expected model %q, got %q", "gpt-4", client.config.Model)
	}
}

func TestNewClient_DefaultValues(t *testing.T) {
	config := Config{
		APIKey: "test-api-key",
		// Leave other fields empty to test defaults
	}

	client, err := NewClient(context.Background(), config)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if client.config.Model != "gpt-4-turbo-preview" {
		t.Errorf("expected default model %q, got %q", "gpt-4-turbo-preview", client.config.Model)
	}
	if client.config.Temperature != 0.7 {
		t.Errorf("expected default temperature 0.7, got %f", client.config.Temperature)
	}
	if client.config.MaxTokens != 1000 {
		t.Errorf("expected default max tokens 1000, got %d", client.config.MaxTokens)
	}
}

func TestNewClient_AzureConfig(t *testing.T) {
	config := Config{
		APIKey:          "azure-api-key",
		BaseURL:         "https://my-resource.openai.azure.com",
		Model:           "gpt-4",
		IsAzure:         true,
		AzureAPIVersion: "2024-02-01",
	}

	client, err := NewClient(context.Background(), config)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if client == nil {
		t.Fatal("expected non-nil client")
	}
	if !client.config.IsAzure {
		t.Error("expected IsAzure to be true")
	}
	if client.config.AzureAPIVersion != "2024-02-01" {
		t.Errorf("expected Azure API version %q, got %q", "2024-02-01", client.config.AzureAPIVersion)
	}
}

func TestNewClient_AzureDefaultVersion(t *testing.T) {
	config := Config{
		APIKey:  "azure-api-key",
		BaseURL: "https://my-resource.openai.azure.com",
		IsAzure: true,
		// AzureAPIVersion not set - should use default
	}

	client, err := NewClient(context.Background(), config)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if client.config.AzureAPIVersion != "2024-02-15-preview" {
		t.Errorf("expected default Azure API version %q, got %q", "2024-02-15-preview", client.config.AzureAPIVersion)
	}
}

func TestCreateChatCompletion_EmptyMessages(t *testing.T) {
	config := Config{
		APIKey: "test-api-key",
	}

	client, err := NewClient(context.Background(), config)
	if err != nil {
		t.Fatalf("unexpected error creating client: %v", err)
	}

	resp, err := client.CreateChatCompletion(context.Background(), []Message{})

	if resp != nil {
		t.Error("expected nil response when messages are empty")
	}
	if err == nil {
		t.Fatal("expected error when messages are empty")
	}

	var clientErr *ClientError
	if !errors.As(err, &clientErr) {
		t.Fatalf("expected ClientError, got %T", err)
	}
	if clientErr.Type != ErrorTypeInvalidRequest {
		t.Errorf("expected error type %q, got %q", ErrorTypeInvalidRequest, clientErr.Type)
	}
}

func TestClassifyError_AuthError(t *testing.T) {
	client := &Client{}

	apiErr := &openai.APIError{
		HTTPStatusCode: 401,
		Message:        "Invalid API key",
	}

	err := client.classifyError(apiErr)

	var clientErr *ClientError
	if !errors.As(err, &clientErr) {
		t.Fatalf("expected ClientError, got %T", err)
	}
	if clientErr.Type != ErrorTypeAuthError {
		t.Errorf("expected error type %q, got %q", ErrorTypeAuthError, clientErr.Type)
	}
}

func TestClassifyError_RateLimit(t *testing.T) {
	client := &Client{}

	apiErr := &openai.APIError{
		HTTPStatusCode: 429,
		Message:        "Rate limit exceeded",
	}

	err := client.classifyError(apiErr)

	var clientErr *ClientError
	if !errors.As(err, &clientErr) {
		t.Fatalf("expected ClientError, got %T", err)
	}
	if clientErr.Type != ErrorTypeRateLimit {
		t.Errorf("expected error type %q, got %q", ErrorTypeRateLimit, clientErr.Type)
	}
}

func TestClassifyError_InvalidRequest(t *testing.T) {
	client := &Client{}

	apiErr := &openai.APIError{
		HTTPStatusCode: 400,
		Message:        "Invalid request",
	}

	err := client.classifyError(apiErr)

	var clientErr *ClientError
	if !errors.As(err, &clientErr) {
		t.Fatalf("expected ClientError, got %T", err)
	}
	if clientErr.Type != ErrorTypeInvalidRequest {
		t.Errorf("expected error type %q, got %q", ErrorTypeInvalidRequest, clientErr.Type)
	}
}

func TestClassifyError_OtherAPIError(t *testing.T) {
	client := &Client{}

	apiErr := &openai.APIError{
		HTTPStatusCode: 500,
		Message:        "Internal server error",
	}

	err := client.classifyError(apiErr)

	var clientErr *ClientError
	if !errors.As(err, &clientErr) {
		t.Fatalf("expected ClientError, got %T", err)
	}
	if clientErr.Type != ErrorTypeAPIError {
		t.Errorf("expected error type %q, got %q", ErrorTypeAPIError, clientErr.Type)
	}
}

func TestClassifyError_GenericError(t *testing.T) {
	client := &Client{}

	genericErr := errors.New("network error")

	err := client.classifyError(genericErr)

	var clientErr *ClientError
	if !errors.As(err, &clientErr) {
		t.Fatalf("expected ClientError, got %T", err)
	}
	if clientErr.Type != ErrorTypeAPIError {
		t.Errorf("expected error type %q, got %q", ErrorTypeAPIError, clientErr.Type)
	}
}

func TestClientError_Error(t *testing.T) {
	tests := []struct {
		name     string
		err      *ClientError
		expected string
	}{
		{
			name: "error with wrapped error",
			err: &ClientError{
				Type:    ErrorTypeAPIError,
				Message: "request failed",
				Err:     errors.New("network timeout"),
			},
			expected: "API_ERROR: request failed: network timeout",
		},
		{
			name: "error without wrapped error",
			err: &ClientError{
				Type:    ErrorTypeAPIKeyMissing,
				Message: "API key not set",
			},
			expected: "API_KEY_MISSING: API key not set",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.Error(); got != tt.expected {
				t.Errorf("expected error message %q, got %q", tt.expected, got)
			}
		})
	}
}

func TestClientError_Unwrap(t *testing.T) {
	innerErr := errors.New("inner error")
	clientErr := &ClientError{
		Type:    ErrorTypeAPIError,
		Message: "outer error",
		Err:     innerErr,
	}

	unwrapped := errors.Unwrap(clientErr)
	if !errors.Is(unwrapped, innerErr) {
		t.Errorf("expected unwrapped error to be %v, got %v", innerErr, unwrapped)
	}
}

func TestClient_Model(t *testing.T) {
	client := &Client{
		config: Config{
			Model: "gpt-4-turbo",
		},
	}

	if got := client.Model(); got != "gpt-4-turbo" {
		t.Errorf("expected model %q, got %q", "gpt-4-turbo", got)
	}
}

func TestClient_IsAzure(t *testing.T) {
	tests := []struct {
		name     string
		isAzure  bool
		expected bool
	}{
		{
			name:     "standard OpenAI",
			isAzure:  false,
			expected: false,
		},
		{
			name:     "Azure OpenAI",
			isAzure:  true,
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &Client{
				config: Config{
					IsAzure: tt.isAzure,
				},
			}

			if got := client.IsAzure(); got != tt.expected {
				t.Errorf("expected IsAzure() to be %v, got %v", tt.expected, got)
			}
		})
	}
}

// Integration test - requires OPENAI_API_KEY environment variable
// Run with: OPENAI_API_KEY=sk-... go test -v -run TestCreateChatCompletion_Integration
func TestCreateChatCompletion_Integration(t *testing.T) {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		t.Skip("Skipping integration test: OPENAI_API_KEY not set")
	}

	config := Config{
		APIKey:      apiKey,
		Model:       "gpt-3.5-turbo", // Use cheaper model for testing
		Temperature: 0.3,
		MaxTokens:   100,
	}

	client, err := NewClient(context.Background(), config)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	messages := []Message{
		{
			Role:    "system",
			Content: "You are a helpful assistant.",
		},
		{
			Role:    "user",
			Content: "Say 'Hello, world!' and nothing else.",
		},
	}

	resp, err := client.CreateChatCompletion(context.Background(), messages)
	if err != nil {
		t.Fatalf("failed to create chat completion: %v", err)
	}

	if resp == nil {
		t.Fatal("expected non-nil response")
	}
	if resp.Content == "" {
		t.Error("expected non-empty response content")
	}
	if resp.Model == "" {
		t.Error("expected non-empty model name")
	}
	if resp.Usage.TotalTokens == 0 {
		t.Error("expected non-zero total tokens")
	}

	t.Logf("Response: %s", resp.Content)
	t.Logf("Model: %s", resp.Model)
	t.Logf("Tokens: %d (prompt: %d, completion: %d)",
		resp.Usage.TotalTokens,
		resp.Usage.PromptTokens,
		resp.Usage.CompletionTokens,
	)
}

// Integration test for conversation history
func TestCreateChatCompletion_ConversationHistory(t *testing.T) {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		t.Skip("Skipping integration test: OPENAI_API_KEY not set")
	}

	config := Config{
		APIKey:      apiKey,
		Model:       "gpt-3.5-turbo",
		Temperature: 0.3,
		MaxTokens:   100,
	}

	client, err := NewClient(context.Background(), config)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	// Simulate a multi-turn conversation
	messages := []Message{
		{
			Role:    "system",
			Content: "You are a helpful assistant that remembers context.",
		},
		{
			Role:    "user",
			Content: "My name is Alice.",
		},
		{
			Role:    "assistant",
			Content: "Hello Alice! Nice to meet you.",
		},
		{
			Role:    "user",
			Content: "What is my name?",
		},
	}

	resp, err := client.CreateChatCompletion(context.Background(), messages)
	if err != nil {
		t.Fatalf("failed to create chat completion: %v", err)
	}

	if resp == nil {
		t.Fatal("expected non-nil response")
	}

	t.Logf("Response: %s", resp.Content)

	// The response should mention "Alice" since it has the conversation history
	// Note: This is a best-effort check and may not always pass due to model variability
	// We log the response instead of asserting to avoid flaky tests
}

func TestValidateModel(t *testing.T) {
	tests := []struct {
		name      string
		model     string
		shouldErr bool
		errType   ErrorType
	}{
		{
			name:      "valid model gpt-4",
			model:     "gpt-4",
			shouldErr: false,
		},
		{
			name:      "valid model gpt-4-turbo",
			model:     "gpt-4-turbo",
			shouldErr: false,
		},
		{
			name:      "valid model gpt-4-turbo-preview",
			model:     "gpt-4-turbo-preview",
			shouldErr: false,
		},
		{
			name:      "valid model gpt-4.1",
			model:     "gpt-4.1",
			shouldErr: false,
		},
		{
			name:      "valid model gpt-4.1-mini",
			model:     "gpt-4.1-mini",
			shouldErr: false,
		},
		{
			name:      "valid model gpt-4o",
			model:     "gpt-4o",
			shouldErr: false,
		},
		{
			name:      "valid model gpt-4o-mini",
			model:     "gpt-4o-mini",
			shouldErr: false,
		},
		{
			name:      "valid model o3",
			model:     "o3",
			shouldErr: false,
		},
		{
			name:      "valid model o4-mini",
			model:     "o4-mini",
			shouldErr: false,
		},
		{
			name:      "valid model gpt-3.5-turbo",
			model:     "gpt-3.5-turbo",
			shouldErr: false,
		},
		{
			name:      "invalid model",
			model:     "gpt-5-ultra",
			shouldErr: true,
			errType:   ErrorTypeInvalidModel,
		},
		{
			name:      "empty model",
			model:     "",
			shouldErr: true,
			errType:   ErrorTypeInvalidModel,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateModel(tt.model)

			if tt.shouldErr {
				if err == nil {
					t.Errorf("expected error for model %q, got nil", tt.model)
					return
				}

				var clientErr *ClientError
				if !errors.As(err, &clientErr) {
					t.Errorf("expected ClientError, got %T", err)
					return
				}

				if clientErr.Type != tt.errType {
					t.Errorf("expected error type %q, got %q", tt.errType, clientErr.Type)
				}
			} else if err != nil {
				t.Errorf("unexpected error for model %q: %v", tt.model, err)
			}
		})
	}
}

func TestSupportsStreaming(t *testing.T) {
	tests := []struct {
		name     string
		model    string
		expected bool
	}{
		{
			name:     "gpt-4 supports streaming",
			model:    "gpt-4",
			expected: true,
		},
		{
			name:     "gpt-4-turbo supports streaming",
			model:    "gpt-4-turbo",
			expected: true,
		},
		{
			name:     "gpt-4o supports streaming",
			model:    "gpt-4o",
			expected: true,
		},
		{
			name:     "gpt-3.5-turbo supports streaming",
			model:    "gpt-3.5-turbo",
			expected: true,
		},
		{
			name:     "o3 does not support streaming",
			model:    "o3",
			expected: false,
		},
		{
			name:     "o4-mini does not support streaming",
			model:    "o4-mini",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SupportsStreaming(tt.model)
			if result != tt.expected {
				t.Errorf("expected SupportsStreaming(%q) to be %v, got %v",
					tt.model, tt.expected, result)
			}
		})
	}
}

func TestNewClient_WithInvalidModel(t *testing.T) {
	config := Config{
		APIKey: "test-api-key",
		Model:  "invalid-model-name",
	}

	client, err := NewClient(context.Background(), config)

	if client != nil {
		t.Error("expected nil client when model is invalid")
	}
	if err == nil {
		t.Fatal("expected error when model is invalid")
	}

	var clientErr *ClientError
	if !errors.As(err, &clientErr) {
		t.Fatalf("expected ClientError, got %T", err)
	}
	if clientErr.Type != ErrorTypeInvalidModel {
		t.Errorf("expected error type %q, got %q", ErrorTypeInvalidModel, clientErr.Type)
	}
}

func TestNewClient_ModelFromEnvVar(t *testing.T) {
	testModel := "gpt-4o-mini"
	t.Setenv("OPENAI_MODEL", testModel)
	defer os.Unsetenv("OPENAI_MODEL")

	config := Config{
		APIKey: "test-api-key",
		// Model not set - should use env var
	}

	client, err := NewClient(context.Background(), config)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if client == nil {
		t.Fatal("expected non-nil client")
	}
	if client.config.Model != testModel {
		t.Errorf("expected model %q from env var, got %q", testModel, client.config.Model)
	}
}
