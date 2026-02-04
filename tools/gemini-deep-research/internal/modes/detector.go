// Package modes handles detection and routing between general and competitive analysis modes.
package modes

import (
	"net/url"
	"strings"
)

// Mode represents the analysis mode
type Mode string

const (
	// ModeGeneral is the standard deep research mode
	ModeGeneral Mode = "general"
	// ModeCompetitive is the competitive analysis mode with automatic discovery
	ModeCompetitive Mode = "competitive"
)

// DetectMode determines which mode to use based on input and flags
// Priority: explicit flag > URL detection > keyword matching > default general
func DetectMode(input string, explicitMode string) Mode {
	// Priority 1: Explicit mode flag overrides everything
	if explicitMode != "" {
		if explicitMode == string(ModeCompetitive) {
			return ModeCompetitive
		}
		return ModeGeneral
	}

	// Priority 2: If input is a URL, use general mode
	if isURL(input) {
		return ModeGeneral
	}

	// Priority 3: Keyword matching for competitive intent
	keywords := []string{
		"competitive",
		"vs",
		"compare",
		"what can we learn",
		"gap analysis",
		"competitor",
	}

	lowerInput := strings.ToLower(input)
	for _, keyword := range keywords {
		if strings.Contains(lowerInput, keyword) {
			return ModeCompetitive
		}
	}

	// Default: general mode
	return ModeGeneral
}

// isURL checks if the input is a valid URL
func isURL(input string) bool {
	// Empty or too short to be a URL
	if len(input) < 7 { // Minimum: http://a
		return false
	}

	// Simple heuristic: starts with http:// or https://
	if strings.HasPrefix(strings.ToLower(input), "http://") ||
		strings.HasPrefix(strings.ToLower(input), "https://") {
		_, err := url.Parse(input)
		return err == nil
	}

	return false
}

// ParseMode converts a string to a Mode, defaulting to General if invalid
func ParseMode(s string) Mode {
	switch strings.ToLower(s) {
	case string(ModeCompetitive):
		return ModeCompetitive
	case string(ModeGeneral):
		return ModeGeneral
	default:
		return ModeGeneral
	}
}

// String returns the string representation of the Mode
func (m Mode) String() string {
	return string(m)
}

// IsCompetitive returns true if the mode is competitive
func (m Mode) IsCompetitive() bool {
	return m == ModeCompetitive
}

// IsGeneral returns true if the mode is general
func (m Mode) IsGeneral() bool {
	return m == ModeGeneral
}
