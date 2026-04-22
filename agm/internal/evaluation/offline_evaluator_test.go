package evaluation

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// customDetailedJudge allows custom evaluation logic for testing
type customDetailedJudge struct {
	evaluateFunc func(ctx context.Context, input, expectedOutput string, criteria EvaluationCriteria) (*JudgeResponse, error)
}

func (c *customDetailedJudge) EvaluateDetailed(ctx context.Context, input, expectedOutput string, criteria EvaluationCriteria) (*JudgeResponse, error) {
	if c.evaluateFunc != nil {
		return c.evaluateFunc(ctx, input, expectedOutput, criteria)
	}
	return &JudgeResponse{Pass: true, Score: 1.0, Reasoning: "Default"}, nil
}

func TestEvaluateOffline(t *testing.T) {
	t.Run("all test cases pass", func(t *testing.T) {
		testCases := []TestCase{
			{
				ID:             "test-1",
				Input:          "input1",
				ExpectedOutput: "output1",
				ActualOutput:   "output1",
				Criteria: EvaluationCriteria{
					Name:      "correctness",
					Threshold: 0.7,
				},
			},
			{
				ID:             "test-2",
				Input:          "input2",
				ExpectedOutput: "output2",
				ActualOutput:   "output2",
				Criteria: EvaluationCriteria{
					Name:      "correctness",
					Threshold: 0.7,
				},
			},
		}

		// Mock judge that always passes
		mockJudge := &MockDetailedJudge{
			Response: &JudgeResponse{
				Pass:      true,
				Score:     0.9,
				Reasoning: "Test passed",
			},
		}

		config := DefaultOfflineConfig()
		report, err := EvaluateOffline(context.Background(), testCases, mockJudge, config)

		require.NoError(t, err)
		assert.Equal(t, 2, report.TotalCases)
		assert.Equal(t, 2, report.PassedCases)
		assert.Equal(t, 0, report.FailedCases)
		assert.Equal(t, 1.0, report.PassRate)
		assert.Equal(t, 0.9, report.AverageScore)
		assert.False(t, report.BlockDeployment)
		assert.Empty(t, report.FailedTestIDs)
		assert.Empty(t, report.Failures)
	})

	t.Run("some test cases fail below threshold", func(t *testing.T) {
		testCases := []TestCase{
			{
				ID:             "test-1",
				Input:          "input1",
				ExpectedOutput: "output1",
				ActualOutput:   "output1",
				Criteria: EvaluationCriteria{
					Name:      "correctness",
					Threshold: 0.7,
				},
			},
			{
				ID:             "test-2",
				Input:          "input2",
				ExpectedOutput: "output2",
				ActualOutput:   "bad-output",
				Criteria: EvaluationCriteria{
					Name:      "correctness",
					Threshold: 0.7,
				},
			},
		}

		// Create custom mock with state
		callCount := 0
		customMock := &customDetailedJudge{
			evaluateFunc: func(ctx context.Context, input, expectedOutput string, criteria EvaluationCriteria) (*JudgeResponse, error) {
				callCount++
				if callCount == 1 {
					return &JudgeResponse{Pass: true, Score: 0.9, Reasoning: "Good"}, nil
				}
				return &JudgeResponse{Pass: false, Score: 0.4, Reasoning: "Poor quality"}, nil
			},
		}

		config := DefaultOfflineConfig()
		config.MinPassRate = 0.9 // Require 90% pass rate

		report, err := EvaluateOffline(context.Background(), testCases, customMock, config)

		require.NoError(t, err)
		assert.Equal(t, 2, report.TotalCases)
		assert.Equal(t, 1, report.PassedCases)
		assert.Equal(t, 1, report.FailedCases)
		assert.Equal(t, 0.5, report.PassRate)
		assert.Equal(t, 0.65, report.AverageScore) // (0.9 + 0.4) / 2
		assert.True(t, report.BlockDeployment)     // Pass rate below threshold
		assert.Contains(t, report.FailedTestIDs, "test-2")
		assert.Len(t, report.Failures, 1)
		assert.Equal(t, "test-2", report.Failures[0].TestID)
		assert.Equal(t, 0.4, report.Failures[0].Score)
	})

	t.Run("deployment blocked due to low average score", func(t *testing.T) {
		testCases := []TestCase{
			{
				ID:             "test-1",
				Input:          "input1",
				ExpectedOutput: "output1",
				ActualOutput:   "output1",
				Criteria: EvaluationCriteria{
					Name:      "correctness",
					Threshold: 0.5, // Low threshold
				},
			},
		}

		// Mock judge with passing but low score
		mockJudge := &MockDetailedJudge{
			Response: &JudgeResponse{
				Pass:      true, // Passes threshold
				Score:     0.6,  // But score is low
				Reasoning: "Marginally acceptable",
			},
		}

		config := DefaultOfflineConfig()
		config.MinAverageScore = 0.8 // Require 0.8 average

		report, err := EvaluateOffline(context.Background(), testCases, mockJudge, config)

		require.NoError(t, err)
		assert.Equal(t, 1, report.PassedCases)
		assert.Equal(t, 0, report.FailedCases)
		assert.True(t, report.BlockDeployment) // Blocked due to low average score
	})

	t.Run("deployment allowed when BlockOnFailure is false", func(t *testing.T) {
		testCases := []TestCase{
			{
				ID:             "test-1",
				Input:          "input1",
				ExpectedOutput: "output1",
				ActualOutput:   "bad-output",
				Criteria: EvaluationCriteria{
					Name:      "correctness",
					Threshold: 0.9,
				},
			},
		}

		mockJudge := &MockDetailedJudge{
			Response: &JudgeResponse{
				Pass:      false,
				Score:     0.3,
				Reasoning: "Failed",
			},
		}

		config := DefaultOfflineConfig()
		config.BlockOnFailure = false

		report, err := EvaluateOffline(context.Background(), testCases, mockJudge, config)

		require.NoError(t, err)
		assert.Equal(t, 0, report.PassedCases)
		assert.Equal(t, 1, report.FailedCases)
		assert.False(t, report.BlockDeployment) // Not blocked
	})

	t.Run("max failed cases threshold", func(t *testing.T) {
		testCases := []TestCase{
			{ID: "test-1", Criteria: EvaluationCriteria{Threshold: 0.7}},
			{ID: "test-2", Criteria: EvaluationCriteria{Threshold: 0.7}},
			{ID: "test-3", Criteria: EvaluationCriteria{Threshold: 0.7}},
		}

		callCount := 0
		customMock := &customDetailedJudge{
			evaluateFunc: func(ctx context.Context, input, expectedOutput string, criteria EvaluationCriteria) (*JudgeResponse, error) {
				callCount++
				if callCount <= 2 {
					return &JudgeResponse{Pass: false, Score: 0.5, Reasoning: "Failed"}, nil
				}
				return &JudgeResponse{Pass: true, Score: 0.9, Reasoning: "Passed"}, nil
			},
		}

		config := DefaultOfflineConfig()
		config.MaxFailedCases = 1 // Allow only 1 failure

		report, err := EvaluateOffline(context.Background(), testCases, customMock, config)

		require.NoError(t, err)
		assert.Equal(t, 2, report.FailedCases)
		assert.True(t, report.BlockDeployment) // Blocked: 2 failures > 1 allowed
	})

	t.Run("nil judge error", func(t *testing.T) {
		testCases := []TestCase{{ID: "test-1"}}
		config := DefaultOfflineConfig()

		report, err := EvaluateOffline(context.Background(), testCases, nil, config)

		assert.Error(t, err)
		assert.Nil(t, report)
		assert.Contains(t, err.Error(), "judge cannot be nil")
	})

	t.Run("no test cases error", func(t *testing.T) {
		mockJudge := &MockDetailedJudge{}
		config := DefaultOfflineConfig()

		report, err := EvaluateOffline(context.Background(), []TestCase{}, mockJudge, config)

		assert.Error(t, err)
		assert.Nil(t, report)
		assert.Contains(t, err.Error(), "no test cases provided")
	})

	t.Run("judge evaluation error", func(t *testing.T) {
		testCases := []TestCase{
			{ID: "test-1", Criteria: EvaluationCriteria{Threshold: 0.7}},
		}

		mockJudge := &MockDetailedJudge{
			Err: assert.AnError,
		}

		config := DefaultOfflineConfig()

		report, err := EvaluateOffline(context.Background(), testCases, mockJudge, config)

		assert.Error(t, err)
		assert.Nil(t, report)
		assert.Contains(t, err.Error(), "failed to evaluate test case")
	})
}

