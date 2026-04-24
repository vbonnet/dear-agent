// Package history provides history functionality.
package history

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// HistoryLocation represents conversation history file paths for a session
type HistoryLocation struct {
	SessionName string            `json:"session_name,omitempty"`
	SessionID   string            `json:"session_id,omitempty"`
	Harness     string            `json:"harness"`
	UUID        string            `json:"uuid"`
	Paths       []string          `json:"paths"`
	Exists      bool              `json:"exists"`
	Metadata    map[string]string `json:"metadata,omitempty"`
	Error       *LocationError    `json:"error,omitempty"`
}

// LocationError represents an error during path discovery
type LocationError struct {
	Code       string `json:"code"`
	Message    string `json:"message"`
	Suggestion string `json:"suggestion,omitempty"`
}

// Error implements the error interface
func (e *LocationError) Error() string {
	if e.Suggestion != "" {
		return fmt.Sprintf("%s: %s (suggestion: %s)", e.Code, e.Message, e.Suggestion)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// GetHistoryPaths returns conversation history file paths for a session
func GetHistoryPaths(harness, uuid, workingDir string, verify bool) (*HistoryLocation, error) {
	if harness == "" {
		return nil, &LocationError{
			Code:       "HARNESS_UNKNOWN",
			Message:    "Harness type not specified",
			Suggestion: "Check session metadata in AGM database",
		}
	}

	if uuid == "" {
		return nil, &LocationError{
			Code:       "UUID_MISSING",
			Message:    "Session UUID not provided",
			Suggestion: "Run 'agm session get-uuid' to discover UUID",
		}
	}

	var paths []string
	var metadata map[string]string
	var err error

	switch strings.ToLower(harness) {
	case "claude-code", "claude":
		paths, metadata, err = getClaudeCodePaths(uuid, workingDir)
	case "gemini-cli", "gemini":
		paths, metadata, err = getGeminiCLIPaths(uuid, workingDir)
	case "opencode-cli", "opencode":
		paths, metadata, err = getOpenCodePaths(uuid)
	case "codex-cli", "codex", "openai":
		paths, metadata, err = getCodexPaths(uuid)
	default:
		return nil, &LocationError{
			Code:       "HARNESS_UNKNOWN",
			Message:    fmt.Sprintf("Unknown harness type: %s", harness),
			Suggestion: "Supported harnesses: claude-code, gemini-cli, opencode-cli, codex-cli",
		}
	}

	if err != nil {
		return nil, err
	}

	exists := true
	if verify {
		exists = verifyPathsExist(paths)
	}

	return &HistoryLocation{
		Harness:  harness,
		UUID:     uuid,
		Paths:    paths,
		Exists:   exists,
		Metadata: metadata,
	}, nil
}

// getClaudeCodePaths returns paths for Claude Code harness
func getClaudeCodePaths(uuid, workingDir string) ([]string, map[string]string, error) {
	if workingDir == "" {
		return nil, nil, &LocationError{
			Code:       "WORKING_DIR_MISSING",
			Message:    "Working directory required for Claude Code paths",
			Suggestion: "Ensure session has working directory in metadata",
		}
	}

	// Encode working directory using dash-substitution
	encoded := EncodeDashSubstitution(workingDir)

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	projectDir := filepath.Join(homeDir, ".claude", "projects", encoded)

	paths := []string{
		filepath.Join(projectDir, uuid+".jsonl"),
		filepath.Join(projectDir, "sessions-index.json"),
	}

	metadata := map[string]string{
		"harness":           "claude",
		"encoding_method":   "dash-substitution",
		"working_directory": workingDir,
		"encoded_directory": encoded,
	}

	return paths, metadata, nil
}

// getGeminiCLIPaths returns paths for Gemini CLI harness
func getGeminiCLIPaths(uuid, workingDir string) ([]string, map[string]string, error) {
	if workingDir == "" {
		return nil, nil, &LocationError{
			Code:       "WORKING_DIR_MISSING",
			Message:    "Working directory required for Gemini CLI paths",
			Suggestion: "Ensure session has working directory in metadata",
		}
	}

	// Hash working directory
	hash := HashDirectory(workingDir)

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	tmpDir := filepath.Join(homeDir, ".gemini", "tmp", hash)

	paths := []string{
		filepath.Join(tmpDir, "chats") + string(filepath.Separator), // Directory
		filepath.Join(tmpDir, "logs.json"),
	}

	metadata := map[string]string{
		"harness":           "gemini",
		"hash_method":       "sha256-8char",
		"working_directory": workingDir,
		"project_hash":      hash,
	}

	return paths, metadata, nil
}

// getOpenCodePaths returns paths for OpenCode harness
func getOpenCodePaths(uuid string) ([]string, map[string]string, error) {
	// Check for custom data directory
	baseDir := os.Getenv("OPENCODE_DATA_DIR")
	if baseDir == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return nil, nil, fmt.Errorf("failed to get home directory: %w", err)
		}
		baseDir = filepath.Join(homeDir, ".local", "share", "opencode")
	}

	storageDir := filepath.Join(baseDir, "storage")

	// OpenCode uses session ID pattern like "ses_abc123"
	// We'll return both possible locations
	paths := []string{
		filepath.Join(storageDir, "message", uuid) + string(filepath.Separator), // Messages directory
		filepath.Join(storageDir, "session") + string(filepath.Separator),       // Session directory (pattern)
	}

	metadata := map[string]string{
		"harness":  "opencode",
		"base_dir": baseDir,
	}

	if baseDir != os.Getenv("OPENCODE_DATA_DIR") {
		metadata["env_override"] = "false"
	} else {
		metadata["env_override"] = "true"
	}

	return paths, metadata, nil
}

