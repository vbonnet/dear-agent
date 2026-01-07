package config

import (
	"os"
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.QueueFilePath != "TASK-QUEUE.yaml" {
		t.Errorf("expected queue file 'TASK-QUEUE.yaml', got %s", cfg.QueueFilePath)
	}
	if cfg.MaxIterations != 3 {
		t.Errorf("expected max iterations 3, got %d", cfg.MaxIterations)
	}
	if cfg.SessionTimeout != 1*time.Hour {
		t.Errorf("expected session timeout 1h, got %v", cfg.SessionTimeout)
	}
	if cfg.TestCoverageThreshold != 0.80 {
		t.Errorf("expected coverage threshold 0.80, got %f", cfg.TestCoverageThreshold)
	}
}

func TestLoadFromEnv(t *testing.T) {
	// Set environment variables
	if err := os.Setenv("SWARM_QUEUE_FILE", "custom-queue.yaml"); err != nil {
		t.Fatalf("failed to set env var: %v", err)
	}
	if err := os.Setenv("SWARM_MAX_ITERATIONS", "5"); err != nil {
		t.Fatalf("failed to set env var: %v", err)
	}
	if err := os.Setenv("SWARM_SESSION_TIMEOUT", "2h"); err != nil {
		t.Fatalf("failed to set env var: %v", err)
	}
	if err := os.Setenv("SWARM_TEST_COVERAGE_THRESHOLD", "0.90"); err != nil {
		t.Fatalf("failed to set env var: %v", err)
	}
	defer func() {
		_ = os.Unsetenv("SWARM_QUEUE_FILE")
		_ = os.Unsetenv("SWARM_MAX_ITERATIONS")
		_ = os.Unsetenv("SWARM_SESSION_TIMEOUT")
		_ = os.Unsetenv("SWARM_TEST_COVERAGE_THRESHOLD")
	}()

	cfg := LoadFromEnv()

	if cfg.QueueFilePath != "custom-queue.yaml" {
		t.Errorf("expected queue file 'custom-queue.yaml', got %s", cfg.QueueFilePath)
	}
	if cfg.MaxIterations != 5 {
		t.Errorf("expected max iterations 5, got %d", cfg.MaxIterations)
	}
	if cfg.SessionTimeout != 2*time.Hour {
		t.Errorf("expected session timeout 2h, got %v", cfg.SessionTimeout)
	}
	if cfg.TestCoverageThreshold != 0.90 {
		t.Errorf("expected coverage threshold 0.90, got %f", cfg.TestCoverageThreshold)
	}
}

func TestValidate_ValidConfig(t *testing.T) {
	cfg := DefaultConfig()
	if err := cfg.Validate(); err != nil {
		t.Errorf("expected valid config, got error: %v", err)
	}
}

func TestValidate_EmptyQueueFilePath(t *testing.T) {
	cfg := DefaultConfig()
	cfg.QueueFilePath = ""
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for empty queue file path")
	}
}

func TestValidate_InvalidMaxIterations(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MaxIterations = 0
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for max iterations = 0")
	}
}

func TestValidate_InvalidSessionTimeout(t *testing.T) {
	cfg := DefaultConfig()
	cfg.SessionTimeout = 0
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for session timeout = 0")
	}
}

func TestValidate_InvalidCoverageThreshold(t *testing.T) {
	tests := []struct {
		name      string
		threshold float64
	}{
		{"negative", -0.1},
		{"greater than 1", 1.5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.TestCoverageThreshold = tt.threshold
			if err := cfg.Validate(); err == nil {
				t.Errorf("expected error for coverage threshold %f", tt.threshold)
			}
		})
	}
}
