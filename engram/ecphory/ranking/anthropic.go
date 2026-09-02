// Package ranking provides semantic ranking implementations for ecphory retrieval.
package ranking

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/vbonnet/dear-agent/pkg/costtrack"
	llmauth "github.com/vbonnet/dear-agent/pkg/llm/auth"
)

// AnthropicProvider implements semantic ranking using Anthropic Claude models
type AnthropicProvider struct {
	client   anthropic.Client
	model    string
	costSink costtrack.CostSink
}

// NewAnthropicProvider creates a new Anthropic provider using pkg/llm/auth
func NewAnthropicProvider(config AnthropicConfig) (Provider, error) {
	// Use pkg/llm/auth to get API key
	apiKey, err := llmauth.GetAPIKey("anthropic")
	if err != nil {
		return nil, fmt.Errorf("failed to get Anthropic API key: %w", err)
	}

	// Validate API key format
	if err := llmauth.ValidateAPIKey("anthropic", apiKey); err != nil {
		return nil, fmt.Errorf("invalid Anthropic API key: %w", err)
	}

	client := anthropic.NewClient(option.WithAPIKey(apiKey))

	// Use model from config, default to Claude 3.5 Haiku
	model := config.Model
	if model == "" {
		model = "claude-3-5-haiku-20241022"
	}

	// TODO: Cost sink should be passed from factory
	// For now, use stdout sink as default
	costSink := costtrack.NewStdoutSink()

	return &AnthropicProvider{
		client:   client, // anthropic.Client is a value type in v1.x
		model:    model,
		costSink: costSink,
	}, nil
}

// Name returns the provider name
func (p *AnthropicProvider) Name() string {
	return "anthropic"
}

// Model returns the model being used
func (p *AnthropicProvider) Model() string {
	return p.model
}

// Capabilities returns provider capabilities
func (p *AnthropicProvider) Capabilities() Capabilities {
	return Capabilities{
		SupportsCaching:          true,
		SupportsStructuredOutput: true,
		MaxConcurrentRequests:    5,      // Conservative rate limit
		MaxTokensPerRequest:      200000, // Claude context window
	}
}

// Rank ranks candidates using Anthropic Claude
func (p *AnthropicProvider) Rank(ctx context.Context, query string, candidates []Candidate) ([]RankedResult, error) {
	if len(candidates) == 0 {
		return []RankedResult{}, nil
	}

	// Call Anthropic API with structured output
	resp, err := p.client.Messages.New(ctx, buildClaudeRankingRequest(p.model, query, candidates))

	if err != nil {
		return nil, fmt.Errorf("failed to call Anthropic API: %w", err)
	}

	// Extract ranking results
	results, err := parseClaudeRankingResponse(resp, candidates)
	if err != nil {
		return nil, fmt.Errorf("failed to parse ranking response: %w", err)
	}

	// Track costs if sink is configured
	if p.costSink != nil {
		if err := p.recordCost(ctx, resp); err != nil {
			// Log error but don't fail the ranking
			fmt.Fprintf(os.Stderr, "Warning: failed to record cost: %v\n", err)
		}
	}

	return results, nil
}

// recordCost records API usage costs
func (p *AnthropicProvider) recordCost(ctx context.Context, resp *anthropic.Message) error {
	if resp == nil {
		return fmt.Errorf("nil response from API")
	}

	usage := resp.Usage

	tokens := costtrack.Tokens{
		Input:      int(usage.InputTokens),
		Output:     int(usage.OutputTokens),
		CacheRead:  int(usage.CacheReadInputTokens),
		CacheWrite: int(usage.CacheCreationInputTokens),
	}

	// Get pricing for model
	pricing := costtrack.GetPricingOrDefault(p.model)

	// Calculate costs
	cost := costtrack.CalculateCost(tokens, pricing)

	// Calculate cache metrics
	cache := costtrack.CalculateCacheMetrics(tokens, cost)

	// Record cost
	costInfo := &costtrack.CostInfo{
		Provider: "anthropic",
		Model:    p.model,
		Tokens:   tokens,
		Cost:     cost,
		Cache:    cache,
	}

	metadata := &costtrack.CostMetadata{
		Operation: "rank",
		Timestamp: time.Now(),
		RequestID: resp.ID,
	}

	return p.costSink.Record(ctx, costInfo, metadata)
}
