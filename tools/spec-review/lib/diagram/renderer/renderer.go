// Package renderer provides a unified interface for rendering diagrams from various formats.
package renderer

import (
	"context"
	"io"
)

// Format represents a diagram format type
type Format string

// Supported diagram source formats.
const (
	FormatD2          Format = "d2"
	FormatStructurizr Format = "structurizr"
	FormatMermaid     Format = "mermaid"
	FormatPlantUML    Format = "plantuml"
)

// OutputFormat represents the output rendering format
type OutputFormat string

// Supported diagram output formats.
const (
	OutputSVG  OutputFormat = "svg"
	OutputPNG  OutputFormat = "png"
	OutputPDF  OutputFormat = "pdf"
	OutputJSON OutputFormat = "json"
)

// LayoutEngine represents the layout algorithm to use
type LayoutEngine string

// Supported layout engine names.
const (
	LayoutELK   LayoutEngine = "elk"
	LayoutDagre LayoutEngine = "dagre"
	LayoutTALA  LayoutEngine = "tala"
	LayoutDot   LayoutEngine = "dot"
	LayoutAuto  LayoutEngine = "auto"
)

// RenderOptions contains configuration for diagram rendering
type RenderOptions struct {
	OutputFormat OutputFormat
	LayoutEngine LayoutEngine
	Theme        string
	Width        int
	Height       int
	Sketch       bool
	// Additional format-specific options
	Extra map[string]interface{}
}

// Renderer defines the interface for diagram rendering
type Renderer interface {
	// Render processes diagram source and writes output
	Render(ctx context.Context, source io.Reader, dest io.Writer, opts *RenderOptions) error

	// Validate checks if the diagram source is syntactically valid
	Validate(ctx context.Context, source io.Reader) error

	// SupportedFormats returns the output formats supported by this renderer
	SupportedFormats() []OutputFormat

	// SupportedEngines returns the layout engines supported by this renderer
	SupportedEngines() []LayoutEngine

	// Format returns the diagram format this renderer handles
	Format() Format
}

// Registry manages available renderers
type Registry struct {
	renderers map[Format]Renderer
}

// NewRegistry creates a new renderer registry
func NewRegistry() *Registry {
	return &Registry{
		renderers: make(map[Format]Renderer),
	}
}

// Register adds a renderer to the registry
func (r *Registry) Register(renderer Renderer) {
	r.renderers[renderer.Format()] = renderer
}

// Get retrieves a renderer for the specified format
func (r *Registry) Get(format Format) (Renderer, bool) {
	renderer, ok := r.renderers[format]
	return renderer, ok
}

// Formats returns all registered diagram formats
func (r *Registry) Formats() []Format {
	formats := make([]Format, 0, len(r.renderers))
	for format := range r.renderers {
		formats = append(formats, format)
	}
	return formats
}

// DefaultRegistry is the global registry instance
var DefaultRegistry = NewRegistry()
