package validator

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const ValidatorVersion = "1.0.0"

// AddSignature adds validation signature to file frontmatter
// Signature includes:
// - validated: true
// - validated_at: timestamp
// - validator_version: version string
// - checksum: SHA256 of file content (excluding frontmatter)
func AddSignature(filePath string) error {
	// Read file
	content, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	contentStr := string(content)

	// Extract existing frontmatter and body
	frontmatter, body, err := splitFrontmatterAndBody(contentStr)
	if err != nil {
		return fmt.Errorf("failed to parse frontmatter: %w", err)
	}

	// Parse existing frontmatter
	var fm map[string]interface{}
	if err := yaml.Unmarshal([]byte(frontmatter), &fm); err != nil {
		return fmt.Errorf("failed to parse YAML frontmatter: %w", err)
	}

	// Calculate checksum of body content (excluding frontmatter)
	checksum := calculateChecksum(body)

	// Add signature fields
	fm["validated"] = true
	fm["validated_at"] = time.Now().Format(time.RFC3339)
	fm["validator_version"] = ValidatorVersion
	fm["checksum"] = checksum

	// Marshal back to YAML
	updatedFrontmatter, err := yaml.Marshal(fm)
	if err != nil {
		return fmt.Errorf("failed to marshal YAML: %w", err)
	}

	// Rebuild file content
	newContent := fmt.Sprintf("---\n%s---\n%s", string(updatedFrontmatter), body)

	// Write back to file
	if err := os.WriteFile(filePath, []byte(newContent), 0o600); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}

// HasSignature checks if file has a valid validation signature
// Returns (hasSignature, error)
func HasSignature(filePath string) (bool, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return false, fmt.Errorf("failed to read file: %w", err)
	}

	contentStr := string(content)

	// Extract frontmatter
	frontmatter, _, err := splitFrontmatterAndBody(contentStr)
	if err != nil {
		return false, nil // No frontmatter = no signature
	}

	// Parse frontmatter
	var fm map[string]interface{}
	if err := yaml.Unmarshal([]byte(frontmatter), &fm); err != nil {
		return false, nil // Invalid YAML = no signature
	}

	// Check for validated field
	validated, ok := fm["validated"].(bool)
	if !ok || !validated {
		return false, nil
	}

	// Check for required signature fields
	if _, ok := fm["validated_at"]; !ok {
		return false, nil
	}
	if _, ok := fm["validator_version"]; !ok {
		return false, nil
	}
	if _, ok := fm["checksum"]; !ok {
		return false, nil
	}

	return true, nil
}

// RemoveSignature removes validation signature from file frontmatter
// Used for testing and resetting validation state
func RemoveSignature(filePath string) error {
	// Read file
	content, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	contentStr := string(content)

	// Extract existing frontmatter and body
	frontmatter, body, err := splitFrontmatterAndBody(contentStr)
	if err != nil {
		return fmt.Errorf("failed to parse frontmatter: %w", err)
	}

	// Parse existing frontmatter
	var fm map[string]interface{}
	if err := yaml.Unmarshal([]byte(frontmatter), &fm); err != nil {
		return fmt.Errorf("failed to parse YAML frontmatter: %w", err)
	}

	// Remove signature fields
	delete(fm, "validated")
	delete(fm, "validated_at")
	delete(fm, "validator_version")
	delete(fm, "checksum")

	// Marshal back to YAML
	updatedFrontmatter, err := yaml.Marshal(fm)
	if err != nil {
		return fmt.Errorf("failed to marshal YAML: %w", err)
	}

	// Rebuild file content
	newContent := fmt.Sprintf("---\n%s---\n%s", string(updatedFrontmatter), body)

	// Write back to file
	if err := os.WriteFile(filePath, []byte(newContent), 0o600); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}

// ValidateChecksum verifies that the file's checksum matches the signature
// Returns (valid, error)
func ValidateChecksum(filePath string) (bool, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return false, fmt.Errorf("failed to read file: %w", err)
	}

	contentStr := string(content)

	// Extract frontmatter and body
	frontmatter, body, err := splitFrontmatterAndBody(contentStr)
	if err != nil {
		return false, fmt.Errorf("failed to parse frontmatter: %w", err)
	}

	// Parse frontmatter
	var fm map[string]interface{}
	if err := yaml.Unmarshal([]byte(frontmatter), &fm); err != nil {
		return false, fmt.Errorf("failed to parse YAML frontmatter: %w", err)
	}

	// Get stored checksum
	storedChecksum, ok := fm["checksum"].(string)
	if !ok {
		return false, nil // No checksum = not validated
	}

	// Calculate current checksum
	currentChecksum := calculateChecksum(body)

	// Compare
	return storedChecksum == currentChecksum, nil
}

// splitFrontmatterAndBody splits file content into frontmatter and body
// Returns (frontmatter, body, error)
func splitFrontmatterAndBody(content string) (string, string, error) {
	if !strings.HasPrefix(content, "---\n") {
		return "", "", fmt.Errorf("file does not start with YAML frontmatter delimiter (---)")
	}

	// Find closing delimiter
	lines := strings.Split(content, "\n")
	closingIdx := -1
	for i := 1; i < len(lines); i++ {
		if lines[i] == "---" {
			closingIdx = i
			break
		}
	}

	if closingIdx == -1 {
		return "", "", fmt.Errorf("no closing frontmatter delimiter (---) found")
	}

	// Extract frontmatter (between delimiters, excluding ---)
	frontmatter := strings.Join(lines[1:closingIdx], "\n")

	// Extract body (everything after closing ---)
	body := strings.Join(lines[closingIdx+1:], "\n")

	return frontmatter, body, nil
}

// calculateChecksum computes SHA256 hash of content
func calculateChecksum(content string) string {
	hash := sha256.Sum256([]byte(content))
	return "sha256-" + hex.EncodeToString(hash[:])
}
