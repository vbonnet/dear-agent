package dashboard

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/internal/telemetry/agent"
)

func TestFormatSpecificityTable_Markdown(t *testing.T) {
	metrics := []SpecificityMetric{
		{Level: "High (>0.7)", Total: 120, Successes: 102, SuccessRate: 85.0},
		{Level: "Medium (0.4-0.7)", Total: 80, Successes: 50, SuccessRate: 62.5},
		{Level: "Low (<0.4)", Total: 50, Successes: 19, SuccessRate: 38.0},
	}

	output, err := FormatSpecificityTable(metrics, "markdown")
	if err != nil {
		t.Fatalf("FormatSpecificityTable() error = %v", err)
	}

	// Verify output contains expected elements
	if !strings.Contains(output, "Success Rate by Prompt Specificity") {
		t.Error("Expected title not found in output")
	}

	if !strings.Contains(output, "High (>0.7)") {
		t.Error("Expected High specificity level not found")
	}

	if !strings.Contains(output, "85.0%") {
		t.Error("Expected success rate not found")
	}

	// Check for standard Markdown table pipe characters
	if !strings.Contains(output, "|") {
		t.Error("Expected Markdown table pipe characters not found")
	}
}

func TestFormatExampleTable_CSV(t *testing.T) {
	metrics := []ExampleMetric{
		{Status: "With Examples", Total: 150, Successes: 118, SuccessRate: 78.7},
		{Status: "Without Examples", Total: 100, Successes: 51, SuccessRate: 51.0},
	}

	output, err := FormatExampleTable(metrics, "csv")
	if err != nil {
		t.Fatalf("FormatExampleTable() error = %v", err)
	}

	// Verify CSV format
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) < 3 {
		t.Errorf("Expected at least 3 lines (header + 2 data), got %d", len(lines))
	}

	// Check header
	if !strings.Contains(lines[0], "Example Status") {
		t.Error("Expected CSV header not found")
	}

	// Check data
	if !strings.Contains(output, "With Examples") {
		t.Error("Expected data not found in CSV")
	}
}

func TestFormatEfficiencyTable_JSON(t *testing.T) {
	metrics := []EfficiencyMetric{
		{PromptType: "Specific (>0.7)", AvgTokens: 1205, AvgRetries: 0.3, SuccessRate: 85.0},
		{PromptType: "Vague (<=0.7)", AvgTokens: 982, AvgRetries: 1.8, SuccessRate: 38.0},
	}

	output, err := FormatEfficiencyTable(metrics, "json")
	if err != nil {
		t.Fatalf("FormatEfficiencyTable() error = %v", err)
	}

	// Verify JSON structure (field names are capitalized in JSON output)
	if !strings.Contains(output, "Specific") {
		t.Error("Expected prompt type 'Specific' not found in JSON")
	}

	if !strings.Contains(output, "[") || !strings.Contains(output, "]") {
		t.Error("Expected JSON array markers not found")
	}
}

func TestFormatTrendTable_Markdown(t *testing.T) {
	metrics := []TrendMetric{
		{Date: "2026-01-14", TotalLaunches: 25, Successes: 21, SuccessRate: 84.0},
		{Date: "2026-01-13", TotalLaunches: 32, Successes: 27, SuccessRate: 84.4},
	}

	output, err := FormatTrendTable(metrics, "markdown")
	if err != nil {
		t.Fatalf("FormatTrendTable() error = %v", err)
	}

	// Verify output
	if !strings.Contains(output, "Trends Over Time") {
		t.Error("Expected title not found")
	}

	if !strings.Contains(output, "2026-01-14") {
		t.Error("Expected date not found")
	}

	if !strings.Contains(output, "84.0%") {
		t.Error("Expected success rate not found")
	}
}

func TestDisplay_InvalidMetric(t *testing.T) {
	// Test validates metric before opening database, so any path works
	opts := DisplayOptions{
		Metric: "invalid",
		Format: "table",
		DBPath: "/any/path",
	}

	err := Display(opts)
	if err == nil {
		t.Error("Expected error for invalid metric, got nil")
	}

	if !strings.Contains(err.Error(), "invalid metric") {
		t.Errorf("Expected 'invalid metric' error, got: %v", err)
	}
}

