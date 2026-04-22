package statusline

import (
	"testing"

	"github.com/vbonnet/dear-agent/agm/internal/session"
)

// TestNewFormatter tests formatter creation
func TestNewFormatter(t *testing.T) {
	tests := []struct {
		name        string
		template    string
		expectError bool
	}{
		{
			name:        "valid template",
			template:    "{{.SessionName}}",
			expectError: false,
		},
		{
			name:        "complex template",
			template:    DefaultTemplate(),
			expectError: false,
		},
		{
			name:        "empty template",
			template:    "",
			expectError: true,
		},
		{
			name:        "invalid syntax",
			template:    "{{.SessionName",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, err := NewFormatter(tt.template)
			if tt.expectError {
				if err == nil {
					t.Errorf("NewFormatter() expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("NewFormatter() unexpected error: %v", err)
				}
				if f == nil {
					t.Errorf("NewFormatter() returned nil formatter")
				}
			}
		})
	}
}

// TestFormatter_Format tests template rendering
func TestFormatter_Format(t *testing.T) {
	tests := []struct {
		name        string
		template    string
		data        *session.StatusLineData
		expected    string
		expectError bool
	}{
		{
			name:     "simple template",
			template: "{{.SessionName}}",
			data: &session.StatusLineData{
				SessionName: "test-session",
			},
			expected:    "test-session",
			expectError: false,
		},
		{
			name:     "state with color",
			template: "#[fg={{.StateColor}}]{{.State}}#[default]",
			data: &session.StatusLineData{
				State:      "DONE",
				StateColor: "green",
			},
			expected:    "#[fg=green]DONE#[default]",
			expectError: false,
		},
		{
			name:     "context percentage",
			template: "{{printf \"%.0f\" .ContextPercent}}%",
			data: &session.StatusLineData{
				ContextPercent: 73.0,
			},
			expected:    "73%",
			expectError: false,
		},
		{
			name:     "context unavailable",
			template: "{{if ge .ContextPercent 0.0}}{{printf \"%.0f\" .ContextPercent}}%{{else}}--{{end}}",
			data: &session.StatusLineData{
				ContextPercent: -1,
			},
			expected:    "--",
			expectError: false,
		},
		{
			name:     "git status",
			template: "{{.Branch}} (+{{.Uncommitted}})",
			data: &session.StatusLineData{
				Branch:      "main",
				Uncommitted: 3,
			},
			expected:    "main (+3)",
			expectError: false,
		},
		{
			name:     "agent icon and type",
			template: "{{.AgentIcon}}{{.AgentType}}",
			data: &session.StatusLineData{
				AgentType: "claude",
				AgentIcon: "🤖",
			},
			expected:    "🤖claude",
			expectError: false,
		},
		{
			name:        "nil data",
			template:    "{{.SessionName}}",
			data:        nil,
			expected:    "",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, err := NewFormatter(tt.template)
			if err != nil {
				t.Fatalf("NewFormatter() failed: %v", err)
			}

			result, err := f.Format(tt.data)
			if tt.expectError {
				if err == nil {
					t.Errorf("Format() expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("Format() unexpected error: %v", err)
				}
				if result != tt.expected {
					t.Errorf("Format() = %q, expected %q", result, tt.expected)
				}
			}
		})
	}
}

// TestDefaultTemplate tests the default template renders without error
func TestDefaultTemplate(t *testing.T) {
	template := DefaultTemplate()
	if template == "" {
		t.Fatal("DefaultTemplate() returned empty string")
	}

	f, err := NewFormatter(template)
	if err != nil {
		t.Fatalf("DefaultTemplate() is invalid: %v", err)
	}

	data := &session.StatusLineData{
		SessionName:    "test-session",
		State:          "DONE",
		StateColor:     "green",
		Branch:         "main",
		Uncommitted:    3,
		ContextPercent: 73.0,
		ContextColor:   "yellow",
		ContextUsed:    "146k",
		ContextTotal:   "200k",
		AgentType:      "claude",
		AgentIcon:      "🤖",
		Cost:           "$2.45",
		CostColor:      "yellow",
		ModelShort:     "Opus",
		RateLimit5h:    35.0,
		RateLimitColor: "green",
	}

	result, err := f.Format(data)
	if err != nil {
		t.Errorf("Format() with default template failed: %v", err)
	}
	if result == "" {
		t.Error("Format() with default template returned empty string")
	}

	// Verify expected components are present
	expectedParts := []string{"🤖", "Opus", "DONE", "146k", "200k", "$2.45", "test-session"}
	for _, part := range expectedParts {
		if !contains(result, part) {
			t.Errorf("Format() result missing expected part %q, got: %q", part, result)
		}
	}
}

