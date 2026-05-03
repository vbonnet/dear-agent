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

// MermaidRenderer implements the Renderer interface for Mermaid diagrams
type MermaidRenderer struct{}

// NewMermaidRenderer creates a new Mermaid renderer
func NewMermaidRenderer() *MermaidRenderer {
	return &MermaidRenderer{}
}

// Render generates output from Mermaid source using mmdc CLI
func (r *MermaidRenderer) Render(ctx context.Context, source io.Reader, dest io.Writer, opts *RenderOptions) error {
	if opts == nil {
		opts = &RenderOptions{
			OutputFormat: OutputSVG,
		}
	}

	// Read source
	sourceData, err := io.ReadAll(source)
	if err != nil {
		return fmt.Errorf("failed to read Mermaid source: %w", err)
	}

	// Create temp directory
	tempDir, err := os.MkdirTemp("", "mermaid-render-*")
	if err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	// Write source to temp file
	sourceFile := filepath.Join(tempDir, "diagram.mmd")
	if err := os.WriteFile(sourceFile, sourceData, 0o600); err != nil {
		return fmt.Errorf("failed to write Mermaid source file: %w", err)
	}

	// Determine output format
	var ext string
	switch opts.OutputFormat {
	case OutputSVG:
		ext = "svg"
	case OutputPNG:
		ext = "png"
	case OutputPDF:
		ext = "pdf"
	case OutputJSON:
		return fmt.Errorf("unsupported output format for Mermaid: %s", opts.OutputFormat)
	}

	outputFile := filepath.Join(tempDir, fmt.Sprintf("diagram.%s", ext))

	// Build mmdc command
	args := []string{
		"-i", sourceFile,
		"-o", outputFile,
	}

	// Add theme if specified
	if opts.Theme != "" {
		args = append(args, "-t", opts.Theme)
	}

	// Add dimensions if specified
	if opts.Width > 0 {
		args = append(args, "-w", fmt.Sprintf("%d", opts.Width))
	}
	if opts.Height > 0 {
		args = append(args, "-H", fmt.Sprintf("%d", opts.Height))
	}

	// Execute mmdc command
	cmd := exec.CommandContext(ctx, "mmdc", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("mermaid rendering failed: %w (stderr: %s)", err, stderr.String())
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

// Validate checks Mermaid syntax (mmdc doesn't have a validate-only mode, so we do a dry render)
func (r *MermaidRenderer) Validate(ctx context.Context, source io.Reader) error {
	sourceData, err := io.ReadAll(source)
	if err != nil {
		return fmt.Errorf("failed to read Mermaid source: %w", err)
	}

	// Create temp file
	tempFile, err := os.CreateTemp("", "mermaid-validate-*.mmd")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	defer os.Remove(tempFile.Name())

	if _, err := tempFile.Write(sourceData); err != nil {
		tempFile.Close()
		return fmt.Errorf("failed to write temp file: %w", err)
	}
	tempFile.Close()

	// Try to render to a temp output (validation through rendering)
	tempOutput := tempFile.Name() + ".svg"
	defer os.Remove(tempOutput)

	cmd := exec.CommandContext(ctx, "mmdc", "-i", tempFile.Name(), "-o", tempOutput, "--quiet")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("mermaid validation failed: %s", stderr.String())
	}

	return nil
}

// SupportedFormats returns output formats supported by Mermaid
func (r *MermaidRenderer) SupportedFormats() []OutputFormat {
	return []OutputFormat{OutputSVG, OutputPNG, OutputPDF}
}

// SupportedEngines returns layout engines (Mermaid uses built-in dagre)
func (r *MermaidRenderer) SupportedEngines() []LayoutEngine {
	return []LayoutEngine{LayoutDagre}
}

// Format returns the diagram format
func (r *MermaidRenderer) Format() Format {
	return FormatMermaid
}

func init() {
	// Register Mermaid renderer with default registry
	DefaultRegistry.Register(NewMermaidRenderer())
}
