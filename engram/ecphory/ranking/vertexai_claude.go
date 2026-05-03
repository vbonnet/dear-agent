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
)

// VertexAIClaudeProvider implements semantic ranking using Claude via Vertex AI
type VertexAIClaudeProvider struct {
	client    anthropic.Client
	projectID string
	location  string
	model     string
	costSink  costtrack.CostSink
}

// NewVertexAIClaudeProvider creates a new Vertex AI Claude provider
func NewVertexAIClaudeProvider(config VertexAIClaudeConfig) (Provider, error) {
	// Get project ID from environment
	projectIDEnv := config.ProjectIDEnv
	if projectIDEnv == "" {
		projectIDEnv = "GOOGLE_CLOUD_PROJECT"
	}

	projectID := os.Getenv(projectIDEnv)
	if projectID == "" {
		return nil, fmt.Errorf("%s environment variable not set", projectIDEnv)
	}

	// Validate location (Claude only in us-east5)
	location := config.Location
	if location == "" {
		location = "us-east5"
	}
	if location != "us-east5" {
		return nil, fmt.Errorf("vertex AI Claude only available in us-east5, got: %s", location)
	}

	// Default model
	model := config.Model
	if model == "" {
		model = "claude-sonnet-4-5@20250929"
	}

	// Create Anthropic client with Vertex AI endpoint
	baseURL := fmt.Sprintf("https://%s-aiplatform.googleapis.com/v1/projects/%s/locations/%s/publishers/anthropic/models/%s",
		location, projectID, location, model)

	// TODO: Vertex AI requires Google Cloud authentication
	// For now, this will fail at runtime without proper auth
	// Future: Add Google Cloud credential handling
	client := anthropic.NewClient(
		option.WithBaseURL(baseURL),
		// Google Cloud authentication should be handled by ADC
		// (Application Default Credentials)
	)

	// TODO: Cost sink should be passed from factory
	costSink := costtrack.NewStdoutSink()

	return &VertexAIClaudeProvider{
		client:    client, // anthropic.Client is a value type in v1.x
		projectID: projectID,
		location:  location,
		model:     model,
		costSink:  costSink,
	}, nil
}

// Name returns the provider name
func (p *VertexAIClaudeProvider) Name() string {
	return "vertexai-claude"
}

// Model returns the model being used
func (p *VertexAIClaudeProvider) Model() string {
	return p.model
}

// Capabilities returns provider capabilities
func (p *VertexAIClaudeProvider) Capabilities() Capabilities {
	return Capabilities{
		SupportsCaching:          true,   // Vertex AI Claude supports caching
		SupportsStructuredOutput: true,   // JSON mode available
		MaxConcurrentRequests:    5,      // Conservative rate limit
		MaxTokensPerRequest:      200000, // Claude context window
	}
}

// Rank ranks candidates using Vertex AI Claude
func (p *VertexAIClaudeProvider) Rank(ctx context.Context, query string, candidates []Candidate) ([]RankedResult, error) {
	if len(candidates) == 0 {
		return []RankedResult{}, nil
	}

	// Build prompt (same as Anthropic provider)
	prompt := p.buildRankingPrompt(query, candidates)

	// Call Vertex AI Claude API
	resp, err := p.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     p.model,
		MaxTokens: 4096,
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(prompt)),
		},
		System: []anthropic.TextBlockParam{
			{Text: systemPromptVertexClaude},
		},
	})

	if err != nil {
		return nil, fmt.Errorf("failed to call Vertex AI Claude: %w", err)
	}

	// Parse response (same as Anthropic provider)
	results, err := p.parseRankingResponse(resp, candidates)
	if err != nil {
		return nil, fmt.Errorf("failed to parse ranking response: %w", err)
	}

	// Track costs
	if p.costSink != nil {
		if err := p.recordCost(ctx, resp); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to record cost: %v\n", err)
		}
	}

	return results, nil
}

// buildRankingPrompt builds the prompt for semantic ranking
func (p *VertexAIClaudeProvider) buildRankingPrompt(query string, candidates []Candidate) string {
	var builder strings.Builder
	fmt.Fprintf(&builder, "Query: %s\n\nCandidates:\n", query)

	for i, candidate := range candidates {
		fmt.Fprintf(&builder, "%d. Name: %s\n", i, candidate.Name)
		if candidate.Description != "" {
			fmt.Fprintf(&builder, "   Description: %s\n", candidate.Description)
		}
		if len(candidate.Tags) > 0 {
			fmt.Fprintf(&builder, "   Tags: %v\n", candidate.Tags)
		}
		builder.WriteString("\n")
	}

	builder.WriteString("\nProvide a ranked list of candidate indices (0-indexed) with scores and reasoning.")
	return builder.String()
}

// parseRankingResponse parses the API response
func (p *VertexAIClaudeProvider) parseRankingResponse(resp *anthropic.Message, candidates []Candidate) ([]RankedResult, error) {
	var responseText string
	for _, block := range resp.Content {
		if block.Type == "text" {
			responseText += block.Text
		}
	}

	if responseText == "" {
		return nil, fmt.Errorf("empty response from API")
	}

	// Parse JSON
	var structuredResults []struct {
		Index     int     `json:"index"`
		Score     float64 `json:"score"`
		Reasoning string  `json:"reasoning"`
	}

	if err := json.Unmarshal([]byte(responseText), &structuredResults); err == nil {
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

	// Fallback
	results := make([]RankedResult, len(candidates))
	for i, candidate := range candidates {
		results[i] = RankedResult{
			Candidate: candidate,
			Score:     1.0 / float64(i+1),
			Reasoning: "Fallback ranking (structured output parsing failed)",
		}
	}

	return results, nil
}

// recordCost records API usage costs
func (p *VertexAIClaudeProvider) recordCost(ctx context.Context, resp *anthropic.Message) error {
	usage := resp.Usage

	tokens := costtrack.Tokens{
		Input:      int(usage.InputTokens),
		Output:     int(usage.OutputTokens),
		CacheRead:  0, // TODO: Vertex AI cache fields when available
		CacheWrite: 0, // TODO: Vertex AI cache fields when available
	}

	// Get pricing (use Vertex AI Claude pricing from table)
	pricing := costtrack.GetPricingOrDefault(p.model)

	// Calculate costs
	cost := costtrack.CalculateCost(tokens, pricing)
	cache := costtrack.CalculateCacheMetrics(tokens, cost)

	// Record cost
	costInfo := &costtrack.CostInfo{
		Provider: "vertexai-claude",
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

// systemPromptVertexClaude defines the system instructions
const systemPromptVertexClaude = `You are an expert at semantic ranking. Given a user query and a list of candidates, your task is to:

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
