package healthchecker

import (
	"context"
	"sync"
)

// Runner executes a collection of health checks
type Runner struct {
	checks   []Check
	parallel bool // Run checks in parallel
}

// NewRunner creates a new health check runner
func NewRunner(checks ...Check) *Runner {
	return &Runner{
		checks:   checks,
		parallel: false,
	}
}

// WithParallel enables parallel check execution
func (r *Runner) WithParallel(parallel bool) *Runner {
	r.parallel = parallel
	return r
}

// RunAll executes all health checks and returns results
func (r *Runner) RunAll(ctx context.Context) ([]Result, error) {
	if r.parallel {
		return r.runParallel(ctx)
	}
	return r.runSequential(ctx)
}

// runSequential runs checks sequentially
func (r *Runner) runSequential(ctx context.Context) ([]Result, error) {
	results := make([]Result, 0, len(r.checks))
	for _, check := range r.checks {
		// Check if context is cancelled
		select {
		case <-ctx.Done():
			return results, ctx.Err()
		default:
		}

		result := check.Run(ctx)
		results = append(results, result)
	}
	return results, nil
}

// runParallel runs checks in parallel
func (r *Runner) runParallel(ctx context.Context) ([]Result, error) {
	results := make([]Result, len(r.checks))
	var wg sync.WaitGroup
	var mu sync.Mutex
	errorCh := make(chan error, 1)

	for i, check := range r.checks {
		wg.Add(1)
		go func(idx int, c Check) {
			defer wg.Done()

			// Check if context is cancelled
			select {
			case <-ctx.Done():
				select {
				case errorCh <- ctx.Err():
				default:
				}
				return
			default:
			}

			result := c.Run(ctx)
			mu.Lock()
			results[idx] = result
			mu.Unlock()
		}(i, check)
	}

	wg.Wait()
	close(errorCh)

	// Return first error if any
	if err := <-errorCh; err != nil {
		return results, err
	}

	return results, nil
}

// Summary generates aggregate statistics from results
type Summary struct {
	Total    int // Total number of checks
	Passed   int // Checks with ok/info status
	Warnings int // Checks with warning status
	Errors   int // Checks with error status
	Fixable  int // Number of fixable issues
}

// Summarize generates a summary from check results
func Summarize(results []Result) Summary {
	summary := Summary{
		Total: len(results),
	}

	for _, r := range results {
		switch r.Status {
		case StatusOK, StatusInfo:
			summary.Passed++
		case StatusWarning:
			summary.Warnings++
		case StatusError:
			summary.Errors++
		}

		if r.Fixable {
			summary.Fixable++
		}
	}

	return summary
}

// IsHealthy returns true if there are no errors or warnings
func (s Summary) IsHealthy() bool {
	return s.Errors == 0 && s.Warnings == 0
}

// HasIssues returns true if there are any errors or warnings
func (s Summary) HasIssues() bool {
	return s.Errors > 0 || s.Warnings > 0
}

// ExitCode returns appropriate Unix exit code
// 0 = healthy, 1 = warnings, 2 = errors
func (s Summary) ExitCode() int {
	if s.Errors > 0 {
		return 2
	}
	if s.Warnings > 0 {
		return 1
	}
	return 0
}

// OverallStatus returns a string representing overall health
func (s Summary) OverallStatus() string {
	if s.Errors > 0 {
		return "Critical"
	}
	if s.Warnings > 0 {
		return "Degraded"
	}
	if s.Passed > 0 {
		return "Healthy"
	}
	return "Unknown"
}

// FilterIssues returns only warnings and errors from results
func FilterIssues(results []Result) []Result {
	issues := []Result{}
	for _, r := range results {
		if r.IsIssue() {
			issues = append(issues, r)
		}
	}
	return issues
}

// FilterFixable returns only fixable results
func FilterFixable(results []Result) []Result {
	fixable := []Result{}
	for _, r := range results {
		if r.Fixable && r.Fix != nil {
			fixable = append(fixable, r)
		}
	}
	return fixable
}

// GroupByCategory groups results by their category
func GroupByCategory(results []Result) map[string][]Result {
	grouped := make(map[string][]Result)
	for _, r := range results {
		grouped[r.Category] = append(grouped[r.Category], r)
	}
	return grouped
}
