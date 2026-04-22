package hooks

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockExecutor implements Executor for testing
type MockExecutor struct {
	results map[string]*VerificationResult
	errors  map[string]error
}

func NewMockExecutor() *MockExecutor {
	return &MockExecutor{
		results: make(map[string]*VerificationResult),
		errors:  make(map[string]error),
	}
}

func (m *MockExecutor) SetResult(hookName string, result *VerificationResult) {
	m.results[hookName] = result
}

func (m *MockExecutor) SetError(hookName string, err error) {
	m.errors[hookName] = err
}

func (m *MockExecutor) Execute(ctx context.Context, hook Hook) (*VerificationResult, error) {
	if err, ok := m.errors[hook.Name]; ok {
		return nil, err
	}
	if result, ok := m.results[hook.Name]; ok {
		return result, nil
	}
	// Default: pass
	return &VerificationResult{
		HookName: hook.Name,
		Status:   VerificationStatusPass,
		Duration: time.Second,
		ExitCode: 0,
	}, nil
}

func TestAggregateResults(t *testing.T) {
	executor := NewMockExecutor()
	aggregator := NewAggregator(executor)

	// Set up test hooks
	hooks := []Hook{
		{Name: "hook1", Priority: 10},
		{Name: "hook2", Priority: 20},
		{Name: "hook3", Priority: 5},
	}

	// Set up mock results
	executor.SetResult("hook1", &VerificationResult{
		HookName: "hook1",
		Status:   VerificationStatusPass,
	})
	executor.SetResult("hook2", &VerificationResult{
		HookName: "hook2",
		Status:   VerificationStatusFail,
		Violations: []Violation{
			{Severity: "high", Message: "Test failure"},
		},
	})
	executor.SetResult("hook3", &VerificationResult{
		HookName: "hook3",
		Status:   VerificationStatusWarning,
	})

	// Run aggregation
	ctx := context.Background()
	report, err := aggregator.AggregateResults(ctx, HookEventSessionCompletion, hooks)
	require.NoError(t, err)

	// Verify results
	assert.Equal(t, HookEventSessionCompletion, report.Event)
	assert.Len(t, report.Results, 3)
	assert.Equal(t, 3, report.Summary.TotalHooks)
	assert.Equal(t, 1, report.Summary.PassedHooks)
	assert.Equal(t, 1, report.Summary.FailedHooks)
	assert.Equal(t, 1, report.Summary.WarningHooks)
	assert.Equal(t, 1, report.Summary.TotalViolations)
	assert.Equal(t, 1, report.Summary.ExitCode) // Fail
}

func TestAggregateResults_WithErrors(t *testing.T) {
	executor := NewMockExecutor()
	aggregator := NewAggregator(executor)

	// Set up test hooks
	hooks := []Hook{
		{Name: "hook1", Priority: 10},
		{Name: "hook2", Priority: 20},
	}

	// Set up mock results
	executor.SetResult("hook1", &VerificationResult{
		HookName: "hook1",
		Status:   VerificationStatusPass,
	})
	executor.SetError("hook2", assert.AnError)

	// Run aggregation
	ctx := context.Background()
	report, err := aggregator.AggregateResults(ctx, HookEventSessionCompletion, hooks)
	require.NoError(t, err)

	// Verify graceful degradation
	assert.Len(t, report.Results, 1)
	assert.Len(t, report.Warnings, 1)
	assert.Equal(t, "hook2", report.Warnings[0].Hook)
}

func TestAggregatorCalculateSummary(t *testing.T) {
	aggregator := NewAggregator(nil)

	report := &AggregatedReport{
		Results: []VerificationResult{
			{Status: VerificationStatusPass},
			{Status: VerificationStatusFail, Violations: []Violation{{}, {}}},
			{Status: VerificationStatusWarning, Violations: []Violation{{}}},
		},
		Warnings: []HookWarning{
			{Hook: "failed-hook"},
		},
	}

	summary := aggregator.calculateSummary(report)

	assert.Equal(t, 4, summary.TotalHooks) // 3 results + 1 warning
	assert.Equal(t, 1, summary.PassedHooks)
	assert.Equal(t, 1, summary.FailedHooks)
	assert.Equal(t, 2, summary.WarningHooks) // 1 result + 1 warning
	assert.Equal(t, 3, summary.TotalViolations)
	assert.Equal(t, 1, summary.ExitCode) // Fail due to FailedHooks > 0
}

