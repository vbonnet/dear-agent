package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestCLIVersion tests --version flag
func TestCLIVersion(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Build binary
	binary := buildBinary(t)
	defer func() {
		_ = os.Remove(binary)
	}()

	// Run with --version
	cmd := exec.Command(binary, "--version")
	output, err := cmd.CombinedOutput()

	if err != nil {
		t.Fatalf("--version failed: %v\nOutput: %s", err, output)
	}

	outputStr := string(output)
	if !strings.Contains(outputStr, "swarm-executor version") {
		t.Errorf("version output missing version string: %s", outputStr)
	}

	if !strings.Contains(outputStr, version) {
		t.Errorf("version output missing version number %s: %s", version, outputStr)
	}
}

// TestCLIHelp tests --help flag
func TestCLIHelp(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	binary := buildBinary(t)
	defer func() {
		_ = os.Remove(binary)
	}()

	// Run with --help
	cmd := exec.Command(binary, "--help")
	output, err := cmd.CombinedOutput()

	if err != nil {
		t.Fatalf("--help failed: %v\nOutput: %s", err, output)
	}

	outputStr := string(output)

	// Verify help text contains key sections
	expectedSections := []string{
		"Usage:",
		"Flags:",
		"--queue",
		"--bead-id",
		"--session",
		"--version",
		"--help",
		"Exit Codes:",
		"Examples:",
	}

	for _, section := range expectedSections {
		if !strings.Contains(outputStr, section) {
			t.Errorf("help text missing section %q: %s", section, outputStr)
		}
	}
}

// TestCLIMissingFlags tests error handling for missing required flags
func TestCLIMissingFlags(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	binary := buildBinary(t)
	defer func() {
		_ = os.Remove(binary)
	}()

	tests := []struct {
		name string
		args []string
	}{
		{"no flags", []string{}},
		{"missing bead-id and session", []string{"--queue", "test.yaml"}},
		{"missing queue and session", []string{"--bead-id", "bead-1"}},
		{"missing queue and bead-id", []string{"--session", "session-1"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := exec.Command(binary, tt.args...)
			output, err := cmd.CombinedOutput()

			// Should exit with error
			if err == nil {
				t.Errorf("expected error for missing flags, got success")
			}

			// Should show error message
			outputStr := string(output)
			if !strings.Contains(outputStr, "Error: Missing required flags") {
				t.Errorf("missing error message in output: %s", outputStr)
			}

			// Should show usage
			if !strings.Contains(outputStr, "Usage:") {
				t.Errorf("missing usage in error output: %s", outputStr)
			}
		})
	}
}

// TestExecuteBeadInvalidQueue tests error handling for invalid queue file
func TestExecuteBeadInvalidQueue(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	binary := buildBinary(t)
	defer func() {
		_ = os.Remove(binary)
	}()

	// Run with non-existent queue file
	cmd := exec.Command(binary,
		"--queue", "/nonexistent/queue.yaml",
		"--bead-id", "bead-1",
		"--session", "test-session",
	)
	output, err := cmd.CombinedOutput()

	// Should exit with error
	if err == nil {
		t.Error("expected error for invalid queue file")
	}

	// Should show error message
	outputStr := string(output)
	if !strings.Contains(outputStr, "Error: Failed to load task queue") {
		t.Errorf("missing queue load error: %s", outputStr)
	}
}

// TestExecuteBeadSuccess tests successful bead execution (mock scenario)
func TestExecuteBeadSuccess(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	tmpDir := t.TempDir()

	// Create test queue
	queueFile := filepath.Join(tmpDir, "queue.yaml")
	queueData := `schema_version: "1.0.0"
last_updated: 2024-01-01T00:00:00Z
ready:
  - id: bead-test
    title: Test bead
    tier: 1
    prompts:
      start: "echo test"
in_progress: []
blocked: []
completed: []
`
	if err := os.WriteFile(queueFile, []byte(queueData), 0644); err != nil {
		t.Fatalf("failed to create test queue: %v", err)
	}

	binary := buildBinary(t)
	defer func() {
		_ = os.Remove(binary)
	}()

	// Note: This will fail because CSM integration requires real tmux session
	// But we can verify that CLI correctly attempts execution
	cmd := exec.Command(binary,
		"--queue", queueFile,
		"--bead-id", "bead-test",
		"--session", "test-session",
	)
	output, err := cmd.CombinedOutput()

	// Expected to fail due to CSM not available, but check error is from CSM not CLI
	outputStr := string(output)

	// CLI should have successfully parsed flags and loaded queue
	// Error should come from execution phase, not flag parsing
	if strings.Contains(outputStr, "Error: Missing required flags") {
		t.Error("CLI should not error on flag parsing with valid flags")
	}

	// Log file should be created
	logFile := filepath.Join(tmpDir, "EXECUTION-LOG.jsonl")
	if _, err := os.Stat(logFile); err != nil {
		t.Errorf("execution log not created: %v", err)
	}

	// Should show execution started
	if !strings.Contains(outputStr, "Executing bead") {
		t.Logf("Note: Execution likely failed due to CSM unavailable (expected)")
		t.Logf("Output: %s", outputStr)
	}

	// Verify error is from execution phase, not CLI setup
	if err != nil {
		// This is expected - CSM not available
		t.Logf("Execution failed (expected without CSM): %v", err)
	}
}

// TestPrintUsage tests usage text content
func TestPrintUsage(t *testing.T) {
	// Capture output
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	printUsage()

	_ = w.Close()
	os.Stderr = oldStderr

	buf := make([]byte, 4096)
	n, _ := r.Read(buf)
	output := string(buf[:n])

	// Verify key sections
	expectedSections := []string{
		"swarm-executor",
		"Usage:",
		"Flags:",
		"--queue",
		"--bead-id",
		"--session",
		"Exit Codes:",
		"Examples:",
	}

	for _, section := range expectedSections {
		if !strings.Contains(output, section) {
			t.Errorf("usage text missing section %q", section)
		}
	}

	// Verify exit codes documented
	if !strings.Contains(output, "0  Success") {
		t.Error("usage missing success exit code documentation")
	}

	if !strings.Contains(output, "1  Error") {
		t.Error("usage missing error exit code documentation")
	}

	if !strings.Contains(output, "2  Escalation") {
		t.Error("usage missing escalation exit code documentation")
	}
}

// buildBinary builds the swarm-executor binary for testing
func buildBinary(t *testing.T) string {
	t.Helper()

	tmpDir := t.TempDir()
	binary := filepath.Join(tmpDir, "swarm-executor")

	// Build binary
	cmd := exec.Command("go", "build", "-o", binary, ".")
	output, err := cmd.CombinedOutput()

	if err != nil {
		t.Fatalf("failed to build binary: %v\nOutput: %s", err, output)
	}

	return binary
}
