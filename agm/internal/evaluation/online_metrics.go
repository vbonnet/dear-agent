package evaluation

import (
	"math"
	"sort"
	"time"
)

// OnlineMetrics tracks production system health metrics.
type OnlineMetrics struct {
	// SessionSuccessRate is the percentage of sessions without errors
	SessionSuccessRate float64

	// AgentResponseTime tracks latency percentiles in milliseconds
	AgentResponseTime ResponseTimeMetrics

	// UserSatisfaction is the thumbs up/down ratio
	UserSatisfaction SatisfactionMetrics

	// Timestamp when metrics were collected
	CollectedAt time.Time
}

// ResponseTimeMetrics captures latency percentiles.
type ResponseTimeMetrics struct {
	P50 float64 // Median response time in ms
	P95 float64 // 95th percentile in ms
	P99 float64 // 99th percentile in ms
}

// SatisfactionMetrics captures user feedback.
type SatisfactionMetrics struct {
	ThumbsUp   int     // Number of positive feedback
	ThumbsDown int     // Number of negative feedback
	Ratio      float64 // ThumbsUp / (ThumbsUp + ThumbsDown)
}

// SessionEvent represents a session completion event from production.
type SessionEvent struct {
	SessionID    string
	Success      bool      // false if session had errors
	ResponseTime float64   // in milliseconds
	Feedback     *Feedback // nil if no feedback provided
	Timestamp    time.Time
}

// Feedback represents user satisfaction feedback.
type Feedback struct {
	Positive bool // true for thumbs up, false for thumbs down
}

// MetricsCollector aggregates session events into metrics.
type MetricsCollector struct {
	events []SessionEvent
}

// NewMetricsCollector creates a new metrics collector.
func NewMetricsCollector() *MetricsCollector {
	return &MetricsCollector{
		events: make([]SessionEvent, 0),
	}
}

// AddEvent records a session event.
func (mc *MetricsCollector) AddEvent(event SessionEvent) {
	mc.events = append(mc.events, event)
}

// Compute calculates metrics from collected events.
func (mc *MetricsCollector) Compute() OnlineMetrics {
	if len(mc.events) == 0 {
		return OnlineMetrics{
			SessionSuccessRate: 0,
			AgentResponseTime: ResponseTimeMetrics{
				P50: 0,
				P95: 0,
				P99: 0,
			},
			UserSatisfaction: SatisfactionMetrics{
				ThumbsUp:   0,
				ThumbsDown: 0,
				Ratio:      0,
			},
			CollectedAt: time.Now(),
		}
	}

	return OnlineMetrics{
		SessionSuccessRate: mc.computeSuccessRate(),
		AgentResponseTime:  mc.computeResponseTime(),
		UserSatisfaction:   mc.computeSatisfaction(),
		CollectedAt:        time.Now(),
	}
}

func (mc *MetricsCollector) computeSuccessRate() float64 {
	if len(mc.events) == 0 {
		return 0
	}

	successCount := 0
	for _, event := range mc.events {
		if event.Success {
			successCount++
		}
	}

	return float64(successCount) / float64(len(mc.events)) * 100.0
}

func (mc *MetricsCollector) computeResponseTime() ResponseTimeMetrics {
	if len(mc.events) == 0 {
		return ResponseTimeMetrics{P50: 0, P95: 0, P99: 0}
	}

	// Extract response times
	times := make([]float64, 0, len(mc.events))
	for _, event := range mc.events {
		times = append(times, event.ResponseTime)
	}

	// Sort for percentile calculation
	sort.Float64s(times)

	return ResponseTimeMetrics{
		P50: percentile(times, 0.50),
		P95: percentile(times, 0.95),
		P99: percentile(times, 0.99),
	}
}

func (mc *MetricsCollector) computeSatisfaction() SatisfactionMetrics {
	thumbsUp := 0
	thumbsDown := 0

	for _, event := range mc.events {
		if event.Feedback != nil {
			if event.Feedback.Positive {
				thumbsUp++
			} else {
				thumbsDown++
			}
		}
	}

	total := thumbsUp + thumbsDown
	ratio := 0.0
	if total > 0 {
		ratio = float64(thumbsUp) / float64(total)
	}

	return SatisfactionMetrics{
		ThumbsUp:   thumbsUp,
		ThumbsDown: thumbsDown,
		Ratio:      ratio,
	}
}

// percentile calculates the percentile value from sorted data.
// p is a value between 0 and 1 (e.g., 0.50 for median, 0.95 for P95).
func percentile(sortedData []float64, p float64) float64 {
	if len(sortedData) == 0 {
		return 0
	}

	if len(sortedData) == 1 {
		return sortedData[0]
	}

	// Linear interpolation between closest ranks
	rank := p * float64(len(sortedData)-1)
	lowerIndex := int(math.Floor(rank))
	upperIndex := int(math.Ceil(rank))

	if lowerIndex == upperIndex {
		return sortedData[lowerIndex]
	}

	// Interpolate
	fraction := rank - float64(lowerIndex)
	return sortedData[lowerIndex] + fraction*(sortedData[upperIndex]-sortedData[lowerIndex])
}

// Reset clears all collected events.
func (mc *MetricsCollector) Reset() {
	mc.events = make([]SessionEvent, 0)
}
