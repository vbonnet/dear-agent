package benchmark

import (
	"os/exec"
	"testing"
	"time"
)

// Helper to set up git repo for testing (same as common package)
func setupGitRepo(t *testing.T) (string, func()) {
	t.Helper()

	tmpDir := t.TempDir()
	t.Chdir(tmpDir)

	cmd := exec.Command("git", "init")
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to init git repo: %v", err)
	}

	exec.Command("git", "config", "user.name", "Test User").Run()
	exec.Command("git", "config", "user.email", "test@example.com").Run()

	cleanup := func() {}

	return tmpDir, cleanup
}

func TestExecutor_Run_Success(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	_, cleanup := setupGitRepo(t)
	defer cleanup()

	// Create a simple mock hook that succeeds quickly
	hookCmd := "echo 'test hook'"

	executor := NewExecutor(hookCmd)
	executor.Runs = 5
	executor.WarmupRuns = 1
	executor.Scenario = "empty" // No files to stage

	result, err := executor.Run()
	if err != nil {
		t.Fatalf("Run() failed: %v", err)
	}

	if result == nil {
		t.Fatal("result should not be nil")
	}

	if len(result.Timings) != 5 {
		t.Errorf("expected 5 timings, got %d", len(result.Timings))
	}

	if result.MedianMS <= 0 {
		t.Errorf("median should be > 0, got %f", result.MedianMS)
	}

	if result.CVPercent < 0 {
		t.Errorf("CV%% should be >= 0, got %f", result.CVPercent)
	}
}

func TestExecutor_Run_Timeout(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	_, cleanup := setupGitRepo(t)
	defer cleanup()

	// Create a hook that sleeps longer than timeout
	hookCmd := "sleep 2"

	executor := NewExecutor(hookCmd)
	executor.Runs = 1
	executor.WarmupRuns = 0
	executor.Scenario = "empty"
	executor.Timeout = 100 * time.Millisecond // Very short timeout

	result, err := executor.Run()
	if err != nil {
		t.Fatalf("Run() failed: %v", err)
	}

	// Should have timeout errors
	if len(result.Errors) == 0 {
		t.Error("expected timeout errors, got none")
	}
}

func TestExecutor_Run_WithFiles(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	_, cleanup := setupGitRepo(t)
	defer cleanup()

	hookCmd := "true" // Always succeeds

	executor := NewExecutor(hookCmd)
	executor.Runs = 3
	executor.WarmupRuns = 1
	executor.Scenario = "small" // Stage 10 files

	result, err := executor.Run()
	if err != nil {
		t.Fatalf("Run() failed: %v", err)
	}

	if len(result.Timings) != 3 {
		t.Errorf("expected 3 timings, got %d", len(result.Timings))
	}

	// Verify scenario was recorded
	if result.Scenario != "small" {
		t.Errorf("expected scenario 'small', got %q", result.Scenario)
	}
}

func TestCalculateStats(t *testing.T) {
	result := &BenchmarkResult{
		Timings: []time.Duration{
			10 * time.Millisecond,
			20 * time.Millisecond,
			30 * time.Millisecond,
		},
	}

	err := calculateStats(result)
	if err != nil {
		t.Fatalf("calculateStats() failed: %v", err)
	}

	if result.MedianMS != 20.0 {
		t.Errorf("median = %f, want 20.0", result.MedianMS)
	}

	if result.MeanMS != 20.0 {
		t.Errorf("mean = %f, want 20.0", result.MeanMS)
	}

	if result.MinMS != 10.0 {
		t.Errorf("min = %f, want 10.0", result.MinMS)
	}

	if result.MaxMS != 30.0 {
		t.Errorf("max = %f, want 30.0", result.MaxMS)
	}
}
