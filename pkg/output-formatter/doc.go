// Package outputformatter provides shared utilities for formatting status results
// with icons, summaries, and multiple output formats.
//
// This package consolidates duplicated formatting patterns across engram and ai-tools
// repositories, providing consistent output styling, icon mapping, and summary generation.
//
// Core concepts:
//   - Result: Interface for formattable result items with status, message, and category
//   - StatusLevel: Enum for ok/warning/error/info status levels
//   - IconMapper: Converts status levels to emoji or plain text icons
//   - SummaryGenerator: Aggregates results and generates summary statistics
//   - JSONFormatter: Formats results and summaries as JSON
//
// Example usage:
//
//	// Create your result type implementing the Result interface
//	type HealthCheck struct {
//	    status   outputformatter.StatusLevel
//	    message  string
//	    category string
//	}
//
//	func (h HealthCheck) Status() outputformatter.StatusLevel { return h.status }
//	func (h HealthCheck) Message() string                     { return h.message }
//	func (h HealthCheck) Category() string                    { return h.category }
//
//	// Use the formatters
//	checks := []outputformatter.Result{
//	    HealthCheck{outputformatter.StatusOK, "Config valid", "core"},
//	    HealthCheck{outputformatter.StatusWarning, "Low disk space", "resources"},
//	}
//
//	iconMapper := outputformatter.NewIconMapper(false) // Use emoji icons
//	summaryGen := outputformatter.NewSummaryGenerator(iconMapper)
//
//	summary := summaryGen.Generate(checks)
//	fmt.Println(summaryGen.Format(summary))
//
//	// Output:
//	//   ✅ 1 checks passed
//	//   ⚠️  1 warnings
package outputformatter
