package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/vbonnet/dear-agent/pkg/costtrack"
	"github.com/vbonnet/dear-agent/pkg/llm/auth"
)

// OpenRouterProvider implements Provider for OpenRouter API.
//
// OpenRouter provides access to multiple LLM models through a unified
// OpenAI-compatible API endpoint.
type OpenRouterProvider struct {
	apiKey   string
	client   *http.Client
	model    string
	costSink costtrack.CostSink
}

// OpenRouterConfig contains configuration for OpenRouter provider.
type OpenRouterConfig struct {
	// Model is the OpenRouter model identifier (e.g., "anthropic/claude-3-5-sonnet")
	// If empty, defaults to "anthropic/claude-3-5-sonnet"
	Model string

	// CostSink is optional cost tracking sink
	// If nil, uses stdout sink as default
	CostSink costtrack.CostSink
}

// NewOpenRouterProvider creates a new OpenRouter provider.
//
// Authentication uses pkg/llm/auth hierarchy:
//   - Checks for OPENROUTER_API_KEY environment variable
//   - Validates API key format (must start with "sk-or-")
//
// Returns error if authentication fails.
func NewOpenRouterProvider(config OpenRouterConfig) (*OpenRouterProvider, error) {
	// Get API key using auth package
	apiKey, err := auth.GetAPIKey("openrouter")
	if err != nil {
		return nil, NewProviderError("openrouter", "authenticate", err)
	}

	// Validate API key format
	if err := auth.ValidateAPIKey("openrouter", apiKey); err != nil {
		return nil, NewProviderError("openrouter", "authenticate", err)
	}

	// Use model from config, default to Claude 3.5 Sonnet
	model := config.Model
	if model == "" {
		model = "anthropic/claude-3-5-sonnet"
	}

	// Use provided cost sink or default to stdout
	costSink := config.CostSink
	if costSink == nil {
		costSink = costtrack.NewStdoutSink()
	}

	return &OpenRouterProvider{
		apiKey:   apiKey,
		client:   &http.Client{Timeout: 120 * time.Second},
		model:    model,
		costSink: costSink,
	}, nil
}

// Name returns the provider identifier.
func (p *OpenRouterProvider) Name() string {
	return "openrouter"
}

// Generate executes text generation with OpenRouter.
func (p *OpenRouterProvider) Generate(ctx context.Context, req *GenerateRequest) (*GenerateResponse, error) {
	if req.Prompt == "" {
		return nil, NewProviderError("openrouter", "generate", fmt.Errorf("prompt cannot be empty"))
	}

	// Use request model or provider default
	model := req.Model
	if model == "" {
		model = p.model
	}

	// Build OpenAI-compatible request
	openRouterReq := openRouterRequest{
		Model:       model,
		MaxTokens:   req.MaxTokens,
		Temperature: req.Temperature,
		Messages:    []openRouterMessage{},
	}

	// Add system message if provided
	if req.SystemPrompt != "" {
		openRouterReq.Messages = append(openRouterReq.Messages, openRouterMessage{
			Role:    "system",
			Content: req.SystemPrompt,
		})
	}

	// Add user message
	openRouterReq.Messages = append(openRouterReq.Messages, openRouterMessage{
		Role:    "user",
		Content: req.Prompt,
	})

	// Marshal request to JSON
	reqBody, err := json.Marshal(openRouterReq)
	if err != nil {
		return nil, NewProviderError("openrouter", "generate", fmt.Errorf("failed to marshal request: %w", err))
	}

	// Create HTTP request
	httpReq, err := http.NewRequestWithContext(ctx, "POST", "https://openrouter.ai/api/v1/chat/completions", bytes.NewReader(reqBody))
	if err != nil {
		return nil, NewProviderError("openrouter", "generate", fmt.Errorf("failed to create HTTP request: %w", err))
	}

	// Set headers
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
	httpReq.Header.Set("HTTP-Referer", "https://github.com/vbonnet/dear-agent")
	httpReq.Header.Set("X-Title", "Engram LLM Agent")

	// Execute request
	httpResp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, NewProviderError("openrouter", "generate", fmt.Errorf("HTTP request failed: %w", err))
	}
	defer httpResp.Body.Close()

	// Read response body
	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, NewProviderError("openrouter", "generate", fmt.Errorf("failed to read response: %w", err))
	}

	// Check for HTTP error
	if httpResp.StatusCode != http.StatusOK {
		return nil, NewProviderError("openrouter", "generate", fmt.Errorf("HTTP %d: %s", httpResp.StatusCode, string(respBody)))
	}

	// Parse response
	var openRouterResp openRouterResponse
	if err := json.Unmarshal(respBody, &openRouterResp); err != nil {
		return nil, NewProviderError("openrouter", "generate", fmt.Errorf("failed to unmarshal response: %w", err))
	}

	// Extract text from response
	if len(openRouterResp.Choices) == 0 {
		return nil, NewProviderError("openrouter", "generate", fmt.Errorf("no choices in response"))
	}

	responseText := openRouterResp.Choices[0].Message.Content
	if responseText == "" {
		return nil, NewProviderError("openrouter", "generate", fmt.Errorf("empty response from API"))
	}

	// Calculate usage and cost
	usage := p.calculateUsage(model, openRouterResp.Usage)

	// Record cost if sink is configured
	if p.costSink != nil {
		if err := p.recordCost(ctx, model, openRouterResp, req.Metadata); err != nil {
			// Log warning but don't fail the request
			fmt.Printf("Warning: failed to record cost: %v\n", err)
		}
	}

	// Build response
	return &GenerateResponse{
		Text:  responseText,
		Model: model,
		Usage: usage,
		Metadata: map[string]any{
			"openrouter_id":    openRouterResp.ID,
			"finish_reason":    openRouterResp.Choices[0].FinishReason,
			"input_tokens":     openRouterResp.Usage.PromptTokens,
			"output_tokens":    openRouterResp.Usage.CompletionTokens,
			"request_metadata": req.Metadata,
		},
	}, nil
}

