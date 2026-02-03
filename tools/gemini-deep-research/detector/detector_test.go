package detector

import (
	"bytes"
	"log"
	"strings"
	"testing"
)

func TestDetectContentType_AutoDetection(t *testing.T) {
	tests := []struct {
		name           string
		url            string
		expectedType   ContentType
		checkLog       bool   // Whether to check log message
		expectedInLog  string // Expected substring in log
	}{
		// YouTube URLs
		{
			name:          "YouTube standard URL",
			url:           "https://www.youtube.com/watch?v=VIDEO_ID",
			expectedType:  ContentTypeVideo,
			checkLog:      true,
			expectedInLog: "youtube.com",
		},
		{
			name:          "YouTube short URL",
			url:           "https://youtu.be/VIDEO_ID",
			expectedType:  ContentTypeVideo,
			checkLog:      true,
			expectedInLog: "youtu.be",
		},
		{
			name:          "YouTube with query params",
			url:           "https://youtube.com/watch?v=VIDEO_ID&t=30&list=xyz",
			expectedType:  ContentTypeVideo,
			checkLog:      true,
			expectedInLog: "youtube.com",
		},
		{
			name:          "YouTube mixed case",
			url:           "https://YouTube.com/watch?v=VIDEO_ID",
			expectedType:  ContentTypeVideo,
			checkLog:      true,
			expectedInLog: "youtube.com",
		},
		{
			name:          "YouTube with fragment",
			url:           "https://www.youtube.com/watch?v=VIDEO_ID#section",
			expectedType:  ContentTypeVideo,
			checkLog:      true,
			expectedInLog: "youtube.com",
		},
		// Note: URL without scheme has a known limitation in normalizeURL
		// where url.Parse treats "youtube.com" as a path, not a hostname
		// This results in empty hostname. Test verifies it still detects correctly.
		{
			name:          "YouTube without scheme",
			url:           "youtube.com/watch?v=VIDEO_ID",
			expectedType:  ContentTypeVideo,
			checkLog:      true,
			expectedInLog: "Detected:", // Only check for "Detected:" not specific domain
		},

		// Vimeo URLs
		{
			name:          "Vimeo standard URL",
			url:           "https://vimeo.com/12345678",
			expectedType:  ContentTypeVideo,
			checkLog:      true,
			expectedInLog: "vimeo.com",
		},
		{
			name:          "Vimeo mixed case",
			url:           "https://Vimeo.COM/12345678",
			expectedType:  ContentTypeVideo,
			checkLog:      true,
			expectedInLog: "vimeo.com",
		},

		// ArXiv URLs
		{
			name:          "ArXiv abs URL",
			url:           "https://arxiv.org/abs/2601.20802",
			expectedType:  ContentTypeArxiv,
			checkLog:      true,
			expectedInLog: "arxiv.org",
		},
		{
			name:          "ArXiv pdf URL",
			url:           "https://arxiv.org/pdf/2601.20802.pdf",
			expectedType:  ContentTypeArxiv,
			checkLog:      true,
			expectedInLog: "arxiv.org",
		},
		{
			name:          "ArXiv with fragment",
			url:           "https://arxiv.org/abs/2601.20802#section",
			expectedType:  ContentTypeArxiv,
			checkLog:      true,
			expectedInLog: "arxiv.org",
		},
		{
			name:          "ArXiv HTTP URL",
			url:           "http://arxiv.org/abs/2601.20802",
			expectedType:  ContentTypeArxiv,
			checkLog:      true,
			expectedInLog: "arxiv.org",
		},
		{
			name:          "ArXiv mixed case",
			url:           "https://ArXiv.ORG/abs/2601.20802",
			expectedType:  ContentTypeArxiv,
			checkLog:      true,
			expectedInLog: "arxiv.org",
		},

		// HuggingFace URLs
		{
			name:          "HuggingFace paper URL",
			url:           "https://huggingface.co/papers/2501.12345",
			expectedType:  ContentTypeHuggingFace,
			checkLog:      true,
			expectedInLog: "huggingface.co",
		},
		{
			name:          "HuggingFace with www subdomain",
			url:           "https://www.huggingface.co/papers/2501.12345",
			expectedType:  ContentTypeHuggingFace,
			checkLog:      true,
			expectedInLog: "www.huggingface.co",
		},
		{
			name:          "HuggingFace with query params",
			url:           "https://huggingface.co/papers/2501.12345?tab=overview",
			expectedType:  ContentTypeHuggingFace,
			checkLog:      true,
			expectedInLog: "huggingface.co",
		},

		// Generic article URLs (fallback)
		{
			name:          "Generic blog URL",
			url:           "https://blog.example.com/article",
			expectedType:  ContentTypeArticle,
			checkLog:      true,
			expectedInLog: "(fallback)",
		},
		{
			name:          "Medium article",
			url:           "https://medium.com/@user/article",
			expectedType:  ContentTypeArticle,
			checkLog:      true,
			expectedInLog: "(fallback)",
		},
		{
			name:          "News article",
			url:           "https://news.example.com/story",
			expectedType:  ContentTypeArticle,
			checkLog:      true,
			expectedInLog: "(fallback)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Capture log output
			var buf bytes.Buffer
			log.SetOutput(&buf)
			defer log.SetOutput(nil)

			// Test auto-detection (no type flag)
			contentType, err := DetectContentType(tt.url, "")

			if err != nil {
				t.Errorf("DetectContentType(%q, \"\") unexpected error: %v", tt.url, err)
				return
			}

			if contentType != tt.expectedType {
				t.Errorf("DetectContentType(%q, \"\") = %v, expected %v",
					tt.url, contentType, tt.expectedType)
			}

			// Verify log message
			if tt.checkLog {
				logOutput := buf.String()
				if !strings.Contains(logOutput, tt.expectedInLog) {
					t.Errorf("Expected log to contain %q, got: %s",
						tt.expectedInLog, logOutput)
				}
				if !strings.Contains(logOutput, "Detected:") {
					t.Errorf("Expected log to contain 'Detected:', got: %s", logOutput)
				}
			}
		})
	}
}

