package scratchpad

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// Note: These tests require Docker to be installed and running
// Run with: go test -v -tags=integration

func skipWithoutDocker(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("Skipping Docker integration test in short mode")
	}
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("Skipping: Docker not installed")
	}
}

func TestNewSandbox(t *testing.T) {
	skipWithoutDocker(t)

	ctx := context.Background()
	config := &SandboxConfig{
		Image:         "python:3.11-slim",
		MaxExecutions: 10,
		MemoryLimit:   "512m",
		Timeout:       30 * time.Second,
	}

	sandbox, err := NewSandbox(ctx, config)
	if err != nil {
		t.Fatalf("NewSandbox failed: %v", err)
	}
	defer sandbox.Cleanup(ctx)

	if sandbox.containerID == "" {
		t.Error("Container ID is empty")
	}

	if sandbox.workdir == "" {
		t.Error("Workdir is empty")
	}

	if sandbox.maxExecutions != 10 {
		t.Errorf("MaxExecutions = %d, want 10", sandbox.maxExecutions)
	}
}

func TestExecute_Python(t *testing.T) {
	skipWithoutDocker(t)

	ctx := context.Background()
	sandbox, err := NewSandbox(ctx, &SandboxConfig{
		Image:         "python:3.11-slim",
		MaxExecutions: 10,
		MemoryLimit:   "512m",
		Timeout:       30 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewSandbox failed: %v", err)
	}
	defer sandbox.Cleanup(ctx)

	// Test: Execute simple Python script
	response, err := sandbox.Execute(ctx, &ExecuteRequest{
		Language: "python",
		Code:     "print('Hello from sandbox')",
		Timeout:  5 * time.Second,
	})

	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if response.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0", response.ExitCode)
	}

	if !strings.Contains(response.Stdout, "Hello from sandbox") {
		t.Errorf("Stdout = %q, want to contain 'Hello from sandbox'", response.Stdout)
	}

	if response.Error != "" {
		t.Errorf("Unexpected error: %s", response.Error)
	}
}

func TestExecute_Python_JSON(t *testing.T) {
	skipWithoutDocker(t)

	ctx := context.Background()
	sandbox, err := NewSandbox(ctx, &SandboxConfig{
		Image:         "python:3.11-slim",
		MaxExecutions: 10,
		MemoryLimit:   "512m",
		Timeout:       30 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewSandbox failed: %v", err)
	}
	defer sandbox.Cleanup(ctx)

	code := `
import json
data = {"status": "working", "version": "1.0"}
print(json.dumps(data))
`

	response, err := sandbox.Execute(ctx, &ExecuteRequest{
		Language: "python",
		Code:     code,
		Timeout:  5 * time.Second,
	})

	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if response.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0", response.ExitCode)
	}

	if !strings.Contains(response.Stdout, `"status": "working"`) {
		t.Errorf("Stdout = %q, want JSON output", response.Stdout)
	}
}

func TestExecute_Bash(t *testing.T) {
	skipWithoutDocker(t)

	ctx := context.Background()
	sandbox, err := NewSandbox(ctx, &SandboxConfig{
		Image:         "python:3.11-slim", // Has bash
		MaxExecutions: 10,
		MemoryLimit:   "512m",
		Timeout:       30 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewSandbox failed: %v", err)
	}
	defer sandbox.Cleanup(ctx)

	code := `#!/bin/bash
echo "Testing bash"
echo "Exit code test"
exit 0
`

	response, err := sandbox.Execute(ctx, &ExecuteRequest{
		Language: "bash",
		Code:     code,
		Timeout:  5 * time.Second,
	})

	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if response.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0", response.ExitCode)
	}

	if !strings.Contains(response.Stdout, "Testing bash") {
		t.Errorf("Stdout = %q, want to contain 'Testing bash'", response.Stdout)
	}
}

