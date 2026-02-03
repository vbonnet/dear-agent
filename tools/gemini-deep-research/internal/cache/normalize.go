package cache

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

// Normalize converts URL to canonical cache key based on content type
func Normalize(rawURL string, contentType string) (string, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("parse URL: %w", err)
	}

	switch contentType {
	case "youtube":
		return normalizeYouTube(parsed)
	case "arxiv":
		return normalizeArxiv(parsed)
	default:
		return normalizeGeneric(parsed)
	}
}

// normalizeYouTube extracts video ID and builds canonical URL
func normalizeYouTube(u *url.URL) (string, error) {
	var videoID string

	// Check for v= query param (youtube.com/watch?v=ID)
	videoID = u.Query().Get("v")

	// If not found, check for youtu.be short URL (youtu.be/ID)
	if videoID == "" && strings.Contains(u.Host, "youtu.be") {
		videoID = strings.TrimPrefix(u.Path, "/")
	}

	if videoID == "" {
		return "", fmt.Errorf("no video ID in YouTube URL")
	}

	return fmt.Sprintf("https://youtube.com/watch?v=%s", videoID), nil
}

// normalizeArxiv extracts paper ID and strips version
func normalizeArxiv(u *url.URL) (string, error) {
	// Extract ID from path (e.g., /abs/2601.20802v1)
	re := regexp.MustCompile(`/abs/(\d+\.\d+)(v\d+)?`)
	matches := re.FindStringSubmatch(u.Path)
	if len(matches) < 2 {
		return "", fmt.Errorf("invalid arxiv URL format")
	}

	paperID := matches[1]
	return fmt.Sprintf("https://arxiv.org/abs/%s", paperID), nil
}

// normalizeGeneric performs standard URL normalization
func normalizeGeneric(u *url.URL) (string, error) {
	// Lowercase scheme and host
	u.Scheme = strings.ToLower(u.Scheme)
	u.Host = strings.ToLower(u.Host)

	// Remove www. prefix
	u.Host = strings.TrimPrefix(u.Host, "www.")

	// Remove trailing slash from path
	u.Path = strings.TrimSuffix(u.Path, "/")

	// Remove UTM tracking params
	q := u.Query()
	for key := range q {
		if strings.HasPrefix(key, "utm_") {
			q.Del(key)
		}
	}
	u.RawQuery = q.Encode()

	return u.String(), nil
}
