package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	// MaxFileSize is the maximum file size allowed (1MB)
	MaxFileSize = 1 * 1024 * 1024 // 1MB in bytes
)

// ResolveFile resolves @file syntax in a string value.
// If the value starts with '@', it reads the file at the path (substring after @).
// Otherwise, it returns the value unchanged.
//
// The path is resolved relative to the current working directory.
//
// Returns an error if:
//   - File not found
//   - File is too large (>1MB)
//   - File read error
//   - File encoding error (non-UTF-8)
func ResolveFile(value string) (string, error) {
	// Pass through non-@file values
	if !strings.HasPrefix(value, "@") {
		return value, nil
	}

	// Extract file path (remove @ prefix)
	filePath := strings.TrimPrefix(value, "@")
	if filePath == "" {
		return "", fmt.Errorf("empty file path after '@' prefix")
	}

	// Resolve path (cross-platform support)
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to resolve file path '%s': %w", filePath, err)
	}

	// Check if file exists
	fileInfo, err := os.Stat(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			// Helpful error message with path suggestion
			return "", fmt.Errorf("file not found: %s\nTry: ls -la to check file path", filePath)
		}
		return "", fmt.Errorf("failed to stat file '%s': %w", filePath, err)
	}

	// Check if it's a regular file (not a directory)
	if fileInfo.IsDir() {
		return "", fmt.Errorf("path is a directory, not a file: %s", filePath)
	}

	// Check file size (prevent accidental large files)
	if fileInfo.Size() > MaxFileSize {
		sizeMB := float64(fileInfo.Size()) / (1024 * 1024)
		return "", fmt.Errorf("file too large: %.2f MB (max: 1 MB)\nConsider using a smaller prompt or config file: %s", sizeMB, filePath)
	}

	// Read file content (UTF-8)
	content, err := os.ReadFile(absPath)
	if err != nil {
		return "", fmt.Errorf("failed to read file '%s': %w", filePath, err)
	}

	// Validate UTF-8 encoding (Go's ReadFile handles this, but we can check explicitly)
	// If the content contains invalid UTF-8, string conversion will replace with U+FFFD
	result := string(content)
	if strings.Contains(result, "\uFFFD") {
		return "", fmt.Errorf("file contains invalid UTF-8 encoding: %s\nEnsure the file is UTF-8 encoded", filePath)
	}

	return result, nil
}

// ResolveFiles resolves @file syntax in multiple prompt values.
// This is a convenience function for applying ResolveFile to multiple fields.
//
// Example:
//
//	prompts := map[string]string{
//	    "extract": "@extract.txt",
//	    "analyze": "Analyze {topics}",
//	    "research": "@research.txt",
//	}
//	resolved, err := ResolveFiles(prompts)
//
// Returns a new map with @file values replaced with file contents.
// Non-@file values are returned unchanged.
func ResolveFiles(prompts map[string]string) (map[string]string, error) {
	resolved := make(map[string]string, len(prompts))

	for key, value := range prompts {
		resolvedValue, err := ResolveFile(value)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve %s prompt: %w", key, err)
		}
		resolved[key] = resolvedValue
	}

	return resolved, nil
}
