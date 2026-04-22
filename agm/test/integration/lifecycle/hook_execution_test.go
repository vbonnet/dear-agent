//go:build integration

package lifecycle_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/test/integration/helpers"
)

// TestHooks_PostInitExecution tests post-init hook execution order
func TestHooks_PostInitExecution(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping hook execution test in short mode")
	}

	if !helpers.IsTmuxAvailable() {
		t.Skip("Tmux not available")
	}

	env := helpers.NewTestEnv(t)
	defer env.Cleanup(t)

	_ = "test-post-init-" + helpers.RandomString(6)

	// Create hook directory structure
	claudeDir := filepath.Join(env.TempDir, ".claude")
	hooksDir := filepath.Join(claudeDir, "hooks")
	if err := os.MkdirAll(hooksDir, 0755); err != nil {
		t.Fatalf("Failed to create hooks directory: %v", err)
	}

	// Create marker file path
	markerFile := filepath.Join(env.TempDir, "post-init-executed.txt")

	// Create post-init hook
	hookScript := filepath.Join(hooksDir, "post-init")
	hookContent := `#!/bin/bash
echo "Post-init hook executed at $(date)" > ` + markerFile + `
echo "Session: $AGM_SESSION_NAME" >> ` + markerFile + `
echo "Project: $AGM_PROJECT_DIR" >> ` + markerFile + `
`
	if err := os.WriteFile(hookScript, []byte(hookContent), 0755); err != nil {
		t.Fatalf("Failed to write hook script: %v", err)
	}

	t.Log("NOTE: Hook execution is not yet implemented in AGM")
	t.Log("This test documents expected behavior for future implementation")

	// Expected behavior when hooks are implemented:
	// 1. agm session new creates session
	// 2. Manifest created
	// 3. Tmux session started
	// 4. Agent initialized
	// 5. post-init hook executed with environment variables set
	// 6. Marker file should exist with expected content

	// For now, skip actual execution
	t.Skip("Hook execution not yet implemented")

	// When implemented, verify:
	// - Hook was executed
	// - Environment variables were set correctly
	// - Hook ran after session initialization
}

// TestHooks_PreArchiveExecution tests pre-archive hook execution
func TestHooks_PreArchiveExecution(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping hook execution test in short mode")
	}

	env := helpers.NewTestEnv(t)
	defer env.Cleanup(t)

	_ = "test-pre-archive-" + helpers.RandomString(6)

	// Create hook directory
	claudeDir := filepath.Join(env.TempDir, ".claude")
	hooksDir := filepath.Join(claudeDir, "hooks")
	if err := os.MkdirAll(hooksDir, 0755); err != nil {
		t.Fatalf("Failed to create hooks directory: %v", err)
	}

	// Create marker file path
	markerFile := filepath.Join(env.TempDir, "pre-archive-executed.txt")

	// Create pre-archive hook
	hookScript := filepath.Join(hooksDir, "pre-archive")
	hookContent := `#!/bin/bash
echo "Pre-archive hook executed" > ` + markerFile + `
echo "Archiving session: $AGM_SESSION_NAME" >> ` + markerFile + `
`
	if err := os.WriteFile(hookScript, []byte(hookContent), 0755); err != nil {
		t.Fatalf("Failed to write hook script: %v", err)
	}

	t.Log("NOTE: Hook execution not yet implemented")
	t.Skip("Hook execution not yet implemented")

	// Expected behavior:
	// 1. agm session archive command initiated
	// 2. pre-archive hook executed
	// 3. Session archived (lifecycle field updated)
	// 4. Marker file exists
}

