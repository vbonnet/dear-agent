package evaluation

import (
	"testing"
	"time"
)

func TestMetricsCollector_EmptyEvents(t *testing.T) {
	collector := NewMetricsCollector()
	metrics := collector.Compute()

	if metrics.SessionSuccessRate != 0 {
		t.Errorf("Expected success rate 0, got %.2f", metrics.SessionSuccessRate)
	}

	if metrics.AgentResponseTime.P50 != 0 {
		t.Errorf("Expected P50 0, got %.2f", metrics.AgentResponseTime.P50)
	}

	if metrics.UserSatisfaction.Ratio != 0 {
		t.Errorf("Expected satisfaction ratio 0, got %.2f", metrics.UserSatisfaction.Ratio)
	}
}

func TestMetricsCollector_SessionSuccessRate(t *testing.T) {
	tests := []struct {
		name     string
		events   []SessionEvent
		expected float64
	}{
		{
			name: "all successful",
			events: []SessionEvent{
				{SessionID: "s1", Success: true, ResponseTime: 100},
				{SessionID: "s2", Success: true, ResponseTime: 200},
				{SessionID: "s3", Success: true, ResponseTime: 300},
			},
			expected: 100.0,
		},
		{
			name: "all failed",
			events: []SessionEvent{
				{SessionID: "s1", Success: false, ResponseTime: 100},
				{SessionID: "s2", Success: false, ResponseTime: 200},
			},
			expected: 0.0,
		},
		{
			name: "mixed success",
			events: []SessionEvent{
				{SessionID: "s1", Success: true, ResponseTime: 100},
				{SessionID: "s2", Success: false, ResponseTime: 200},
				{SessionID: "s3", Success: true, ResponseTime: 300},
				{SessionID: "s4", Success: true, ResponseTime: 400},
			},
			expected: 75.0, // 3 out of 4
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			collector := NewMetricsCollector()
			for _, event := range tt.events {
				collector.AddEvent(event)
			}

			metrics := collector.Compute()
			if metrics.SessionSuccessRate != tt.expected {
				t.Errorf("Expected success rate %.2f, got %.2f",
					tt.expected, metrics.SessionSuccessRate)
			}
		})
	}
}

func TestMetricsCollector_ResponseTimePercentiles(t *testing.T) {
	tests := []struct {
		name          string
		responseTimes []float64
		expectedP50   float64
		expectedP95   float64
		expectedP99   float64
	}{
		{
			name:          "single value",
			responseTimes: []float64{100},
			expectedP50:   100,
			expectedP95:   100,
			expectedP99:   100,
		},
		{
			name:          "two values",
			responseTimes: []float64{100, 200},
			expectedP50:   150, // interpolated
			expectedP95:   195, // interpolated
			expectedP99:   199, // interpolated
		},
		{
			name:          "multiple values",
			responseTimes: []float64{100, 200, 300, 400, 500, 600, 700, 800, 900, 1000},
			expectedP50:   550, // median of 10 values
			expectedP95:   955, // 95th percentile
			expectedP99:   991, // 99th percentile
		},
		{
			name: "realistic latencies",
			responseTimes: []float64{
				50, 75, 100, 125, 150, 175, 200, 250, 300, 350,
				400, 450, 500, 600, 700, 800, 900, 1000, 1500, 2000,
			},
			expectedP50: 375,  // median (between 350 and 400)
			expectedP95: 1525, // 95th percentile (at index 18.05, interpolate between 1500 and 2000)
			expectedP99: 1905, // 99th percentile (at index 18.81, interpolate between 1500 and 2000)
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			collector := NewMetricsCollector()
			for _, rt := range tt.responseTimes {
				collector.AddEvent(SessionEvent{
					SessionID:    "test",
					Success:      true,
					ResponseTime: rt,
				})
			}

			metrics := collector.Compute()

			// Allow small floating point tolerance
			tolerance := 0.1
			if diff := abs(metrics.AgentResponseTime.P50 - tt.expectedP50); diff > tolerance {
				t.Errorf("P50: expected %.2f, got %.2f (diff: %.2f)",
					tt.expectedP50, metrics.AgentResponseTime.P50, diff)
			}
			if diff := abs(metrics.AgentResponseTime.P95 - tt.expectedP95); diff > tolerance {
				t.Errorf("P95: expected %.2f, got %.2f (diff: %.2f)",
					tt.expectedP95, metrics.AgentResponseTime.P95, diff)
			}
			if diff := abs(metrics.AgentResponseTime.P99 - tt.expectedP99); diff > tolerance {
				t.Errorf("P99: expected %.2f, got %.2f (diff: %.2f)",
					tt.expectedP99, metrics.AgentResponseTime.P99, diff)
			}
		})
	}
}

