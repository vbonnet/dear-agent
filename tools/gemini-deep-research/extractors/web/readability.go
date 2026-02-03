package web

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"

	"github.com/go-shiori/go-readability"
)

// ReadabilityParser wraps go-readability for extracting article content
type ReadabilityParser struct {
	httpClient *http.Client
	userAgent  string
}

// NewReadabilityParser creates a new readability parser
func NewReadabilityParser(httpClient *http.Client) *ReadabilityParser {
	if httpClient == nil {
		httpClient = &http.Client{}
	}

	return &ReadabilityParser{
		httpClient: httpClient,
		userAgent:  "gemini-deep-research/2.0 (+https://github.com/vbonnet/ai-tools/tools/gemini-deep-research)",
	}
}

// ExtractArticle extracts article content from a URL using readability
func (r *ReadabilityParser) ExtractArticle(ctx context.Context, url string) (*Content, error) {
	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("User-Agent", r.userAgent)

	// Execute request
	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	// Check status code
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
	}

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	// Parse with readability
	article, err := readability.FromReader(strings.NewReader(string(body)), req.URL)
	if err != nil {
		return nil, fmt.Errorf("parse with readability: %w", err)
	}

	// Convert HTML content to plain text
	plainText := htmlToPlainText(article.TextContent)

	// Validate content length
	if len(plainText) < 100 {
		return nil, ErrEmptyContent
	}

	// Check for paywall indicators
	if detectPaywall(plainText) {
		return nil, ErrPaywallDetected
	}

	// Build content
	content := &Content{
		Title:    article.Title,
		Authors:  article.Byline,
		Abstract: article.Excerpt,
		Content:  plainText,
		Metadata: map[string]string{
			"url": url,
		},
	}

	return content, nil
}

// htmlToPlainText converts HTML to plain text
func htmlToPlainText(html string) string {
	// Remove HTML tags
	re := regexp.MustCompile(`<[^>]*>`)
	text := re.ReplaceAllString(html, " ")

	// Decode HTML entities
	text = decodeHTMLEntities(text)

	// Clean up whitespace
	text = cleanWhitespace(text)

	return text
}

// decodeHTMLEntities decodes common HTML entities
func decodeHTMLEntities(text string) string {
	replacements := map[string]string{
		"&nbsp;":  " ",
		"&amp;":   "&",
		"&lt;":    "<",
		"&gt;":    ">",
		"&quot;":  "\"",
		"&#39;":   "'",
		"&apos;":  "'",
		"&mdash;": "—",
		"&ndash;": "–",
		"&hellip;": "...",
	}

	for entity, replacement := range replacements {
		text = strings.ReplaceAll(text, entity, replacement)
	}

	return text
}

// cleanWhitespace cleans up excessive whitespace
func cleanWhitespace(text string) string {
	// Replace multiple spaces with single space
	re := regexp.MustCompile(`\s+`)
	text = re.ReplaceAllString(text, " ")

	// Trim leading/trailing whitespace
	text = strings.TrimSpace(text)

	return text
}

// detectPaywall checks for common paywall indicators
func detectPaywall(content string) bool {
	paywallIndicators := []string{
		"subscribe to continue reading",
		"this article is for subscribers only",
		"please sign in to continue",
		"become a member to read",
		"subscription required",
		"login to view",
		"register to read",
		"premium content",
	}

	lowerContent := strings.ToLower(content)
	for _, indicator := range paywallIndicators {
		if strings.Contains(lowerContent, indicator) {
			return true
		}
	}

	return false
}
