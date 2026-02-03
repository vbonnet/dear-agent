package cache

import "testing"

func TestGenerateSlug(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "YouTubeURL",
			input:    "https://www.youtube.com/watch?v=dQw4w9WgXcQ",
			expected: "youtube-dqw4w9wgxcq",
		},
		{
			name:     "ArxivURL",
			input:    "https://arxiv.org/abs/2601.20802",
			expected: "arxiv-2601-20802",
		},
		{
			name:     "GenericURL",
			input:    "https://example.com/blog/my-article",
			expected: "blog-my-article",
		},
		{
			name:     "URLWithQueryParams",
			input:    "https://example.com/article?id=123&name=test",
			expected: "article",
		},
		{
			name:     "LongURL",
			input:    "https://example.com/very/long/path/to/some/article/with/many/segments/that/exceeds/fifty/characters",
			expected: "very-long-path-to-some-article-with-many-segments-",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GenerateSlug(tt.input)
			if result != tt.expected {
				t.Errorf("GenerateSlug() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

func TestSanitize(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Lowercase",
			input:    "HELLO-WORLD",
			expected: "hello-world",
		},
		{
			name:     "SpecialChars",
			input:    "hello@world!test",
			expected: "hello-world-test",
		},
		{
			name:     "MultipleHyphens",
			input:    "hello---world",
			expected: "hello---world",
		},
		{
			name:     "LeadingTrailingHyphens",
			input:    "-hello-world-",
			expected: "hello-world",
		},
		{
			name:     "Truncation",
			input:    "this-is-a-very-long-string-that-should-be-truncated-to-fifty-characters-maximum",
			expected: "this-is-a-very-long-string-that-should-be-truncate",
		},
		{
			name:     "Empty",
			input:    "",
			expected: "research",
		},
		{
			name:     "OnlySpecialChars",
			input:    "!@#$%^&*()",
			expected: "research",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitize(tt.input)
			if result != tt.expected {
				t.Errorf("sanitize() = %v, expected %v", result, tt.expected)
			}

			// Verify length constraint
			if len(result) > 50 {
				t.Errorf("sanitize() result too long: %d characters", len(result))
			}
		})
	}
}
