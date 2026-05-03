package openai_test

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/vbonnet/dear-agent/agm/internal/agent/openai"
)

var logger = slog.Default()

// ExampleNewClient demonstrates creating a new OpenAI client.
func ExampleNewClient() {
	config := openai.Config{
		APIKey:      "your-api-key-here",
		Model:       "gpt-4-turbo",
		Temperature: 0.7,
		MaxTokens:   1000,
	}

	client, err := openai.NewClient(context.Background(), config)
	if err != nil {
		logger.Error("Failed to create OpenAI client", "error", err)
		return
	}

	fmt.Printf("Client created for model: %s\n", client.Model())
	// Output: Client created for model: gpt-4-turbo
}

// ExampleNewClient_defaultConfig demonstrates creating a client with default configuration.
func ExampleNewClient_defaultConfig() {
	// Uses OPENAI_API_KEY environment variable
	config := openai.DefaultConfig()

	// Override specific settings
	config.Model = "gpt-4"
	config.Temperature = 0.3

	client, err := openai.NewClient(context.Background(), config)
	if err != nil {
		// In real code, you would handle this error
		// For example purposes, we skip if API key is not set
		return
	}

	fmt.Printf("Client model: %s\n", client.Model())
}

// ExampleNewClient_azure demonstrates creating an Azure OpenAI client.
func ExampleNewClient_azure() {
	config := openai.Config{
		APIKey:          "your-azure-api-key",
		BaseURL:         "https://your-resource.openai.azure.com",
		Model:           "gpt-4",
		IsAzure:         true,
		AzureAPIVersion: "2024-02-15-preview",
	}

	client, err := openai.NewClient(context.Background(), config)
	if err != nil {
		logger.Error("Failed to create Azure OpenAI client", "error", err)
		return
	}

	fmt.Printf("Azure client: %v\n", client.IsAzure())
	// Output: Azure client: true
}

// ExampleClient_CreateChatCompletion demonstrates sending a simple chat message.
func ExampleClient_CreateChatCompletion() {
	config := openai.Config{
		APIKey: "your-api-key-here",
		Model:  "gpt-3.5-turbo",
	}

	client, err := openai.NewClient(context.Background(), config)
	if err != nil {
		// Handle error
		return
	}

	messages := []openai.Message{
		{
			Role:    "system",
			Content: "You are a helpful assistant.",
		},
		{
			Role:    "user",
			Content: "What is 2+2?",
		},
	}

	resp, err := client.CreateChatCompletion(context.Background(), messages)
	if err != nil {
		// Handle error
		return
	}

	fmt.Printf("Response: %s\n", resp.Content)
	fmt.Printf("Tokens used: %d\n", resp.Usage.TotalTokens)
}

// ExampleClient_CreateChatCompletion_conversationHistory demonstrates a multi-turn conversation.
func ExampleClient_CreateChatCompletion_conversationHistory() {
	config := openai.Config{
		APIKey: "your-api-key-here",
		Model:  "gpt-3.5-turbo",
	}

	client, err := openai.NewClient(context.Background(), config)
	if err != nil {
		// Handle error
		return
	}

	// Build conversation history
	messages := []openai.Message{
		{
			Role:    "system",
			Content: "You are a helpful assistant that remembers context.",
		},
		{
			Role:    "user",
			Content: "My favorite color is blue.",
		},
		{
			Role:    "assistant",
			Content: "I'll remember that your favorite color is blue.",
		},
		{
			Role:    "user",
			Content: "What is my favorite color?",
		},
	}

	resp, err := client.CreateChatCompletion(context.Background(), messages)
	if err != nil {
		// Handle error
		return
	}

	// The response should mention "blue" based on conversation history
	fmt.Printf("Response: %s\n", resp.Content)
}

// ExampleClient_CreateChatCompletion_errorHandling demonstrates error handling.
func ExampleClient_CreateChatCompletion_errorHandling() {
	config := openai.Config{
		APIKey: "invalid-key",
		Model:  "gpt-3.5-turbo",
	}

	client, err := openai.NewClient(context.Background(), config)
	if err != nil {
		// Handle client creation error
		return
	}

	messages := []openai.Message{
		{
			Role:    "user",
			Content: "Hello!",
		},
	}

	resp, err := client.CreateChatCompletion(context.Background(), messages)
	if err != nil {
		// Check for specific error types
		clientErr := &openai.ClientError{}
		if errors.As(err, &clientErr) {
			switch clientErr.Type {
			case openai.ErrorTypeAPIKeyMissing:
				fmt.Println("API key is not configured")
			case openai.ErrorTypeAuthError:
				fmt.Println("Authentication failed - check your API key")
			case openai.ErrorTypeRateLimit:
				fmt.Println("Rate limit exceeded - try again later")
			case openai.ErrorTypeInvalidRequest:
				fmt.Println("Invalid request parameters")
			case openai.ErrorTypeAPIError:
				fmt.Println("API error occurred")
			}
			return
		}
		// Handle other errors
		return
	}

	fmt.Printf("Response: %s\n", resp.Content)
}
