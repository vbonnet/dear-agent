package provider

import (
	"context"
	"fmt"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/vbonnet/dear-agent/pkg/costtrack"
	"github.com/vbonnet/dear-agent/pkg/llm/auth"
	"github.com/vbonnet/dear-agent/pkg/promptcache"
)

// AnthropicProvider implements Provider for Anthropic Claude models.
type AnthropicProvider struct {
	client   anthropic.Client
	model    string
	costSink costtrack.CostSink
}

// AnthropicConfig contains configuration for Anthropic provider.
type AnthropicConfig struct {
	// Model is the Claude model identifier (e.g., "claude-3-5-sonnet-20241022")
	// If empty, defaults to claude-3-5-haiku-20241022
	Model string

	// CostSink is optional cost tracking sink
	// If nil, uses stdout sink as default
	CostSink costtrack.CostSink
}

// NewAnthropicProvider creates a new Anthropic provider.
//
// Authentication uses pkg/llm/auth hierarchy:
//   - Checks for ANTHROPIC_API_KEY environment variable
//   - Validates API key format
//
// Returns error if authentication fails.
func NewAnthropicProvider(config AnthropicConfig) (*AnthropicProvider, error) {
	// Get API key using auth package
	apiKey, err := auth.GetAPIKey("anthropic")
	if err != nil {
		return nil, NewProviderError("anthropic", "authenticate", err)
	}

	// Validate API key format
	if err := auth.ValidateAPIKey("anthropic", apiKey); err != nil {
		return nil, NewProviderError("anthropic", "authenticate", err)
	}

	// Create Anthropic client
	client := anthropic.NewClient(option.WithAPIKey(apiKey))

	// Use model from config, default to Claude 3.5 Haiku
	model := config.Model
	if model == "" {
		model = "claude-3-5-haiku-20241022"
	}

	// Use provided cost sink or default to stdout
	costSink := config.CostSink
	if costSink == nil {
		costSink = costtrack.NewStdoutSink()
	}

	return &AnthropicProvider{
		client:   client,
		model:    model,
		costSink: costSink,
	}, nil
}

// Name returns the provider identifier.
func (p *AnthropicProvider) Name() string {
	return "anthropic"
}

// Generate executes text generation with Anthropic Claude.
func (p *AnthropicProvider) Generate(ctx context.Context, req *GenerateRequest) (*GenerateResponse, error) {
	if req.Prompt == "" {
		return nil, NewProviderError("anthropic", "generate", fmt.Errorf("prompt cannot be empty"))
	}

	// Use request model or provider default
	model := req.Model
	if model == "" {
		model = p.model
	}

	// Build message parameters
	messages := []anthropic.MessageParam{
		anthropic.NewUserMessage(anthropic.NewTextBlock(req.Prompt)),
	}

	params := anthropic.MessageNewParams{
		Model:     model,
		MaxTokens: int64(req.MaxTokens),
		Messages:  messages,
	}

	// Add system prompt if provided, with prompt caching enabled
	if req.SystemPrompt != "" {
		cc := promptcache.GetCacheControl(promptcache.TierDefault)
		ttl := anthropic.CacheControlEphemeralTTLTTL5m
		if cc.TTL >= promptcache.TTL1Hour {
			ttl = anthropic.CacheControlEphemeralTTLTTL1h
		}
		params.System = []anthropic.TextBlockParam{
			{
				Text: req.SystemPrompt,
				CacheControl: anthropic.CacheControlEphemeralParam{
					TTL: ttl,
				},
			},
		}
	}

	// Set temperature if specified
	if req.Temperature > 0 {
		params.Temperature = anthropic.Float(req.Temperature)
	}

	// Call Anthropic API
	resp, err := p.client.Messages.New(ctx, params)
	if err != nil {
		return nil, NewProviderError("anthropic", "generate", err)
	}

	// Extract text from response
	var responseText string
	for _, block := range resp.Content {
		if block.Type == "text" {
			responseText += block.Text
		}
	}

	if responseText == "" {
		return nil, NewProviderError("anthropic", "generate", fmt.Errorf("empty response from API"))
	}

	// Calculate usage and cost
	usage := p.calculateUsage(resp)

	// Record cost if sink is configured
	if p.costSink != nil {
		if err := p.recordCost(ctx, model, resp, req.Metadata); err != nil {
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
			"anthropic_id":     string(resp.ID),
			"anthropic_role":   string(resp.Role),
			"stop_reason":      string(resp.StopReason),
			"input_tokens":     resp.Usage.InputTokens,
			"output_tokens":    resp.Usage.OutputTokens,
			"request_metadata": req.Metadata,
		},
	}, nil
}

// Capabilities returns provider capabilities.
func (p *AnthropicProvider) Capabilities() Capabilities {
	return Capabilities{
		SupportsCaching:       true,
		SupportsStreaming:     true,
		MaxTokensPerRequest:   200000, // Claude context window
		MaxConcurrentRequests: 5,      // Conservative rate limit
		SupportedModels: []string{
			"claude-opus-4-6",
			"claude-3-5-sonnet-20241022",
			"claude-3-5-haiku-20241022",
			"claude-3-haiku-20240307",
		},
	}
}

// calculateUsage extracts usage information from API response.
func (p *AnthropicProvider) calculateUsage(resp *anthropic.Message) Usage {
	inputTokens := int(resp.Usage.InputTokens)
	outputTokens := int(resp.Usage.OutputTokens)
	totalTokens := inputTokens + outputTokens

	// Get pricing for model
	pricing := costtrack.GetPricingOrDefault(p.model)

	// Calculate cost
	tokens := costtrack.Tokens{
		Input:  inputTokens,
		Output: outputTokens,
		// TODO: Add cache tokens when SDK supports them
		// CacheRead:  int(resp.Usage.CacheReadInputTokens),
		// CacheWrite: int(resp.Usage.CacheCreationInputTokens),
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
func (p *AnthropicProvider) recordCost(ctx context.Context, model string, resp *anthropic.Message, _ map[string]any) error {
	tokens := costtrack.Tokens{
		Input:  int(resp.Usage.InputTokens),
		Output: int(resp.Usage.OutputTokens),
		// TODO: Add cache tokens when SDK supports them
		CacheRead:  0,
		CacheWrite: 0,
	}

	// Get pricing for model
	pricing := costtrack.GetPricingOrDefault(model)

	// Calculate costs
	cost := costtrack.CalculateCost(tokens, pricing)

	// Calculate cache metrics
	cache := costtrack.CalculateCacheMetrics(tokens, cost)

	// Record cost
	costInfo := &costtrack.CostInfo{
		Provider: "anthropic",
		Model:    model,
		Tokens:   tokens,
		Cost:     cost,
		Cache:    cache,
	}

	costMetadata := &costtrack.CostMetadata{
		Operation: "generate",
		Timestamp: time.Now(),
		RequestID: string(resp.ID),
	}

	return p.costSink.Record(ctx, costInfo, costMetadata)
}
