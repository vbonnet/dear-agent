package healthchecker

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestFixer_ApplyRejectsMalformedBatchBeforeCallbacks(t *testing.T) {
	for _, dryRun := range []bool{false, true} {
		t.Run(map[bool]string{false: "normal", true: "dry-run"}[dryRun], func(t *testing.T) {
			callbackCalls := 0
			validFix := &Fix{
				Name: "valid fix",
				Apply: func(context.Context) error {
					callbackCalls++
					return nil
				},
			}
			zeroStatusFix := &Fix{
				Name: "zero status fix",
				Apply: func(context.Context) error {
					callbackCalls++
					return nil
				},
			}
			bogusStatusFix := &Fix{
				Name: "bogus status fix",
				Apply: func(context.Context) error {
					callbackCalls++
					return nil
				},
			}
			results := []Result{
				{Name: "valid", Status: StatusError, Message: "valid message", Fixable: true, Fix: validFix},
				{Name: "zero", Message: "zero message", Fixable: true, Fix: zeroStatusFix},
				{Name: "bogus", Status: Status("bogus"), Message: "bogus message", Fixable: false, Fix: bogusStatusFix},
			}

			applied, safe, err := NewFixer().WithDryRun(dryRun).Apply(context.Background(), results)

			if applied != 0 {
				t.Fatalf("Apply() applied = %d, want 0", applied)
			}
			if !errors.Is(err, ErrInvalidStatus) {
				t.Fatalf("Apply() error = %v, want ErrInvalidStatus", err)
			}
			for _, index := range []string{"result[1]", "result[2]"} {
				if !strings.Contains(err.Error(), index) {
					t.Errorf("Apply() error = %q, want indexed error %q", err, index)
				}
			}
			if callbackCalls != 0 {
				t.Fatalf("Apply() invoked %d callbacks, want 0", callbackCalls)
			}
			if len(safe) != len(results) {
				t.Fatalf("Apply() safe result length = %d, want %d", len(safe), len(results))
			}
			if &safe[0] == &results[0] {
				t.Fatal("Apply() returned a safe slice that aliases the caller-owned slice")
			}
			if safe[0].Fix != validFix || safe[0].Status != StatusError || !safe[0].Fixable {
				t.Errorf("Apply() changed valid prepared result: %+v", safe[0])
			}
			for i := 1; i < len(safe); i++ {
				if safe[i].Status != StatusError || safe[i].Fixable || safe[i].Fix != nil {
					t.Errorf("Apply() safe[%d] = %+v, want non-executable error", i, safe[i])
				}
			}

			if results[0].Fix != validFix || results[0].Status != StatusError || !results[0].Fixable {
				t.Errorf("Apply() mutated valid input: %+v", results[0])
			}
			if results[1].Status != Status("") || !results[1].Fixable || results[1].Fix != zeroStatusFix || results[1].Message != "zero message" {
				t.Errorf("Apply() mutated zero-status input: %+v", results[1])
			}
			if results[2].Status != Status("bogus") || results[2].Fixable || results[2].Fix != bogusStatusFix || results[2].Message != "bogus message" {
				t.Errorf("Apply() mutated bogus-status input: %+v", results[2])
			}
		})
	}
}

