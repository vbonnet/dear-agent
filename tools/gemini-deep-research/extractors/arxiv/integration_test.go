// +build integration

package arxiv

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// These tests require network access and real arxiv API
// Run with: go test -tags=integration ./extractors/arxiv/

func TestIntegration_APIClient_FetchMetadata(t *testing.T) {
	client := NewAPIClient()
	ctx := context.Background()

	// Use a known stable arxiv paper
	paperID := "2301.07041"

	metadata, err := client.FetchMetadata(ctx, paperID)
	if err != nil {
		t.Fatalf("FetchMetadata() error = %v", err)
	}

	// Validate metadata structure
	if metadata.Title == "" {
		t.Error("FetchMetadata() returned empty title")
	}

	if len(metadata.Authors) == 0 {
		t.Error("FetchMetadata() returned no authors")
	}

	if metadata.Abstract == "" {
		t.Error("FetchMetadata() returned empty abstract")
	}

	if metadata.Published == "" {
		t.Error("FetchMetadata() returned empty published date")
	}

	expectedPDFURL := "https://arxiv.org/pdf/" + paperID + ".pdf"
	if metadata.PDFURL != expectedPDFURL {
		t.Errorf("FetchMetadata() PDFURL = %q, want %q", metadata.PDFURL, expectedPDFURL)
	}

	t.Logf("Successfully fetched: %s", metadata.Title)
	t.Logf("Authors: %v", metadata.Authors)
	t.Logf("Abstract length: %d chars", len(metadata.Abstract))
}

func TestIntegration_APIClient_FetchMetadata_NotFound(t *testing.T) {
	client := NewAPIClient()
	ctx := context.Background()

	// Use an invalid paper ID
	paperID := "9999.99999"

	_, err := client.FetchMetadata(ctx, paperID)
	if err == nil {
		t.Error("FetchMetadata() expected error for invalid paper ID")
	}

	if !errors.Is(err, ErrPaperNotFound) {
		t.Errorf("FetchMetadata() error = %v, want ErrPaperNotFound", err)
	}
}

func TestIntegration_PDFParser_ExtractFromArxiv(t *testing.T) {
	parser := NewPDFParser()
	ctx := context.Background()

	// Use a known paper with reasonable size
	paperID := "2301.07041"

	text, err := parser.ExtractFromArxiv(ctx, paperID)
	if err != nil {
		t.Fatalf("ExtractFromArxiv() error = %v", err)
	}

	// Validate extracted text
	if len(strings.TrimSpace(text)) < 1000 {
		t.Errorf("ExtractFromArxiv() returned suspiciously short text: %d chars", len(text))
	}

	t.Logf("Extracted %d characters from PDF", len(text))
}

func TestIntegration_Extractor_Extract_Complete(t *testing.T) {
	extractor := NewExtractor()
	ctx := context.Background()

	testCases := []struct {
		name string
		url  string
	}{
		{
			name: "abs URL",
			url:  "https://arxiv.org/abs/2301.07041",
		},
		{
			name: "pdf URL",
			url:  "https://arxiv.org/pdf/2301.07041.pdf",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			content, err := extractor.Extract(ctx, tc.url)
			if err != nil {
				t.Fatalf("Extract() error = %v", err)
			}

			// Validate content
			if content.Title == "" {
				t.Error("Extract() returned empty title")
			}

			if len(content.Authors) == 0 {
				t.Error("Extract() returned no authors")
			}

			if content.Abstract == "" {
				t.Error("Extract() returned empty abstract")
			}

			if content.PublishedDate == "" {
				t.Error("Extract() returned empty published date")
			}

			// Format and validate output
			formatted := content.Format()
			requiredSections := []string{
				"PAPER:",
				"AUTHORS:",
				"ABSTRACT:",
				"PUBLICATION DATE:",
				"FULL TEXT:",
			}

			for _, section := range requiredSections {
				if !strings.Contains(formatted, section) {
					t.Errorf("Format() missing section: %s", section)
				}
			}

			t.Logf("Extraction mode: %s", content.ExtractionMode)
			t.Logf("Title: %s", content.Title)
			t.Logf("Authors: %d", len(content.Authors))

			if content.ExtractionMode == "full" && content.FullText != "" {
				t.Logf("Full text extracted: %d chars", len(content.FullText))
			} else {
				t.Log("Abstract-only mode")
			}
		})
	}
}

func TestIntegration_Extractor_Extract_InvalidID(t *testing.T) {
	extractor := NewExtractor()
	ctx := context.Background()

	_, err := extractor.Extract(ctx, "https://arxiv.org/abs/9999.99999")
	if err == nil {
		t.Error("Extract() expected error for invalid paper ID")
	}

	var extractionErr *ExtractionError
	if !errors.As(err, &extractionErr) {
		t.Errorf("Extract() error type = %T, want *ExtractionError", err)
	}
}
