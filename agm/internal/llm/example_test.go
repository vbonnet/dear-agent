package llm_test

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	openai "github.com/sashabaranov/go-openai"
	"github.com/vbonnet/dear-agent/agm/internal/llm"
)

var logger = slog.Default()

// Example_openAIBasic demonstrates basic usage with OpenAI
func Example_openAIBasic() {
	// Configure the client (uses OPENAI_API_KEY from env)
	client, err := llm.NewOpenAIClient(llm.OpenAIConfig{
		Provider: llm.ProviderOpenAI,
		Model:    "gpt-4",
	})
	if err != nil {
		logger.Error("Operation failed", "error", err)
		return
	}

	// Create a simple chat completion
	ctx := context.Background()
	messages := []openai.ChatCompletionMessage{
		{
			Role:    openai.ChatMessageRoleUser,
			Content: "Say hello!",
		},
	}

	resp, err := client.CreateChatCompletion(ctx, messages)
	if err != nil {
		logger.Error("Operation failed", "error", err)
		return
	}

	fmt.Println(resp.Choices[0].Message.Content)
}

// Example_azureOpenAI demonstrates usage with Azure OpenAI
func Example_azureOpenAI() {
	// Configure for Azure (uses env vars if not specified)
	client, err := llm.NewOpenAIClient(llm.OpenAIConfig{
		Provider:        llm.ProviderAzure,
		AzureEndpoint:   "https://your-resource.openai.azure.com",
		AzureDeployment: "gpt-4-deployment",
		AzureAPIVersion: "2024-02-15-preview",
	})
	if err != nil {
		logger.Error("Operation failed", "error", err)
		return
	}

	ctx := context.Background()
	messages := []openai.ChatCompletionMessage{
		{
			Role:    openai.ChatMessageRoleUser,
			Content: "Hello from Azure!",
		},
	}

	resp, err := client.CreateChatCompletion(ctx, messages)
	if err != nil {
		logger.Error("Operation failed", "error", err)
		return
	}

	fmt.Println(resp.Choices[0].Message.Content)
}

// Example_autoDetection demonstrates automatic provider detection
func Example_autoDetection() {
	// Client automatically detects provider based on env vars
	// If AZURE_OPENAI_ENDPOINT is set → Azure
	// Otherwise → OpenAI
	client, err := llm.NewOpenAIClient(llm.OpenAIConfig{})
	if err != nil {
		logger.Error("Operation failed", "error", err)
		return
	}

	provider := client.GetProvider()
	endpoint := client.GetEndpoint()
	model := client.GetModel()

	fmt.Printf("Provider: %s\n", provider)
	fmt.Printf("Endpoint: %s\n", endpoint)
	fmt.Printf("Model: %s\n", model)
}

// Example_functionCalling demonstrates function calling with both providers
func Example_functionCalling() {
	client, err := llm.NewOpenAIClient(llm.OpenAIConfig{
		Provider: llm.ProviderOpenAI,
		Model:    "gpt-4-turbo-preview",
	})
	if err != nil {
		logger.Error("Operation failed", "error", err)
		return
	}

	// Define available functions
	functions := []openai.FunctionDefinition{
		{
			Name:        "get_current_weather",
			Description: "Get the current weather in a given location",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"location": map[string]interface{}{
						"type":        "string",
						"description": "The city and state, e.g. San Francisco, CA",
					},
					"unit": map[string]interface{}{
						"type": "string",
						"enum": []string{"celsius", "fahrenheit"},
					},
				},
				"required": []string{"location"},
			},
		},
	}

	messages := []openai.ChatCompletionMessage{
		{
			Role:    openai.ChatMessageRoleUser,
			Content: "What's the weather like in Boston?",
		},
	}

	ctx := context.Background()
	resp, err := client.CreateChatCompletionWithFunctions(ctx, messages, functions)
	if err != nil {
		logger.Error("Operation failed", "error", err)
		return
	}

	// Check if the model wants to call a function
	if len(resp.Choices) > 0 && len(resp.Choices[0].Message.ToolCalls) > 0 {
		toolCall := resp.Choices[0].Message.ToolCalls[0]
		fmt.Printf("Function called: %s\n", toolCall.Function.Name)
		fmt.Printf("Arguments: %s\n", toolCall.Function.Arguments)
	}
}

// Example_validation demonstrates connection validation
func Example_validation() {
	client, err := llm.NewOpenAIClient(llm.OpenAIConfig{
		Provider: llm.ProviderOpenAI,
		Model:    "gpt-4",
	})
	if err != nil {
		logger.Error("Operation failed", "error", err)
		return
	}

	// Validate connection before doing real work
	ctx := context.Background()
	if err := client.ValidateConnection(ctx); err != nil {
		logger.Error("Connection validation failed", "error", err)
		return
	}

	fmt.Println("Connection validated successfully!")
}

