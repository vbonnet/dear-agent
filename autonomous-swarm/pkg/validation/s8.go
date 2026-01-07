package validation

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// S8ValidationResult represents the result of S8 file validation
type S8ValidationResult struct {
	Valid  bool
	Errors []string
	Files  []string // Files validated
}

// ValidateS8 validates file existence and syntax for S8 implementation files
func ValidateS8(filePaths []string) (*S8ValidationResult, error) {
	result := &S8ValidationResult{
		Valid:  true,
		Errors: []string{},
		Files:  []string{},
	}

	for _, filePath := range filePaths {
		result.Files = append(result.Files, filePath)

		// Check file existence
		if _, err := os.Stat(filePath); err != nil {
			result.Valid = false
			result.Errors = append(result.Errors, fmt.Sprintf("%s: file not found", filePath))
			continue
		}

		// Determine file type and validate syntax
		ext := filepath.Ext(filePath)

		switch ext {
		case ".go":
			if err := validateGoSyntax(filePath); err != nil {
				result.Valid = false
				result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", filePath, err))
			}
		case ".yaml", ".yml":
			if err := validateYAMLSyntax(filePath); err != nil {
				result.Valid = false
				result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", filePath, err))
			}
		default:
			// Unknown file type, skip syntax validation
			continue
		}
	}

	return result, nil
}

// validateGoSyntax validates Go file syntax using gofmt
func validateGoSyntax(filePath string) error {
	cmd := exec.Command("gofmt", "-e", filePath)
	output, err := cmd.CombinedOutput()

	if err != nil {
		return fmt.Errorf("syntax error: %w", err)
	}

	// gofmt -e prints errors to stdout
	if strings.Contains(string(output), "expected") || strings.Contains(string(output), "error") {
		return fmt.Errorf("syntax error: %s", string(output))
	}

	return nil
}

// validateYAMLSyntax validates YAML file syntax using yaml.Unmarshal
func validateYAMLSyntax(filePath string) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	var content interface{}
	if err := yaml.Unmarshal(data, &content); err != nil {
		return fmt.Errorf("syntax error: %w", err)
	}

	return nil
}
