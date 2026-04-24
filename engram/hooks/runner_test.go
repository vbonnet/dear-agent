package hooks

import (
	"context"
	"testing"
	"time"
)

func TestRunnerRunHook(t *testing.T) {
	registry := NewRegistry()
	validator := NewCommandValidator()
	runner := NewRunner(registry, validator)

	validator.AddCommand("echo")

	hook := Hook{
		Name:     "test-hook",
		Event:    HookEventSessionCompletion,
		Priority: 10,
		Type:     HookTypeBinary,
		Command:  "echo",
		Args:     []string{"test"},
		Timeout:  5,
	}

	ctx := context.Background()
	result, err := runner.RunHook(ctx, hook)
	if err != nil {
		t.Fatalf("RunHook failed: %v", err)
	}

	if result == nil {
		t.Fatal("Expected result, got nil")
	}

	if result.HookName != "test-hook" {
		t.Errorf("Expected hook name test-hook, got %s", result.HookName)
	}
}

func TestRunnerRunAll(t *testing.T) {
	registry := NewRegistry()
	validator := NewCommandValidator()
	runner := NewRunner(registry, validator)

	validator.AddCommand("echo")

	// Register multiple hooks
	hooks := []Hook{
		{
			Name:     "hook-1",
			Event:    HookEventSessionCompletion,
			Priority: 10,
			Type:     HookTypeBinary,
			Command:  "echo",
			Args:     []string{"hook1"},
			Timeout:  5,
		},
		{
			Name:     "hook-2",
			Event:    HookEventSessionCompletion,
			Priority: 5,
			Type:     HookTypeBinary,
			Command:  "echo",
			Args:     []string{"hook2"},
			Timeout:  5,
		},
		{
			Name:     "hook-3",
			Event:    HookEventPhaseCompletion, // Different event
			Priority: 15,
			Type:     HookTypeBinary,
			Command:  "echo",
			Args:     []string{"hook3"},
			Timeout:  5,
		},
	}

	for _, hook := range hooks {
		if err := registry.Register(hook); err != nil {
			t.Fatalf("Failed to register hook %s: %v", hook.Name, err)
		}
	}

	ctx := context.Background()
	report, err := runner.RunAll(ctx, HookEventSessionCompletion)
	if err != nil {
		t.Fatalf("RunAll failed: %v", err)
	}

	// Should execute 2 hooks (hook-1 and hook-2)
	if len(report.Results) != 2 {
		t.Errorf("Expected 2 results, got %d", len(report.Results))
	}

	// Verify summary
	if report.Summary.TotalHooks != 2 {
		t.Errorf("Expected 2 total hooks, got %d", report.Summary.TotalHooks)
	}

	if report.Summary.PassedHooks != 2 {
		t.Errorf("Expected 2 passed hooks, got %d", report.Summary.PassedHooks)
	}

	if report.Summary.ExitCode != 0 {
		t.Errorf("Expected exit code 0, got %d", report.Summary.ExitCode)
	}
}

func TestRunnerNoHooks(t *testing.T) {
	registry := NewRegistry()
	validator := NewCommandValidator()
	runner := NewRunner(registry, validator)

	ctx := context.Background()
	report, err := runner.RunAll(ctx, HookEventSessionCompletion)
	if err != nil {
		t.Fatalf("RunAll failed: %v", err)
	}

	if len(report.Results) != 0 {
		t.Errorf("Expected 0 results, got %d", len(report.Results))
	}

	if report.Summary.TotalHooks != 0 {
		t.Errorf("Expected 0 total hooks, got %d", report.Summary.TotalHooks)
	}

	if report.Summary.ExitCode != 0 {
		t.Errorf("Expected exit code 0 for no hooks, got %d", report.Summary.ExitCode)
	}
}

func TestRunnerGracefulDegradation(t *testing.T) {
	registry := NewRegistry()
	validator := NewCommandValidator()
	runner := NewRunner(registry, validator)

	// Register a hook with disallowed command (should fail gracefully)
	hook := Hook{
		Name:     "bad-hook",
		Event:    HookEventSessionCompletion,
		Priority: 10,
		Type:     HookTypeBinary,
		Command:  "bash", // Not in allowlist
		Timeout:  5,
	}

	if err := registry.Register(hook); err != nil {
		t.Fatalf("Failed to register hook: %v", err)
	}

	ctx := context.Background()
	report, err := runner.RunAll(ctx, HookEventSessionCompletion)
	if err != nil {
		t.Fatalf("RunAll should not fail on hook errors: %v", err)
	}

	// Should have warnings for failed hook
	if len(report.Warnings) == 0 {
		t.Error("Expected warnings for failed hook")
	}

	// Should still have a result (security violation result)
	if len(report.Results) == 0 {
		t.Error("Expected result even for security violation")
	}

	// Exit code should be 1 (failure)
	if report.Summary.ExitCode != 1 {
		t.Errorf("Expected exit code 1 for failed hook, got %d", report.Summary.ExitCode)
	}
}

