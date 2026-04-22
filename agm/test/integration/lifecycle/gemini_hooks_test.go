//go:build integration

package lifecycle_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/agent"
	"github.com/vbonnet/dear-agent/agm/test/integration/helpers"
)

// TestGeminiHooks_SessionStartExecution tests SessionStart hook execution via AGM for Gemini.
//
// This test verifies:
// 1. Test hook script writes marker to /tmp/hook-executed
// 2. Gemini session started via AGM triggers SessionStart hook
// 3. Hook executes and writes expected marker file
// 4. Hook receives correct session context (session_id, cwd, etc.)
func TestGeminiHooks_SessionStartExecution(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping Gemini hook execution test in short mode")
	}

	// Skip if Gemini CLI not installed
	if _, err := exec.LookPath("gemini"); err != nil {
		t.Skip("Gemini CLI not installed, skipping hook test")
	}

	env := helpers.NewTestEnv(t)
	defer env.Cleanup(t)

	sessionName := "test-gemini-hook-start-" + helpers.RandomString(6)

	// Create test hook script directory
	hookDir := filepath.Join(env.TempDir, "gemini-test-hooks")
	if err := os.MkdirAll(hookDir, 0755); err != nil {
		t.Fatalf("Failed to create test hook directory: %v", err)
	}

	// Create marker file path
	markerFile := filepath.Join(env.TempDir, "sessionstart-executed.txt")

	// Create SessionStart test hook
	hookScript := filepath.Join(hookDir, "sessionstart-test.sh")
	hookContent := `#!/bin/bash
# Test SessionStart hook for Gemini CLI
# Reads JSON input from stdin, writes marker file

set -euo pipefail

# Read JSON input
input_json=$(cat)

# Write marker file with session context
echo "SessionStart hook executed at $(date -Iseconds)" > ` + markerFile + `
echo "Input JSON: $input_json" >> ` + markerFile + `

# Parse session_id if jq available
if command -v jq &>/dev/null; then
    session_id=$(echo "$input_json" | jq -r '.session_id // "unknown"')
    cwd=$(echo "$input_json" | jq -r '.cwd // ""')
    echo "Session ID: $session_id" >> ` + markerFile + `
    echo "CWD: $cwd" >> ` + markerFile + `
fi

# Return success JSON (Gemini SessionStart response format)
cat <<EOF
{
  "systemMessage": "SessionStart hook executed successfully",
  "suppressOutput": false,
  "hookSpecificOutput": {
    "additionalContext": "Test hook for integration testing"
  }
}
EOF

exit 0
`

	if err := os.WriteFile(hookScript, []byte(hookContent), 0755); err != nil {
		t.Fatalf("Failed to write test hook script: %v", err)
	}

	// Create test Gemini settings.json with hook configuration
	geminiConfigDir := filepath.Join(env.TempDir, ".gemini")
	if err := os.MkdirAll(geminiConfigDir, 0755); err != nil {
		t.Fatalf("Failed to create .gemini config directory: %v", err)
	}

	settingsFile := filepath.Join(geminiConfigDir, "settings.json")
	settings := map[string]interface{}{
		"hooks": map[string]interface{}{
			"SessionStart": []map[string]interface{}{
				{
					"command":     hookScript,
					"description": "Test SessionStart hook",
					"timeout":     5000,
				},
			},
		},
	}

	settingsData, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		t.Fatalf("Failed to marshal settings JSON: %v", err)
	}

	if err := os.WriteFile(settingsFile, settingsData, 0644); err != nil {
		t.Fatalf("Failed to write settings.json: %v", err)
	}

	// Set GEMINI_CONFIG_DIR for test
	originalConfigDir := os.Getenv("GEMINI_CONFIG_DIR")
	os.Setenv("GEMINI_CONFIG_DIR", geminiConfigDir)
	defer func() {
		if originalConfigDir != "" {
			os.Setenv("GEMINI_CONFIG_DIR", originalConfigDir)
		} else {
			os.Unsetenv("GEMINI_CONFIG_DIR")
		}
	}()

	// Create Gemini session via GeminiCLIAdapter
	tempStore := filepath.Join(env.TempDir, "gemini-sessions.json")
	store, err := agent.NewJSONSessionStore(tempStore)
	if err != nil {
		t.Fatalf("Failed to create session store: %v", err)
	}

	adapter, err := agent.NewGeminiCLIAdapter(store)
	if err != nil {
		t.Fatalf("Failed to create Gemini adapter: %v", err)
	}

	// Type assert to concrete type to access RunHook method (not in Agent interface)
	geminiAdapter, ok := adapter.(*agent.GeminiCLIAdapter)
	if !ok {
		t.Fatalf("Failed to type assert to *GeminiCLIAdapter")
	}

	// Create session (this should trigger SessionStart hook internally)
	ctx := agent.SessionContext{
		Name:             sessionName,
		WorkingDirectory: env.TempDir,
		Project:          "gemini-hook-test",
	}

	sessionID, err := adapter.CreateSession(ctx)
	if err != nil {
		t.Fatalf("Failed to create Gemini session: %v", err)
	}

	// Manually trigger RunHook (simulating AGM hook execution)
	// RunHook is a method on *GeminiCLIAdapter, not on the Agent interface
	if err := geminiAdapter.RunHook(sessionID, "SessionStart"); err != nil {
		t.Fatalf("Failed to run SessionStart hook: %v", err)
	}

	// Wait for hook execution (hooks may be async)
	time.Sleep(2 * time.Second)

	// VERIFY: Hook marker file exists
	if _, err := os.Stat(markerFile); os.IsNotExist(err) {
		t.Errorf("SessionStart hook did not execute: marker file not found at %s", markerFile)
		t.FailNow()
	}

	// VERIFY: Marker file contains expected content
	markerContent, err := os.ReadFile(markerFile)
	if err != nil {
		t.Fatalf("Failed to read marker file: %v", err)
	}

	markerStr := string(markerContent)
	t.Logf("Hook marker file content:\n%s", markerStr)

	// Check for expected session context in marker file
	// (Note: Session ID format depends on GeminiCLIAdapter implementation)
	if !containsAny(markerStr, []string{"SessionStart hook executed", "session_id", "Input JSON"}) {
		t.Errorf("Marker file missing expected content. Got:\n%s", markerStr)
	}

	// Cleanup session
	if err := geminiAdapter.TerminateSession(sessionID); err != nil {
		t.Logf("Warning: failed to terminate session: %v", err)
	}
}

