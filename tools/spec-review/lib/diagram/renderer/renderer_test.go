package renderer

import (
	"testing"
)

func TestRegistry(t *testing.T) {
	registry := NewRegistry()

	// Register renderers
	registry.Register(NewD2Renderer())
	registry.Register(NewMermaidRenderer())
	registry.Register(NewStructurizrRenderer(""))

	// Test format retrieval
	tests := []struct {
		format Format
		want   bool
	}{
		{FormatD2, true},
		{FormatMermaid, true},
		{FormatStructurizr, true},
		{FormatPlantUML, false}, // Not registered by default
	}

	for _, tt := range tests {
		t.Run(string(tt.format), func(t *testing.T) {
			_, ok := registry.Get(tt.format)
			if ok != tt.want {
				t.Errorf("Registry.Get(%v) exists = %v, want %v", tt.format, ok, tt.want)
			}
		})
	}

	// Test formats list
	formats := registry.Formats()
	if len(formats) != 3 {
		t.Errorf("Registry.Formats() = %d formats, want 3", len(formats))
	}
}

func TestD2Renderer_Format(t *testing.T) {
	r := NewD2Renderer()
	if got := r.Format(); got != FormatD2 {
		t.Errorf("D2Renderer.Format() = %v, want %v", got, FormatD2)
	}
}

func TestD2Renderer_SupportedFormats(t *testing.T) {
	r := NewD2Renderer()
	formats := r.SupportedFormats()

	expected := []OutputFormat{OutputSVG, OutputPNG, OutputPDF}
	if len(formats) != len(expected) {
		t.Errorf("D2Renderer.SupportedFormats() = %d formats, want %d", len(formats), len(expected))
	}

	// Check each format exists
	for _, want := range expected {
		found := false
		for _, got := range formats {
			if got == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("D2Renderer.SupportedFormats() missing %v", want)
		}
	}
}

func TestMermaidRenderer_SupportedEngines(t *testing.T) {
	r := NewMermaidRenderer()
	engines := r.SupportedEngines()

	if len(engines) != 1 {
		t.Errorf("MermaidRenderer.SupportedEngines() = %d engines, want 1", len(engines))
	}

	if engines[0] != LayoutDagre {
		t.Errorf("MermaidRenderer.SupportedEngines()[0] = %v, want %v", engines[0], LayoutDagre)
	}
}
