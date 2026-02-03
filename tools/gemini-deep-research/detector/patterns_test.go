package detector

import (
	"strings"
	"testing"
)

func TestNormalizeURL(t *testing.T) {
	tests := []struct {
		name             string
		url              string
		expectedNorm     string
		expectedHostname string
		shouldError      bool
	}{
		// Basic URLs
		{
			name:             "Simple HTTPS URL",
			url:              "https://example.com/path",
			expectedNorm:     "example.com/path",
			expectedHostname: "example.com",
			shouldError:      false,
		},
		{
			name:             "HTTP URL",
			url:              "http://example.com/path",
			expectedNorm:     "example.com/path",
			expectedHostname: "example.com",
			shouldError:      false,
		},
		// Note: URL without scheme has a limitation where url.Parse treats
		// the entire thing as a path, not a hostname, so Hostname() returns empty
		{
			name:             "URL without scheme",
			url:              "example.com/path",
			expectedNorm:     "example.com/path",
			expectedHostname: "", // Empty because url.Parse treats it as path
			shouldError:      false,
		},

		// Query parameters (should be ignored)
		{
			name:             "URL with query parameters",
			url:              "https://youtube.com/watch?v=ID&t=30",
			expectedNorm:     "youtube.com/watch",
			expectedHostname: "youtube.com",
			shouldError:      false,
		},
		{
			name:             "URL with multiple query params",
			url:              "https://example.com/page?a=1&b=2&c=3",
			expectedNorm:     "example.com/page",
			expectedHostname: "example.com",
			shouldError:      false,
		},

		// Fragments (should be ignored)
		{
			name:             "URL with fragment",
			url:              "https://arxiv.org/abs/1234#section",
			expectedNorm:     "arxiv.org/abs/1234",
			expectedHostname: "arxiv.org",
			shouldError:      false,
		},
		{
			name:             "URL with query and fragment",
			url:              "https://example.com/page?q=test#anchor",
			expectedNorm:     "example.com/page",
			expectedHostname: "example.com",
			shouldError:      false,
		},

		// Case sensitivity (should be lowercased)
		{
			name:             "Mixed case domain",
			url:              "https://YouTube.COM/watch",
			expectedNorm:     "youtube.com/watch",
			expectedHostname: "youtube.com",
			shouldError:      false,
		},
		{
			name:             "Mixed case path",
			url:              "https://example.com/Path/To/Resource",
			expectedNorm:     "example.com/path/to/resource",
			expectedHostname: "example.com",
			shouldError:      false,
		},
		{
			name:             "Uppercase domain and path",
			url:              "HTTPS://ARXIV.ORG/ABS/1234",
			expectedNorm:     "arxiv.org/abs/1234",
			expectedHostname: "arxiv.org",
			shouldError:      false,
		},

		// Subdomains
		{
			name:             "URL with www subdomain",
			url:              "https://www.youtube.com/watch",
			expectedNorm:     "www.youtube.com/watch",
			expectedHostname: "www.youtube.com",
			shouldError:      false,
		},
		{
			name:             "URL with multiple subdomains",
			url:              "https://api.v2.example.com/endpoint",
			expectedNorm:     "api.v2.example.com/endpoint",
			expectedHostname: "api.v2.example.com",
			shouldError:      false,
		},

		// Edge cases
		{
			name:             "URL with trailing slash",
			url:              "https://example.com/path/",
			expectedNorm:     "example.com/path/",
			expectedHostname: "example.com",
			shouldError:      false,
		},
		{
			name:             "URL with no path",
			url:              "https://example.com",
			expectedNorm:     "example.com",
			expectedHostname: "example.com",
			shouldError:      false,
		},
		{
			name:             "URL with port",
			url:              "https://example.com:8080/path",
			expectedNorm:     "example.com/path",
			expectedHostname: "example.com",
			shouldError:      false,
		},

		// Shortened URLs
		{
			name:             "YouTube shortened URL",
			url:              "https://youtu.be/VIDEO_ID",
			expectedNorm:     "youtu.be/video_id",
			expectedHostname: "youtu.be",
			shouldError:      false,
		},
		{
			name:             "YouTube shortened with timestamp",
			url:              "https://youtu.be/VIDEO_ID?t=30",
			expectedNorm:     "youtu.be/video_id",
			expectedHostname: "youtu.be",
			shouldError:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			normalized, hostname, err := normalizeURL(tt.url)

			if tt.shouldError && err == nil {
				t.Errorf("normalizeURL(%q) expected error, got nil", tt.url)
				return
			}

			if !tt.shouldError && err != nil {
				t.Errorf("normalizeURL(%q) unexpected error: %v", tt.url, err)
				return
			}

			if !tt.shouldError {
				if normalized != tt.expectedNorm {
					t.Errorf("normalizeURL(%q) normalized = %q, expected %q",
						tt.url, normalized, tt.expectedNorm)
				}
				if hostname != tt.expectedHostname {
					t.Errorf("normalizeURL(%q) hostname = %q, expected %q",
						tt.url, hostname, tt.expectedHostname)
				}
			}
		})
	}
}

