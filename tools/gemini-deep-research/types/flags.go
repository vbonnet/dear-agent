package types

import (
	"fmt"
	"os"
)

// Flags represents CLI flags for the application
type Flags struct {
	Input     string // --input: Short custom prompt
	InputFile string // --input-file: Path to file containing prompt
	Type      string // --type: Content type override
	OutputDir string // --output-dir: Output directory
	Timeout   int    // --timeout: Deep Research timeout in minutes
	Project   string // --project: GCP project ID
	Force     bool   // --force: Force refresh of existing research
}

// Validate validates flag values
func (f *Flags) Validate() error {
	// Check mutual exclusion of --input and --input-file
	if f.Input != "" && f.InputFile != "" {
		return fmt.Errorf("--input and --input-file are mutually exclusive")
	}

	// Check input file exists if specified
	if f.InputFile != "" {
		if _, err := os.Stat(f.InputFile); os.IsNotExist(err) {
			return fmt.Errorf("input file does not exist: %s", f.InputFile)
		}
	}

	// Validate type if specified
	if f.Type != "" {
		validTypes := map[string]bool{
			"video":       true,
			"article":     true,
			"arxiv":       true,
			"huggingface": true,
		}
		if !validTypes[f.Type] {
			return fmt.Errorf("invalid type: %s (must be: video, article, arxiv, huggingface)", f.Type)
		}
	}

	return nil
}

// GetPrompt returns the prompt text from either --input flag or --input-file
// Returns empty string if neither is specified (will use default prompt)
//
// Supports Azure @file syntax: if f.Input starts with '@', it reads the file.
// This provides backward compatibility: existing --input "text" works,
// and new --input "@file.txt" syntax works too.
func (f *Flags) GetPrompt() (string, error) {
	if f.Input != "" {
		// Support Azure @file syntax in --input flag
		// This allows: --input "@prompt.txt" as an alternative to --input-file
		if len(f.Input) > 0 && f.Input[0] == '@' {
			// Use ResolveFile to handle @file syntax
			// Import path: "github.com/vbonnet/ai-tools/tools/gemini-deep-research/config"
			// For now, inline the file reading to avoid circular import
			// (config package doesn't depend on types, but types would depend on config)
			filePath := f.Input[1:] // Remove @ prefix
			content, err := os.ReadFile(filePath)
			if err != nil {
				if os.IsNotExist(err) {
					return "", fmt.Errorf("file not found: %s\nTry: ls -la to check file path", filePath)
				}
				return "", fmt.Errorf("failed to read file '%s': %w", filePath, err)
			}
			return string(content), nil
		}
		return f.Input, nil
	}

	if f.InputFile != "" {
		content, err := os.ReadFile(f.InputFile)
		if err != nil {
			return "", fmt.Errorf("failed to read input file: %w", err)
		}
		return string(content), nil
	}

	// No custom prompt specified, will use default
	return "", nil
}
