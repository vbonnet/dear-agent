package healthchecker

import (
	"context"
	"errors"
	"testing"
	"time"
)

// Mock check for testing
type mockCheck struct {
	name     string
	category string
	status   Status
	delay    time.Duration
}

func (m mockCheck) Name() string     { return m.name }
func (m mockCheck) Category() string { return m.category }

func (m mockCheck) Run(ctx context.Context) Result {
	if m.delay > 0 {
		time.Sleep(m.delay)
	}
	return Result{
		Name:     m.name,
		Category: m.category,
		Status:   m.status,
	}
}

func TestRunner_RunAll_Sequential(t *testing.T) {
	checks := []Check{
		mockCheck{name: "check1", category: "core", status: StatusOK},
		mockCheck{name: "check2", category: "dependency", status: StatusWarning},
		mockCheck{name: "check3", category: "core", status: StatusError},
	}

	runner := NewRunner(checks...)
	results, err := runner.RunAll(context.Background())

	if err != nil {
		t.Fatalf("RunAll() error = %v", err)
	}

	if len(results) != 3 {
		t.Errorf("RunAll() returned %d results, want 3", len(results))
	}

	// Verify results are in order
	if results[0].Name != "check1" {
		t.Errorf("results[0].Name = %q, want %q", results[0].Name, "check1")
	}
	if results[0].Status != StatusOK {
		t.Errorf("results[0].Status = %v, want %v", results[0].Status, StatusOK)
	}
}

func TestRunner_RunAll_Parallel(t *testing.T) {
	checks := []Check{
		mockCheck{name: "check1", category: "core", status: StatusOK, delay: 10 * time.Millisecond},
		mockCheck{name: "check2", category: "dependency", status: StatusWarning, delay: 10 * time.Millisecond},
		mockCheck{name: "check3", category: "core", status: StatusError, delay: 10 * time.Millisecond},
	}

	runner := NewRunner(checks...).WithParallel(true)
	start := time.Now()
	results, err := runner.RunAll(context.Background())
	duration := time.Since(start)

	if err != nil {
		t.Fatalf("RunAll() error = %v", err)
	}

	if len(results) != 3 {
		t.Errorf("RunAll() returned %d results, want 3", len(results))
	}

	// Parallel execution should be faster than sequential
	// (30ms sequential vs ~10ms parallel)
	if duration > 25*time.Millisecond {
		t.Errorf("Parallel execution took %v, expected < 25ms", duration)
	}
}

func TestRunner_RunAll_ContextCancellation(t *testing.T) {
	checks := []Check{
		mockCheck{name: "check1", category: "core", status: StatusOK, delay: 50 * time.Millisecond},
		mockCheck{name: "check2", category: "dependency", status: StatusOK, delay: 50 * time.Millisecond},
	}

	runner := NewRunner(checks...)
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()

	_, err := runner.RunAll(ctx)

	if err == nil {
		t.Error("RunAll() expected error due to context cancellation")
	}

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("RunAll() error = %v, want %v", err, context.DeadlineExceeded)
	}
}

func TestSummarize(t *testing.T) {
	results := []Result{
		{Status: StatusOK},
		{Status: StatusOK},
		{Status: StatusInfo},
		{Status: StatusWarning, Fixable: true},
		{Status: StatusError, Fixable: true},
		{Status: StatusError},
	}

	summary := Summarize(results)

	if summary.Total != 6 {
		t.Errorf("Total = %d, want 6", summary.Total)
	}
	if summary.Passed != 3 {
		t.Errorf("Passed = %d, want 3", summary.Passed)
	}
	if summary.Warnings != 1 {
		t.Errorf("Warnings = %d, want 1", summary.Warnings)
	}
	if summary.Errors != 2 {
		t.Errorf("Errors = %d, want 2", summary.Errors)
	}
	if summary.Fixable != 2 {
		t.Errorf("Fixable = %d, want 2", summary.Fixable)
	}
}