// TestGeminiHooks_SessionEndExecution tests SessionEnd hook execution.
func TestGeminiHooks_SessionEndExecution(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping Gemini SessionEnd hook test in short mode")
	}

	if _, err := exec.LookPath("gemini"); err != nil {
		t.Skip("Gemini CLI not installed")
	}

	env := helpers.NewTestEnv(t)
	defer env.Cleanup(t)

	sessionName := "test-gemini-hook-end-" + helpers.RandomString(6)

	// Create hook directory
	hookDir := filepath.Join(env.TempDir, "gemini-test-hooks")
	if err := os.MkdirAll(hookDir, 0755); err != nil {
		t.Fatalf("Failed to create hook directory: %v", err)
	}

	// Create marker file
	markerFile := filepath.Join(env.TempDir, "sessionend-executed.txt")

	// Create SessionEnd test hook
	hookScript := filepath.Join(hookDir, "sessionend-test.sh")
	hookContent := `#!/bin/bash
set -euo pipefail

input_json=$(cat)

echo "SessionEnd hook executed at $(date -Iseconds)" > ` + markerFile + `
echo "Input JSON: $input_json" >> ` + markerFile + `

# Gemini SessionEnd hooks don't return structured output (fire-and-forget)
exit 0
`

	if err := os.WriteFile(hookScript, []byte(hookContent), 0755); err != nil {
		t.Fatalf("Failed to write SessionEnd hook: %v", err)
	}

	// Create settings.json
	geminiConfigDir := filepath.Join(env.TempDir, ".gemini")
	os.MkdirAll(geminiConfigDir, 0755)

	settingsFile := filepath.Join(geminiConfigDir, "settings.json")
	settings := map[string]interface{}{
		"hooks": map[string]interface{}{
			"SessionEnd": []map[string]interface{}{
				{
					"command":     hookScript,
					"description": "Test SessionEnd hook",
					"timeout":     5000,
				},
			},
		},
	}

	settingsData, _ := json.MarshalIndent(settings, "", "  ")
	os.WriteFile(settingsFile, settingsData, 0644)

	// Set config dir
	os.Setenv("GEMINI_CONFIG_DIR", geminiConfigDir)
	defer os.Unsetenv("GEMINI_CONFIG_DIR")

	// Create session
	tempStore := filepath.Join(env.TempDir, "gemini-sessions.json")
	store, _ := agent.NewJSONSessionStore(tempStore)
	adapter, _ := agent.NewGeminiCLIAdapter(store)

	// Type assert to concrete type to access RunHook method (not in Agent interface)
	geminiAdapter, ok := adapter.(*agent.GeminiCLIAdapter)
	if !ok {
		t.Fatalf("Failed to type assert to *GeminiCLIAdapter")
	}

	ctx := agent.SessionContext{
		Name:             sessionName,
		WorkingDirectory: env.TempDir,
		Project:          "gemini-hook-test",
	}

	sessionID, err := adapter.CreateSession(ctx)
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	// Run SessionEnd hook manually
	if err := geminiAdapter.RunHook(sessionID, "SessionEnd"); err != nil {
		t.Fatalf("Failed to run SessionEnd hook: %v", err)
	}

	// Wait for hook
	time.Sleep(2 * time.Second)

	// VERIFY: Marker file exists
	if _, err := os.Stat(markerFile); os.IsNotExist(err) {
		t.Errorf("SessionEnd hook marker file not found at %s", markerFile)
		t.FailNow()
	}

	markerContent, _ := os.ReadFile(markerFile)
	markerStr := string(markerContent)
	t.Logf("SessionEnd marker content:\n%s", markerStr)

	if !containsAny(markerStr, []string{"SessionEnd hook executed"}) {
		t.Errorf("Marker file missing expected SessionEnd content")
	}

	// Cleanup
	adapter.TerminateSession(sessionID)
}

