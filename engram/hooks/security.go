package hooks

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/pelletier/go-toml/v2"
)

var (
	// ErrCommandNotAllowed is returned when a command is not in the allowlist
	ErrCommandNotAllowed = errors.New("command not in allowlist")
	// ErrHashMismatch is returned when command hash verification fails
	ErrHashMismatch = errors.New("command hash mismatch")
	// ErrCommandNotFound is returned when a command binary cannot be found
	ErrCommandNotFound = errors.New("command not found")
)

// Default allowed commands (safe for subprocess execution)
var defaultAllowedCommands = map[string]bool{
	"git":       true,
	"npm":       true,
	"pytest":    true,
	"go":        true,
	"cargo":     true,
	"bd":        true,
	"wayfinder": true,
	"engram":    true,
	"bow-core":  true,
	"agm":       true,
}

// CommandValidator manages command allowlist and hash verification
type CommandValidator struct {
	mu              sync.RWMutex
	allowedCommands map[string]bool
	hashCache       map[string]string // Command -> hash cache
	allowlistPath   string
}

// allowedCommandsFile represents the TOML structure for allowed-commands.toml
type allowedCommandsFile struct {
	AllowedCommands []string `toml:"allowed_commands"`
}

// NewCommandValidator creates a new command validator with default allowlist
func NewCommandValidator() *CommandValidator {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		homeDir = "."
	}
	path := filepath.Join(homeDir, ".engram", "allowed-commands.toml")

	cv := &CommandValidator{
		allowedCommands: make(map[string]bool),
		hashCache:       make(map[string]string),
		allowlistPath:   path,
	}

	// Load default allowlist
	for cmd := range defaultAllowedCommands {
		cv.allowedCommands[cmd] = true
	}

	// Try to load user allowlist (optional, gracefully handle missing file)
	_ = cv.LoadAllowlist()

	return cv
}

// NewCommandValidatorWithPath creates a validator with a custom allowlist path
func NewCommandValidatorWithPath(path string) *CommandValidator {
	cv := &CommandValidator{
		allowedCommands: make(map[string]bool),
		hashCache:       make(map[string]string),
		allowlistPath:   path,
	}

	// Load default allowlist
	for cmd := range defaultAllowedCommands {
		cv.allowedCommands[cmd] = true
	}

	// Try to load user allowlist
	_ = cv.LoadAllowlist()

	return cv
}

// LoadAllowlist loads the user's custom allowlist from TOML
func (cv *CommandValidator) LoadAllowlist() error {
	cv.mu.Lock()
	defer cv.mu.Unlock()

	// Check if file exists
	if _, err := os.Stat(cv.allowlistPath); os.IsNotExist(err) {
		// File doesn't exist, use defaults only
		return nil
	}

	data, err := os.ReadFile(cv.allowlistPath)
	if err != nil {
		return fmt.Errorf("failed to read allowlist file: %w", err)
	}

	var acf allowedCommandsFile
	if err := toml.Unmarshal(data, &acf); err != nil {
		return fmt.Errorf("failed to parse allowlist file: %w", err)
	}

	// Add user-defined commands to allowlist (merged with defaults)
	for _, cmd := range acf.AllowedCommands {
		cv.allowedCommands[cmd] = true
	}

	return nil
}

// SaveAllowlist saves the current allowlist to TOML
func (cv *CommandValidator) SaveAllowlist() error {
	cv.mu.RLock()
	defer cv.mu.RUnlock()

	// Create directory if it doesn't exist
	dir := filepath.Dir(cv.allowlistPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create allowlist directory: %w", err)
	}

	// Convert map to slice
	commands := make([]string, 0, len(cv.allowedCommands))
	for cmd := range cv.allowedCommands {
		commands = append(commands, cmd)
	}

	acf := allowedCommandsFile{AllowedCommands: commands}

	data, err := toml.Marshal(acf)
	if err != nil {
		return fmt.Errorf("failed to marshal allowlist: %w", err)
	}

	// Atomic write
	tempPath := cv.allowlistPath + ".tmp"
	if err := os.WriteFile(tempPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write temp file: %w", err)
	}

	if err := os.Rename(tempPath, cv.allowlistPath); err != nil {
		os.Remove(tempPath)
		return fmt.Errorf("failed to rename temp file: %w", err)
	}

	return nil
}

