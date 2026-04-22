// Package evaluation provides LLM-based evaluation tools for AI agents.
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

// ClaudeJudgeConfig holds configuration for the Claude judge
type ClaudeJudgeConfig struct {
	APIKey      string
	Model       string // e.g., "claude-3-5-sonnet-20241022"
	MaxTokens   int
	Temperature float64
}

// DefaultClaudeConfig returns default configuration
func DefaultClaudeConfig(apiKey string) ClaudeJudgeConfig {
	return ClaudeJudgeConfig{
		APIKey:      apiKey,
		Model:       "claude-3-5-sonnet-20241022",
		MaxTokens:   2048,
		Temperature: 0.0, // Deterministic for evaluation
	}
}

// ClaudeClient interface for making API calls (for testing)
type ClaudeClient interface {
	CreateMessage(ctx context.Context, req ClaudeRequest) (*ClaudeResponse, error)
}

// RealClaudeClient implements ClaudeClient using Anthropic API
type RealClaudeClient struct {
	config     ClaudeJudgeConfig
	httpClient HTTPClient
}

// NewRealClaudeClient creates a new Anthropic API client
func NewRealClaudeClient(config ClaudeJudgeConfig) *RealClaudeClient {
	return &RealClaudeClient{
		config:     config,
		httpClient: &http.Client{},
	}
}

// ClaudeRequest represents an Anthropic API request
type ClaudeRequest struct {
	Model       string          `json:"model"`
	MaxTokens   int             `json:"max_tokens"`
	Temperature float64         `json:"temperature,omitempty"`
	Messages    []ClaudeMessage `json:"messages"`
	System      string          `json:"system,omitempty"`
	Tools       []ClaudeTool    `json:"tools,omitempty"`
}

// ClaudeMessage represents a message in the conversation
type ClaudeMessage struct {
	Role    string `json:"role"` // "user" or "assistant"
	Content string `json:"content"`
}

// ClaudeTool defines a tool/function Claude can use
type ClaudeTool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"input_schema"`
}

// ClaudeResponse represents an Anthropic API response
type ClaudeResponse struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Role    string `json:"role"`
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text,omitempty"`
	} `json:"content"`
	Model        string `json:"model"`
	StopReason   string `json:"stop_reason"`
	StopSequence string `json:"stop_sequence,omitempty"`
	Usage        struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

// ClaudeJudgeOutput is the structured JSON output with Chain of Thought
type ClaudeJudgeOutput struct {
	ChainOfThought string  `json:"chain_of_thought"` // CoT reasoning process
	Pass           bool    `json:"pass"`
	Score          float64 `json:"score"`
	Reasoning      string  `json:"reasoning"` // Final reasoning summary
}

// CreateMessage sends a request to Anthropic API
func (c *RealClaudeClient) CreateMessage(ctx context.Context, req ClaudeRequest) (*ClaudeResponse, error) {
	reqBody, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", "https://api.anthropic.com/v1/messages", bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", c.config.APIKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")

	httpResp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(httpResp.Body) //nolint:errcheck // Error reading body is not critical for error reporting
		return nil, fmt.Errorf("API error (status %d): %s", httpResp.StatusCode, string(body))
	}

	var resp ClaudeResponse
	if err := json.NewDecoder(httpResp.Body).Decode(&resp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &resp, nil
}

// NewClaudeJudge creates a new Claude judge with the given client
func NewClaudeJudge(client ClaudeClient, config ClaudeJudgeConfig) *ClaudeJudge {
	return &ClaudeJudge{
		client: client,
		config: config,
	}
}

// ClaudeJudge implementation with actual client
type ClaudeJudge struct {
	client ClaudeClient
	config ClaudeJudgeConfig
}

// EvaluateDetailed assesses output using Claude with Chain of Thought reasoning
func (c *ClaudeJudge) EvaluateDetailed(ctx context.Context, input, expectedOutput string, criteria EvaluationCriteria) (*JudgeResponse, error) {
	tracer := otel.Tracer("engram/evaluation")
	ctx, span := tracer.Start(ctx, "judge_evaluation",
		trace.WithAttributes(
			attribute.String("judge.type", "claude"),
			attribute.String("judge.criteria", criteria.Name),
			attribute.Float64("judge.threshold", criteria.Threshold),
		))
	defer span.End()

	if c.client == nil {
		span.RecordError(fmt.Errorf("claude client not initialized"))
		return nil, fmt.Errorf("claude client not initialized")
	}

	// System prompt emphasizing Chain of Thought
	systemPrompt := `You are an expert evaluator. Use Chain of Thought reasoning to assess AI outputs.

Your response MUST be valid JSON with this exact structure:
{
  "chain_of_thought": "Your detailed step-by-step reasoning process",
  "pass": true or false,
  "score": 0.0 to 1.0,
  "reasoning": "Final summary of your judgment"
}

Think step-by-step in chain_of_thought, then provide your final verdict.`

	userPrompt := fmt.Sprintf(`Evaluate the following output using Chain of Thought reasoning:

Input: %s

Expected Output: %s

Actual Output: %s

Criteria: %s - %s
Threshold: %.2f (score must be >= %.2f to pass)

Provide your evaluation as JSON with:
1. chain_of_thought: Your step-by-step reasoning process
2. pass: boolean (true if score >= %.2f)
3. score: float between 0.0 and 1.0
4. reasoning: summary of your final judgment`, input, expectedOutput, expectedOutput, criteria.Name, criteria.Description, criteria.Threshold, criteria.Threshold, criteria.Threshold)

	req := ClaudeRequest{
		Model:       c.config.Model,
		MaxTokens:   c.config.MaxTokens,
		Temperature: c.config.Temperature,
		System:      systemPrompt,
		Messages: []ClaudeMessage{
			{Role: "user", Content: userPrompt},
		},
	}

	resp, err := c.client.CreateMessage(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("API call failed: %w", err)
	}

	if len(resp.Content) == 0 {
		return nil, fmt.Errorf("no response from API")
	}

	// Extract text from response
	var responseText string
	for _, content := range resp.Content {
		if content.Type == "text" {
			responseText = content.Text
			break
		}
	}

	if responseText == "" {
		return nil, fmt.Errorf("no text content in response")
	}

	// Parse the JSON response
	var output ClaudeJudgeOutput
	if err := json.Unmarshal([]byte(responseText), &output); err != nil {
		return nil, fmt.Errorf("failed to parse JSON response: %w (response: %s)", err, responseText)
	}

	// Combine CoT and reasoning for full context
	fullReasoning := output.Reasoning
	if output.ChainOfThought != "" {
		fullReasoning = fmt.Sprintf("Chain of Thought: %s\n\nSummary: %s", output.ChainOfThought, output.Reasoning)
	}

	span.SetAttributes(
		attribute.Bool("judge.pass", output.Pass),
		attribute.Float64("judge.score", output.Score),
	)

	return &JudgeResponse{
		Pass:      output.Pass,
		Score:     output.Score,
		Reasoning: fullReasoning,
	}, nil
}

// Evaluate implements the simple Judge interface
func (c *ClaudeJudge) Evaluate(ctx context.Context, prompt string, response string) (float64, error) {
	criteria := EvaluationCriteria{
		Name:        "quality",
		Description: "Evaluate response quality",
		Threshold:   0.7,
	}
	result, err := c.EvaluateDetailed(ctx, prompt, response, criteria)
	if err != nil {
		return 0, err
	}
	return result.Score, nil
}
