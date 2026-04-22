package agents

import "strings"

// SelectHarness selects a harness based on session name and keyword matching.
// Matches session name against keyword lists (case-insensitive substring matching).
// Returns harness from first matching preference, or default_harness if no match.
func SelectHarness(sessionName string, config *HarnessConfig) string {
	// Handle empty session name edge case
	if sessionName == "" {
		return config.DefaultHarness
	}

	// Case-insensitive matching
	sessionLower := strings.ToLower(sessionName)

	// Check each preference in order (first match wins)
	for _, pref := range config.Preferences {
		for _, keyword := range pref.Keywords {
			keywordLower := strings.ToLower(keyword)

			// Substring matching (simple, no regex)
			if strings.Contains(sessionLower, keywordLower) {
				return pref.Harness // First match wins
			}
		}
	}

	// No keyword matched → use default harness
	return config.DefaultHarness
}
