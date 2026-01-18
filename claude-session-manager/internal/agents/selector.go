package agents

import "strings"

// SelectAgent selects an agent based on session name and keyword matching.
// Matches session name against keyword lists (case-insensitive substring matching).
// Returns agent from first matching preference, or default_agent if no match.
func SelectAgent(sessionName string, config *AgentsConfig) string {
	// Handle empty session name edge case
	if sessionName == "" {
		return config.DefaultAgent
	}

	// Case-insensitive matching
	sessionLower := strings.ToLower(sessionName)

	// Check each preference in order (first match wins)
	for _, pref := range config.Preferences {
		for _, keyword := range pref.Keywords {
			keywordLower := strings.ToLower(keyword)

			// Substring matching (simple, no regex)
			if strings.Contains(sessionLower, keywordLower) {
				return pref.Agent // First match wins
			}
		}
	}

	// No keyword matched → use default agent
	return config.DefaultAgent
}
