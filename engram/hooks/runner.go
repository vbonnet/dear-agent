package hooks

import (
	"context"
	"fmt"
	"sync"
	"time"
)

const (
	// MaxConcurrentHooks is the maximum number of hooks to run in parallel
	MaxConcurrentHooks = 4
)

// Runner executes verification hooks
type Runner struct {
	registry *registryImpl
	executor *Executor
}

// NewRunner creates a new hook runner
func NewRunner(registry Registry, validator *CommandValidator) *Runner {
	// Type assert to get access to internal registry
	regImpl, ok := registry.(*registryImpl)
	if !ok {
		// Fallback: create a new registry
		regImpl = &registryImpl{
			hooks: make(map[string]Hook),
		}
	}

	return &Runner{
		registry: regImpl,
		executor: NewExecutor(validator),
	}
}

// RunHook executes a single hook
func (r *Runner) RunHook(ctx context.Context, hook Hook) (*VerificationResult, error) {
	return r.executor.Execute(ctx, hook)
}

// RunAll executes all hooks for an event in priority order with parallel execution
func (r *Runner) RunAll(ctx context.Context, event HookEvent) (*AggregatedReport, error) {
	hooks := r.registry.GetHooksByEvent(event)

	if len(hooks) == 0 {
		return &AggregatedReport{
			Timestamp: time.Now(),
			Event:     event,
			Results:   []VerificationResult{},
			Warnings:  []HookWarning{},
			Summary: Summary{
				TotalHooks:      0,
				PassedHooks:     0,
				FailedHooks:     0,
				WarningHooks:    0,
				TotalViolations: 0,
				ExitCode:        0,
			},
		}, nil
	}

	report := &AggregatedReport{
		Timestamp: time.Now(),
		Event:     event,
		Results:   make([]VerificationResult, 0, len(hooks)),
		Warnings:  make([]HookWarning, 0),
	}

	// Use semaphore to limit concurrent execution
	sem := make(chan struct{}, MaxConcurrentHooks)
	var wg sync.WaitGroup
	var mu sync.Mutex

	// Execute hooks in parallel
	for _, hook := range hooks {
		wg.Add(1)
		hook := hook // Capture loop variable

		go func() {
			defer wg.Done()

			// Acquire semaphore
			sem <- struct{}{}
			defer func() { <-sem }()

			// Execute hook
			result, err := r.RunHook(ctx, hook)
			if err != nil {
				// Graceful degradation - log warning and continue
				mu.Lock()
				report.Warnings = append(report.Warnings, HookWarning{
					Hook:    hook.Name,
					Message: fmt.Sprintf("Hook execution failed: %v", err),
				})
				mu.Unlock()

				// Still include result if available (e.g., for security violations)
				if result != nil {
					mu.Lock()
					report.Results = append(report.Results, *result)
					mu.Unlock()
				}
				return
			}

			// Add successful result
			mu.Lock()
			report.Results = append(report.Results, *result)
			mu.Unlock()
		}()
	}

	// Wait for all hooks to complete
	wg.Wait()

	// Calculate summary
	report.Summary = calculateSummary(report.Results, report.Warnings)

	return report, nil
}

// calculateSummary generates summary statistics from results
func calculateSummary(results []VerificationResult, warnings []HookWarning) Summary {
	summary := Summary{
		TotalHooks:   len(results),
		WarningHooks: len(warnings),
	}

	for _, result := range results {
		switch result.Status {
		case VerificationStatusPass:
			summary.PassedHooks++
		case VerificationStatusFail:
			summary.FailedHooks++
		case VerificationStatusWarning:
			summary.WarningHooks++
		}

		summary.TotalViolations += len(result.Violations)
	}

	// Determine exit code
	// 0 = all pass, 1 = any fail, 2 = warnings only
	if summary.FailedHooks > 0 {
		summary.ExitCode = 1
	} else if summary.WarningHooks > 0 || len(warnings) > 0 {
		summary.ExitCode = 2
	} else {
		summary.ExitCode = 0
	}

	return summary
}
