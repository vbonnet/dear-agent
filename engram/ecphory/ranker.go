package ecphory

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/vbonnet/dear-agent/engram/internal/prompt"
	"golang.org/x/time/rate"
)

// Provider interface for different LLM backends
type Provider interface {
	Complete(ctx context.Context, prompt string) (string, error)
	Close() error
}

// RateLimiter wraps golang.org/x/time/rate for API rate limiting
// Enforces both per-second (1 req/sec) and session limits (20 req/session)
type RateLimiter struct {
	mu            sync.Mutex
	limiter       *rate.Limiter
	sessionTokens int
}

// NewRateLimiter creates a rate limiter using stdlib-endorsed rate package
// Limits: 1 request/second with burst of 1, max 20 requests per session
func NewRateLimiter() *RateLimiter {
	return &RateLimiter{
		limiter:       rate.NewLimiter(rate.Every(1*time.Second), 1),
		sessionTokens: 20,
	}
}

// Allow checks if a request can be made and updates the limiter state
func (rl *RateLimiter) Allow() error {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	// Check session limit
	if rl.sessionTokens <= 0 {
		return fmt.Errorf("session rate limit exceeded (20/session)")
	}

	// Check rate limit using stdlib limiter
	if !rl.limiter.Allow() {
		return fmt.Errorf("rate limit: wait before next request")
	}

	// Consume session token
	rl.sessionTokens--

	return nil
}

// Ranker handles relevance ranking of engrams using LLM provider
type Ranker struct {
	provider    Provider
	rateLimiter *RateLimiter
}

// RankingResult represents a ranked engram
type RankingResult struct {
	Path      string  `json:"path"`
	Relevance float64 `json:"relevance"`
	Reasoning string  `json:"reasoning"`
}

// NewRanker creates a new ranker with auto-detected provider (Anthropic or VertexAI)
func NewRanker() (*Ranker, error) {
	// Try VertexAI first (check for Google Cloud project)
	if project := os.Getenv("GOOGLE_CLOUD_PROJECT"); project != "" {
		provider, err := NewVertexAIProvider(project)
		if err != nil {
			return nil, fmt.Errorf("failed to create VertexAI provider: %w", err)
		}
		return &Ranker{
			provider:    provider,
			rateLimiter: NewRateLimiter(),
		}, nil
	}

	// Fallback to Anthropic API
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("neither GOOGLE_CLOUD_PROJECT nor ANTHROPIC_API_KEY environment variable set")
	}

	// P0-1 FIX: Validate API key format to prevent invalid keys from being used
	// Anthropic API keys should start with "sk-ant-"
	if !strings.HasPrefix(apiKey, "sk-ant-") {
		// Never log the actual key value for security
		return nil, fmt.Errorf("invalid API key format (expected sk-ant- prefix)")
	}

	provider, err := NewAnthropicProvider(apiKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create Anthropic provider: %w", err)
	}

	return &Ranker{
		provider:    provider,
		rateLimiter: NewRateLimiter(),
	}, nil
}

// Rank ranks engrams by relevance to a query
func (r *Ranker) Rank(ctx context.Context, query string, candidates []string) ([]RankingResult, error) {
	if len(candidates) == 0 {
		return nil, nil
	}

	// P0-2: Check rate limit before making API call
	if err := r.rateLimiter.Allow(); err != nil {
		return nil, fmt.Errorf("rate limit: %w", err)
	}

	// P1-4: Check context cancellation
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("context cancelled: %w", err)
	}

	// Build ranking prompt
	rankingPrompt, err := r.buildRankingPrompt(query, candidates)
	if err != nil {
		return nil, fmt.Errorf("failed to build prompt: %w", err)
	}

	// Call provider
	responseText, err := r.provider.Complete(ctx, rankingPrompt)
	if err != nil {
		return nil, fmt.Errorf("provider call failed: %w", err)
	}

	// Parse JSON response
	var results []RankingResult
	if err := json.Unmarshal([]byte(responseText), &results); err != nil {
		return nil, fmt.Errorf("failed to parse ranking results: %w", err)
	}

	// P1-5: Validate parsed results
	if err := validateRankingResults(results, candidates); err != nil {
		return nil, fmt.Errorf("invalid ranking results: %w", err)
	}

	return results, nil
}

