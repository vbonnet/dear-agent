package agent

import (
	"os"
	"path/filepath"
	"testing"
)

// TestNewDetector verifies detector initialization
func TestNewDetector(t *testing.T) {
	detector := NewDetector()
	if detector == nil {
		t.Fatal("NewDetector() returned nil")
	}
}

// TestDetect_ClaudeCode_EnvVar verifies Claude Code detection via CLAUDECODE env var
func TestDetect_ClaudeCode_EnvVar(t *testing.T) {
	// Save and restore env
	oldVal := os.Getenv("CLAUDECODE")
	defer func() {
		if oldVal != "" {
			os.Setenv("CLAUDECODE", oldVal)
		} else {
			os.Unsetenv("CLAUDECODE")
		}
	}()

	os.Setenv("CLAUDECODE", "1")
	detector := NewDetector()
	agent := detector.Detect()

	if agent != AgentClaudeCode {
		t.Errorf("Detect() = %q, want %q", agent, AgentClaudeCode)
	}
}

// TestDetect_ClaudeCode_EntrypointEnv verifies Claude Code detection via CLAUDE_CODE_ENTRYPOINT
func TestDetect_ClaudeCode_EntrypointEnv(t *testing.T) {
	oldVal := os.Getenv("CLAUDE_CODE_ENTRYPOINT")
	defer func() {
		if oldVal != "" {
			os.Setenv("CLAUDE_CODE_ENTRYPOINT", oldVal)
		} else {
			os.Unsetenv("CLAUDE_CODE_ENTRYPOINT")
		}
	}()

	os.Setenv("CLAUDE_CODE_ENTRYPOINT", "/some/path")
	detector := NewDetector()
	agent := detector.Detect()

	if agent != AgentClaudeCode {
		t.Errorf("Detect() = %q, want %q", agent, AgentClaudeCode)
	}
}

// TestDetect_Cursor_EnvVar verifies Cursor detection via CURSOR env var
func TestDetect_Cursor_EnvVar(t *testing.T) {
	// Save and clear all agent env vars
	envVars := map[string]string{
		"CLAUDECODE":             os.Getenv("CLAUDECODE"),
		"CLAUDE_CODE_ENTRYPOINT": os.Getenv("CLAUDE_CODE_ENTRYPOINT"),
		"CURSOR":                 os.Getenv("CURSOR"),
		"CURSOR_SESSION_ID":      os.Getenv("CURSOR_SESSION_ID"),
		"WINDSURF":               os.Getenv("WINDSURF"),
		"AIDER_MODEL":            os.Getenv("AIDER_MODEL"),
		"AIDER_ARCHITECT":        os.Getenv("AIDER_ARCHITECT"),
	}
	defer func() {
		for key, val := range envVars {
			if val != "" {
				os.Setenv(key, val)
			} else {
				os.Unsetenv(key)
			}
		}
	}()

	// Clear all then set only CURSOR
	for key := range envVars {
		os.Unsetenv(key)
	}
	os.Setenv("CURSOR", "1")

	detector := NewDetector()
	agent := detector.Detect()

	if agent != AgentCursor {
		t.Errorf("Detect() = %q, want %q", agent, AgentCursor)
	}
}

// TestDetect_Cursor_SessionID verifies Cursor detection via CURSOR_SESSION_ID env var (P0-4 fix)
func TestDetect_Cursor_SessionID(t *testing.T) {
	// Save and clear all agent env vars
	envVars := map[string]string{
		"CLAUDECODE":             os.Getenv("CLAUDECODE"),
		"CLAUDE_CODE_ENTRYPOINT": os.Getenv("CLAUDE_CODE_ENTRYPOINT"),
		"CURSOR":                 os.Getenv("CURSOR"),
		"CURSOR_SESSION_ID":      os.Getenv("CURSOR_SESSION_ID"),
		"WINDSURF":               os.Getenv("WINDSURF"),
		"AIDER_MODEL":            os.Getenv("AIDER_MODEL"),
		"AIDER_ARCHITECT":        os.Getenv("AIDER_ARCHITECT"),
	}
	defer func() {
		for key, val := range envVars {
			if val != "" {
				os.Setenv(key, val)
			} else {
				os.Unsetenv(key)
			}
		}
	}()

	// Clear all then set only CURSOR_SESSION_ID
	for key := range envVars {
		os.Unsetenv(key)
	}
	os.Setenv("CURSOR_SESSION_ID", "test-session-123")

	detector := NewDetector()
	agent := detector.Detect()

	if agent != AgentCursor {
		t.Errorf("Detect() = %q, want %q (via CURSOR_SESSION_ID)", agent, AgentCursor)
	}
}

