package cache

import (
	"net/url"
	"regexp"
	"strings"
)

// GenerateSlug creates a filesystem-safe slug from URL
func GenerateSlug(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		// Fallback to raw URL if parsing fails
		return sanitize(rawURL)
	}

	// Extract meaningful part from URL
	var base string

	// YouTube: use video ID
	if strings.Contains(parsed.Host, "youtube.com") || strings.Contains(parsed.Host, "youtu.be") {
		videoID := parsed.Query().Get("v")
		if videoID != "" {
			base = "youtube-" + videoID
		}
	}

	// ArXiv: use paper ID
	if strings.Contains(parsed.Host, "arxiv.org") {
		re := regexp.MustCompile(`/abs/(\d+\.\d+)`)
		matches := re.FindStringSubmatch(parsed.Path)
		if len(matches) >= 2 {
			base = "arxiv-" + strings.ReplaceAll(matches[1], ".", "-")
		}
	}

	// Generic: use path
	if base == "" {
		base = strings.TrimPrefix(parsed.Path, "/")
		if base == "" {
			base = parsed.Host
		}
	}

	return sanitize(base)
}

// sanitize cleans string for filesystem use
func sanitize(s string) string {
	// Convert to lowercase
	s = strings.ToLower(s)

	// Replace non-alphanumeric with hyphens
	re := regexp.MustCompile(`[^a-z0-9-]+`)
	s = re.ReplaceAllString(s, "-")

	// Remove leading/trailing hyphens
	s = strings.Trim(s, "-")

	// Truncate to 50 chars
	if len(s) > 50 {
		s = s[:50]
	}

	// Ensure not empty
	if s == "" {
		s = "research"
	}

	return s
}
