package extractors

import (
	"context"

	"github.com/vbonnet/ai-tools/tools/gemini-deep-research/detector"
)

// Content represents extracted content from any source
// This is a unified structure that all extractors must return
type Content struct {
	// Raw is the extracted content as plain text
	Raw string

	// Metadata contains additional information about the content
	Metadata map[string]interface{}
}

// Extractor is the interface that all content extractors must implement
type Extractor interface {
	// Extract extracts content from a URL
	// Returns extracted content or an error
	Extract(ctx context.Context, url string) (*Content, error)

	// Name returns the human-readable name of the extractor
	Name() string

	// ContentType returns the content type this extractor handles
	ContentType() detector.ContentType
}
