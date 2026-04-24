package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/vbonnet/dear-agent/pkg/costtrack"
)

const (
	// defaultOllamaEndpoint is the default Ollama API endpoint.
	defaultOllamaEndpoint = "http://localhost:11434"

	// defaultOllamaModel is the default model if none is specified.
	defaultOllamaModel = "llama3.2"

	// ollamaEnvEndpoint is the environment variable for the Ollama endpoint.
	ollamaEnvEndpoint = "OLLAMA_HOST"
)

// OllamaProvider implements Provider for local Ollama models.
//
// Ollama runs locally and requires no authentication. The endpoint
// defaults to http://localhost:11434 but can be overridden via the
// OLLAMA_HOST environment variable or OllamaConfig.Endpoint.
type OllamaProvider struct {
	endpoint string
	client   *http.Client
	model    string
	costSink costtrack.CostSink
}

// OllamaConfig contains configuration for Ollama provider.
type OllamaConfig struct {
	// Endpoint is the Ollama API base URL.
	// If empty, uses OLLAMA_HOST env var, then http://localhost:11434.
	Endpoint string

	// Model is the Ollama model identifier (e.g., "llama3.2", "mistral", "codellama").
	// If empty, defaults to "llama3.2".
	Model string

	// CostSink is optional cost tracking sink.
	// If nil, uses stdout sink as default.
	CostSink costtrack.CostSink
}

// NewOllamaProvider creates a new Ollama provider.
//
// No authentication is required. The endpoint is resolved in this order:
//  1. OllamaConfig.Endpoint (if non-empty)
//  2. OLLAMA_HOST environment variable (if set)
//  3. http://localhost:11434 (default)
//
// Returns error if the endpoint is invalid.
func NewOllamaProvider(config OllamaConfig) (*OllamaProvider, error) {
	// Resolve endpoint: config > env > default
	endpoint := config.Endpoint
	if endpoint == "" {
		endpoint = os.Getenv(ollamaEnvEndpoint)
	}
	if endpoint == "" {
		endpoint = defaultOllamaEndpoint
	}

	// Use model from config, fall back to default
	model := config.Model
	if model == "" {
		model = defaultOllamaModel
	}

	// Use provided cost sink or default to stdout
	costSink := config.CostSink
	if costSink == nil {
		costSink = costtrack.NewStdoutSink()
	}

	return &OllamaProvider{
		endpoint: endpoint,
		client:   &http.Client{Timeout: 300 * time.Second}, // Local inference can be slow
		model:    model,
		costSink: costSink,
	}, nil
}

// Name returns the provider identifier.
func (p *OllamaProvider) Name() string {
	return "ollama"
}

// Generate executes text generation with a local Ollama model.
func (p *OllamaProvider) Generate(ctx context.Context, req *GenerateRequest) (*GenerateResponse, error) {
	if req.Prompt == "" {
		return nil, NewProviderError("ollama", "generate", fmt.Errorf("prompt cannot be empty"))
	}

	// Use request model or provider default
	model := req.Model
	if model == "" {
		model = p.model
	}

	ollamaResp, err := p.callAPI(ctx, p.buildChatRequest(model, req))
	if err != nil {
		return nil, err
	}

	responseText := ollamaResp.Message.Content
	if responseText == "" {
		return nil, NewProviderError("ollama", "generate", fmt.Errorf("empty response from API"))
	}

	usage := p.calculateUsage(ollamaResp)

	if p.costSink != nil {
		if err := p.recordCost(ctx, model, ollamaResp, req.Metadata); err != nil {
			fmt.Printf("Warning: failed to record cost: %v\n", err)
		}
	}

	return &GenerateResponse{
		Text:  responseText,
		Model: ollamaResp.Model,
		Usage: usage,
		Metadata: map[string]any{
			"done":             ollamaResp.Done,
			"done_reason":      ollamaResp.DoneReason,
			"input_tokens":     ollamaResp.PromptEvalCount,
			"output_tokens":    ollamaResp.EvalCount,
			"request_metadata": req.Metadata,
		},
	}, nil
}

// buildChatRequest constructs an ollamaChatRequest from a GenerateRequest.
func (p *OllamaProvider) buildChatRequest(model string, req *GenerateRequest) ollamaChatRequest {
	messages := []ollamaMessage{}

	if req.SystemPrompt != "" {
		messages = append(messages, ollamaMessage{Role: "system", Content: req.SystemPrompt})
	}
	messages = append(messages, ollamaMessage{Role: "user", Content: req.Prompt})

	chatReq := ollamaChatRequest{
		Model:    model,
		Messages: messages,
		Stream:   false,
	}

	if req.MaxTokens > 0 || req.Temperature > 0 {
		opts := ollamaOptions{}
		if req.MaxTokens > 0 {
			opts.NumPredict = req.MaxTokens
		}
		if req.Temperature > 0 {
			opts.Temperature = req.Temperature
		}
		chatReq.Options = &opts
	}

	return chatReq
}

