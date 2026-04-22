# output-formatter

Shared output formatting library with status icons, summaries, and multiple output formats.

## Features

- 📊 **Status Icons** - Consistent emoji or plain text status indicators
- 📈 **Summary Generation** - Aggregate result counts by status level
- 📄 **Multiple Formats** - JSON, text, compact summaries
- ♿ **Accessibility** - `--no-color` mode with plain text icons
- ⚡ **Zero dependencies** - Pure Go standard library
- ✅ **Well tested** - Comprehensive test coverage

## Installation

```bash
go get github.com/vbonnet/engram/libs/output-formatter
```

## Quick Start

### 1. Implement the Result Interface

```go
import "github.com/vbonnet/engram/libs/output-formatter"

type HealthCheck struct {
    status   outputformatter.StatusLevel
    message  string
    category string
}

func (h HealthCheck) Status() outputformatter.StatusLevel { return h.status }
func (h HealthCheck) Message() string                     { return h.message }
func (h HealthCheck) Category() string                    { return h.category }
```

### 2. Create and Format Results

```go
// Create results
checks := []outputformatter.Result{
    HealthCheck{outputformatter.StatusOK, "Config valid", "core"},
    HealthCheck{outputformatter.StatusWarning, "Low disk space", "resources"},
    HealthCheck{outputformatter.StatusError, "Database unreachable", "services"},
}

// Generate summary
iconMapper := outputformatter.NewIconMapper(false) // Use emoji icons
summaryGen := outputformatter.NewSummaryGenerator(iconMapper)
summary := summaryGen.Generate(checks)

// Format output
fmt.Println("Summary:")
fmt.Println(summaryGen.Format(summary))
fmt.Println()
fmt.Printf("Overall Status: %s\n", summary.OverallStatus())
fmt.Printf("Exit Code: %d\n", summary.ExitCode())
```

**Output:**
```
Summary:
  ✅ 1 checks passed
  ⚠️  1 warnings
  ❌ 1 errors

Overall Status: Critical
Exit Code: 2
```

### 3. JSON Output

```go
jsonFormatter := outputformatter.NewJSONFormatter(true) // Pretty print
output, _ := jsonFormatter.FormatWithSummary(checks, summary)
fmt.Println(output)
```

**Output:**
```json
{
  "summary": {
    "total": 3,
    "passed": 1,
    "info": 0,
    "warnings": 1,
    "errors": 1,
    "status": "Critical",
    "exit_code": 2,
    "is_healthy": false
  },
  "results": [
    {
      "status": "ok",
      "message": "Config valid",
      "category": "core"
    },
    {
      "status": "warning",
      "message": "Low disk space",
      "category": "resources"
    },
    {
      "status": "error",
      "message": "Database unreachable",
      "category": "services"
    }
  ]
}
```

## API Reference

### Status Levels

```go
const (
    StatusOK       StatusLevel = "ok"       // Success
    StatusSuccess  StatusLevel = "success"  // Alias for ok
    StatusInfo     StatusLevel = "info"     // Informational
    StatusWarning  StatusLevel = "warning"  // Warning
    StatusError    StatusLevel = "error"    // Error
    StatusFailed   StatusLevel = "failed"   // Alias for error
    StatusUnknown  StatusLevel = "unknown"  // Unknown
)
```

### Result Interface

```go
type Result interface {
    Status() StatusLevel    // Status level
    Message() string        // Human-readable message
    Category() string       // Category/group
}
```

### IconMapper

Maps status levels to visual icons.

```go
// Create mapper
iconMapper := outputformatter.NewIconMapper(false) // Emoji icons
iconMapper := outputformatter.NewIconMapper(true)  // Plain text icons

// Get icon
icon := iconMapper.GetIcon(outputformatter.StatusOK)       // "✅" or "[OK]"
icon := iconMapper.GetIcon(outputformatter.StatusWarning)  // "⚠️ " or "[WARN]"

// Format with icon
text := iconMapper.FormatWithIcon(outputformatter.StatusOK, "All systems operational")
// "✅ All systems operational" or "[OK] All systems operational"
```

**Icon Mapping:**

| Status | Emoji | Plain Text |
|--------|-------|------------|
| ok/success | ✅ | [OK] |
| info | ℹ️  | [INFO] |
| warning | ⚠️  | [WARN] |
| error/failed | ❌ | [ERROR] |
| unknown | ❓ | [?] |

