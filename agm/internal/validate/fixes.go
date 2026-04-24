package validate

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/vbonnet/dear-agent/agm/internal/manifest"
)

// FixStrategy defines how to fix a specific issue type.
type FixStrategy struct {
	RequiresBackup bool
	Confirmation   string // Empty means no confirmation needed
	Apply          func(*manifest.Manifest, *Issue) error
}

// getClaudeVersion retrieves the current Claude Code version.
func getClaudeVersion() (string, error) {
	cmd := exec.Command("claude", "--version")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to get claude version: %w", err)
	}

	// Output format: "Claude Code CLI v2.1.2" (extract version number)
	versionStr := strings.TrimSpace(string(output))
	parts := strings.Fields(versionStr)
	if len(parts) < 2 {
		return "", fmt.Errorf("unexpected version output format: %s", versionStr)
	}

	// Remove 'v' prefix if present
	version := parts[len(parts)-1]
	version = strings.TrimPrefix(version, "v")
	return version, nil
}

// applyVersionFix updates the version in the session-env manifest.
func applyVersionFix(m *manifest.Manifest, issue *Issue) error {
	// Get current Claude version
	currentVersion, err := getClaudeVersion()
	if err != nil {
		return &FixError{
			Session:     m.Name,
			IssueType:   IssueVersionMismatch,
			Description: "failed to get current Claude version",
			Cause:       err,
		}
	}

	// CRITICAL SECURITY: Validate UUID format to prevent path traversal
	if err := validateUUID(m.Claude.UUID); err != nil {
		return &FixError{
			Session:     m.Name,
			IssueType:   IssueVersionMismatch,
			Description: "invalid UUID format in manifest",
			Cause:       err,
		}
	}

	// Build session-env manifest path (use cross-platform home dir)
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return &FixError{
			Session:     m.Name,
			IssueType:   IssueVersionMismatch,
			Description: "failed to get home directory",
			Cause:       err,
		}
	}

	sessionEnvPath := filepath.Join(
		homeDir,
		".claude/session-env",
		m.Claude.UUID,
		"manifest.yaml",
	)

	// Read manifest
	data, err := os.ReadFile(sessionEnvPath)
	if err != nil {
		return &FixError{
			Session:     m.Name,
			IssueType:   IssueVersionMismatch,
			Description: "failed to read session-env manifest",
			Cause:       err,
		}
	}

	// Replace version line
	newData := replaceVersionLine(string(data), currentVersion)

	// Write back (0600 for security - session data may contain sensitive info)
	err = os.WriteFile(sessionEnvPath, []byte(newData), 0600)
	if err != nil {
		return &FixError{
			Session:     m.Name,
			IssueType:   IssueVersionMismatch,
			Description: "failed to write session-env manifest",
			Cause:       err,
		}
	}

	return nil
}

// replaceVersionLine replaces the version field in YAML manifest content.
func replaceVersionLine(content, newVersion string) string {
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "version:") {
			// Preserve indentation
			indent := strings.TrimRight(line[:len(line)-len(strings.TrimLeft(line, " \t"))], "\r")
			lines[i] = indent + "version: " + newVersion
			break
		}
	}
	return strings.Join(lines, "\n")
}

// findJSONLPath locates the JSONL file for a given session UUID.
// UUID is validated to prevent path traversal attacks.
func findJSONLPath(uuid string) (string, error) {
	// CRITICAL SECURITY: Validate UUID format to prevent path traversal
	if err := validateUUID(uuid); err != nil {
		return "", fmt.Errorf("invalid UUID for JSONL path: %w", err)
	}

	// Use cross-platform home directory
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}

	claudeDir := filepath.Join(homeDir, ".claude")

	// JSONL file is at ~/.claude/<uuid>.jsonl (UUID validated above)
	jsonlPath := filepath.Join(claudeDir, uuid+".jsonl")
	if _, err := os.Stat(jsonlPath); err == nil {
		return jsonlPath, nil
	}

	return "", fmt.Errorf("JSONL file not found for UUID: %s", uuid)
}

