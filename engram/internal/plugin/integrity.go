package plugin

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// VerifyIntegrity verifies file integrity for a plugin
// Returns nil if integrity checks pass, error if tampering detected
func VerifyIntegrity(pluginDir string, integrity Integrity) error {
	// If no integrity section, skip verification
	// This is for backwards compatibility with legacy plugins
	if len(integrity.Files) == 0 {
		return nil
	}

	// Validate algorithm
	if integrity.Algorithm != "sha256" {
		return fmt.Errorf("unsupported hash algorithm: %s (only sha256 is supported)", integrity.Algorithm)
	}

	// Verify each file hash
	for relPath, expectedHash := range integrity.Files {
		filePath := filepath.Join(pluginDir, relPath)

		// Check file exists
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			return fmt.Errorf("integrity verification failed: file %q not found", relPath)
		}

		// Compute actual hash
		actualHash, err := computeSHA256(filePath)
		if err != nil {
			return fmt.Errorf("integrity verification failed for %q: %w", relPath, err)
		}

		// Compare hashes
		if actualHash != expectedHash {
			return fmt.Errorf("integrity verification failed: %q has been modified (expected %s, got %s)", relPath, expectedHash[:16]+"...", actualHash[:16]+"...")
		}
	}

	return nil
}

// computeSHA256 computes SHA-256 hash of a file
func computeSHA256(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}

// GenerateIntegrity generates integrity hashes for plugin files
// This is a helper for plugin developers to create the integrity section
func GenerateIntegrity(pluginDir string, files []string) (*Integrity, error) {
	integrity := &Integrity{
		Algorithm: "sha256",
		Files:     make(map[string]string),
	}

	for _, relPath := range files {
		filePath := filepath.Join(pluginDir, relPath)

		// Check file exists
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			return nil, fmt.Errorf("file %q not found", relPath)
		}

		// Compute hash
		hash, err := computeSHA256(filePath)
		if err != nil {
			return nil, fmt.Errorf("failed to hash %q: %w", relPath, err)
		}

		integrity.Files[relPath] = hash
	}

	return integrity, nil
}