func TestExecute_Error(t *testing.T) {
	skipWithoutDocker(t)

	ctx := context.Background()
	sandbox, err := NewSandbox(ctx, &SandboxConfig{
		Image:         "python:3.11-slim",
		MaxExecutions: 10,
		MemoryLimit:   "512m",
		Timeout:       30 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewSandbox failed: %v", err)
	}
	defer sandbox.Cleanup(ctx)

	// Test: Python script with error
	response, err := sandbox.Execute(ctx, &ExecuteRequest{
		Language: "python",
		Code:     "import nonexistent_module",
		Timeout:  5 * time.Second,
	})

	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if response.ExitCode == 0 {
		t.Error("Expected non-zero exit code for error")
	}

	if !strings.Contains(response.Stderr, "ModuleNotFoundError") && !strings.Contains(response.Stderr, "No module named") {
		t.Errorf("Stderr = %q, want to contain module error", response.Stderr)
	}
}

func TestExecute_Timeout(t *testing.T) {
	skipWithoutDocker(t)

	ctx := context.Background()
	sandbox, err := NewSandbox(ctx, &SandboxConfig{
		Image:         "python:3.11-slim",
		MaxExecutions: 10,
		MemoryLimit:   "512m",
		Timeout:       30 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewSandbox failed: %v", err)
	}
	defer sandbox.Cleanup(ctx)

	// Test: Script that sleeps longer than timeout
	code := `
import time
time.sleep(10)
print("Should not reach here")
`

	response, err := sandbox.Execute(ctx, &ExecuteRequest{
		Language: "python",
		Code:     code,
		Timeout:  1 * time.Second,
	})

	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	// Should timeout
	if response.ExitCode == 0 {
		t.Error("Expected non-zero exit code for timeout")
	}

	if response.Duration < 1*time.Second {
		t.Errorf("Duration = %v, expected at least 1 second", response.Duration)
	}
}

func TestExecute_ExecutionLimit(t *testing.T) {
	skipWithoutDocker(t)

	ctx := context.Background()
	sandbox, err := NewSandbox(ctx, &SandboxConfig{
		Image:         "python:3.11-slim",
		MaxExecutions: 3, // Small limit
		MemoryLimit:   "512m",
		Timeout:       30 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewSandbox failed: %v", err)
	}
	defer sandbox.Cleanup(ctx)

	// Execute 3 times successfully
	for i := 0; i < 3; i++ {
		_, err := sandbox.Execute(ctx, &ExecuteRequest{
			Language: "python",
			Code:     "print('test')",
			Timeout:  5 * time.Second,
		})
		if err != nil {
			t.Fatalf("Execute %d failed: %v", i+1, err)
		}
	}

	// 4th execution should fail
	_, err = sandbox.Execute(ctx, &ExecuteRequest{
		Language: "python",
		Code:     "print('should fail')",
		Timeout:  5 * time.Second,
	})

	if err == nil {
		t.Error("Expected error for execution limit exceeded")
	}

	if !strings.Contains(err.Error(), "execution limit reached") {
		t.Errorf("Error = %v, want execution limit error", err)
	}
}

func TestExecute_UnsupportedLanguage(t *testing.T) {
	skipWithoutDocker(t)

	ctx := context.Background()
	sandbox, err := NewSandbox(ctx, &SandboxConfig{
		Image:         "python:3.11-slim",
		MaxExecutions: 10,
		MemoryLimit:   "512m",
		Timeout:       30 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewSandbox failed: %v", err)
	}
	defer sandbox.Cleanup(ctx)

	// Test: Unsupported language
	_, err = sandbox.Execute(ctx, &ExecuteRequest{
		Language: "ruby",
		Code:     "puts 'hello'",
		Timeout:  5 * time.Second,
	})

	if err == nil {
		t.Fatal("Expected error for unsupported language, got nil")
	}

	if !strings.Contains(err.Error(), "unsupported language") {
		t.Errorf("Error = %v, want unsupported language error", err)
	}
}

func TestCleanup(t *testing.T) {
	skipWithoutDocker(t)

	ctx := context.Background()
	sandbox, err := NewSandbox(ctx, &SandboxConfig{
		Image:         "python:3.11-slim",
		MaxExecutions: 10,
		MemoryLimit:   "512m",
		Timeout:       30 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewSandbox failed: %v", err)
	}

	containerID := sandbox.containerID
	workdir := sandbox.workdir

	// Cleanup
	if err := sandbox.Cleanup(ctx); err != nil {
		t.Fatalf("Cleanup failed: %v", err)
	}

	// Verify container removed (docker ps -a should not list it)
	// This is hard to verify without docker CLI, so we just check no error

	// Verify workdir removed
	if _, err := os.Stat(workdir); !os.IsNotExist(err) {
		t.Errorf("Workdir still exists after cleanup: %s", workdir)
	}

	// Verify containerID was set
	if containerID == "" {
		t.Error("ContainerID was empty before cleanup")
	}
}

func TestGetFileExtension(t *testing.T) {
	tests := []struct {
		language string
		want     string
	}{
		{"python", ".py"},
		{"bash", ".sh"},
		{"node", ".js"},
		{"unknown", ".txt"},
	}

	for _, tt := range tests {
		got := getFileExtension(tt.language)
		if got != tt.want {
			t.Errorf("getFileExtension(%q) = %q, want %q", tt.language, got, tt.want)
		}
	}
}