func TestSummary_IsHealthy(t *testing.T) {
	tests := []struct {
		name    string
		summary Summary
		want    bool
	}{
		{
			name:    "all passed is healthy",
			summary: Summary{Passed: 10},
			want:    true,
		},
		{
			name:    "warnings make it unhealthy",
			summary: Summary{Passed: 5, Warnings: 1},
			want:    false,
		},
		{
			name:    "errors make it unhealthy",
			summary: Summary{Passed: 5, Errors: 1},
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.summary.IsHealthy()
			if got != tt.want {
				t.Errorf("IsHealthy() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSummary_ExitCode(t *testing.T) {
	tests := []struct {
		name    string
		summary Summary
		want    int
	}{
		{
			name:    "healthy returns 0",
			summary: Summary{Passed: 10},
			want:    0,
		},
		{
			name:    "warnings return 1",
			summary: Summary{Passed: 5, Warnings: 2},
			want:    1,
		},
		{
			name:    "errors return 2",
			summary: Summary{Passed: 5, Errors: 1},
			want:    2,
		},
		{
			name:    "errors take precedence over warnings",
			summary: Summary{Warnings: 5, Errors: 1},
			want:    2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.summary.ExitCode()
			if got != tt.want {
				t.Errorf("ExitCode() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestSummary_OverallStatus(t *testing.T) {
	tests := []struct {
		name    string
		summary Summary
		want    string
	}{
		{
			name:    "passed is healthy",
			summary: Summary{Passed: 10},
			want:    "Healthy",
		},
		{
			name:    "warnings are degraded",
			summary: Summary{Passed: 5, Warnings: 2},
			want:    "Degraded",
		},
		{
			name:    "errors are critical",
			summary: Summary{Passed: 5, Errors: 1},
			want:    "Critical",
		},
		{
			name:    "empty is unknown",
			summary: Summary{},
			want:    "Unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.summary.OverallStatus()
			if got != tt.want {
				t.Errorf("OverallStatus() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFilterIssues(t *testing.T) {
	results := []Result{
		{Status: StatusOK},
		{Status: StatusWarning},
		{Status: StatusInfo},
		{Status: StatusError},
	}

	issues := FilterIssues(results)

	if len(issues) != 2 {
		t.Errorf("FilterIssues() returned %d issues, want 2", len(issues))
	}

	if issues[0].Status != StatusWarning {
		t.Errorf("issues[0].Status = %v, want %v", issues[0].Status, StatusWarning)
	}
	if issues[1].Status != StatusError {
		t.Errorf("issues[1].Status = %v, want %v", issues[1].Status, StatusError)
	}
}

func TestFilterFixable(t *testing.T) {
	results := []Result{
		{Status: StatusError, Fixable: true, Fix: &Fix{Name: "fix1"}},
		{Status: StatusError, Fixable: false},
		{Status: StatusWarning, Fixable: true, Fix: &Fix{Name: "fix2"}},
	}

	fixable := FilterFixable(results)

	if len(fixable) != 2 {
		t.Errorf("FilterFixable() returned %d results, want 2", len(fixable))
	}

	if !fixable[0].Fixable {
		t.Error("FilterFixable() returned non-fixable result")
	}
	if fixable[0].Fix == nil {
		t.Error("FilterFixable() returned result without fix")
	}
}

func TestGroupByCategory(t *testing.T) {
	results := []Result{
		{Name: "check1", Category: "core"},
		{Name: "check2", Category: "dependency"},
		{Name: "check3", Category: "core"},
		{Name: "check4", Category: "hooks"},
	}

	grouped := GroupByCategory(results)

	if len(grouped) != 3 {
		t.Errorf("GroupByCategory() returned %d categories, want 3", len(grouped))
	}

	if len(grouped["core"]) != 2 {
		t.Errorf("core category has %d checks, want 2", len(grouped["core"]))
	}
	if len(grouped["dependency"]) != 1 {
		t.Errorf("dependency category has %d checks, want 1", len(grouped["dependency"]))
	}
	if len(grouped["hooks"]) != 1 {
		t.Errorf("hooks category has %d checks, want 1", len(grouped["hooks"]))
	}
}
