// Package evaluation provides evaluation functionality.
package evaluation

import (
	"context"
	"fmt"
	"time"
)

// TestCase represents a single test case for offline evaluation
type TestCase struct {
	ID             string
	Input          string
	ExpectedOutput string
	ActualOutput   string
	Criteria       EvaluationCriteria
	Metadata       map[string]string // Optional metadata for tracking
}

// EvaluationReport contains the results of offline evaluation
type EvaluationReport struct {
	TotalCases      int
	PassedCases     int
	FailedCases     int
	AverageScore    float64
	PassRate        float64
	FailedTestIDs   []string
	Failures        []FailureDetail
	EvaluatedAt     time.Time
	BlockDeployment bool
}

// FailureDetail contains information about a failed test case
type FailureDetail struct {
	TestID    string
	Input     string
	Expected  string
	Actual    string
	Score     float64
	Threshold float64
	Reasoning string
}

// OfflineEvaluatorConfig holds configuration for offline evaluation
type OfflineEvaluatorConfig struct {
	MinPassRate     float64 // Minimum pass rate to allow deployment (0.0-1.0)
	MinAverageScore float64 // Minimum average score to allow deployment (0.0-1.0)
	BlockOnFailure  bool    // Whether to block deployment on any failure
	MaxFailedCases  int     // Maximum number of failed cases allowed (0 = no limit)
}

// DefaultOfflineConfig returns default configuration
func DefaultOfflineConfig() OfflineEvaluatorConfig {
	return OfflineEvaluatorConfig{
		MinPassRate:     0.95, // 95% of test cases must pass
		MinAverageScore: 0.80, // Average score must be >= 0.80
		BlockOnFailure:  true, // Block deployment if thresholds not met
		MaxFailedCases:  0,    // No specific limit on failed cases
	}
}

// EvaluateOffline runs offline evaluation on a set of test cases using a judge
func EvaluateOffline(ctx context.Context, testCases []TestCase, judge DetailedJudge, config OfflineEvaluatorConfig) (*EvaluationReport, error) {
	if judge == nil {
		return nil, fmt.Errorf("judge cannot be nil")
	}

	if len(testCases) == 0 {
		return nil, fmt.Errorf("no test cases provided")
	}

	report := &EvaluationReport{
		TotalCases:    len(testCases),
		PassedCases:   0,
		FailedCases:   0,
		FailedTestIDs: []string{},
		Failures:      []FailureDetail{},
		EvaluatedAt:   time.Now(),
	}

	totalScore := 0.0

	// Evaluate each test case
	for _, tc := range testCases {
		result, err := judge.EvaluateDetailed(ctx, tc.Input, tc.ExpectedOutput, tc.Criteria)
		if err != nil {
			return nil, fmt.Errorf("failed to evaluate test case %s: %w", tc.ID, err)
		}

		totalScore += result.Score

		if result.Pass {
			report.PassedCases++
		} else {
			report.FailedCases++
			report.FailedTestIDs = append(report.FailedTestIDs, tc.ID)
			report.Failures = append(report.Failures, FailureDetail{
				TestID:    tc.ID,
				Input:     tc.Input,
				Expected:  tc.ExpectedOutput,
				Actual:    tc.ActualOutput,
				Score:     result.Score,
				Threshold: tc.Criteria.Threshold,
				Reasoning: result.Reasoning,
			})
		}
	}

	// Calculate aggregate metrics
	report.AverageScore = totalScore / float64(len(testCases))
	report.PassRate = float64(report.PassedCases) / float64(report.TotalCases)

	// Determine if deployment should be blocked
	report.BlockDeployment = shouldBlockDeployment(report, config)

	return report, nil
}

// shouldBlockDeployment determines if deployment should be blocked based on results
func shouldBlockDeployment(report *EvaluationReport, config OfflineEvaluatorConfig) bool {
	if !config.BlockOnFailure {
		return false
	}

	// Check pass rate threshold
	if report.PassRate < config.MinPassRate {
		return true
	}

	// Check average score threshold
	if report.AverageScore < config.MinAverageScore {
		return true
	}

	// Check maximum failed cases (if configured)
	if config.MaxFailedCases > 0 && report.FailedCases > config.MaxFailedCases {
		return true
	}

	return false
}

// FormatReport generates a human-readable report
func FormatReport(report *EvaluationReport) string {
	result := "=== Offline Evaluation Report ===\n"
	result += fmt.Sprintf("Evaluated at: %s\n\n", report.EvaluatedAt.Format(time.RFC3339))
	result += fmt.Sprintf("Total test cases: %d\n", report.TotalCases)
	result += fmt.Sprintf("Passed: %d\n", report.PassedCases)
	result += fmt.Sprintf("Failed: %d\n", report.FailedCases)
	result += fmt.Sprintf("Pass rate: %.2f%%\n", report.PassRate*100)
	result += fmt.Sprintf("Average score: %.2f\n\n", report.AverageScore)

	if report.BlockDeployment {
		result += "**DEPLOYMENT BLOCKED**\n"
		result += "Evaluation thresholds not met.\n\n"
	} else {
		result += "**DEPLOYMENT ALLOWED**\n"
		result += "All evaluation thresholds passed.\n\n"
	}

	if len(report.Failures) > 0 {
		result += fmt.Sprintf("=== Failed Test Cases (%d) ===\n", len(report.Failures))
		for i, failure := range report.Failures {
			result += fmt.Sprintf("\n%d. Test ID: %s\n", i+1, failure.TestID)
			result += fmt.Sprintf("   Score: %.2f (threshold: %.2f)\n", failure.Score, failure.Threshold)
			result += fmt.Sprintf("   Input: %s\n", truncateString(failure.Input, 100))
			result += fmt.Sprintf("   Expected: %s\n", truncateString(failure.Expected, 100))
			result += fmt.Sprintf("   Actual: %s\n", truncateString(failure.Actual, 100))
			result += fmt.Sprintf("   Reasoning: %s\n", truncateString(failure.Reasoning, 200))
		}
	}

	return result
}

// truncateString truncates a string to a maximum length with ellipsis
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// BatchEvaluateWithMetrics evaluates test cases using multiple metrics
func BatchEvaluateWithMetrics(testCases []TestCase, metrics []Metric, thresholds map[string]float64) map[string][]MetricResult {
	results := make(map[string][]MetricResult)

	for _, tc := range testCases {
		metricResults := EvaluateMetrics(tc.Input, tc.ExpectedOutput, tc.ActualOutput, metrics, thresholds)
		results[tc.ID] = metricResults
	}

	return results
}

// AggregateMetricResults aggregates metric results across all test cases
func AggregateMetricResults(results map[string][]MetricResult) map[string]float64 {
	metricSums := make(map[string]float64)
	metricCounts := make(map[string]int)

	for _, metricResults := range results {
		for _, result := range metricResults {
			metricSums[result.MetricName] += result.Score
			metricCounts[result.MetricName]++
		}
	}

	averages := make(map[string]float64)
	for metricName, sum := range metricSums {
		count := metricCounts[metricName]
		if count > 0 {
			averages[metricName] = sum / float64(count)
		}
	}

	return averages
}
