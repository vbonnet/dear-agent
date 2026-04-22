package daemon

import (
	"testing"
	"time"
)

func TestNewMetricsCollector(t *testing.T) {
	mc := NewMetricsCollector()

	if mc == nil {
		t.Fatal("NewMetricsCollector returned nil")
		return
	}

	if mc.stateDetectionAccuracy == nil {
		t.Error("stateDetectionAccuracy map not initialized")
	}

	if mc.deliveryLatencies == nil {
		t.Error("deliveryLatencies slice not initialized")
	}
}

func TestMetricsCollector_RecordDeliveryAttempt(t *testing.T) {
	mc := NewMetricsCollector()

	// Record successful delivery
	mc.RecordDeliveryAttempt(true, 100*time.Millisecond)
	metrics := mc.GetMetrics()

	if metrics.TotalDeliveryAttempts != 1 {
		t.Errorf("Expected 1 total attempt, got %d", metrics.TotalDeliveryAttempts)
	}

	if metrics.TotalMessagesDelivered != 1 {
		t.Errorf("Expected 1 delivered, got %d", metrics.TotalMessagesDelivered)
	}

	if metrics.TotalMessagesFailed != 0 {
		t.Errorf("Expected 0 failed, got %d", metrics.TotalMessagesFailed)
	}

	// Record failed delivery
	mc.RecordDeliveryAttempt(false, 0)
	metrics = mc.GetMetrics()

	if metrics.TotalDeliveryAttempts != 2 {
		t.Errorf("Expected 2 total attempts, got %d", metrics.TotalDeliveryAttempts)
	}

	if metrics.TotalMessagesDelivered != 1 {
		t.Errorf("Expected 1 delivered, got %d", metrics.TotalMessagesDelivered)
	}

	if metrics.TotalMessagesFailed != 1 {
		t.Errorf("Expected 1 failed, got %d", metrics.TotalMessagesFailed)
	}
}

func TestMetricsCollector_SuccessRate(t *testing.T) {
	mc := NewMetricsCollector()

	// Record deliveries: 3 success, 1 failure
	mc.RecordDeliveryAttempt(true, 100*time.Millisecond)
	mc.RecordDeliveryAttempt(true, 150*time.Millisecond)
	mc.RecordDeliveryAttempt(true, 200*time.Millisecond)
	mc.RecordDeliveryAttempt(false, 0)

	metrics := mc.GetMetrics()

	expectedSuccessRate := 75.0 // 3/4 * 100
	if metrics.SuccessRate != expectedSuccessRate {
		t.Errorf("Expected success rate %.2f%%, got %.2f%%",
			expectedSuccessRate, metrics.SuccessRate)
	}
}

func TestMetricsCollector_DeliveryLatency(t *testing.T) {
	mc := NewMetricsCollector()

	latencies := []time.Duration{
		100 * time.Millisecond,
		200 * time.Millisecond,
		300 * time.Millisecond,
	}

	for _, lat := range latencies {
		mc.RecordDeliveryAttempt(true, lat)
	}

	metrics := mc.GetMetrics()

	expectedAvg := 200 * time.Millisecond
	if metrics.AvgDeliveryLatency != expectedAvg {
		t.Errorf("Expected avg latency %v, got %v",
			expectedAvg, metrics.AvgDeliveryLatency)
	}

	expectedMin := 100 * time.Millisecond
	if metrics.MinDeliveryLatency != expectedMin {
		t.Errorf("Expected min latency %v, got %v",
			expectedMin, metrics.MinDeliveryLatency)
	}

	expectedMax := 300 * time.Millisecond
	if metrics.MaxDeliveryLatency != expectedMax {
		t.Errorf("Expected max latency %v, got %v",
			expectedMax, metrics.MaxDeliveryLatency)
	}
}

func TestMetricsCollector_StateDetection(t *testing.T) {
	mc := NewMetricsCollector()

	// Record state detections
	mc.RecordStateDetection("DONE")
	mc.RecordStateDetection("DONE")
	mc.RecordStateDetection("WORKING")
	mc.RecordStateDetectionError()

	metrics := mc.GetMetrics()

	if count, ok := metrics.StateDetectionAccuracy["DONE"]; !ok || count != 2 {
		t.Errorf("Expected 2 DONE detections, got %d", count)
	}

	if count, ok := metrics.StateDetectionAccuracy["WORKING"]; !ok || count != 1 {
		t.Errorf("Expected 1 WORKING detection, got %d", count)
	}

	if metrics.StateDetectionErrors != 1 {
		t.Errorf("Expected 1 state detection error, got %d",
			metrics.StateDetectionErrors)
	}
}