func TestDisplay_MissingDatabase(t *testing.T) {
	opts := DisplayOptions{
		Metric: "all",
		Format: "table",
		DBPath: "/nonexistent/path/telemetry.db",
	}

	err := Display(opts)
	if err == nil {
		t.Error("Expected error for missing database, got nil")
	}

	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("Expected 'not found' error, got: %v", err)
	}
}

func TestCheckDataExists_EmptyDatabase(t *testing.T) {
	// Create empty database
	storage, err := agent.NewStorageAt(":memory:")
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	defer storage.Close()

	err = checkDataExists(storage)
	if err == nil {
		t.Error("Expected error for empty database, got nil")
	}

	if !strings.Contains(err.Error(), "no agent launches found") {
		t.Errorf("Expected 'no agent launches found' error, got: %v", err)
	}
}

func TestDisplaySpecificity(t *testing.T) {
	// Create test database with data
	storage, err := agent.NewStorageAt(":memory:")
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	defer storage.Close()

	// Insert test data
	ctx := context.Background()
	features := agent.Features{
		WordCount:             50,
		TokenCount:            1200,
		SpecificityScore:      0.8,
		HasExamples:           true,
		HasConstraints:        true,
		ContextEmbeddingScore: 0.9,
	}
	id, _ := storage.LogLaunch(ctx, "Test prompt", "claude-sonnet-4.5", features)
	storage.UpdateOutcome(ctx, id, "success", 1500)

	// Test displaySpecificity
	opts := DisplayOptions{
		Format: "table",
	}

	err = displaySpecificity(ctx, storage, opts)
	if err != nil {
		t.Errorf("displaySpecificity() error = %v", err)
	}
}

func TestDisplayExamples_EmptyResults(t *testing.T) {
	// Create empty database
	storage, err := agent.NewStorageAt(":memory:")
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	defer storage.Close()

	// Test displayExamples with empty database
	ctx := context.Background()
	opts := DisplayOptions{
		Format: "table",
		Since:  time.Now(), // Filter to future (empty results)
	}

	err = displayExamples(ctx, storage, opts)
	// Should not error, just show "No data available"
	if err != nil {
		t.Errorf("displayExamples() should not error on empty results, got: %v", err)
	}
}

func TestFormatCSV(t *testing.T) {
	headers := []string{"Name", "Value"}
	rows := [][]string{
		{"Test1", "100"},
		{"Test2", "200"},
	}

	output := formatCSV(headers, rows)

	// Verify CSV format
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) != 3 {
		t.Errorf("Expected 3 lines (header + 2 data), got %d", len(lines))
	}

	// Check header
	if !strings.Contains(lines[0], "Name") {
		t.Error("Expected CSV header not found")
	}

	// Check data
	if !strings.Contains(output, "Test1") || !strings.Contains(output, "100") {
		t.Error("Expected CSV data not found")
	}
}

func TestDisplayAllMetrics(t *testing.T) {
	// Create test database with data
	storage, err := agent.NewStorageAt(":memory:")
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	defer storage.Close()

	// Insert test data
	ctx := context.Background()
	features := agent.Features{
		WordCount:             50,
		TokenCount:            1200,
		SpecificityScore:      0.8,
		HasExamples:           true,
		HasConstraints:        true,
		ContextEmbeddingScore: 0.9,
	}
	id, _ := storage.LogLaunch(ctx, "Test prompt", "claude-sonnet-4.5", features)
	storage.UpdateOutcome(ctx, id, "success", 1500)

	// Test displayAllMetrics
	opts := DisplayOptions{
		Format: "table",
	}

	err = displayAllMetrics(ctx, storage, opts)
	if err != nil {
		t.Errorf("displayAllMetrics() error = %v", err)
	}
}

func TestDisplayEfficiency(t *testing.T) {
	// Create test database with data
	storage, err := agent.NewStorageAt(":memory:")
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	defer storage.Close()

	// Insert test data
	ctx := context.Background()
	features := agent.Features{
		WordCount:             50,
		TokenCount:            1200,
		SpecificityScore:      0.8,
		HasExamples:           true,
		HasConstraints:        true,
		ContextEmbeddingScore: 0.9,
	}
	id, _ := storage.LogLaunch(ctx, "Test prompt", "claude-sonnet-4.5", features)
	storage.UpdateOutcome(ctx, id, "success", 1500)

	// Test displayEfficiency
	opts := DisplayOptions{
		Format: "table",
	}

	err = displayEfficiency(ctx, storage, opts)
	if err != nil {
		t.Errorf("displayEfficiency() error = %v", err)
	}
}