func TestRunnerParallelExecution(t *testing.T) {
	registry := NewRegistry()
	validator := NewCommandValidator()
	runner := NewRunner(registry, validator)

	validator.AddCommand("sleep")

	// Register 8 hooks (more than MaxConcurrentHooks=4)
	for i := 0; i < 8; i++ {
		hook := Hook{
			Name:     "parallel-hook-" + string(rune('0'+i)),
			Event:    HookEventSessionCompletion,
			Priority: 10 - i,
			Type:     HookTypeBinary,
			Command:  "sleep",
			Args:     []string{"0.1"}, // 100ms sleep
			Timeout:  5,
		}
		if err := registry.Register(hook); err != nil {
			t.Fatalf("Failed to register hook: %v", err)
		}
	}

	ctx := context.Background()
	start := time.Now()
	report, err := runner.RunAll(ctx, HookEventSessionCompletion)
	duration := time.Since(start)

	if err != nil {
		t.Fatalf("RunAll failed: %v", err)
	}

	if len(report.Results) != 8 {
		t.Errorf("Expected 8 results, got %d", len(report.Results))
	}

	// With parallel execution (4 concurrent), 8 hooks should take ~200ms
	// (2 batches of 4 hooks at 100ms each)
	// Without parallelism, it would take ~800ms
	if duration > 500*time.Millisecond {
		t.Logf("Warning: Parallel execution may not be working efficiently (took %v)", duration)
	}
}

func TestCalculateSummary(t *testing.T) {
	tests := []struct {
		name             string
		results          []VerificationResult
		warnings         []HookWarning
		wantPassed       int
		wantFailed       int
		wantWarningHooks int
		wantViolations   int
		wantExitCode     int
	}{
		{
			name: "all pass",
			results: []VerificationResult{
				{Status: VerificationStatusPass, Violations: []Violation{}},
				{Status: VerificationStatusPass, Violations: []Violation{}},
			},
			wantPassed:       2,
			wantFailed:       0,
			wantWarningHooks: 0,
			wantViolations:   0,
			wantExitCode:     0,
		},
		{
			name: "some fail",
			results: []VerificationResult{
				{Status: VerificationStatusPass, Violations: []Violation{}},
				{Status: VerificationStatusFail, Violations: []Violation{{Severity: "high", Message: "test"}}},
			},
			wantPassed:       1,
			wantFailed:       1,
			wantWarningHooks: 0,
			wantViolations:   1,
			wantExitCode:     1,
		},
		{
			name: "warnings only",
			results: []VerificationResult{
				{Status: VerificationStatusPass, Violations: []Violation{}},
				{Status: VerificationStatusWarning, Violations: []Violation{{Severity: "medium", Message: "test"}}},
			},
			wantPassed:       1,
			wantFailed:       0,
			wantWarningHooks: 1,
			wantViolations:   1,
			wantExitCode:     2,
		},
		{
			name: "hook warnings",
			results: []VerificationResult{
				{Status: VerificationStatusPass, Violations: []Violation{}},
			},
			warnings: []HookWarning{
				{Hook: "test", Message: "warning"},
			},
			wantPassed:       1,
			wantFailed:       0,
			wantWarningHooks: 1,
			wantViolations:   0,
			wantExitCode:     2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			summary := calculateSummary(tt.results, tt.warnings)

			if summary.PassedHooks != tt.wantPassed {
				t.Errorf("PassedHooks = %d, want %d", summary.PassedHooks, tt.wantPassed)
			}
			if summary.FailedHooks != tt.wantFailed {
				t.Errorf("FailedHooks = %d, want %d", summary.FailedHooks, tt.wantFailed)
			}
			if summary.WarningHooks != tt.wantWarningHooks {
				t.Errorf("WarningHooks = %d, want %d", summary.WarningHooks, tt.wantWarningHooks)
			}
			if summary.TotalViolations != tt.wantViolations {
				t.Errorf("TotalViolations = %d, want %d", summary.TotalViolations, tt.wantViolations)
			}
			if summary.ExitCode != tt.wantExitCode {
				t.Errorf("ExitCode = %d, want %d", summary.ExitCode, tt.wantExitCode)
			}
		})
	}
}

func TestRunnerContextCancellation(t *testing.T) {
	registry := NewRegistry()
	validator := NewCommandValidator()
	runner := NewRunner(registry, validator)

	validator.AddCommand("sleep")

	hook := Hook{
		Name:     "long-hook",
		Event:    HookEventSessionCompletion,
		Priority: 10,
		Type:     HookTypeBinary,
		Command:  "sleep",
		Args:     []string{"10"},
		Timeout:  20,
	}

	if err := registry.Register(hook); err != nil {
		t.Fatalf("Failed to register hook: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := runner.RunAll(ctx, HookEventSessionCompletion)
	duration := time.Since(start)

	if err != nil {
		t.Logf("RunAll returned error (expected): %v", err)
	}

	// Should complete quickly due to context cancellation
	if duration > 2*time.Second {
		t.Errorf("Expected quick completion due to context cancellation, took %v", duration)
	}
}