// AddCommand adds a command to the allowlist
func (cv *CommandValidator) AddCommand(command string) {
	cv.mu.Lock()
	defer cv.mu.Unlock()
	cv.allowedCommands[command] = true
}

// RemoveCommand removes a command from the allowlist
func (cv *CommandValidator) RemoveCommand(command string) {
	cv.mu.Lock()
	defer cv.mu.Unlock()
	delete(cv.allowedCommands, command)
}

// IsAllowed checks if a command is in the allowlist
func (cv *CommandValidator) IsAllowed(command string) bool {
	cv.mu.RLock()
	defer cv.mu.RUnlock()
	return cv.allowedCommands[command]
}

// ValidateCommand validates that a command is in the allowlist
func (cv *CommandValidator) ValidateCommand(command string) error {
	// Extract base command (handle paths)
	baseCmd := filepath.Base(command)

	// Also check without any path or extension
	baseCmd = strings.TrimSuffix(baseCmd, filepath.Ext(baseCmd))

	if !cv.IsAllowed(baseCmd) && !cv.IsAllowed(command) {
		return fmt.Errorf("%w: %s (not in allowlist)", ErrCommandNotAllowed, command)
	}

	return nil
}

// CalculateCommandHash calculates the SHA-256 hash of a command binary
func (cv *CommandValidator) CalculateCommandHash(command string) (string, error) {
	// Check cache first
	cv.mu.RLock()
	if hash, ok := cv.hashCache[command]; ok {
		cv.mu.RUnlock()
		return hash, nil
	}
	cv.mu.RUnlock()

	// Find command binary path
	cmdPath, err := exec.LookPath(command)
	if err != nil {
		return "", fmt.Errorf("%w: %s", ErrCommandNotFound, command)
	}

	// Open and hash the file
	f, err := os.Open(cmdPath)
	if err != nil {
		return "", fmt.Errorf("failed to open command binary: %w", err)
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", fmt.Errorf("failed to hash command binary: %w", err)
	}

	hash := hex.EncodeToString(h.Sum(nil))

	// Cache the hash
	cv.mu.Lock()
	cv.hashCache[command] = hash
	cv.mu.Unlock()

	return hash, nil
}

// VerifyCommandHash verifies that a command's hash matches the expected hash
func (cv *CommandValidator) VerifyCommandHash(command string, expectedHash string) error {
	actualHash, err := cv.CalculateCommandHash(command)
	if err != nil {
		return err
	}

	// Normalize hashes (remove any "sha256:" prefix - case insensitive, then lowercase)
	actualHash = strings.ToLower(actualHash)
	actualHash = strings.TrimPrefix(actualHash, "sha256:")

	expectedHash = strings.ToLower(expectedHash)
	expectedHash = strings.TrimPrefix(expectedHash, "sha256:")

	if actualHash != expectedHash {
		return fmt.Errorf("%w: expected %s, got %s", ErrHashMismatch, expectedHash, actualHash)
	}

	return nil
}

// ClearHashCache clears the hash cache (useful for testing or after binary updates)
func (cv *CommandValidator) ClearHashCache() {
	cv.mu.Lock()
	defer cv.mu.Unlock()
	cv.hashCache = make(map[string]string)
}

// GetAllowedCommands returns a copy of the allowed commands list
func (cv *CommandValidator) GetAllowedCommands() []string {
	cv.mu.RLock()
	defer cv.mu.RUnlock()

	commands := make([]string, 0, len(cv.allowedCommands))
	for cmd := range cv.allowedCommands {
		commands = append(commands, cmd)
	}
	return commands
}
