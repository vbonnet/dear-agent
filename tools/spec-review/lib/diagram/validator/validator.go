// Package validator provides unified syntax validation for diagram formats
package validator

import (
	"context"
	"fmt"
	"io"

	"github.com/vbonnet/dear-agent/tools/spec-review/lib/diagram/renderer"
)

// SyntaxValidator validates diagram syntax
type SyntaxValidator struct {
	registry *renderer.Registry
}

// NewSyntaxValidator creates a new syntax validator
func NewSyntaxValidator() *SyntaxValidator {
	return &SyntaxValidator{
		registry: renderer.DefaultRegistry,
	}
}

// Validate checks diagram syntax for the specified format
func (v *SyntaxValidator) Validate(ctx context.Context, format renderer.Format, source io.Reader) error {
	r, ok := v.registry.Get(format)
	if !ok {
		return fmt.Errorf("no renderer found for format: %s", format)
	}

	return r.Validate(ctx, source)
}

// ValidateD2 validates D2 diagram syntax
func (v *SyntaxValidator) ValidateD2(ctx context.Context, source io.Reader) error {
	return v.Validate(ctx, renderer.FormatD2, source)
}

// ValidateStructurizr validates Structurizr DSL syntax
func (v *SyntaxValidator) ValidateStructurizr(ctx context.Context, source io.Reader) error {
	return v.Validate(ctx, renderer.FormatStructurizr, source)
}

// ValidateMermaid validates Mermaid diagram syntax
func (v *SyntaxValidator) ValidateMermaid(ctx context.Context, source io.Reader) error {
	return v.Validate(ctx, renderer.FormatMermaid, source)
}

// ValidatePlantUML validates PlantUML diagram syntax
func (v *SyntaxValidator) ValidatePlantUML(ctx context.Context, source io.Reader) error {
	return v.Validate(ctx, renderer.FormatPlantUML, source)
}

// ValidateAll checks if source is valid for any supported format
// Returns the detected format and any validation error
func (v *SyntaxValidator) ValidateAll(ctx context.Context, source io.Reader) (renderer.Format, error) {
	// Try to read source once
	data, err := io.ReadAll(source)
	if err != nil {
		return "", fmt.Errorf("failed to read source: %w", err)
	}

	// Try each format
	formats := v.registry.Formats()
	for _, format := range formats {
		r, ok := v.registry.Get(format)
		if !ok {
			continue
		}

		// Create new reader for this attempt
		reader := &readerAdapter{data: data}

		if err := r.Validate(ctx, reader); err == nil {
			// Valid for this format
			return format, nil
		}
	}

	return "", fmt.Errorf("source is not valid for any supported format")
}

// readerAdapter implements io.Reader for validation attempts
type readerAdapter struct {
	data []byte
	pos  int
}

func (r *readerAdapter) Read(p []byte) (n int, err error) {
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}
	n = copy(p, r.data[r.pos:])
	r.pos += n
	return n, nil
}
