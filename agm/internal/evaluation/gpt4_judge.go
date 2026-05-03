package evaluation

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// GPT4JudgeConfig holds configuration for the GPT-4 judge
type GPT4JudgeConfig struct {
	APIKey      string
	Model       string // e.g., "gpt-4-turbo-preview"
	Temperature float64
	MaxTokens   int
}

// DefaultGPT4Config returns default configuration
func DefaultGPT4Config(apiKey string) GPT4JudgeConfig {
	return GPT4JudgeConfig{
		APIKey:      apiKey,
		Model:       "gpt-4-turbo-preview",
		Temperature: 0.0, // Deterministic for evaluation
		MaxTokens:   1000,
	}
}

// GPT4Client interface for making API calls (for testing)
type GPT4Client interface {
	CreateChatCompletion(ctx context.Context, req GPT4Request) (*GPT4Response, error)
}

// HTTPClient wraps http.Client for testability
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// RealGPT4Client implements GPT4Client using OpenAI API
type RealGPT4Client struct {
	config     GPT4JudgeConfig
	httpClient HTTPClient
}

// NewRealGPT4Client creates a new OpenAI API client
func NewRealGPT4Client(config GPT4JudgeConfig) *RealGPT4Client {
	return &RealGPT4Client{
		config:     config,
		httpClient: &http.Client{},
	}
}

// GPT4Request represents an OpenAI API request
type GPT4Request struct {
	Model          string          `json:"model"`
	Messages       []GPT4Message   `json:"messages"`
	Temperature    float64         `json:"temperature"`
	MaxTokens      int             `json:"max_tokens"`
	ResponseFormat *ResponseFormat `json:"response_format,omitempty"`
}

// ResponseFormat specifies JSON schema for structured output
type ResponseFormat struct {
	Type       string      `json:"type"` // "json_schema"
	JSONSchema *JSONSchema `json:"json_schema,omitempty"`
}

// JSONSchema defines the structure of JSON output
type JSONSchema struct {
	Name   string                 `json:"name"`
	Strict bool                   `json:"strict"`
	Schema map[string]interface{} `json:"schema"`
}

// GPT4Message represents a message in the conversation
type GPT4Message struct {
	Role    string `json:"role"` // "system", "user", "assistant"
	Content string `json:"content"`
}

// GPT4Response represents an OpenAI API response
type GPT4Response struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	Model   string `json:"model"`
	Choices []struct {
		Index   int `json:"index"`
		Message struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

// JudgeOutput is the structured JSON output from the judge
type JudgeOutput struct {
	Pass      bool    `json:"pass"`
	Score     float64 `json:"score"`
	Reasoning string  `json:"reasoning"`
}

// CreateChatCompletion sends a request to OpenAI API
func (c *RealGPT4Client) CreateChatCompletion(ctx context.Context, req GPT4Request) (*GPT4Response, error) {
	reqBody, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.openai.com/v1/chat/completions", bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.config.APIKey)

	httpResp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(httpResp.Body) //nolint:errcheck // Error reading body is not critical for error reporting
		return nil, fmt.Errorf("API error (status %d): %s", httpResp.StatusCode, string(body))
	}

	var resp GPT4Response
	if err := json.NewDecoder(httpResp.Body).Decode(&resp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &resp, nil
}

// NewGPT4Judge creates a new GPT-4 judge with the given client
func NewGPT4Judge(client GPT4Client, config GPT4JudgeConfig) *GPT4Judge {
	return &GPT4Judge{
		client: client,
		config: config,
	}
}

// GPT4Judge implementation with actual client
type GPT4Judge struct {
	client GPT4Client
	config GPT4JudgeConfig
}

// EvaluateDetailed assesses output using GPT-4 with structured JSON output
func (g *GPT4Judge) EvaluateDetailed(ctx context.Context, input, expectedOutput string, criteria EvaluationCriteria) (*JudgeResponse, error) {
	tracer := otel.Tracer("engram/evaluation")
	ctx, span := tracer.Start(ctx, "judge_evaluation",
		trace.WithAttributes(
			attribute.String("judge.type", "gpt4"),
			attribute.String("judge.criteria", criteria.Name),
			attribute.Float64("judge.threshold", criteria.Threshold),
		))
	defer span.End()

	if g.client == nil {
		span.RecordError(fmt.Errorf("GPT-4 client not initialized"))
		return nil, fmt.Errorf("GPT-4 client not initialized")
	}

	// Build the evaluation prompt
	systemPrompt := "You are an expert evaluator. Assess the quality of AI outputs based on given criteria. Return your evaluation in JSON format."

	userPrompt := fmt.Sprintf(`Evaluate the following output:

Input: %s

Expected Output: %s

Actual Output: %s

Criteria: %s - %s
Threshold: %.2f

Provide your evaluation in JSON format with:
- pass: boolean (true if score >= threshold)
- score: float between 0.0 and 1.0
- reasoning: string explaining your judgment
`, input, expectedOutput, expectedOutput, criteria.Name, criteria.Description, criteria.Threshold)

	// Create JSON schema for structured output
	schema := &JSONSchema{
		Name:   "evaluation_response",
		Strict: true,
		Schema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"pass": map[string]interface{}{
					"type": "boolean",
				},
				"score": map[string]interface{}{
					"type": "number",
				},
				"reasoning": map[string]interface{}{
					"type": "string",
				},
			},
			"required":             []string{"pass", "score", "reasoning"},
			"additionalProperties": false,
		},
	}

	req := GPT4Request{
		Model: g.config.Model,
		Messages: []GPT4Message{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
		Temperature: g.config.Temperature,
		MaxTokens:   g.config.MaxTokens,
		ResponseFormat: &ResponseFormat{
			Type:       "json_schema",
			JSONSchema: schema,
		},
	}

	resp, err := g.client.CreateChatCompletion(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("API call failed: %w", err)
	}

	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("no response from API")
	}

	// Parse the JSON response
	var output JudgeOutput
	if err := json.Unmarshal([]byte(resp.Choices[0].Message.Content), &output); err != nil {
		return nil, fmt.Errorf("failed to parse JSON response: %w", err)
	}

	span.SetAttributes(
		attribute.Bool("judge.pass", output.Pass),
		attribute.Float64("judge.score", output.Score),
	)

	return &JudgeResponse{
		Pass:      output.Pass,
		Score:     output.Score,
		Reasoning: output.Reasoning,
	}, nil
}

// Evaluate implements the simple Judge interface
func (g *GPT4Judge) Evaluate(ctx context.Context, prompt string, response string) (float64, error) {
	criteria := EvaluationCriteria{
		Name:        "quality",
		Description: "Evaluate response quality",
		Threshold:   0.7,
	}
	result, err := g.EvaluateDetailed(ctx, prompt, response, criteria)
	if err != nil {
		return 0, err
	}
	return result.Score, nil
}