func TestFixer_ApplyOneRejectsMalformedBeforeDryRunOrEligibility(t *testing.T) {
	statuses := []struct {
		name   string
		status Status
	}{
		{name: "zero", status: Status("")},
		{name: "arbitrary", status: Status("bogus")},
	}
	for _, status := range statuses {
		for _, dryRun := range []bool{false, true} {
			for _, fixable := range []bool{false, true} {
				for _, cancelled := range []bool{false, true} {
					name := status.name + "/" + map[bool]string{false: "normal", true: "dry-run"}[dryRun] + "/" +
						map[bool]string{false: "non-fixable", true: "fixable"}[fixable] + "/" +
						map[bool]string{false: "active-context", true: "cancelled-context"}[cancelled]
					t.Run(name, func(t *testing.T) {
						callbackCalls := 0
						fix := &Fix{Apply: func(context.Context) error {
							callbackCalls++
							return nil
						}}
						result := Result{
							Name:     "malformed",
							Status:   status.status,
							Message:  "original",
							Fixable:  fixable,
							Fix:      fix,
							Category: "test",
						}
						ctx := context.Background()
						if cancelled {
							cancelledCtx, cancel := context.WithCancel(ctx)
							cancel()
							ctx = cancelledCtx
						}

						success, safe, err := NewFixer().WithDryRun(dryRun).ApplyOne(ctx, result)

						if success {
							t.Fatal("ApplyOne() success = true, want false")
						}
						if !errors.Is(err, ErrInvalidStatus) {
							t.Fatalf("ApplyOne() error = %v, want ErrInvalidStatus", err)
						}
						if callbackCalls != 0 {
							t.Fatalf("ApplyOne() invoked %d callbacks, want 0", callbackCalls)
						}
						if safe.Status != StatusError || safe.Fixable || safe.Fix != nil {
							t.Errorf("ApplyOne() safe result = %+v, want non-executable error", safe)
						}
						if result.Status != status.status || result.Fixable != fixable || result.Fix != fix || result.Message != "original" {
							t.Errorf("ApplyOne() mutated input: %+v", result)
						}
					})
				}
			}
		}
	}
}

func TestFixer_ApplyWithReportRejectsMalformedBeforeCountsOrCallbacks(t *testing.T) {
	statuses := []struct {
		name   string
		status Status
	}{
		{name: "zero", status: Status("")},
		{name: "arbitrary", status: Status("future")},
	}
	for _, status := range statuses {
		for _, dryRun := range []bool{false, true} {
			t.Run(status.name+"/"+map[bool]string{false: "normal", true: "dry-run"}[dryRun], func(t *testing.T) {
				callbackCalls := 0
				validFix := &Fix{Apply: func(context.Context) error {
					callbackCalls++
					return nil
				}}
				invalidFix := &Fix{Apply: func(context.Context) error {
					callbackCalls++
					return nil
				}}
				results := []Result{
					{Name: "valid", Status: StatusWarning, Fixable: true, Fix: validFix},
					{Name: "invalid", Status: status.status, Fixable: true, Fix: invalidFix},
				}

				report, safe, err := NewFixer().WithDryRun(dryRun).ApplyWithReport(context.Background(), results)

				if !errors.Is(err, ErrInvalidStatus) {
					t.Fatalf("ApplyWithReport() error = %v, want ErrInvalidStatus", err)
				}
				if callbackCalls != 0 {
					t.Fatalf("ApplyWithReport() invoked %d callbacks, want 0", callbackCalls)
				}
				if report == nil {
					t.Fatal("ApplyWithReport() report = nil, want empty non-nil report")
				}
				if report.Total != 0 || report.Applied != 0 || report.Failed != 0 || report.Skipped != 0 || len(report.Successes) != 0 || len(report.Failures) != 0 {
					t.Errorf("ApplyWithReport() report = %+v, want empty report", report)
				}
				if report.Successes == nil || report.Failures == nil {
					t.Errorf("ApplyWithReport() report slices must remain non-nil: %+v", report)
				}
				if len(safe) != 2 || &safe[0] == &results[0] {
					t.Fatalf("ApplyWithReport() safe results do not form an independent full copy: %+v", safe)
				}
				if safe[0].Fix != validFix || safe[1].Status != StatusError || safe[1].Fixable || safe[1].Fix != nil {
					t.Errorf("ApplyWithReport() safe results = %+v, want valid entry plus non-executable error", safe)
				}
				if results[0].Fix != validFix || results[1].Status != status.status || results[1].Fix != invalidFix || !results[1].Fixable {
					t.Errorf("ApplyWithReport() mutated input: %+v", results)
				}
			})
		}
	}
}

