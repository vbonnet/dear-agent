package hooks

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestValidateCommand(t *testing.T) {
	cv := NewCommandValidator()

	tests := []struct {
		name    string
		command string
		wantErr bool
	}{
		{
			name:    "allowed command - git",
			command: "git",
			wantErr: false,
		},
		{
			name:    "allowed command - npm",
			command: "npm",
			wantErr: false,
		},
		{
			name:    "allowed command - go",
			command: "go",
			wantErr: false,
		},
		{
			name:    "allowed command - bow-core",
			command: "bow-core",
			wantErr: false,
		},
		{
			name:    "disallowed command - sh",
			command: "sh",
			wantErr: true,
		},
		{
			name:    "disallowed command - bash",
			command: "bash",
			wantErr: true,
		},
		{
			name:    "disallowed command - eval",
			command: "eval",
			wantErr: true,
		},
		{
			name:    "disallowed command - rm",
			command: "rm",
			wantErr: true,
		},
		{
			name:    "disallowed command - curl",
			command: "curl",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := cv.ValidateCommand(tt.command)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateCommand(%s) error = %v, wantErr %v", tt.command, err, tt.wantErr)
			}
		})
	}
}

func TestAddRemoveCommand(t *testing.T) {
	cv := NewCommandValidator()

	// Initially not allowed
	if cv.IsAllowed("custom-tool") {
		t.Error("custom-tool should not be allowed initially")
	}

	// Add command
	cv.AddCommand("custom-tool")

	// Now should be allowed
	if !cv.IsAllowed("custom-tool") {
		t.Error("custom-tool should be allowed after adding")
	}

	// Validate should pass
	if err := cv.ValidateCommand("custom-tool"); err != nil {
		t.Errorf("ValidateCommand(custom-tool) should pass after adding, got error: %v", err)
	}

	// Remove command
	cv.RemoveCommand("custom-tool")

	// Should not be allowed anymore
	if cv.IsAllowed("custom-tool") {
		t.Error("custom-tool should not be allowed after removing")
	}
}

func TestLoadSaveAllowlist(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "allowed-commands.toml")

	cv := NewCommandValidatorWithPath(path)

	// Add custom commands
	cv.AddCommand("custom-tool-1")
	cv.AddCommand("custom-tool-2")

	// Save
	if err := cv.SaveAllowlist(); err != nil {
		t.Fatalf("SaveAllowlist failed: %v", err)
	}

	// Create new validator and load
	cv2 := NewCommandValidatorWithPath(path)
	if err := cv2.LoadAllowlist(); err != nil {
		t.Fatalf("LoadAllowlist failed: %v", err)
	}

	// Verify custom commands are loaded
	if !cv2.IsAllowed("custom-tool-1") {
		t.Error("custom-tool-1 should be loaded")
	}
	if !cv2.IsAllowed("custom-tool-2") {
		t.Error("custom-tool-2 should be loaded")
	}

	// Verify default commands still present
	if !cv2.IsAllowed("git") {
		t.Error("git should still be allowed (default)")
	}
}

func TestCalculateCommandHash(t *testing.T) {
	cv := NewCommandValidator()

	// Test with a known command (e.g., /bin/echo or /usr/bin/env)
	// Use a command that's likely to exist on most systems
	testCmd := "echo"
	if _, err := exec.LookPath(testCmd); err != nil {
		t.Skipf("Skipping test: %s not found in PATH", testCmd)
	}

	hash1, err := cv.CalculateCommandHash(testCmd)
	if err != nil {
		t.Fatalf("CalculateCommandHash failed: %v", err)
	}

	if hash1 == "" {
		t.Error("Hash should not be empty")
	}

	if len(hash1) != 64 {
		t.Errorf("SHA-256 hash should be 64 characters, got %d", len(hash1))
	}

	// Calculate again - should use cache
	hash2, err := cv.CalculateCommandHash(testCmd)
	if err != nil {
		t.Fatalf("CalculateCommandHash (cached) failed: %v", err)
	}

	if hash1 != hash2 {
		t.Error("Cached hash should match original hash")
	}
}

func TestVerifyCommandHash(t *testing.T) {
	cv := NewCommandValidator()

	// Find echo command
	testCmd := "echo"
	cmdPath, err := exec.LookPath(testCmd)
	if err != nil {
		t.Skipf("Skipping test: %s not found in PATH", testCmd)
	}

	// Calculate actual hash manually
	data, err := os.ReadFile(cmdPath)
	if err != nil {
		t.Fatalf("Failed to read command: %v", err)
	}
	h := sha256.New()
	h.Reset()
	h.Write(data)
	expectedHash := hex.EncodeToString(h.Sum(nil))

	// Verify with correct hash
	if err := cv.VerifyCommandHash(testCmd, expectedHash); err != nil {
		t.Errorf("VerifyCommandHash with correct hash failed: %v", err)
	}

	// Verify with incorrect hash
	wrongHash := "0000000000000000000000000000000000000000000000000000000000000000"
	if err := cv.VerifyCommandHash(testCmd, wrongHash); err == nil {
		t.Error("VerifyCommandHash with wrong hash should fail")
	}
}

