// Package ranking provides semantic ranking implementations for ecphory retrieval.
package ranking

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
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

	// Build prompt for ranking
	prompt := p.buildRankingPrompt(query, candidates)

	// Call Anthropic API with structured output
	resp, err := p.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     p.model,
		MaxTokens: 4096,
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(prompt)),
		},
		System: []anthropic.TextBlockParam{
			{Text: systemPrompt},
		},
	})

	if err != nil {
		return nil, fmt.Errorf("failed to call Anthropic API: %w", err)
	}

	// Extract ranking results
	results, err := p.parseRankingResponse(resp, candidates)
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

// buildRankingPrompt builds the prompt for semantic ranking
func (p *AnthropicProvider) buildRankingPrompt(query string, candidates []Candidate) string {
	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("Query: %s\n\nCandidates:\n", query))

	for i, candidate := range candidates {
		builder.WriteString(fmt.Sprintf("%d. Name: %s\n", i, candidate.Name))
		if candidate.Description != "" {
			builder.WriteString(fmt.Sprintf("   Description: %s\n", candidate.Description))
		}
		if len(candidate.Tags) > 0 {
			builder.WriteString(fmt.Sprintf("   Tags: %v\n", candidate.Tags))
		}
		builder.WriteString("\n")
	}

	builder.WriteString("\nProvide a ranked list of candidate indices (0-indexed) with scores and reasoning.")
	return builder.String()
}

// parseRankingResponse parses the API response into ranked results
func (p *AnthropicProvider) parseRankingResponse(resp *anthropic.Message, candidates []Candidate) ([]RankedResult, error) {
	// Extract text from response
	var responseText string
	for _, block := range resp.Content {
		if block.Type == "text" {
			responseText += block.Text
		}
	}

	if responseText == "" {
		return nil, fmt.Errorf("empty response from API")
	}

	// Try to parse as JSON first (structured output)
	var structuredResults []struct {
		Index     int     `json:"index"`
		Score     float64 `json:"score"`
		Reasoning string  `json:"reasoning"`
	}

	if err := json.Unmarshal([]byte(responseText), &structuredResults); err == nil {
		// Structured JSON output
		results := make([]RankedResult, 0, len(structuredResults))
		for _, sr := range structuredResults {
			if sr.Index >= 0 && sr.Index < len(candidates) {
				results = append(results, RankedResult{
					Candidate: candidates[sr.Index],
					Score:     sr.Score,
					Reasoning: sr.Reasoning,
				})
			}
		}
		return results, nil
	}

	// Fallback: parse natural language response
	// For now, return all candidates with default scores
	// TODO: Implement natural language parsing if structured output fails
	results := make([]RankedResult, len(candidates))
	for i, candidate := range candidates {
		results[i] = RankedResult{
			Candidate: candidate,
			Score:     1.0 / float64(i+1), // Simple descending scores
			Reasoning: "Fallback ranking (structured output parsing failed)",
		}
	}

	return results, nil
}

// recordCost records API usage costs
func (p *AnthropicProvider) recordCost(ctx context.Context, resp *anthropic.Message) error {
	usage := resp.Usage

	// TODO: SDK doesn't expose cache fields yet - set to 0 for now
	// Will need to update when anthropic-sdk-go adds support for:
	// - usage.CacheCreationInputTokens
	// - usage.CacheReadInputTokens
	tokens := costtrack.Tokens{
		Input:      int(usage.InputTokens),
		Output:     int(usage.OutputTokens),
		CacheRead:  0, // TODO: int(usage.CacheReadInputTokens) when available
		CacheWrite: 0, // TODO: int(usage.CacheCreationInputTokens) when available
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

// systemPrompt defines the system instructions for ranking
const systemPrompt = `You are an expert at semantic ranking. Given a user query and a list of candidates, your task is to:

1. Analyze the semantic similarity between the query and each candidate
2. Consider the description and tags of each candidate
3. Rank the candidates by relevance to the query
4. Provide a score (0.0-1.0) and brief reasoning for each candidate

Return your response as a JSON array of objects, each containing:
- "index": the 0-based index of the candidate
- "score": a float between 0.0 (not relevant) and 1.0 (highly relevant)
- "reasoning": a brief explanation (1-2 sentences) of why this score was assigned

Example output format:
[
  {"index": 2, "score": 0.95, "reasoning": "Exact match on key concepts and tags"},
  {"index": 0, "score": 0.7, "reasoning": "Partial match on description"},
  {"index": 1, "score": 0.3, "reasoning": "Weak semantic connection"}
]`
