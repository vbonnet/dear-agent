package ranking

import (
	"context"
	"fmt"
	"os"
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
		return nil, fmt.Errorf("Vertex AI Claude only available in us-east5, got: %s", location) //nolint:staticcheck // proper noun
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

	// Call Vertex AI Claude API
	resp, err := p.client.Messages.New(ctx, buildClaudeRankingRequest(p.model, query, candidates))

	if err != nil {
		return nil, fmt.Errorf("failed to call Vertex AI Claude: %w", err)
	}

	// Parse response (same as Anthropic provider)
	results, err := parseClaudeRankingResponse(resp, candidates)
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

// recordCost records API usage costs
func (p *VertexAIClaudeProvider) recordCost(ctx context.Context, resp *anthropic.Message) error {
	if resp == nil {
		return fmt.Errorf("nil response from API")
	}

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
