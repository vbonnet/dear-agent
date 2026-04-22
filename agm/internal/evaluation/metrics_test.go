package evaluation

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCorrectnessMetric(t *testing.T) {
	t.Run("exact match mode - matching strings", func(t *testing.T) {
		metric := &CorrectnessMetric{
			CaseSensitive: true,
			ExactMatch:    true,
		}

		score := metric.Evaluate("input", "expected output", "expected output")
		assert.Equal(t, 1.0, score)
	})

	t.Run("exact match mode - non-matching strings", func(t *testing.T) {
		metric := &CorrectnessMetric{
			CaseSensitive: true,
			ExactMatch:    true,
		}

		score := metric.Evaluate("input", "expected output", "different output")
		assert.Equal(t, 0.0, score)
	})

	t.Run("case insensitive exact match", func(t *testing.T) {
		metric := &CorrectnessMetric{
			CaseSensitive: false,
			ExactMatch:    true,
		}

		score := metric.Evaluate("input", "Expected Output", "expected output")
		assert.Equal(t, 1.0, score)
	})

	t.Run("similarity mode - identical strings", func(t *testing.T) {
		metric := &CorrectnessMetric{
			CaseSensitive: true,
			ExactMatch:    false,
		}

		score := metric.Evaluate("input", "test output", "test output")
		assert.Equal(t, 1.0, score)
	})

	t.Run("similarity mode - similar strings", func(t *testing.T) {
		metric := &CorrectnessMetric{
			CaseSensitive: false,
			ExactMatch:    false,
		}

		score := metric.Evaluate("input", "hello world", "hello there")
		assert.Greater(t, score, 0.0)
		assert.Less(t, score, 1.0)
	})

	t.Run("similarity mode - completely different strings", func(t *testing.T) {
		metric := &CorrectnessMetric{
			CaseSensitive: true,
			ExactMatch:    false,
		}

		score := metric.Evaluate("input", "abc", "xyz")
		// No character overlap, so score should be 0.0
		assert.Equal(t, 0.0, score)
	})

	t.Run("empty strings handling", func(t *testing.T) {
		metric := &CorrectnessMetric{
			CaseSensitive: true,
			ExactMatch:    false,
		}

		score := metric.Evaluate("input", "", "")
		assert.Equal(t, 1.0, score)

		score = metric.Evaluate("input", "something", "")
		assert.Equal(t, 0.0, score)

		score = metric.Evaluate("input", "", "something")
		assert.Equal(t, 0.0, score)
	})

	t.Run("metric name", func(t *testing.T) {
		metric := &CorrectnessMetric{}
		assert.Equal(t, "correctness", metric.Name())
	})
}

func TestSafetyMetric(t *testing.T) {
	t.Run("safe output - no violations", func(t *testing.T) {
		metric := &SafetyMetric{
			HarmfulKeywords: []string{"violent", "harmful"},
		}

		score := metric.Evaluate("input", "expected", "This is a safe output")
		assert.Equal(t, 1.0, score)
	})

	t.Run("unsafe output - keyword detected", func(t *testing.T) {
		metric := &SafetyMetric{
			HarmfulKeywords: []string{"violent", "harmful"},
		}

		score := metric.Evaluate("input", "expected", "This output contains violent content")
		assert.Less(t, score, 1.0)
	})

	t.Run("multiple violations", func(t *testing.T) {
		metric := &SafetyMetric{
			HarmfulKeywords: []string{"violent", "harmful", "dangerous"},
		}

		score := metric.Evaluate("input", "expected", "violent and harmful content")
		assert.Less(t, score, 0.5) // 2 out of 3 checks failed
	})

	t.Run("case insensitive keyword matching", func(t *testing.T) {
		metric := &SafetyMetric{
			HarmfulKeywords: []string{"VIOLENT"},
		}

		score := metric.Evaluate("input", "expected", "violent content")
		assert.Less(t, score, 1.0)
	})

	t.Run("blocklist patterns", func(t *testing.T) {
		metric := &SafetyMetric{
			Blocklist: []string{"forbidden", "blocked"},
		}

		score := metric.Evaluate("input", "expected", "This contains forbidden text")
		assert.Less(t, score, 1.0)
	})

	t.Run("combined keywords and blocklist", func(t *testing.T) {
		metric := &SafetyMetric{
			HarmfulKeywords: []string{"violent"},
			Blocklist:       []string{"forbidden"},
		}

		// No violations
		score := metric.Evaluate("input", "expected", "safe content")
		assert.Equal(t, 1.0, score)

		// One violation
		score = metric.Evaluate("input", "expected", "violent content")
		assert.Equal(t, 0.5, score) // 1 out of 2 checks failed
	})

	t.Run("empty output is safe", func(t *testing.T) {
		metric := &SafetyMetric{
			HarmfulKeywords: []string{"violent"},
		}

		score := metric.Evaluate("input", "expected", "")
		assert.Equal(t, 1.0, score)
	})

	t.Run("no checks configured returns safe", func(t *testing.T) {
		metric := &SafetyMetric{}

		score := metric.Evaluate("input", "expected", "any content")
		assert.Equal(t, 1.0, score)
	})

	t.Run("metric name", func(t *testing.T) {
		metric := &SafetyMetric{}
		assert.Equal(t, "safety", metric.Name())
	})

	t.Run("default safety keywords", func(t *testing.T) {
		keywords := DefaultSafetyKeywords()
		assert.NotEmpty(t, keywords)
		assert.Contains(t, keywords, "violent")
		assert.Contains(t, keywords, "harmful")
	})
}

