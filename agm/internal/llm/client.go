package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	aiplatform "cloud.google.com/go/aiplatform/apiv1"
	"cloud.google.com/go/aiplatform/apiv1/aiplatformpb"
	"golang.org/x/time/rate"
	"google.golang.org/api/option"
	"google.golang.org/protobuf/types/known/structpb"
)

// Client wraps Vertex AI API for semantic search
type Client struct {
	projectID string
	location  string
	modelID   string
	limiter   *rate.Limiter
}

// ClientConfig holds configuration for the LLM client
type ClientConfig struct {
	ProjectID string // GCP project ID (required)
	Location  string // GCP region (default: "us-central1")
	ModelID   string // Model name (default: "claude-3-5-haiku@20241022")
	RateLimit int    // Searches per minute (default: 10)
}

// NewClient creates a new Vertex AI client with Application Default Credentials
func NewClient(cfg ClientConfig) (*Client, error) {
	// Set defaults
	if cfg.Location == "" {
		cfg.Location = "us-central1"
	}
	if cfg.ModelID == "" {
		cfg.ModelID = "claude-3-5-haiku@20241022"
	}
	if cfg.RateLimit == 0 {
		cfg.RateLimit = 10
	}

	if cfg.ProjectID == "" {
		return nil, fmt.Errorf("project ID is required (set via config or GOOGLE_CLOUD_PROJECT env var)")
	}

	// Rate limiter: 10 searches/minute = 1 search every 6 seconds
	interval := time.Minute / time.Duration(cfg.RateLimit)
	limiter := rate.NewLimiter(rate.Every(interval), cfg.RateLimit)

	return &Client{
		projectID: cfg.ProjectID,
		location:  cfg.Location,
		modelID:   cfg.ModelID,
		limiter:   limiter,
	}, nil
}

// SearchRequest represents a semantic search request
type SearchRequest struct {
	Query      string
	Sessions   []SessionMetadata
	MaxResults int // Maximum results to return (default: 10)
}

// SessionMetadata holds information about a session for search
type SessionMetadata struct {
	SessionID string
	Name      string
	Tags      []string
	Project   string
}

// Search performs semantic search using Vertex AI (Claude on Vertex)
func (c *Client) Search(ctx context.Context, req SearchRequest) ([]SearchResult, error) {
	// Rate limiting
	if err := c.limiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("rate limit wait failed: %w", err)
	}

	// Build prompt
	prompt := c.buildSearchPrompt(req)

	// Create Vertex AI client
	client, err := aiplatform.NewPredictionClient(ctx, option.WithEndpoint(fmt.Sprintf("%s-aiplatform.googleapis.com:443", c.location)))
	if err != nil {
		return nil, fmt.Errorf("failed to create Vertex AI client: %w\nHint: Run 'gcloud auth application-default login' to authenticate", err)
	}
	defer client.Close()

	// Build request
	endpoint := fmt.Sprintf("projects/%s/locations/%s/publishers/anthropic/models/%s",
		c.projectID, c.location, c.modelID)

	// Construct request parameters
	parameters, err := structpb.NewStruct(map[string]interface{}{
		"anthropic_version": "vertex-2023-10-16",
		"max_tokens":        1024,
		"temperature":       0.0, // Deterministic for search
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create parameters: %w", err)
	}

	// Construct messages
	messages := []interface{}{
		map[string]interface{}{
			"role": "user",
			"content": []interface{}{
				map[string]interface{}{
					"type": "text",
					"text": prompt,
				},
			},
		},
	}

	messagesValue, err := structpb.NewValue(messages)
	if err != nil {
		return nil, fmt.Errorf("failed to create messages value: %w", err)
	}

	instances := []*structpb.Value{
		structpb.NewStructValue(&structpb.Struct{
			Fields: map[string]*structpb.Value{
				"messages": messagesValue,
			},
		}),
	}

	// Create context with timeout
	ctxWithTimeout, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// Make API request
	apiReq := &aiplatformpb.PredictRequest{
		Endpoint:   endpoint,
		Instances:  instances,
		Parameters: structpb.NewStructValue(parameters),
	}

	resp, err := client.Predict(ctxWithTimeout, apiReq)
	if err != nil {
		return nil, fmt.Errorf("Vertex AI API request failed: %w\nHint: Ensure GCP credentials are configured with 'gcloud auth application-default login'", err)
	}

	// Parse response
	results, err := c.parseSearchResponse(resp, req.Sessions)
	if err != nil {
		return nil, fmt.Errorf("failed to parse search results: %w", err)
	}

	// Limit results
	maxResults := req.MaxResults
	if maxResults == 0 {
		maxResults = 10
	}
	if len(results) > maxResults {
		results = results[:maxResults]
	}

	return results, nil
}

