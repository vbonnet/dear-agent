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

// D2Renderer implements the Renderer interface for D2 diagrams
type D2Renderer struct{}

// NewD2Renderer creates a new D2 renderer instance
func NewD2Renderer() *D2Renderer {
	return &D2Renderer{}
}

// Render generates output from D2 source using the d2 CLI
func (r *D2Renderer) Render(ctx context.Context, source io.Reader, dest io.Writer, opts *RenderOptions) error {
	if opts == nil {
		opts = &RenderOptions{
			OutputFormat: OutputSVG,
			LayoutEngine: LayoutELK,
		}
	}

	// Read source into buffer
	sourceData, err := io.ReadAll(source)
	if err != nil {
		return fmt.Errorf("failed to read D2 source: %w", err)
	}

	// Create temp directory for rendering
	tempDir, err := os.MkdirTemp("", "d2-render-*")
	if err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	// Write source to temp file
	sourceFile := filepath.Join(tempDir, "diagram.d2")
	if err := os.WriteFile(sourceFile, sourceData, 0644); err != nil {
		return fmt.Errorf("failed to write D2 source file: %w", err)
	}

	// Determine output file extension
	var ext string
	switch opts.OutputFormat {
	case OutputSVG:
		ext = "svg"
	case OutputPNG:
		ext = "png"
	case OutputPDF:
		ext = "pdf"
	case OutputJSON:
		return fmt.Errorf("unsupported output format for D2: %s", opts.OutputFormat)
	}

	outputFile := filepath.Join(tempDir, fmt.Sprintf("diagram.%s", ext))

	// Build d2 command
	args := []string{sourceFile, outputFile}

	// Add layout engine flag
	if opts.LayoutEngine != "" && opts.LayoutEngine != LayoutAuto {
		args = append(args, "--layout", string(opts.LayoutEngine))
	}

	// Add theme if specified
	if opts.Theme != "" {
		args = append(args, "--theme", opts.Theme)
	}

	// Add sketch mode if enabled
	if opts.Sketch {
		args = append(args, "--sketch")
	}

	// Execute d2 command
	cmd := exec.CommandContext(ctx, "d2", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("d2 rendering failed: %w (stderr: %s)", err, stderr.String())
	}

	// Read output and write to destination
	outputData, err := os.ReadFile(outputFile)
	if err != nil {
		return fmt.Errorf("failed to read rendered output: %w", err)
	}

	if _, err := dest.Write(outputData); err != nil {
		return fmt.Errorf("failed to write output: %w", err)
	}

	return nil
}

// Validate checks D2 syntax using dry-run compilation
func (r *D2Renderer) Validate(ctx context.Context, source io.Reader) error {
	sourceData, err := io.ReadAll(source)
	if err != nil {
		return fmt.Errorf("failed to read D2 source: %w", err)
	}

	// Create temp file for validation
	tempFile, err := os.CreateTemp("", "d2-validate-*.d2")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	defer os.Remove(tempFile.Name())

	if _, err := tempFile.Write(sourceData); err != nil {
		tempFile.Close()
		return fmt.Errorf("failed to write temp file: %w", err)
	}
	tempFile.Close()

	// Run d2 compile with dry-run (validation only)
	// Note: d2 doesn't have a dedicated validate flag, so we compile to /dev/null
	cmd := exec.CommandContext(ctx, "d2", tempFile.Name(), "/dev/null", "--dry-run")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("D2 validation failed: %s", stderr.String())
	}

	return nil
}

// SupportedFormats returns output formats supported by D2
func (r *D2Renderer) SupportedFormats() []OutputFormat {
	return []OutputFormat{OutputSVG, OutputPNG, OutputPDF}
}

// SupportedEngines returns layout engines supported by D2
func (r *D2Renderer) SupportedEngines() []LayoutEngine {
	return []LayoutEngine{LayoutELK, LayoutDagre, LayoutTALA}
}

// Format returns the diagram format
func (r *D2Renderer) Format() Format {
	return FormatD2
}

func init() {
	// Register D2 renderer with default registry
	DefaultRegistry.Register(NewD2Renderer())
}