// Capabilities returns provider capabilities.
func (p *OpenRouterProvider) Capabilities() Capabilities {
	return Capabilities{
		SupportsCaching:       false, // OpenRouter doesn't support prompt caching
		SupportsStreaming:     true,
		MaxTokensPerRequest:   200000, // Depends on underlying model
		MaxConcurrentRequests: 20,     // OpenRouter has higher rate limits
		SupportedModels: []string{
			"anthropic/claude-3-5-sonnet",
			"anthropic/claude-3-5-haiku",
			"anthropic/claude-opus-4",
			"openai/gpt-4",
			"openai/gpt-4-turbo",
			"google/gemini-pro-1.5",
		},
	}
}

// calculateUsage extracts usage information from API response and calculates cost.
func (p *OpenRouterProvider) calculateUsage(model string, usage openRouterUsage) Usage {
	inputTokens := usage.PromptTokens
	outputTokens := usage.CompletionTokens
	totalTokens := usage.TotalTokens

	// Get pricing for model
	pricing := costtrack.GetPricingOrDefault(model)

	// Calculate cost
	tokens := costtrack.Tokens{
		Input:  inputTokens,
		Output: outputTokens,
	}

	cost := costtrack.CalculateCost(tokens, pricing)

	return Usage{
		InputTokens:  inputTokens,
		OutputTokens: outputTokens,
		TotalTokens:  totalTokens,
		CostUSD:      cost.Total,
	}
}

// recordCost records API usage costs via cost sink.
func (p *OpenRouterProvider) recordCost(ctx context.Context, model string, resp openRouterResponse, metadata map[string]any) error {
	tokens := costtrack.Tokens{
		Input:  resp.Usage.PromptTokens,
		Output: resp.Usage.CompletionTokens,
	}

	// Get pricing for model
	pricing := costtrack.GetPricingOrDefault(model)

	// Calculate costs
	cost := costtrack.CalculateCost(tokens, pricing)

	// Calculate cache metrics (no caching support, so zeros)
	cache := costtrack.CalculateCacheMetrics(tokens, cost)

	// Record cost
	costInfo := &costtrack.CostInfo{
		Provider: "openrouter",
		Model:    model,
		Tokens:   tokens,
		Cost:     cost,
		Cache:    cache,
	}

	costMetadata := &costtrack.CostMetadata{
		Operation: "generate",
		Timestamp: time.Now(),
		RequestID: resp.ID,
	}

	return p.costSink.Record(ctx, costInfo, costMetadata)
}

// openRouterRequest represents the OpenAI-compatible request format for OpenRouter.
type openRouterRequest struct {
	Model       string              `json:"model"`
	Messages    []openRouterMessage `json:"messages"`
	MaxTokens   int                 `json:"max_tokens,omitempty"`
	Temperature float64             `json:"temperature,omitempty"`
}

// openRouterMessage represents a message in the OpenRouter request/response.
type openRouterMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// openRouterResponse represents the OpenAI-compatible response from OpenRouter.
type openRouterResponse struct {
	ID      string             `json:"id"`
	Choices []openRouterChoice `json:"choices"`
	Usage   openRouterUsage    `json:"usage"`
}

// openRouterChoice represents a completion choice in the response.
type openRouterChoice struct {
	Message      openRouterMessage `json:"message"`
	FinishReason string            `json:"finish_reason"`
}

// openRouterUsage represents token usage information.
type openRouterUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}
