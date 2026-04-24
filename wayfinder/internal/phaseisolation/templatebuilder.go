package phaseisolation

import (
	"fmt"
	"strings"
)

// TemplateBuilder provides a fluent API for building multi-section markdown strings.
type TemplateBuilder struct {
	sections []string
}

// NewTemplateBuilder creates a new TemplateBuilder.
func NewTemplateBuilder() *TemplateBuilder {
	return &TemplateBuilder{}
}

// Heading adds a markdown heading (h1-h6).
func (tb *TemplateBuilder) Heading(level int, text string) *TemplateBuilder {
	if level < 1 || level > 6 {
		panic(fmt.Sprintf("invalid heading level: %d, must be 1-6", level))
	}
	tb.sections = append(tb.sections, strings.Repeat("#", level)+" "+text, "")
	return tb
}

// Text adds a text paragraph with trailing blank line.
func (tb *TemplateBuilder) Text(content string) *TemplateBuilder {
	tb.sections = append(tb.sections, content, "")
	return tb
}

// List adds a bulleted or numbered list.
func (tb *TemplateBuilder) List(items []string, ordered bool) *TemplateBuilder {
	for i, item := range items {
		prefix := "-"
		if ordered {
			prefix = fmt.Sprintf("%d.", i+1)
		}
		tb.sections = append(tb.sections, prefix+" "+item)
	}
	tb.sections = append(tb.sections, "")
	return tb
}

// Section adds a section (h2 heading + content).
func (tb *TemplateBuilder) Section(title string, content ...string) *TemplateBuilder {
	tb.Heading(2, title)
	for _, c := range content {
		tb.Text(c)
	}
	return tb
}

// Build returns the final string.
func (tb *TemplateBuilder) Build() string {
	if len(tb.sections) == 0 {
		return ""
	}
	return strings.Join(tb.sections, "\n") + "\n"
}
