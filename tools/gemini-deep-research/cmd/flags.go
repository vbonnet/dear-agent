package cmd

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/vbonnet/ai-tools/tools/gemini-deep-research/types"
)

// Validate validates the flags for conflicts and correctness
func Validate(f *types.Flags) error {
	// Check for mutually exclusive flags
	if f.Input != "" && f.InputFile != "" {
		return errors.New("cannot specify both --input and --input-file")
	}

	// Validate input file exists if specified
	if f.InputFile != "" {
		if _, err := os.Stat(f.InputFile); os.IsNotExist(err) {
			return fmt.Errorf("input file not found: %s", f.InputFile)
		} else if err != nil {
			return fmt.Errorf("cannot access input file %s: %v", f.InputFile, err)
		}
	}

	// Validate content type if specified
	if f.Type != "" {
		validTypes := map[string]bool{
			"video":       true,
			"article":     true,
			"arxiv":       true,
			"huggingface": true,
		}
		if !validTypes[f.Type] {
			return fmt.Errorf("invalid content type: %s. Valid types: video, article, arxiv, huggingface", f.Type)
		}
	}

	return nil
}

// ValidateURL validates the URL format
func ValidateURL(urlStr string) error {
	// Parse URL
	u, err := url.Parse(urlStr)
	if err != nil {
		return fmt.Errorf("invalid URL format: %s", urlStr)
	}

	// Check for required scheme
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("invalid URL scheme: %s (must be http or https)", u.Scheme)
	}

	// Check for host
	if u.Host == "" {
		return fmt.Errorf("invalid URL: missing host")
	}

	return nil
}

// GetPrompt retrieves the prompt from either --input flag or --input-file
// Returns empty string if neither is specified (use default prompt)
func GetPrompt(f *types.Flags) (string, error) {
	// If both are empty, return empty (use default)
	if f.Input == "" && f.InputFile == "" {
		return "", nil
	}

	// If --input is specified, return it
	if f.Input != "" {
		// If input is just whitespace, treat as empty (use default)
		if strings.TrimSpace(f.Input) == "" {
			return "", nil
		}
		return f.Input, nil
	}

	// If --input-file is specified, read it
	if f.InputFile != "" {
		content, err := os.ReadFile(f.InputFile)
		if err != nil {
			return "", fmt.Errorf("failed to read input file %s: %v", f.InputFile, err)
		}
		return string(content), nil
	}

	return "", nil
}