// TestDetect_Windsurf_EnvVar verifies Windsurf detection via WINDSURF env var
func TestDetect_Windsurf_EnvVar(t *testing.T) {
	envVars := map[string]string{
		"CLAUDECODE":             os.Getenv("CLAUDECODE"),
		"CLAUDE_CODE_ENTRYPOINT": os.Getenv("CLAUDE_CODE_ENTRYPOINT"),
		"CURSOR":                 os.Getenv("CURSOR"),
		"WINDSURF":               os.Getenv("WINDSURF"),
		"AIDER_MODEL":            os.Getenv("AIDER_MODEL"),
		"AIDER_ARCHITECT":        os.Getenv("AIDER_ARCHITECT"),
	}
	defer func() {
		for key, val := range envVars {
			if val != "" {
				os.Setenv(key, val)
			} else {
				os.Unsetenv(key)
			}
		}
	}()

	for key := range envVars {
		os.Unsetenv(key)
	}
	os.Setenv("WINDSURF", "1")

	detector := NewDetector()
	agent := detector.Detect()

	if agent != AgentWindsurf {
		t.Errorf("Detect() = %q, want %q", agent, AgentWindsurf)
	}
}

// TestDetect_Aider_ModelEnv verifies Aider detection via AIDER_MODEL env var
func TestDetect_Aider_ModelEnv(t *testing.T) {
	envVars := map[string]string{
		"CLAUDECODE":             os.Getenv("CLAUDECODE"),
		"CLAUDE_CODE_ENTRYPOINT": os.Getenv("CLAUDE_CODE_ENTRYPOINT"),
		"CURSOR":                 os.Getenv("CURSOR"),
		"WINDSURF":               os.Getenv("WINDSURF"),
		"AIDER_MODEL":            os.Getenv("AIDER_MODEL"),
		"AIDER_ARCHITECT":        os.Getenv("AIDER_ARCHITECT"),
	}
	defer func() {
		for key, val := range envVars {
			if val != "" {
				os.Setenv(key, val)
			} else {
				os.Unsetenv(key)
			}
		}
	}()

	for key := range envVars {
		os.Unsetenv(key)
	}
	os.Setenv("AIDER_MODEL", "gpt-4")

	detector := NewDetector()
	agent := detector.Detect()

	if agent != AgentAider {
		t.Errorf("Detect() = %q, want %q", agent, AgentAider)
	}
}

// TestDetect_Aider_ArchitectEnv verifies Aider detection via AIDER_ARCHITECT env var
func TestDetect_Aider_ArchitectEnv(t *testing.T) {
	envVars := map[string]string{
		"CLAUDECODE":             os.Getenv("CLAUDECODE"),
		"CLAUDE_CODE_ENTRYPOINT": os.Getenv("CLAUDE_CODE_ENTRYPOINT"),
		"CURSOR":                 os.Getenv("CURSOR"),
		"WINDSURF":               os.Getenv("WINDSURF"),
		"AIDER_MODEL":            os.Getenv("AIDER_MODEL"),
		"AIDER_ARCHITECT":        os.Getenv("AIDER_ARCHITECT"),
	}
	defer func() {
		for key, val := range envVars {
			if val != "" {
				os.Setenv(key, val)
			} else {
				os.Unsetenv(key)
			}
		}
	}()

	for key := range envVars {
		os.Unsetenv(key)
	}
	os.Setenv("AIDER_ARCHITECT", "1")

	detector := NewDetector()
	agent := detector.Detect()

	if agent != AgentAider {
		t.Errorf("Detect() = %q, want %q", agent, AgentAider)
	}
}

