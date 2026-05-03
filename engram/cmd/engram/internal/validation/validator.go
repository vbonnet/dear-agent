// Package validation provides validation-related functionality.
package validation

import (
	"path/filepath"
	"regexp"
	"strings"

	"github.com/vbonnet/dear-agent/engram/cmd/engram/internal/cli"
)

// ValidateProjectName checks if the project name is valid
func ValidateProjectName(name string) error {
	// Only allow alphanumeric characters and hyphens/underscores
	if name == "" {
		return cli.InvalidInputError("project name", name, "non-empty string with alphanumeric characters, hyphens, or underscores")
	}

	projectNameRegex := regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)
	if !projectNameRegex.MatchString(name) {
		return cli.InvalidInputError("project name", name, "alphanumeric characters, hyphens, or underscores")
	}

	return nil
}

// ValidateEngramName checks if the engram name is valid for markdown filename
func ValidateEngramName(name string) error {
	if name == "" {
		return cli.InvalidInputError("engram name", name, "non-empty string")
	}

	// Check for invalid characters (reject emojis, special chars)
	validNameRegex := regexp.MustCompile(`^[a-zA-Z0-9-_. ]+$`)
	if !validNameRegex.MatchString(name) {
		return cli.InvalidInputError("engram name", name, "alphanumeric characters, hyphens, underscores, dots, or spaces only")
	}

	// Ensure name has some valid content after removing spaces
	sanitizedName := regexp.MustCompile(`[^a-zA-Z0-9-_.]`).ReplaceAllString(name, "")
	if sanitizedName == "" {
		return cli.InvalidInputError("engram name", name, "name must contain valid characters")
	}

	return nil
}

// ValidateProjectPath checks if the project path is valid and safe
func ValidateProjectPath(path string) error {
	if path == "" {
		return cli.InvalidInputError("project path", path, "absolute or relative directory path")
	}

	// Resolve to absolute path
	absPath, err := filepath.Abs(path)
	if err != nil {
		return cli.InvalidInputError("project path", path, "valid filesystem path")
	}

	// Basic path validation
	cleanPath := filepath.Clean(absPath)
	if !filepath.IsAbs(cleanPath) {
		return cli.InvalidInputError("project path", path, "absolute directory path")
	}

	return nil
}

// ValidateTemplate checks if the selected template is valid
func ValidateTemplate(template string, availableTemplates []string) error {
	if template == "" {
		return cli.InvalidInputError("template", template, strings.Join(availableTemplates, "|"))
	}

	for _, validTemplate := range availableTemplates {
		if template == validTemplate {
			return nil
		}
	}

	return cli.InvalidInputError("template", template, strings.Join(availableTemplates, "|"))
}