func TestDisplayTrends(t *testing.T) {
	// Create test database with data
	storage, err := agent.NewStorageAt(":memory:")
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	defer storage.Close()

	// Insert test data
	ctx := context.Background()
	features := agent.Features{
		WordCount:             50,
		TokenCount:            1200,
		SpecificityScore:      0.8,
		HasExamples:           true,
		HasConstraints:        true,
		ContextEmbeddingScore: 0.9,
	}
	id, _ := storage.LogLaunch(ctx, "Test prompt", "claude-sonnet-4.5", features)
	storage.UpdateOutcome(ctx, id, "success", 1500)

	// Test displayTrends
	opts := DisplayOptions{
		Format: "table",
	}

	err = displayTrends(ctx, storage, opts)
	if err != nil {
		t.Errorf("displayTrends() error = %v", err)
	}
}

func TestFormatSpecificityTable_AllFormats(t *testing.T) {
	metrics := []SpecificityMetric{
		{Level: "High (>0.7)", Total: 120, Successes: 102, SuccessRate: 85.0},
	}

	// Test all formats
	formats := []string{"table", "markdown", "csv", "json"}
	for _, format := range formats {
		output, err := FormatSpecificityTable(metrics, format)
		if err != nil {
			t.Errorf("FormatSpecificityTable(%s) error = %v", format, err)
		}
		if output == "" {
			t.Errorf("FormatSpecificityTable(%s) returned empty output", format)
		}
	}
}

func TestFormatExampleTable_AllFormats(t *testing.T) {
	metrics := []ExampleMetric{
		{Status: "With Examples", Total: 150, Successes: 118, SuccessRate: 78.7},
	}

	formats := []string{"table", "csv", "json"}
	for _, format := range formats {
		output, err := FormatExampleTable(metrics, format)
		if err != nil {
			t.Errorf("FormatExampleTable(%s) error = %v", format, err)
		}
		if output == "" {
			t.Errorf("FormatExampleTable(%s) returned empty output", format)
		}
	}
}

func TestFormatEfficiencyTable_AllFormats(t *testing.T) {
	metrics := []EfficiencyMetric{
		{PromptType: "Specific", AvgTokens: 1200, AvgRetries: 0.3, SuccessRate: 85.0},
	}

	formats := []string{"table", "csv", "json"}
	for _, format := range formats {
		output, err := FormatEfficiencyTable(metrics, format)
		if err != nil {
			t.Errorf("FormatEfficiencyTable(%s) error = %v", format, err)
		}
		if output == "" {
			t.Errorf("FormatEfficiencyTable(%s) returned empty output", format)
		}
	}
}

func TestFormatTrendTable_AllFormats(t *testing.T) {
	metrics := []TrendMetric{
		{Date: "2026-01-14", TotalLaunches: 25, Successes: 21, SuccessRate: 84.0},
	}

	formats := []string{"table", "csv", "json"}
	for _, format := range formats {
		output, err := FormatTrendTable(metrics, format)
		if err != nil {
			t.Errorf("FormatTrendTable(%s) error = %v", format, err)
		}
		if output == "" {
			t.Errorf("FormatTrendTable(%s) returned empty output", format)
		}
	}
}

func TestOpenStorage_DefaultPath(t *testing.T) {
	// Test with empty path (should use default)
	_, err := openStorage("")
	// Will fail if database doesn't exist, which is expected
	if err != nil && !strings.Contains(err.Error(), "not found") {
		t.Errorf("Expected 'not found' error for default path, got: %v", err)
	}
}

func TestFileExists(t *testing.T) {
	// Test with non-existent file
	if fileExists("/nonexistent/file/path") {
		t.Error("fileExists() returned true for non-existent file")
	}

	// Test with existing file (create a temp file)
	storage, err := agent.NewStorageAt("/tmp/test-exists.db")
	if err != nil {
		t.Fatalf("failed to create temp database: %v", err)
	}
	storage.Close()

	if !fileExists("/tmp/test-exists.db") {
		t.Error("fileExists() returned false for existing file")
	}
}