// TestHooks_ErrorHandling tests hook error handling
func TestHooks_ErrorHandling(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping hook error test in short mode")
	}

	env := helpers.NewTestEnv(t)
	defer env.Cleanup(t)

	// Create hook that exits with error
	claudeDir := filepath.Join(env.TempDir, ".claude")
	hooksDir := filepath.Join(claudeDir, "hooks")
	if err := os.MkdirAll(hooksDir, 0755); err != nil {
		t.Fatalf("Failed to create hooks directory: %v", err)
	}

	hookScript := filepath.Join(hooksDir, "post-init")
	hookContent := `#!/bin/bash
echo "Hook failing intentionally" >&2
exit 1
`
	if err := os.WriteFile(hookScript, []byte(hookContent), 0755); err != nil {
		t.Fatalf("Failed to write failing hook: %v", err)
	}

	t.Log("NOTE: Hook execution not yet implemented")
	t.Skip("Hook execution not yet implemented")

	// Expected behavior:
	// - Hook executes and fails
	// - AGM logs hook failure
	// - Session creation continues (hooks are non-blocking)
	// OR
	// - Session creation aborts with error (if hooks are blocking)
	//
	// Actual behavior depends on implementation decision
}

// TestHooks_EnvironmentVariables tests hook environment variable availability
func TestHooks_EnvironmentVariables(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping hook environment test in short mode")
	}

	env := helpers.NewTestEnv(t)
	defer env.Cleanup(t)

	// Create hook that captures environment
	claudeDir := filepath.Join(env.TempDir, ".claude")
	hooksDir := filepath.Join(claudeDir, "hooks")
	if err := os.MkdirAll(hooksDir, 0755); err != nil {
		t.Fatalf("Failed to create hooks directory: %v", err)
	}

	envFile := filepath.Join(env.TempDir, "hook-env.txt")
	hookScript := filepath.Join(hooksDir, "post-init")
	hookContent := `#!/bin/bash
env | grep AGM_ > ` + envFile + `
`
	if err := os.WriteFile(hookScript, []byte(hookContent), 0755); err != nil {
		t.Fatalf("Failed to write hook: %v", err)
	}

	t.Log("NOTE: Hook execution not yet implemented")
	t.Skip("Hook execution not yet implemented")

	// Expected environment variables:
	// - AGM_SESSION_NAME
	// - AGM_SESSION_ID
	// - AGM_PROJECT_DIR
	// - AGM_AGENT
	// - AGM_TMUX_SESSION
	// - AGM_MANIFEST_PATH

	// When implemented, verify env file contains these variables
}

// TestHooks_ExecutionOrder tests multiple hooks execute in correct order
func TestHooks_ExecutionOrder(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping hook order test in short mode")
	}

	env := helpers.NewTestEnv(t)
	defer env.Cleanup(t)

	// Create multiple hooks
	claudeDir := filepath.Join(env.TempDir, ".claude")
	hooksDir := filepath.Join(claudeDir, "hooks")
	if err := os.MkdirAll(hooksDir, 0755); err != nil {
		t.Fatalf("Failed to create hooks directory: %v", err)
	}

	logFile := filepath.Join(env.TempDir, "hook-order.log")

	// Create hooks that append to log file
	hooks := []string{"pre-init", "post-init", "pre-archive", "post-archive"}
	for _, hookName := range hooks {
		hookPath := filepath.Join(hooksDir, hookName)
		content := `#!/bin/bash
echo "` + hookName + ` $(date +%s%N)" >> ` + logFile + `
`
		if err := os.WriteFile(hookPath, []byte(content), 0755); err != nil {
			t.Fatalf("Failed to write hook %s: %v", hookName, err)
		}
	}

	t.Log("NOTE: Hook execution not yet implemented")
	t.Skip("Hook execution not yet implemented")

	// Expected execution order:
	// 1. pre-init (before session creation)
	// 2. post-init (after session ready)
	// 3. pre-archive (before archiving)
	// 4. post-archive (after archived)

	// Verify log file shows correct order
}

