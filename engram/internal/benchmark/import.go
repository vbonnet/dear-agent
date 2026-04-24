package benchmark

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ImportFromMarkdown parses markdown tables and inserts benchmark runs.
// If dryRun is true, validates and prints summary without inserting.
func ImportFromMarkdown(storage *Storage, dir string, dryRun bool) error {
	// Find markdown files
	files, err := filepath.Glob(filepath.Join(dir, "*.md"))
	if err != nil {
		return fmt.Errorf("failed to glob markdown files: %w", err)
	}

	if len(files) == 0 {
		return fmt.Errorf("no markdown files found in %s", dir)
	}

	var allRuns []BenchmarkRun

	for _, file := range files {
		content, err := os.ReadFile(file)
		if err != nil {
			return fmt.Errorf("failed to read %s: %w", file, err)
		}

		// Parse tables from file
		tables := parseMarkdownTables(string(content))

		// Extract benchmark runs from tables
		for _, table := range tables {
			runs, err := extractBenchmarkRuns(table)
			if err != nil {
				// Skip tables that don't contain benchmark data
				continue
			}
			allRuns = append(allRuns, runs...)
		}
	}

	// Validate runs
	for _, run := range allRuns {
		if err := validateRun(run); err != nil {
			return fmt.Errorf("validation failed for run %s: %w", run.RunID, err)
		}
	}

	// Dry run mode
	if dryRun {
		fmt.Printf("Dry run: Found %d benchmark runs\n", len(allRuns))
		for _, run := range allRuns {
			fmt.Printf("  - %s: %s / %s / %s\n", run.RunID, run.Variant, run.ProjectSize, run.ProjectName)
		}
		return nil
	}

	// Insert runs
	inserted := 0
	for _, run := range allRuns {
		if err := storage.InsertRun(run); err != nil {
			// Skip duplicates (run_id UNIQUE constraint)
			if strings.Contains(err.Error(), "UNIQUE constraint failed") {
				continue
			}
			return fmt.Errorf("failed to insert run %s: %w", run.RunID, err)
		}
		inserted++
	}

	fmt.Printf("Imported %d runs (%d skipped as duplicates)\n", inserted, len(allRuns)-inserted)
	return nil
}

// parseMarkdownTables extracts tables from markdown content.
// Returns slice of tables, where each table is a slice of row maps (column -> value).
func parseMarkdownTables(content string) [][]map[string]string {
	lines := strings.Split(content, "\n")
	var tables [][]map[string]string
	var currentTable []map[string]string
	var headers []string
	inTable := false

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Check if line is a table row
		if !strings.HasPrefix(trimmed, "|") {
			if inTable && len(currentTable) > 0 {
				tables = append(tables, currentTable)
				currentTable = nil
				headers = nil
				inTable = false
			}
			continue
		}

		// Parse table row
		cells := parsePipeSeparatedRow(trimmed)
		if len(cells) == 0 {
			continue
		}

		// First row = headers
		if !inTable {
			headers = cells
			inTable = true
			continue
		}

		// Second row = separator (skip)
		if i > 0 && isSeparatorRow(trimmed) {
			continue
		}

		// Data row
		if len(headers) > 0 {
			row := make(map[string]string)
			for j, cell := range cells {
				if j < len(headers) {
					row[headers[j]] = strings.TrimSpace(cell)
				}
			}
			currentTable = append(currentTable, row)
		}
	}

	// Add final table if exists
	if len(currentTable) > 0 {
		tables = append(tables, currentTable)
	}

	return tables
}

// parsePipeSeparatedRow splits a table row by | delimiter.
func parsePipeSeparatedRow(line string) []string {
	parts := strings.Split(line, "|")
	// Remove empty first/last cells
	if len(parts) > 0 && strings.TrimSpace(parts[0]) == "" {
		parts = parts[1:]
	}
	if len(parts) > 0 && strings.TrimSpace(parts[len(parts)-1]) == "" {
		parts = parts[:len(parts)-1]
	}
	return parts
}

// isSeparatorRow checks if a row is a separator (|---|---|).
func isSeparatorRow(line string) bool {
	return strings.Contains(line, "---")
}

