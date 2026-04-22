package hooks

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestExecutorValidateCommand(t *testing.T) {
	validator := NewCommandValidator()
	executor := NewExecutor(validator)

	hook := Hook{
		Name:     "test-hook",
		Event:    HookEventSessionCompletion,
		Priority: 10,
		Type:     HookTypeBinary,
		Command:  "bash", // Not in allowlist
		Timeout:  5,
	}

	ctx := context.Background()
	result, err := executor.Execute(ctx, hook)

	if err == nil {
		t.Error("Expected error for disallowed command, got nil")
	}

	if result == nil {
		t.Fatal("Expected result even on security error")
	}

	if result.Status != VerificationStatusFail {
		t.Errorf("Expected status fail, got %s", result.Status)
	}

	if len(result.Violations) == 0 {
		t.Error("Expected security violation")
	}

	if result.Violations[0].Severity != "high" {
		t.Error("Expected high severity for security violation")
	}
}

func TestExecutorHashVerification(t *testing.T) {
	validator := NewCommandValidator()
	executor := NewExecutor(validator)

	// Add echo to allowlist
	validator.AddCommand("echo")

	// Calculate actual hash for echo
	actualHash, err := validator.CalculateCommandHash("echo")
	if err != nil {
		t.Skipf("Skipping test: echo not found")
	}

	// Test with correct hash
	hook := Hook{
		Name:        "test-hook",
		Event:       HookEventSessionCompletion,
		Priority:    10,
		Type:        HookTypeBinary,
		Command:     "echo",
		Args:        []string{"test"},
		Timeout:     5,
		CommandHash: actualHash,
	}

	ctx := context.Background()
	_, err = executor.Execute(ctx, hook)
	if err != nil {
		t.Errorf("Execute with correct hash should succeed, got error: %v", err)
	}

	// Test with incorrect hash
	hook.CommandHash = "0000000000000000000000000000000000000000000000000000000000000000"
	result, err := executor.Execute(ctx, hook)
	if err == nil {
		t.Error("Expected error for incorrect hash, got nil")
	}

	if result.Status != VerificationStatusFail {
		t.Errorf("Expected fail status for hash mismatch, got %s", result.Status)
	}
}

func TestExecutorTimeout(t *testing.T) {
	validator := NewCommandValidator()
	executor := NewExecutor(validator)

	// Add sleep to allowlist
	validator.AddCommand("sleep")

	hook := Hook{
		Name:     "timeout-test",
		Event:    HookEventSessionCompletion,
		Priority: 10,
		Type:     HookTypeBinary,
		Command:  "sleep",
		Args:     []string{"10"}, // Sleep for 10 seconds
		Timeout:  1,              // 1 second timeout
	}

	ctx := context.Background()
	result, err := executor.Execute(ctx, hook)

	if err != ErrTimeout {
		t.Errorf("Expected ErrTimeout, got: %v", err)
	}

	if result == nil {
		t.Fatal("Expected result even on timeout")
	}

	if result.Status != VerificationStatusWarning {
		t.Errorf("Expected warning status for timeout, got %s", result.Status)
	}

	if result.Duration < time.Second || result.Duration > 2*time.Second {
		t.Errorf("Expected duration around 1s, got %v", result.Duration)
	}
}

func TestExecutorJSONParsing(t *testing.T) {
	validator := NewCommandValidator()
	executor := NewExecutor(validator)

	// Create a script that outputs valid JSON
	tmpDir := t.TempDir()
	scriptPath := filepath.Join(tmpDir, "json-script.sh")

	result := VerificationResult{
		Status: VerificationStatusPass,
		Violations: []Violation{
			{
				Severity:   "low",
				Message:    "Test violation",
				Suggestion: "Fix it",
			},
		},
	}
	resultJSON, _ := json.Marshal(result)
	scriptContent := "#!/bin/bash\necho '" + string(resultJSON) + "'\n"

	if err := os.WriteFile(scriptPath, []byte(scriptContent), 0755); err != nil {
		t.Fatalf("Failed to create test script: %v", err)
	}

	validator.AddCommand("bash")

	hook := Hook{
		Name:     "json-test",
		Event:    HookEventSessionCompletion,
		Priority: 10,
		Type:     HookTypeScript,
		Command:  "bash",
		Args:     []string{scriptPath},
		Timeout:  5,
	}

	ctx := context.Background()
	res, err := executor.Execute(ctx, hook)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if res.Status != VerificationStatusPass {
		t.Errorf("Expected pass status, got %s", res.Status)
	}

	if len(res.Violations) != 1 {
		t.Errorf("Expected 1 violation, got %d", len(res.Violations))
	}

	if res.HookName != "json-test" {
		t.Errorf("Expected hook name json-test, got %s", res.HookName)
	}
}