// callAPI sends the chat request to the Ollama API and returns the parsed response.
func (p *OllamaProvider) callAPI(ctx context.Context, chatReq ollamaChatRequest) (ollamaChatResponse, error) {
	reqBody, err := json.Marshal(chatReq)
	if err != nil {
		return ollamaChatResponse{}, NewProviderError("ollama", "generate", fmt.Errorf("failed to marshal request: %w", err))
	}

	url := p.endpoint + "/api/chat"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(reqBody))
	if err != nil {
		return ollamaChatResponse{}, NewProviderError("ollama", "generate", fmt.Errorf("failed to create HTTP request: %w", err))
	}
	httpReq.Header.Set("Content-Type", "application/json")

	httpResp, err := p.client.Do(httpReq) //nolint:gosec // endpoint is user-configured, SSRF is intentional
	if err != nil {
		return ollamaChatResponse{}, NewProviderError("ollama", "generate", fmt.Errorf("HTTP request failed: %w", err))
	}
	defer httpResp.Body.Close()

	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return ollamaChatResponse{}, NewProviderError("ollama", "generate", fmt.Errorf("failed to read response: %w", err))
	}

	if httpResp.StatusCode != http.StatusOK {
		return ollamaChatResponse{}, NewProviderError("ollama", "generate", fmt.Errorf("HTTP %d: %s", httpResp.StatusCode, string(respBody)))
	}

	var resp ollamaChatResponse
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return ollamaChatResponse{}, NewProviderError("ollama", "generate", fmt.Errorf("failed to unmarshal response: %w", err))
	}

	return resp, nil
}

// Capabilities returns provider capabilities.
func (p *OllamaProvider) Capabilities() Capabilities {
	return Capabilities{
		SupportsCaching:       false, // Ollama does not support prompt caching
		SupportsStreaming:     true,
		MaxTokensPerRequest:   131072, // Common context window for modern local models
		MaxConcurrentRequests: 1,      // Local inference is typically single-threaded
		SupportedModels: []string{
			"llama3.2",
			"llama3.1",
			"mistral",
			"codellama",
			"phi3",
			"gemma2",
		},
	}
}

// calculateUsage extracts token counts from the Ollama response.
// Cost is always $0 for local inference.
func (p *OllamaProvider) calculateUsage(resp ollamaChatResponse) Usage {
	inputTokens := resp.PromptEvalCount
	outputTokens := resp.EvalCount
	totalTokens := inputTokens + outputTokens

	return Usage{
		InputTokens:  inputTokens,
		OutputTokens: outputTokens,
		TotalTokens:  totalTokens,
		CostUSD:      0, // Local inference is free
	}
}

// recordCost records zero-cost usage via the cost sink.
func (p *OllamaProvider) recordCost(ctx context.Context, model string, resp ollamaChatResponse, _ map[string]any) error {
	tokens := costtrack.Tokens{
		Input:  resp.PromptEvalCount,
		Output: resp.EvalCount,
	}

	// Local models have no cost — use zero pricing
	pricing := costtrack.Pricing{}
	cost := costtrack.CalculateCost(tokens, pricing)
	cache := costtrack.CalculateCacheMetrics(tokens, cost)

	costInfo := &costtrack.CostInfo{
		Provider: "ollama",
		Model:    model,
		Tokens:   tokens,
		Cost:     cost,
		Cache:    cache,
	}

	costMetadata := &costtrack.CostMetadata{
		Operation: "generate",
		Timestamp: time.Now(),
	}

	return p.costSink.Record(ctx, costInfo, costMetadata)
}

// ollamaChatRequest represents the Ollama /api/chat request body.
type ollamaChatRequest struct {
	Model    string          `json:"model"`
	Messages []ollamaMessage `json:"messages"`
	Stream   bool            `json:"stream"`
	Options  *ollamaOptions  `json:"options,omitempty"`
}

// ollamaMessage represents a single chat message.
type ollamaMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ollamaOptions contains generation parameters.
type ollamaOptions struct {
	NumPredict  int     `json:"num_predict,omitempty"`
	Temperature float64 `json:"temperature,omitempty"`
}

// ollamaChatResponse represents the Ollama /api/chat response body.
type ollamaChatResponse struct {
	Model           string        `json:"model"`
	Message         ollamaMessage `json:"message"`
	Done            bool          `json:"done"`
	DoneReason      string        `json:"done_reason"`
	PromptEvalCount int           `json:"prompt_eval_count"`
	EvalCount       int           `json:"eval_count"`
}
