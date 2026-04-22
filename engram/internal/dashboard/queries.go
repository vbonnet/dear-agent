package dashboard

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// SpecificityMetric represents success rate by specificity level.
type SpecificityMetric struct {
	Level       string
	Total       int
	Successes   int
	SuccessRate float64
}

// ExampleMetric represents success rate by example presence.
type ExampleMetric struct {
	Status      string
	Total       int
	Successes   int
	SuccessRate float64
}

// EfficiencyMetric represents token efficiency by prompt type.
type EfficiencyMetric struct {
	PromptType  string
	AvgTokens   float64
	AvgRetries  float64
	SuccessRate float64
}

// TrendMetric represents daily success rate trend.
type TrendMetric struct {
	Date          string
	TotalLaunches int
	Successes     int
	SuccessRate   float64
}

// QuerySuccessBySpecificity returns success rates grouped by specificity level.
func QuerySuccessBySpecificity(ctx context.Context, db *sql.DB, since, until time.Time) ([]SpecificityMetric, error) {
	query := `
		SELECT
		  CASE
		    WHEN specificity_score > 0.7 THEN 'High (>0.7)'
		    WHEN specificity_score >= 0.4 THEN 'Medium (0.4-0.7)'
		    ELSE 'Low (<0.4)'
		  END as specificity_level,
		  COUNT(*) as total,
		  SUM(CASE WHEN outcome = 'success' THEN 1 ELSE 0 END) as successes,
		  CAST(SUM(CASE WHEN outcome = 'success' THEN 1 ELSE 0 END) AS REAL) / COUNT(*) * 100 as success_rate
		FROM agent_launches
		WHERE 1=1
	`

	args := []interface{}{}

	if !since.IsZero() {
		query += " AND created_at >= ?"
		args = append(args, since.Format(time.RFC3339))
	}

	if !until.IsZero() {
		query += " AND created_at <= ?"
		args = append(args, until.Format(time.RFC3339))
	}

	query += " GROUP BY specificity_level ORDER BY specificity_level DESC"

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query success by specificity: %w", err)
	}
	defer rows.Close()

	var results []SpecificityMetric
	for rows.Next() {
		var m SpecificityMetric
		if err := rows.Scan(&m.Level, &m.Total, &m.Successes, &m.SuccessRate); err != nil {
			return nil, fmt.Errorf("failed to scan specificity metric: %w", err)
		}
		results = append(results, m)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating specificity results: %w", err)
	}

	return results, nil
}

// QuerySuccessByExamples returns success rates grouped by example presence.
func QuerySuccessByExamples(ctx context.Context, db *sql.DB, since, until time.Time) ([]ExampleMetric, error) {
	query := `
		SELECT
		  CASE WHEN has_examples = 1 THEN 'With Examples' ELSE 'Without Examples' END as example_status,
		  COUNT(*) as total,
		  SUM(CASE WHEN outcome = 'success' THEN 1 ELSE 0 END) as successes,
		  CAST(SUM(CASE WHEN outcome = 'success' THEN 1 ELSE 0 END) AS REAL) / COUNT(*) * 100 as success_rate
		FROM agent_launches
		WHERE 1=1
	`

	args := []interface{}{}

	if !since.IsZero() {
		query += " AND created_at >= ?"
		args = append(args, since.Format(time.RFC3339))
	}

	if !until.IsZero() {
		query += " AND created_at <= ?"
		args = append(args, until.Format(time.RFC3339))
	}

	query += " GROUP BY has_examples ORDER BY has_examples DESC"

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query success by examples: %w", err)
	}
	defer rows.Close()

	var results []ExampleMetric
	for rows.Next() {
		var m ExampleMetric
		if err := rows.Scan(&m.Status, &m.Total, &m.Successes, &m.SuccessRate); err != nil {
			return nil, fmt.Errorf("failed to scan example metric: %w", err)
		}
		results = append(results, m)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating example results: %w", err)
	}

	return results, nil
}

// QueryTokenEfficiency returns token efficiency metrics by prompt type.
func QueryTokenEfficiency(ctx context.Context, db *sql.DB, since, until time.Time) ([]EfficiencyMetric, error) {
	query := `
		SELECT
		  CASE
		    WHEN specificity_score > 0.7 THEN 'Specific (>0.7)'
		    ELSE 'Vague (<=0.7)'
		  END as prompt_type,
		  AVG(token_count) as avg_tokens,
		  AVG(retry_count) as avg_retries,
		  CAST(SUM(CASE WHEN outcome = 'success' THEN 1 ELSE 0 END) AS REAL) / COUNT(*) * 100 as success_rate
		FROM agent_launches
		WHERE 1=1
	`

	args := []interface{}{}

	if !since.IsZero() {
		query += " AND created_at >= ?"
		args = append(args, since.Format(time.RFC3339))
	}

	if !until.IsZero() {
		query += " AND created_at <= ?"
		args = append(args, until.Format(time.RFC3339))
	}

	query += " GROUP BY prompt_type ORDER BY prompt_type DESC"

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query token efficiency: %w", err)
	}
	defer rows.Close()

	var results []EfficiencyMetric
	for rows.Next() {
		var m EfficiencyMetric
		if err := rows.Scan(&m.PromptType, &m.AvgTokens, &m.AvgRetries, &m.SuccessRate); err != nil {
			return nil, fmt.Errorf("failed to scan efficiency metric: %w", err)
		}
		results = append(results, m)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating efficiency results: %w", err)
	}

	return results, nil
}

// QueryTrendsOverTime returns daily success rate trends (last 30 days by default).
func QueryTrendsOverTime(ctx context.Context, db *sql.DB, since, until time.Time) ([]TrendMetric, error) {
	query := `
		SELECT
		  DATE(created_at) as day,
		  COUNT(*) as total_launches,
		  SUM(CASE WHEN outcome = 'success' THEN 1 ELSE 0 END) as successes,
		  CAST(SUM(CASE WHEN outcome = 'success' THEN 1 ELSE 0 END) AS REAL) / COUNT(*) * 100 as success_rate
		FROM agent_launches
		WHERE 1=1
	`

	args := []interface{}{}

	if !since.IsZero() {
		query += " AND created_at >= ?"
		args = append(args, since.Format(time.RFC3339))
	}

	if !until.IsZero() {
		query += " AND created_at <= ?"
		args = append(args, until.Format(time.RFC3339))
	}

	query += " GROUP BY DATE(created_at) ORDER BY day DESC LIMIT 30"

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query trends over time: %w", err)
	}
	defer rows.Close()

	var results []TrendMetric
	for rows.Next() {
		var m TrendMetric
		if err := rows.Scan(&m.Date, &m.TotalLaunches, &m.Successes, &m.SuccessRate); err != nil {
			return nil, fmt.Errorf("failed to scan trend metric: %w", err)
		}
		results = append(results, m)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating trend results: %w", err)
	}

	return results, nil
}