### SummaryGenerator

Aggregates results and generates summaries.

```go
gen := outputformatter.NewSummaryGenerator(iconMapper)

// Generate summary from results
summary := gen.Generate(results)

// Format as multi-line text
text := gen.Format(summary)
//   ✅ 5 checks passed
//   ⚠️  2 warnings

// Format as single-line compact text
compact := gen.FormatCompact(summary)
// "5 passed, 2 warnings"
```

### Summary Methods

```go
summary := Summary{Passed: 5, Warnings: 2, Errors: 1}

summary.IsHealthy()      // false (has warnings/errors)
summary.HasIssues()      // true (has warnings or errors)
summary.ExitCode()       // 2 (0=healthy, 1=warnings, 2=errors)
summary.OverallStatus()  // "Critical" (Healthy/Degraded/Critical/Unknown)
```

### JSONFormatter

Formats results and summaries as JSON.

```go
formatter := outputformatter.NewJSONFormatter(true) // Pretty print
formatter := outputformatter.NewJSONFormatter(false) // Compact

// Format results with auto-generated summary
json, _ := formatter.Format(results)

// Format results with existing summary
json, _ := formatter.FormatWithSummary(results, summary)

// Format only summary (no results)
json, _ := formatter.FormatSummaryOnly(summary)
```

### Helper Functions

```go
// Filter results to only warnings and errors
issues := outputformatter.GetIssues(results)
```

## Accessibility Support

Enable `--no-color` mode for accessibility:

```go
// Detect from flag or environment variable
noColor := os.Getenv("NO_COLOR") != "" || flagNoColor

// Create icon mapper with accessibility mode
iconMapper := outputformatter.NewIconMapper(noColor)
```

**Output comparison:**

| Mode | Output |
|------|--------|
| Emoji | `✅ Config valid` |
| Plain Text | `[OK] Config valid` |

## Use Cases

### Health Check Formatting

```go
type HealthCheck struct {
    status   outputformatter.StatusLevel
    message  string
    category string
}

// Implement Result interface...

checks := runHealthChecks()
summary := summaryGen.Generate(checks)

if !summary.IsHealthy() {
    fmt.Fprintf(os.Stderr, "Health check failed: %s\n", summary.OverallStatus())
    os.Exit(summary.ExitCode())
}
```

### Test Result Formatting

```go
type TestResult struct {
    status   outputformatter.StatusLevel
    message  string
    category string
}

// Implement Result interface...

results := runTests()
summary := summaryGen.Generate(results)

fmt.Println(summaryGen.FormatCompact(summary))
// "10 passed, 2 warnings"
```

### Analytics Reporting

```go
type AnalyticsResult struct {
    status   outputformatter.StatusLevel
    message  string
    category string
}

// Generate analytics report with JSON output
jsonFormatter := outputformatter.NewJSONFormatter(true)
output, _ := jsonFormatter.FormatWithSummary(results, summary)

// Export to file for analysis
os.WriteFile("report.json", []byte(output), 0644)
```

## Migration Guide

### From Custom Formatters

**Before:**
```go
// Custom icon mapping
func getIcon(status string) string {
    switch status {
    case "ok": return "✓"
    case "warning": return "⚠️ "
    case "error": return "❌"
    default: return "?"
    }
}

// Custom summary
type Summary struct {
    Passed int
    Failed int
}
```

**After:**
```go
import "github.com/vbonnet/engram/libs/output-formatter"

// Use shared library
iconMapper := outputformatter.NewIconMapper(false)
summaryGen := outputformatter.NewSummaryGenerator(iconMapper)

summary := summaryGen.Generate(results)
```

**Benefits:**
- ✅ Consistent icon set across tools
- ✅ Built-in accessibility support
- ✅ Standardized exit codes
- ✅ JSON formatting included
- ✅ Comprehensive testing

## Requirements

- Go 1.21 or later
- No external dependencies (pure standard library)

## Testing

```bash
# Run tests
go test -v

# Run tests with coverage
go test -v -cover

# Generate coverage report
go test -coverprofile=coverage.out
go tool cover -html=coverage.out
```

## License

Same as parent engram project.

## Contributing

This library is part of the engram monorepo. See the main engram repository for contribution guidelines.
