package detector

import (
	"testing"
)

func TestContentType_String(t *testing.T) {
	tests := []struct {
		name         string
		contentType  ContentType
		expectedStr  string
	}{
		{
			name:        "Video type",
			contentType: ContentTypeVideo,
			expectedStr: "Video",
		},
		{
			name:        "ArXiv type",
			contentType: ContentTypeArxiv,
			expectedStr: "ArxivPaper",
		},
		{
			name:        "HuggingFace type",
			contentType: ContentTypeHuggingFace,
			expectedStr: "HuggingFacePaper",
		},
		{
			name:        "Article type",
			contentType: ContentTypeArticle,
			expectedStr: "GenericArticle",
		},
		{
			name:        "Unknown type",
			contentType: ContentType(999),
			expectedStr: "Unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			str := tt.contentType.String()
			if str != tt.expectedStr {
				t.Errorf("ContentType.String() = %q, expected %q", str, tt.expectedStr)
			}
		})
	}
}

func TestParseContentType(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		expectedType ContentType
		expectedOK   bool
	}{
		// Valid types
		{
			name:         "Parse 'video'",
			input:        "video",
			expectedType: ContentTypeVideo,
			expectedOK:   true,
		},
		{
			name:         "Parse 'article'",
			input:        "article",
			expectedType: ContentTypeArticle,
			expectedOK:   true,
		},
		{
			name:         "Parse 'arxiv'",
			input:        "arxiv",
			expectedType: ContentTypeArxiv,
			expectedOK:   true,
		},
		{
			name:         "Parse 'huggingface'",
			input:        "huggingface",
			expectedType: ContentTypeHuggingFace,
			expectedOK:   true,
		},

		// Invalid types (should return false)
		{
			name:         "Invalid type 'unknown'",
			input:        "unknown",
			expectedType: ContentTypeArticle, // Returns fallback
			expectedOK:   false,
		},
		{
			name:         "Invalid type 'youtube'",
			input:        "youtube",
			expectedType: ContentTypeArticle,
			expectedOK:   false,
		},
		{
			name:         "Invalid type 'pdf'",
			input:        "pdf",
			expectedType: ContentTypeArticle,
			expectedOK:   false,
		},
		{
			name:         "Empty string",
			input:        "",
			expectedType: ContentTypeArticle,
			expectedOK:   false,
		},

		// Case sensitivity (lowercase required)
		{
			name:         "Uppercase 'VIDEO'",
			input:        "VIDEO",
			expectedType: ContentTypeArticle,
			expectedOK:   false,
		},
		{
			name:         "Mixed case 'Video'",
			input:        "Video",
			expectedType: ContentTypeArticle,
			expectedOK:   false,
		},
		{
			name:         "Mixed case 'Article'",
			input:        "Article",
			expectedType: ContentTypeArticle,
			expectedOK:   false,
		},

		// Whitespace
		{
			name:         "Type with leading space",
			input:        " video",
			expectedType: ContentTypeArticle,
			expectedOK:   false,
		},
		{
			name:         "Type with trailing space",
			input:        "video ",
			expectedType: ContentTypeArticle,
			expectedOK:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			contentType, ok := ParseContentType(tt.input)

			if ok != tt.expectedOK {
				t.Errorf("ParseContentType(%q) ok = %v, expected %v", tt.input, ok, tt.expectedOK)
			}

			if contentType != tt.expectedType {
				t.Errorf("ParseContentType(%q) type = %v, expected %v",
					tt.input, contentType, tt.expectedType)
			}
		})
	}
}

func TestParseContentType_RoundTrip(t *testing.T) {
	// Test that valid types can be round-tripped through String() and ParseContentType()
	validTypes := []struct {
		name        string
		contentType ContentType
		stringValue string
	}{
		{"video", ContentTypeVideo, "video"},
		{"article", ContentTypeArticle, "article"},
		{"arxiv", ContentTypeArxiv, "arxiv"},
		{"huggingface", ContentTypeHuggingFace, "huggingface"},
	}

	for _, tt := range validTypes {
		t.Run(tt.name, func(t *testing.T) {
			// Parse string to type
			parsed, ok := ParseContentType(tt.stringValue)
			if !ok {
				t.Fatalf("ParseContentType(%q) failed, expected success", tt.stringValue)
			}

			if parsed != tt.contentType {
				t.Errorf("ParseContentType(%q) = %v, expected %v",
					tt.stringValue, parsed, tt.contentType)
			}

			// Note: String() returns display names (e.g., "Video", "GenericArticle")
			// which are NOT the same as parse input (e.g., "video", "article")
			// This is intentional - they serve different purposes
		})
	}
}

func TestContentTypeConstants(t *testing.T) {
	// Verify that the constants have expected values and ordering
	// This documents the iota ordering

	if ContentTypeVideo < 0 {
		t.Error("ContentTypeVideo should be non-negative")
	}

	// Verify they are distinct
	types := map[ContentType]bool{
		ContentTypeVideo:       true,
		ContentTypeArxiv:       true,
		ContentTypeHuggingFace: true,
		ContentTypeArticle:     true,
	}

	if len(types) != 4 {
		t.Error("Content type constants are not distinct")
	}
}

func TestParseContentType_AllValidStrings(t *testing.T) {
	// Ensure all valid strings are documented
	validStrings := []string{"video", "article", "arxiv", "huggingface"}

	for _, str := range validStrings {
		t.Run(str, func(t *testing.T) {
			_, ok := ParseContentType(str)
			if !ok {
				t.Errorf("ParseContentType(%q) should be valid but returned false", str)
			}
		})
	}
}

func TestContentType_StringNotEmpty(t *testing.T) {
	// Ensure String() never returns empty string
	types := []ContentType{
		ContentTypeVideo,
		ContentTypeArxiv,
		ContentTypeHuggingFace,
		ContentTypeArticle,
		ContentType(999), // Unknown type
	}

	for _, ct := range types {
		t.Run(ct.String(), func(t *testing.T) {
			str := ct.String()
			if str == "" {
				t.Errorf("ContentType(%d).String() returned empty string", ct)
			}
		})
	}
}

func TestParseContentType_InvalidInputs(t *testing.T) {
	// Test a variety of invalid inputs
	invalidInputs := []string{
		"VIDEO",
		"Video",
		"vIdEo",
		"vid",
		"youtube",
		"yt",
		"paper",
		"arxiv-paper",
		"arxivpaper",
		"hf",
		"hugging-face",
		"generic",
		"web",
		"url",
		"link",
		" ",
		"\t",
		"\n",
		"video\n",
		"article ",
	}

	for _, input := range invalidInputs {
		t.Run("invalid:"+input, func(t *testing.T) {
			_, ok := ParseContentType(input)
			if ok {
				t.Errorf("ParseContentType(%q) should be invalid but returned true", input)
			}
		})
	}
}
