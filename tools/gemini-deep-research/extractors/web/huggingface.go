package web

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/vbonnet/ai-tools/tools/gemini-deep-research/detector"
	"github.com/vbonnet/ai-tools/tools/gemini-deep-research/research"
)

// HuggingFaceExtractor extracts content from HuggingFace papers
type HuggingFaceExtractor struct {
	httpClient *http.Client
	parser     *ReadabilityParser
	apiBaseURL string // For testing
}

// NewHuggingFaceExtractor creates a new HuggingFace extractor
func NewHuggingFaceExtractor(httpClient *http.Client) *HuggingFaceExtractor {
	if httpClient == nil {
		httpClient = &http.Client{}
	}

	return &HuggingFaceExtractor{
		httpClient: httpClient,
		parser:     NewReadabilityParser(httpClient),
		apiBaseURL: "https://huggingface.co",
	}
}

// Extract extracts content from a HuggingFace paper URL
func (e *HuggingFaceExtractor) Extract(ctx context.Context, paperURL string) (*Content, error) {
	// Parse paper ID from URL
	paperID, err := e.parsePaperID(paperURL)
	if err != nil {
		return nil, NewInvalidURLError(paperURL, err)
	}

	// Try API-first approach
	content, err := e.extractViaAPI(ctx, paperID)
	if err == nil {
		return content, nil
	}

	// API failed, fall back to web scraping
	fmt.Printf("HuggingFace API unavailable, using web scraping fallback\n")
	return e.extractViaWebScraping(ctx, paperURL)
}

// Name returns the extractor name
func (e *HuggingFaceExtractor) Name() string {
	return "HuggingFace"
}

// ContentType returns the content type
func (e *HuggingFaceExtractor) ContentType() detector.ContentType {
	return detector.ContentTypeHuggingFace
}

// parsePaperID extracts the paper ID from a HuggingFace URL
// Example: https://huggingface.co/papers/2501.12345 -> 2501.12345
func (e *HuggingFaceExtractor) parsePaperID(paperURL string) (string, error) {
	// Parse URL
	u, err := url.Parse(paperURL)
	if err != nil {
		return "", fmt.Errorf("parse URL: %w", err)
	}

	// Extract path segments
	path := strings.Trim(u.Path, "/")
	segments := strings.Split(path, "/")

	// Expected format: papers/<ID>
	if len(segments) < 2 || segments[0] != "papers" {
		return "", fmt.Errorf("invalid HuggingFace paper URL format")
	}

	paperID := segments[1]

	// Validate paper ID format (e.g., 2501.12345)
	re := regexp.MustCompile(`^\d{4}\.\d{5}$`)
	if !re.MatchString(paperID) {
		return "", fmt.Errorf("invalid paper ID format: %s", paperID)
	}

	return paperID, nil
}

// extractViaAPI extracts content using the HuggingFace API
func (e *HuggingFaceExtractor) extractViaAPI(ctx context.Context, paperID string) (*Content, error) {
	// Build API URL
	apiURL := fmt.Sprintf("%s/api/papers/%s", e.apiBaseURL, paperID)

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	// Execute request with retry
	var respBody []byte
	err = research.RetryWithBackoff(ctx, func() error {
		resp, err := e.httpClient.Do(req)
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
			return &research.HTTPError{
				StatusCode: resp.StatusCode,
				Message:    string(respBody),
			}
		}

		return nil
	}, research.DefaultRetryConfig())

	if err != nil {
		return nil, fmt.Errorf("fetch API: %w", err)
	}

	// Parse JSON response
	var apiResp HuggingFaceAPIResponse
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return nil, fmt.Errorf("parse API response: %w", err)
	}

	// Parse authors (can be array or object)
	authors := e.parseAuthors(apiResp.Authors)

	// Build content
	content := &Content{
		Title:       apiResp.Title,
		Authors:     authors,
		Abstract:    apiResp.Abstract,
		Upvotes:     apiResp.Upvotes,
		Collections: apiResp.Collections,
		Content:     e.formatAPIContent(&apiResp, authors),
		Metadata: map[string]string{
			"id": apiResp.ID,
		},
	}

	return content, nil
}

// extractViaWebScraping extracts content using web scraping fallback
func (e *HuggingFaceExtractor) extractViaWebScraping(ctx context.Context, paperURL string) (*Content, error) {
	// Use readability parser
	content, err := e.parser.ExtractArticle(ctx, paperURL)
	if err != nil {
		return nil, NewExtractionFailedError(paperURL, err)
	}

	return content, nil
}

// parseAuthors parses the authors field which can be an array or object
func (e *HuggingFaceExtractor) parseAuthors(authorsData json.RawMessage) string {
	// Try parsing as array first
	var authorsArray []string
	if err := json.Unmarshal(authorsData, &authorsArray); err == nil {
		return strings.Join(authorsArray, ", ")
	}

	// Try parsing as object (e.g., {"name": "Author Name"})
	var authorsObj map[string]interface{}
	if err := json.Unmarshal(authorsData, &authorsObj); err == nil {
		// Extract names from object
		var names []string
		for _, v := range authorsObj {
			if name, ok := v.(string); ok {
				names = append(names, name)
			}
		}
		return strings.Join(names, ", ")
	}

	// Fallback: return as string
	return string(authorsData)
}

// formatAPIContent formats the API response into plain text content
func (e *HuggingFaceExtractor) formatAPIContent(resp *HuggingFaceAPIResponse, authors string) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("PAPER: %s\n\n", resp.Title))

	if authors != "" {
		sb.WriteString(fmt.Sprintf("AUTHORS: %s\n\n", authors))
	}

	if resp.Abstract != "" {
		sb.WriteString(fmt.Sprintf("ABSTRACT:\n%s\n\n", resp.Abstract))
	}

	sb.WriteString(fmt.Sprintf("UPVOTES: %d\n", resp.Upvotes))

	if len(resp.Collections) > 0 {
		sb.WriteString(fmt.Sprintf("COLLECTIONS: %s\n", strings.Join(resp.Collections, ", ")))
	}

	return sb.String()
}