func TestVerifyCommandHashWithPrefix(t *testing.T) {
	cv := NewCommandValidator()

	testCmd := "echo"
	if _, err := exec.LookPath(testCmd); err != nil {
		t.Skipf("Skipping test: %s not found in PATH", testCmd)
	}

	// Get actual hash
	actualHash, err := cv.CalculateCommandHash(testCmd)
	if err != nil {
		t.Fatalf("CalculateCommandHash failed: %v", err)
	}

	// Test with sha256: prefix
	hashWithPrefix := "sha256:" + actualHash
	if err := cv.VerifyCommandHash(testCmd, hashWithPrefix); err != nil {
		t.Errorf("VerifyCommandHash should handle sha256: prefix, got error: %v", err)
	}

	// Test with uppercase
	hashUppercase := "SHA256:" + actualHash
	if err := cv.VerifyCommandHash(testCmd, hashUppercase); err != nil {
		t.Errorf("VerifyCommandHash should be case-insensitive, got error: %v", err)
	}
}

func TestHashCache(t *testing.T) {
	cv := NewCommandValidator()

	testCmd := "echo"
	if _, err := exec.LookPath(testCmd); err != nil {
		t.Skipf("Skipping test: %s not found in PATH", testCmd)
	}

	// Calculate hash
	hash1, err := cv.CalculateCommandHash(testCmd)
	if err != nil {
		t.Fatalf("CalculateCommandHash failed: %v", err)
	}

	// Clear cache
	cv.ClearHashCache()

	// Calculate again (should recalculate)
	hash2, err := cv.CalculateCommandHash(testCmd)
	if err != nil {
		t.Fatalf("CalculateCommandHash after cache clear failed: %v", err)
	}

	// Hashes should still match (same binary)
	if hash1 != hash2 {
		t.Error("Hash should be consistent even after cache clear")
	}
}

func TestCommandNotFound(t *testing.T) {
	cv := NewCommandValidator()

	_, err := cv.CalculateCommandHash("nonexistent-command-xyz")
	if err == nil {
		t.Error("CalculateCommandHash should fail for nonexistent command")
	}

	if !isErrorType(err, ErrCommandNotFound) {
		t.Errorf("Expected ErrCommandNotFound, got: %v", err)
	}
}

func TestGetAllowedCommands(t *testing.T) {
	cv := NewCommandValidator()

	commands := cv.GetAllowedCommands()

	// Should contain default commands
	found := false
	for _, cmd := range commands {
		if cmd == "git" {
			found = true
			break
		}
	}
	if !found {
		t.Error("GetAllowedCommands should contain 'git' (default)")
	}

	// Add custom command
	cv.AddCommand("custom-test")
	commands = cv.GetAllowedCommands()

	found = false
	for _, cmd := range commands {
		if cmd == "custom-test" {
			found = true
			break
		}
	}
	if !found {
		t.Error("GetAllowedCommands should contain added 'custom-test'")
	}
}

func TestLoadNonexistentAllowlist(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "nonexistent.toml")

	cv := NewCommandValidatorWithPath(path)

	// Should not error (graceful handling)
	if err := cv.LoadAllowlist(); err != nil {
		t.Errorf("LoadAllowlist with nonexistent file should not error, got: %v", err)
	}

	// Should have default commands
	if !cv.IsAllowed("git") {
		t.Error("Should have default commands even without allowlist file")
	}
}

func TestValidateCommandWithPath(t *testing.T) {
	cv := NewCommandValidator()

	// Test with full path (should extract base command)
	if err := cv.ValidateCommand("/usr/bin/git"); err != nil {
		t.Errorf("ValidateCommand should handle full paths, got error: %v", err)
	}

	// Test with relative path
	if err := cv.ValidateCommand("./git"); err != nil {
		t.Errorf("ValidateCommand should handle relative paths, got error: %v", err)
	}
}

func TestSecurityThreatMitigation(t *testing.T) {
	cv := NewCommandValidator()

	// Dangerous commands that should be rejected
	dangerousCommands := []string{
		"sh",
		"bash",
		"eval",
		"curl",
		"wget",
		"rm",
		"dd",
		"sudo",
	}

	for _, cmd := range dangerousCommands {
		t.Run("reject_"+cmd, func(t *testing.T) {
			if err := cv.ValidateCommand(cmd); err == nil {
				t.Errorf("Dangerous command %s should be rejected", cmd)
			}
		})
	}
}

func TestAllowlistPersistence(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "allowed-commands.toml")

	// Create first validator, add command, save
	cv1 := NewCommandValidatorWithPath(path)
	cv1.AddCommand("test-tool-persist")
	if err := cv1.SaveAllowlist(); err != nil {
		t.Fatalf("SaveAllowlist failed: %v", err)
	}

	// Verify file was created
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Error("Allowlist file was not created")
	}

	// Create second validator, load
	cv2 := NewCommandValidatorWithPath(path)

	// Verify command is loaded
	if !cv2.IsAllowed("test-tool-persist") {
		t.Error("Command should persist across validator instances")
	}
}
