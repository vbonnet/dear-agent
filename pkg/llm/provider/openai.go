package provider

import (
	"context"
	"fmt"
	"strings"
	"time"

	openai "github.com/sashabaranov/go-openai"
	"github.com/vbonnet/dear-agent/pkg/costtrack"
	"github.com/vbonnet/dear-agent/pkg/llm/auth"
)

// OpenAIProvider implements Provider for direct OpenAI ChatCompletion calls.
//
// This is the native OpenAI integration that complements the OpenRouter
// implementation in this package. Use OpenAIProvider when you want to call
// OpenAI directly (no extra hop, no OpenRouter pricing) and have an
// OPENAI_API_KEY available. Azure OpenAI lives in agm/internal/llm and is
// not handled here — it has a different env-var surface and deployment
// model.
type OpenAIProvider struct {
	client   openAIChatClient
	model    string
	costSink costtrack.CostSink
}

// openAIChatClient is the slice of go-openai's *openai.Client surface that
// the provider depends on. Defining it as an interface lets tests swap in
// a fake without standing up an HTTP server.
type openAIChatClient interface {
	CreateChatCompletion(ctx context.Context, req openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error)
}

// OpenAIConfig contains configuration for the native OpenAI provider.
type OpenAIConfig struct {
	// Model is the OpenAI model identifier (e.g. "gpt-4o", "gpt-4-turbo",
	// "gpt-5-pro"). If empty, defaults to "gpt-4o-mini" — the cheapest
	// general-purpose model in the GPT-4 family at the time of writing.
	Model string

	// CostSink is optional cost tracking sink. If nil, uses stdout sink.
	CostSink costtrack.CostSink

	// client is an internal seam used by tests. Production callers should
	// leave this nil; the constructor builds a real *openai.Client from
	// the API key returned by pkg/llm/auth.
	client openAIChatClient
}

// NewOpenAIProvider creates a native OpenAI provider.
//
// Authentication: reads OPENAI_API_KEY via pkg/llm/auth.GetAPIKey and
// validates the format (must start with "sk-"). Returns a ProviderError
// on any auth failure so callers can branch on it cleanly.
func NewOpenAIProvider(config OpenAIConfig) (*OpenAIProvider, error) {
	client := config.client
	if client == nil {
		apiKey, err := auth.GetAPIKey("openai")
		if err != nil {
			return nil, NewProviderError("openai", "authenticate", err)
		}
		if err := auth.ValidateAPIKey("openai", apiKey); err != nil {
			return nil, NewProviderError("openai", "authenticate", err)
		}
		client = openai.NewClient(apiKey)
	}

	model := config.Model
	if model == "" {
		model = "gpt-4o-mini"
	}

	costSink := config.CostSink
	if costSink == nil {
		costSink = costtrack.NewStdoutSink()
	}

	return &OpenAIProvider{
		client:   client,
		model:    model,
		costSink: costSink,
	}, nil
}

// Name returns the provider identifier.
func (p *OpenAIProvider) Name() string {
	return "openai"
}