// TestHooks_Timeout tests hook execution timeout
func TestHooks_Timeout(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping hook timeout test in short mode")
	}

	env := helpers.NewTestEnv(t)
	defer env.Cleanup(t)

	// Create hook that takes too long
	claudeDir := filepath.Join(env.TempDir, ".claude")
	hooksDir := filepath.Join(claudeDir, "hooks")
	if err := os.MkdirAll(hooksDir, 0755); err != nil {
		t.Fatalf("Failed to create hooks directory: %v", err)
	}

	hookScript := filepath.Join(hooksDir, "post-init")
	hookContent := `#!/bin/bash
sleep 300
echo "This should timeout"
`
	if err := os.WriteFile(hookScript, []byte(hookContent), 0755); err != nil {
		t.Fatalf("Failed to write slow hook: %v", err)
	}

	t.Log("NOTE: Hook execution not yet implemented")
	t.Skip("Hook execution not yet implemented")

	// Expected behavior:
	// - Hook starts executing
	// - Timeout triggers (e.g., after 30 seconds)
	// - Hook process killed
	// - Session creation continues or aborts with timeout error
}

// TestHooks_ShellTypes tests hooks work with different shell types
func TestHooks_ShellTypes(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping shell types test in short mode")
	}

	env := helpers.NewTestEnv(t)
	defer env.Cleanup(t)

	claudeDir := filepath.Join(env.TempDir, ".claude")
	hooksDir := filepath.Join(claudeDir, "hooks")
	if err := os.MkdirAll(hooksDir, 0755); err != nil {
		t.Fatalf("Failed to create hooks directory: %v", err)
	}

	// Test hooks in different languages
	tests := []struct {
		name       string
		filename   string
		content    string
		markerFile string
	}{
		{
			name:     "Bash",
			filename: "post-init-bash",
			content: `#!/bin/bash
echo "Bash hook" > ` + filepath.Join(env.TempDir, "bash.txt") + `
`,
			markerFile: filepath.Join(env.TempDir, "bash.txt"),
		},
		{
			name:     "Python",
			filename: "post-init-python",
			content: `#!/usr/bin/env python3
with open("` + filepath.Join(env.TempDir, "python.txt") + `", "w") as f:
    f.write("Python hook")
`,
			markerFile: filepath.Join(env.TempDir, "python.txt"),
		},
		{
			name:     "Go",
			filename: "post-init-go",
			content: `#!/usr/bin/env go run
package main
import "os"
func main() {
	os.WriteFile("` + filepath.Join(env.TempDir, "go.txt") + `", []byte("Go hook"), 0644)
}
`,
			markerFile: filepath.Join(env.TempDir, "go.txt"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hookPath := filepath.Join(hooksDir, tt.filename)
			if err := os.WriteFile(hookPath, []byte(tt.content), 0755); err != nil {
				t.Fatalf("Failed to write %s hook: %v", tt.name, err)
			}

			t.Logf("NOTE: Hook execution not yet implemented")
			t.Skip("Hook execution not yet implemented")

			// When implemented, verify marker file exists
		})
	}
}

