// Package healthchecker provides a framework for implementing CLI health check (doctor) commands.
//
// This package consolidates duplicated health check patterns from engram and ai-tools,
// providing interfaces for checks, auto-fix operations, and result aggregation.
//
// Core Concepts:
//   - Check: Interface for implementing health checks
//   - Result: Outcome of a check with status, message, and optional fix
//   - Runner: Executes checks sequentially or in parallel
//   - Fixer: Applies auto-fix operations to fixable results
//
// Example Usage:
//
//	// Define a custom check
//	type WorkspaceCheck struct {
//	    path string
//	}
//
//	func (c WorkspaceCheck) Name() string     { return "workspace_exists" }
//	func (c WorkspaceCheck) Category() string { return "core" }
//
//	func (c WorkspaceCheck) Run(ctx context.Context) healthchecker.Result {
//	    if _, err := os.Stat(c.path); os.IsNotExist(err) {
//	        return healthchecker.Result{
//	            Name:     c.Name(),
//	            Category: c.Category(),
//	            Status:   healthchecker.StatusError,
//	            Message:  "Workspace directory missing",
//	            Fixable:  true,
//	            Fix: &healthchecker.Fix{
//	                Name:        "Create workspace",
//	                Description: "Creates the workspace directory",
//	                Command:     "mkdir -p " + c.path,
//	                Apply: func(ctx context.Context) error {
//	                    return os.MkdirAll(c.path, 0755)
//	                },
//	                Reversible: true,
//	            },
//	        }
//	    }
//	    return healthchecker.Result{
//	        Name:     c.Name(),
//	        Category: c.Category(),
//	        Status:   healthchecker.StatusOK,
//	    }
//	}
//
//	// Use the framework
//	func main() {
//	    checks := []healthchecker.Check{
//	        WorkspaceCheck{path: "~/.myapp"},
//	        // Add more checks...
//	    }
//
//	    runner := healthchecker.NewRunner(checks...)
//	    results, _ := runner.RunAll(context.Background())
//
//	    summary := healthchecker.Summarize(results)
//	    fmt.Printf("Status: %s\n", summary.OverallStatus())
//
//	    if summary.Fixable > 0 {
//	        fixer := healthchecker.NewFixer()
//	        applied, updated, _ := fixer.Apply(context.Background(), results)
//	        fmt.Printf("Applied %d fixes\n", applied)
//	    }
//
//	    os.Exit(summary.ExitCode())
//	}
package healthchecker