// Generate executes a ChatCompletion call against OpenAI.
func (p *OpenAIProvider) Generate(ctx context.Context, req *GenerateRequest) (*GenerateResponse, error) {
	if req == nil {
		return nil, NewProviderError("openai", "generate", fmt.Errorf("request cannot be nil"))
	}
	if req.Prompt == "" {
		return nil, NewProviderError("openai", "generate", fmt.Errorf("prompt cannot be empty"))
	}

	model := req.Model
	if model == "" {
		model = p.model
	}

	messages := make([]openai.ChatCompletionMessage, 0, 2)
	if req.SystemPrompt != "" {
		messages = append(messages, openai.ChatCompletionMessage{
			Role:    openai.ChatMessageRoleSystem,
			Content: req.SystemPrompt,
		})
	}
	messages = append(messages, openai.ChatCompletionMessage{
		Role:    openai.ChatMessageRoleUser,
		Content: req.Prompt,
	})

	chatReq := openai.ChatCompletionRequest{
		Model:    model,
		Messages: messages,
	}
	if req.MaxTokens > 0 {
		// o1-series models reject MaxTokens; callers targeting those
		// should set the model accordingly. We use MaxCompletionTokens
		// which is accepted by both legacy and reasoning families.
		chatReq.MaxCompletionTokens = req.MaxTokens
	}
	if req.Temperature > 0 {
		chatReq.Temperature = float32(req.Temperature)
	}

	resp, err := p.client.CreateChatCompletion(ctx, chatReq)
	if err != nil {
		return nil, NewProviderError("openai", "generate", err)
	}

	if len(resp.Choices) == 0 {
		return nil, NewProviderError("openai", "generate", fmt.Errorf("empty choices in response"))
	}
	text := resp.Choices[0].Message.Content
	if text == "" {
		return nil, NewProviderError("openai", "generate", fmt.Errorf("empty response from API"))
	}

	usage := p.calculateUsage(model, resp.Usage)

	if p.costSink != nil {
		if recordErr := p.recordCost(ctx, model, resp); recordErr != nil {
			// Cost recording is best-effort; don't fail the call.
			fmt.Printf("Warning: failed to record cost: %v\n", recordErr)
		}
	}

	return &GenerateResponse{
		Text:  text,
		Model: model,
		Usage: usage,
		Metadata: map[string]any{
			"openai_id":        resp.ID,
			"finish_reason":    string(resp.Choices[0].FinishReason),
			"input_tokens":     resp.Usage.PromptTokens,
			"output_tokens":    resp.Usage.CompletionTokens,
			"request_metadata": req.Metadata,
		},
	}, nil
}

// Capabilities returns provider capabilities.
func (p *OpenAIProvider) Capabilities() Capabilities {
	return Capabilities{
		// Prompt caching on OpenAI is automatic for repeated prefixes;
		// we don't expose a per-call cache control yet, so report false.
		SupportsCaching:       false,
		SupportsStreaming:     true,
		MaxTokensPerRequest:   128000, // GPT-4o context window
		MaxConcurrentRequests: 10,
		SupportedModels: []string{
			"gpt-4o",
			"gpt-4o-mini",
			"gpt-4-turbo",
			"gpt-4",
			"o1",
			"o1-mini",
		},
	}
}

func (p *OpenAIProvider) calculateUsage(model string, u openai.Usage) Usage {
	tokens := costtrack.Tokens{
		Input:  u.PromptTokens,
		Output: u.CompletionTokens,
	}
	pricing := costtrack.GetPricingOrDefault(model)
	cost := costtrack.CalculateCost(tokens, pricing)
	return Usage{
		InputTokens:  u.PromptTokens,
		OutputTokens: u.CompletionTokens,
		TotalTokens:  u.TotalTokens,
		CostUSD:      cost.Total,
	}
}

func (p *OpenAIProvider) recordCost(ctx context.Context, model string, resp openai.ChatCompletionResponse) error {
	tokens := costtrack.Tokens{
		Input:  resp.Usage.PromptTokens,
		Output: resp.Usage.CompletionTokens,
	}
	pricing := costtrack.GetPricingOrDefault(model)
	cost := costtrack.CalculateCost(tokens, pricing)
	cache := costtrack.CalculateCacheMetrics(tokens, cost)

	info := &costtrack.CostInfo{
		Provider: "openai",
		Model:    model,
		Tokens:   tokens,
		Cost:     cost,
		Cache:    cache,
	}
	meta := &costtrack.CostMetadata{
		Operation: "generate",
		Timestamp: time.Now(),
		RequestID: resp.ID,
	}
	return p.costSink.Record(ctx, info, meta)
}

// looksLikeOpenAIModel reports whether a model id should route to the
// OpenAI provider family. It is consulted by Resolver — kept here so the
// list lives next to the provider that owns it.
func looksLikeOpenAIModel(id string) bool {
	id = strings.ToLower(id)
	switch {
	case strings.HasPrefix(id, "gpt-"):
		return true
	case strings.HasPrefix(id, "o1"), strings.HasPrefix(id, "o3"), strings.HasPrefix(id, "o4"):
		return true
	case id == "chatgpt-4o-latest":
		return true
	}
	return false
}
