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

// applyTimeRange appends the optional created_at bounds and returns the query
// with the matching positional arguments.
//
// Every query in this file takes the same two optional bounds and built them
// with the same eight lines, which is most of what made the four functions
// token-identical to a clone detector.
func applyTimeRange(query string, since, until time.Time) (string, []any) {
	var args []any

	if !since.IsZero() {
		query += " AND created_at >= ?"
		args = append(args, since.Format(time.RFC3339))
	}
	if !until.IsZero() {
		query += " AND created_at <= ?"
		args = append(args, until.Format(time.RFC3339))
	}

	return query, args
}

// queryMetrics runs a metrics query and scans every row through scan.
//
// subject names what is being read and appears in each error, so a failure
// still says which metric it came from rather than collapsing four distinct
// messages into one generic string.
func queryMetrics[T any](
	ctx context.Context,
	db *sql.DB,
	query string,
	args []any,
	subject string,
	scan func(*sql.Rows, *T) error,
) ([]T, error) {
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query %s: %w", subject, err)
	}
	defer rows.Close()

	var results []T
	for rows.Next() {
		var m T
		if err := scan(rows, &m); err != nil {
			return nil, fmt.Errorf("failed to scan %s metric: %w", subject, err)
		}
		results = append(results, m)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating %s results: %w", subject, err)
	}

	return results, nil
}

// QuerySuccessBySpecificity returns success rates grouped by specificity level.
func QuerySuccessBySpecificity(ctx context.Context, db *sql.DB, since, until time.Time) ([]SpecificityMetric, error) {
	query, args := applyTimeRange(`
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
	`, since, until)
	query += " GROUP BY specificity_level ORDER BY specificity_level DESC"

	return queryMetrics(ctx, db, query, args, "success by specificity",
		func(rows *sql.Rows, m *SpecificityMetric) error {
			return rows.Scan(&m.Level, &m.Total, &m.Successes, &m.SuccessRate)
		})
}

// QuerySuccessByExamples returns success rates grouped by example presence.
func QuerySuccessByExamples(ctx context.Context, db *sql.DB, since, until time.Time) ([]ExampleMetric, error) {
	query, args := applyTimeRange(`
		SELECT
		  CASE WHEN has_examples = 1 THEN 'With Examples' ELSE 'Without Examples' END as example_status,
		  COUNT(*) as total,
		  SUM(CASE WHEN outcome = 'success' THEN 1 ELSE 0 END) as successes,
		  CAST(SUM(CASE WHEN outcome = 'success' THEN 1 ELSE 0 END) AS REAL) / COUNT(*) * 100 as success_rate
		FROM agent_launches
		WHERE 1=1
	`, since, until)
	query += " GROUP BY has_examples ORDER BY has_examples DESC"

	return queryMetrics(ctx, db, query, args, "success by examples",
		func(rows *sql.Rows, m *ExampleMetric) error {
			return rows.Scan(&m.Status, &m.Total, &m.Successes, &m.SuccessRate)
		})
}

// QueryTokenEfficiency returns token efficiency grouped by prompt type.
func QueryTokenEfficiency(ctx context.Context, db *sql.DB, since, until time.Time) ([]EfficiencyMetric, error) {
	query, args := applyTimeRange(`
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
	`, since, until)
	query += " GROUP BY prompt_type ORDER BY prompt_type DESC"

	return queryMetrics(ctx, db, query, args, "token efficiency",
		func(rows *sql.Rows, m *EfficiencyMetric) error {
			return rows.Scan(&m.PromptType, &m.AvgTokens, &m.AvgRetries, &m.SuccessRate)
		})
}

// QueryTrendsOverTime returns the daily success-rate trend.
func QueryTrendsOverTime(ctx context.Context, db *sql.DB, since, until time.Time) ([]TrendMetric, error) {
	query, args := applyTimeRange(`
		SELECT
		  DATE(created_at) as day,
		  COUNT(*) as total_launches,
		  SUM(CASE WHEN outcome = 'success' THEN 1 ELSE 0 END) as successes,
		  CAST(SUM(CASE WHEN outcome = 'success' THEN 1 ELSE 0 END) AS REAL) / COUNT(*) * 100 as success_rate
		FROM agent_launches
		WHERE 1=1
	`, since, until)
	query += " GROUP BY DATE(created_at) ORDER BY day DESC LIMIT 30"

	return queryMetrics(ctx, db, query, args, "trends over time",
		func(rows *sql.Rows, m *TrendMetric) error {
			return rows.Scan(&m.Date, &m.TotalLaunches, &m.Successes, &m.SuccessRate)
		})
}