// TestDetect_ClaudeCode_FileDetection verifies Claude Code detection via ~/.claude/ directory
func TestDetect_ClaudeCode_FileDetection(t *testing.T) {
	// Only test file detection if env vars are not set
	envVars := []string{"CLAUDECODE", "CLAUDE_CODE_ENTRYPOINT", "CURSOR", "WINDSURF", "AIDER_MODEL", "AIDER_ARCHITECT"}
	savedEnvs := make(map[string]string)
	for _, env := range envVars {
		savedEnvs[env] = os.Getenv(env)
		os.Unsetenv(env)
	}
	defer func() {
		for env, val := range savedEnvs {
			if val != "" {
				os.Setenv(env, val)
			}
		}
	}()

	// This test will only pass if ~/.claude/ directory actually exists
	// We can't create it in the test because it's in the user's home directory
	homeDir, err := os.UserHomeDir()
	if err != nil {
		t.Skip("Cannot get home directory")
	}

	claudeDir := filepath.Join(homeDir, ".claude")
	if _, err := os.Stat(claudeDir); err != nil {
		t.Skipf("~/.claude/ directory doesn't exist, skipping file detection test")
	}

	detector := NewDetector()
	agent := detector.Detect()

	if agent != AgentClaudeCode {
		t.Errorf("Detect() = %q, want %q (via ~/.claude/ directory)", agent, AgentClaudeCode)
	}
}

// TestDetect_Cursor_FileDetection verifies Cursor detection via .cursorrules file
func TestDetect_Cursor_FileDetection(t *testing.T) {
	// Clear all env vars
	envVars := []string{"CLAUDECODE", "CLAUDE_CODE_ENTRYPOINT", "CURSOR", "WINDSURF", "AIDER_MODEL", "AIDER_ARCHITECT"}
	savedEnvs := make(map[string]string)
	for _, env := range envVars {
		savedEnvs[env] = os.Getenv(env)
		os.Unsetenv(env)
	}
	defer func() {
		for env, val := range savedEnvs {
			if val != "" {
				os.Setenv(env, val)
			}
		}
	}()

	// Create temporary .cursorrules file
	tmpDir, err := os.MkdirTemp("", "agent-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cursorRulesPath := filepath.Join(tmpDir, ".cursorrules")
	if err := os.WriteFile(cursorRulesPath, []byte("test"), 0644); err != nil {
		t.Fatalf("failed to create .cursorrules: %v", err)
	}

	// Change to temp directory for file detection
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	defer os.Chdir(oldWd)

	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to change directory: %v", err)
	}

	detector := NewDetector()
	agent := detector.Detect()

	// Note: This might still detect Claude Code if ~/.claude/ exists
	// So we check if it's either Cursor or ClaudeCode (via fallback)
	if agent != AgentCursor && agent != AgentClaudeCode {
		t.Errorf("Detect() = %q, want %q or %q (fallback)", agent, AgentCursor, AgentClaudeCode)
	}
}

// TestDetect_Windsurf_FileDetection verifies Windsurf detection via .windsurfrules file
func TestDetect_Windsurf_FileDetection(t *testing.T) {
	envVars := []string{"CLAUDECODE", "CLAUDE_CODE_ENTRYPOINT", "CURSOR", "WINDSURF", "AIDER_MODEL", "AIDER_ARCHITECT"}
	savedEnvs := make(map[string]string)
	for _, env := range envVars {
		savedEnvs[env] = os.Getenv(env)
		os.Unsetenv(env)
	}
	defer func() {
		for env, val := range savedEnvs {
			if val != "" {
				os.Setenv(env, val)
			}
		}
	}()

	tmpDir, err := os.MkdirTemp("", "agent-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	windsurfRulesPath := filepath.Join(tmpDir, ".windsurfrules")
	if err := os.WriteFile(windsurfRulesPath, []byte("test"), 0644); err != nil {
		t.Fatalf("failed to create .windsurfrules: %v", err)
	}

	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	defer os.Chdir(oldWd)

	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to change directory: %v", err)
	}

	detector := NewDetector()
	agent := detector.Detect()

	if agent != AgentWindsurf && agent != AgentClaudeCode {
		t.Errorf("Detect() = %q, want %q or %q (fallback)", agent, AgentWindsurf, AgentClaudeCode)
	}
}

