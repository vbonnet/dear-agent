package renderer

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
)

// PlantUMLRenderer implements the Renderer interface for PlantUML diagrams
// This is a compatibility layer for migration support
type PlantUMLRenderer struct {
	jarPath string // Path to plantuml.jar
}

// NewPlantUMLRenderer creates a new PlantUML renderer
func NewPlantUMLRenderer(jarPath string) *PlantUMLRenderer {
	if jarPath == "" {
		// Try common locations
		jarPath = "/usr/share/plantuml/plantuml.jar"
	}
	return &PlantUMLRenderer{jarPath: jarPath}
}

// Render generates output from PlantUML source
func (r *PlantUMLRenderer) Render(ctx context.Context, source io.Reader, dest io.Writer, opts *RenderOptions) error {
	if opts == nil {
		opts = &RenderOptions{
			OutputFormat: OutputSVG,
		}
	}

	// Read source
	sourceData, err := io.ReadAll(source)
	if err != nil {
		return fmt.Errorf("failed to read PlantUML source: %w", err)
	}

	// Create temp directory
	tempDir, err := os.MkdirTemp("", "plantuml-render-*")
	if err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	// Write source to temp file
	sourceFile := filepath.Join(tempDir, "diagram.puml")
	if err := os.WriteFile(sourceFile, sourceData, 0o600); err != nil {
		return fmt.Errorf("failed to write PlantUML source file: %w", err)
	}

	// Determine output format flag
	var formatFlag string
	var ext string
	switch opts.OutputFormat {
	case OutputSVG:
		formatFlag = "-tsvg"
		ext = "svg"
	case OutputPNG:
		formatFlag = "-tpng"
		ext = "png"
	case OutputPDF:
		formatFlag = "-tpdf"
		ext = "pdf"
	case OutputJSON:
		return fmt.Errorf("unsupported output format for PlantUML: %s", opts.OutputFormat)
	}

	// Build plantuml command
	args := []string{"-jar", r.jarPath, formatFlag, sourceFile}

	// Execute plantuml command
	cmd := exec.CommandContext(ctx, "java", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	cmd.Dir = tempDir

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("plantuml rendering failed: %w (stderr: %s)", err, stderr.String())
	}

	// PlantUML outputs to same directory with different extension
	outputFile := filepath.Join(tempDir, fmt.Sprintf("diagram.%s", ext))

	// Read output
	outputData, err := os.ReadFile(outputFile)
	if err != nil {
		return fmt.Errorf("failed to read rendered output: %w", err)
	}

	if _, err := dest.Write(outputData); err != nil {
		return fmt.Errorf("failed to write output: %w", err)
	}

	return nil
}

// Validate checks PlantUML syntax
func (r *PlantUMLRenderer) Validate(ctx context.Context, source io.Reader) error {
	sourceData, err := io.ReadAll(source)
	if err != nil {
		return fmt.Errorf("failed to read PlantUML source: %w", err)
	}

	// Create temp file
	tempFile, err := os.CreateTemp("", "plantuml-validate-*.puml")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	defer os.Remove(tempFile.Name())

	if _, err := tempFile.Write(sourceData); err != nil {
		tempFile.Close()
		return fmt.Errorf("failed to write temp file: %w", err)
	}
	tempFile.Close()

	// PlantUML has a syntax check mode
	cmd := exec.CommandContext(ctx, "java", "-jar", r.jarPath, "-syntax", tempFile.Name())
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("PlantUML validation failed: %s", stderr.String())
	}

	return nil
}

// SupportedFormats returns output formats supported by PlantUML
func (r *PlantUMLRenderer) SupportedFormats() []OutputFormat {
	return []OutputFormat{OutputSVG, OutputPNG, OutputPDF}
}

// SupportedEngines returns layout engines (PlantUML uses GraphViz)
func (r *PlantUMLRenderer) SupportedEngines() []LayoutEngine {
	return []LayoutEngine{LayoutDot}
}

// Format returns the diagram format
func (r *PlantUMLRenderer) Format() Format {
	return FormatPlantUML
}

// Note: PlantUML renderer is NOT auto-registered since it's optional
// Users can register it manually if they have PlantUML installed