// extractBenchmarkRuns extracts benchmark runs from a table.
// Returns error if table doesn't look like benchmark data.
func extractBenchmarkRuns(table []map[string]string) ([]BenchmarkRun, error) {
	if len(table) == 0 {
		return nil, fmt.Errorf("empty table")
	}

	// Check if table has variant column (key indicator)
	firstRow := table[0]
	if _, hasVariant := firstRow["Variant"]; !hasVariant {
		return nil, fmt.Errorf("table missing Variant column")
	}

	var runs []BenchmarkRun

	for _, row := range table {
		run, err := rowToBenchmarkRun(row)
		if err != nil {
			// Skip invalid rows
			continue
		}
		runs = append(runs, run)
	}

	if len(runs) == 0 {
		return nil, fmt.Errorf("no valid benchmark runs extracted")
	}

	return runs, nil
}

// rowToBenchmarkRun converts a table row to a BenchmarkRun.
func rowToBenchmarkRun(row map[string]string) (BenchmarkRun, error) {
	run := BenchmarkRun{
		RunID:      "migrated-" + uuid.New().String(),
		Timestamp:  time.Now(),
		Successful: true, // Assume success for migrated data
		Metadata:   make(map[string]interface{}),
	}

	// Parse required fields
	variant, err := parseVariant(row)
	if err != nil {
		return run, err
	}
	run.Variant = variant
	run.ProjectSize = parseProjectSize(row)

	// Parse optional fields
	parseOptionalQuality(row, &run)
	parseOptionalCost(row, &run)
	parseOptionalTime(row, &run)
	parseOptionalFileCount(row, &run)
	parseOptionalDocs(row, &run)
	parseOptionalTokens(row, &run)

	return run, nil
}

// parseVariant extracts and normalizes variant from row
func parseVariant(row map[string]string) (string, error) {
	variant := strings.ToLower(strings.TrimSpace(row["Variant"]))
	if variant == "raw" || variant == "raw claude" || variant == "raw claude code" {
		return "raw", nil
	}
	if variant == "engram" || variant == "claude+engram" || variant == "claude + engram" {
		return "engram", nil
	}
	if variant == "wayfinder" || variant == "claude+engram+wayfinder" {
		return "wayfinder", nil
	}
	return "", fmt.Errorf("unknown variant: %s", variant)
}

// parseProjectSize extracts and normalizes project size from row
func parseProjectSize(row map[string]string) string {
	// Try multiple column names
	projectSize := getColumnValue(row, "Project Size", "Size")
	projectSize = strings.ToLower(strings.TrimSpace(projectSize))

	if projectSize == "small" || projectSize == "medium" || projectSize == "large" || projectSize == "xl" {
		return projectSize
	}
	return "small" // Default
}

// getColumnValue returns first non-empty value from column name candidates
func getColumnValue(row map[string]string, columnNames ...string) string {
	for _, name := range columnNames {
		if val, ok := row[name]; ok && val != "" {
			return val
		}
	}
	return ""
}

// parseOptionalQuality parses quality score field
func parseOptionalQuality(row map[string]string, run *BenchmarkRun) {
	if qualityStr := getColumnValue(row, "Quality", "Quality Score"); qualityStr != "" {
		if score := parseQualityScore(qualityStr); score != nil {
			run.QualityScore = score
		}
	}
}

// parseOptionalCost parses cost field
func parseOptionalCost(row map[string]string, run *BenchmarkRun) {
	if costStr := getColumnValue(row, "Cost"); costStr != "" {
		if cost := parseCost(costStr); cost != nil {
			run.CostUSD = cost
		}
	}
}

// parseOptionalTime parses time/duration field
func parseOptionalTime(row map[string]string, run *BenchmarkRun) {
	if timeStr := getColumnValue(row, "Time"); timeStr != "" {
		if duration := parseTime(timeStr); duration != nil {
			run.DurationMs = duration
		}
	}
}

// parseOptionalFileCount parses file count field
func parseOptionalFileCount(row map[string]string, run *BenchmarkRun) {
	if filesStr := getColumnValue(row, "Files", "File Count"); filesStr != "" {
		if count := parseInt(filesStr); count != nil {
			run.FileCount = count
		}
	}
}

