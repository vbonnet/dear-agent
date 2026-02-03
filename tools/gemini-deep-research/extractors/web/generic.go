package web

import (
	"context"
	"fmt"
	"net/http"

	"github.com/vbonnet/ai-tools/tools/gemini-deep-research/detector"
	"github.com/vbonnet/ai-tools/tools/gemini-deep-research/research"
)

// GenericWebExtractor extracts content from generic web articles
type GenericWebExtractor struct {
	httpClient *http.Client
	parser     *ReadabilityParser
}

// NewGenericWebExtractor creates a new generic web extractor
func NewGenericWebExtractor(httpClient *http.Client) *GenericWebExtractor {
	if httpClient == nil {
		httpClient = &http.Client{}
	}

	return &GenericWebExtractor{
		httpClient: httpClient,
		parser:     NewReadabilityParser(httpClient),
	}
}

// Extract extracts content from a generic web article URL
func (e *GenericWebExtractor) Extract(ctx context.Context, articleURL string) (*Content, error) {
	// Extract article content with retry logic
	var content *Content
	var extractErr error

	err := research.RetryWithBackoff(ctx, func() error {
		var err error
		content, err = e.parser.ExtractArticle(ctx, articleURL)
		if err != nil {
			extractErr = err
			return err
		}
		return nil
	}, research.DefaultRetryConfig())

	if err != nil {
		// Check for specific error types
		if extractErr == ErrPaywallDetected {
			return nil, NewPaywallError(articleURL)
		}
		if extractErr == ErrEmptyContent {
			return nil, NewEmptyContentError(articleURL)
		}
		// Check if it's a retryable error that failed after retries
		if research.IsRetryable(extractErr) {
			return nil, fmt.Errorf("network timeout after 3 retries: %w", extractErr)
		}
		// Generic extraction failure
		return nil, NewExtractionFailedError(articleURL, extractErr)
	}

	// Validate content
	if content == nil {
		return nil, NewEmptyContentError(articleURL)
	}

	if len(content.Content) < 100 {
		return nil, NewEmptyContentError(articleURL)
	}

	return content, nil
}

// Name returns the extractor name
func (e *GenericWebExtractor) Name() string {
	return "Web"
}

// ContentType returns the content type
func (e *GenericWebExtractor) ContentType() detector.ContentType {
	return detector.ContentTypeArticle
}
