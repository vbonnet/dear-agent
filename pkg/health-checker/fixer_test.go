package healthchecker

import (
	"context"
	"errors"
	"testing"
)

func TestFixer_Preview(t *testing.T) {
	results := []Result{
		{Name: "check1", Status: StatusError, Fixable: true, Fix: &Fix{Name: "fix1"}},
		{Name: "check2", Status: StatusError, Fixable: false},
		{Name: "check3", Status: StatusWarning, Fixable: true, Fix: &Fix{Name: "fix2"}},
	}

	fixer := NewFixer()
	fixable := fixer.Preview(results)

	if len(fixable) != 2 {
		t.Errorf("Preview() returned %d fixable results, want 2", len(fixable))
	}
}

func TestFixer_Apply_Success(t *testing.T) {
	fixCalled := 0
	results := []Result{
		{
			Name:    "check1",
			Status:  StatusError,
			Message: "Directory missing",
			Fixable: true,
			Fix: &Fix{
				Name: "Create directory",
				Apply: func(ctx context.Context) error {
					fixCalled++
					return nil
				},
			},
		},
	}

	fixer := NewFixer()
	applied, updated, err := fixer.Apply(context.Background(), results)

	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	if applied != 1 {
		t.Errorf("Apply() applied = %d, want 1", applied)
	}

	if fixCalled != 1 {
		t.Errorf("Fix function called %d times, want 1", fixCalled)
	}

	// Verify result was updated
	if updated[0].Status != StatusOK {
		t.Errorf("updated[0].Status = %v, want %v", updated[0].Status, StatusOK)
	}
	if updated[0].Message != "" {
		t.Errorf("updated[0].Message = %q, want empty", updated[0].Message)
	}
	if updated[0].Fixable {
		t.Error("updated[0].Fixable = true, want false")
	}
	if updated[0].Fix != nil {
		t.Error("updated[0].Fix != nil, want nil")
	}
}

func TestFixer_Apply_Failure(t *testing.T) {
	results := []Result{
		{
			Name:    "check1",
			Status:  StatusError,
			Message: "Permission denied",
			Fixable: true,
			Fix: &Fix{
				Name: "Fix permissions",
				Apply: func(ctx context.Context) error {
					return errors.New("operation not permitted")
				},
			},
		},
	}

	fixer := NewFixer()
	applied, updated, err := fixer.Apply(context.Background(), results)

	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	if applied != 0 {
		t.Errorf("Apply() applied = %d, want 0 (fix failed)", applied)
	}

	// Verify result was NOT updated (fix failed)
	if updated[0].Status != StatusError {
		t.Errorf("updated[0].Status = %v, want %v (should remain error)", updated[0].Status, StatusError)
	}
	if updated[0].Fixable != true {
		t.Error("updated[0].Fixable = false, want true (should remain fixable)")
	}
}

func TestFixer_Apply_DryRun(t *testing.T) {
	fixCalled := 0
	results := []Result{
		{
			Name:    "check1",
			Status:  StatusError,
			Fixable: true,
			Fix: &Fix{
				Name: "Create directory",
				Apply: func(ctx context.Context) error {
					fixCalled++
					return nil
				},
			},
		},
	}

	fixer := NewFixer().WithDryRun(true)
	applied, updated, err := fixer.Apply(context.Background(), results)

	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	if applied != 0 {
		t.Errorf("Apply() applied = %d, want 0 (dry-run mode)", applied)
	}

	if fixCalled != 0 {
		t.Errorf("Fix function called %d times, want 0 (dry-run should not execute)", fixCalled)
	}

	// Verify results unchanged
	if updated[0].Status != StatusError {
		t.Errorf("updated[0].Status = %v, want %v (dry-run should not change)", updated[0].Status, StatusError)
	}
}

func TestFixer_ApplyOne_Success(t *testing.T) {
	fixCalled := 0
	result := Result{
		Name:    "check1",
		Status:  StatusError,
		Message: "File missing",
		Fixable: true,
		Fix: &Fix{
			Name: "Create file",
			Apply: func(ctx context.Context) error {
				fixCalled++
				return nil
			},
		},
	}

	fixer := NewFixer()
	success, updated, err := fixer.ApplyOne(context.Background(), result)

	if err != nil {
		t.Fatalf("ApplyOne() error = %v", err)
	}

	if !success {
		t.Error("ApplyOne() success = false, want true")
	}

	if fixCalled != 1 {
		t.Errorf("Fix function called %d times, want 1", fixCalled)
	}

	if updated.Status != StatusOK {
		t.Errorf("updated.Status = %v, want %v", updated.Status, StatusOK)
	}
}

func TestFixer_ApplyOne_NotFixable(t *testing.T) {
	result := Result{
		Name:    "check1",
		Status:  StatusError,
		Fixable: false,
	}

	fixer := NewFixer()
	success, _, err := fixer.ApplyOne(context.Background(), result)

	if err == nil {
		t.Error("ApplyOne() expected error for non-fixable result")
	}

	if success {
		t.Error("ApplyOne() success = true, want false")
	}
}

