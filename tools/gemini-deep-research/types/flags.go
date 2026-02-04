package types

import (
	"fmt"
	"os"
)

// Flags represents CLI flags for the application
type Flags struct {
	// Legacy flags (deprecated)
	Input     string // --input: Short custom prompt (DEPRECATED: use --analyze-prompt)
	InputFile string // --input-file: Path to file containing prompt (DEPRECATED: use --analyze-prompt)

	// New per-stage prompt flags
	ExtractPrompt  string // --extract-prompt: Custom prompt for extraction stage
	AnalyzePrompt  string // --analyze-prompt: Custom prompt for topic analysis stage
	ResearchPrompt string // --research-prompt: Custom prompt for deep research stage

	// Other flags
	Type      string // --type: Content type override
	Mode      string // --mode: Analysis mode (general|competitive)
	OutputDir string // --output-dir: Output directory
	Timeout   int    // --timeout: Deep Research timeout in minutes
	Project   string // --project: GCP project ID
	Force     bool   // --force: Force refresh of existing research

	// Competitive mode discovery flags
	NoDiscovery    bool // --no-discovery: Skip URL discovery in competitive mode
	DiscoveryLimit int  // --discovery-limit: Max URLs to discover (default: 5)
}

// Validate validates flag values
func (f *Flags) Validate() error {
	// Check mutual exclusion of legacy --input and --input-file
	if f.Input != "" && f.InputFile != "" {
		return fmt.Errorf("--input and --input-file are mutually exclusive")
	}

	// Check mutual exclusion of legacy --input and new --analyze-prompt
	if f.Input != "" && f.AnalyzePrompt != "" {
		return fmt.Errorf("--input and --analyze-prompt are mutually exclusive (use --analyze-prompt)")
	}

	// Check mutual exclusion of legacy --input-file and new --analyze-prompt
	if f.InputFile != "" && f.AnalyzePrompt != "" {
		return fmt.Errorf("--input-file and --analyze-prompt are mutually exclusive (use --analyze-prompt)")
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

	// Validate mode if specified
	if f.Mode != "" {
		validModes := map[string]bool{
			"general":     true,
			"competitive": true,
		}
		if !validModes[f.Mode] {
			return fmt.Errorf("invalid mode: %s (must be: general, competitive)", f.Mode)
		}
	}

	// Validate discovery limit if specified
	if f.DiscoveryLimit < 0 {
		return fmt.Errorf("invalid discovery-limit: %d (must be >= 0)", f.DiscoveryLimit)
	}
	if f.DiscoveryLimit > 20 {
		return fmt.Errorf("invalid discovery-limit: %d (must be <= 20 to avoid excessive API usage)", f.DiscoveryLimit)
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
