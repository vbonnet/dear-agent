package consolidation

import "regexp"

// PurgePattern defines a named regex pattern for detecting sensitive data.
type PurgePattern struct {
	// Name identifies the pattern type (e.g., "api_key", "email").
	Name string

	// Pattern is the compiled regex for matching sensitive content.
	Pattern *regexp.Regexp

	// Replacement is the string to substitute for matches.
	Replacement string
}

// DefaultPurgePatterns returns the standard set of PII/secret detection patterns.
//
// Patterns cover:
//   - API keys: OpenAI (sk-), Google Cloud (AIza), GitHub tokens (ghp_, gho_)
//   - Email addresses: standard RFC 5322 simplified
//   - Bearer tokens: Authorization header values
//   - Connection strings: database URIs with credentials
//   - Phone numbers: US format with optional country code
//   - SSN: US Social Security Numbers
func DefaultPurgePatterns() []PurgePattern {
	return []PurgePattern{
		{
			Name:        "api_key",
			Pattern:     regexp.MustCompile(`(?i)(?:sk-[a-zA-Z0-9]{20,}|AIza[a-zA-Z0-9_-]{35}|ghp_[a-zA-Z0-9]{36}|gho_[a-zA-Z0-9]{36})`),
			Replacement: "[REDACTED_API_KEY]",
		},
		{
			Name:        "email",
			Pattern:     regexp.MustCompile(`[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}`),
			Replacement: "[REDACTED_EMAIL]",
		},
		{
			Name:        "bearer_token",
			Pattern:     regexp.MustCompile(`(?i)(?:Bearer\s+)[a-zA-Z0-9_\-.~+/]+=*`),
			Replacement: "Bearer [REDACTED_TOKEN]",
		},
		{
			Name:        "connection_string",
			Pattern:     regexp.MustCompile(`(?i)(?:mongodb|postgres|mysql|redis|amqp)://[^\s"'` + "`" + `]+`),
			Replacement: "[REDACTED_CONNECTION_STRING]",
		},
		{
			Name:        "phone",
			Pattern:     regexp.MustCompile(`(?:\+?1[-.\s]?)?\(?\d{3}\)?[-.\s]?\d{3}[-.\s]?\d{4}\b`),
			Replacement: "[REDACTED_PHONE]",
		},
		{
			Name:        "ssn",
			Pattern:     regexp.MustCompile(`\b\d{3}-\d{2}-\d{4}\b`),
			Replacement: "[REDACTED_SSN]",
		},
	}
}