// validateRankingResults validates the structure and content of ranking results
func validateRankingResults(results []RankingResult, candidates []string) error {
	if len(results) == 0 {
		return fmt.Errorf("no ranking results returned")
	}

	// Validate each result
	for i, result := range results {
		if result.Path == "" {
			return fmt.Errorf("result[%d]: empty path", i)
		}
		if result.Relevance < 0.0 || result.Relevance > 1.0 {
			return fmt.Errorf("result[%d]: relevance %.2f out of range [0.0, 1.0]", i, result.Relevance)
		}
		// Path should be one of the candidates
		found := false
		for _, candidate := range candidates {
			if result.Path == candidate {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("result[%d]: path %s not in candidate list", i, result.Path)
		}
	}

	return nil
}

// buildRankingPrompt builds the ranking prompt for the API with prompt injection defense.
// It sanitizes the user query and wraps it in XML hierarchy to prevent injection attacks.
//
// Security:
//   - User query is sanitized (rejects XML tags and injection patterns)
//   - Query wrapped in <user> tags
//   - Candidate paths wrapped in <untrusted_data> tags
//   - See: core/docs/specs/prompt-injection-defense.md
func (r *Ranker) buildRankingPrompt(query string, candidates []string) (string, error) {
	// Sanitize user query
	sanitizer := prompt.NewQuerySanitizer()
	sanitizedQuery, err := sanitizer.Sanitize(query)
	if err != nil {
		return "", fmt.Errorf("query validation failed: %w", err)
	}

	// Build candidate list (external data)
	var candidateList strings.Builder
	candidateList.WriteString("Engram candidates:\n")
	for i, candidate := range candidates {
		fmt.Fprintf(&candidateList, "%d. %s\n", i+1, candidate)
	}

	// Build task description
	candidateList.WriteString(`
Task: Rank these engrams by relevance to the user's query.

Return a JSON array of objects with:
- path: The engram path
- relevance: Score from 0.0 to 1.0
- reasoning: Brief explanation of relevance

Example:
[
  {"path": "patterns/foo.ai.md", "relevance": 0.9, "reasoning": "Directly addresses query"},
  {"path": "strategies/bar.ai.md", "relevance": 0.6, "reasoning": "Related but less specific"}
]

Return only the JSON array, no other text.`)

	// Use prompt template with XML hierarchy
	renderedPrompt, err := prompt.RenderPrompt(sanitizedQuery, candidateList.String())
	if err != nil {
		return "", fmt.Errorf("prompt rendering failed: %w", err)
	}

	return renderedPrompt, nil
}

// Close cleans up ranker resources
func (r *Ranker) Close() error {
	if r.provider != nil {
		return r.provider.Close()
	}
	return nil
}

// AnthropicProvider implements Provider interface for Anthropic API
type AnthropicProvider struct {
	client anthropic.Client
	model  string
}

// NewAnthropicProvider creates a new Anthropic provider
func NewAnthropicProvider(apiKey string) (*AnthropicProvider, error) {
	client := anthropic.NewClient(option.WithAPIKey(apiKey))
	return &AnthropicProvider{
		client: client,
		model:  "claude-3-5-haiku-20241022",
	}, nil
}

// Complete sends a prompt to Anthropic API and returns the response
func (p *AnthropicProvider) Complete(ctx context.Context, prompt string) (string, error) {
	response, err := p.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     p.model,
		MaxTokens: 4096,
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(prompt)),
		},
	})
	if err != nil {
		return "", fmt.Errorf("API call failed: %w", err)
	}

	// Validate API response structure
	if len(response.Content) == 0 {
		return "", fmt.Errorf("empty API response")
	}

	textBlock := response.Content[0].AsText()
	if textBlock.Type != "text" {
		return "", fmt.Errorf("unexpected response format: expected TextBlock")
	}

	return textBlock.Text, nil
}

// Close cleans up Anthropic provider resources
func (p *AnthropicProvider) Close() error {
	// No resources to clean up
	return nil
}

// VertexAIProvider implements Provider interface for Google VertexAI
type VertexAIProvider struct {
	project  string
	location string
	model    string
}

// NewVertexAIProvider creates a new VertexAI provider
func NewVertexAIProvider(project string) (*VertexAIProvider, error) {
	// Default location and model
	location := os.Getenv("VERTEX_LOCATION")
	if location == "" {
		location = "us-central1" // Default to us-central1
	}

	model := os.Getenv("VERTEX_MODEL")
	if model == "" {
		model = "claude-3-5-sonnet-v2@20241022" // Default Claude model on Vertex
	}

	return &VertexAIProvider{
		project:  project,
		location: location,
		model:    model,
	}, nil
}

// Complete sends a prompt to VertexAI and returns the response
//
//nolint:gocyclo // Function handles complete HTTP request/response cycle; complexity acceptable
func (p *VertexAIProvider) Complete(ctx context.Context, prompt string) (string, error) {
	// VertexAI endpoint for Claude models
	url := fmt.Sprintf("https://%s-aiplatform.googleapis.com/v1/projects/%s/locations/%s/publishers/anthropic/models/%s:streamRawPredict",
		p.location, p.project, p.location, p.model)

	// Build request body (Anthropic Messages API format)
	reqBody := map[string]interface{}{
		"anthropic_version": "vertex-2023-10-16",
		"messages": []map[string]interface{}{
			{
				"role": "user",
				"content": []map[string]interface{}{
					{
						"type": "text",
						"text": prompt,
					},
				},
			},
		},
		"max_tokens": 4096,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	// Get access token from Application Default Credentials
	// Note: This requires gcloud auth application-default login or GOOGLE_APPLICATION_CREDENTIALS
	token, err := p.getAccessToken(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get access token: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	// Make request
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("API call failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return "", fmt.Errorf("API returned status %d (failed to read error body: %w)", resp.StatusCode, err)
		}
		return "", fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
	}

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	// Parse response (streaming format - multiple JSON objects)
	// We need to extract the text content from the response
	var fullText strings.Builder
	lines := strings.Split(string(body), "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}

		var event map[string]interface{}
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			continue // Skip non-JSON lines
		}

		// Look for content_block_delta events
		if eventType, ok := event["type"].(string); ok && eventType == "content_block_delta" {
			if delta, ok := event["delta"].(map[string]interface{}); ok {
				if text, ok := delta["text"].(string); ok {
					fullText.WriteString(text)
				}
			}
		}
	}

	if fullText.Len() == 0 {
		return "", fmt.Errorf("no text content in response")
	}

	return fullText.String(), nil
}

// getAccessToken retrieves an access token for VertexAI API
func (p *VertexAIProvider) getAccessToken(ctx context.Context) (string, error) {
	// Use gcloud command to get access token
	// This works if user has run: gcloud auth application-default login
	cmd := exec.CommandContext(ctx, "gcloud", "auth", "application-default", "print-access-token") //nolint:gosec // G702: hardcoded command and args, no user input
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get access token (run: gcloud auth application-default login): %w", err)
	}
	return strings.TrimSpace(string(output)), nil
}

// Close cleans up VertexAI provider resources
func (p *VertexAIProvider) Close() error {
	// No resources to clean up
	return nil
}