func TestExecutorNonJSONOutput(t *testing.T) {
	validator := NewCommandValidator()
	executor := NewExecutor(validator)

	validator.AddCommand("echo")

	// Test with non-JSON output (should create result from exit code)
	hook := Hook{
		Name:     "non-json-test",
		Event:    HookEventSessionCompletion,
		Priority: 10,
		Type:     HookTypeBinary,
		Command:  "echo",
		Args:     []string{"plain text output"},
		Timeout:  5,
	}

	ctx := context.Background()
	result, err := executor.Execute(ctx, hook)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	// Non-JSON output with exit code 0 should be pass
	if result.Status != VerificationStatusPass {
		t.Errorf("Expected pass status for exit code 0, got %s", result.Status)
	}

	if result.ExitCode != 0 {
		t.Errorf("Expected exit code 0, got %d", result.ExitCode)
	}
}

func TestExecutorNonZeroExitCode(t *testing.T) {
	validator := NewCommandValidator()
	executor := NewExecutor(validator)

	// Create a script that exits with non-zero code
	tmpDir := t.TempDir()
	scriptPath := filepath.Join(tmpDir, "fail-script.sh")
	scriptContent := "#!/bin/bash\nexit 42\n"
	if err := os.WriteFile(scriptPath, []byte(scriptContent), 0755); err != nil {
		t.Fatalf("Failed to create test script: %v", err)
	}

	validator.AddCommand("bash")

	hook := Hook{
		Name:     "fail-test",
		Event:    HookEventSessionCompletion,
		Priority: 10,
		Type:     HookTypeScript,
		Command:  "bash",
		Args:     []string{scriptPath},
		Timeout:  5,
	}

	ctx := context.Background()
	result, err := executor.Execute(ctx, hook)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if result.Status != VerificationStatusFail {
		t.Errorf("Expected fail status for non-zero exit code, got %s", result.Status)
	}

	if result.ExitCode != 42 {
		t.Errorf("Expected exit code 42, got %d", result.ExitCode)
	}

	if len(result.Violations) == 0 {
		t.Error("Expected violation for non-zero exit code")
	}
}

func TestExecutorDefaultTimeout(t *testing.T) {
	validator := NewCommandValidator()
	executor := NewExecutor(validator)

	validator.AddCommand("echo")

	// Hook with timeout 0 should use default (60s)
	hook := Hook{
		Name:     "default-timeout-test",
		Event:    HookEventSessionCompletion,
		Priority: 10,
		Type:     HookTypeBinary,
		Command:  "echo",
		Args:     []string{"test"},
		Timeout:  0, // Should use default
	}

	ctx := context.Background()
	result, err := executor.Execute(ctx, hook)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if result.Status != VerificationStatusPass {
		t.Errorf("Expected pass status, got %s", result.Status)
	}

	// Duration should be much less than default timeout
	if result.Duration > time.Second {
		t.Errorf("Expected fast execution, got %v", result.Duration)
	}
}

func TestParseHookOutput(t *testing.T) {
	tests := []struct {
		name       string
		output     string
		wantErr    bool
		wantStatus VerificationStatus
	}{
		{
			name:       "valid JSON",
			output:     `{"status":"pass","violations":[]}`,
			wantErr:    false,
			wantStatus: VerificationStatusPass,
		},
		{
			name:       "valid JSON with violations",
			output:     `{"status":"fail","violations":[{"severity":"high","message":"test"}]}`,
			wantErr:    false,
			wantStatus: VerificationStatusFail,
		},
		{
			name:    "invalid JSON",
			output:  `not json`,
			wantErr: true,
		},
		{
			name:    "empty output",
			output:  ``,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseHookOutput([]byte(tt.output))
			if (err != nil) != tt.wantErr {
				t.Errorf("parseHookOutput() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if result.Status != tt.wantStatus {
					t.Errorf("Expected status %s, got %s", tt.wantStatus, result.Status)
				}
			}
		})
	}
}
