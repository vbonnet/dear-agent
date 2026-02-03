package extractors

import (
	"context"
	"testing"

	"github.com/vbonnet/ai-tools/tools/gemini-deep-research/detector"
)

// mockExtractor is a mock implementation of the Extractor interface for testing
type mockExtractor struct {
	name        string
	contentType detector.ContentType
	extractFunc func(ctx context.Context, url string) (*Content, error)
}

func (m *mockExtractor) Extract(ctx context.Context, url string) (*Content, error) {
	if m.extractFunc != nil {
		return m.extractFunc(ctx, url)
	}
	return &Content{
		Raw: "mock content",
		Metadata: map[string]interface{}{
			"source": "mock",
		},
	}, nil
}

func (m *mockExtractor) Name() string {
	return m.name
}

func (m *mockExtractor) ContentType() detector.ContentType {
	return m.contentType
}

func TestExtractor_Interface(t *testing.T) {
	tests := []struct {
		name        string
		extractor   Extractor
		wantName    string
		wantType    detector.ContentType
		extractErr  bool
		expectedRaw string
	}{
		{
			name: "mock video extractor",
			extractor: &mockExtractor{
				name:        "MockVideo",
				contentType: detector.ContentTypeVideo,
			},
			wantName:    "MockVideo",
			wantType:    detector.ContentTypeVideo,
			extractErr:  false,
			expectedRaw: "mock content",
		},
		{
			name: "mock arxiv extractor",
			extractor: &mockExtractor{
				name:        "MockArxiv",
				contentType: detector.ContentTypeArxiv,
			},
			wantName:    "MockArxiv",
			wantType:    detector.ContentTypeArxiv,
			extractErr:  false,
			expectedRaw: "mock content",
		},
		{
			name: "mock extractor with custom content",
			extractor: &mockExtractor{
				name:        "MockCustom",
				contentType: detector.ContentTypeArticle,
				extractFunc: func(ctx context.Context, url string) (*Content, error) {
					return &Content{
						Raw: "custom mock content",
						Metadata: map[string]interface{}{
							"custom": true,
						},
					}, nil
				},
			},
			wantName:    "MockCustom",
			wantType:    detector.ContentTypeArticle,
			extractErr:  false,
			expectedRaw: "custom mock content",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test Name()
			if got := tt.extractor.Name(); got != tt.wantName {
				t.Errorf("Name() = %v, want %v", got, tt.wantName)
			}

			// Test ContentType()
			if got := tt.extractor.ContentType(); got != tt.wantType {
				t.Errorf("ContentType() = %v, want %v", got, tt.wantType)
			}

			// Test Extract()
			ctx := context.Background()
			content, err := tt.extractor.Extract(ctx, "https://example.com")

			if tt.extractErr {
				if err == nil {
					t.Error("Extract() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("Extract() unexpected error: %v", err)
				return
			}

			if content == nil {
				t.Error("Extract() returned nil content")
				return
			}

			if content.Raw != tt.expectedRaw {
				t.Errorf("Extract() Raw = %v, want %v", content.Raw, tt.expectedRaw)
			}

			if content.Metadata == nil {
				t.Error("Extract() Metadata is nil")
			}
		})
	}
}

func TestContent_Structure(t *testing.T) {
	content := &Content{
		Raw: "test content",
		Metadata: map[string]interface{}{
			"title":  "Test Title",
			"author": "Test Author",
			"count":  42,
		},
	}

	if content.Raw != "test content" {
		t.Errorf("Raw = %v, want 'test content'", content.Raw)
	}

	if len(content.Metadata) != 3 {
		t.Errorf("Metadata length = %d, want 3", len(content.Metadata))
	}

	// Test metadata access
	if title, ok := content.Metadata["title"].(string); !ok || title != "Test Title" {
		t.Errorf("Metadata['title'] = %v, want 'Test Title'", content.Metadata["title"])
	}

	if count, ok := content.Metadata["count"].(int); !ok || count != 42 {
		t.Errorf("Metadata['count'] = %v, want 42", content.Metadata["count"])
	}
}