func TestFormatReport(t *testing.T) {
	t.Run("format passing report", func(t *testing.T) {
		report := &EvaluationReport{
			TotalCases:      5,
			PassedCases:     5,
			FailedCases:     0,
			AverageScore:    0.92,
			PassRate:        1.0,
			BlockDeployment: false,
		}

		formatted := FormatReport(report)

		assert.Contains(t, formatted, "Total test cases: 5")
		assert.Contains(t, formatted, "Passed: 5")
		assert.Contains(t, formatted, "Failed: 0")
		assert.Contains(t, formatted, "Pass rate: 100.00%")
		assert.Contains(t, formatted, "Average score: 0.92")
		assert.Contains(t, formatted, "DEPLOYMENT ALLOWED")
	})

	t.Run("format failing report with details", func(t *testing.T) {
		report := &EvaluationReport{
			TotalCases:      3,
			PassedCases:     1,
			FailedCases:     2,
			AverageScore:    0.65,
			PassRate:        0.33,
			BlockDeployment: true,
			FailedTestIDs:   []string{"test-1", "test-2"},
			Failures: []FailureDetail{
				{
					TestID:    "test-1",
					Input:     "input1",
					Expected:  "expected1",
					Actual:    "actual1",
					Score:     0.4,
					Threshold: 0.7,
					Reasoning: "Quality below threshold",
				},
			},
		}

		formatted := FormatReport(report)

		assert.Contains(t, formatted, "Total test cases: 3")
		assert.Contains(t, formatted, "Failed: 2")
		assert.Contains(t, formatted, "Pass rate: 33.00%")
		assert.Contains(t, formatted, "DEPLOYMENT BLOCKED")
		assert.Contains(t, formatted, "Failed Test Cases (1)")
		assert.Contains(t, formatted, "Test ID: test-1")
		assert.Contains(t, formatted, "Score: 0.40 (threshold: 0.70)")
	})
}

