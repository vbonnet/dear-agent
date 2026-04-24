package agents

import "testing"

func TestSelectHarness_KeywordMatch(t *testing.T) {
	config := &HarnessConfig{
		DefaultHarness: "claude",
		Preferences: []Preference{
			{
				Keywords: []string{"creative", "design"},
				Harness:  "gemini",
			},
			{
				Keywords: []string{"code", "debug"},
				Harness:  "claude",
			},
		},
	}

	tests := []struct {
		sessionName string
		expected    string
	}{
		{"creative-project", "gemini"}, // Matches "creative"
		{"design-system", "gemini"},    // Matches "design"
		{"code-refactor", "claude"},    // Matches "code"
		{"debug-api", "claude"},        // Matches "debug"
		{"random-task", "claude"},      // No match, uses default
		{"my-creative-idea", "gemini"}, // Substring match
		{"decode-service", "claude"},   // Substring "code" in "decode"
	}

	for _, tt := range tests {
		t.Run(tt.sessionName, func(t *testing.T) {
			agent := SelectHarness(tt.sessionName, config)
			if agent != tt.expected {
				t.Errorf("SelectHarness(%q) = %q, want %q", tt.sessionName, agent, tt.expected)
			}
		})
	}
}

func TestSelectHarness_CaseInsensitive(t *testing.T) {
	config := &HarnessConfig{
		DefaultHarness: "claude",
		Preferences: []Preference{
			{
				Keywords: []string{"creative"},
				Harness:  "gemini",
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
			agent := SelectHarness(sessionName, config)
			if agent != "gemini" {
				t.Errorf("SelectHarness(%q) = %q, want 'gemini' (case-insensitive)", sessionName, agent)
			}
		})
	}
}

func TestSelectHarness_FirstMatchWins(t *testing.T) {
	config := &HarnessConfig{
		DefaultHarness: "claude",
		Preferences: []Preference{
			{
				Keywords: []string{"project"},
				Harness:  "gemini",
			},
			{
				Keywords: []string{"creative"},
				Harness:  "gpt4",
			},
		},
	}

	// "creative-project" matches both "project" and "creative"
	// Should return "gemini" (first preference)
	agent := SelectHarness("creative-project", config)
	if agent != "gemini" {
		t.Errorf("SelectHarness() = %q, want 'gemini' (first match wins)", agent)
	}
}

func TestSelectHarness_EmptySessionName(t *testing.T) {
	config := &HarnessConfig{
		DefaultHarness: "claude",
		Preferences: []Preference{
			{
				Keywords: []string{"creative"},
				Harness:  "gemini",
			},
		},
	}

	agent := SelectHarness("", config)
	if agent != "claude" {
		t.Errorf("SelectHarness(\"\") = %q, want 'claude' (default for empty)", agent)
	}
}

func TestSelectHarness_EmptyPreferences(t *testing.T) {
	config := &HarnessConfig{
		DefaultHarness: "claude",
		Preferences:    []Preference{}, // No preferences
	}

	agent := SelectHarness("any-session-name", config)
	if agent != "claude" {
		t.Errorf("SelectHarness() = %q, want 'claude' (default when no preferences)", agent)
	}
}

func TestSelectHarness_MultipleKeywordsInPreference(t *testing.T) {
	config := &HarnessConfig{
		DefaultHarness: "claude",
		Preferences: []Preference{
			{
				Keywords: []string{"creative", "design", "art", "brainstorm"},
				Harness:  "gemini",
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
			agent := SelectHarness(tt.sessionName, config)
			if agent != tt.expected {
				t.Errorf("SelectHarness(%q) = %q, want %q", tt.sessionName, agent, tt.expected)
			}
		})
	}
}

func TestSelectHarness_OpenAIRouting(t *testing.T) {
	config := &HarnessConfig{
		DefaultHarness: "claude",
		Preferences: []Preference{
			{
				Keywords: []string{"research", "analyze", "data", "gpt", "openai"},
				Harness:  "openai",
			},
			{
				Keywords: []string{"code", "debug"},
				Harness:  "claude",
			},
			{
				Keywords: []string{"creative", "design"},
				Harness:  "gemini",
			},
		},
	}

	tests := []struct {
		sessionName string
		expected    string
	}{
		{"research-analysis", "openai"},      // Matches "research"
		{"data-analysis", "openai"},          // Matches "data"
		{"analyze-report", "openai"},         // Matches "analyze"
		{"gpt-session", "openai"},            // Matches "gpt"
		{"openai-test", "openai"},            // Matches "openai"
		{"code-review", "claude"},            // Matches "code"
		{"creative-project", "gemini"},       // Matches "creative"
		{"random-task", "claude"},            // No match, uses default
		{"research-data-analysis", "openai"}, // Multiple keyword matches, first preference wins
	}

	for _, tt := range tests {
		t.Run(tt.sessionName, func(t *testing.T) {
			agent := SelectHarness(tt.sessionName, config)
			if agent != tt.expected {
				t.Errorf("SelectHarness(%q) = %q, want %q", tt.sessionName, agent, tt.expected)
			}
		})
	}
}
