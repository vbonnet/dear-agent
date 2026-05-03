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

// StructurizrRenderer implements the Renderer interface for Structurizr DSL
type StructurizrRenderer struct {
	cliPath string // Path to structurizr.sh
}

// NewStructurizrRenderer creates a new Structurizr renderer
func NewStructurizrRenderer(cliPath string) *StructurizrRenderer {
	if cliPath == "" {
		// Default path
		cliPath = "/tmp/structurizr/structurizr.sh"
	}
	return &StructurizrRenderer{cliPath: cliPath}
}

// Render generates output from Structurizr DSL using the CLI
func (r *StructurizrRenderer) Render(ctx context.Context, source io.Reader, dest io.Writer, opts *RenderOptions) error {
	if opts == nil {
		opts = &RenderOptions{
			OutputFormat: OutputJSON,
		}
	}

	// Read source
	sourceData, err := io.ReadAll(source)
	if err != nil {
		return fmt.Errorf("failed to read Structurizr source: %w", err)
	}

	// Create temp directory
	tempDir, err := os.MkdirTemp("", "structurizr-render-*")
	if err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	// Write source to workspace.dsl
	workspaceFile := filepath.Join(tempDir, "workspace.dsl")
	if err := os.WriteFile(workspaceFile, sourceData, 0o600); err != nil {
		return fmt.Errorf("failed to write workspace file: %w", err)
	}

	// Structurizr CLI exports to various formats
	// For now, we export to JSON and PlantUML
	var cmd *exec.Cmd
	var outputFile string

	switch opts.OutputFormat {
	case OutputJSON:
		// Export to JSON
		cmd = exec.CommandContext(ctx, r.cliPath, "export", "-workspace", workspaceFile, "-format", "json", "-output", tempDir)
		outputFile = filepath.Join(tempDir, "workspace.json")
	case OutputSVG, OutputPNG:
		// Export to PlantUML, then convert (future enhancement)
		cmd = exec.CommandContext(ctx, r.cliPath, "export", "-workspace", workspaceFile, "-format", "plantuml", "-output", tempDir)
		outputFile = filepath.Join(tempDir, "structurizr-SystemContext.puml")
	case OutputPDF:
		return fmt.Errorf("unsupported output format for Structurizr: %s", opts.OutputFormat)
	}

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("structurizr export failed: %w (stderr: %s)", err, stderr.String())
	}

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

// Validate checks Structurizr DSL syntax
func (r *StructurizrRenderer) Validate(ctx context.Context, source io.Reader) error {
	sourceData, err := io.ReadAll(source)
	if err != nil {
		return fmt.Errorf("failed to read Structurizr source: %w", err)
	}

	// Create temp file
	tempFile, err := os.CreateTemp("", "structurizr-validate-*.dsl")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	defer os.Remove(tempFile.Name())

	if _, err := tempFile.Write(sourceData); err != nil {
		tempFile.Close()
		return fmt.Errorf("failed to write temp file: %w", err)
	}
	tempFile.Close()

	// Run structurizr validate command
	cmd := exec.CommandContext(ctx, r.cliPath, "validate", "-workspace", tempFile.Name())
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("structurizr validation failed: %s", stderr.String())
	}

	return nil
}

// SupportedFormats returns output formats supported by Structurizr
func (r *StructurizrRenderer) SupportedFormats() []OutputFormat {
	return []OutputFormat{OutputJSON}
}

// SupportedEngines returns layout engines (Structurizr uses GraphViz internally)
func (r *StructurizrRenderer) SupportedEngines() []LayoutEngine {
	return []LayoutEngine{LayoutDot}
}

// Format returns the diagram format
func (r *StructurizrRenderer) Format() Format {
	return FormatStructurizr
}

func init() {
	// Register Structurizr renderer with default registry
	DefaultRegistry.Register(NewStructurizrRenderer(""))
}