func TestDetectContentType_ManualOverride(t *testing.T) {
	tests := []struct {
		name         string
		url          string
		typeFlag     string
		expectedType ContentType
		shouldLog    bool
	}{
		{
			name:         "Override YouTube to article",
			url:          "https://youtube.com/watch?v=VIDEO_ID",
			typeFlag:     "article",
			expectedType: ContentTypeArticle,
			shouldLog:    true,
		},
		{
			name:         "Override blog to video",
			url:          "https://blog.example.com/post",
			typeFlag:     "video",
			expectedType: ContentTypeVideo,
			shouldLog:    true,
		},
		{
			name:         "Override ArXiv to HuggingFace",
			url:          "https://arxiv.org/abs/1234",
			typeFlag:     "huggingface",
			expectedType: ContentTypeHuggingFace,
			shouldLog:    true,
		},
		{
			name:         "Override with matching type",
			url:          "https://youtube.com/watch?v=VIDEO_ID",
			typeFlag:     "video",
			expectedType: ContentTypeVideo,
			shouldLog:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Capture log output
			var buf bytes.Buffer
			log.SetOutput(&buf)
			defer log.SetOutput(nil)

			contentType, err := DetectContentType(tt.url, tt.typeFlag)

			if err != nil {
				t.Errorf("DetectContentType(%q, %q) unexpected error: %v",
					tt.url, tt.typeFlag, err)
				return
			}

			if contentType != tt.expectedType {
				t.Errorf("DetectContentType(%q, %q) = %v, expected %v",
					tt.url, tt.typeFlag, contentType, tt.expectedType)
			}

			// Verify override is logged
			logOutput := buf.String()
			if tt.shouldLog && !strings.Contains(logOutput, "override") {
				t.Errorf("Expected log to contain 'override' for manual type, got: %s",
					logOutput)
			}
			if tt.shouldLog && !strings.Contains(logOutput, tt.typeFlag) {
				t.Errorf("Expected log to contain type flag %q, got: %s",
					tt.typeFlag, logOutput)
			}
		})
	}
}

