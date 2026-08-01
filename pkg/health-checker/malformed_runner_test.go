package healthchecker

import (
	"context"
	"errors"
	"testing"
)

type fixedResultCheck struct {
	name     string
	category string
	result   Result
}

func (c fixedResultCheck) Name() string     { return c.name }
func (c fixedResultCheck) Category() string { return c.category }
func (c fixedResultCheck) Run(context.Context) Result {
	return c.result
}

type panicResultCheck struct {
	name     string
	category string
}

func (c panicResultCheck) Name() string     { return c.name }
func (c panicResultCheck) Category() string { return c.category }
func (panicResultCheck) Run(context.Context) Result {
	panic("boom")
}

type typedNilPanicCheck struct {
	name string
}

func (c *typedNilPanicCheck) Name() string             { return c.name }
func (*typedNilPanicCheck) Category() string           { return "runtime" }
func (*typedNilPanicCheck) Run(context.Context) Result { panic("typed nil") }

func TestRunner_RunAll_ParallelPanicPreservesCheckIdentity(t *testing.T) {
	results, err := NewRunner(
		panicResultCheck{name: "panic-check", category: "runtime"},
	).WithParallel(true).RunAll(context.Background())
	if err != nil {
		t.Fatalf("RunAll() error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("RunAll() returned %d results, want 1", len(results))
	}

	got := results[0]
	if got.Name != "panic-check" || got.Category != "runtime" {
		t.Errorf("panic identity = %q/%q, want %q/%q", got.Category, got.Name, "runtime", "panic-check")
	}
	if got.Status != StatusError || got.Message != "check panicked: boom" {
		t.Errorf("panic result = %+v, want status error and stable diagnostic", got)
	}
}

func TestRunner_RunAll_ParallelTypedNilCheckFailsClosed(t *testing.T) {
	var check *typedNilPanicCheck
	results, err := NewRunner(check).WithParallel(true).RunAll(context.Background())
	if err != nil {
		t.Fatalf("RunAll() error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("RunAll() returned %d results, want 1", len(results))
	}

	got := results[0]
	if got.Status != StatusError || got.Message != "check panicked: typed nil" {
		t.Errorf("typed-nil result = %+v, want status error and stable diagnostic", got)
	}
	if got.Name != "" || got.Category != "" {
		t.Errorf("typed-nil identity = %q/%q, want empty unavailable identity", got.Category, got.Name)
	}
}

func TestRunner_RunAll_ParallelPreCancelledTypedNilCheckFailsClosed(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var check *typedNilPanicCheck
	results, err := NewRunner(check).WithParallel(true).RunAll(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("RunAll() error = %v, want %v", err, context.Canceled)
	}
	if len(results) != 1 {
		t.Fatalf("RunAll() returned %d results, want 1", len(results))
	}

	got := results[0]
	if got.Status != StatusError || got.Message == "" {
		t.Errorf("typed-nil cancellation result = %+v, want fail-closed diagnostic", got)
	}
	if got.Name != "" || got.Category != "" {
		t.Errorf("typed-nil identity = %q/%q, want empty unavailable identity", got.Category, got.Name)
	}
}

func TestRunner_RunAll_NormalizesMalformedResults(t *testing.T) {
	tests := []struct {
		name     string
		parallel bool
		status   Status
	}{
		{name: "sequential zero", status: Status("")},
		{name: "sequential arbitrary", status: Status("future")},
		{name: "parallel zero", parallel: true, status: Status("")},
		{name: "parallel arbitrary", parallel: true, status: Status("future")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixCalled := false
			malformed := Result{
				Name:     "untrusted-name",
				Category: "untrusted-category",
				Status:   tt.status,
				Message:  "producer detail",
				Fixable:  true,
				Fix: &Fix{Apply: func(context.Context) error {
					fixCalled = true
					return nil
				}},
			}
			checks := []Check{
				fixedResultCheck{name: "malformed-check", category: "contract", result: malformed},
				fixedResultCheck{
					name:     "healthy-check",
					category: "core",
					result:   Result{Name: "healthy-check", Category: "core", Status: StatusOK},
				},
			}

			results, err := NewRunner(checks...).WithParallel(tt.parallel).RunAll(context.Background())
			if err != nil {
				t.Fatalf("RunAll() error = %v", err)
			}
			if len(results) != len(checks) {
				t.Fatalf("RunAll() returned %d results, want %d", len(results), len(checks))
			}

			got := results[0]
			if got.Name != "malformed-check" || got.Category != "contract" {
				t.Errorf("malformed identity = %q/%q, want %q/%q", got.Category, got.Name, "contract", "malformed-check")
			}
			if got.Status != StatusError {
				t.Errorf("malformed Status = %q, want %q", got.Status, StatusError)
			}
			if got.Fixable || got.Fix != nil {
				t.Errorf("malformed fix metadata = Fixable:%v Fix:%v, want cleared", got.Fixable, got.Fix)
			}
			if results[1].Name != "healthy-check" || results[1].Status != StatusOK {
				t.Errorf("results[1] = %+v, want unchanged healthy result at declaration index 1", results[1])
			}
			if fixCalled {
				t.Error("normalizing runner invoked malformed fix callback")
			}
		})
	}
}

func TestRunner_RunAll_PreservesValidResultIdentity(t *testing.T) {
	for _, parallel := range []bool{false, true} {
		name := "sequential"
		if parallel {
			name = "parallel"
		}
		t.Run(name, func(t *testing.T) {
			fix := &Fix{Name: "legacy valid fix metadata"}
			valid := Result{
				Name:     "result-name",
				Category: "result-category",
				Status:   StatusInfo,
				Message:  "caller-supplied message",
				Fixable:  true,
				Fix:      fix,
			}

			results, err := NewRunner(
				fixedResultCheck{name: "check-name", category: "check-category", result: valid},
			).WithParallel(parallel).RunAll(context.Background())
			if err != nil {
				t.Fatalf("RunAll() error = %v", err)
			}
			if len(results) != 1 {
				t.Fatalf("RunAll() returned %d results, want 1", len(results))
			}
			if got := results[0]; got != valid || got.Fix != fix {
				t.Errorf("RunAll() result = %+v, want exact valid result %+v", got, valid)
			}
		})
	}
}

func TestRunner_RunAll_ParallelPreCancelledCanonicalizesEverySlot(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	checks := []Check{
		fixedResultCheck{name: "check1", category: "core", result: Result{Status: StatusOK}},
		fixedResultCheck{name: "check2", category: "dependency", result: Result{Status: StatusWarning}},
	}
	results, err := NewRunner(checks...).WithParallel(true).RunAll(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("RunAll() error = %v, want %v", err, context.Canceled)
	}
	if len(results) != len(checks) {
		t.Fatalf("RunAll() returned %d results, want %d", len(results), len(checks))
	}

	for i, got := range results {
		if got.Status != StatusError {
			t.Errorf("results[%d].Status = %q, want %q", i, got.Status, StatusError)
		}
		if got.Name != checks[i].Name() || got.Category != checks[i].Category() {
			t.Errorf("results[%d] identity = %q/%q, want %q/%q", i, got.Category, got.Name, checks[i].Category(), checks[i].Name())
		}
		if got.Fixable || got.Fix != nil {
			t.Errorf("results[%d] retained fix metadata: %+v", i, got)
		}
	}
}

func TestSummarize_MalformedFailsClosed(t *testing.T) {
	tests := []struct {
		name   string
		status Status
	}{
		{name: "zero", status: Status("")},
		{name: "arbitrary", status: Status("future")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fix := &Fix{Name: "must not be eligible"}
			results := []Result{
				{Status: StatusOK},
				{Status: tt.status, Message: "producer detail", Fixable: true, Fix: fix},
			}

			summary := Summarize(results)
			if summary.Total != 2 || summary.Passed != 1 || summary.Warnings != 0 || summary.Errors != 1 || summary.Fixable != 0 {
				t.Errorf("Summarize() = %+v, want Total:2 Passed:1 Warnings:0 Errors:1 Fixable:0", summary)
			}
			if summary.IsHealthy() || !summary.HasIssues() {
				t.Errorf("malformed summary verdict = healthy:%v issues:%v, want false/true", summary.IsHealthy(), summary.HasIssues())
			}
			if summary.ExitCode() != 2 || summary.OverallStatus() != "Critical" {
				t.Errorf("malformed summary = exit %d/status %q, want 2/Critical", summary.ExitCode(), summary.OverallStatus())
			}
			if results[1].Status != tt.status || !results[1].Fixable || results[1].Fix != fix {
				t.Errorf("Summarize mutated caller input: %+v", results[1])
			}
		})
	}
}

