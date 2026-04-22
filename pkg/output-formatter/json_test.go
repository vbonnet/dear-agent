package outputformatter

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestJSONFormatter_Format(t *testing.T) {
	tests := []struct {
		name    string
		results []Result
		pretty  bool
		wantErr bool
	}{
		{
			name:    "empty results",
			results: []Result{},
			pretty:  false,
			wantErr: false,
		},
		{
			name: "single result compact",
			results: []Result{
				mockResult{StatusOK, "Check passed", "core"},
			},
			pretty:  false,
			wantErr: false,
		},
		{
			name: "multiple results pretty",
			results: []Result{
				mockResult{StatusOK, "Check 1", "core"},
				mockResult{StatusWarning, "Check 2", "deps"},
				mockResult{StatusError, "Check 3", "hooks"},
			},
			pretty:  true,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			formatter := NewJSONFormatter(tt.pretty)
			got, err := formatter.Format(tt.results)
			if (err != nil) != tt.wantErr {
				t.Errorf("Format() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			// Verify it's valid JSON
			var report ReportJSON
			if err := json.Unmarshal([]byte(got), &report); err != nil {
				t.Errorf("Format() produced invalid JSON: %v", err)
			}

			// Verify structure
			if len(report.Results) != len(tt.results) {
				t.Errorf("Format() produced %d results, want %d", len(report.Results), len(tt.results))
			}

			// If pretty, verify indentation
			if tt.pretty && !strings.Contains(got, "\n") {
				t.Errorf("Format() with pretty=true should have newlines")
			}
		})
	}
}

func TestJSONFormatter_FormatWithSummary(t *testing.T) {
	results := []Result{
		mockResult{StatusOK, "Check 1", "core"},
		mockResult{StatusWarning, "Check 2", "deps"},
		mockResult{StatusError, "Check 3", "hooks"},
	}

	gen := NewSummaryGenerator(NewIconMapper(false))
	summary := gen.Generate(results)

	formatter := NewJSONFormatter(true)
	got, err := formatter.FormatWithSummary(results, summary)
	if err != nil {
		t.Fatalf("FormatWithSummary() error = %v", err)
	}

	// Parse JSON
	var report ReportJSON
	if err := json.Unmarshal([]byte(got), &report); err != nil {
		t.Fatalf("FormatWithSummary() produced invalid JSON: %v", err)
	}

	// Verify summary fields
	if report.Summary.Total != 3 {
		t.Errorf("Summary.Total = %d, want 3", report.Summary.Total)
	}
	if report.Summary.Passed != 1 {
		t.Errorf("Summary.Passed = %d, want 1", report.Summary.Passed)
	}
	if report.Summary.Warnings != 1 {
		t.Errorf("Summary.Warnings = %d, want 1", report.Summary.Warnings)
	}
	if report.Summary.Errors != 1 {
		t.Errorf("Summary.Errors = %d, want 1", report.Summary.Errors)
	}
	if report.Summary.Status != "Critical" {
		t.Errorf("Summary.Status = %q, want %q", report.Summary.Status, "Critical")
	}
	if report.Summary.ExitCode != 2 {
		t.Errorf("Summary.ExitCode = %d, want 2", report.Summary.ExitCode)
	}
	if report.Summary.IsHealthy != false {
		t.Errorf("Summary.IsHealthy = %v, want false", report.Summary.IsHealthy)
	}

	// Verify results
	if len(report.Results) != 3 {
		t.Errorf("len(Results) = %d, want 3", len(report.Results))
	}

	// Verify first result
	if report.Results[0].Status != "ok" {
		t.Errorf("Results[0].Status = %q, want %q", report.Results[0].Status, "ok")
	}
	if report.Results[0].Message != "Check 1" {
		t.Errorf("Results[0].Message = %q, want %q", report.Results[0].Message, "Check 1")
	}
	if report.Results[0].Category != "core" {
		t.Errorf("Results[0].Category = %q, want %q", report.Results[0].Category, "core")
	}
}

func TestJSONFormatter_FormatSummaryOnly(t *testing.T) {
	summary := Summary{
		Total:    10,
		Passed:   7,
		Info:     1,
		Warnings: 2,
		Errors:   0,
	}

	tests := []struct {
		name   string
		pretty bool
	}{
		{
			name:   "compact format",
			pretty: false,
		},
		{
			name:   "pretty format",
			pretty: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			formatter := NewJSONFormatter(tt.pretty)
			got, err := formatter.FormatSummaryOnly(summary)
			if err != nil {
				t.Fatalf("FormatSummaryOnly() error = %v", err)
			}

			// Parse JSON
			var jsonSummary SummaryJSON
			if err := json.Unmarshal([]byte(got), &jsonSummary); err != nil {
				t.Fatalf("FormatSummaryOnly() produced invalid JSON: %v", err)
			}

			// Verify fields
			if jsonSummary.Total != 10 {
				t.Errorf("Total = %d, want 10", jsonSummary.Total)
			}
			if jsonSummary.Passed != 7 {
				t.Errorf("Passed = %d, want 7", jsonSummary.Passed)
			}
			if jsonSummary.Info != 1 {
				t.Errorf("Info = %d, want 1", jsonSummary.Info)
			}
			if jsonSummary.Warnings != 2 {
				t.Errorf("Warnings = %d, want 2", jsonSummary.Warnings)
			}
			if jsonSummary.Errors != 0 {
				t.Errorf("Errors = %d, want 0", jsonSummary.Errors)
			}
			if jsonSummary.Status != "Degraded" {
				t.Errorf("Status = %q, want %q", jsonSummary.Status, "Degraded")
			}
			if jsonSummary.ExitCode != 1 {
				t.Errorf("ExitCode = %d, want 1", jsonSummary.ExitCode)
			}
			if jsonSummary.IsHealthy != false {
				t.Errorf("IsHealthy = %v, want false", jsonSummary.IsHealthy)
			}

			// If pretty, verify indentation
			if tt.pretty && !strings.Contains(got, "\n") {
				t.Errorf("FormatSummaryOnly() with pretty=true should have newlines")
			}
		})
	}
}

func TestJSONFormatter_ResultJSON_Marshaling(t *testing.T) {
	result := ResultJSON{
		Status:   "ok",
		Message:  "Test message",
		Category: "core",
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	var unmarshaled ResultJSON
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	if unmarshaled.Status != result.Status {
		t.Errorf("Status = %q, want %q", unmarshaled.Status, result.Status)
	}
	if unmarshaled.Message != result.Message {
		t.Errorf("Message = %q, want %q", unmarshaled.Message, result.Message)
	}
	if unmarshaled.Category != result.Category {
		t.Errorf("Category = %q, want %q", unmarshaled.Category, result.Category)
	}
}
