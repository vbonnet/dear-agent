package outputformatter

import (
	"encoding/json"
	"fmt"
)

// JSONFormatter formats results as JSON
type JSONFormatter struct {
	Pretty bool // Enable pretty-printing with indentation
}

// NewJSONFormatter creates a new JSON formatter
func NewJSONFormatter(pretty bool) *JSONFormatter {
	return &JSONFormatter{Pretty: pretty}
}

// ResultJSON represents a result in JSON format
type ResultJSON struct {
	Status   string `json:"status"`
	Message  string `json:"message"`
	Category string `json:"category"`
}

// SummaryJSON represents a summary in JSON format
type SummaryJSON struct {
	Total     int    `json:"total"`
	Passed    int    `json:"passed"`
	Info      int    `json:"info"`
	Warnings  int    `json:"warnings"`
	Errors    int    `json:"errors"`
	Unknown   int    `json:"unknown,omitempty"`
	Status    string `json:"status"`
	ExitCode  int    `json:"exit_code"`
	IsHealthy bool   `json:"is_healthy"`
}

// ReportJSON represents a complete report with results and summary
type ReportJSON struct {
	Summary SummaryJSON  `json:"summary"`
	Results []ResultJSON `json:"results"`
}

// Format formats a collection of results as JSON
func (f *JSONFormatter) Format(results []Result) (string, error) {
	return f.FormatWithSummary(results, Summary{})
}

// FormatWithSummary formats results and summary as JSON
func (f *JSONFormatter) FormatWithSummary(results []Result, summary Summary) (string, error) {
	// Convert results to JSON format
	jsonResults := make([]ResultJSON, len(results))
	for i, r := range results {
		jsonResults[i] = ResultJSON{
			Status:   string(r.Status()),
			Message:  r.Message(),
			Category: r.Category(),
		}
	}

	// Convert summary to JSON format
	jsonSummary := SummaryJSON{
		Total:     summary.Total,
		Passed:    summary.Passed,
		Info:      summary.Info,
		Warnings:  summary.Warnings,
		Errors:    summary.Errors,
		Unknown:   summary.Unknown,
		Status:    summary.OverallStatus(),
		ExitCode:  summary.ExitCode(),
		IsHealthy: summary.IsHealthy(),
	}

	// Create report
	report := ReportJSON{
		Summary: jsonSummary,
		Results: jsonResults,
	}

	// Marshal to JSON
	var data []byte
	var err error
	if f.Pretty {
		data, err = json.MarshalIndent(report, "", "  ")
	} else {
		data, err = json.Marshal(report)
	}

	if err != nil {
		return "", fmt.Errorf("marshal JSON: %w", err)
	}

	return string(data), nil
}

// FormatSummaryOnly formats only the summary as JSON
func (f *JSONFormatter) FormatSummaryOnly(summary Summary) (string, error) {
	jsonSummary := SummaryJSON{
		Total:     summary.Total,
		Passed:    summary.Passed,
		Info:      summary.Info,
		Warnings:  summary.Warnings,
		Errors:    summary.Errors,
		Unknown:   summary.Unknown,
		Status:    summary.OverallStatus(),
		ExitCode:  summary.ExitCode(),
		IsHealthy: summary.IsHealthy(),
	}

	var data []byte
	var err error
	if f.Pretty {
		data, err = json.MarshalIndent(jsonSummary, "", "  ")
	} else {
		data, err = json.Marshal(jsonSummary)
	}

	if err != nil {
		return "", fmt.Errorf("marshal JSON: %w", err)
	}

	return string(data), nil
}