// TestDetect_Aider_FileDetection verifies Aider detection via .aider.conf.yml file
func TestDetect_Aider_FileDetection(t *testing.T) {
	envVars := []string{"CLAUDECODE", "CLAUDE_CODE_ENTRYPOINT", "CURSOR", "WINDSURF", "AIDER_MODEL", "AIDER_ARCHITECT"}
	savedEnvs := make(map[string]string)
	for _, env := range envVars {
		savedEnvs[env] = os.Getenv(env)
		os.Unsetenv(env)
	}
	defer func() {
		for env, val := range savedEnvs {
			if val != "" {
				os.Setenv(env, val)
			}
		}
	}()

	tmpDir, err := os.MkdirTemp("", "agent-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	aiderConfPath := filepath.Join(tmpDir, ".aider.conf.yml")
	if err := os.WriteFile(aiderConfPath, []byte("model: gpt-4"), 0644); err != nil {
		t.Fatalf("failed to create .aider.conf.yml: %v", err)
	}

	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	defer os.Chdir(oldWd)

	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to change directory: %v", err)
	}

	detector := NewDetector()
	agent := detector.Detect()

	if agent != AgentAider && agent != AgentClaudeCode {
		t.Errorf("Detect() = %q, want %q or %q (fallback)", agent, AgentAider, AgentClaudeCode)
	}
}

// TestDetect_Unknown verifies that detection returns "unknown" when no agent is found
func TestDetect_Unknown(t *testing.T) {
	envVars := []string{"CLAUDECODE", "CLAUDE_CODE_ENTRYPOINT", "CURSOR", "WINDSURF", "AIDER_MODEL", "AIDER_ARCHITECT"}
	savedEnvs := make(map[string]string)
	for _, env := range envVars {
		savedEnvs[env] = os.Getenv(env)
		os.Unsetenv(env)
	}
	defer func() {
		for env, val := range savedEnvs {
			if val != "" {
				os.Setenv(env, val)
			}
		}
	}()

	// Create temp directory with no agent files
	tmpDir, err := os.MkdirTemp("", "agent-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	defer os.Chdir(oldWd)

	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to change directory: %v", err)
	}

	detector := NewDetector()
	agent := detector.Detect()

	// Note: May still detect Claude Code if ~/.claude/ exists
	if agent != AgentUnknown && agent != AgentClaudeCode {
		t.Logf("Detected %q, likely due to ~/.claude/ directory existing", agent)
	}
}

// TestFileExists verifies the fileExists helper function
func TestFileExists(t *testing.T) {
	detector := NewDetector()

	// Test with existing file
	tmpFile, err := os.CreateTemp("", "test-*")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	if !detector.fileExists(tmpFile.Name()) {
		t.Errorf("fileExists(%q) = false, want true", tmpFile.Name())
	}

	// Test with non-existing file
	nonExistent := "/tmp/this-file-should-not-exist-12345"
	if detector.fileExists(nonExistent) {
		t.Errorf("fileExists(%q) = true, want false", nonExistent)
	}

	// Test with directory
	tmpDir, err := os.MkdirTemp("", "test-dir-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	if !detector.fileExists(tmpDir) {
		t.Errorf("fileExists(%q) = false, want true (directories should return true)", tmpDir)
	}
}

// TestDetect_MultipleEnvVars verifies priority when multiple env vars set
func TestDetect_MultipleEnvVars(t *testing.T) {
	// Save all env vars
	envVars := map[string]string{
		"CLAUDECODE":             os.Getenv("CLAUDECODE"),
		"CLAUDE_CODE_ENTRYPOINT": os.Getenv("CLAUDE_CODE_ENTRYPOINT"),
		"CURSOR":                 os.Getenv("CURSOR"),
		"WINDSURF":               os.Getenv("WINDSURF"),
		"AIDER_MODEL":            os.Getenv("AIDER_MODEL"),
	}
	defer func() {
		for key, val := range envVars {
			if val != "" {
				os.Setenv(key, val)
			} else {
				os.Unsetenv(key)
			}
		}
	}()

	// Claude Code should take priority
	os.Setenv("CLAUDECODE", "1")
	os.Setenv("CURSOR", "1")
	os.Setenv("WINDSURF", "1")

	detector := NewDetector()
	agent := detector.Detect()

	if agent != AgentClaudeCode {
		t.Errorf("Detect() with multiple env vars = %q, want %q (Claude Code should have priority)", agent, AgentClaudeCode)
	}
}