func TestFixer_ApplyWithReport(t *testing.T) {
	results := []Result{
		{
			Name:    "check1",
			Status:  StatusError,
			Fixable: true,
			Fix: &Fix{
				Apply: func(ctx context.Context) error {
					return nil // Success
				},
			},
		},
		{
			Name:    "check2",
			Status:  StatusError,
			Fixable: true,
			Fix: &Fix{
				Apply: func(ctx context.Context) error {
					return errors.New("failed") // Failure
				},
			},
		},
		{
			Name:    "check3",
			Status:  StatusWarning,
			Fixable: false,
		},
	}

	fixer := NewFixer()
	report, updated, err := fixer.ApplyWithReport(context.Background(), results)

	if err != nil {
		t.Fatalf("ApplyWithReport() error = %v", err)
	}

	if report.Total != 2 {
		t.Errorf("report.Total = %d, want 2", report.Total)
	}
	if report.Applied != 1 {
		t.Errorf("report.Applied = %d, want 1", report.Applied)
	}
	if report.Failed != 1 {
		t.Errorf("report.Failed = %d, want 1", report.Failed)
	}
	if len(report.Successes) != 1 {
		t.Errorf("len(report.Successes) = %d, want 1", len(report.Successes))
	}
	if len(report.Failures) != 1 {
		t.Errorf("len(report.Failures) = %d, want 1", len(report.Failures))
	}

	// Verify updated results
	if updated[0].Status != StatusOK {
		t.Errorf("updated[0].Status = %v, want %v (successful fix)", updated[0].Status, StatusOK)
	}
	if updated[1].Status != StatusError {
		t.Errorf("updated[1].Status = %v, want %v (failed fix)", updated[1].Status, StatusError)
	}
}

func TestFixer_ApplyWithReport_DryRun(t *testing.T) {
	results := []Result{
		{Name: "check1", Status: StatusError, Fixable: true, Fix: &Fix{Apply: func(ctx context.Context) error { return nil }}},
		{Name: "check2", Status: StatusError, Fixable: true, Fix: &Fix{Apply: func(ctx context.Context) error { return nil }}},
	}

	fixer := NewFixer().WithDryRun(true)
	report, _, err := fixer.ApplyWithReport(context.Background(), results)

	if err != nil {
		t.Fatalf("ApplyWithReport() error = %v", err)
	}

	if report.Total != 2 {
		t.Errorf("report.Total = %d, want 2", report.Total)
	}
	if report.Skipped != 2 {
		t.Errorf("report.Skipped = %d, want 2 (dry-run mode)", report.Skipped)
	}
	if report.Applied != 0 {
		t.Errorf("report.Applied = %d, want 0 (dry-run mode)", report.Applied)
	}
}

func TestFixable_FalseWithNonNilFix(t *testing.T) {
	t.Run("Apply skips result with Fixable=false but non-nil Fix", func(t *testing.T) {
		fixCalled := 0
		results := []Result{
			{
				Name:    "check1",
				Status:  StatusError,
				Message: "Something wrong",
				Fixable: false,
				Fix: &Fix{
					Name: "Should not run",
					Apply: func(ctx context.Context) error {
						fixCalled++
						return nil
					},
				},
			},
		}

		fixer := NewFixer()
		applied, updated, err := fixer.Apply(context.Background(), results)

		if err != nil {
			t.Fatalf("Apply() error = %v", err)
		}

		if applied != 0 {
			t.Errorf("Apply() applied = %d, want 0 (Fixable=false should skip)", applied)
		}

		if fixCalled != 0 {
			t.Errorf("Fix function called %d times, want 0 (Fixable=false should prevent execution)", fixCalled)
		}

		// Status should remain unchanged
		if updated[0].Status != StatusError {
			t.Errorf("updated[0].Status = %v, want %v (should remain unchanged)", updated[0].Status, StatusError)
		}

		// Fix struct should remain (not cleared since fix wasn't applied)
		if updated[0].Fix == nil {
			t.Error("updated[0].Fix = nil, want non-nil (should remain unchanged)")
		}
	})

	t.Run("ApplyOne returns error for Fixable=false with non-nil Fix", func(t *testing.T) {
		fixCalled := 0
		result := Result{
			Name:    "check1",
			Status:  StatusError,
			Fixable: false,
			Fix: &Fix{
				Name: "Should not run",
				Apply: func(ctx context.Context) error {
					fixCalled++
					return nil
				},
			},
		}

		fixer := NewFixer()
		success, _, err := fixer.ApplyOne(context.Background(), result)

		if err == nil {
			t.Error("ApplyOne() expected error for Fixable=false result")
		}

		if success {
			t.Error("ApplyOne() success = true, want false")
		}

		if fixCalled != 0 {
			t.Errorf("Fix function called %d times, want 0", fixCalled)
		}
	})

	t.Run("FilterFixable excludes Fixable=false with non-nil Fix", func(t *testing.T) {
		results := []Result{
			{Name: "check1", Status: StatusError, Fixable: true, Fix: &Fix{Name: "real fix"}},
			{Name: "check2", Status: StatusError, Fixable: false, Fix: &Fix{Name: "should be excluded"}},
			{Name: "check3", Status: StatusError, Fixable: false},
		}

		fixable := FilterFixable(results)

		if len(fixable) != 1 {
			t.Errorf("FilterFixable() returned %d results, want 1 (only Fixable=true with Fix)", len(fixable))
		}

		if len(fixable) > 0 && fixable[0].Name != "check1" {
			t.Errorf("FilterFixable()[0].Name = %q, want %q", fixable[0].Name, "check1")
		}
	})
}

func TestFixer_Apply_ContextCancellation(t *testing.T) {
	results := []Result{
		{
			Name:    "check1",
			Status:  StatusError,
			Fixable: true,
			Fix: &Fix{
				Apply: func(ctx context.Context) error {
					<-ctx.Done()
					return ctx.Err()
				},
			},
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	fixer := NewFixer()
	_, _, err := fixer.Apply(ctx, results)

	if !errors.Is(err, context.Canceled) {
		t.Errorf("Apply() error = %v, want %v", err, context.Canceled)
	}
}
