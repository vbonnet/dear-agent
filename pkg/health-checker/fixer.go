package healthchecker

import (
	"context"
	"fmt"
)

// Fixer applies auto-fix operations to health check results
type Fixer struct {
	dryRun bool // Preview only, don't actually apply fixes
}

// NewFixer creates a new fixer instance
func NewFixer() *Fixer {
	return &Fixer{
		dryRun: false,
	}
}

// WithDryRun enables dry-run mode (preview only)
func (f *Fixer) WithDryRun(dryRun bool) *Fixer {
	f.dryRun = dryRun
	return f
}

// Preview returns all fixable results
func (f *Fixer) Preview(results []Result) []Result {
	return FilterFixable(results)
}

// Apply executes fixes for all fixable results
// Returns: (number of fixes applied, updated results, error)
func (f *Fixer) Apply(ctx context.Context, results []Result) (int, []Result, error) {
	if f.dryRun {
		return 0, results, nil
	}

	applied := 0
	updated := make([]Result, len(results))
	copy(updated, results)

	for i, r := range updated {
		if !r.Fixable || r.Fix == nil {
			continue
		}

		// Check if context is cancelled
		select {
		case <-ctx.Done():
			return applied, updated, ctx.Err()
		default:
		}

		// Apply the fix
		if err := r.Fix.Apply(ctx); err != nil {
			// Fix failed - update message but don't change status
			updated[i].Message = fmt.Sprintf("%s (fix failed: %v)", r.Message, err)
			continue
		}

		// Fix succeeded - mark as resolved
		applied++
		updated[i].Status = StatusOK
		updated[i].Message = ""
		updated[i].Fixable = false
		updated[i].Fix = nil
	}

	return applied, updated, nil
}

// ApplyOne executes a fix for a single result
// Returns: (success, updated result, error)
func (f *Fixer) ApplyOne(ctx context.Context, result Result) (bool, Result, error) {
	if f.dryRun {
		return false, result, nil
	}

	if !result.Fixable || result.Fix == nil {
		return false, result, fmt.Errorf("result is not fixable")
	}

	// Check if context is cancelled
	select {
	case <-ctx.Done():
		return false, result, ctx.Err()
	default:
	}

	// Apply the fix
	if err := result.Fix.Apply(ctx); err != nil {
		// Fix failed
		updated := result
		updated.Message = fmt.Sprintf("%s (fix failed: %v)", result.Message, err)
		return false, updated, err
	}

	// Fix succeeded
	updated := result
	updated.Status = StatusOK
	updated.Message = ""
	updated.Fixable = false
	updated.Fix = nil

	return true, updated, nil
}

// FixReport represents the outcome of applying fixes
type FixReport struct {
	Total     int      // Total fixable issues found
	Applied   int      // Number of fixes successfully applied
	Failed    int      // Number of fixes that failed
	Skipped   int      // Number of fixes skipped (e.g., dry-run)
	Successes []Result // Successfully fixed results
	Failures  []Result // Failed fix attempts
}

// ApplyWithReport executes fixes and returns a detailed report
func (f *Fixer) ApplyWithReport(ctx context.Context, results []Result) (*FixReport, []Result, error) {
	report := &FixReport{
		Successes: []Result{},
		Failures:  []Result{},
	}

	fixable := FilterFixable(results)
	report.Total = len(fixable)

	if f.dryRun {
		report.Skipped = report.Total
		return report, results, nil
	}

	applied, updated, err := f.Apply(ctx, results)
	if err != nil {
		return report, updated, err
	}

	report.Applied = applied
	report.Failed = report.Total - applied

	// Collect successes and failures
	for i, orig := range results {
		if !orig.Fixable {
			continue
		}

		if updated[i].Status == StatusOK && !updated[i].Fixable {
			report.Successes = append(report.Successes, updated[i])
		} else {
			report.Failures = append(report.Failures, updated[i])
		}
	}

	return report, updated, nil
}