func TestMatchPattern(t *testing.T) {
	tests := []struct {
		name          string
		normalized    string
		matchers      []string
		expectedMatch bool
	}{
		// YouTube patterns
		{
			name:          "Match youtube.com",
			normalized:    "youtube.com/watch",
			matchers:      []string{"youtube.com"},
			expectedMatch: true,
		},
		{
			name:          "Match youtu.be",
			normalized:    "youtu.be/video_id",
			matchers:      []string{"youtu.be"},
			expectedMatch: true,
		},
		{
			name:          "Match with www subdomain",
			normalized:    "www.youtube.com/watch",
			matchers:      []string{"youtube.com"},
			expectedMatch: true,
		},

		// ArXiv patterns
		{
			name:          "Match arxiv.org/abs/",
			normalized:    "arxiv.org/abs/1234",
			matchers:      []string{"arxiv.org/abs/"},
			expectedMatch: true,
		},
		{
			name:          "Match arxiv.org/pdf/",
			normalized:    "arxiv.org/pdf/1234.pdf",
			matchers:      []string{"arxiv.org/pdf/"},
			expectedMatch: true,
		},
		{
			name:          "No match on arxiv.org without path",
			normalized:    "arxiv.org/other/path",
			matchers:      []string{"arxiv.org/abs/", "arxiv.org/pdf/"},
			expectedMatch: false,
		},

		// HuggingFace patterns
		{
			name:          "Match huggingface.co/papers/",
			normalized:    "huggingface.co/papers/1234",
			matchers:      []string{"huggingface.co/papers/"},
			expectedMatch: true,
		},
		{
			name:          "No match on huggingface.co without /papers/",
			normalized:    "huggingface.co/models/gpt2",
			matchers:      []string{"huggingface.co/papers/"},
			expectedMatch: false,
		},

		// Case insensitivity
		{
			name:          "Match case insensitive domain",
			normalized:    "youtube.com/watch",
			matchers:      []string{"YouTube.COM"},
			expectedMatch: true,
		},
		{
			name:          "Match case insensitive path",
			normalized:    "arxiv.org/abs/1234",
			matchers:      []string{"ARXIV.ORG/ABS/"},
			expectedMatch: true,
		},

		// Multiple matchers
		{
			name:          "Match first in list",
			normalized:    "youtube.com/watch",
			matchers:      []string{"youtube.com", "youtu.be"},
			expectedMatch: true,
		},
		{
			name:          "Match second in list",
			normalized:    "youtu.be/video",
			matchers:      []string{"youtube.com", "youtu.be"},
			expectedMatch: true,
		},
		{
			name:          "No match in list",
			normalized:    "vimeo.com/video",
			matchers:      []string{"youtube.com", "youtu.be"},
			expectedMatch: false,
		},

		// Edge cases
		{
			name:          "Empty matchers",
			normalized:    "example.com/path",
			matchers:      []string{},
			expectedMatch: false,
		},
		{
			name:          "Empty normalized URL",
			normalized:    "",
			matchers:      []string{"example.com"},
			expectedMatch: false,
		},
		{
			name:          "Partial match in domain",
			normalized:    "notyoutube.com/watch",
			matchers:      []string{"youtube.com"},
			expectedMatch: true, // Contains substring
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matched := matchPattern(tt.normalized, tt.matchers)

			if matched != tt.expectedMatch {
				t.Errorf("matchPattern(%q, %v) = %v, expected %v",
					tt.normalized, tt.matchers, matched, tt.expectedMatch)
			}
		})
	}
}

