package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Security constants define limits for input validation to prevent attacks
const (
	// String Limits - Maximum lengths for various input types
	MaxQueryLength     = 1000  // Maximum query string length (prevents DoS)
	MaxContentLength   = 10000 // Maximum content length for memory store
	MaxNamespaceLength = 500   // Maximum total namespace length
	MaxComponentLength = 50    // Maximum length per namespace component
	MaxComponents      = 10    // Maximum number of namespace components
	MaxPathDepth       = 10    // Maximum directory depth

	// Allowed Directories (relative to home directory)
	AllowedEngramDir  = ".engram"
	AllowedClaudeDir  = ".claude"
	AllowedTempPrefix = "/tmp/engram-"
)

// Shell metacharacters that could enable command injection attacks
var shellMetacharacters = []string{
	";", "|", "&", "$", "`", "(", ")", "<", ">", "\n", "\r", "\x00",
	"&&", "||", ">>", "<<", "${", "$(", "``",
}

// Path Security Functions

// ValidateSafePath ensures a file path is within allowed directories and prevents path traversal attacks.
// It validates that the expanded, absolute, and cleaned path starts with one of the allowed prefixes.
//
// Security checks performed:
//   - Expands environment variables and tilde (~)
//   - Resolves to absolute path
//   - Cleans path to remove .. and . components
//   - Checks for null bytes (path traversal attack vector)
//   - Validates against allowlist of allowed directory prefixes
//
// Example:
//
//	home, _ := os.UserHomeDir()
//	allowedPaths := []string{filepath.Join(home, ".engram")}
//	err := ValidateSafePath("path", userInput, allowedPaths)
func ValidateSafePath(field, path string, allowedPrefixes []string) error {
	if path == "" {
		return nil // Empty paths are often valid for optional fields
	}

	// Check for null bytes (common path traversal attack)
	if strings.Contains(path, "\x00") {
		return &EngramError{
			Symbol:  "✗",
			Message: fmt.Sprintf("Invalid %s: contains null bytes", field),
			Suggestions: []string{
				"Remove null bytes from path",
				"Use standard filesystem paths only",
			},
		}
	}

	// Expand environment variables and tilde
	expanded := expandPath(path)

	// Get absolute path (follows symlinks on most systems)
	absPath, err := filepath.Abs(expanded)
	if err != nil {
		return &EngramError{
			Symbol:  "✗",
			Message: fmt.Sprintf("Invalid %s: cannot resolve absolute path", field),
			Cause:   err,
			Suggestions: []string{
				"Verify path is valid for your filesystem",
				"Check path syntax",
			},
		}
	}

	// Clean path (removes .., ., and redundant separators)
	cleanPath := filepath.Clean(absPath)

	// Check against allowlist of prefixes
	allowed := false
	for _, prefix := range allowedPrefixes {
		// Ensure prefix is also cleaned and absolute
		cleanPrefix := filepath.Clean(prefix)
		absPrefix, err := filepath.Abs(cleanPrefix)
		if err != nil {
			continue // Skip invalid prefix
		}

		if strings.HasPrefix(cleanPath, absPrefix) {
			allowed = true
			break
		}
	}

	if !allowed {
		return &EngramError{
			Symbol:  "✗",
			Message: fmt.Sprintf("Path not allowed: %s", path),
			Suggestions: []string{
				"Path must be within allowed directories",
				fmt.Sprintf("Allowed directories: %s", strings.Join(allowedPrefixes, ", ")),
				"Check for path traversal attempts (..)",
			},
		}
	}

	return nil
}

// ValidateNoTraversal checks if a path contains directory traversal patterns (..).
// This is a simpler check than ValidateSafePath for cases where you want to reject
// paths with .. regardless of where they resolve to.
func ValidateNoTraversal(field, path string) error {
	if path == "" {
		return nil
	}

	// Check for .. in path components
	if strings.Contains(path, "..") {
		return &EngramError{
			Symbol:  "✗",
			Message: fmt.Sprintf("Path traversal detected in %s: %s", field, path),
			Suggestions: []string{
				"Remove '..' from path",
				"Use absolute paths or paths relative to allowed directories",
			},
		}
	}

	return nil
}