func TestMetricsCollector_QueueDepth(t *testing.T) {
	mc := NewMetricsCollector()

	mc.UpdateQueueDepth(42)
	metrics := mc.GetMetrics()

	if metrics.CurrentQueueDepth != 42 {
		t.Errorf("Expected queue depth 42, got %d", metrics.CurrentQueueDepth)
	}
}

func TestMetricsCollector_Poll(t *testing.T) {
	mc := NewMetricsCollector()

	pollDuration := 500 * time.Millisecond
	mc.RecordPoll(pollDuration)

	metrics := mc.GetMetrics()

	if metrics.LastPollDuration != pollDuration {
		t.Errorf("Expected poll duration %v, got %v",
			pollDuration, metrics.LastPollDuration)
	}

	if metrics.LastPollTime.IsZero() {
		t.Error("Last poll time should not be zero")
	}
}

func TestAlertRules_HighQueueDepth(t *testing.T) {
	rules := GetDefaultAlertRules()

	tests := []struct {
		name          string
		queueDepth    int
		expectedLevel AlertLevel
		shouldAlert   bool
	}{
		{"Normal queue", 10, AlertLevelInfo, false},
		{"Warning threshold", 51, AlertLevelWarning, true},
		{"Critical threshold", 101, AlertLevelCritical, true},
		{"Edge case - exactly 50", 50, AlertLevelInfo, false},
		{"Edge case - exactly 100", 100, AlertLevelWarning, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metrics := MetricsSnapshot{
				CurrentQueueDepth: tt.queueDepth,
			}

			alerts := CheckAlerts(metrics, rules)

			if tt.shouldAlert {
				if len(alerts) == 0 {
					t.Error("Expected alert but got none")
				} else {
					foundQueueAlert := false
					for _, alert := range alerts {
						if alert.Metric == "queue_depth" {
							foundQueueAlert = true
							if alert.Level != tt.expectedLevel {
								t.Errorf("Expected alert level %v, got %v",
									tt.expectedLevel, alert.Level)
							}
						}
					}
					if !foundQueueAlert {
						t.Error("Expected queue_depth alert but didn't find one")
					}
				}
			} else {
				for _, alert := range alerts {
					if alert.Metric == "queue_depth" {
						t.Errorf("Unexpected queue_depth alert: %v", alert)
					}
				}
			}
		})
	}
}

func TestAlertRules_HighFailureRate(t *testing.T) {
	rules := GetDefaultAlertRules()

	tests := []struct {
		name          string
		delivered     int64
		failed        int64
		expectedLevel AlertLevel
		shouldAlert   bool
	}{
		{"High success rate", 90, 10, AlertLevelInfo, false},
		{"Warning threshold", 74, 26, AlertLevelWarning, true},
		{"Critical threshold", 49, 51, AlertLevelCritical, true},
		{"Too few attempts", 5, 5, AlertLevelInfo, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			total := tt.delivered + tt.failed
			successRate := float64(tt.delivered) / float64(total) * 100

			metrics := MetricsSnapshot{
				TotalMessagesDelivered: tt.delivered,
				TotalMessagesFailed:    tt.failed,
				TotalDeliveryAttempts:  total,
				SuccessRate:            successRate,
			}

			alerts := CheckAlerts(metrics, rules)

			if tt.shouldAlert {
				foundFailureAlert := false
				for _, alert := range alerts {
					if alert.Metric == "success_rate" {
						foundFailureAlert = true
						if alert.Level != tt.expectedLevel {
							t.Errorf("Expected alert level %v, got %v",
								tt.expectedLevel, alert.Level)
						}
					}
				}
				if !foundFailureAlert {
					t.Error("Expected success_rate alert but didn't find one")
				}
			}
		})
	}
}

