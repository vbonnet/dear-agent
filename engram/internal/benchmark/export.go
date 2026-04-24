package benchmark

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"time"
)

// ExportMetadata contains information about the export operation.
type ExportMetadata struct {
	ExportedAt  string      `json:"exported_at"`
	QueryParams QueryParams `json:"query_params,omitempty"`
	ResultCount int         `json:"result_count"`
}

// ExportResult combines metadata with benchmark runs for JSON export.
type ExportResult struct {
	Metadata ExportMetadata `json:"metadata"`
	Results  []BenchmarkRun `json:"results"`
}

// ExportJSON exports benchmark runs as JSON with metadata.
func (s *Storage) ExportJSON(params QueryParams, w io.Writer) error {
	// Query runs
	runs, err := s.Query(params)
	if err != nil {
		return fmt.Errorf("query failed: %w", err)
	}

	// Build export result with metadata
	result := ExportResult{
		Metadata: ExportMetadata{
			ExportedAt:  time.Now().Format(time.RFC3339),
			QueryParams: params,
			ResultCount: len(runs),
		},
		Results: runs,
	}

	// Marshal to JSON with indentation
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(result); err != nil {
		return fmt.Errorf("failed to encode JSON: %w", err)
	}

	return nil
}

// ExportCSV exports benchmark runs as CSV.
// Note: Metadata JSON is omitted from CSV export (use JSON for complete data).
func (s *Storage) ExportCSV(params QueryParams, w io.Writer) error {
	// Query runs
	runs, err := s.Query(params)
	if err != nil {
		return fmt.Errorf("query failed: %w", err)
	}

	// Create CSV writer
	writer := csv.NewWriter(w)
	defer writer.Flush()

	// Write header
	header := []string{
		"run_id",
		"timestamp",
		"variant",
		"project_size",
		"project_name",
		"quality_score",
		"cost_usd",
		"tokens_input",
		"tokens_output",
		"duration_ms",
		"file_count",
		"documentation_kb",
		"phases_completed",
		"successful",
	}
	if err := writer.Write(header); err != nil {
		return fmt.Errorf("failed to write CSV header: %w", err)
	}

	// Write rows
	for _, run := range runs {
		row := []string{
			run.RunID,
			run.Timestamp.Format(time.RFC3339),
			run.Variant,
			run.ProjectSize,
			run.ProjectName,
			formatFloat(run.QualityScore),
			formatFloat(run.CostUSD),
			formatInt64(run.TokensInput),
			formatInt64(run.TokensOutput),
			formatInt64(run.DurationMs),
			formatInt(run.FileCount),
			formatFloat(run.DocumentationKB),
			formatInt(run.PhasesCompleted),
			formatBool(run.Successful),
		}
		if err := writer.Write(row); err != nil {
			return fmt.Errorf("failed to write CSV row: %w", err)
		}
	}

	return writer.Error()
}

// Helper functions for CSV formatting

func formatFloat(val *float64) string {
	if val == nil {
		return ""
	}
	return fmt.Sprintf("%.2f", *val)
}

func formatInt64(val *int64) string {
	if val == nil {
		return ""
	}
	return fmt.Sprintf("%d", *val)
}

func formatInt(val *int) string {
	if val == nil {
		return ""
	}
	return fmt.Sprintf("%d", *val)
}

func formatBool(val bool) string {
	if val {
		return "true"
	}
	return "false"
}