// parseOptionalDocs parses documentation size field
func parseOptionalDocs(row map[string]string, run *BenchmarkRun) {
	if docsStr := getColumnValue(row, "Docs"); docsStr != "" {
		if kb := parseDocSize(docsStr); kb != nil {
			run.DocumentationKB = kb
		}
	}
}

// parseOptionalTokens parses tokens field (input/output format)
func parseOptionalTokens(row map[string]string, run *BenchmarkRun) {
	if tokensStr := getColumnValue(row, "Tokens"); tokensStr != "" {
		// Format might be "input/output" or just total
		parts := strings.Split(tokensStr, "/")
		if len(parts) == 2 {
			if input := parseInt64(parts[0]); input != nil {
				run.TokensInput = input
			}
			if output := parseInt64(parts[1]); output != nil {
				run.TokensOutput = output
			}
		}
	}
}

// Parsing helper functions

func parseQualityScore(s string) *float64 {
	// Format: "6.9/10" or "6.9"
	s = strings.TrimSpace(s)
	s = strings.Split(s, "/")[0] // Take first part if "/10" present
	if val, err := strconv.ParseFloat(s, 64); err == nil {
		return &val
	}
	return nil
}

func parseCost(s string) *float64 {
	// Format: "$0.30" or "0.30"
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "$")
	if val, err := strconv.ParseFloat(s, 64); err == nil {
		return &val
	}
	return nil
}

func parseTime(s string) *int64 {
	// Format: "15 min" or "4-5h" or "15min"
	s = strings.TrimSpace(strings.ToLower(s))

	// Extract number using regex
	re := regexp.MustCompile(`(\d+(?:\.\d+)?)`)
	matches := re.FindStringSubmatch(s)
	if len(matches) == 0 {
		return nil
	}

	num, err := strconv.ParseFloat(matches[1], 64)
	if err != nil {
		return nil
	}

	// Determine unit
	var ms int64
	if strings.Contains(s, "h") {
		ms = int64(num * 3600000) // hours to ms
	} else if strings.Contains(s, "min") || strings.Contains(s, "m") {
		ms = int64(num * 60000) // minutes to ms
	} else if strings.Contains(s, "s") {
		ms = int64(num * 1000) // seconds to ms
	} else {
		ms = int64(num * 60000) // default to minutes
	}

	return &ms
}

func parseInt(s string) *int {
	s = strings.TrimSpace(s)
	if val, err := strconv.Atoi(s); err == nil {
		return &val
	}
	return nil
}

func parseInt64(s string) *int64 {
	s = strings.TrimSpace(s)
	if val, err := strconv.ParseInt(s, 10, 64); err == nil {
		return &val
	}
	return nil
}

func parseDocSize(s string) *float64 {
	// Format: "176KB" or "176 KB" or "176"
	s = strings.TrimSpace(strings.ToLower(s))
	s = strings.TrimSuffix(s, "kb")
	s = strings.TrimSpace(s)
	if val, err := strconv.ParseFloat(s, 64); err == nil {
		return &val
	}
	return nil
}

// validateRun validates a benchmark run.
func validateRun(run BenchmarkRun) error {
	if run.RunID == "" {
		return fmt.Errorf("run_id is empty")
	}
	if run.Variant == "" {
		return fmt.Errorf("variant is empty")
	}
	if !isValidVariant(run.Variant) {
		return fmt.Errorf("invalid variant: %s", run.Variant)
	}
	if run.ProjectSize == "" {
		return fmt.Errorf("project_size is empty")
	}
	if !isValidProjectSize(run.ProjectSize) {
		return fmt.Errorf("invalid project_size: %s", run.ProjectSize)
	}
	if run.QualityScore != nil && (*run.QualityScore < 0 || *run.QualityScore > 10) {
		return fmt.Errorf("quality_score out of range: %.2f", *run.QualityScore)
	}
	if run.CostUSD != nil && *run.CostUSD < 0 {
		return fmt.Errorf("cost_usd cannot be negative: %.2f", *run.CostUSD)
	}
	return nil
}