// TestMinimalTemplate tests the minimal template
func TestMinimalTemplate(t *testing.T) {
	template := MinimalTemplate()
	f, err := NewFormatter(template)
	if err != nil {
		t.Fatalf("MinimalTemplate() is invalid: %v", err)
	}

	data := &session.StatusLineData{
		State:          "DONE",
		ContextPercent: 73.0,
		ContextUsed:    "146k",
		ContextTotal:   "200k",
		AgentIcon:      "🤖",
	}

	result, err := f.Format(data)
	if err != nil {
		t.Errorf("Format() with minimal template failed: %v", err)
	}

	expectedParts := []string{"🤖", "DONE", "146k/200k"}
	for _, part := range expectedParts {
		if !contains(result, part) {
			t.Errorf("Format() result missing expected part %q, got: %q", part, result)
		}
	}
}

// TestCompactTemplate tests the compact template
func TestCompactTemplate(t *testing.T) {
	template := CompactTemplate()
	f, err := NewFormatter(template)
	if err != nil {
		t.Fatalf("CompactTemplate() is invalid: %v", err)
	}

	data := &session.StatusLineData{
		StateColor:     "green",
		ContextPercent: 73.0,
		ContextUsed:    "146k",
		ContextTotal:   "200k",
		Branch:         "main",
		AgentIcon:      "🤖",
	}

	result, err := f.Format(data)
	if err != nil {
		t.Errorf("Format() with compact template failed: %v", err)
	}

	expectedParts := []string{"🤖", "●", "146k/200k"}
	for _, part := range expectedParts {
		if !contains(result, part) {
			t.Errorf("Format() result missing expected part %q, got: %q", part, result)
		}
	}
}

// TestMultiAgentTemplate tests the multi-agent template
func TestMultiAgentTemplate(t *testing.T) {
	template := MultiAgentTemplate()
	f, err := NewFormatter(template)
	if err != nil {
		t.Fatalf("MultiAgentTemplate() is invalid: %v", err)
	}

	data := &session.StatusLineData{
		AgentType:      "gemini",
		AgentIcon:      "✨",
		State:          "WORKING",
		StateColor:     "blue",
		ContextPercent: 82.0,
		ContextUsed:    "164k",
		ContextTotal:   "200k",
	}

	result, err := f.Format(data)
	if err != nil {
		t.Errorf("Format() with multi-agent template failed: %v", err)
	}

	expectedParts := []string{"✨", "gemini", "WORKING", "164k/200k"}
	for _, part := range expectedParts {
		if !contains(result, part) {
			t.Errorf("Format() result missing expected part %q, got: %q", part, result)
		}
	}
}

// TestFullTemplate tests the full template
func TestFullTemplate(t *testing.T) {
	template := FullTemplate()
	f, err := NewFormatter(template)
	if err != nil {
		t.Fatalf("FullTemplate() is invalid: %v", err)
	}

	data := &session.StatusLineData{
		SessionName:    "test-session",
		State:          "DONE",
		StateColor:     "green",
		Branch:         "feature-branch",
		Uncommitted:    5,
		ContextPercent: 85.0,
		ContextColor:   "orange",
		ContextUsed:    "170k",
		ContextTotal:   "200k",
		AgentIcon:      "🤖",
	}

	result, err := f.Format(data)
	if err != nil {
		t.Errorf("Format() with full template failed: %v", err)
	}

	expectedParts := []string{"🤖", "DONE", "170k", "200k", "feature-branch", "(+5)", "test-session"}
	for _, part := range expectedParts {
		if !contains(result, part) {
			t.Errorf("Format() result missing expected part %q, got: %q", part, result)
		}
	}
}

// TestAllTemplatesWithContextUnavailable tests all templates handle missing context
func TestAllTemplatesWithContextUnavailable(t *testing.T) {
	templates := map[string]string{
		"default":     DefaultTemplate(),
		"minimal":     MinimalTemplate(),
		"compact":     CompactTemplate(),
		"multi-agent": MultiAgentTemplate(),
		"full":        FullTemplate(),
	}

	data := &session.StatusLineData{
		SessionName:    "test",
		State:          "DONE",
		StateColor:     "green",
		Branch:         "main",
		Uncommitted:    0,
		ContextPercent: -1, // Context unavailable
		ContextColor:   "grey",
		ContextUsed:    "",
		ContextTotal:   "",
		AgentType:      "claude",
		AgentIcon:      "🤖",
		RateLimit5h:    -1,
	}

	for name, tmpl := range templates {
		t.Run(name, func(t *testing.T) {
			f, err := NewFormatter(tmpl)
			if err != nil {
				t.Fatalf("NewFormatter() failed: %v", err)
			}

			result, err := f.Format(data)
			if err != nil {
				t.Errorf("Format() failed: %v", err)
			}

			// Should show "--" for unavailable context
			if !contains(result, "--") {
				t.Errorf("Format() should show '--' for unavailable context, got: %q", result)
			}
		})
	}
}

