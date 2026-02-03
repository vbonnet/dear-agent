package detector

import (
	"net/url"
	"strings"
)

// URLPattern represents a URL pattern for content type detection
type URLPattern struct {
	ContentType ContentType
	Matchers    []string // List of substrings to match in the URL
}

// patterns defines the URL pattern matching rules
// Order matters: more specific patterns should come first
var patterns = []URLPattern{
	{
		ContentType: ContentTypeVideo,
		Matchers: []string{
			"youtube.com",
			"youtu.be",
			"vimeo.com",
		},
	},
	{
		ContentType: ContentTypeArxiv,
		Matchers: []string{
			"arxiv.org/abs/",
			"arxiv.org/pdf/",
		},
	},
	{
		ContentType: ContentTypeHuggingFace,
		Matchers: []string{
			"huggingface.co/papers/",
		},
	},
}

// normalizeURL normalizes a URL for pattern matching
// Handles case-insensitivity, query parameters, and fragments
func normalizeURL(rawURL string) (string, string, error) {
	// Parse the URL
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", "", err
	}

	// Add scheme if missing
	if u.Scheme == "" {
		u.Scheme = "https"
	}

	// Extract hostname and path (ignore query params and fragments)
	hostname := strings.ToLower(u.Hostname())
	path := strings.ToLower(u.Path)

	// Combine hostname and path for matching
	normalized := hostname + path

	return normalized, hostname, nil
}

// matchPattern checks if a normalized URL matches a given pattern
func matchPattern(normalized string, matchers []string) bool {
	for _, matcher := range matchers {
		// Case-insensitive matching
		if strings.Contains(normalized, strings.ToLower(matcher)) {
			return true
		}
	}
	return false
}

// detectFromPattern detects content type from URL patterns
// Returns ContentTypeArticle as fallback if no pattern matches
func detectFromPattern(rawURL string) (ContentType, string, error) {
	normalized, hostname, err := normalizeURL(rawURL)
	if err != nil {
		return ContentTypeArticle, "", err
	}

	// Try to match against known patterns
	for _, pattern := range patterns {
		if matchPattern(normalized, pattern.Matchers) {
			return pattern.ContentType, hostname, nil
		}
	}

	// Fallback to generic article
	return ContentTypeArticle, hostname, nil
}
