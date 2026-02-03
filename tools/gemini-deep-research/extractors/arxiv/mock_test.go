package arxiv

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// MockAPIClient for testing
type MockAPIClient struct {
	metadata *PaperMetadata
	pdfData  []byte
	err      error
}

func (m *MockAPIClient) FetchMetadata(ctx context.Context, paperID string) (*PaperMetadata, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.metadata, nil
}

func (m *MockAPIClient) DownloadPDF(ctx context.Context, paperID string) ([]byte, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.pdfData, nil
}

// MockPDFParser for testing
type MockPDFParser struct {
	text string
	err  error
}

func (m *MockPDFParser) ExtractFromArxiv(ctx context.Context, paperID string) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	return m.text, nil
}

func TestExtractor_Extract_WithMocks_Success(t *testing.T) {
	// Note: Since we can't easily inject mocks due to struct fields,
	// we'll test the real implementation with edge cases instead
	t.Skip("Requires dependency injection refactoring for proper mocking")

	// Create extractor with mocked dependencies would be:
	// e := &Extractor{...}
}

func TestExtractor_Extract_MetadataError(t *testing.T) {
	// Test that metadata errors are propagated correctly
	e := NewExtractor()
	ctx := context.Background()

	// Invalid URL should fail at paper ID extraction
	_, err := e.Extract(ctx, "https://example.com/not-arxiv")
	if err == nil {
		t.Error("Extract() expected error for invalid URL")
	}

	var extractionErr *ExtractionError
	if !errors.As(err, &extractionErr) {
		t.Errorf("Extract() error type = %T, want *ExtractionError", err)
	}

	if !errors.Is(err, ErrInvalidArxivID) {
		t.Error("Extract() should return ErrInvalidArxivID for invalid URL")
	}
}

func TestContent_Format_Variations(t *testing.T) {
	tests := []struct {
		name    string
		content *Content
		checks  func(t *testing.T, output string)
	}{
		{
			name: "single author",
			content: &Content{
				Title:          "Single Author Paper",
				Authors:        []string{"Alice Smith"},
				Abstract:       "Abstract text.",
				PublishedDate:  "2024-01-01",
				FullText:       "Full text here.",
				ExtractionMode: "full",
			},
			checks: func(t *testing.T, output string) {
				if !strings.Contains(output, "AUTHORS: Alice Smith") {
					t.Error("Should format single author correctly")
				}
			},
		},
		{
			name: "multiple authors",
			content: &Content{
				Title:          "Multi-Author Paper",
				Authors:        []string{"Alice", "Bob", "Charlie"},
				Abstract:       "Abstract.",
				PublishedDate:  "2024-01-02",
				FullText:       "",
				ExtractionMode: "abstract-only",
			},
			checks: func(t *testing.T, output string) {
				if !strings.Contains(output, "AUTHORS: Alice, Bob, Charlie") {
					t.Error("Should join multiple authors with commas")
				}
				if !strings.Contains(output, "[Extraction failed, abstract only]") {
					t.Error("Should indicate abstract-only mode")
				}
			},
		},
		{
			name: "empty full text",
			content: &Content{
				Title:          "Abstract Only",
				Authors:        []string{"Author"},
				Abstract:       "Abstract.",
				PublishedDate:  "2024-01-03",
				FullText:       "",
				ExtractionMode: "abstract-only",
			},
			checks: func(t *testing.T, output string) {
				if strings.Contains(output, "FULL TEXT:\n\n") {
					t.Error("Should not have empty full text section")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := tt.content.Format()
			tt.checks(t, output)
		})
	}
}

func TestExtractPaperID_EdgeCases(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		want    string
		wantErr bool
	}{
		{
			name:    "URL with query parameters",
			url:     "https://arxiv.org/abs/2601.20802?context=cs",
			want:    "2601.20802",
			wantErr: false,
		},
		{
			name:    "URL with fragment",
			url:     "https://arxiv.org/abs/2601.20802#section",
			want:    "2601.20802",
			wantErr: false,
		},
		{
			name:    "Old format with year/month",
			url:     "https://arxiv.org/abs/1234.5678",
			want:    "1234.5678",
			wantErr: false,
		},
		{
			name:    "Version 10",
			url:     "https://arxiv.org/abs/2601.20802v10",
			want:    "2601.20802",
			wantErr: false,
		},
		{
			name:    "Just digits",
			url:     "202120802",
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

func TestAPIClient_RateLimiting_EdgeCases(t *testing.T) {
	client := NewAPIClient()

	// Test that first call sets lastRequestAt
	if !client.lastRequestAt.IsZero() {
		t.Error("New client should have zero lastRequestAt")
	}

	client.enforceRateLimit()
	if client.lastRequestAt.IsZero() {
		t.Error("After first call, lastRequestAt should be set")
	}

	// Test that the timestamp is updated on each call
	first := client.lastRequestAt
	client.enforceRateLimit()
	second := client.lastRequestAt

	if !second.After(first) {
		t.Error("Subsequent calls should update lastRequestAt")
	}
}

func TestPDFParser_NewPDFParser(t *testing.T) {
	parser := NewPDFParser()

	if parser == nil {
		t.Fatal("NewPDFParser() returned nil")
	}

	if parser.apiClient == nil {
		t.Error("NewPDFParser() should initialize apiClient")
	}
}

func TestExtractor_Interfaces(t *testing.T) {
	e := NewExtractor()

	// Test that Name() returns expected value
	if name := e.Name(); name != "Arxiv" {
		t.Errorf("Name() = %q, want %q", name, "Arxiv")
	}

	// Test that ContentType() returns correct type
	contentType := e.ContentType()
	if contentType.String() != "ArxivPaper" {
		t.Errorf("ContentType().String() = %q, want %q", contentType.String(), "ArxivPaper")
	}
}