// TestTemplateTypeComparisonsRegression tests that templates use correct type comparisons
// Regression test for bug where templates compared float64 to int (e.g., ge .ContextPercent 0)
func TestTemplateTypeComparisonsRegression(t *testing.T) {
	tests := []struct {
		name          string
		template      string
		data          *session.StatusLineData
		expectError   bool
		errorContains string
	}{
		{
			name:     "float comparison with 0.0 works",
			template: "{{if ge .ContextPercent 0.0}}yes{{end}}",
			data: &session.StatusLineData{
				ContextPercent: 45.5,
			},
			expectError: false,
		},
		{
			name:     "float comparison with 0 fails",
			template: "{{if ge .ContextPercent 0}}yes{{end}}",
			data: &session.StatusLineData{
				ContextPercent: 45.5,
			},
			expectError:   true,
			errorContains: "incompatible types",
		},
		{
			name:     "int comparison with 0 works",
			template: "{{if gt .Uncommitted 0}}yes{{end}}",
			data: &session.StatusLineData{
				Uncommitted: 3,
			},
			expectError: false,
		},
		{
			name:     "int comparison with 0.0 fails",
			template: "{{if gt .Uncommitted 0.0}}yes{{end}}",
			data: &session.StatusLineData{
				Uncommitted: 3,
			},
			expectError:   true,
			errorContains: "incompatible types",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, err := NewFormatter(tt.template)
			if err != nil {
				t.Fatalf("NewFormatter() failed: %v", err)
			}

			_, err = f.Format(tt.data)
			if tt.expectError {
				if err == nil {
					t.Errorf("Format() expected error containing %q, got nil", tt.errorContains)
				} else if tt.errorContains != "" && !contains(err.Error(), tt.errorContains) {
					t.Errorf("Format() error = %q, want error containing %q", err.Error(), tt.errorContains)
				}
			} else {
				if err != nil {
					t.Errorf("Format() unexpected error: %v", err)
				}
			}
		})
	}
}

// TestDefaultTemplateTypeCorrectness verifies default template uses correct types
// This prevents regressions where templates use int literals with float fields
func TestDefaultTemplateTypeCorrectness(t *testing.T) {
	// Test all default templates with edge case values
	templates := map[string]string{
		"default":     DefaultTemplate(),
		"minimal":     MinimalTemplate(),
		"compact":     CompactTemplate(),
		"multi-agent": MultiAgentTemplate(),
		"full":        FullTemplate(),
	}

	// Edge case data: context at boundary values
	testData := []*session.StatusLineData{
		{
			SessionName:    "test",
			State:          "DONE",
			StateColor:     "green",
			Branch:         "main",
			Uncommitted:    0,   // Boundary: 0 uncommitted
			ContextPercent: 0.0, // Boundary: 0% context
			ContextColor:   "green",
			ContextUsed:    "0k",
			ContextTotal:   "200k",
			AgentType:      "claude",
			AgentIcon:      "🤖",
			Cost:           "$0.50",
			CostColor:      "green",
			ModelShort:     "Sonnet",
			RateLimit5h:    10.0,
			RateLimitColor: "green",
		},
		{
			SessionName:    "test",
			State:          "DONE",
			StateColor:     "green",
			Branch:         "main",
			Uncommitted:    1,   // Just above 0
			ContextPercent: 0.1, // Just above 0%
			ContextColor:   "green",
			ContextUsed:    "0k",
			ContextTotal:   "200k",
			AgentType:      "claude",
			AgentIcon:      "🤖",
			Cost:           "$5.00",
			CostColor:      "yellow",
			ModelShort:     "Opus",
			RateLimit5h:    60.0,
			RateLimitColor: "yellow",
		},
		{
			SessionName:    "test",
			State:          "OFFLINE",
			StateColor:     "grey",
			Branch:         "unknown",
			Uncommitted:    0,
			ContextPercent: -1.0, // Missing data sentinel
			ContextColor:   "grey",
			ContextUsed:    "",
			ContextTotal:   "",
			AgentType:      "claude",
			AgentIcon:      "🤖",
			RateLimit5h:    -1.0,
		},
	}

	for tmplName, tmpl := range templates {
		t.Run(tmplName, func(t *testing.T) {
			f, err := NewFormatter(tmpl)
			if err != nil {
				t.Fatalf("NewFormatter() failed for %q: %v", tmplName, err)
			}

			for i, data := range testData {
				_, err := f.Format(data)
				if err != nil {
					t.Errorf("Format() failed for %q with test data %d: %v", tmplName, i, err)
					t.Errorf("ContextPercent: %f, Uncommitted: %d", data.ContextPercent, data.Uncommitted)
				}
			}
		})
	}
}

// contains is a helper function to check if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && (s[:len(substr)] == substr ||
			contains(s[1:], substr))))
}