// buildSearchPrompt constructs the LLM prompt for semantic search
func (c *Client) buildSearchPrompt(req SearchRequest) string {
	var sb strings.Builder

	fmt.Fprintf(&sb, "Find archived Claude sessions related to: \"%s\"\n\n", req.Query)
	sb.WriteString("Available sessions:\n")

	for i, s := range req.Sessions {
		fmt.Fprintf(&sb, "%d. Session ID: %s\n", i+1, s.SessionID)
		fmt.Fprintf(&sb, "   Name: %s\n", s.Name)
		if len(s.Tags) > 0 {
			fmt.Fprintf(&sb, "   Tags: [%s]\n", strings.Join(s.Tags, ", "))
		}
		if s.Project != "" {
			fmt.Fprintf(&sb, "   Project: %s\n", s.Project)
		}
		sb.WriteString("\n")
	}

	sb.WriteString("\nReturn ONLY a JSON array of session IDs ranked by relevance (most relevant first).\n")
	sb.WriteString("Format: [\"session-id-1\", \"session-id-2\", ...]\n")
	sb.WriteString("If no sessions match, return an empty array: []\n")

	return sb.String()
}

// parseSearchResponse extracts session IDs from the LLM response
func (c *Client) parseSearchResponse(resp *aiplatformpb.PredictResponse, sessions []SessionMetadata) ([]SearchResult, error) {
	if len(resp.Predictions) == 0 {
		return []SearchResult{}, nil
	}

	// Extract text from response
	prediction := resp.Predictions[0]
	predMap := prediction.GetStructValue().AsMap()

	// Navigate to content
	content, ok := predMap["content"].([]interface{})
	if !ok || len(content) == 0 {
		return nil, fmt.Errorf("unexpected response format: no content field")
	}

	contentBlock, ok := content[0].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("unexpected response format: invalid content block")
	}

	text, ok := contentBlock["text"].(string)
	if !ok {
		return nil, fmt.Errorf("unexpected response format: no text in content block")
	}

	// Parse JSON array from text
	text = strings.TrimSpace(text)

	// Extract JSON array (handle markdown code blocks)
	if strings.Contains(text, "```json") {
		start := strings.Index(text, "[")
		end := strings.LastIndex(text, "]")
		if start >= 0 && end > start {
			text = text[start : end+1]
		}
	} else if strings.Contains(text, "```") {
		start := strings.Index(text, "[")
		end := strings.LastIndex(text, "]")
		if start >= 0 && end > start {
			text = text[start : end+1]
		}
	}

	var sessionIDs []string
	if err := json.Unmarshal([]byte(text), &sessionIDs); err != nil {
		return nil, fmt.Errorf("failed to parse session IDs from response: %w\nResponse text: %s", err, text)
	}

	// Convert to SearchResults with relevance scores
	var results []SearchResult
	for i, sessionID := range sessionIDs {
		// Simple relevance score based on position (1.0 for first, decreasing)
		relevance := 1.0 - (float64(i) * 0.1)
		if relevance < 0 {
			relevance = 0.0
		}

		results = append(results, SearchResult{
			SessionID: sessionID,
			Relevance: relevance,
			Reason:    fmt.Sprintf("Ranked #%d by LLM", i+1),
		})
	}

	return results, nil
}
