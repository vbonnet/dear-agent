package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHuggingFaceExtractor_ParsePaperID(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		want    string
		wantErr bool
	}{
		{
			name:    "valid paper URL",
			url:     "https://huggingface.co/papers/2501.12345",
			want:    "2501.12345",
			wantErr: false,
		},
		{
			name:    "valid paper URL with trailing slash",
			url:     "https://huggingface.co/papers/2501.12345/",
			want:    "2501.12345",
			wantErr: false,
		},
		{
			name:    "invalid format - missing papers prefix",
			url:     "https://huggingface.co/models/2501.12345",
			wantErr: true,
		},
		{
			name:    "invalid paper ID format",
			url:     "https://huggingface.co/papers/invalid",
			wantErr: true,
		},
		{
			name:    "invalid URL",
			url:     "not-a-url",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := NewHuggingFaceExtractor(nil)
			got, err := e.parsePaperID(tt.url)

			if tt.wantErr {
				if err == nil {
					t.Errorf("parsePaperID() expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("parsePaperID() unexpected error: %v", err)
				return
			}

			if got != tt.want {
				t.Errorf("parsePaperID() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestHuggingFaceExtractor_ExtractViaAPI(t *testing.T) {
	tests := []struct {
		name         string
		paperID      string
		responseBody string
		statusCode   int
		wantErr      bool
		wantTitle    string
		wantUpvotes  int
	}{
		{
			name:    "successful API response",
			paperID: "2501.12345",
			responseBody: `{
				"id": "2501.12345",
				"title": "Test Paper Title",
				"authors": ["Author One", "Author Two"],
				"abstract": "This is a test abstract",
				"upvotes": 42,
				"collections": ["collection1", "collection2"]
			}`,
			statusCode:  http.StatusOK,
			wantErr:     false,
			wantTitle:   "Test Paper Title",
			wantUpvotes: 42,
		},
		{
			name:         "API not found",
			paperID:      "2501.99999",
			responseBody: "Not Found",
			statusCode:   http.StatusNotFound,
			wantErr:      true,
		},
		{
			name:         "API server error",
			paperID:      "2501.12345",
			responseBody: "Internal Server Error",
			statusCode:   http.StatusInternalServerError,
			wantErr:      true,
		},
		{
			name:         "invalid JSON response",
			paperID:      "2501.12345",
			responseBody: "not valid json",
			statusCode:   http.StatusOK,
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock HTTP server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Verify request path
				expectedPath := "/api/papers/" + tt.paperID
				if r.URL.Path != expectedPath {
					t.Errorf("Request path = %q, want %q", r.URL.Path, expectedPath)
				}

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.statusCode)
				w.Write([]byte(tt.responseBody))
			}))
			defer server.Close()

			// Create extractor with mock server URL
			e := NewHuggingFaceExtractor(server.Client())
			e.apiBaseURL = server.URL // Override for testing
			content, err := e.extractViaAPI(context.Background(), tt.paperID)

			if tt.wantErr {
				if err == nil {
					t.Errorf("extractViaAPI() expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("extractViaAPI() unexpected error: %v", err)
				return
			}

			if content == nil {
				t.Errorf("extractViaAPI() returned nil content")
				return
			}

			if content.Title != tt.wantTitle {
				t.Errorf("extractViaAPI() title = %q, want %q", content.Title, tt.wantTitle)
			}

			if content.Upvotes != tt.wantUpvotes {
				t.Errorf("extractViaAPI() upvotes = %d, want %d", content.Upvotes, tt.wantUpvotes)
			}
		})
	}
}

func TestHuggingFaceExtractor_ExtractViaWebScraping(t *testing.T) {
	// Create mock HTTP server with HTML content
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		html := `
<!DOCTYPE html>
<html>
<head><title>Test Paper</title></head>
<body>
	<article>
		<h1>Test Paper from Web Scraping</h1>
		<p class="author">By Author Name</p>
		<p>` + strings.Repeat("This is the paper content extracted via web scraping. ", 30) + `</p>
	</article>
</body>
</html>`
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(html))
	}))
	defer server.Close()

	e := NewHuggingFaceExtractor(server.Client())
	content, err := e.extractViaWebScraping(context.Background(), server.URL)

	if err != nil {
		t.Errorf("extractViaWebScraping() unexpected error: %v", err)
		return
	}

	if content == nil {
		t.Errorf("extractViaWebScraping() returned nil content")
		return
	}

	if len(content.Content) < 100 {
		t.Errorf("extractViaWebScraping() content too short: %d characters", len(content.Content))
	}
}

func TestHuggingFaceExtractor_FormatAPIContent(t *testing.T) {
	e := NewHuggingFaceExtractor(nil)

	resp := &HuggingFaceAPIResponse{
		ID:          "2501.12345",
		Title:       "Test Paper",
		Authors:     nil, // Will be provided separately
		Abstract:    "This is a test abstract",
		Upvotes:     42,
		Collections: []string{"collection1", "collection2"},
	}

	authors := "Author One, Author Two"
	content := e.formatAPIContent(resp, authors)

	// Check that content includes all expected fields
	expectedParts := []string{
		"PAPER: Test Paper",
		"AUTHORS: Author One, Author Two",
		"ABSTRACT:",
		"This is a test abstract",
		"UPVOTES: 42",
		"COLLECTIONS: collection1, collection2",
	}

	for _, part := range expectedParts {
		if !strings.Contains(content, part) {
			t.Errorf("formatAPIContent() missing expected part: %q", part)
		}
	}
}

func TestHuggingFaceExtractor_Name(t *testing.T) {
	e := NewHuggingFaceExtractor(nil)
	if got := e.Name(); got != "HuggingFace" {
		t.Errorf("Name() = %q, want %q", got, "HuggingFace")
	}
}

func TestHuggingFaceExtractor_ContentType(t *testing.T) {
	e := NewHuggingFaceExtractor(nil)
	// Note: This test assumes detector.ContentTypeHuggingFace exists
	// The actual value check would depend on the detector package implementation
	contentType := e.ContentType()
	if contentType.String() != "HuggingFacePaper" {
		t.Errorf("ContentType() = %v, want HuggingFacePaper", contentType)
	}
}

func TestHuggingFaceExtractor_ParseAuthors(t *testing.T) {
	tests := []struct {
		name        string
		authorsJSON string
		want        string
	}{
		{
			name:        "array format",
			authorsJSON: `["Author One", "Author Two", "Author Three"]`,
			want:        "Author One, Author Two, Author Three",
		},
		{
			name:        "object format",
			authorsJSON: `{"author1": "First Author", "author2": "Second Author"}`,
			want:        "", // Object keys are non-deterministic, just check it doesn't panic
		},
		{
			name:        "invalid JSON",
			authorsJSON: `not-valid-json`,
			want:        "not-valid-json", // Fallback to string
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := NewHuggingFaceExtractor(nil)
			got := e.parseAuthors([]byte(tt.authorsJSON))

			if tt.name == "object format" {
				// Just verify it doesn't panic for object format
				if got == "" {
					t.Logf("Object format returned empty string (expected due to map iteration)")
				}
				return
			}

			if got != tt.want {
				t.Errorf("parseAuthors() = %q, want %q", got, tt.want)
			}
		})
	}
}
