package extractors

import (
	"context"
	"fmt"
	"log"

	"github.com/vbonnet/ai-tools/tools/gemini-deep-research/detector"
)

// Detector interface defines content type detection
type Detector interface {
	DetectContentType(url string, typeOverride string) (detector.ContentType, error)
}

// Orchestrator coordinates content extraction by detecting content type
// and routing to the appropriate extractor
type Orchestrator struct {
	factory  *ExtractorFactory
	detector Detector
}

// NewOrchestrator creates a new Orchestrator
// Returns an error if factory initialization fails
func NewOrchestrator() (*Orchestrator, error) {
	factory, err := NewExtractorFactory()
	if err != nil {
		return nil, fmt.Errorf("create extractor factory: %w", err)
	}

	return &Orchestrator{
		factory:  factory,
		detector: &detectorWrapper{},
	}, nil
}

// ExtractContent extracts content from a URL
// Flow:
//  1. Detect content type (use detector package)
//  2. Get appropriate extractor from factory
//  3. Call extractor.Extract()
//  4. Return extracted content or error with context
//
// Parameters:
//   - ctx: Context for cancellation and timeout
//   - url: The URL to extract content from
//   - typeOverride: Optional content type override (empty string for auto-detection)
//
// Returns:
//   - *Content: Extracted content with metadata
//   - error: Error with context if extraction fails
func (o *Orchestrator) ExtractContent(ctx context.Context, url string, typeOverride string) (*Content, error) {
	// Step 1: Detect content type
	contentType, err := o.detector.DetectContentType(url, typeOverride)
	if err != nil {
		return nil, &ExtractionError{
			URL:       url,
			Operation: "detect content type",
			Err:       err,
			Troubleshooting: []string{
				"Verify URL format is correct",
				"Check that URL is accessible",
				"Try specifying content type with --type flag",
			},
		}
	}

	log.Printf("[Orchestrator] Content type: %s", contentType.String())

	// Step 2: Get appropriate extractor
	extractor, err := o.factory.GetExtractor(contentType)
	if err != nil {
		return nil, &ExtractionError{
			URL:       url,
			Operation: "get extractor",
			Err:       err,
			Troubleshooting: []string{
				fmt.Sprintf("Content type %s is not supported", contentType.String()),
				"Try a different URL or content type",
			},
		}
	}

	log.Printf("[Orchestrator] Using %s extractor", extractor.Name())

	// Step 3: Extract content
	content, err := extractor.Extract(ctx, url)
	if err != nil {
		// Wrap error with orchestrator context
		return nil, fmt.Errorf("extract content using %s extractor: %w", extractor.Name(), err)
	}

	log.Printf("[Orchestrator] Extracted %d characters", len(content.Raw))

	return content, nil
}

// detectorWrapper wraps the detector package to implement the Detector interface
type detectorWrapper struct{}

func (d *detectorWrapper) DetectContentType(url string, typeOverride string) (detector.ContentType, error) {
	return detector.DetectContentType(url, typeOverride)
}

// ExtractionError represents an error during content extraction orchestration
type ExtractionError struct {
	URL             string
	Operation       string
	Err             error
	Troubleshooting []string
}

// Error returns a formatted error message with troubleshooting tips
func (e *ExtractionError) Error() string {
	msg := fmt.Sprintf("Failed to %s\n", e.Operation)
	msg += fmt.Sprintf("  URL: %s\n", e.URL)
	msg += fmt.Sprintf("  Reason: %v\n", e.Err)

	if len(e.Troubleshooting) > 0 {
		msg += "  Troubleshooting:\n"
		for _, tip := range e.Troubleshooting {
			msg += fmt.Sprintf("    - %s\n", tip)
		}
	}

	return msg
}

// Unwrap returns the underlying error for error chain inspection
func (e *ExtractionError) Unwrap() error {
	return e.Err
}