// getCodexPaths returns paths for Codex harness
func getCodexPaths(uuid string) ([]string, map[string]string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	// Try to extract date from UUID or use current date
	year, month, day, err := ExtractDateFromUUID(uuid)
	if err != nil {
		// Fallback: use current date
		now := time.Now()
		year, month, day = now.Year(), int(now.Month()), now.Day()
	}

	sessionsDir := filepath.Join(homeDir, ".codex", "sessions",
		fmt.Sprintf("%04d", year),
		fmt.Sprintf("%02d", month),
		fmt.Sprintf("%02d", day))

	paths := []string{
		filepath.Join(sessionsDir, "rollout-*.jsonl"), // Pattern
	}

	metadata := map[string]string{
		"harness": "codex",
		"year":    fmt.Sprintf("%04d", year),
		"month":   fmt.Sprintf("%02d", month),
		"day":     fmt.Sprintf("%02d", day),
	}

	return paths, metadata, nil
}

// EncodeDashSubstitution encodes a path using Claude Code's dash-substitution algorithm
// Replaces all non-alphanumeric characters with a single dash
func EncodeDashSubstitution(path string) string {
	// Replace all non-alphanumeric characters with dash
	reg := regexp.MustCompile(`[^a-zA-Z0-9]`)
	return reg.ReplaceAllString(path, "-")
}

// HashDirectory creates an 8-character hash of a directory path
// Uses first 8 characters of SHA256 hash
func HashDirectory(path string) string {
	hash := sha256.Sum256([]byte(path))
	return fmt.Sprintf("%x", hash[:4]) // First 4 bytes = 8 hex chars
}

// ExtractDateFromUUID attempts to extract date components from a UUID
// Returns year, month, day, or error if extraction fails
func ExtractDateFromUUID(uuid string) (year, month, day int, err error) {
	// Try to parse date from UUID if it contains a date pattern
	// Pattern: YYYY-MM-DD or similar
	datePattern := regexp.MustCompile(`(\d{4})-(\d{2})-(\d{2})`)
	matches := datePattern.FindStringSubmatch(uuid)

	if len(matches) >= 4 {
		fmt.Sscanf(matches[1], "%d", &year)
		fmt.Sscanf(matches[2], "%d", &month)
		fmt.Sscanf(matches[3], "%d", &day)
		return year, month, day, nil
	}

	return 0, 0, 0, fmt.Errorf("could not extract date from UUID: %s", uuid)
}

// verifyPathsExist checks if all paths exist on the filesystem
func verifyPathsExist(paths []string) bool {
	for _, path := range paths {
		// Handle wildcard patterns
		if strings.Contains(path, "*") {
			matches, err := filepath.Glob(path)
			if err != nil || len(matches) == 0 {
				return false
			}
			continue
		}

		// Check if path exists
		if _, err := os.Stat(path); os.IsNotExist(err) {
			return false
		}
	}
	return true
}