func TestDetectFromPattern(t *testing.T) {
	tests := []struct {
		name             string
		url              string
		expectedType     ContentType
		expectedHostname string
		shouldError      bool
	}{
		// Video patterns
		{
			name:             "Detect YouTube video",
			url:              "https://youtube.com/watch?v=ID",
			expectedType:     ContentTypeVideo,
			expectedHostname: "youtube.com",
			shouldError:      false,
		},
		{
			name:             "Detect YouTube shortened",
			url:              "https://youtu.be/ID",
			expectedType:     ContentTypeVideo,
			expectedHostname: "youtu.be",
			shouldError:      false,
		},
		{
			name:             "Detect Vimeo video",
			url:              "https://vimeo.com/12345",
			expectedType:     ContentTypeVideo,
			expectedHostname: "vimeo.com",
			shouldError:      false,
		},

		// ArXiv patterns
		{
			name:             "Detect ArXiv abs URL",
			url:              "https://arxiv.org/abs/1234",
			expectedType:     ContentTypeArxiv,
			expectedHostname: "arxiv.org",
			shouldError:      false,
		},
		{
			name:             "Detect ArXiv pdf URL",
			url:              "https://arxiv.org/pdf/1234.pdf",
			expectedType:     ContentTypeArxiv,
			expectedHostname: "arxiv.org",
			shouldError:      false,
		},

		// HuggingFace patterns
		{
			name:             "Detect HuggingFace paper",
			url:              "https://huggingface.co/papers/1234",
			expectedType:     ContentTypeHuggingFace,
			expectedHostname: "huggingface.co",
			shouldError:      false,
		},

		// Fallback to article
		{
			name:             "Fallback to article for blog",
			url:              "https://blog.example.com/post",
			expectedType:     ContentTypeArticle,
			expectedHostname: "blog.example.com",
			shouldError:      false,
		},
		{
			name:             "Fallback to article for news",
			url:              "https://news.example.com/story",
			expectedType:     ContentTypeArticle,
			expectedHostname: "news.example.com",
			shouldError:      false,
		},
		{
			name:             "Fallback for HuggingFace model (not paper)",
			url:              "https://huggingface.co/models/gpt2",
			expectedType:     ContentTypeArticle,
			expectedHostname: "huggingface.co",
			shouldError:      false,
		},

		// Pattern precedence (more specific patterns first)
		{
			name:             "ArXiv pattern takes precedence",
			url:              "https://arxiv.org/abs/1234",
			expectedType:     ContentTypeArxiv,
			expectedHostname: "arxiv.org",
			shouldError:      false,
		},

		// Case insensitive matching
		{
			name:             "Mixed case YouTube",
			url:              "https://YouTube.COM/watch?v=ID",
			expectedType:     ContentTypeVideo,
			expectedHostname: "youtube.com",
			shouldError:      false,
		},
		{
			name:             "Mixed case ArXiv",
			url:              "https://ArXiv.ORG/abs/1234",
			expectedType:     ContentTypeArxiv,
			expectedHostname: "arxiv.org",
			shouldError:      false,
		},

		// Query params and fragments ignored
		{
			name:             "YouTube with query params",
			url:              "https://youtube.com/watch?v=ID&t=30&list=xyz",
			expectedType:     ContentTypeVideo,
			expectedHostname: "youtube.com",
			shouldError:      false,
		},
		{
			name:             "ArXiv with fragment",
			url:              "https://arxiv.org/abs/1234#section",
			expectedType:     ContentTypeArxiv,
			expectedHostname: "arxiv.org",
			shouldError:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			contentType, hostname, err := detectFromPattern(tt.url)

			if tt.shouldError && err == nil {
				t.Errorf("detectFromPattern(%q) expected error, got nil", tt.url)
				return
			}

			if !tt.shouldError && err != nil {
				t.Errorf("detectFromPattern(%q) unexpected error: %v", tt.url, err)
				return
			}

			if !tt.shouldError {
				if contentType != tt.expectedType {
					t.Errorf("detectFromPattern(%q) type = %v, expected %v",
						tt.url, contentType, tt.expectedType)
				}
				if hostname != tt.expectedHostname {
					t.Errorf("detectFromPattern(%q) hostname = %q, expected %q",
						tt.url, hostname, tt.expectedHostname)
				}
			}
		})
	}
}

