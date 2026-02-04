// Package templates handles loading and rendering of prompt templates.
package templates

import (
	"bytes"
	"embed"
	"fmt"
	"text/template"
)

//go:embed competitive/*.tmpl
var competitiveTemplates embed.FS

// PromptData holds variables for template rendering
type PromptData struct {
	Competitor string   // Competitor name (e.g., "GitHub Copilot")
	Target     string   // Our tool name (e.g., "Engram")
	URL        string   // URL being analyzed
	Topics     []string // Research topics from analysis stage
}

// TemplateType represents the type of competitive template
type TemplateType string

const (
	// TemplateAnalyze is used for extracting competitor capabilities
	TemplateAnalyze TemplateType = "analyze"
	// TemplateGapAnalysis is used for comparing capabilities and identifying gaps
	TemplateGapAnalysis TemplateType = "gap-analysis"
)

// Loader handles template loading and rendering
type Loader struct {
	templates map[string]*template.Template
}

// NewLoader creates a new template loader and validates all templates
func NewLoader() (*Loader, error) {
	loader := &Loader{
		templates: make(map[string]*template.Template),
	}

	// Load and validate templates
	templateFiles := []struct {
		name string
		path string
	}{
		{name: "analyze", path: "competitive/analyze.tmpl"},
		{name: "gap-analysis", path: "competitive/gap-analysis.tmpl"},
	}

	for _, tf := range templateFiles {
		content, err := competitiveTemplates.ReadFile(tf.path)
		if err != nil {
			return nil, fmt.Errorf("failed to read template %s: %w", tf.name, err)
		}

		tmpl, err := template.New(tf.name).Parse(string(content))
		if err != nil {
			return nil, fmt.Errorf("failed to parse template %s: %w", tf.name, err)
		}

		loader.templates[tf.name] = tmpl
	}

	return loader, nil
}

// Render renders a template with the provided data
func (l *Loader) Render(templateType TemplateType, data PromptData) (string, error) {
	tmpl, ok := l.templates[string(templateType)]
	if !ok {
		return "", fmt.Errorf("template not found: %s", templateType)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to execute template %s: %w", templateType, err)
	}

	return buf.String(), nil
}

// ValidateData validates that PromptData has required fields
func ValidateData(data PromptData) error {
	if data.Competitor == "" {
		return fmt.Errorf("competitor name is required")
	}
	if data.Target == "" {
		return fmt.Errorf("target name is required")
	}
	if data.URL == "" {
		return fmt.Errorf("URL is required")
	}
	// Topics can be empty (will be populated by analysis stage)
	return nil
}