func TestBatchEvaluateWithMetrics(t *testing.T) {
	t.Run("evaluate multiple test cases with metrics", func(t *testing.T) {
		testCases := []TestCase{
			{
				ID:             "test-1",
				Input:          "input1",
				ExpectedOutput: "expected",
				ActualOutput:   "expected",
			},
			{
				ID:             "test-2",
				Input:          "input2",
				ExpectedOutput: "expected",
				ActualOutput:   "different",
			},
		}

		metrics := []Metric{
			&CorrectnessMetric{ExactMatch: true},
			&SafetyMetric{HarmfulKeywords: []string{"bad"}},
		}

		thresholds := map[string]float64{
			"correctness": 0.8,
			"safety":      0.9,
		}

		results := BatchEvaluateWithMetrics(testCases, metrics, thresholds)

		assert.Len(t, results, 2)
		assert.Contains(t, results, "test-1")
		assert.Contains(t, results, "test-2")

		// test-1 should have 2 metric results
		assert.Len(t, results["test-1"], 2)
		assert.Equal(t, "correctness", results["test-1"][0].MetricName)
		assert.Equal(t, "safety", results["test-1"][1].MetricName)
	})
}

func TestAggregateMetricResults(t *testing.T) {
	t.Run("aggregate results across test cases", func(t *testing.T) {
		results := map[string][]MetricResult{
			"test-1": {
				{MetricName: "correctness", Score: 0.9},
				{MetricName: "safety", Score: 1.0},
			},
			"test-2": {
				{MetricName: "correctness", Score: 0.7},
				{MetricName: "safety", Score: 0.8},
			},
		}

		averages := AggregateMetricResults(results)

		assert.Len(t, averages, 2)
		assert.Equal(t, 0.8, averages["correctness"]) // (0.9 + 0.7) / 2
		assert.Equal(t, 0.9, averages["safety"])      // (1.0 + 0.8) / 2
	})

	t.Run("empty results", func(t *testing.T) {
		results := map[string][]MetricResult{}

		averages := AggregateMetricResults(results)

		assert.Empty(t, averages)
	})
}

func TestTruncateString(t *testing.T) {
	t.Run("short string not truncated", func(t *testing.T) {
		result := truncateString("short", 100)
		assert.Equal(t, "short", result)
	})

	t.Run("long string truncated", func(t *testing.T) {
		long := strings.Repeat("a", 200)
		result := truncateString(long, 100)
		assert.Len(t, result, 100)
		assert.True(t, strings.HasSuffix(result, "..."))
	})
}

func TestDefaultOfflineConfig(t *testing.T) {
	t.Run("creates valid default config", func(t *testing.T) {
		config := DefaultOfflineConfig()
		assert.Equal(t, 0.95, config.MinPassRate)
		assert.Equal(t, 0.80, config.MinAverageScore)
		assert.True(t, config.BlockOnFailure)
		assert.Equal(t, 0, config.MaxFailedCases)
	})
}

func TestShouldBlockDeployment(t *testing.T) {
	t.Run("block when pass rate below threshold", func(t *testing.T) {
		report := &EvaluationReport{
			PassRate:     0.85,
			AverageScore: 0.9,
		}
		config := OfflineEvaluatorConfig{
			MinPassRate:     0.90,
			MinAverageScore: 0.80,
			BlockOnFailure:  true,
		}

		blocked := shouldBlockDeployment(report, config)
		assert.True(t, blocked)
	})

	t.Run("block when average score below threshold", func(t *testing.T) {
		report := &EvaluationReport{
			PassRate:     0.95,
			AverageScore: 0.75,
		}
		config := OfflineEvaluatorConfig{
			MinPassRate:     0.90,
			MinAverageScore: 0.80,
			BlockOnFailure:  true,
		}

		blocked := shouldBlockDeployment(report, config)
		assert.True(t, blocked)
	})

	t.Run("allow when all thresholds met", func(t *testing.T) {
		report := &EvaluationReport{
			PassRate:     0.95,
			AverageScore: 0.85,
		}
		config := OfflineEvaluatorConfig{
			MinPassRate:     0.90,
			MinAverageScore: 0.80,
			BlockOnFailure:  true,
		}

		blocked := shouldBlockDeployment(report, config)
		assert.False(t, blocked)
	})

	t.Run("allow when BlockOnFailure is false", func(t *testing.T) {
		report := &EvaluationReport{
			PassRate:     0.50,
			AverageScore: 0.30,
		}
		config := OfflineEvaluatorConfig{
			MinPassRate:     0.90,
			MinAverageScore: 0.80,
			BlockOnFailure:  false,
		}

		blocked := shouldBlockDeployment(report, config)
		assert.False(t, blocked)
	})
}