func TestAlertRules_HighLatency(t *testing.T) {
	rules := GetDefaultAlertRules()

	tests := []struct {
		name          string
		avgLatency    time.Duration
		expectedLevel AlertLevel
		shouldAlert   bool
	}{
		{"Normal latency", 2 * time.Second, AlertLevelInfo, false},
		{"Warning threshold", 11 * time.Second, AlertLevelWarning, true},
		{"Critical threshold", 31 * time.Second, AlertLevelCritical, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metrics := MetricsSnapshot{
				AvgDeliveryLatency: tt.avgLatency,
			}

			alerts := CheckAlerts(metrics, rules)

			if tt.shouldAlert {
				foundLatencyAlert := false
				for _, alert := range alerts {
					if alert.Metric == "avg_latency" {
						foundLatencyAlert = true
						if alert.Level != tt.expectedLevel {
							t.Errorf("Expected alert level %v, got %v",
								tt.expectedLevel, alert.Level)
						}
					}
				}
				if !foundLatencyAlert {
					t.Error("Expected avg_latency alert but didn't find one")
				}
			}
		})
	}
}

func TestAlertRules_DaemonNotPolling(t *testing.T) {
	rules := GetDefaultAlertRules()

	tests := []struct {
		name        string
		lastPoll    time.Time
		shouldAlert bool
	}{
		{"Recent poll", time.Now().Add(-1 * time.Minute), false},
		{"Old poll", time.Now().Add(-6 * time.Minute), true},
		{"No poll yet", time.Time{}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metrics := MetricsSnapshot{
				LastPollTime: tt.lastPoll,
			}

			alerts := CheckAlerts(metrics, rules)

			foundPollAlert := false
			for _, alert := range alerts {
				if alert.Metric == "last_poll" {
					foundPollAlert = true
				}
			}

			if tt.shouldAlert && !foundPollAlert {
				t.Error("Expected last_poll alert but didn't find one")
			} else if !tt.shouldAlert && foundPollAlert {
				t.Error("Unexpected last_poll alert")
			}
		})
	}
}

func TestAlertRules_StateDetectionErrorRate(t *testing.T) {
	rules := GetDefaultAlertRules()

	tests := []struct {
		name          string
		successCount  int64
		errorCount    int64
		expectedLevel AlertLevel
		shouldAlert   bool
	}{
		{"Low error rate", 95, 5, AlertLevelInfo, false},
		{"Warning threshold", 89, 11, AlertLevelWarning, true},
		{"Critical threshold", 74, 26, AlertLevelCritical, true},
		{"Too few detections", 5, 1, AlertLevelInfo, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metrics := MetricsSnapshot{
				StateDetectionAccuracy: map[string]int64{
					"DONE": tt.successCount,
				},
				StateDetectionErrors: tt.errorCount,
			}

			alerts := CheckAlerts(metrics, rules)

			if tt.shouldAlert {
				foundErrorAlert := false
				for _, alert := range alerts {
					if alert.Metric == "state_detection_error_rate" {
						foundErrorAlert = true
						if alert.Level != tt.expectedLevel {
							t.Errorf("Expected alert level %v, got %v",
								tt.expectedLevel, alert.Level)
						}
					}
				}
				if !foundErrorAlert {
					t.Error("Expected state_detection_error_rate alert but didn't find one")
				}
			}
		})
	}
}

func TestMetricsSnapshot_String(t *testing.T) {
	snapshot := MetricsSnapshot{
		Uptime:                 1 * time.Hour,
		TotalMessagesDelivered: 100,
		TotalMessagesFailed:    10,
		TotalDeliveryAttempts:  110,
		CurrentQueueDepth:      5,
		SuccessRate:            90.91,
		AvgDeliveryLatency:     500 * time.Millisecond,
		MinDeliveryLatency:     100 * time.Millisecond,
		MaxDeliveryLatency:     2 * time.Second,
		LastPollTime:           time.Now(),
		LastPollDuration:       200 * time.Millisecond,
		StateDetectionErrors:   2,
	}

	str := snapshot.String()

	// Verify that key metrics are in the output
	expectedStrings := []string{
		"Uptime:",
		"Messages Delivered: 100",
		"Messages Failed: 10",
		"Total Attempts: 110",
		"Success Rate: 90.91%",
		"Current Queue Depth: 5",
		"State Detection Errors: 2",
	}

	for _, expected := range expectedStrings {
		if !contains(str, expected) {
			t.Errorf("Expected output to contain %q, got:\n%s", expected, str)
		}
	}
}

func TestAlertLevel_String(t *testing.T) {
	tests := []struct {
		level    AlertLevel
		expected string
	}{
		{AlertLevelInfo, "INFO"},
		{AlertLevelWarning, "WARNING"},
		{AlertLevelCritical, "CRITICAL"},
		{AlertLevel(999), "UNKNOWN"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if got := tt.level.String(); got != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, got)
			}
		})
	}
}

// Helper function to check if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
