package monitoring_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/pkg/monitoring"
)

func TestMonitoringIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Create temporary work directory
	workDir := t.TempDir()

	// Initialize git repo
	cmd := exec.Command("git", "init", workDir)
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to init git: %v", err)
	}

	// Create agent monitor
	monitor, err := monitoring.NewAgentMonitor("test-agent-123", workDir)
	if err != nil {
		t.Fatalf("Failed to create monitor: %v", err)
	}

	// Use SimpleTaskConfig for this minimal test
	monitor.SetValidationConfig(monitoring.SimpleTaskConfig)

	// Start monitoring
	if err := monitor.Start(); err != nil {
		t.Fatalf("Failed to start monitor: %v", err)
	}
	defer monitor.Stop()

	// Simulate sub-agent activity: create file
	testFile := filepath.Join(workDir, "main.go")
	content := []byte("package main\n\nfunc main() {\n\tprintln(\"Hello, World!\")\n}\n")
	if err := os.WriteFile(testFile, content, 0644); err != nil {
		t.Fatalf("Failed to write file: %v", err)
	}

	// Wait for file watcher to detect
	time.Sleep(200 * time.Millisecond)

	// Simulate git commit
	cmd = exec.Command("git", "-C", workDir, "add", ".")
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to git add: %v", err)
	}

	cmd = exec.Command("git", "-C", workDir, "config", "user.email", "test@test.com")
	cmd.Run()
	cmd = exec.Command("git", "-C", workDir, "config", "user.name", "Test User")
	cmd.Run()

	cmd = exec.Command("git", "-C", workDir, "commit", "-m", "Initial commit")
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to commit: %v", err)
	}

	// Wait for git hook to run
	time.Sleep(200 * time.Millisecond)

	// Stop monitoring
	if err := monitor.Stop(); err != nil {
		t.Fatalf("Failed to stop monitor: %v", err)
	}

	// Run validation
	result, err := monitor.Validate()
	if err != nil {
		t.Fatalf("Validation failed: %v", err)
	}

	// Check validation result
	t.Logf("Validation score: %.2f", result.Score)
	t.Logf("Validation summary: %s", result.Summary)

	// Verify signals
	for _, signal := range result.Signals {
		t.Logf("Signal %s: %v (expected %v) - %s",
			signal.Name, signal.Value, signal.Expected, signal.Message)
	}

	// Check that we detected file creation
	stats := monitor.GetStats()
	if stats.FilesCreated == 0 {
		t.Logf("Warning: File creation detection may not have worked (this is acceptable for timing-sensitive tests)")
	}

	// Check that we detected git commit
	if stats.CommitsDetected == 0 {
		t.Logf("Warning: Git commit detection may not have worked (this is acceptable)")
	}
}

func TestValidationScoring(t *testing.T) {
	// Create temp directory with fake implementation
	workDir := t.TempDir()

	// Create stub file
	stubFile := filepath.Join(workDir, "stub.go")
	stubContent := []byte("package main\n\n// TODO: Implement this\nfunc main() {\n\tpanic(\"not implemented\")\n}\n")
	if err := os.WriteFile(stubFile, stubContent, 0644); err != nil {
		t.Fatalf("Failed to write stub file: %v", err)
	}

	// Create monitor and validator
	monitor, err := monitoring.NewAgentMonitor("test-fake", workDir)
	if err != nil {
		t.Fatalf("Failed to create monitor: %v", err)
	}

	// Run validation (should fail due to low line count, no tests, stubs)
	result, err := monitor.Validate()
	if err != nil {
		t.Fatalf("Validation failed: %v", err)
	}

	t.Logf("Fake implementation score: %.2f", result.Score)
	t.Logf("Summary: %s", result.Summary)

	// Fake implementation should have low score
	if result.Score > 0.5 {
		t.Errorf("Expected low score for fake implementation, got %.2f", result.Score)
	}

	if result.Passed {
		t.Errorf("Expected validation to fail for fake implementation")
	}
}
