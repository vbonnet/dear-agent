package research

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

var (
	// BaseURL is the base URL for the Generative Language API
	// Made var instead of const to allow test overrides
	BaseURL = "https://generativelanguage.googleapis.com/v1beta"
)

const (
	// InteractionsPath is the API path for interactions
	InteractionsPath = "/interactions"

	// DefaultAgent is the default agent for Deep Research
	DefaultAgent = "deep-research-pro-preview-12-2025"
)

// Client represents a Deep Research API client
type Client struct {
	httpClient *http.Client
	apiKey     string
	projectID  string
}

// NewClient creates a new Deep Research API client
func NewClient(projectID string) (*Client, error) {
	apiKey, err := GetAPIKey(projectID)
	if err != nil {
		return nil, err
	}

	return &Client{
		httpClient: &http.Client{},
		apiKey:     apiKey,
		projectID:  projectID,
	}, nil
}

// InteractionRequest represents a Deep Research interaction request
type InteractionRequest struct {
	Input      string `json:"input"`
	Agent      string `json:"agent"`
	Background bool   `json:"background"`
}

// InteractionResponse represents a Deep Research interaction response
type InteractionResponse struct {
	ID      string                   `json:"id"`
	Status  string                   `json:"status"`
	Outputs []InteractionOutputItem  `json:"outputs,omitempty"`
	Error   string                   `json:"error,omitempty"`
}

// InteractionOutputItem represents a single output item
type InteractionOutputItem struct {
	Text string `json:"text"`
}

// StartResearch creates a new Deep Research interaction
// Returns the interaction ID and API key for polling
func (c *Client) StartResearch(ctx context.Context, topics []string) (string, error) {
	// Format topics into research prompt
	topicsStr := strings.Join(topics, ", ")
	prompt := fmt.Sprintf("Research the following topics in depth: %s. For each topic, investigate current state, recent developments, and key challenges.", topicsStr)

	// Create request payload
	reqPayload := InteractionRequest{
		Input:      prompt,
		Agent:      DefaultAgent,
		Background: true,
	}

	// Marshal to JSON
	reqBody, err := json.Marshal(reqPayload)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	// Build URL with API key
	url := fmt.Sprintf("%s%s?key=%s", BaseURL, InteractionsPath, c.apiKey)

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(reqBody))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	// Execute request with retry
	var respBody []byte
	err = RetryWithBackoff(ctx, func() error {
		resp, err := c.httpClient.Do(req)
		if err != nil {
			return fmt.Errorf("execute request: %w", err)
		}
		defer resp.Body.Close()

		// Read response body
		respBody, err = io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("read response: %w", err)
		}

		// Check status code
		if resp.StatusCode != http.StatusOK {
			return c.handleHTTPError(resp.StatusCode, string(respBody))
		}

		return nil
	}, DefaultRetryConfig())

	if err != nil {
		return "", fmt.Errorf("start research: %w", err)
	}

	// Parse response
	var interactionResp InteractionResponse
	if err := json.Unmarshal(respBody, &interactionResp); err != nil {
		return "", fmt.Errorf("parse response: %w", err)
	}

	if interactionResp.ID == "" {
		return "", NewMalformedResponseError("", string(respBody))
	}

	return interactionResp.ID, nil
}

// handleHTTPError converts HTTP errors to appropriate error types
func (c *Client) handleHTTPError(statusCode int, body string) error {
	switch statusCode {
	case http.StatusUnauthorized:
		return NewAuthError(body)
	case http.StatusTooManyRequests:
		return NewRateLimitError("")
	case http.StatusNotFound:
		return &HTTPError{StatusCode: statusCode, Message: body}
	default:
		return &HTTPError{StatusCode: statusCode, Message: body}
	}
}

// GetInteractionStatus retrieves the current status of an interaction
func (c *Client) GetInteractionStatus(ctx context.Context, interactionID string) (*InteractionResponse, error) {
	// Build URL
	url := fmt.Sprintf("%s%s/%s?key=%s", BaseURL, InteractionsPath, interactionID, c.apiKey)

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	// Execute request with retry
	var respBody []byte
	err = RetryWithBackoff(ctx, func() error {
		resp, err := c.httpClient.Do(req)
		if err != nil {
			return fmt.Errorf("execute request: %w", err)
		}
		defer resp.Body.Close()

		// Read response body
		respBody, err = io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("read response: %w", err)
		}

		// Check status code
		if resp.StatusCode != http.StatusOK {
			if resp.StatusCode == http.StatusNotFound {
				return NewInteractionNotFoundError(interactionID)
			}
			return c.handleHTTPError(resp.StatusCode, string(respBody))
		}

		return nil
	}, DefaultRetryConfig())

	if err != nil {
		return nil, fmt.Errorf("get interaction status: %w", err)
	}

	// Parse response
	var interactionResp InteractionResponse
	if err := json.Unmarshal(respBody, &interactionResp); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	return &interactionResp, nil
}
