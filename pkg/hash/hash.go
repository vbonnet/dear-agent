package hash

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// ExpandPath expands tilde (~) to user's home directory and returns an absolute path.
//
// Supports:
//   - ~ (tilde only) - expands to home directory
//   - ~/path (tilde with path) - expands to home directory + path
//   - /absolute/path - returns absolute path
//   - relative/path - converts to absolute path
//
// Does not support:
//   - ~user/path - returns error (user-specific home directories not supported)
//
// Example:
//
//	path, err := hash.ExpandPath("~/Documents/file.txt")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	fmt.Println(path) // /home/username/Documents/file.txt
func ExpandPath(path string) (string, error) {
	if !strings.HasPrefix(path, "~") {
		return filepath.Abs(path)
	}

	// Get home directory
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}

	// Replace ~ with home directory
	if path == "~" {
		return home, nil
	}
	if strings.HasPrefix(path, "~/") {
		return filepath.Join(home, path[2:]), nil
	}

	// Path like ~user/... is not supported
	return "", fmt.Errorf("cannot expand path: %s (only ~ and ~/ are supported)", path)
}

// CalculateFileHash calculates SHA-256 hash of a file and returns it in the format "sha256:{hex_hash}".
//
// The function:
//   - Expands tilde (~) in paths to the user's home directory
//   - Opens and reads the file
//   - Computes SHA-256 hash
//   - Returns hash in format: sha256:0123456789abcdef...
//
// Returns error if:
//   - Path expansion fails (unsupported tilde format)
//   - File cannot be opened (not found, permissions, etc.)
//   - File cannot be read
//
// Example:
//
//	hash, err := hash.CalculateFileHash("~/myfile.txt")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	fmt.Println(hash) // sha256:a665a45920422f9d417e4867efdc4fb8a04a1f3fff1fa07e998e86f7f7a27ae3
func CalculateFileHash(path string) (string, error) {
	// Expand path (handle ~)
	absPath, err := ExpandPath(path)
	if err != nil {
		return "", fmt.Errorf("failed to expand path: %w", err)
	}

	// Open file
	file, err := os.Open(absPath)
	if err != nil {
		return "", fmt.Errorf("failed to open file %s: %w", absPath, err)
	}
	defer file.Close()

	// Calculate SHA-256
	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return "", fmt.Errorf("failed to hash file: %w", err)
	}

	// Format as sha256:{hex}
	hash := fmt.Sprintf("sha256:%x", hasher.Sum(nil))
	return hash, nil
}