func TestPerformanceMetric(t *testing.T) {
	t.Run("latency within threshold", func(t *testing.T) {
		metric := &PerformanceMetric{
			MaxLatencyMs:  1000,
			ActualLatency: 500,
		}

		score := metric.Evaluate("input", "expected", "actual")
		assert.Equal(t, 1.0, score)
	})

	t.Run("latency exceeds threshold", func(t *testing.T) {
		metric := &PerformanceMetric{
			MaxLatencyMs:  1000,
			ActualLatency: 2000,
		}

		score := metric.Evaluate("input", "expected", "actual")
		assert.Equal(t, 0.5, score) // 1000/2000 = 0.5
	})

	t.Run("throughput meets threshold", func(t *testing.T) {
		metric := &PerformanceMetric{
			MinThroughput:  10,
			ActualRequests: 20,
			TimeWindowSec:  1,
		}

		score := metric.Evaluate("input", "expected", "actual")
		assert.Equal(t, 1.0, score)
	})

	t.Run("throughput below threshold", func(t *testing.T) {
		metric := &PerformanceMetric{
			MinThroughput:  10,
			ActualRequests: 5,
			TimeWindowSec:  1,
		}

		score := metric.Evaluate("input", "expected", "actual")
		assert.Equal(t, 0.5, score) // 5/10 = 0.5
	})

	t.Run("combined latency and throughput", func(t *testing.T) {
		metric := &PerformanceMetric{
			MaxLatencyMs:   1000,
			ActualLatency:  1000, // Perfect (score = 1.0)
			MinThroughput:  10,
			ActualRequests: 20, // Double threshold (score = 1.0)
			TimeWindowSec:  1,
		}

		score := metric.Evaluate("input", "expected", "actual")
		assert.Equal(t, 1.0, score) // Average of 1.0 and 1.0
	})

	t.Run("no metrics configured returns 1.0", func(t *testing.T) {
		metric := &PerformanceMetric{}

		score := metric.Evaluate("input", "expected", "actual")
		assert.Equal(t, 1.0, score)
	})

	t.Run("metric name", func(t *testing.T) {
		metric := &PerformanceMetric{}
		assert.Equal(t, "performance", metric.Name())
	})
}

func TestEvaluateLatency(t *testing.T) {
	t.Run("under threshold", func(t *testing.T) {
		score := evaluateLatency(500, 1000)
		assert.Equal(t, 1.0, score)
	})

	t.Run("at threshold", func(t *testing.T) {
		score := evaluateLatency(1000, 1000)
		assert.Equal(t, 1.0, score)
	})

	t.Run("over threshold", func(t *testing.T) {
		score := evaluateLatency(2000, 1000)
		assert.Equal(t, 0.5, score)
	})

	t.Run("zero values", func(t *testing.T) {
		score := evaluateLatency(0, 1000)
		assert.Equal(t, 1.0, score)

		score = evaluateLatency(1000, 0)
		assert.Equal(t, 1.0, score)
	})
}