func TestSummarize_EmptyCompatibility(t *testing.T) {
	summary := Summarize(nil)
	if summary != (Summary{}) {
		t.Errorf("Summarize(nil) = %+v, want zero Summary", summary)
	}
	if !summary.IsHealthy() || summary.HasIssues() {
		t.Errorf("empty summary verdict = healthy:%v issues:%v, want true/false", summary.IsHealthy(), summary.HasIssues())
	}
	if summary.ExitCode() != 0 || summary.OverallStatus() != "Unknown" {
		t.Errorf("empty summary = exit %d/status %q, want 0/Unknown", summary.ExitCode(), summary.OverallStatus())
	}
}

func TestFilters_NormalizeMalformedResults(t *testing.T) {
	tests := []struct {
		name   string
		status Status
	}{
		{name: "zero", status: Status("")},
		{name: "arbitrary", status: Status("future")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			malformedFix := &Fix{Name: "malformed fix"}
			validFix := &Fix{Name: "valid fix"}
			results := []Result{
				{Name: "healthy", Status: StatusOK},
				{Name: "malformed", Status: tt.status, Message: "producer detail", Fixable: true, Fix: malformedFix},
				{Name: "warning", Status: StatusWarning, Fixable: true, Fix: validFix},
			}

			issues := FilterIssues(results)
			if len(issues) != 2 {
				t.Fatalf("FilterIssues() returned %d results, want 2", len(issues))
			}
			if got := issues[0]; got.Name != "malformed" || got.Status != StatusError || got.Fixable || got.Fix != nil {
				t.Errorf("FilterIssues()[0] = %+v, want canonical malformed error", got)
			}
			if got := issues[1]; got.Name != "warning" || got.Status != StatusWarning || !got.Fixable || got.Fix != validFix {
				t.Errorf("FilterIssues()[1] = %+v, want unchanged warning", got)
			}

			fixable := FilterFixable(results)
			if len(fixable) != 1 {
				t.Fatalf("FilterFixable() returned %d results, want 1", len(fixable))
			}
			if got := fixable[0]; got.Name != "warning" || got.Fix != validFix {
				t.Errorf("FilterFixable()[0] = %+v, want valid warning fix", got)
			}
			if results[1].Status != tt.status || !results[1].Fixable || results[1].Fix != malformedFix {
				t.Errorf("filters mutated caller input: %+v", results[1])
			}
		})
	}
}
