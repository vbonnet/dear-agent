package arxiv

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/vbonnet/ai-tools/tools/gemini-deep-research/detector"
)

// Extractor extracts content from arxiv papers
type Extractor struct {
	apiClient *APIClient
	pdfParser *PDFParser
}

// Content represents extracted arxiv paper content
type Content struct {
	Title          string
	Authors        []string
	Abstract       string
	PublishedDate  string
	FullText       string
	ExtractionMode string // "full" or "abstract-only"
}

// NewExtractor creates a new arxiv extractor
func NewExtractor() *Extractor {
	return &Extractor{
		apiClient: NewAPIClient(),
		pdfParser: NewPDFParser(),
	}
}

// Extract extracts content from an arxiv paper URL
func (e *Extractor) Extract(ctx context.Context, url string) (*Content, error) {
	// Extract paper ID from URL
	paperID, err := extractPaperID(url)
	if err != nil {
		return nil, &ExtractionError{
			URL:       url,
			Operation: "extract arxiv paper ID",
			Err:       err,
			Troubleshooting: []string{
				"Verify URL format is correct (e.g., https://arxiv.org/abs/2601.20802)",
				"Check that the arxiv ID is valid",
				"Try accessing the URL in a web browser",
			},
		}
	}

	// Fetch metadata from arxiv API
	metadata, err := e.apiClient.FetchMetadata(ctx, paperID)
	if err != nil {
		return nil, &ExtractionError{
			URL:       url,
			Operation: "fetch arxiv metadata",
			Err:       err,
			Troubleshooting: []string{
				"Check internet connection",
				"Verify arxiv API is accessible",
				"Ensure paper ID is correct: " + paperID,
			},
		}
	}

	// Download and extract PDF text
	fullText, err := e.pdfParser.ExtractFromArxiv(ctx, paperID)

	extractionMode := "full"
	if err != nil {
		// Log warning but continue with abstract-only mode
		extractionMode = "abstract-only"
		fullText = ""
		// Note: We don't return error here - this is graceful degradation
	}

	return &Content{
		Title:          metadata.Title,
		Authors:        metadata.Authors,
		Abstract:       metadata.Abstract,
		PublishedDate:  metadata.Published,
		FullText:       fullText,
		ExtractionMode: extractionMode,
	}, nil
}

// Name returns the extractor name
func (e *Extractor) Name() string {
	return "Arxiv"
}

// ContentType returns the content type this extractor handles
func (e *Extractor) ContentType() detector.ContentType {
	return detector.ContentTypeArxiv
}

// Format formats the extracted content as a string
func (c *Content) Format() string {
	var b strings.Builder

	fmt.Fprintf(&b, "PAPER: %s\n", c.Title)
	fmt.Fprintf(&b, "AUTHORS: %s\n", strings.Join(c.Authors, ", "))
	fmt.Fprintf(&b, "ABSTRACT: %s\n", c.Abstract)
	fmt.Fprintf(&b, "PUBLICATION DATE: %s\n\n", c.PublishedDate)

	if c.FullText != "" {
		fmt.Fprintf(&b, "FULL TEXT:\n%s\n", c.FullText)
	} else {
		fmt.Fprintf(&b, "FULL TEXT: [Extraction failed, abstract only]\n")
	}

	return b.String()
}

var (
	// arxivIDPattern matches arxiv paper IDs in URLs
	// Supports formats like: 2601.20802, 2601.20802v1, 2601.20802v2
	arxivIDPattern = regexp.MustCompile(`(\d{4}\.\d{4,5})(v\d+)?`)
)

// extractPaperID extracts the arxiv paper ID from a URL
// Handles formats:
//   - https://arxiv.org/abs/2601.20802
//   - https://arxiv.org/pdf/2601.20802.pdf
//   - https://arxiv.org/abs/2601.20802v1
//   - http://arxiv.org/abs/2601.20802
func extractPaperID(url string) (string, error) {
	matches := arxivIDPattern.FindStringSubmatch(url)
	if len(matches) < 2 {
		return "", ErrInvalidArxivID
	}

	// Return base ID without version suffix (v1, v2, etc.)
	return matches[1], nil
}