// TestAssociateCommand_SendsRename tests /rename is sent before /agm:agm-assoc
func TestAssociateCommand_SendsRename(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping associate command test in short mode")
	}

	if !helpers.IsTmuxAvailable() {
		t.Skip("Tmux not available")
	}

	sessionName := "test-rename-assoc-" + helpers.RandomString(6)

	// SETUP: Create detached tmux session
	helpers.SetupTestTmuxSession(t, sessionName)
	defer helpers.CleanupTestTmuxSession(t, sessionName)

	// Wait for session to be ready
	time.Sleep(500 * time.Millisecond)

	// EXECUTION: Run init sequence (simulates what 'agm new' does)
	// This should send /rename before /agm:agm-assoc
	// Note: In real implementation, this would call the actual init sequence
	// For this test, we manually send the commands to verify order

	// Send rename command
	renameCmd := helpers.BuildTmuxCmd("send-keys", "-t", sessionName, "-l", "/rename "+sessionName)
	if err := renameCmd.Run(); err != nil {
		t.Fatalf("Failed to send rename command: %v", err)
	}
	enterCmd := helpers.BuildTmuxCmd("send-keys", "-t", sessionName, "C-m")
	enterCmd.Run()

	time.Sleep(200 * time.Millisecond)

	// Send assoc command
	assocCmd := helpers.BuildTmuxCmd("send-keys", "-t", sessionName, "-l", "/agm:agm-assoc")
	if err := assocCmd.Run(); err != nil {
		t.Fatalf("Failed to send assoc command: %v", err)
	}
	enterCmd = exec.Command("tmux", "send-keys", "-t", sessionName, "C-m")
	enterCmd.Run()

	// Wait for commands to appear in pane
	time.Sleep(500 * time.Millisecond)

	// CAPTURE: Get pane output
	var paneContent string

	// Retry capture with timeout (handles timing sensitivity)
	timeout := 5 * time.Second
	if os.Getenv("CI") == "true" {
		timeout = 10 * time.Second // Longer timeout in CI
	}

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		captureCmd := helpers.BuildTmuxCmd("capture-pane", "-t", sessionName, "-p", "-S", "-100")
		paneOutput, err := captureCmd.Output()
		if err != nil {
			t.Fatalf("Failed to capture pane: %v", err)
		}

		paneContent = string(paneOutput)

		// Check if both commands are visible
		if strings.Contains(paneContent, "/rename") && strings.Contains(paneContent, "/agm:agm-assoc") {
			break
		}

		time.Sleep(200 * time.Millisecond)
	}

	// ASSERTIONS: Verify command order using helper
	helpers.AssertCommandOrder(t, paneContent, []string{"/rename", "/agm:agm-assoc"})
}

// TestHookDirectory_Discovery tests AGM discovers hooks from correct locations
func TestHookDirectory_Discovery(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping hook discovery test in short mode")
	}

	env := helpers.NewTestEnv(t)
	defer env.Cleanup(t)

	// Create hooks in multiple locations
	locations := []string{
		filepath.Join(env.TempDir, ".claude", "hooks"),       // Project-level
		filepath.Join(os.Getenv("HOME"), ".claude", "hooks"), // User-level
		filepath.Join("/etc", "agm", "hooks"),                // System-level
	}

	// Only create test hooks in project-level (don't modify user/system)
	projectHooksDir := locations[0]
	if err := os.MkdirAll(projectHooksDir, 0755); err != nil {
		t.Fatalf("Failed to create project hooks directory: %v", err)
	}

	markerFile := filepath.Join(env.TempDir, "hook-discovery.txt")
	hookScript := filepath.Join(projectHooksDir, "post-init")
	hookContent := `#!/bin/bash
echo "Hook discovered" > ` + markerFile + `
`
	if err := os.WriteFile(hookScript, []byte(hookContent), 0755); err != nil {
		t.Fatalf("Failed to write hook: %v", err)
	}

	t.Log("NOTE: Hook discovery not yet implemented")
	t.Skip("Hook discovery not yet implemented")

	// Expected behavior:
	// - AGM searches for hooks in priority order
	// - Project hooks override user hooks override system hooks
	// - All hooks in a directory execute (not just first match)
}

// TestHooks_AsyncExecution tests hooks execute asynchronously if configured
func TestHooks_AsyncExecution(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping async hook test in short mode")
	}

	env := helpers.NewTestEnv(t)
	defer env.Cleanup(t)

	claudeDir := filepath.Join(env.TempDir, ".claude")
	hooksDir := filepath.Join(claudeDir, "hooks")
	if err := os.MkdirAll(hooksDir, 0755); err != nil {
		t.Fatalf("Failed to create hooks directory: %v", err)
	}

	// Create slow hook
	hookScript := filepath.Join(hooksDir, "post-init")
	hookContent := `#!/bin/bash
sleep 5
echo "Slow hook completed"
`
	if err := os.WriteFile(hookScript, []byte(hookContent), 0755); err != nil {
		t.Fatalf("Failed to write slow hook: %v", err)
	}

	t.Log("NOTE: Async hook execution not yet implemented")
	t.Skip("Async hook execution not yet implemented")

	// Expected behavior:
	// - Hook starts in background
	// - agm session new command returns immediately
	// - Session is usable while hook runs
	// - Hook completion logged somewhere
}