// Example_environmentVariables shows recommended environment variable setup
func Example_environmentVariables() {
	// For OpenAI:
	// export OPENAI_API_KEY="sk-your-api-key"
	// export OPENAI_MODEL="gpt-4-turbo-preview"  # optional

	// For Azure OpenAI:
	// export AZURE_OPENAI_KEY="your-azure-key"
	// export AZURE_OPENAI_ENDPOINT="https://your-resource.openai.azure.com"
	// export AZURE_OPENAI_DEPLOYMENT="gpt-4-deployment"
	// export AZURE_OPENAI_API_VERSION="2024-02-15-preview"  # optional

	// Client will auto-detect and load from environment
	client, err := llm.NewOpenAIClient(llm.OpenAIConfig{})
	if err != nil {
		logger.Error("Operation failed", "error", err)
		return
	}

	fmt.Printf("Using provider: %s\n", client.GetProvider())
}

// Example_switchingProviders demonstrates switching between providers
func Example_switchingProviders() {
	// Create OpenAI client
	openaiClient, err := llm.NewOpenAIClient(llm.OpenAIConfig{
		Provider: llm.ProviderOpenAI,
		APIKey:   os.Getenv("OPENAI_API_KEY"),
		Model:    "gpt-4",
	})
	if err != nil {
		logger.Error("Operation failed", "error", err)
		return
	}

	// Create Azure client
	azureClient, err := llm.NewOpenAIClient(llm.OpenAIConfig{
		Provider:        llm.ProviderAzure,
		APIKey:          os.Getenv("AZURE_OPENAI_KEY"),
		AzureEndpoint:   os.Getenv("AZURE_OPENAI_ENDPOINT"),
		AzureDeployment: os.Getenv("AZURE_OPENAI_DEPLOYMENT"),
		AzureAPIVersion: "2024-02-15-preview",
	})
	if err != nil {
		logger.Error("Operation failed", "error", err)
		return
	}

	// Both clients have the same interface
	ctx := context.Background()
	messages := []openai.ChatCompletionMessage{
		{
			Role:    openai.ChatMessageRoleUser,
			Content: "Hello!",
		},
	}

	// Use either client with identical code
	_, err = openaiClient.CreateChatCompletion(ctx, messages)
	if err != nil {
		logger.Warn("OpenAI error", "error", err)
	}

	_, err = azureClient.CreateChatCompletion(ctx, messages)
	if err != nil {
		logger.Warn("Azure error", "error", err)
	}
}

// Example_errorHandling demonstrates error handling
func Example_errorHandling() {
	// Example: Invalid API key
	client, err := llm.NewOpenAIClient(llm.OpenAIConfig{
		Provider: llm.ProviderOpenAI,
		APIKey:   "invalid-key",
		Model:    "gpt-4",
	})
	if err != nil {
		logger.Error("Operation failed", "error", err)
		return
	}

	ctx := context.Background()
	err = client.ValidateConnection(ctx)
	if err != nil {
		// Error message will indicate authentication failure
		fmt.Printf("Validation failed: %v\n", err)
	}

	// Example: Missing configuration
	_, err = llm.NewOpenAIClient(llm.OpenAIConfig{
		Provider: llm.ProviderAzure,
		// Missing required Azure fields
	})
	if err != nil {
		fmt.Printf("Config error: %v\n", err)
	}
}

// Example_customConfiguration demonstrates advanced configuration
func Example_customConfiguration() {
	client, err := llm.NewOpenAIClient(llm.OpenAIConfig{
		Provider:    llm.ProviderOpenAI,
		APIKey:      os.Getenv("OPENAI_API_KEY"),
		Model:       "gpt-4-turbo-preview",
		Temperature: 0.2, // More deterministic
		MaxTokens:   500, // Limit response length
	})
	if err != nil {
		logger.Error("Operation failed", "error", err)
		return
	}

	ctx := context.Background()
	messages := []openai.ChatCompletionMessage{
		{
			Role:    openai.ChatMessageRoleSystem,
			Content: "You are a concise assistant. Keep responses under 100 words.",
		},
		{
			Role:    openai.ChatMessageRoleUser,
			Content: "Explain quantum computing.",
		},
	}

	resp, err := client.CreateChatCompletion(ctx, messages)
	if err != nil {
		logger.Error("Operation failed", "error", err)
		return
	}

	fmt.Println(resp.Choices[0].Message.Content)
}

// Example_multiTurn demonstrates a multi-turn conversation
func Example_multiTurn() {
	client, err := llm.NewOpenAIClient(llm.OpenAIConfig{
		Provider: llm.ProviderOpenAI,
		Model:    "gpt-4",
	})
	if err != nil {
		logger.Error("Operation failed", "error", err)
		return
	}

	ctx := context.Background()

	// Start conversation
	messages := []openai.ChatCompletionMessage{
		{
			Role:    openai.ChatMessageRoleUser,
			Content: "What is the capital of France?",
		},
	}

	resp, err := client.CreateChatCompletion(ctx, messages)
	if err != nil {
		logger.Error("Operation failed", "error", err)
		return
	}

	// Add assistant's response to conversation
	messages = append(messages, openai.ChatCompletionMessage{
		Role:    openai.ChatMessageRoleAssistant,
		Content: resp.Choices[0].Message.Content,
	})

	// Continue conversation
	messages = append(messages, openai.ChatCompletionMessage{
		Role:    openai.ChatMessageRoleUser,
		Content: "What's the population?",
	})

	resp, err = client.CreateChatCompletion(ctx, messages)
	if err != nil {
		logger.Error("Operation failed", "error", err)
		return
	}

	fmt.Println(resp.Choices[0].Message.Content)
}