func TestEvaluateThroughput(t *testing.T) {
	t.Run("above threshold", func(t *testing.T) {
		score := evaluateThroughput(20.0, 10.0)
		assert.Equal(t, 1.0, score)
	})

	t.Run("at threshold", func(t *testing.T) {
		score := evaluateThroughput(10.0, 10.0)
		assert.Equal(t, 1.0, score)
	})

	t.Run("below threshold", func(t *testing.T) {
		score := evaluateThroughput(5.0, 10.0)
		assert.Equal(t, 0.5, score)
	})

	t.Run("zero values", func(t *testing.T) {
		score := evaluateThroughput(0, 10.0)
		assert.Equal(t, 1.0, score)

		score = evaluateThroughput(10.0, 0)
		assert.Equal(t, 1.0, score)
	})
}

func TestEvaluateMetrics(t *testing.T) {
	t.Run("multiple metrics evaluation", func(t *testing.T) {
		metrics := []Metric{
			&CorrectnessMetric{ExactMatch: true, CaseSensitive: false},
			&SafetyMetric{HarmfulKeywords: []string{"bad"}},
			&PerformanceMetric{MaxLatencyMs: 1000, ActualLatency: 500},
		}

		thresholds := map[string]float64{
			"correctness": 0.8,
			"safety":      0.9,
			"performance": 0.7,
		}

		results := EvaluateMetrics("input", "output", "output", metrics, thresholds)

		assert.Len(t, results, 3)
		assert.Equal(t, "correctness", results[0].MetricName)
		assert.Equal(t, "safety", results[1].MetricName)
		assert.Equal(t, "performance", results[2].MetricName)

		// Correctness should pass (exact match)
		assert.True(t, results[0].Pass)
		assert.Equal(t, 1.0, results[0].Score)

		// Safety should pass (no harmful content)
		assert.True(t, results[1].Pass)
		assert.Equal(t, 1.0, results[1].Score)

		// Performance should pass (latency under threshold)
		assert.True(t, results[2].Pass)
		assert.Equal(t, 1.0, results[2].Score)
	})

	t.Run("default threshold when not specified", func(t *testing.T) {
		metrics := []Metric{
			&CorrectnessMetric{ExactMatch: false},
		}

		thresholds := map[string]float64{}

		results := EvaluateMetrics("input", "test", "test", metrics, thresholds)

		assert.Len(t, results, 1)
		// Should use default threshold of 0.7
		assert.True(t, results[0].Pass) // Score is 1.0, which passes 0.7 threshold
	})

	t.Run("failing metrics", func(t *testing.T) {
		metrics := []Metric{
			&CorrectnessMetric{ExactMatch: true},
		}

		thresholds := map[string]float64{
			"correctness": 0.9,
		}

		results := EvaluateMetrics("input", "expected", "different", metrics, thresholds)

		assert.Len(t, results, 1)
		assert.False(t, results[0].Pass) // Exact match fails
		assert.Equal(t, 0.0, results[0].Score)
	})

	t.Run("results include timestamp", func(t *testing.T) {
		metrics := []Metric{
			&CorrectnessMetric{},
		}

		results := EvaluateMetrics("input", "expected", "actual", metrics, nil)

		assert.Len(t, results, 1)
		assert.False(t, results[0].Timestamp.IsZero())
	})
}

func TestCalculateSimilarity(t *testing.T) {
	t.Run("identical strings", func(t *testing.T) {
		score := calculateSimilarity("test", "test")
		assert.Equal(t, 1.0, score)
	})

	t.Run("completely different strings", func(t *testing.T) {
		score := calculateSimilarity("abc", "xyz")
		assert.Equal(t, 0.0, score)
	})

	t.Run("partial overlap", func(t *testing.T) {
		score := calculateSimilarity("hello", "help")
		assert.Greater(t, score, 0.0)
		assert.Less(t, score, 1.0)
	})
}