func TestFixer_PreviewExcludesMalformedResults(t *testing.T) {
	statuses := []struct {
		name   string
		status Status
	}{
		{name: "zero", status: Status("")},
		{name: "arbitrary", status: Status("bogus")},
	}
	for _, status := range statuses {
		t.Run(status.name, func(t *testing.T) {
			validFix := &Fix{Name: "valid"}
			invalidFix := &Fix{Name: "invalid"}
			results := []Result{
				{Name: "valid", Status: StatusWarning, Fixable: true, Fix: validFix},
				{Name: "invalid", Status: status.status, Fixable: true, Fix: invalidFix},
			}

			preview := NewFixer().Preview(results)

			if len(preview) != 1 {
				t.Fatalf("Preview() length = %d, want 1", len(preview))
			}
			if preview[0].Name != "valid" || preview[0].Fix != validFix {
				t.Errorf("Preview()[0] = %+v, want unchanged valid result", preview[0])
			}
			if results[1].Status != status.status || results[1].Fix != invalidFix || !results[1].Fixable {
				t.Errorf("Preview() mutated malformed input: %+v", results[1])
			}
		})
	}
}

func TestFixer_ValidDryRunPreservesInputSlice(t *testing.T) {
	tests := []struct {
		name    string
		results []Result
	}{
		{name: "nil", results: nil},
		{name: "empty", results: make([]Result, 0)},
		{
			name: "populated",
			results: []Result{{
				Name:     "valid",
				Status:   StatusWarning,
				Fixable:  true,
				Fix:      &Fix{Apply: func(context.Context) error { return nil }},
				Category: "test",
			}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixer := NewFixer().WithDryRun(true)

			applied, updated, err := fixer.Apply(context.Background(), tt.results)
			if err != nil {
				t.Fatalf("Apply() error = %v", err)
			}
			if applied != 0 {
				t.Errorf("Apply() applied = %d, want 0", applied)
			}
			assertSameSlice(t, "Apply", tt.results, updated)

			report, reported, err := fixer.ApplyWithReport(context.Background(), tt.results)
			if err != nil {
				t.Fatalf("ApplyWithReport() error = %v", err)
			}
			if report == nil {
				t.Fatal("ApplyWithReport() report = nil")
			}
			assertSameSlice(t, "ApplyWithReport", tt.results, reported)
			if len(tt.results) == 1 && (report.Total != 1 || report.Skipped != 1) {
				t.Errorf("ApplyWithReport() report = %+v, want Total=1 Skipped=1", report)
			}
		})
	}
}

func TestFixer_ValidNilNonDryRunCompatibility(t *testing.T) {
	applied, updated, err := NewFixer().Apply(context.Background(), nil)
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if applied != 0 {
		t.Errorf("Apply() applied = %d, want 0", applied)
	}
	if updated == nil || len(updated) != 0 {
		t.Errorf("Apply() result = %#v, want non-nil empty slice", updated)
	}

	report, reported, err := NewFixer().ApplyWithReport(context.Background(), nil)
	if err != nil {
		t.Fatalf("ApplyWithReport() error = %v", err)
	}
	if report == nil || report.Successes == nil || report.Failures == nil {
		t.Errorf("ApplyWithReport() report = %#v, want non-nil report and slices", report)
	}
	if reported == nil || len(reported) != 0 {
		t.Errorf("ApplyWithReport() results = %#v, want non-nil empty slice", reported)
	}
}

func assertSameSlice(t *testing.T, operation string, want, got []Result) {
	t.Helper()
	if (got == nil) != (want == nil) {
		t.Fatalf("%s() result nilness = %t, want %t", operation, got == nil, want == nil)
	}
	if len(got) != len(want) {
		t.Fatalf("%s() result length = %d, want %d", operation, len(got), len(want))
	}
	if len(want) > 0 && &got[0] != &want[0] {
		t.Fatalf("%s() result does not preserve caller backing-array identity", operation)
	}
}