func TestCalculateSummary_ExitCodes(t *testing.T) {
	aggregator := NewAggregator(nil)

	tests := []struct {
		name         string
		results      []VerificationResult
		warnings     []HookWarning
		wantExitCode int
	}{
		{
			name: "all pass",
			results: []VerificationResult{
				{Status: VerificationStatusPass},
			},
			wantExitCode: 0,
		},
		{
			name: "warnings only",
			results: []VerificationResult{
				{Status: VerificationStatusWarning},
			},
			wantExitCode: 2,
		},
		{
			name: "failures",
			results: []VerificationResult{
				{Status: VerificationStatusFail},
			},
			wantExitCode: 1,
		},
		{
			name: "hook warnings",
			warnings: []HookWarning{
				{Hook: "failed"},
			},
			wantExitCode: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			report := &AggregatedReport{
				Results:  tt.results,
				Warnings: tt.warnings,
			}
			summary := aggregator.calculateSummary(report)
			assert.Equal(t, tt.wantExitCode, summary.ExitCode)
		})
	}
}

func TestFormatTerminal(t *testing.T) {
	aggregator := NewAggregator(nil)

	report := &AggregatedReport{
		Timestamp: time.Date(2026, 2, 19, 12, 0, 0, 0, time.UTC),
		Event:     HookEventSessionCompletion,
		Results: []VerificationResult{
			{
				HookName: "test-hook",
				Status:   VerificationStatusFail,
				Violations: []Violation{
					{
						Severity:   "high",
						Message:    "Test failure",
						Files:      []string{"test.go"},
						Suggestion: "Fix the test",
					},
				},
			},
		},
		Summary: Summary{
			TotalHooks:      1,
			FailedHooks:     1,
			TotalViolations: 1,
			ExitCode:        1,
		},
	}

	output := aggregator.FormatTerminal(report)

	assert.Contains(t, output, "Verification Report")
	assert.Contains(t, output, "session-completion")
	assert.Contains(t, output, "HIGH SEVERITY VIOLATIONS")
	assert.Contains(t, output, "Test failure")
	assert.Contains(t, output, "test.go")
	assert.Contains(t, output, "Fix the test")
	assert.Contains(t, output, "SUMMARY")
}

func TestFormatMarkdown(t *testing.T) {
	aggregator := NewAggregator(nil)

	report := &AggregatedReport{
		Timestamp: time.Date(2026, 2, 19, 12, 0, 0, 0, time.UTC),
		Event:     HookEventSessionCompletion,
		Results: []VerificationResult{
			{
				HookName: "test-hook",
				Status:   VerificationStatusFail,
				Violations: []Violation{
					{
						Severity:   "high",
						Message:    "Test failure",
						Files:      []string{"test.go"},
						Suggestion: "Fix the test",
					},
				},
			},
		},
		Summary: Summary{
			TotalHooks:      1,
			FailedHooks:     1,
			TotalViolations: 1,
			ExitCode:        1,
		},
	}

	output := aggregator.FormatMarkdown(report)

	assert.Contains(t, output, "# Verification Report")
	assert.Contains(t, output, "## High Severity Violations")
	assert.Contains(t, output, "**[test-hook]** Test failure")
	assert.Contains(t, output, "## Summary")
}

func TestFormatJSON(t *testing.T) {
	aggregator := NewAggregator(nil)

	report := &AggregatedReport{
		Timestamp: time.Date(2026, 2, 19, 12, 0, 0, 0, time.UTC),
		Event:     HookEventSessionCompletion,
		Results: []VerificationResult{
			{
				HookName: "test-hook",
				Status:   VerificationStatusPass,
			},
		},
		Summary: Summary{
			TotalHooks:  1,
			PassedHooks: 1,
			ExitCode:    0,
		},
	}

	output, err := aggregator.FormatJSON(report)
	require.NoError(t, err)

	assert.Contains(t, output, `"hook_name": "test-hook"`)
	assert.Contains(t, output, `"status": "pass"`)
	assert.Contains(t, output, `"exit_code": 0`)
}

func TestPriorityOrdering(t *testing.T) {
	executor := NewMockExecutor()
	aggregator := NewAggregator(executor)

	// Set up hooks with different priorities
	hooks := []Hook{
		{Name: "low", Priority: 10},
		{Name: "high", Priority: 100},
		{Name: "medium", Priority: 50},
	}

	var executionOrder []string
	executor.results["low"] = &VerificationResult{HookName: "low", Status: VerificationStatusPass}
	executor.results["high"] = &VerificationResult{HookName: "high", Status: VerificationStatusPass}
	executor.results["medium"] = &VerificationResult{HookName: "medium", Status: VerificationStatusPass}

	// Run aggregation
	ctx := context.Background()
	report, err := aggregator.AggregateResults(ctx, HookEventSessionCompletion, hooks)
	require.NoError(t, err)

	// Extract execution order from results
	for _, result := range report.Results {
		executionOrder = append(executionOrder, result.HookName)
	}

	// Verify priority ordering (high -> medium -> low)
	assert.Equal(t, []string{"high", "medium", "low"}, executionOrder)
}
