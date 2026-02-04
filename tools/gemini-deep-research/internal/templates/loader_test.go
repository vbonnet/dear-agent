package templates

import (
	"strings"
	"testing"
)

func TestNewLoader(t *testing.T) {
	loader, err := NewLoader()
	if err != nil {
		t.Fatalf("NewLoader() failed: %v", err)
	}

	if loader == nil {
		t.Fatal("NewLoader() returned nil loader")
	}

	// Verify both templates were loaded
	expectedTemplates := []string{"analyze", "gap-analysis"}
	for _, name := range expectedTemplates {
		if _, ok := loader.templates[name]; !ok {
			t.Errorf("NewLoader() did not load template: %s", name)
		}
	}
}

func TestLoader_Render_Analyze(t *testing.T) {
	loader, err := NewLoader()
	if err != nil {
		t.Fatalf("NewLoader() failed: %v", err)
	}

	data := PromptData{
		Competitor: "GitHub Copilot",
		Target:     "Engram",
		URL:        "https://github.com/features/copilot",
		Topics:     []string{"Code completion", "AI assistance", "IDE integration"},
	}

	result, err := loader.Render(TemplateAnalyze, data)
	if err != nil {
		t.Fatalf("Render() failed: %v", err)
	}

	// Verify key content is present
	expectedStrings := []string{
		"GitHub Copilot",
		"Engram",
		"https://github.com/features/copilot",
		"Code completion",
		"AI assistance",
		"IDE integration",
		"Competitor Capabilities",
		"Focus Areas",
	}

	for _, expected := range expectedStrings {
		if !strings.Contains(result, expected) {
			t.Errorf("Render() result missing expected string: %q", expected)
		}
	}
}

func TestLoader_Render_GapAnalysis(t *testing.T) {
	loader, err := NewLoader()
	if err != nil {
		t.Fatalf("NewLoader() failed: %v", err)
	}

	data := PromptData{
		Competitor: "Notion",
		Target:     "Obsidian",
		URL:        "https://www.notion.so",
		Topics:     []string{"Knowledge management", "Collaboration", "Templates"},
	}

	result, err := loader.Render(TemplateGapAnalysis, data)
	if err != nil {
		t.Fatalf("Render() failed: %v", err)
	}

	// Verify key content is present
	expectedStrings := []string{
		"Notion",
		"Obsidian",
		"https://www.notion.so",
		"Knowledge management",
		"Collaboration",
		"Templates",
		"Competitor Strengths",
		"Our Gaps",
		"Prioritized Recommendations",
	}

	for _, expected := range expectedStrings {
		if !strings.Contains(result, expected) {
			t.Errorf("Render() result missing expected string: %q", expected)
		}
	}
}

func TestLoader_Render_InvalidTemplate(t *testing.T) {
	loader, err := NewLoader()
	if err != nil {
		t.Fatalf("NewLoader() failed: %v", err)
	}

	data := PromptData{
		Competitor: "Test",
		Target:     "Test",
		URL:        "https://example.com",
	}

	_, err = loader.Render("nonexistent", data)
	if err == nil {
		t.Error("Render() should fail for nonexistent template")
	}

	expectedError := "template not found"
	if !strings.Contains(err.Error(), expectedError) {
		t.Errorf("Render() error = %q, want substring %q", err.Error(), expectedError)
	}
}

func TestLoader_Render_EmptyTopics(t *testing.T) {
	loader, err := NewLoader()
	if err != nil {
		t.Fatalf("NewLoader() failed: %v", err)
	}

	data := PromptData{
		Competitor: "Competitor",
		Target:     "Target",
		URL:        "https://example.com",
		Topics:     []string{}, // Empty topics
	}

	// Should render without error (topics are optional)
	result, err := loader.Render(TemplateAnalyze, data)
	if err != nil {
		t.Fatalf("Render() failed with empty topics: %v", err)
	}

	if !strings.Contains(result, "Competitor") {
		t.Error("Render() with empty topics should still render template")
	}
}