// ValidateFileExtension validates that a file has one of the allowed extensions.
// This is a defense-in-depth measure to prevent reading/writing unexpected file types.
func ValidateFileExtension(field, path string, allowedExts []string) error {
	if path == "" {
		return nil
	}

	ext := strings.ToLower(filepath.Ext(path))
	for _, allowed := range allowedExts {
		if ext == strings.ToLower(allowed) {
			return nil
		}
	}

	return &EngramError{
		Symbol:  "✗",
		Message: fmt.Sprintf("Invalid file extension for %s: %s", field, path),
		Suggestions: []string{
			fmt.Sprintf("Allowed extensions: %s", strings.Join(allowedExts, ", ")),
			"Check file type is supported",
		},
	}
}

// ValidateRelativePathSafe validates a relative path and ensures it stays within basePath.
// This is useful when accepting user-provided subdirectories or filenames.
func ValidateRelativePathSafe(field, relPath, basePath string) error {
	if relPath == "" {
		return nil
	}

	// Check for path traversal
	if err := ValidateNoTraversal(field, relPath); err != nil {
		return err
	}

	// Ensure it's actually relative
	if filepath.IsAbs(relPath) {
		return &EngramError{
			Symbol:  "✗",
			Message: fmt.Sprintf("Expected relative path for %s, got absolute: %s", field, relPath),
			Suggestions: []string{
				"Use relative paths only",
				fmt.Sprintf("Remove leading separator from path"),
			},
		}
	}

	// Validate the joined path is safe
	fullPath := filepath.Join(basePath, relPath)
	return ValidateSafePath(field, fullPath, []string{basePath})
}

// String Security Functions

// ValidateMaxLength validates that a string doesn't exceed maximum length.
// This prevents denial-of-service attacks via memory exhaustion.
func ValidateMaxLength(field, value string, maxLen int) error {
	if len(value) > maxLen {
		return &EngramError{
			Symbol:  "✗",
			Message: fmt.Sprintf("Invalid %s: exceeds maximum length", field),
			Suggestions: []string{
				fmt.Sprintf("Maximum length: %d characters", maxLen),
				fmt.Sprintf("Current length: %d characters", len(value)),
				"Shorten the input",
			},
		}
	}
	return nil
}

// ValidateNoShellMetacharacters rejects strings containing shell metacharacters.
// This prevents command injection attacks when strings might be passed to shell commands.
//
// Blocked characters: ; | & $ ` ( ) < > newlines and null bytes
func ValidateNoShellMetacharacters(field, value string) error {
	if value == "" {
		return nil
	}

	for _, char := range shellMetacharacters {
		if strings.Contains(value, char) {
			return &EngramError{
				Symbol:  "✗",
				Message: fmt.Sprintf("Invalid %s: contains shell metacharacter", field),
				Suggestions: []string{
					"Remove special characters: ; | & $ ` ( ) < >",
					"Use alphanumeric characters only",
				},
			}
		}
	}

	return nil
}

