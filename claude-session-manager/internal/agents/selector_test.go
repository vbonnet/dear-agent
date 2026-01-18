package agents

import "testing"

func TestSelectAgent_KeywordMatch(t *testing.T) {
	config := &AgentsConfig{
		DefaultAgent: "claude",
		Preferences: []Preference{
			{
				Keywords: []string{"creative", "design"},
				Agent:    "gemini",
			},
			{
				Keywords: []string{"code", "debug"},
				Agent:    "claude",
			},
		},
	}

	tests := []struct {
		sessionName string
		expected    string
	}{
		{"creative-project", "gemini"},       // Matches "creative"
		{"design-system", "gemini"},          // Matches "design"
		{"code-refactor", "claude"},          // Matches "code"
		{"debug-api", "claude"},              // Matches "debug"
		{"random-task", "claude"},            // No match, uses default
		{"my-creative-idea", "gemini"},       // Substring match
		{"decode-service", "claude"},         // Substring "code" in "decode"
	}

	for _, tt := range tests {
		t.Run(tt.sessionName, func(t *testing.T) {
			agent := SelectAgent(tt.sessionName, config)
			if agent != tt.expected {
				t.Errorf("SelectAgent(%q) = %q, want %q", tt.sessionName, agent, tt.expected)
			}
		})
	}
}

func TestSelectAgent_CaseInsensitive(t *testing.T) {
	config := &AgentsConfig{
		DefaultAgent: "claude",
		Preferences: []Preference{
			{
				Keywords: []string{"creative"},
				Agent:    "gemini",
			},
		},
	}

	tests := []string{
		"creative-project",
		"Creative-Project",
		"CREATIVE-PROJECT",
		"CrEaTiVe-PrOjEcT",
	}

	for _, sessionName := range tests {
		t.Run(sessionName, func(t *testing.T) {
			agent := SelectAgent(sessionName, config)
			if agent != "gemini" {
				t.Errorf("SelectAgent(%q) = %q, want 'gemini' (case-insensitive)", sessionName, agent)
			}
		})
	}
}

func TestSelectAgent_FirstMatchWins(t *testing.T) {
	config := &AgentsConfig{
		DefaultAgent: "claude",
		Preferences: []Preference{
			{
				Keywords: []string{"project"},
				Agent:    "gemini",
			},
			{
				Keywords: []string{"creative"},
				Agent:    "gpt4",
			},
		},
	}

	// "creative-project" matches both "project" and "creative"
	// Should return "gemini" (first preference)
	agent := SelectAgent("creative-project", config)
	if agent != "gemini" {
		t.Errorf("SelectAgent() = %q, want 'gemini' (first match wins)", agent)
	}
}

func TestSelectAgent_EmptySessionName(t *testing.T) {
	config := &AgentsConfig{
		DefaultAgent: "claude",
		Preferences: []Preference{
			{
				Keywords: []string{"creative"},
				Agent:    "gemini",
			},
		},
	}

	agent := SelectAgent("", config)
	if agent != "claude" {
		t.Errorf("SelectAgent(\"\") = %q, want 'claude' (default for empty)", agent)
	}
}

func TestSelectAgent_EmptyPreferences(t *testing.T) {
	config := &AgentsConfig{
		DefaultAgent: "claude",
		Preferences:  []Preference{}, // No preferences
	}

	agent := SelectAgent("any-session-name", config)
	if agent != "claude" {
		t.Errorf("SelectAgent() = %q, want 'claude' (default when no preferences)", agent)
	}
}

func TestSelectAgent_MultipleKeywordsInPreference(t *testing.T) {
	config := &AgentsConfig{
		DefaultAgent: "claude",
		Preferences: []Preference{
			{
				Keywords: []string{"creative", "design", "art", "brainstorm"},
				Agent:    "gemini",
			},
		},
	}

	tests := []struct {
		sessionName string
		expected    string
	}{
		{"creative-work", "gemini"},    // Matches first keyword
		{"art-project", "gemini"},      // Matches third keyword
		{"brainstorm-ideas", "gemini"}, // Matches fourth keyword
		{"coding-task", "claude"},      // No match, uses default
	}

	for _, tt := range tests {
		t.Run(tt.sessionName, func(t *testing.T) {
			agent := SelectAgent(tt.sessionName, config)
			if agent != tt.expected {
				t.Errorf("SelectAgent(%q) = %q, want %q", tt.sessionName, agent, tt.expected)
			}
		})
	}
}
