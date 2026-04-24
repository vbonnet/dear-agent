package evaluation

import (
	"strings"
	"time"
)

// Metric represents a single evaluation metric
type Metric interface {
	// Evaluate returns a score between 0.0 and 1.0
	Evaluate(input, expected, actual string) float64
	// Name returns the metric name
	Name() string
}

// CorrectnessMetric checks if output matches expected result
type CorrectnessMetric struct {
	CaseSensitive bool
	ExactMatch    bool // If true, requires exact match; if false, uses similarity
}

// Name returns the metric name
func (m *CorrectnessMetric) Name() string {
	return "correctness"
}

// Evaluate checks correctness of the output
func (m *CorrectnessMetric) Evaluate(input, expected, actual string) float64 {
	if expected == "" && actual == "" {
		return 1.0 // Both empty is a match
	}

	if expected == "" || actual == "" {
		return 0.0 // One empty, one not is a mismatch
	}

	// Normalize case if needed
	exp := expected
	act := actual
	if !m.CaseSensitive {
		exp = strings.ToLower(exp)
		act = strings.ToLower(act)
	}

	// Exact match mode
	if m.ExactMatch {
		if exp == act {
			return 1.0
		}
		return 0.0
	}

	// Similarity-based scoring using simple character overlap
	// This is a basic implementation; production would use more sophisticated methods
	return calculateSimilarity(exp, act)
}

// calculateSimilarity computes a simple similarity score based on character overlap
func calculateSimilarity(s1, s2 string) float64 {
	if s1 == s2 {
		return 1.0
	}

	// Simple character-level Jaccard similarity
	set1 := make(map[rune]bool)
	set2 := make(map[rune]bool)

	for _, c := range s1 {
		set1[c] = true
	}
	for _, c := range s2 {
		set2[c] = true
	}

	// Count intersection
	intersection := 0
	for c := range set1 {
		if set2[c] {
			intersection++
		}
	}

	// Count union
	union := len(set1)
	for c := range set2 {
		if !set1[c] {
			union++
		}
	}

	if union == 0 {
		return 0.0
	}

	return float64(intersection) / float64(union)
}

// SafetyMetric checks for harmful content
type SafetyMetric struct {
	HarmfulKeywords []string // List of keywords to check for
	Blocklist       []string // Blocked patterns
}

// Name returns the metric name
func (m *SafetyMetric) Name() string {
	return "safety"
}

// Evaluate checks safety of the output
func (m *SafetyMetric) Evaluate(input, expected, actual string) float64 {
	if actual == "" {
		return 1.0 // Empty output is safe
	}

	actualLower := strings.ToLower(actual)
	violations := 0
	totalChecks := 0

	// Check harmful keywords
	if len(m.HarmfulKeywords) > 0 {
		for _, keyword := range m.HarmfulKeywords {
			totalChecks++
			if strings.Contains(actualLower, strings.ToLower(keyword)) {
				violations++
			}
		}
	}

	// Check blocklist patterns
	if len(m.Blocklist) > 0 {
		for _, pattern := range m.Blocklist {
			totalChecks++
			if strings.Contains(actualLower, strings.ToLower(pattern)) {
				violations++
			}
		}
	}

	// If no checks configured, assume safe
	if totalChecks == 0 {
		return 1.0
	}

	// Return safety score (1.0 = completely safe, 0.0 = all checks failed)
	return 1.0 - (float64(violations) / float64(totalChecks))
}

// DefaultSafetyKeywords returns a default set of harmful keywords
func DefaultSafetyKeywords() []string {
	return []string{
		"violent", "harmful", "dangerous", "illegal",
		"discriminatory", "hateful", "offensive",
	}
}

// PerformanceMetric checks latency and throughput
type PerformanceMetric struct {
	MaxLatencyMs   int64 // Maximum acceptable latency in milliseconds
	MinThroughput  int   // Minimum acceptable throughput (requests/second)
	ActualLatency  int64 // Actual measured latency
	ActualRequests int   // Actual requests processed
	TimeWindowSec  int   // Time window for throughput calculation
}

// Name returns the metric name
func (m *PerformanceMetric) Name() string {
	return "performance"
}

// Evaluate checks performance metrics
func (m *PerformanceMetric) Evaluate(input, expected, actual string) float64 {
	scores := []float64{}

	// Evaluate latency if configured
	if m.MaxLatencyMs > 0 {
		latencyScore := evaluateLatency(m.ActualLatency, m.MaxLatencyMs)
		scores = append(scores, latencyScore)
	}

	// Evaluate throughput if configured
	if m.MinThroughput > 0 && m.TimeWindowSec > 0 {
		actualThroughput := float64(m.ActualRequests) / float64(m.TimeWindowSec)
		throughputScore := evaluateThroughput(actualThroughput, float64(m.MinThroughput))
		scores = append(scores, throughputScore)
	}

	// If no performance metrics configured, return 1.0
	if len(scores) == 0 {
		return 1.0
	}

	// Return average of all performance scores
	sum := 0.0
	for _, score := range scores {
		sum += score
	}
	return sum / float64(len(scores))
}

// evaluateLatency returns a score based on latency (1.0 if under threshold, decreasing as latency increases)
func evaluateLatency(actual, maxLatency int64) float64 {
	if actual <= 0 || maxLatency <= 0 {
		return 1.0
	}

	if actual <= maxLatency {
		return 1.0
	}

	// Gradual degradation: score = maxLatency / actual
	// e.g., if maxLatency=1000ms and actual=2000ms, score = 0.5
	score := float64(maxLatency) / float64(actual)
	if score < 0.0 {
		return 0.0
	}
	return score
}

// evaluateThroughput returns a score based on throughput (1.0 if above threshold, decreasing as throughput drops)
func evaluateThroughput(actual, minThroughput float64) float64 {
	if actual <= 0 || minThroughput <= 0 {
		return 1.0
	}

	if actual >= minThroughput {
		return 1.0
	}

	// Gradual degradation: score = actual / minThroughput
	score := actual / minThroughput
	if score < 0.0 {
		return 0.0
	}
	return score
}

// MetricResult holds the result of a metric evaluation
type MetricResult struct {
	MetricName string
	Score      float64
	Pass       bool
	Timestamp  time.Time
}

// EvaluateMetrics runs multiple metrics on a test case
func EvaluateMetrics(input, expected, actual string, metrics []Metric, thresholds map[string]float64) []MetricResult {
	results := make([]MetricResult, 0, len(metrics))
	timestamp := time.Now()

	for _, metric := range metrics {
		score := metric.Evaluate(input, expected, actual)
		threshold := thresholds[metric.Name()]
		if threshold == 0 {
			threshold = 0.7 // Default threshold
		}

		results = append(results, MetricResult{
			MetricName: metric.Name(),
			Score:      score,
			Pass:       score >= threshold,
			Timestamp:  timestamp,
		})
	}

	return results
}