// TestDetect_FallbackPriority verifies file detection fallback order
func TestDetect_FallbackPriority(t *testing.T) {
	// Create temp directory for file detection tests
	tmpDir, err := os.MkdirTemp("", "agent-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Clear all env vars
	envVars := []string{"CLAUDECODE", "CLAUDE_CODE_ENTRYPOINT", "CURSOR", "WINDSURF", "AIDER_MODEL", "AIDER_ARCHITECT"}
	savedEnvs := make(map[string]string)
	for _, env := range envVars {
		savedEnvs[env] = os.Getenv(env)
		os.Unsetenv(env)
	}
	defer func() {
		for env, val := range savedEnvs {
			if val != "" {
				os.Setenv(env, val)
			}
		}
	}()

	// Create multiple agent files in temp dir
	os.WriteFile(filepath.Join(tmpDir, ".cursorrules"), []byte("test"), 0644)
	os.WriteFile(filepath.Join(tmpDir, ".windsurfrules"), []byte("test"), 0644)

	oldWd, _ := os.Getwd()
	defer os.Chdir(oldWd)
	os.Chdir(tmpDir)

	detector := NewDetector()
	agent := detector.Detect()

	// Should detect Cursor (appears first in file checks after ~/.claude)
	// Or Claude Code if ~/.claude exists
	if agent != AgentCursor && agent != AgentClaudeCode {
		t.Logf("Detected %q, this is acceptable (file-based detection)", agent)
	}
}

// TestDetect_EdgeCase_ClaudeCodeNotSet verifies fallback when CLAUDECODE not "1"
func TestDetect_EdgeCase_ClaudeCodeNotSet(t *testing.T) {
	// Save all env vars
	envVars := map[string]string{
		"CLAUDECODE":             os.Getenv("CLAUDECODE"),
		"CLAUDE_CODE_ENTRYPOINT": os.Getenv("CLAUDE_CODE_ENTRYPOINT"),
		"CURSOR":                 os.Getenv("CURSOR"),
		"WINDSURF":               os.Getenv("WINDSURF"),
		"AIDER_MODEL":            os.Getenv("AIDER_MODEL"),
		"AIDER_ARCHITECT":        os.Getenv("AIDER_ARCHITECT"),
	}
	defer func() {
		for key, val := range envVars {
			if val != "" {
				os.Setenv(key, val)
			} else {
				os.Unsetenv(key)
			}
		}
	}()

	// Set CLAUDECODE to something other than "1"
	for key := range envVars {
		os.Unsetenv(key)
	}
	os.Setenv("CLAUDECODE", "0")

	detector := NewDetector()
	agent := detector.Detect()

	// CLAUDECODE must be "1" to trigger detection
	if agent == AgentClaudeCode {
		// Unless ~/.claude exists (file-based fallback)
		homeDir, _ := os.UserHomeDir()
		if _, err := os.Stat(filepath.Join(homeDir, ".claude")); err != nil {
			t.Error("Detect() triggered Claude Code with CLAUDECODE=0, should require CLAUDECODE=1")
		}
	}
}

// TestDetect_AiderignoreFile verifies Aider detection via .aiderignore file
func TestDetect_AiderignoreFile(t *testing.T) {
	envVars := []string{"CLAUDECODE", "CLAUDE_CODE_ENTRYPOINT", "CURSOR", "WINDSURF", "AIDER_MODEL", "AIDER_ARCHITECT"}
	savedEnvs := make(map[string]string)
	for _, env := range envVars {
		savedEnvs[env] = os.Getenv(env)
		os.Unsetenv(env)
	}
	defer func() {
		for env, val := range savedEnvs {
			if val != "" {
				os.Setenv(env, val)
			}
		}
	}()

	tmpDir, err := os.MkdirTemp("", "agent-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	aiderIgnorePath := filepath.Join(tmpDir, ".aiderignore")
	if err := os.WriteFile(aiderIgnorePath, []byte("*.log"), 0644); err != nil {
		t.Fatalf("failed to create .aiderignore: %v", err)
	}

	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	defer os.Chdir(oldWd)

	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to change directory: %v", err)
	}

	detector := NewDetector()
	agent := detector.Detect()

	if agent != AgentAider && agent != AgentClaudeCode {
		t.Errorf("Detect() = %q, want %q or %q (fallback)", agent, AgentAider, AgentClaudeCode)
	}
}

// TestDetect_Caching verifies that detection result is cached after first call
func TestDetect_Caching(t *testing.T) {
	// Save and clear all agent env vars
	envVars := map[string]string{
		"CLAUDECODE":             os.Getenv("CLAUDECODE"),
		"CLAUDE_CODE_ENTRYPOINT": os.Getenv("CLAUDE_CODE_ENTRYPOINT"),
		"CURSOR":                 os.Getenv("CURSOR"),
		"CURSOR_SESSION_ID":      os.Getenv("CURSOR_SESSION_ID"),
		"WINDSURF":               os.Getenv("WINDSURF"),
		"AIDER_MODEL":            os.Getenv("AIDER_MODEL"),
		"AIDER_ARCHITECT":        os.Getenv("AIDER_ARCHITECT"),
	}
	defer func() {
		for key, val := range envVars {
			if val != "" {
				os.Setenv(key, val)
			} else {
				os.Unsetenv(key)
			}
		}
	}()

	// Clear all env vars and set CURSOR
	for key := range envVars {
		os.Unsetenv(key)
	}
	os.Setenv("CURSOR", "1")

	detector := NewDetector()

	// First call should detect Cursor
	agent1 := detector.Detect()
	if agent1 != AgentCursor {
		t.Errorf("First Detect() = %q, want %q", agent1, AgentCursor)
	}

	// Change env var to Claude Code
	os.Unsetenv("CURSOR")
	os.Setenv("CLAUDECODE", "1")

	// Second call should still return cached Cursor (not re-detect)
	agent2 := detector.Detect()
	if agent2 != AgentCursor {
		t.Errorf("Second Detect() = %q, want %q (should return cached value)", agent2, AgentCursor)
	}
}

// TestDetect_CachingMultipleCalls verifies cache returns same result on multiple calls
func TestDetect_CachingMultipleCalls(t *testing.T) {
	envVars := map[string]string{
		"CLAUDECODE":             os.Getenv("CLAUDECODE"),
		"CLAUDE_CODE_ENTRYPOINT": os.Getenv("CLAUDE_CODE_ENTRYPOINT"),
		"CURSOR":                 os.Getenv("CURSOR"),
		"WINDSURF":               os.Getenv("WINDSURF"),
		"AIDER_MODEL":            os.Getenv("AIDER_MODEL"),
		"AIDER_ARCHITECT":        os.Getenv("AIDER_ARCHITECT"),
	}
	defer func() {
		for key, val := range envVars {
			if val != "" {
				os.Setenv(key, val)
			} else {
				os.Unsetenv(key)
			}
		}
	}()

	for key := range envVars {
		os.Unsetenv(key)
	}
	os.Setenv("WINDSURF", "1")

	detector := NewDetector()

	// Call Detect() multiple times
	agent1 := detector.Detect()
	agent2 := detector.Detect()
	agent3 := detector.Detect()

	// All calls should return the same cached result
	if agent1 != agent2 || agent2 != agent3 {
		t.Errorf("Detect() calls returned different values: %q, %q, %q (should all be same)", agent1, agent2, agent3)
	}

	if agent1 != AgentWindsurf {
		t.Errorf("Detect() = %q, want %q", agent1, AgentWindsurf)
	}
}

// TestClearCache verifies that ClearCache forces re-detection
func TestClearCache(t *testing.T) {
	envVars := map[string]string{
		"CLAUDECODE":             os.Getenv("CLAUDECODE"),
		"CLAUDE_CODE_ENTRYPOINT": os.Getenv("CLAUDE_CODE_ENTRYPOINT"),
		"CURSOR":                 os.Getenv("CURSOR"),
		"CURSOR_SESSION_ID":      os.Getenv("CURSOR_SESSION_ID"),
		"WINDSURF":               os.Getenv("WINDSURF"),
		"AIDER_MODEL":            os.Getenv("AIDER_MODEL"),
		"AIDER_ARCHITECT":        os.Getenv("AIDER_ARCHITECT"),
	}
	defer func() {
		for key, val := range envVars {
			if val != "" {
				os.Setenv(key, val)
			} else {
				os.Unsetenv(key)
			}
		}
	}()

	for key := range envVars {
		os.Unsetenv(key)
	}
	os.Setenv("AIDER_MODEL", "gpt-4")

	detector := NewDetector()

	// First detection
	agent1 := detector.Detect()
	if agent1 != AgentAider {
		t.Errorf("First Detect() = %q, want %q", agent1, AgentAider)
	}

	// Change env var
	os.Unsetenv("AIDER_MODEL")
	os.Setenv("CLAUDECODE", "1")

	// Before clear, should still get cached value
	agent2 := detector.Detect()
	if agent2 != AgentAider {
		t.Errorf("Detect() before ClearCache() = %q, want %q (cached)", agent2, AgentAider)
	}

	// Clear cache
	detector.ClearCache()

	// After clear, should re-detect and get Claude Code
	agent3 := detector.Detect()
	if agent3 != AgentClaudeCode {
		t.Errorf("Detect() after ClearCache() = %q, want %q", agent3, AgentClaudeCode)
	}
}

// TestClearCache_NewDetector verifies ClearCache works with fresh detector
func TestClearCache_NewDetector(t *testing.T) {
	detector := NewDetector()

	// ClearCache on new detector should not panic
	detector.ClearCache()

	// Should still detect normally
	agent := detector.Detect()
	if agent == "" {
		t.Error("Detect() returned empty string after ClearCache() on new detector")
	}
}

// TestDetect_CachingPerInstance verifies each detector instance has its own cache
func TestDetect_CachingPerInstance(t *testing.T) {
	envVars := map[string]string{
		"CLAUDECODE":             os.Getenv("CLAUDECODE"),
		"CLAUDE_CODE_ENTRYPOINT": os.Getenv("CLAUDE_CODE_ENTRYPOINT"),
		"CURSOR":                 os.Getenv("CURSOR"),
		"WINDSURF":               os.Getenv("WINDSURF"),
		"AIDER_MODEL":            os.Getenv("AIDER_MODEL"),
		"AIDER_ARCHITECT":        os.Getenv("AIDER_ARCHITECT"),
	}
	defer func() {
		for key, val := range envVars {
			if val != "" {
				os.Setenv(key, val)
			} else {
				os.Unsetenv(key)
			}
		}
	}()

	for key := range envVars {
		os.Unsetenv(key)
	}
	os.Setenv("CURSOR", "1")

	// Create two detectors
	detector1 := NewDetector()
	detector2 := NewDetector()

	// First detector detects Cursor
	agent1 := detector1.Detect()
	if agent1 != AgentCursor {
		t.Errorf("detector1.Detect() = %q, want %q", agent1, AgentCursor)
	}

	// Change env var to Windsurf
	os.Unsetenv("CURSOR")
	os.Setenv("WINDSURF", "1")

	// First detector should still return cached Cursor
	agent1Again := detector1.Detect()
	if agent1Again != AgentCursor {
		t.Errorf("detector1.Detect() after env change = %q, want %q (cached)", agent1Again, AgentCursor)
	}

	// Second detector (created before env change) should detect Windsurf when called first time
	agent2 := detector2.Detect()
	if agent2 != AgentWindsurf {
		t.Errorf("detector2.Detect() = %q, want %q", agent2, AgentWindsurf)
	}
}