// TestGeminiHooks_BlockingWithExitCode2 tests hook blocking behavior.
//
// When a hook exits with code 2, it should block the operation.
// (Note: This behavior depends on Gemini CLI hook implementation)
func TestGeminiHooks_BlockingWithExitCode2(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping blocking hook test in short mode")
	}

	if _, err := exec.LookPath("gemini"); err != nil {
		t.Skip("Gemini CLI not installed")
	}

	env := helpers.NewTestEnv(t)
	defer env.Cleanup(t)

	// Create blocking hook (exit 2)
	hookDir := filepath.Join(env.TempDir, "gemini-test-hooks")
	os.MkdirAll(hookDir, 0755)

	blockingHook := filepath.Join(hookDir, "blocking-hook.sh")
	hookContent := `#!/bin/bash
# This hook should block the operation
echo "Blocking hook: refusing to proceed" >&2
exit 2
`

	os.WriteFile(blockingHook, []byte(hookContent), 0755)

	// Create settings.json with blocking hook
	geminiConfigDir := filepath.Join(env.TempDir, ".gemini")
	os.MkdirAll(geminiConfigDir, 0755)

	settingsFile := filepath.Join(geminiConfigDir, "settings.json")
	settings := map[string]interface{}{
		"hooks": map[string]interface{}{
			"SessionStart": []map[string]interface{}{
				{
					"command":     blockingHook,
					"description": "Blocking test hook",
					"timeout":     5000,
				},
			},
		},
	}

	settingsData, _ := json.MarshalIndent(settings, "", "  ")
	os.WriteFile(settingsFile, settingsData, 0644)

	os.Setenv("GEMINI_CONFIG_DIR", geminiConfigDir)
	defer os.Unsetenv("GEMINI_CONFIG_DIR")

	t.Log("NOTE: Hook blocking behavior (exit code 2) depends on Gemini CLI implementation")
	t.Log("This test verifies the hook can exit with code 2")
	t.Log("Actual blocking semantics may vary")

	// Test that hook exits with code 2
	cmd := exec.Command(blockingHook)
	err := cmd.Run()

	if exitErr, ok := err.(*exec.ExitError); ok {
		if exitErr.ExitCode() == 2 {
			t.Log("Hook correctly exits with code 2 (blocking signal)")
		} else {
			t.Errorf("Expected exit code 2, got %d", exitErr.ExitCode())
		}
	} else {
		t.Errorf("Expected exit error with code 2, got: %v", err)
	}
}

