package ranking

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	aiplatform "cloud.google.com/go/aiplatform/apiv1"
	"cloud.google.com/go/aiplatform/apiv1/aiplatformpb"
	"github.com/vbonnet/dear-agent/pkg/costtrack"
	"google.golang.org/api/option"
	"google.golang.org/protobuf/types/known/structpb"
)

// VertexAIGeminiProvider implements semantic ranking using Google Gemini models
type VertexAIGeminiProvider struct {
	client    *aiplatform.PredictionClient
	projectID string
	location  string
	model     string
	costSink  costtrack.CostSink
}

// NewVertexAIGeminiProvider creates a new Vertex AI Gemini provider
func NewVertexAIGeminiProvider(config VertexAIConfig) (Provider, error) {
	// Get project ID from environment
	projectIDEnv := config.ProjectIDEnv
	if projectIDEnv == "" {
		projectIDEnv = "GOOGLE_CLOUD_PROJECT"
	}

	projectID := os.Getenv(projectIDEnv)
	if projectID == "" {
		return nil, fmt.Errorf("%s environment variable not set", projectIDEnv)
	}

	// Default location
	location := config.Location
	if location == "" {
		location = "us-central1"
	}

	// Default model
	model := config.Model
	if model == "" {
		model = "gemini-2.0-flash-exp"
	}

	// Create Vertex AI client
	ctx := context.Background()
	client, err := aiplatform.NewPredictionClient(ctx, option.WithEndpoint(fmt.Sprintf("%s-aiplatform.googleapis.com:443", location)))
	if err != nil {
		return nil, fmt.Errorf("failed to create Vertex AI client: %w", err)
	}

	// TODO: Cost sink should be passed from factory
	costSink := costtrack.NewStdoutSink()

	return &VertexAIGeminiProvider{
		client:    client,
		projectID: projectID,
		location:  location,
		model:     model,
		costSink:  costSink,
	}, nil
}

// Name returns the provider name
func (p *VertexAIGeminiProvider) Name() string {
	return "vertexai-gemini"
}

// Model returns the model being used
func (p *VertexAIGeminiProvider) Model() string {
	return p.model
}

// Capabilities returns provider capabilities
func (p *VertexAIGeminiProvider) Capabilities() Capabilities {
	return Capabilities{
		SupportsCaching:          false, // Gemini doesn't support prompt caching yet
		SupportsStructuredOutput: true,  // JSON mode available
		MaxConcurrentRequests:    10,
		MaxTokensPerRequest:      1000000, // 1M token context window
	}
}

// Rank ranks candidates using Vertex AI Gemini
func (p *VertexAIGeminiProvider) Rank(ctx context.Context, query string, candidates []Candidate) ([]RankedResult, error) {
	if len(candidates) == 0 {
		return []RankedResult{}, nil
	}

	// Build prompt
	prompt := p.buildRankingPrompt(query, candidates)

	// Create prediction request
	endpoint := fmt.Sprintf("projects/%s/locations/%s/publishers/google/models/%s",
		p.projectID, p.location, p.model)

	// Build request instances
	promptValue, err := structpb.NewValue(map[string]any{
		"content": prompt,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create prompt value: %w", err)
	}

	req := &aiplatformpb.PredictRequest{
		Endpoint:  endpoint,
		Instances: []*structpb.Value{promptValue},
		Parameters: &structpb.Value{
			Kind: &structpb.Value_StructValue{
				StructValue: &structpb.Struct{
					Fields: map[string]*structpb.Value{
						"temperature": {
							Kind: &structpb.Value_NumberValue{NumberValue: 0.2},
						},
						"maxOutputTokens": {
							Kind: &structpb.Value_NumberValue{NumberValue: 2048},
						},
					},
				},
			},
		},
	}

	// Call Vertex AI
	resp, err := p.client.Predict(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to call Vertex AI: %w", err)
	}

	// Parse response
	results, err := p.parseRankingResponse(resp, candidates)
	if err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	// Track costs if sink is configured
	if p.costSink != nil {
		if err := p.recordCost(ctx, resp); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to record cost: %v\n", err)
		}
	}

	return results, nil
}

// buildRankingPrompt builds the prompt for semantic ranking
func (p *VertexAIGeminiProvider) buildRankingPrompt(query string, candidates []Candidate) string {
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

	builder.WriteString("\nYou are an expert at semantic ranking. Analyze the query and rank candidates by relevance.\n")
	builder.WriteString("Return a JSON array with objects containing: index (0-based), score (0.0-1.0), reasoning.\n")
	builder.WriteString("Example: [{\"index\": 0, \"score\": 0.95, \"reasoning\": \"Exact match\"}]")

	return builder.String()
}

// parseRankingResponse parses the Vertex AI response
func (p *VertexAIGeminiProvider) parseRankingResponse(resp *aiplatformpb.PredictResponse, candidates []Candidate) ([]RankedResult, error) {
	if len(resp.Predictions) == 0 {
		return nil, fmt.Errorf("empty response from Vertex AI")
	}

	// Extract text from first prediction
	prediction := resp.Predictions[0]
	predictionMap := prediction.GetStructValue().AsMap()

	// Gemini response structure: predictions[0].content or predictions[0].text
	var responseText string
	if content, ok := predictionMap["content"].(string); ok {
		responseText = content
	} else if text, ok := predictionMap["text"].(string); ok {
		responseText = text
	} else {
		return nil, fmt.Errorf("unable to extract text from response")
	}

	// Parse JSON
	var structuredResults []struct {
		Index     int     `json:"index"`
		Score     float64 `json:"score"`
		Reasoning string  `json:"reasoning"`
	}

	if err := json.Unmarshal([]byte(responseText), &structuredResults); err != nil {
		// Fallback: return all candidates with default scores
		results := make([]RankedResult, len(candidates))
		for i, candidate := range candidates {
			results[i] = RankedResult{
				Candidate: candidate,
				Score:     1.0 / float64(i+1),
				Reasoning: "Fallback ranking (JSON parsing failed)",
			}
		}
		return results, nil
	}

	// Build results
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

// recordCost records API usage costs
func (p *VertexAIGeminiProvider) recordCost(ctx context.Context, resp *aiplatformpb.PredictResponse) error {
	_ = resp // TODO: Extract token usage from resp.Metadata when available

	// TODO: Extract token usage from metadata
	// Vertex AI doesn't always provide token counts in response
	// For now, estimate or set to zero

	tokens := costtrack.Tokens{
		Input:  0, // TODO: extract from metadata if available
		Output: 0, // TODO: extract from metadata if available
	}

	// Get pricing
	pricing := costtrack.GetPricingOrDefault(p.model)

	// Calculate costs
	cost := costtrack.CalculateCost(tokens, pricing)

	// No caching for Gemini
	var cache *costtrack.Cache

	// Record cost
	costInfo := &costtrack.CostInfo{
		Provider: "vertexai-gemini",
		Model:    p.model,
		Tokens:   tokens,
		Cost:     cost,
		Cache:    cache,
	}

	metadata := &costtrack.CostMetadata{
		Operation: "rank",
		Timestamp: time.Now(),
		RequestID: "", // TODO: extract request ID if available
	}

	return p.costSink.Record(ctx, costInfo, metadata)
}
