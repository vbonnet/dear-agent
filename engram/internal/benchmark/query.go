package benchmark

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// QueryParams defines filters for querying benchmark runs.
type QueryParams struct {
	Variants       []string   // Filter by variant(s)
	ProjectSizes   []string   // Filter by project size(s)
	ProjectNames   []string   // Filter by project name(s)
	QualityMin     *float64   // Minimum quality score
	QualityMax     *float64   // Maximum quality score
	CostMin        *float64   // Minimum cost
	CostMax        *float64   // Maximum cost
	After          *time.Time // Time range start
	Before         *time.Time // Time range end
	SuccessfulOnly bool       // Only successful runs
	Limit          int        // Result limit (default 100)
	OrderBy        string     // "timestamp", "quality_score", "cost_usd"
	OrderDirection string     // "ASC" or "DESC"
}

// Query retrieves benchmark runs matching the given filters.
func (s *Storage) Query(params QueryParams) ([]BenchmarkRun, error) {
	// Build WHERE clause
	conditions, args := buildWhereClause(params)

	// Build query
	query := "SELECT * FROM benchmark_runs"
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}

	// Order by
	orderBy := "timestamp"
	if params.OrderBy != "" {
		orderBy = params.OrderBy
	}
	orderDir := "DESC"
	if params.OrderDirection != "" {
		orderDir = params.OrderDirection
	}
	query += fmt.Sprintf(" ORDER BY %s %s", orderBy, orderDir)

	// Limit
	limit := 100
	if params.Limit > 0 {
		limit = params.Limit
	}
	query += fmt.Sprintf(" LIMIT %d", limit)

	// Execute query
	rows, err := s.db.Query(query, args...) //nolint:noctx // TODO(context): plumb ctx through this layer
	if err != nil {
		return nil, fmt.Errorf("query failed: %w", err)
	}
	defer rows.Close()

	// Scan results
	var results []BenchmarkRun
	for rows.Next() {
		run, err := scanBenchmarkRun(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}
		results = append(results, run)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("row iteration error: %w", err)
	}

	return results, nil
}

// buildWhereClause builds SQL WHERE clause conditions and arguments from query parameters.
func buildWhereClause(params QueryParams) ([]string, []interface{}) {
	var conditions []string
	var args []interface{}

	if len(params.Variants) > 0 {
		placeholders := make([]string, len(params.Variants))
		for i, v := range params.Variants {
			placeholders[i] = "?"
			args = append(args, v)
		}
		conditions = append(conditions, fmt.Sprintf("variant IN (%s)", strings.Join(placeholders, ",")))
	}

	if len(params.ProjectSizes) > 0 {
		placeholders := make([]string, len(params.ProjectSizes))
		for i, p := range params.ProjectSizes {
			placeholders[i] = "?"
			args = append(args, p)
		}
		conditions = append(conditions, fmt.Sprintf("project_size IN (%s)", strings.Join(placeholders, ",")))
	}

	if len(params.ProjectNames) > 0 {
		placeholders := make([]string, len(params.ProjectNames))
		for i, p := range params.ProjectNames {
			placeholders[i] = "?"
			args = append(args, p)
		}
		conditions = append(conditions, fmt.Sprintf("project_name IN (%s)", strings.Join(placeholders, ",")))
	}

	if params.QualityMin != nil {
		conditions = append(conditions, "quality_score >= ?")
		args = append(args, *params.QualityMin)
	}

	if params.QualityMax != nil {
		conditions = append(conditions, "quality_score <= ?")
		args = append(args, *params.QualityMax)
	}

	if params.CostMin != nil {
		conditions = append(conditions, "cost_usd >= ?")
		args = append(args, *params.CostMin)
	}

	if params.CostMax != nil {
		conditions = append(conditions, "cost_usd <= ?")
		args = append(args, *params.CostMax)
	}

	if params.After != nil {
		conditions = append(conditions, "timestamp >= ?")
		args = append(args, params.After.Format(time.RFC3339))
	}

	if params.Before != nil {
		conditions = append(conditions, "timestamp <= ?")
		args = append(args, params.Before.Format(time.RFC3339))
	}

	if params.SuccessfulOnly {
		conditions = append(conditions, "successful = 1")
	}

	return conditions, args
}

// scanBenchmarkRun scans a database row into a BenchmarkRun struct.
func scanBenchmarkRun(rows *sql.Rows) (BenchmarkRun, error) {
	var run BenchmarkRun
	var timestampStr, createdAtStr string
	var projectName, metadataStr sql.NullString
	var qualityScore, costUSD, documentationKB sql.NullFloat64
	var tokensInput, tokensOutput, durationMs sql.NullInt64
	var fileCount, phasesCompleted sql.NullInt32
	var successfulInt int

	err := rows.Scan(
		&run.ID,
		&run.RunID,
		&timestampStr,
		&run.Variant,
		&run.ProjectSize,
		&projectName,
		&qualityScore,
		&costUSD,
		&tokensInput,
		&tokensOutput,
		&durationMs,
		&fileCount,
		&documentationKB,
		&phasesCompleted,
		&successfulInt,
		&metadataStr,
		&createdAtStr,
	)
	if err != nil {
		return run, err
	}

	// Parse timestamps
	run.Timestamp, _ = time.Parse(time.RFC3339, timestampStr)
	run.CreatedAt, _ = time.Parse(time.RFC3339, createdAtStr)

	// Handle nullable strings
	if projectName.Valid {
		run.ProjectName = projectName.String
	}

	// Handle nullable floats
	if qualityScore.Valid {
		val := qualityScore.Float64
		run.QualityScore = &val
	}
	if costUSD.Valid {
		val := costUSD.Float64
		run.CostUSD = &val
	}
	if documentationKB.Valid {
		val := documentationKB.Float64
		run.DocumentationKB = &val
	}

	// Handle nullable ints
	if tokensInput.Valid {
		val := tokensInput.Int64
		run.TokensInput = &val
	}
	if tokensOutput.Valid {
		val := tokensOutput.Int64
		run.TokensOutput = &val
	}
	if durationMs.Valid {
		val := durationMs.Int64
		run.DurationMs = &val
	}
	if fileCount.Valid {
		val := int(fileCount.Int32)
		run.FileCount = &val
	}
	if phasesCompleted.Valid {
		val := int(phasesCompleted.Int32)
		run.PhasesCompleted = &val
	}

	// Parse successful boolean
	run.Successful = successfulInt == 1

	// Parse metadata JSON
	if metadataStr.Valid && metadataStr.String != "" {
		if err := json.Unmarshal([]byte(metadataStr.String), &run.Metadata); err != nil {
			// Log error but don't fail the query
			run.Metadata = nil
		}
	}

	return run, nil
}