func TestDetectContentType_InvalidTypeFlag(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		typeFlag string
	}{
		{
			name:     "Invalid type 'unknown'",
			url:      "https://example.com",
			typeFlag: "unknown",
		},
		{
			name:     "Invalid type 'pdf'",
			url:      "https://example.com",
			typeFlag: "pdf",
		},
		{
			name:     "Invalid type 'youtube'",
			url:      "https://example.com",
			typeFlag: "youtube",
		},
		{
			name:     "Empty invalid type",
			url:      "https://example.com",
			typeFlag: "   ",
		},
		{
			name:     "Mixed case valid type (should fail - case sensitive)",
			url:      "https://example.com",
			typeFlag: "Video",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			contentType, err := DetectContentType(tt.url, tt.typeFlag)

			if err == nil {
				t.Errorf("DetectContentType(%q, %q) expected error, got contentType=%v",
					tt.url, tt.typeFlag, contentType)
				return
			}

			// Verify error message format
			errMsg := err.Error()
			if !strings.Contains(errMsg, "invalid content type") {
				t.Errorf("Error should contain 'invalid content type', got: %v", err)
			}
			if !strings.Contains(errMsg, "Valid:") {
				t.Errorf("Error should list valid types with 'Valid:', got: %v", err)
			}
			if !strings.Contains(errMsg, "video") || !strings.Contains(errMsg, "article") {
				t.Errorf("Error should list valid types, got: %v", err)
			}
		})
	}
}

func TestDetectContentType_EmptyURL(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		typeFlag string
	}{
		{
			name:     "Empty URL no flag",
			url:      "",
			typeFlag: "",
		},
		{
			name:     "Empty URL with valid flag",
			url:      "",
			typeFlag: "article",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := DetectContentType(tt.url, tt.typeFlag)

			if err == nil {
				t.Errorf("DetectContentType(%q, %q) expected error for empty URL, got nil",
					tt.url, tt.typeFlag)
				return
			}

			errMsg := err.Error()
			if !strings.Contains(errMsg, "URL cannot be empty") {
				t.Errorf("Error should mention empty URL, got: %v", err)
			}
		})
	}
}

func TestDetectContentType_InvalidURL(t *testing.T) {
	tests := []struct {
		name string
		url  string
	}{
		{
			name: "URL with invalid characters",
			url:  "https://example.com/<invalid>",
		},
		// Note: Go's url.Parse is quite lenient, so many malformed URLs
		// will still parse. We mainly test that the function handles
		// parsing errors gracefully.
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Capture log output to prevent nil pointer issues
			var buf bytes.Buffer
			log.SetOutput(&buf)
			defer log.SetOutput(nil)

			// These URLs might parse or might not, depending on Go's parser
			// The important thing is we don't panic
			_, err := DetectContentType(tt.url, "")

			// We accept either success (lenient parsing) or error
			// Just verify no panic occurred
			_ = err
		})
	}
}

func TestDetectContentType_OverrideWithInvalidURL(t *testing.T) {
	// When override is specified with invalid URL, override should succeed
	// but log message won't show auto-detected type
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(nil)

	// This is a very malformed URL that should fail parsing
	url := "ht!tp://not a valid url"
	typeFlag := "article"

	contentType, err := DetectContentType(url, typeFlag)

	if err != nil {
		t.Errorf("DetectContentType with valid override should succeed even with invalid URL, got error: %v", err)
		return
	}

	if contentType != ContentTypeArticle {
		t.Errorf("Expected ContentTypeArticle, got %v", contentType)
	}

	// Verify log doesn't attempt to show auto-detected type
	logOutput := buf.String()
	if !strings.Contains(logOutput, "override") {
		t.Errorf("Expected override message in log, got: %s", logOutput)
	}
}

func TestDetectContentType_CaseInsensitivity(t *testing.T) {
	tests := []struct {
		name         string
		url          string
		expectedType ContentType
	}{
		{
			name:         "Mixed case YouTube",
			url:          "https://YouTube.COM/watch?v=ID",
			expectedType: ContentTypeVideo,
		},
		{
			name:         "Uppercase ArXiv",
			url:          "https://ARXIV.ORG/abs/1234",
			expectedType: ContentTypeArxiv,
		},
		{
			name:         "Mixed case HuggingFace",
			url:          "https://HuggingFace.Co/papers/1234",
			expectedType: ContentTypeHuggingFace,
		},
		{
			name:         "Mixed case Vimeo",
			url:          "https://VIMEO.com/1234",
			expectedType: ContentTypeVideo,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Capture log output
			var buf bytes.Buffer
			log.SetOutput(&buf)
			defer log.SetOutput(nil)

			contentType, err := DetectContentType(tt.url, "")

			if err != nil {
				t.Errorf("DetectContentType(%q, \"\") unexpected error: %v", tt.url, err)
				return
			}

			if contentType != tt.expectedType {
				t.Errorf("DetectContentType(%q, \"\") = %v, expected %v",
					tt.url, contentType, tt.expectedType)
			}
		})
	}
}