// separateJSONLEntries separates summary entries from other entries.
func separateJSONLEntries(path string) (summaries []string, others []string, err error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to open JSONL: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	// Set larger buffer to handle long JSONL lines (10MB max line size)
	scanner.Buffer(make([]byte, 1024*1024), 10*1024*1024)

	for scanner.Scan() {
		line := scanner.Text()

		// Parse JSON to check type
		var entry map[string]interface{}
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			return nil, nil, fmt.Errorf("invalid JSON line: %w", err)
		}

		// Check if it's a summary entry
		if entryType, ok := entry["type"].(string); ok && entryType == "summary" {
			summaries = append(summaries, line)
		} else {
			others = append(others, line)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, nil, fmt.Errorf("error reading JSONL: %w", err)
	}

	return summaries, others, nil
}

// writeJSONL writes entries to a JSONL file (others first, then summaries).
// File permissions are 0600 for security (session data may contain sensitive info).
func writeJSONL(path string, others, summaries []string) error {
	// Write to temp file first for atomic operation
	tmpPath := path + ".tmp"
	file, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("failed to create JSONL temp file: %w", err)
	}

	// Ensure cleanup and close
	defer func() {
		file.Close()
		os.Remove(tmpPath) // Clean up temp file (ignore error if already renamed)
	}()

	// Write other entries first
	for _, line := range others {
		if _, err := file.WriteString(line + "\n"); err != nil {
			return fmt.Errorf("failed to write JSONL line: %w", err)
		}
	}

	// Write summaries at end
	for _, line := range summaries {
		if _, err := file.WriteString(line + "\n"); err != nil {
			return fmt.Errorf("failed to write JSONL summary: %w", err)
		}
	}

	// Sync to disk before closing
	if err := file.Sync(); err != nil {
		return fmt.Errorf("failed to sync JSONL file: %w", err)
	}

	// Close before rename
	if err := file.Close(); err != nil {
		return fmt.Errorf("failed to close JSONL file: %w", err)
	}

	// Atomic rename (on POSIX systems)
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("failed to rename JSONL file: %w", err)
	}

	return nil
}

// validateJSONL checks that all lines in the JSONL file are valid JSON.
func validateJSONL(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("failed to open JSONL for validation: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	// Set larger buffer to handle long JSONL lines (10MB max line size)
	scanner.Buffer(make([]byte, 1024*1024), 10*1024*1024)

	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		var entry map[string]interface{}
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			return fmt.Errorf("invalid JSON at line %d: %w", lineNum, err)
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error validating JSONL: %w", err)
	}

	return nil
}

// createBackup creates a backup of the JSONL file.
// Backup file permissions are 0600 for security (session data may contain sensitive info).
func createBackup(jsonlPath string) (string, error) {
	backupPath := jsonlPath + ".backup"

	// Read original
	data, err := os.ReadFile(jsonlPath)
	if err != nil {
		return "", fmt.Errorf("failed to read JSONL for backup: %w", err)
	}

	// Write backup with restrictive permissions
	err = os.WriteFile(backupPath, data, 0600)
	if err != nil {
		return "", fmt.Errorf("failed to create backup: %w", err)
	}

	return backupPath, nil
}

// restoreBackup restores the JSONL file from backup.
// Restored file permissions are 0600 for security.
func restoreBackup(jsonlPath, backupPath string) error {
	data, err := os.ReadFile(backupPath)
	if err != nil {
		return fmt.Errorf("failed to read backup: %w", err)
	}

	err = os.WriteFile(jsonlPath, data, 0600)
	if err != nil {
		return fmt.Errorf("failed to restore backup: %w", err)
	}

	return nil
}