// ValidateAlphanumeric validates that a string contains only alphanumeric characters.
// If allowHyphens is true, hyphens and underscores are also allowed (common for IDs/names).
func ValidateAlphanumeric(field, value string, allowHyphens bool) error {
	if value == "" {
		return nil
	}

	for _, r := range value {
		isValid := (r >= 'a' && r <= 'z') ||
			(r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9')

		if allowHyphens {
			isValid = isValid || r == '-' || r == '_'
		}

		if !isValid {
			suggestion := "Use letters and numbers only"
			if allowHyphens {
				suggestion = "Use letters, numbers, hyphens, and underscores only"
			}
			return &EngramError{
				Symbol:  "✗",
				Message: fmt.Sprintf("Invalid %s: contains invalid characters", field),
				Suggestions: []string{
					suggestion,
					fmt.Sprintf("Found invalid character: %c", r),
				},
			}
		}
	}

	return nil
}

// Environment Security Functions

// ValidateSafeEnvExpansion expands environment variables and then validates the result.
// This prevents environment variable injection attacks where malicious env vars
// could point to unauthorized paths.
func ValidateSafeEnvExpansion(field, value string, allowedPrefixes []string) error {
	if value == "" {
		return nil
	}

	expanded := os.ExpandEnv(value)
	return ValidateSafePath(field, expanded, allowedPrefixes)
}

// GetSafeEnvVar retrieves an environment variable and validates it against allowed prefixes.
// If the variable is not set, it returns the default value (which is also validated).
//
// Example:
//
//	home, _ := os.UserHomeDir()
//	allowedPaths := []string{filepath.Join(home, ".engram")}
//	configPath, err := GetSafeEnvVar("ENGRAM_CONFIG", "~/.engram/config.yaml", allowedPaths)
func GetSafeEnvVar(name, defaultValue string, allowedPrefixes []string) (string, error) {
	value := os.Getenv(name)
	if value == "" {
		value = defaultValue
	}

	// Expand and validate
	expanded := expandPath(value)
	if err := ValidateSafePath(name, expanded, allowedPrefixes); err != nil {
		return "", err
	}

	return expanded, nil
}

// Namespace Security Functions

// ValidateNamespaceComponents validates namespace structure and length limits.
// Namespaces are comma-separated strings, each component has length limits.
//
// Security checks:
//   - Total components <= maxComponents (prevents resource exhaustion)
//   - Each component <= maxCompLen (prevents DoS)
//   - No empty components (prevents parsing issues)
func ValidateNamespaceComponents(namespace string, maxComponents, maxCompLen int) error {
	if namespace == "" {
		return fmt.Errorf("namespace cannot be empty")
	}

	// Check total length first (quick rejection)
	if len(namespace) > MaxNamespaceLength {
		return &EngramError{
			Symbol:  "✗",
			Message: "Namespace exceeds maximum total length",
			Suggestions: []string{
				fmt.Sprintf("Maximum total length: %d characters", MaxNamespaceLength),
				fmt.Sprintf("Current length: %d characters", len(namespace)),
				"Use shorter component names",
			},
		}
	}

	parts := strings.Split(namespace, ",")

	// Check component count
	if len(parts) > maxComponents {
		return &EngramError{
			Symbol:  "✗",
			Message: "Namespace has too many components",
			Suggestions: []string{
				fmt.Sprintf("Maximum components: %d", maxComponents),
				fmt.Sprintf("Current components: %d", len(parts)),
				"Reduce namespace depth",
			},
		}
	}

	// Check each component
	for i, part := range parts {
		trimmed := strings.TrimSpace(part)

		if trimmed == "" {
			return &EngramError{
				Symbol:  "✗",
				Message: "Invalid namespace: contains empty parts",
				Suggestions: []string{
					"Remove empty parts from comma-separated namespace",
					"Example: user,alice,project",
				},
			}
		}

		if len(trimmed) > maxCompLen {
			return &EngramError{
				Symbol:  "✗",
				Message: fmt.Sprintf("Namespace component %d exceeds maximum length", i+1),
				Suggestions: []string{
					fmt.Sprintf("Maximum component length: %d characters", maxCompLen),
					fmt.Sprintf("Component '%s' is %d characters", trimmed, len(trimmed)),
					"Use shorter component names",
				},
			}
		}
	}

	return nil
}

// Query Security Functions

// ValidateQuery validates a search query string with length limits.
// This prevents DoS attacks via extremely long query strings.
func ValidateQuery(query string, maxLen int) error {
	if err := ValidateNonEmpty("query", query); err != nil {
		return err
	}

	if err := ValidateMaxLength("query", query, maxLen); err != nil {
		return err
	}

	return nil
}

// Helper Functions

// expandPath expands environment variables and tilde in a path string.
// This centralizes path expansion logic for consistent handling.
func expandPath(path string) string {
	if path == "" {
		return path
	}

	// Expand environment variables first
	expanded := os.ExpandEnv(path)

	// Handle tilde expansion
	if strings.HasPrefix(expanded, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			expanded = filepath.Join(home, expanded[2:])
		}
	} else if expanded == "~" {
		home, err := os.UserHomeDir()
		if err == nil {
			expanded = home
		}
	}

	return expanded
}

// GetSafeHomePath constructs a safe path within the user's home directory.
// This is a convenience function for common cases where paths should be
// restricted to subdirectories of home.
//
// Example:
//
//	engramPath, err := GetSafeHomePath(".engram", "memories")
//	// Returns: ~/.engram/memories (validated)
func GetSafeHomePath(subpaths ...string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", &EngramError{
			Symbol:  "✗",
			Message: "Cannot determine home directory",
			Cause:   err,
			Suggestions: []string{
				"Check HOME environment variable is set",
				"Verify user account is properly configured",
			},
		}
	}

	// Join all subpaths
	fullPath := home
	for _, subpath := range subpaths {
		fullPath = filepath.Join(fullPath, subpath)
	}

	// Validate the result is actually within home
	if err := ValidateSafePath("path", fullPath, []string{home}); err != nil {
		return "", err
	}

	return fullPath, nil
}

// GetAllowedPaths returns common allowed path prefixes for engram operations.
// This provides consistent security boundaries across the application.
func GetAllowedPaths() ([]string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	return []string{
		filepath.Join(home, AllowedEngramDir),
		filepath.Join(home, AllowedClaudeDir),
	}, nil
}