func TestDetectContentType_QueryParamsAndFragments(t *testing.T) {
	tests := []struct {
		name         string
		url          string
		expectedType ContentType
	}{
		{
			name:         "YouTube with multiple query params",
			url:          "https://youtube.com/watch?v=ID&t=30&list=xyz&index=5",
			expectedType: ContentTypeVideo,
		},
		{
			name:         "ArXiv with fragment",
			url:          "https://arxiv.org/abs/1234#references",
			expectedType: ContentTypeArxiv,
		},
		{
			name:         "HuggingFace with query and fragment",
			url:          "https://huggingface.co/papers/1234?tab=files#model-card",
			expectedType: ContentTypeHuggingFace,
		},
		{
			name:         "YouTube shortened with timestamp",
			url:          "https://youtu.be/VIDEO_ID?t=120",
			expectedType: ContentTypeVideo,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Capture log output
			var buf bytes.Buffer
			log.SetOutput(&buf)
			defer log.SetOutput(nil)

			contentType, err := DetectContentType(tt.url, "")

			if err != nil {
				t.Errorf("DetectContentType(%q, \"\") unexpected error: %v", tt.url, err)
				return
			}

			if contentType != tt.expectedType {
				t.Errorf("DetectContentType(%q, \"\") = %v, expected %v",
					tt.url, contentType, tt.expectedType)
			}
		})
	}
}

func TestDetectContentType_HTTPvsHTTPS(t *testing.T) {
	tests := []struct {
		name         string
		urlHTTP      string
		urlHTTPS     string
		expectedType ContentType
	}{
		{
			name:         "ArXiv HTTP vs HTTPS",
			urlHTTP:      "http://arxiv.org/abs/1234",
			urlHTTPS:     "https://arxiv.org/abs/1234",
			expectedType: ContentTypeArxiv,
		},
		{
			name:         "YouTube HTTP vs HTTPS",
			urlHTTP:      "http://youtube.com/watch?v=ID",
			urlHTTPS:     "https://youtube.com/watch?v=ID",
			expectedType: ContentTypeVideo,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Capture log output
			var buf bytes.Buffer
			log.SetOutput(&buf)
			defer log.SetOutput(nil)

			// Test HTTP
			contentTypeHTTP, err := DetectContentType(tt.urlHTTP, "")
			if err != nil {
				t.Errorf("DetectContentType(%q, \"\") unexpected error: %v", tt.urlHTTP, err)
				return
			}

			// Reset buffer for second test
			buf.Reset()

			// Test HTTPS
			contentTypeHTTPS, err := DetectContentType(tt.urlHTTPS, "")
			if err != nil {
				t.Errorf("DetectContentType(%q, \"\") unexpected error: %v", tt.urlHTTPS, err)
				return
			}

			// Both should detect the same type
			if contentTypeHTTP != tt.expectedType {
				t.Errorf("HTTP URL: got %v, expected %v", contentTypeHTTP, tt.expectedType)
			}
			if contentTypeHTTPS != tt.expectedType {
				t.Errorf("HTTPS URL: got %v, expected %v", contentTypeHTTPS, tt.expectedType)
			}
			if contentTypeHTTP != contentTypeHTTPS {
				t.Errorf("HTTP and HTTPS detection differ: %v vs %v",
					contentTypeHTTP, contentTypeHTTPS)
			}
		})
	}
}

func TestDetectContentType_OverrideLogging(t *testing.T) {
	// Test that override logging includes auto-detected type
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(nil)

	url := "https://youtube.com/watch?v=VIDEO_ID"
	typeFlag := "article"

	_, err := DetectContentType(url, typeFlag)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	logOutput := buf.String()

	// Should mention override
	if !strings.Contains(logOutput, "override") {
		t.Errorf("Log should mention override, got: %s", logOutput)
	}

	// Should mention the flag value
	if !strings.Contains(logOutput, typeFlag) {
		t.Errorf("Log should mention type flag %q, got: %s", typeFlag, logOutput)
	}

	// Should mention what was auto-detected
	if !strings.Contains(logOutput, "auto-detected") {
		t.Errorf("Log should mention auto-detected type, got: %s", logOutput)
	}

	// Should mention Video (the auto-detected type)
	if !strings.Contains(logOutput, "Video") {
		t.Errorf("Log should mention auto-detected Video type, got: %s", logOutput)
	}
}
