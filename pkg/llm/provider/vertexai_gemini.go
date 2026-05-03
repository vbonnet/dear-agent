package provider

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
	"github.com/vbonnet/dear-agent/pkg/llm/auth"
	"google.golang.org/api/option"
	"google.golang.org/protobuf/types/known/structpb"
)

// VertexAIGeminiProvider implements Provider for Google Gemini via Vertex AI.
type VertexAIGeminiProvider struct {
	client    *aiplatform.PredictionClient
	projectID string
	location  string
	model     string
	costSink  costtrack.CostSink
}

// VertexAIGeminiConfig contains configuration for Vertex AI Gemini provider.
type VertexAIGeminiConfig struct {
	// ProjectID is the Google Cloud project ID
	// If empty, uses GOOGLE_CLOUD_PROJECT environment variable
	ProjectID string

	// Location is the Vertex AI region (default: "us-central1")
	Location string

	// Model is the Gemini model identifier (e.g., "gemini-2.0-flash-exp")
	// If empty, defaults to gemini-2.0-flash-exp
	Model string

	// CostSink is optional cost tracking sink
	// If nil, uses stdout sink as default
	CostSink costtrack.CostSink
}

// NewVertexAIGeminiProvider creates a new Vertex AI Gemini provider.
//
// Authentication uses Google Cloud Application Default Credentials (ADC).
// Requires GOOGLE_CLOUD_PROJECT environment variable.
//
// Supported models:
//   - gemini-2.0-flash-exp (default)
//   - gemini-2.5-pro-exp
//   - gemini-1.5-pro
//   - gemini-1.5-flash
//
// Returns error if authentication fails or configuration is invalid.
func NewVertexAIGeminiProvider(config VertexAIGeminiConfig) (*VertexAIGeminiProvider, error) {
	// Check Vertex AI authentication via auth package
	authMethod := auth.DetectAuthMethod("gemini")
	if authMethod != auth.AuthVertexAI {
		return nil, NewProviderError("vertexai-gemini", "authenticate",
			//nolint:staticcheck // proper noun
			fmt.Errorf("Vertex AI authentication not available (need GOOGLE_CLOUD_PROJECT)"))
	}

	// Get project ID
	projectID := config.ProjectID
	if projectID == "" {
		projectID = os.Getenv("GOOGLE_CLOUD_PROJECT")
	}
	if projectID == "" {
		return nil, NewProviderError("vertexai-gemini", "authenticate",
			fmt.Errorf("GOOGLE_CLOUD_PROJECT environment variable not set"))
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

	// Create Vertex AI Prediction client with regional endpoint
	ctx := context.Background()
	endpoint := fmt.Sprintf("%s-aiplatform.googleapis.com:443", location)
	client, err := aiplatform.NewPredictionClient(ctx, option.WithEndpoint(endpoint))
	if err != nil {
		return nil, NewProviderError("vertexai-gemini", "authenticate",
			fmt.Errorf("failed to create Vertex AI client: %w", err))
	}

	// Use provided cost sink or default to stdout
	costSink := config.CostSink
	if costSink == nil {
		costSink = costtrack.NewStdoutSink()
	}

	return &VertexAIGeminiProvider{
		client:    client,
		projectID: projectID,
		location:  location,
		model:     model,
		costSink:  costSink,
	}, nil
}

// Name returns the provider identifier.
func (p *VertexAIGeminiProvider) Name() string {
	return "vertexai-gemini"
}

// Generate executes text generation with Vertex AI Gemini.
func (p *VertexAIGeminiProvider) Generate(ctx context.Context, req *GenerateRequest) (*GenerateResponse, error) {
	if req.Prompt == "" {
		return nil, NewProviderError("vertexai-gemini", "generate", fmt.Errorf("prompt cannot be empty"))
	}

	// Use request model or provider default
	model := req.Model
	if model == "" {
		model = p.model
	}

	// Build the prompt text
	promptText := req.Prompt
	if req.SystemPrompt != "" {
		// Prepend system prompt to user prompt for Gemini
		promptText = req.SystemPrompt + "\n\n" + req.Prompt
	}

	// Create prediction request
	endpoint := fmt.Sprintf("projects/%s/locations/%s/publishers/google/models/%s",
		p.projectID, p.location, model)

	// Build request instance
	promptValue, err := structpb.NewValue(map[string]any{
		"content": promptText,
	})
	if err != nil {
		return nil, NewProviderError("vertexai-gemini", "generate",
			fmt.Errorf("failed to create prompt value: %w", err))
	}

	// Build parameters
	params := map[string]*structpb.Value{
		"maxOutputTokens": {
			Kind: &structpb.Value_NumberValue{NumberValue: float64(req.MaxTokens)},
		},
	}

	// Add temperature if specified
	if req.Temperature > 0 {
		params["temperature"] = &structpb.Value{
			Kind: &structpb.Value_NumberValue{NumberValue: req.Temperature},
		}
	}

	predictReq := &aiplatformpb.PredictRequest{
		Endpoint:  endpoint,
		Instances: []*structpb.Value{promptValue},
		Parameters: &structpb.Value{
			Kind: &structpb.Value_StructValue{
				StructValue: &structpb.Struct{
					Fields: params,
				},
			},
		},
	}

	// Call Vertex AI
	resp, err := p.client.Predict(ctx, predictReq)
	if err != nil {
		return nil, NewProviderError("vertexai-gemini", "generate", err)
	}

	// Parse response
	responseText, err := p.parseResponse(resp)
	if err != nil {
		return nil, NewProviderError("vertexai-gemini", "generate", err)
	}

	if responseText == "" {
		return nil, NewProviderError("vertexai-gemini", "generate", fmt.Errorf("empty response from API"))
	}

	// Calculate usage and cost
	usage := p.calculateUsage(resp, model)

	// Record cost if sink is configured
	if p.costSink != nil {
		if err := p.recordCost(ctx, model, resp, usage); err != nil {
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
		},
	}, nil
}

// Capabilities returns provider capabilities.
func (p *VertexAIGeminiProvider) Capabilities() Capabilities {
	return Capabilities{
		SupportsCaching:       false, // Gemini doesn't support prompt caching
		SupportsStreaming:     true,
		MaxTokensPerRequest:   1000000, // 1M token context window
		MaxConcurrentRequests: 10,
		SupportedModels: []string{
			"gemini-2.0-flash-exp",
			"gemini-2.5-pro-exp",
			"gemini-1.5-pro",
			"gemini-1.5-flash",
		},
	}
}

// parseResponse extracts text from Vertex AI Gemini response.
func (p *VertexAIGeminiProvider) parseResponse(resp *aiplatformpb.PredictResponse) (string, error) {
	if len(resp.Predictions) == 0 {
		return "", fmt.Errorf("empty response from Vertex AI")
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
	} else if candidates, ok := predictionMap["candidates"].([]any); ok && len(candidates) > 0 {
		// Try nested structure: predictions[0].candidates[0].content.parts[0].text
		if candidate, ok := candidates[0].(map[string]any); ok {
			if contentObj, ok := candidate["content"].(map[string]any); ok {
				if parts, ok := contentObj["parts"].([]any); ok && len(parts) > 0 {
					if part, ok := parts[0].(map[string]any); ok {
						if text, ok := part["text"].(string); ok {
							responseText = text
						}
					}
				}
			}
		}
	}

	if responseText == "" {
		// Debug: try to parse as JSON to understand structure
		jsonBytes, _ := json.MarshalIndent(predictionMap, "", "  ")
		return "", fmt.Errorf("unable to extract text from response. Structure: %s", string(jsonBytes))
	}

	return strings.TrimSpace(responseText), nil
}

// calculateUsage extracts usage information from API response.
func (p *VertexAIGeminiProvider) calculateUsage(resp *aiplatformpb.PredictResponse, model string) Usage {
	// Vertex AI Gemini may not always provide token counts in response
	// Try to extract from metadata if available
	inputTokens := 0
	outputTokens := 0

	if resp.Metadata != nil {
		metadataStruct := resp.Metadata.GetStructValue()
		if metadataStruct != nil {
			metadata := metadataStruct.AsMap()
			if usage, ok := metadata["usage"].(map[string]any); ok {
				if inputCount, ok := usage["promptTokenCount"].(float64); ok {
					inputTokens = int(inputCount)
				}
				if outputCount, ok := usage["candidatesTokenCount"].(float64); ok {
					outputTokens = int(outputCount)
				}
			}
		}
	}

	totalTokens := inputTokens + outputTokens

	// Get pricing for model
	pricing := costtrack.GetPricingOrDefault(model)

	// Calculate cost
	tokens := costtrack.Tokens{
		Input:      inputTokens,
		Output:     outputTokens,
		CacheRead:  0, // No caching support
		CacheWrite: 0, // No caching support
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
func (p *VertexAIGeminiProvider) recordCost(ctx context.Context, model string, resp *aiplatformpb.PredictResponse, usage Usage) error {
	tokens := costtrack.Tokens{
		Input:      usage.InputTokens,
		Output:     usage.OutputTokens,
		CacheRead:  0, // No caching
		CacheWrite: 0, // No caching
	}

	// Get pricing for model
	pricing := costtrack.GetPricingOrDefault(model)

	// Calculate costs
	cost := costtrack.CalculateCost(tokens, pricing)

	// No caching for Gemini
	var cache *costtrack.Cache

	// Record cost
	costInfo := &costtrack.CostInfo{
		Provider: "vertexai-gemini",
		Model:    model,
		Tokens:   tokens,
		Cost:     cost,
		Cache:    cache,
	}

	costMetadata := &costtrack.CostMetadata{
		Operation: "generate",
		Timestamp: time.Now(),
		RequestID: "", // Vertex AI doesn't provide request ID in response
		Context:   fmt.Sprintf("project=%s,location=%s", p.projectID, p.location),
	}

	return p.costSink.Record(ctx, costInfo, costMetadata)
}
