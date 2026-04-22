// Package statusline provides statusline functionality.
package statusline

import (
	"bytes"
	"fmt"
	"text/template"

	"github.com/vbonnet/dear-agent/agm/internal/session"
)

// Formatter renders status line data using templates
type Formatter struct {
	template *template.Template
}

// NewFormatter creates a formatter with the given template string
func NewFormatter(templateStr string) (*Formatter, error) {
	if templateStr == "" {
		return nil, fmt.Errorf("template string cannot be empty")
	}

	tmpl, err := template.New("statusline").Parse(templateStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse template: %w", err)
	}

	return &Formatter{template: tmpl}, nil
}

// Format renders the status line using the configured template
func (f *Formatter) Format(data *session.StatusLineData) (string, error) {
	if data == nil {
		return "", fmt.Errorf("status line data cannot be nil")
	}

	var buf bytes.Buffer
	if err := f.template.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to execute template: %w", err)
	}

	return buf.String(), nil
}

// DefaultTemplate returns the default status line template
// Format: 🤖 Opus DONE | 🪟 50k/200k | $1.23 | session-name
func DefaultTemplate() string {
	return "{{.AgentIcon}} {{if .ModelShort}}{{.ModelShort}} {{end}}" +
		"#[fg={{.StateColor}}]{{.State}}#[default] | " +
		"{{if ge .ContextPercent 0.0}}\U0001FA9F  #[fg={{.ContextColor}}]{{.ContextUsed}}#[default]/{{.ContextTotal}}{{else}}--{{end}}" +
		"{{if .Cost}} | #[fg={{.CostColor}}]{{.Cost}}#[default]{{end}} | " +
		"{{.SessionName}}"
}

// MinimalTemplate returns a minimal status line template
func MinimalTemplate() string {
	return "{{.AgentIcon}} {{.State}} | {{if ge .ContextPercent 0.0}}{{.ContextUsed}}/{{.ContextTotal}}{{else}}--{{end}}"
}

// CompactTemplate returns a compact status line template
func CompactTemplate() string {
	return "{{.AgentIcon}} #[fg={{.StateColor}}]\u25CF#[default] " +
		"{{if ge .ContextPercent 0.0}}{{.ContextUsed}}/{{.ContextTotal}}{{else}}--{{end}} | " +
		"{{.SessionName}}"
}

// MultiAgentTemplate returns a template that shows agent type
func MultiAgentTemplate() string {
	return "{{.AgentIcon}}{{.AgentType}} | " +
		"#[fg={{.StateColor}}]{{.State}}#[default] | " +
		"{{if ge .ContextPercent 0.0}}{{.ContextUsed}}/{{.ContextTotal}}{{else}}--{{end}}"
}

// FullTemplate returns a verbose status line template with git info
func FullTemplate() string {
	return "{{.AgentIcon}} #[fg={{.StateColor}}]{{.State}}#[default] | " +
		"{{if ge .ContextPercent 0.0}}\U0001FA9F  #[fg={{.ContextColor}}]{{.ContextUsed}}#[default]/{{.ContextTotal}}{{else}}--{{end}} | " +
		"{{.Branch}}{{if gt .Uncommitted 0}}(+{{.Uncommitted}}){{end}} | " +
		"{{.SessionName}}"
}
