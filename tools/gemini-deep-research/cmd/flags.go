package cmd

import (
	"fmt"
	"net/url"

	"github.com/vbonnet/ai-tools/tools/gemini-deep-research/types"
)

// Validate validates the flags for conflicts and correctness
func Validate(f *types.Flags) error {
	// Validate content type if specified
	if f.Type != "" {
		validTypes := map[string]bool{
			"video":       true,
			"article":     true,
			"arxiv":       true,
			"huggingface": true,
		}
		if !validTypes[f.Type] {
			return fmt.Errorf("invalid content type: %s. Valid types: video, article, arxiv, huggingface", f.Type)
		}
	}

	return nil
}

// ValidateURL validates the URL format
func ValidateURL(urlStr string) error {
	// Parse URL
	u, err := url.Parse(urlStr)
	if err != nil {
		return fmt.Errorf("invalid URL format: %s", urlStr)
	}

	// Check for required scheme
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("invalid URL scheme: %s (must be http or https)", u.Scheme)
	}

	// Check for host
	if u.Host == "" {
		return fmt.Errorf("invalid URL: missing host")
	}

	return nil
}