func TestMetricsCollector_UserSatisfaction(t *testing.T) {
	tests := []struct {
		name          string
		events        []SessionEvent
		expectedUp    int
		expectedDown  int
		expectedRatio float64
	}{
		{
			name: "no feedback",
			events: []SessionEvent{
				{SessionID: "s1", Success: true, ResponseTime: 100, Feedback: nil},
				{SessionID: "s2", Success: true, ResponseTime: 200, Feedback: nil},
			},
			expectedUp:    0,
			expectedDown:  0,
			expectedRatio: 0.0,
		},
		{
			name: "all positive",
			events: []SessionEvent{
				{SessionID: "s1", Success: true, ResponseTime: 100, Feedback: &Feedback{Positive: true}},
				{SessionID: "s2", Success: true, ResponseTime: 200, Feedback: &Feedback{Positive: true}},
				{SessionID: "s3", Success: true, ResponseTime: 300, Feedback: &Feedback{Positive: true}},
			},
			expectedUp:    3,
			expectedDown:  0,
			expectedRatio: 1.0,
		},
		{
			name: "all negative",
			events: []SessionEvent{
				{SessionID: "s1", Success: false, ResponseTime: 100, Feedback: &Feedback{Positive: false}},
				{SessionID: "s2", Success: false, ResponseTime: 200, Feedback: &Feedback{Positive: false}},
			},
			expectedUp:    0,
			expectedDown:  2,
			expectedRatio: 0.0,
		},
		{
			name: "mixed feedback",
			events: []SessionEvent{
				{SessionID: "s1", Success: true, ResponseTime: 100, Feedback: &Feedback{Positive: true}},
				{SessionID: "s2", Success: true, ResponseTime: 200, Feedback: &Feedback{Positive: true}},
				{SessionID: "s3", Success: true, ResponseTime: 300, Feedback: &Feedback{Positive: true}},
				{SessionID: "s4", Success: false, ResponseTime: 400, Feedback: &Feedback{Positive: false}},
			},
			expectedUp:    3,
			expectedDown:  1,
			expectedRatio: 0.75,
		},
		{
			name: "some without feedback",
			events: []SessionEvent{
				{SessionID: "s1", Success: true, ResponseTime: 100, Feedback: &Feedback{Positive: true}},
				{SessionID: "s2", Success: true, ResponseTime: 200, Feedback: nil},
				{SessionID: "s3", Success: true, ResponseTime: 300, Feedback: &Feedback{Positive: false}},
			},
			expectedUp:    1,
			expectedDown:  1,
			expectedRatio: 0.5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			collector := NewMetricsCollector()
			for _, event := range tt.events {
				collector.AddEvent(event)
			}

			metrics := collector.Compute()
			if metrics.UserSatisfaction.ThumbsUp != tt.expectedUp {
				t.Errorf("Expected %d thumbs up, got %d",
					tt.expectedUp, metrics.UserSatisfaction.ThumbsUp)
			}
			if metrics.UserSatisfaction.ThumbsDown != tt.expectedDown {
				t.Errorf("Expected %d thumbs down, got %d",
					tt.expectedDown, metrics.UserSatisfaction.ThumbsDown)
			}
			if metrics.UserSatisfaction.Ratio != tt.expectedRatio {
				t.Errorf("Expected ratio %.2f, got %.2f",
					tt.expectedRatio, metrics.UserSatisfaction.Ratio)
			}
		})
	}
}

func TestMetricsCollector_Reset(t *testing.T) {
	collector := NewMetricsCollector()

	// Add some events
	collector.AddEvent(SessionEvent{SessionID: "s1", Success: true, ResponseTime: 100})
	collector.AddEvent(SessionEvent{SessionID: "s2", Success: true, ResponseTime: 200})

	// Verify we have data
	metrics := collector.Compute()
	if metrics.SessionSuccessRate != 100.0 {
		t.Errorf("Expected success rate 100 before reset, got %.2f", metrics.SessionSuccessRate)
	}

	// Reset
	collector.Reset()

	// Verify data is cleared
	metrics = collector.Compute()
	if metrics.SessionSuccessRate != 0 {
		t.Errorf("Expected success rate 0 after reset, got %.2f", metrics.SessionSuccessRate)
	}
}

func TestMetricsCollector_CollectedAtTimestamp(t *testing.T) {
	collector := NewMetricsCollector()
	collector.AddEvent(SessionEvent{SessionID: "s1", Success: true, ResponseTime: 100})

	before := time.Now()
	metrics := collector.Compute()
	after := time.Now()

	if metrics.CollectedAt.Before(before) || metrics.CollectedAt.After(after) {
		t.Errorf("CollectedAt timestamp not within expected range")
	}
}

func TestPercentile_EdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		data     []float64
		p        float64
		expected float64
	}{
		{
			name:     "empty data",
			data:     []float64{},
			p:        0.5,
			expected: 0,
		},
		{
			name:     "single element",
			data:     []float64{42},
			p:        0.95,
			expected: 42,
		},
		{
			name:     "exact index p50",
			data:     []float64{1, 2, 3},
			p:        0.5,
			expected: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := percentile(tt.data, tt.p)
			if result != tt.expected {
				t.Errorf("Expected %.2f, got %.2f", tt.expected, result)
			}
		})
	}
}

// Helper function
func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