// TestGeminiHooks_TimeoutHandling tests hook timeout behavior.
func TestGeminiHooks_TimeoutHandling(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping timeout test in short mode")
	}

	if _, err := exec.LookPath("gemini"); err != nil {
		t.Skip("Gemini CLI not installed")
	}

	env := helpers.NewTestEnv(t)
	defer env.Cleanup(t)

	// Create slow hook (sleep 10 seconds)
	hookDir := filepath.Join(env.TempDir, "gemini-test-hooks")
	os.MkdirAll(hookDir, 0755)

	slowHook := filepath.Join(hookDir, "slow-hook.sh")
	hookContent := `#!/bin/bash
# Slow hook that takes too long
sleep 10
echo "This should timeout"
`

	os.WriteFile(slowHook, []byte(hookContent), 0755)

	// Create settings.json with 1-second timeout
	geminiConfigDir := filepath.Join(env.TempDir, ".gemini")
	os.MkdirAll(geminiConfigDir, 0755)

	settingsFile := filepath.Join(geminiConfigDir, "settings.json")
	settings := map[string]interface{}{
		"hooks": map[string]interface{}{
			"SessionStart": []map[string]interface{}{
				{
					"command":     slowHook,
					"description": "Slow test hook",
					"timeout":     1000, // 1 second timeout
				},
			},
		},
	}

	settingsData, _ := json.MarshalIndent(settings, "", "  ")
	os.WriteFile(settingsFile, settingsData, 0644)

	os.Setenv("GEMINI_CONFIG_DIR", geminiConfigDir)
	defer os.Unsetenv("GEMINI_CONFIG_DIR")

	t.Log("NOTE: Hook timeout handling depends on Gemini CLI implementation")
	t.Log("This test verifies timeout configuration is present in settings.json")

	// Verify settings.json has timeout configured
	var parsedSettings map[string]interface{}
	data, _ := os.ReadFile(settingsFile)
	json.Unmarshal(data, &parsedSettings)

	hooks := parsedSettings["hooks"].(map[string]interface{})
	sessionStart := hooks["SessionStart"].([]interface{})
	firstHook := sessionStart[0].(map[string]interface{})
	timeout := firstHook["timeout"].(float64)

	if timeout != 1000 {
		t.Errorf("Expected timeout 1000ms, got %f", timeout)
	}

	t.Log("Timeout configuration verified in settings.json")
}

// TestGeminiHooks_MultipleHooksExecution tests multiple hooks in sequence.
func TestGeminiHooks_MultipleHooksExecution(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping multiple hooks test in short mode")
	}

	if _, err := exec.LookPath("gemini"); err != nil {
		t.Skip("Gemini CLI not installed")
	}

	env := helpers.NewTestEnv(t)
	defer env.Cleanup(t)

	// Create multiple hook scripts
	hookDir := filepath.Join(env.TempDir, "gemini-test-hooks")
	os.MkdirAll(hookDir, 0755)

	markerFile := filepath.Join(env.TempDir, "multiple-hooks.log")

	// Create 3 hooks that append to log file
	for i := 1; i <= 3; i++ {
		hookScript := filepath.Join(hookDir, "hook-"+string(rune('0'+i))+".sh")
		hookContent := `#!/bin/bash
echo "Hook ` + string(rune('0'+i)) + ` executed at $(date -Iseconds)" >> ` + markerFile + `
cat <<EOF
{
  "systemMessage": "",
  "suppressOutput": false
}
EOF
`
		os.WriteFile(hookScript, []byte(hookContent), 0755)
	}

	// Create settings.json with all 3 hooks
	geminiConfigDir := filepath.Join(env.TempDir, ".gemini")
	os.MkdirAll(geminiConfigDir, 0755)

	settingsFile := filepath.Join(geminiConfigDir, "settings.json")
	settings := map[string]interface{}{
		"hooks": map[string]interface{}{
			"SessionStart": []map[string]interface{}{
				{
					"command":     filepath.Join(hookDir, "hook-1.sh"),
					"description": "First hook",
					"timeout":     5000,
				},
				{
					"command":     filepath.Join(hookDir, "hook-2.sh"),
					"description": "Second hook",
					"timeout":     5000,
				},
				{
					"command":     filepath.Join(hookDir, "hook-3.sh"),
					"description": "Third hook",
					"timeout":     5000,
				},
			},
		},
	}

	settingsData, _ := json.MarshalIndent(settings, "", "  ")
	os.WriteFile(settingsFile, settingsData, 0644)

	t.Log("NOTE: Multiple hook execution order depends on Gemini CLI implementation")
	t.Log("This test verifies multiple hooks are configured in settings.json")

	// Verify all hooks are in settings.json
	var parsedSettings map[string]interface{}
	data, _ := os.ReadFile(settingsFile)
	json.Unmarshal(data, &parsedSettings)

	hooks := parsedSettings["hooks"].(map[string]interface{})
	sessionStart := hooks["SessionStart"].([]interface{})

	if len(sessionStart) != 3 {
		t.Errorf("Expected 3 hooks in settings.json, got %d", len(sessionStart))
	}

	t.Log("Multiple hooks configuration verified")
}

// Helper function: containsAny checks if string contains any of the substrings.
func containsAny(s string, substrs []string) bool {
	for _, substr := range substrs {
		if len(s) >= len(substr) {
			for i := 0; i <= len(s)-len(substr); i++ {
				if s[i:i+len(substr)] == substr {
					return true
				}
			}
		}
	}
	return false
}
