package arxiv

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/vbonnet/ai-tools/tools/gemini-deep-research/detector"
)

func TestExtractor_Name(t *testing.T) {
	e := NewExtractor()
	if got := e.Name(); got != "Arxiv" {
		t.Errorf("Name() = %q, want %q", got, "Arxiv")
	}
}

func TestExtractor_ContentType(t *testing.T) {
	e := NewExtractor()
	if got := e.ContentType(); got != detector.ContentTypeArxiv {
		t.Errorf("ContentType() = %v, want %v", got, detector.ContentTypeArxiv)
	}
}

func TestExtractPaperID(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		want    string
		wantErr bool
	}{
		{
			name: "abs URL with HTTPS",
			url:  "https://arxiv.org/abs/2601.20802",
			want: "2601.20802",
		},
		{
			name: "abs URL with HTTP",
			url:  "http://arxiv.org/abs/2601.20802",
			want: "2601.20802",
		},
		{
			name: "pdf URL",
			url:  "https://arxiv.org/pdf/2601.20802.pdf",
			want: "2601.20802",
		},
		{
			name: "versioned URL v1",
			url:  "https://arxiv.org/abs/2601.20802v1",
			want: "2601.20802",
		},
		{
			name: "versioned URL v2",
			url:  "https://arxiv.org/abs/2601.20802v2",
			want: "2601.20802",
		},
		{
			name: "5-digit suffix",
			url:  "https://arxiv.org/abs/2501.12345",
			want: "2501.12345",
		},
		{
			name:    "invalid URL - no ID",
			url:     "https://arxiv.org/abs/invalid",
			wantErr: true,
		},
		{
			name:    "invalid URL - wrong format",
			url:     "https://example.com/paper/123",
			wantErr: true,
		},
		{
			name:    "empty URL",
			url:     "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := extractPaperID(tt.url)
			if (err != nil) != tt.wantErr {
				t.Errorf("extractPaperID() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("extractPaperID() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestContent_Format(t *testing.T) {
	tests := []struct {
		name    string
		content *Content
		want    []string // substrings that should be present
	}{
		{
			name: "full content",
			content: &Content{
				Title:          "Test Paper",
				Authors:        []string{"Author One", "Author Two"},
				Abstract:       "This is the abstract.",
				PublishedDate:  "2024-01-15",
				FullText:       "Full text content here.",
				ExtractionMode: "full",
			},
			want: []string{
				"PAPER: Test Paper",
				"AUTHORS: Author One, Author Two",
				"ABSTRACT: This is the abstract.",
				"PUBLICATION DATE: 2024-01-15",
				"FULL TEXT:\nFull text content here.",
			},
		},
		{
			name: "abstract only",
			content: &Content{
				Title:          "Abstract Only Paper",
				Authors:        []string{"Single Author"},
				Abstract:       "Only abstract available.",
				PublishedDate:  "2024-01-20",
				FullText:       "",
				ExtractionMode: "abstract-only",
			},
			want: []string{
				"PAPER: Abstract Only Paper",
				"AUTHORS: Single Author",
				"ABSTRACT: Only abstract available.",
				"PUBLICATION DATE: 2024-01-20",
				"FULL TEXT: [Extraction failed, abstract only]",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.content.Format()
			for _, substr := range tt.want {
				if !strings.Contains(got, substr) {
					t.Errorf("Format() missing substring %q\nGot:\n%s", substr, got)
				}
			}
		})
	}
}

func TestExtractor_Extract_InvalidURL(t *testing.T) {
	e := NewExtractor()
	ctx := context.Background()

	_, err := e.Extract(ctx, "https://example.com/invalid")
	if err == nil {
		t.Error("Extract() expected error for invalid URL, got nil")
	}

	var extractionErr *ExtractionError
	if !errors.As(err, &extractionErr) {
		t.Errorf("Extract() error type = %T, want *ExtractionError", err)
	}
}

// Note: Full integration tests with real API calls are in api_test.go
// and require network access. These are unit tests with mocked dependencies.