// applyJSONLReorderFix reorders JSONL entries to move summaries to the end.
func applyJSONLReorderFix(m *manifest.Manifest, issue *Issue) error {
	// Find JSONL file
	jsonlPath, err := findJSONLPath(m.Claude.UUID)
	if err != nil {
		return &FixError{
			Session:     m.Name,
			IssueType:   IssueCompactedJSONL,
			Description: "failed to find JSONL file",
			Cause:       err,
		}
	}

	// Create backup
	backupPath, err := createBackup(jsonlPath)
	if err != nil {
		return &FixError{
			Session:     m.Name,
			IssueType:   IssueCompactedJSONL,
			Description: "failed to create backup",
			Cause:       err,
		}
	}

	// Separate entries
	summaries, others, err := separateJSONLEntries(jsonlPath)
	if err != nil {
		return &FixError{
			Session:     m.Name,
			IssueType:   IssueCompactedJSONL,
			Description: "failed to separate JSONL entries",
			Cause:       err,
		}
	}

	// Write with summaries at end
	err = writeJSONL(jsonlPath, others, summaries)
	if err != nil {
		// CRITICAL: Check restore error to prevent data loss
		if restoreErr := restoreBackup(jsonlPath, backupPath); restoreErr != nil {
			return &FixError{
				Session:     m.Name,
				IssueType:   IssueCompactedJSONL,
				Description: fmt.Sprintf("failed to write reordered JSONL AND restore failed: %v (restore error: %v)", err, restoreErr),
				Cause:       err,
			}
		}
		return &FixError{
			Session:     m.Name,
			IssueType:   IssueCompactedJSONL,
			Description: "failed to write reordered JSONL (backup restored)",
			Cause:       err,
		}
	}

	// Validate
	err = validateJSONL(jsonlPath)
	if err != nil {
		// CRITICAL: Check restore error to prevent data loss
		if restoreErr := restoreBackup(jsonlPath, backupPath); restoreErr != nil {
			return &FixError{
				Session:     m.Name,
				IssueType:   IssueCompactedJSONL,
				Description: fmt.Sprintf("JSONL validation failed AND restore failed: %v (restore error: %v)", err, restoreErr),
				Cause:       err,
			}
		}
		return &FixError{
			Session:     m.Name,
			IssueType:   IssueCompactedJSONL,
			Description: "JSONL validation failed after reorder (backup restored)",
			Cause:       err,
		}
	}

	return nil
}

// fixStrategies maps issue types to their fix strategies.
// Only auto-fixable issues are included here.
var fixStrategies = map[IssueType]*FixStrategy{
	IssueVersionMismatch: {
		RequiresBackup: false,
		Confirmation:   "",
		Apply:          applyVersionFix,
	},
	IssueCompactedJSONL: {
		RequiresBackup: true,
		Confirmation:   "Reorder JSONL to move summaries to end? Backup will be created.",
		Apply:          applyJSONLReorderFix,
	},
	// Note: Other issue types (empty_session_env, cwd_mismatch) would be added here
	// when their fix implementations are ready
}

// confirmFix prompts the user to confirm a fix operation.
// Returns true if user confirms, false otherwise.
func confirmFix(message string) bool {
	if message == "" {
		return true // No confirmation needed
	}

	fmt.Printf("%s [y/N]: ", message)
	var response string
	fmt.Scanln(&response)
	return strings.ToLower(response) == "y" || strings.ToLower(response) == "yes"
}

// ApplyFix applies the appropriate fix for an issue.
// Returns (fixed bool, error) where fixed indicates if the fix was applied successfully.
func ApplyFix(m *manifest.Manifest, issue *Issue) (bool, error) {
	// Check if this issue type has a fix strategy
	strategy, exists := fixStrategies[issue.Type]
	if !exists {
		return false, fmt.Errorf("no fix strategy for issue type: %s", issue.Type)
	}

	// Prompt for confirmation if required
	if strategy.Confirmation != "" {
		if !confirmFix(strategy.Confirmation) {
			return false, nil // User cancelled
		}
	}

	// Apply the fix
	err := strategy.Apply(m, issue)
	if err != nil {
		return false, err
	}

	return true, nil
}
