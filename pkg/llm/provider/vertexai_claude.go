package provider

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/vbonnet/dear-agent/pkg/costtrack"
	"github.com/vbonnet/dear-agent/pkg/llm/auth"
)

// VertexAIClaudeProvider implements Provider for Claude via Vertex AI.
type VertexAIClaudeProvider struct {
	client    anthropic.Client
	projectID string
	location  string
	model     string
	costSink  costtrack.CostSink
}

// VertexAIClaudeConfig contains configuration for Vertex AI Claude provider.
type VertexAIClaudeConfig struct {
	// ProjectID is the Google Cloud project ID
	// If empty, uses GOOGLE_CLOUD_PROJECT environment variable
	ProjectID string

	// Location is the Vertex AI region (must be "us-east5" for Claude)
	// If empty, defaults to "us-east5"
	Location string

	// Model is the Claude model identifier (e.g., "claude-sonnet-4-5@20250929")
	// If empty, defaults to claude-sonnet-4-5@20250929
	Model string

	// CostSink is optional cost tracking sink
	// If nil, uses stdout sink as default
	CostSink costtrack.CostSink
}

// NewVertexAIClaudeProvider creates a new Vertex AI Claude provider.
//
// Authentication uses Google Cloud Application Default Credentials (ADC).
// Requires GOOGLE_CLOUD_PROJECT environment variable.
//
// Claude is only available in us-east5 region.
//
// Returns error if authentication fails or location is invalid.
func NewVertexAIClaudeProvider(config VertexAIClaudeConfig) (*VertexAIClaudeProvider, error) {
	// Check Vertex AI authentication via auth package
	authMethod := auth.DetectAuthMethod("gemini") // Use gemini for Vertex AI check
	if authMethod != auth.AuthVertexAI {
		return nil, NewProviderError("vertexai-claude", "authenticate",
			//nolint:staticcheck // proper noun
			fmt.Errorf("Vertex AI authentication not available (need GOOGLE_CLOUD_PROJECT)"))
	}

	// Get project ID
	projectID := config.ProjectID
	if projectID == "" {
		projectID = os.Getenv("GOOGLE_CLOUD_PROJECT")
	}
	if projectID == "" {
		return nil, NewProviderError("vertexai-claude", "authenticate",
			fmt.Errorf("GOOGLE_CLOUD_PROJECT environment variable not set"))
	}

	// Validate location (Claude only in us-east5)
	location := config.Location
	if location == "" {
		location = "us-east5"
	}
	if location != "us-east5" {
		return nil, NewProviderError("vertexai-claude", "configure",
			fmt.Errorf("claude on Vertex AI only available in us-east5, got: %s", location))
	}

	// Default model
	model := config.Model
	if model == "" {
		model = "claude-sonnet-4-5@20250929"
	}

	// Create Anthropic client with Vertex AI endpoint
	baseURL := fmt.Sprintf("https://%s-aiplatform.googleapis.com/v1/projects/%s/locations/%s/publishers/anthropic/models/%s",
		location, projectID, location, model)

	client := anthropic.NewClient(
		option.WithBaseURL(baseURL),
		// Google Cloud authentication handled by ADC
	)

	// Use provided cost sink or default to stdout
	costSink := config.CostSink
	if costSink == nil {
		costSink = costtrack.NewStdoutSink()
	}

	return &VertexAIClaudeProvider{
		client:    client, // anthropic.Client is a value type in v1.x
		projectID: projectID,
		location:  location,
		model:     model,
		costSink:  costSink,
	}, nil
}

// Name returns the provider identifier.
func (p *VertexAIClaudeProvider) Name() string {
	return "vertexai-claude"
}

// Generate executes text generation with Vertex AI Claude.
func (p *VertexAIClaudeProvider) Generate(ctx context.Context, req *GenerateRequest) (*GenerateResponse, error) {
	if req.Prompt == "" {
		return nil, NewProviderError("vertexai-claude", "generate", fmt.Errorf("prompt cannot be empty"))
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

	// Add system prompt if provided
	if req.SystemPrompt != "" {
		params.System = []anthropic.TextBlockParam{
			{Text: req.SystemPrompt},
		}
	}

	// Set temperature if specified
	if req.Temperature > 0 {
		params.Temperature = anthropic.Float(req.Temperature)
	}

	// Call Vertex AI Claude API
	resp, err := p.client.Messages.New(ctx, params)
	if err != nil {
		return nil, NewProviderError("vertexai-claude", "generate", err)
	}

	// Extract text from response
	var responseText string
	for _, block := range resp.Content {
		if block.Type == "text" {
			responseText += block.Text
		}
	}

	if responseText == "" {
		return nil, NewProviderError("vertexai-claude", "generate", fmt.Errorf("empty response from API"))
	}

	// Calculate usage and cost
	usage := p.calculateUsage(resp)

	// Record cost if sink is configured
	if p.costSink != nil {
		if err := p.recordCost(ctx, model, resp); err != nil {
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
			"vertexai_project":  p.projectID,
			"vertexai_location": p.location,
			"anthropic_id":      string(resp.ID),
			"anthropic_role":    string(resp.Role),
			"stop_reason":       string(resp.StopReason),
			"input_tokens":      resp.Usage.InputTokens,
			"output_tokens":     resp.Usage.OutputTokens,
		},
	}, nil
}

// Capabilities returns provider capabilities.
func (p *VertexAIClaudeProvider) Capabilities() Capabilities {
	return Capabilities{
		SupportsCaching:       true,
		SupportsStreaming:     true,
		MaxTokensPerRequest:   200000, // Claude context window
		MaxConcurrentRequests: 5,      // Conservative rate limit
		SupportedModels: []string{
			"claude-sonnet-4-5@20250929",
			"claude-3-5-sonnet-v2@20241022",
			"claude-3-5-haiku@20241022",
			"claude-3-haiku@20240307",
		},
	}
}

// calculateUsage extracts usage information from API response.
func (p *VertexAIClaudeProvider) calculateUsage(resp *anthropic.Message) Usage {
	inputTokens := int(resp.Usage.InputTokens)
	outputTokens := int(resp.Usage.OutputTokens)
	totalTokens := inputTokens + outputTokens

	// Get pricing for model (use Vertex AI pricing)
	pricing := costtrack.GetPricingOrDefault(p.model)

	// Calculate cost
	tokens := costtrack.Tokens{
		Input:  inputTokens,
		Output: outputTokens,
		// TODO: Add cache tokens when SDK supports them
		CacheRead:  0,
		CacheWrite: 0,
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
func (p *VertexAIClaudeProvider) recordCost(ctx context.Context, model string, resp *anthropic.Message) error {
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
		Provider: "vertexai-claude",
		Model:    model,
		Tokens:   tokens,
		Cost:     cost,
		Cache:    cache,
	}

	costMetadata := &costtrack.CostMetadata{
		Operation: "generate",
		Timestamp: time.Now(),
		RequestID: string(resp.ID),
		Context:   fmt.Sprintf("project=%s,location=%s", p.projectID, p.location),
	}

	return p.costSink.Record(ctx, costInfo, costMetadata)
}