func TestValidateData(t *testing.T) {
	tests := []struct {
		name      string
		data      PromptData
		wantError string
	}{
		{
			name: "valid data - all fields",
			data: PromptData{
				Competitor: "GitHub Copilot",
				Target:     "Engram",
				URL:        "https://github.com/features/copilot",
				Topics:     []string{"Code completion"},
			},
			wantError: "",
		},
		{
			name: "valid data - no topics",
			data: PromptData{
				Competitor: "GitHub Copilot",
				Target:     "Engram",
				URL:        "https://github.com/features/copilot",
				Topics:     []string{},
			},
			wantError: "",
		},
		{
			name: "missing competitor",
			data: PromptData{
				Competitor: "",
				Target:     "Engram",
				URL:        "https://example.com",
			},
			wantError: "competitor name is required",
		},
		{
			name: "missing target",
			data: PromptData{
				Competitor: "GitHub Copilot",
				Target:     "",
				URL:        "https://example.com",
			},
			wantError: "target name is required",
		},
		{
			name: "missing URL",
			data: PromptData{
				Competitor: "GitHub Copilot",
				Target:     "Engram",
				URL:        "",
			},
			wantError: "URL is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateData(tt.data)
			if tt.wantError == "" {
				if err != nil {
					t.Errorf("ValidateData() unexpected error: %v", err)
				}
			} else {
				if err == nil {
					t.Errorf("ValidateData() expected error containing %q, got nil", tt.wantError)
					return
				}
				if !strings.Contains(err.Error(), tt.wantError) {
					t.Errorf("ValidateData() error = %q, want substring %q", err.Error(), tt.wantError)
				}
			}
		})
	}
}

func TestLoader_Render_VariableSubstitution(t *testing.T) {
	loader, err := NewLoader()
	if err != nil {
		t.Fatalf("NewLoader() failed: %v", err)
	}

	data := PromptData{
		Competitor: "Special <Chars> & \"Quotes\"",
		Target:     "Tool with 'Apostrophes'",
		URL:        "https://example.com?param=value&other=test",
		Topics:     []string{"Topic 1", "Topic 2"},
	}

	// Should handle special characters without errors
	result, err := loader.Render(TemplateAnalyze, data)
	if err != nil {
		t.Fatalf("Render() failed with special characters: %v", err)
	}

	// Verify special characters are preserved
	if !strings.Contains(result, "Special <Chars> & \"Quotes\"") {
		t.Error("Render() did not preserve special characters in Competitor field")
	}
	if !strings.Contains(result, "Tool with 'Apostrophes'") {
		t.Error("Render() did not preserve apostrophes in Target field")
	}
}

func TestLoader_Render_MultipleTopics(t *testing.T) {
	loader, err := NewLoader()
	if err != nil {
		t.Fatalf("NewLoader() failed: %v", err)
	}

	data := PromptData{
		Competitor: "Competitor",
		Target:     "Target",
		URL:        "https://example.com",
		Topics: []string{
			"Topic 1",
			"Topic 2",
			"Topic 3",
			"Topic 4",
			"Topic 5",
		},
	}

	result, err := loader.Render(TemplateAnalyze, data)
	if err != nil {
		t.Fatalf("Render() failed: %v", err)
	}

	// Verify all topics are rendered
	for _, topic := range data.Topics {
		if !strings.Contains(result, topic) {
			t.Errorf("Render() result missing topic: %q", topic)
		}
	}
}

// TestLoader_Security_NoFilesystemAccess tests that templates cannot access filesystem
func TestLoader_Security_NoFilesystemAccess(t *testing.T) {
	// text/template does not provide filesystem functions by default
	// This test verifies the security model is correct

	loader, err := NewLoader()
	if err != nil {
		t.Fatalf("NewLoader() failed: %v", err)
	}

	// Attempt to render with data that might try filesystem access
	data := PromptData{
		Competitor: "/etc/passwd",
		Target:     "../../../etc/passwd",
		URL:        "file:///etc/passwd",
		Topics:     []string{"../../sensitive/file.txt"},
	}

	// Should render successfully but treat paths as plain text
	result, err := loader.Render(TemplateAnalyze, data)
	if err != nil {
		t.Fatalf("Render() failed: %v", err)
	}

	// Verify paths are treated as plain text (not executed)
	if strings.Contains(result, "root:x:0:0") {
		t.Error("Template should not have filesystem access")
	}

	// Verify our "malicious" input is just rendered as text
	if !strings.Contains(result, "/etc/passwd") {
		t.Error("Template should render filesystem paths as plain text")
	}
}

func TestTemplateType_Constants(t *testing.T) {
	// Verify template type constants are defined correctly
	if TemplateAnalyze != "analyze" {
		t.Errorf("TemplateAnalyze = %q, want %q", TemplateAnalyze, "analyze")
	}
	if TemplateGapAnalysis != "gap-analysis" {
		t.Errorf("TemplateGapAnalysis = %q, want %q", TemplateGapAnalysis, "gap-analysis")
	}
}
