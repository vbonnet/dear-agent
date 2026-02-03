package detector

import (
	"fmt"
	"log"
)

// DetectContentType detects the content type from a URL with optional manual override
// Parameters:
//   - url: The URL to analyze
//   - typeFlag: Optional content type override (empty string for auto-detection)
//
// Returns:
//   - ContentType: The detected or overridden content type
//   - error: Error if URL is invalid or type flag is invalid
func DetectContentType(url string, typeFlag string) (ContentType, error) {
	// Validate URL is not empty
	if url == "" {
		return ContentTypeArticle, fmt.Errorf("URL cannot be empty")
	}

	// Handle manual override if --type flag is specified
	if typeFlag != "" {
		contentType, valid := ParseContentType(typeFlag)
		if !valid {
			return ContentTypeArticle, fmt.Errorf(
				"invalid content type: %s. Valid: video, article, arxiv, huggingface",
				typeFlag,
			)
		}

		// Log the override with what was auto-detected
		autoDetected, hostname, err := detectFromPattern(url)
		if err != nil {
			// Log override without showing auto-detected (URL was invalid)
			log.Printf("Content type override: %s (--type flag)", typeFlag)
		} else {
			log.Printf(
				"Content type override: %s (--type flag, auto-detected: %s from %s)",
				typeFlag,
				autoDetected.String(),
				hostname,
			)
		}

		return contentType, nil
	}

	// Auto-detect from URL pattern
	contentType, hostname, err := detectFromPattern(url)
	if err != nil {
		return ContentTypeArticle, fmt.Errorf("invalid URL: %w", err)
	}

	// Log the detected type
	if contentType == ContentTypeArticle {
		log.Printf("Detected: %s (fallback)", contentType.String())
	} else {
		log.Printf("Detected: %s from %s", contentType.String(), hostname)
	}

	return contentType, nil
}