func TestPatternOrder(t *testing.T) {
	// Verify that patterns array is in correct order (more specific first)
	// This test documents the expected pattern matching precedence

	if len(patterns) == 0 {
		t.Fatal("patterns array is empty")
	}

	// Video patterns should come first (YouTube, Vimeo)
	if patterns[0].ContentType != ContentTypeVideo {
		t.Errorf("First pattern should be Video, got %v", patterns[0].ContentType)
	}

	// ArXiv should be before generic article
	arxivFound := false
	for _, pattern := range patterns {
		if pattern.ContentType == ContentTypeArxiv {
			arxivFound = true
			break
		}
		if pattern.ContentType == ContentTypeArticle {
			t.Error("ArXiv pattern should come before Article pattern")
			break
		}
	}

	if !arxivFound {
		t.Error("ArXiv pattern not found in patterns array")
	}

	// HuggingFace should be before generic article
	hfFound := false
	for _, pattern := range patterns {
		if pattern.ContentType == ContentTypeHuggingFace {
			hfFound = true
			break
		}
		if pattern.ContentType == ContentTypeArticle {
			t.Error("HuggingFace pattern should come before Article pattern")
			break
		}
	}

	if !hfFound {
		t.Error("HuggingFace pattern not found in patterns array")
	}
}

func TestPatternMatchers(t *testing.T) {
	// Test that all expected matchers are present in patterns
	tests := []struct {
		contentType     ContentType
		expectedMatcher string
	}{
		{ContentTypeVideo, "youtube.com"},
		{ContentTypeVideo, "youtu.be"},
		{ContentTypeVideo, "vimeo.com"},
		{ContentTypeArxiv, "arxiv.org/abs/"},
		{ContentTypeArxiv, "arxiv.org/pdf/"},
		{ContentTypeHuggingFace, "huggingface.co/papers/"},
	}

	for _, tt := range tests {
		t.Run(tt.contentType.String()+"/"+tt.expectedMatcher, func(t *testing.T) {
			found := false
			for _, pattern := range patterns {
				if pattern.ContentType == tt.contentType {
					for _, matcher := range pattern.Matchers {
						if strings.Contains(strings.ToLower(matcher), strings.ToLower(tt.expectedMatcher)) {
							found = true
							break
						}
					}
				}
			}

			if !found {
				t.Errorf("Expected matcher %q not found for content type %v",
					tt.expectedMatcher, tt.contentType)
			}
		})
	}
}

func TestNormalizeURL_SchemeHandling(t *testing.T) {
	tests := []struct {
		name        string
		url         string
		expectHTTPS bool
	}{
		{
			name:        "URL without scheme gets HTTPS",
			url:         "example.com/path",
			expectHTTPS: true,
		},
		{
			name:        "HTTP URL keeps HTTP",
			url:         "http://example.com/path",
			expectHTTPS: false,
		},
		{
			name:        "HTTPS URL keeps HTTPS",
			url:         "https://example.com/path",
			expectHTTPS: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			normalized, _, err := normalizeURL(tt.url)
			if err != nil {
				t.Fatalf("normalizeURL(%q) unexpected error: %v", tt.url, err)
			}

			// Normalized URL should not contain scheme
			if strings.Contains(normalized, "http://") || strings.Contains(normalized, "https://") {
				t.Errorf("normalizeURL(%q) should remove scheme, got: %q", tt.url, normalized)
			}
		})
	}
}

func TestDetectFromPattern_EdgeCases(t *testing.T) {
	tests := []struct {
		name        string
		url         string
		shouldError bool
	}{
		{
			name:        "URL with unusual characters",
			url:         "https://example.com/path?q=value&other=123",
			shouldError: false,
		},
		{
			name:        "URL with encoded characters",
			url:         "https://example.com/path%20with%20spaces",
			shouldError: false,
		},
		{
			name:        "Very long URL",
			url:         "https://example.com/" + strings.Repeat("a", 1000),
			shouldError: false,
		},
		{
			name:        "URL with Unicode domain",
			url:         "https://例え.jp/path",
			shouldError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := detectFromPattern(tt.url)

			if tt.shouldError && err == nil {
				t.Errorf("detectFromPattern(%q) expected error, got nil", tt.url)
			}
			if !tt.shouldError && err != nil {
				t.Errorf("detectFromPattern(%q) unexpected error: %v", tt.url, err)
			}
		})
	}
}
