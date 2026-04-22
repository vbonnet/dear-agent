# health-checker

Framework for implementing CLI health check (doctor) commands with auto-fix capabilities.

## Features

- 🔍 **Check Interface** - Simple interface for implementing health checks
- 🔧 **Auto-Fix** - Built-in support for automatic remediation
- 📊 **Summary Generation** - Aggregate statistics and exit codes
- ⚡ **Parallel Execution** - Optional parallel check execution
- 🎯 **Categorization** - Group checks by category (core, dependency, etc.)
- ✅ **Well tested** - 92.3% test coverage

## Installation

```bash
go get github.com/vbonnet/engram/libs/health-checker
```

## Quick Start

### 1. Implement a Check

```go
import "github.com/vbonnet/engram/libs/health-checker"

type WorkspaceCheck struct {
    path string
}

func (c WorkspaceCheck) Name() string     { return "workspace_exists" }
func (c WorkspaceCheck) Category() string { return "core" }

func (c WorkspaceCheck) Run(ctx context.Context) healthchecker.Result {
    if _, err := os.Stat(c.path); os.IsNotExist(err) {
        return healthchecker.Result{
            Name:     c.Name(),
            Category: c.Category(),
            Status:   healthchecker.StatusError,
            Message:  "Workspace directory missing",
            Fixable:  true,
            Fix: &healthchecker.Fix{
                Name:        "Create workspace",
                Description: "Creates workspace directory",
                Apply: func(ctx context.Context) error {
                    return os.MkdirAll(c.path, 0755)
                },
                Reversible: true,
            },
        }
    }
    return healthchecker.Result{
        Name:     c.Name(),
        Category: c.Category(),
        Status:   healthchecker.StatusOK,
    }
}
```

### 2. Run Checks and Apply Fixes

```go
func main() {
    checks := []healthchecker.Check{
        WorkspaceCheck{path: "~/.myapp"},
    }

    runner := healthchecker.NewRunner(checks...)
    results, _ := runner.RunAll(context.Background())

    summary := healthchecker.Summarize(results)
    fmt.Printf("Status: %s\n", summary.OverallStatus())

    if summary.Fixable > 0 {
        fixer := healthchecker.NewFixer()
        applied, updated, _ := fixer.Apply(context.Background(), results)
        fmt.Printf("Applied %d fixes\n", applied)
    }

    os.Exit(summary.ExitCode())
}
```

## API Reference

### Status Levels

```go
const (
    StatusOK      Status = "ok"      // Check passed
    StatusInfo    Status = "info"    // Informational
    StatusWarning Status = "warning" // Warning
    StatusError   Status = "error"   // Error
)
```

### Check Interface

```go
type Check interface {
    Name() string
    Category() string
    Run(ctx context.Context) Result
}
```

### Runner

```go
runner := healthchecker.NewRunner(checks...)
results, err := runner.RunAll(ctx)

// Parallel execution
runner := healthchecker.NewRunner(checks...).WithParallel(true)
```

### Summary

```go
summary := healthchecker.Summarize(results)

summary.Total       // Total checks
summary.Passed      // OK/Info count
summary.Warnings    // Warning count
summary.Errors      // Error count
summary.Fixable     // Fixable count

summary.ExitCode()  // 0=healthy, 1=warnings, 2=errors
```

### Fixer

```go
fixer := healthchecker.NewFixer()

// Preview fixable issues
fixable := fixer.Preview(results)

// Apply all fixes
applied, updated, err := fixer.Apply(ctx, results)

// Dry-run mode
fixer := healthchecker.NewFixer().WithDryRun(true)
```

## Exit Codes

| Code | Status | Description |
|------|--------|-------------|
| 0 | Healthy | All checks passed |
| 1 | Degraded | Warnings present |
| 2 | Critical | Errors present |

## Requirements

- Go 1.21 or later
- No external dependencies

## Testing

```bash
go test -v -cover
```

## License

Same as parent engram project.
