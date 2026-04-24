// Package prompt provides query sanitization and prompt security.
package prompt

import (
	"errors"
	"fmt"
	"strings"
)

var (
	// ErrXMLInjectionAttempt indicates the query contains XML tags attempting to break hierarchy.
	ErrXMLInjectionAttempt = errors.New("XML tag injection detected")

	// ErrPromptInjectionDetected indicates the query contains known prompt injection patterns.
	ErrPromptInjectionDetected = errors.New("prompt injection pattern detected")

	// ErrQueryTooLong indicates the query exceeds maximum allowed length.
	ErrQueryTooLong = errors.New("query exceeds maximum length")
)

// QuerySanitizer validates and sanitizes user queries to prevent prompt injection attacks.
// It enforces length limits, detects XML tag injection, and identifies known injection patterns.
//
// Security Features:
//   - Rejects queries containing XML tags (<system>, <user>, etc.)
//   - Detects common prompt injection patterns ("ignore previous", "new instructions", etc.)
//   - Enforces maximum query length
//   - Escapes XML special characters for safe embedding
//
// See: core/docs/specs/prompt-injection-defense.md
type QuerySanitizer struct {
	maxLength      int
	bannedPatterns []string
}

// NewQuerySanitizer creates a new sanitizer with default security settings.
//
// Default Configuration:
//   - Maximum length: 1000 characters
//   - Banned patterns: Common prompt injection attempts
//
// Returns:
//   - Configured QuerySanitizer instance
func NewQuerySanitizer() *QuerySanitizer {
	return &QuerySanitizer{
		maxLength: 1000,
		bannedPatterns: []string{
			// Multi-instruction patterns
			"ignore previous",
			"ignore all previous",
			"new instructions",
			"disregard all",
			"forget everything",
			"override instructions",
			"override",
			"change your behavior",
			"critical update",

			// Role manipulation
			"system:",
			"assistant:",
			"user:",

			// XML tag injection attempts
			"<system",
			"</system>",
			"<user",
			"</user>",
			"<untrusted_data",
			"</untrusted_data>",

			// Direct command patterns
			"sudo ",
			"execute command",
			"run command",
		},
	}
}

// Sanitize validates and sanitizes a user query.
// It performs multiple security checks and returns an error if the query
// is deemed unsafe. Safe queries are escaped for XML embedding.
//
// Validation Steps:
//  1. Length check (must be <= maxLength)
//  2. XML tag injection detection
//  3. Prompt injection pattern detection
//  4. XML special character escaping
//
// Parameters:
//   - query: Raw user query string
//
// Returns:
//   - Sanitized query safe for XML embedding
//   - Error if query fails validation
//
// Example:
//
//	sanitizer := NewQuerySanitizer()
//	clean, err := sanitizer.Sanitize("How to handle errors?")
//	if err != nil {
//	    log.Warn("Query rejected", "error", err)
//	    return err
//	}
func (s *QuerySanitizer) Sanitize(query string) (string, error) {
	// 1. Length check
	if len(query) > s.maxLength {
		return "", fmt.Errorf("%w: %d > %d", ErrQueryTooLong, len(query), s.maxLength)
	}

	// 2. Reject XML tag injection attempts
	if containsXMLTags(query) {
		return "", fmt.Errorf("%w: query contains XML tags", ErrXMLInjectionAttempt)
	}

	// 3. Detect multi-instruction attempts
	if containsPromptInjectionPatterns(query, s.bannedPatterns) {
		return "", fmt.Errorf("%w: detected banned pattern", ErrPromptInjectionDetected)
	}

	// 4. Escape special characters for XML embedding
	sanitized := escapeForXML(query)

	return sanitized, nil
}

// containsXMLTags checks if the string contains XML tags that could break hierarchy.
// It detects both opening tags (<system, <user) and closing tags (</...).
//
// Parameters:
//   - s: String to check
//
// Returns:
//   - true if XML tags detected, false otherwise
func containsXMLTags(s string) bool {
	// Check for closing tags (</...)
	if strings.Contains(s, "</") {
		return true
	}

	// Check for opening tags that match our hierarchy
	suspiciousTags := []string{"<system", "<user", "<untrusted_data"}
	lower := strings.ToLower(s)
	for _, tag := range suspiciousTags {
		if strings.Contains(lower, tag) {
			return true
		}
	}

	return false
}

// containsPromptInjectionPatterns checks if the string contains known injection patterns.
// Patterns are matched case-insensitively.
//
// Parameters:
//   - s: String to check
//   - patterns: List of banned patterns
//
// Returns:
//   - true if any pattern matches, false otherwise
func containsPromptInjectionPatterns(s string, patterns []string) bool {
	lower := strings.ToLower(s)
	for _, pattern := range patterns {
		if strings.Contains(lower, pattern) {
			return true
		}
	}
	return false
}

// escapeForXML escapes special XML characters to prevent parsing errors.
// This ensures user input is treated as text data, not markup.
//
// Escaped Characters:
//   - & → &amp;
//   - < → &lt;
//   - > → &gt;
//   - " → &quot;
//   - ' → &apos;
//
// Parameters:
//   - s: String to escape
//
// Returns:
//   - XML-escaped string
func escapeForXML(s string) string {
	replacer := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		"\"", "&quot;",
		"'", "&apos;",
	)
	return replacer.Replace(s)
}
