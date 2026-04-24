// Package gateway provides gateway functionality.
package gateway

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"text/template"
)

// HandoffPromptGenerator creates prompts for mode transitions.
type HandoffPromptGenerator struct {
	templates map[string]*template.Template
}

// NewHandoffPromptGenerator creates a new prompt generator.
func NewHandoffPromptGenerator() (*HandoffPromptGenerator, error) {
	gen := &HandoffPromptGenerator{
		templates: make(map[string]*template.Template),
	}

	// Register templates
	if err := gen.registerTemplates(); err != nil {
		return nil, fmt.Errorf("failed to register templates: %w", err)
	}

	return gen, nil
}

// GeneratePrompt creates a formatted prompt for mode transition.
func (g *HandoffPromptGenerator) GeneratePrompt(handoff *HandoffContext) (string, error) {
	transition := fmt.Sprintf("%s_to_%s", handoff.FromMode, handoff.ToMode)

	tmpl, exists := g.templates[transition]
	if !exists {
		// Fall back to generic template
		tmpl = g.templates["generic"]
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, handoff); err != nil {
		return "", fmt.Errorf("failed to execute template: %w", err)
	}

	return buf.String(), nil
}

// SerializeContext converts handoff context to JSON for persistence.
func (g *HandoffPromptGenerator) SerializeContext(handoff *HandoffContext) (string, error) {
	data, err := json.MarshalIndent(handoff, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to serialize context: %w", err)
	}

	return string(data), nil
}

// DeserializeContext loads handoff context from JSON.
func (g *HandoffPromptGenerator) DeserializeContext(data string) (*HandoffContext, error) {
	var handoff HandoffContext
	if err := json.Unmarshal([]byte(data), &handoff); err != nil {
		return nil, fmt.Errorf("failed to deserialize context: %w", err)
	}

	return &handoff, nil
}

// registerTemplates loads all prompt templates.
func (g *HandoffPromptGenerator) registerTemplates() error {
	// Define custom template functions
	funcMap := template.FuncMap{
		"add": func(a, b int) int {
			return a + b
		},
	}

	templates := map[string]string{
		"architect_to_implementer": architectToImplementerTemplate,
		"implementer_to_architect": implementerToArchitectTemplate,
		"generic":                  genericHandoffTemplate,
	}

	for name, tmplStr := range templates {
		tmpl, err := template.New(name).Funcs(funcMap).Parse(tmplStr)
		if err != nil {
			return fmt.Errorf("failed to parse template %s: %w", name, err)
		}
		g.templates[name] = tmpl
	}

	return nil
}

// FormatArtifacts creates a human-readable list of artifacts.
func FormatArtifacts(artifacts map[string]string) string {
	if len(artifacts) == 0 {
		return "No artifacts"
	}

	var parts []string
	for name, path := range artifacts {
		parts = append(parts, fmt.Sprintf("- %s: %s", name, path))
	}

	return strings.Join(parts, "\n")
}

// FormatNextSteps creates a formatted checklist.
func FormatNextSteps(steps []string) string {
	if len(steps) == 0 {
		return "No specific next steps defined"
	}

	var parts []string
	for i, step := range steps {
		parts = append(parts, fmt.Sprintf("%d. %s", i+1, step))
	}

	return strings.Join(parts, "\n")
}

// Template: Architect → Implementer
const architectToImplementerTemplate = `# Mode Transition: Architect → Implementer

## Context from Architect Mode

{{.Summary}}

## Design Artifacts

{{template "artifacts" .}}

## Implementation Tasks

You are now in Implementer mode. Your task is to execute the plan created by Architect mode.

**Next Steps:**
{{template "next_steps" .}}

## Guidelines

1. **Follow the Design**: Implement according to the architecture plan
2. **Write Tests**: Add comprehensive tests for all code
3. **Stay Focused**: Focus on implementation, not redesign
4. **Report Issues**: If you find design flaws, document them for Architect review

## Metadata

- Transition timestamp: {{.Timestamp}}
- Previous mode: {{.FromMode}}
- Current mode: {{.ToMode}}

---

{{template "artifacts" .}}
{{define "artifacts"}}{{if .Artifacts}}{{range $name, $path := .Artifacts}}- {{$name}}: {{$path}}
{{end}}{{else}}No artifacts{{end}}{{end}}

{{template "next_steps" .}}
{{define "next_steps"}}{{if .NextSteps}}{{range $i, $step := .NextSteps}}{{add $i 1}}. {{$step}}
{{end}}{{else}}No specific next steps{{end}}{{end}}
`

// Template: Implementer → Architect
const implementerToArchitectTemplate = `# Mode Transition: Implementer → Architect

## Context from Implementer Mode

{{.Summary}}

## Implementation Artifacts

{{template "artifacts" .}}

## Review Tasks

You are now in Architect mode. Your task is to review the implementation and plan the next iteration.

**Next Steps:**
{{template "next_steps" .}}

## Guidelines

1. **Review Architecture**: Assess if implementation follows design
2. **Identify Issues**: Document architectural inconsistencies
3. **Plan Improvements**: Design refactoring or enhancements
4. **Strategic Decisions**: Make high-level decisions about next steps

## Metadata

- Transition timestamp: {{.Timestamp}}
- Previous mode: {{.FromMode}}
- Current mode: {{.ToMode}}

---

{{template "artifacts" .}}
{{define "artifacts"}}{{if .Artifacts}}{{range $name, $path := .Artifacts}}- {{$name}}: {{$path}}
{{end}}{{else}}No artifacts{{end}}{{end}}

{{template "next_steps" .}}
{{define "next_steps"}}{{if .NextSteps}}{{range $i, $step := .NextSteps}}{{add $i 1}}. {{$step}}
{{end}}{{else}}No specific next steps{{end}}{{end}}
`

// Template: Generic handoff
const genericHandoffTemplate = `# Mode Transition: {{.FromMode}} → {{.ToMode}}

## Context Summary

{{.Summary}}

## Artifacts

{{template "artifacts" .}}

## Next Steps

{{template "next_steps" .}}

## Metadata

- Transition timestamp: {{.Timestamp}}
- Previous mode: {{.FromMode}}
- Current mode: {{.ToMode}}

---

{{template "artifacts" .}}
{{define "artifacts"}}{{if .Artifacts}}{{range $name, $path := .Artifacts}}- {{$name}}: {{$path}}
{{end}}{{else}}No artifacts{{end}}{{end}}

{{template "next_steps" .}}
{{define "next_steps"}}{{if .NextSteps}}{{range $i, $step := .NextSteps}}{{add $i 1}}. {{$step}}
{{end}}{{else}}No specific next steps{{end}}{{end}}
`
