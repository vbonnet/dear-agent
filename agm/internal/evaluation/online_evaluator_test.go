package evaluation

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
)

// MockJudge implements Judge interface for testing.
type MockJudge struct {
	score float64
	err   error
}

func (mj *MockJudge) Evaluate(ctx context.Context, prompt string, response string) (float64, error) {
	if mj.err != nil {
		return 0, mj.err
	}
	return mj.score, nil
}

// MockAlerter implements Alerter interface for testing.
type MockAlerter struct {
	mu        sync.Mutex
	alerts    []string
	shouldErr bool
}

func (ma *MockAlerter) Alert(ctx context.Context, message string) error {
	ma.mu.Lock()
	defer ma.mu.Unlock()

	if ma.shouldErr {
		return fmt.Errorf("mock alert error")
	}

	ma.alerts = append(ma.alerts, message)
	return nil
}

func (ma *MockAlerter) GetAlerts() []string {
	ma.mu.Lock()
	defer ma.mu.Unlock()
	return append([]string{}, ma.alerts...)
}

func (ma *MockAlerter) AlertCount() int {
	ma.mu.Lock()
	defer ma.mu.Unlock()
	return len(ma.alerts)
}

func TestNewOnlineEvaluator(t *testing.T) {
	judge := &MockJudge{score: 0.9}
	alerter := &MockAlerter{}
	thresholds := DefaultThresholds()

	evaluator := NewOnlineEvaluator(judge, []Alerter{alerter}, thresholds)

	if evaluator == nil {
		t.Fatal("Expected non-nil evaluator")
	}

	if evaluator.judge != judge {
		t.Error("Judge not set correctly")
	}

	if len(evaluator.alerters) != 1 {
		t.Errorf("Expected 1 alerter, got %d", len(evaluator.alerters))
	}

	if evaluator.running {
		t.Error("Evaluator should not be running initially")
	}
}

func TestOnlineEvaluator_MonitorProduction(t *testing.T) {
	tests := []struct {
		name        string
		sampleRate  float64
		expectError bool
	}{
		{"valid rate 10%", 0.1, false},
		{"valid rate 50%", 0.5, false},
		{"valid rate 100%", 1.0, false},
		{"invalid rate 0", 0.0, true},
		{"invalid rate negative", -0.1, true},
		{"invalid rate > 1", 1.5, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			evaluator := NewOnlineEvaluator(&MockJudge{}, []Alerter{&MockAlerter{}}, DefaultThresholds())
			err := evaluator.MonitorProduction(context.Background(), tt.sampleRate)

			if tt.expectError && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}

			if !tt.expectError && !evaluator.IsRunning() {
				t.Error("Evaluator should be running after MonitorProduction")
			}

			evaluator.Stop()
		})
	}
}

func TestOnlineEvaluator_MonitorAlreadyRunning(t *testing.T) {
	evaluator := NewOnlineEvaluator(&MockJudge{}, []Alerter{&MockAlerter{}}, DefaultThresholds())

	// Start monitoring
	err := evaluator.MonitorProduction(context.Background(), 0.5)
	if err != nil {
		t.Fatalf("Failed to start monitoring: %v", err)
	}

	// Try to start again
	err = evaluator.MonitorProduction(context.Background(), 0.5)
	if err == nil {
		t.Error("Expected error when starting monitor twice")
	}

	evaluator.Stop()
}

func TestOnlineEvaluator_StopAndRestart(t *testing.T) {
	evaluator := NewOnlineEvaluator(&MockJudge{}, []Alerter{&MockAlerter{}}, DefaultThresholds())

	// Start monitoring
	err := evaluator.MonitorProduction(context.Background(), 0.5)
	if err != nil {
		t.Fatalf("Failed to start monitoring: %v", err)
	}

	if !evaluator.IsRunning() {
		t.Error("Evaluator should be running")
	}

	// Stop monitoring
	evaluator.Stop()

	if evaluator.IsRunning() {
		t.Error("Evaluator should not be running after stop")
	}

	// Restart monitoring
	err = evaluator.MonitorProduction(context.Background(), 0.3)
	if err != nil {
		t.Fatalf("Failed to restart monitoring: %v", err)
	}

	if !evaluator.IsRunning() {
		t.Error("Evaluator should be running after restart")
	}

	evaluator.Stop()
}

func TestOnlineEvaluator_ProcessEventWhenNotRunning(t *testing.T) {
	evaluator := NewOnlineEvaluator(&MockJudge{}, []Alerter{&MockAlerter{}}, DefaultThresholds())

	event := SessionEvent{SessionID: "s1", Success: true, ResponseTime: 100}
	err := evaluator.ProcessEvent(context.Background(), event)

	if err == nil {
		t.Error("Expected error when processing event with evaluator not running")
	}
}

func TestOnlineEvaluator_ProcessEventSampling(t *testing.T) {
	evaluator := NewOnlineEvaluator(&MockJudge{}, []Alerter{&MockAlerter{}}, DefaultThresholds())
	err := evaluator.MonitorProduction(context.Background(), 1.0) // 100% sampling
	if err != nil {
		t.Fatalf("Failed to start monitoring: %v", err)
	}
	defer evaluator.Stop()

	// Add events
	for i := 0; i < 10; i++ {
		event := SessionEvent{
			SessionID:    fmt.Sprintf("s%d", i),
			Success:      true,
			ResponseTime: float64(100 * (i + 1)),
		}
		err := evaluator.ProcessEvent(context.Background(), event)
		if err != nil {
			t.Errorf("Failed to process event: %v", err)
		}
	}

	// Check metrics
	metrics := evaluator.GetMetrics()
	if metrics.SessionSuccessRate != 100.0 {
		t.Errorf("Expected 100%% success rate, got %.2f", metrics.SessionSuccessRate)
	}
}

func TestOnlineEvaluator_CheckMetricsNoViolations(t *testing.T) {
	alerter := &MockAlerter{}
	evaluator := NewOnlineEvaluator(&MockJudge{}, []Alerter{alerter}, DefaultThresholds())
	err := evaluator.MonitorProduction(context.Background(), 1.0)
	if err != nil {
		t.Fatalf("Failed to start monitoring: %v", err)
	}
	defer evaluator.Stop()

	// Add events that meet thresholds
	for i := 0; i < 10; i++ {
		event := SessionEvent{
			SessionID:    fmt.Sprintf("s%d", i),
			Success:      true,
			ResponseTime: 500, // Well below P95/P99 thresholds
			Feedback:     &Feedback{Positive: true},
		}
		evaluator.ProcessEvent(context.Background(), event)
	}

	// Check metrics
	err = evaluator.CheckMetrics(context.Background())
	if err != nil {
		t.Errorf("CheckMetrics failed: %v", err)
	}

	// Should have no alerts
	if alerter.AlertCount() != 0 {
		t.Errorf("Expected 0 alerts, got %d", alerter.AlertCount())
	}
}

func TestOnlineEvaluator_CheckMetricsWithViolations(t *testing.T) {
	tests := []struct {
		name           string
		events         []SessionEvent
		expectedAlerts int
		alertContains  string
	}{
		{
			name: "low success rate",
			events: []SessionEvent{
				{SessionID: "s1", Success: false, ResponseTime: 100},
				{SessionID: "s2", Success: false, ResponseTime: 100},
				{SessionID: "s3", Success: false, ResponseTime: 100},
				{SessionID: "s4", Success: true, ResponseTime: 100},
			},
			expectedAlerts: 1,
			alertContains:  "success rate",
		},
		{
			name: "high P95 latency",
			events: []SessionEvent{
				{SessionID: "s1", Success: true, ResponseTime: 3000}, // Above P95 threshold
				{SessionID: "s2", Success: true, ResponseTime: 3000},
				{SessionID: "s3", Success: true, ResponseTime: 100},
			},
			expectedAlerts: 1,
			alertContains:  "P95 response time",
		},
		{
			name: "high P99 latency",
			events: []SessionEvent{
				{SessionID: "s1", Success: true, ResponseTime: 6000}, // Above P99 threshold
				{SessionID: "s2", Success: true, ResponseTime: 100},
				{SessionID: "s3", Success: true, ResponseTime: 100},
			},
			expectedAlerts: 1,
			alertContains:  "P99 response time",
		},
		{
			name: "low satisfaction",
			events: []SessionEvent{
				{SessionID: "s1", Success: true, ResponseTime: 100, Feedback: &Feedback{Positive: false}},
				{SessionID: "s2", Success: true, ResponseTime: 100, Feedback: &Feedback{Positive: false}},
				{SessionID: "s3", Success: true, ResponseTime: 100, Feedback: &Feedback{Positive: false}},
				{SessionID: "s4", Success: true, ResponseTime: 100, Feedback: &Feedback{Positive: true}},
			},
			expectedAlerts: 1,
			alertContains:  "satisfaction",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			alerter := &MockAlerter{}
			evaluator := NewOnlineEvaluator(&MockJudge{}, []Alerter{alerter}, DefaultThresholds())
			err := evaluator.MonitorProduction(context.Background(), 1.0)
			if err != nil {
				t.Fatalf("Failed to start monitoring: %v", err)
			}
			defer evaluator.Stop()

			// Add events
			for _, event := range tt.events {
				evaluator.ProcessEvent(context.Background(), event)
			}

			// Check metrics
			err = evaluator.CheckMetrics(context.Background())
			if err != nil {
				t.Errorf("CheckMetrics failed: %v", err)
			}

			// Verify alerts
			if alerter.AlertCount() != tt.expectedAlerts {
				t.Errorf("Expected %d alerts, got %d", tt.expectedAlerts, alerter.AlertCount())
			}

			// Verify alert content
			if alerter.AlertCount() > 0 {
				alert := alerter.GetAlerts()[0]
				if !strings.Contains(alert, tt.alertContains) {
					t.Errorf("Expected alert to contain '%s', got: %s", tt.alertContains, alert)
				}
			}
		})
	}
}

func TestOnlineEvaluator_MultipleAlerters(t *testing.T) {
	alerter1 := &MockAlerter{}
	alerter2 := &MockAlerter{}
	alerter3 := &MockAlerter{shouldErr: true} // This one will fail

	evaluator := NewOnlineEvaluator(
		&MockJudge{},
		[]Alerter{alerter1, alerter2, alerter3},
		DefaultThresholds(),
	)

	err := evaluator.MonitorProduction(context.Background(), 1.0)
	if err != nil {
		t.Fatalf("Failed to start monitoring: %v", err)
	}
	defer evaluator.Stop()

	// Add events that violate threshold
	for i := 0; i < 10; i++ {
		event := SessionEvent{SessionID: fmt.Sprintf("s%d", i), Success: false, ResponseTime: 100}
		evaluator.ProcessEvent(context.Background(), event)
	}

	// Check metrics - should not fail even if one alerter fails
	err = evaluator.CheckMetrics(context.Background())
	if err != nil {
		t.Errorf("CheckMetrics should not fail even if alerter fails: %v", err)
	}

	// Verify successful alerters received the alert
	if alerter1.AlertCount() != 1 {
		t.Errorf("Alerter1 expected 1 alert, got %d", alerter1.AlertCount())
	}
	if alerter2.AlertCount() != 1 {
		t.Errorf("Alerter2 expected 1 alert, got %d", alerter2.AlertCount())
	}
	if alerter3.AlertCount() != 0 {
		t.Errorf("Alerter3 should have 0 alerts due to error, got %d", alerter3.AlertCount())
	}
}

func TestOnlineEvaluator_ResetMetrics(t *testing.T) {
	evaluator := NewOnlineEvaluator(&MockJudge{}, []Alerter{&MockAlerter{}}, DefaultThresholds())
	err := evaluator.MonitorProduction(context.Background(), 1.0)
	if err != nil {
		t.Fatalf("Failed to start monitoring: %v", err)
	}
	defer evaluator.Stop()

	// Add events
	event := SessionEvent{SessionID: "s1", Success: true, ResponseTime: 100}
	evaluator.ProcessEvent(context.Background(), event)

	// Verify we have metrics
	metrics := evaluator.GetMetrics()
	if metrics.SessionSuccessRate == 0 {
		t.Error("Expected non-zero success rate before reset")
	}

	// Reset
	evaluator.ResetMetrics()

	// Verify metrics are cleared
	metrics = evaluator.GetMetrics()
	if metrics.SessionSuccessRate != 0 {
		t.Errorf("Expected zero success rate after reset, got %.2f", metrics.SessionSuccessRate)
	}
}

func TestLogAlerter(t *testing.T) {
	alerter := NewLogAlerter()
	err := alerter.Alert(context.Background(), "test message")
	if err != nil {
		t.Errorf("LogAlerter.Alert failed: %v", err)
	}
}

func TestWebhookAlerter(t *testing.T) {
	alerter := NewWebhookAlerter("https://example.com/webhook")
	err := alerter.Alert(context.Background(), "test message")
	if err != nil {
		t.Errorf("WebhookAlerter.Alert failed: %v", err)
	}
}

func TestEmailAlerter(t *testing.T) {
	alerter := NewEmailAlerter([]string{"test@example.com"}, "EDD Alert")
	err := alerter.Alert(context.Background(), "test message")
	if err != nil {
		t.Errorf("EmailAlerter.Alert failed: %v", err)
	}
}

func TestDefaultThresholds(t *testing.T) {
	thresholds := DefaultThresholds()

	if thresholds.MinSuccessRate <= 0 || thresholds.MinSuccessRate > 100 {
		t.Errorf("MinSuccessRate should be between 0 and 100, got %.2f", thresholds.MinSuccessRate)
	}

	if thresholds.MaxP95ResponseTime <= 0 {
		t.Errorf("MaxP95ResponseTime should be positive, got %.2f", thresholds.MaxP95ResponseTime)
	}

	if thresholds.MaxP99ResponseTime <= thresholds.MaxP95ResponseTime {
		t.Error("MaxP99ResponseTime should be greater than MaxP95ResponseTime")
	}

	if thresholds.MinSatisfaction < 0 || thresholds.MinSatisfaction > 1 {
		t.Errorf("MinSatisfaction should be between 0 and 1, got %.2f", thresholds.MinSatisfaction)
	}
}

func TestEnvSampleRate(t *testing.T) {
	tests := []struct {
		name     string
		envVal   string
		expected float64
	}{
		{"unset", "", 0.10},
		{"valid 0.5", "0.5", 0.50},
		{"valid 1.0", "1.0", 1.0},
		{"invalid negative", "-0.1", 0.10},
		{"invalid >1", "1.5", 0.10},
		{"invalid text", "abc", 0.10},
		{"valid 0.25", "0.25", 0.25},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envVal != "" {
				t.Setenv("ENGRAM_EVAL_SAMPLE_RATE", tt.envVal)
			}
			got := EnvSampleRate()
			if got != tt.expected {
				t.Errorf("EnvSampleRate() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestShouldSample(t *testing.T) {
	// Test 0% sampling
	sampled := 0
	for i := 0; i < 1000; i++ {
		if shouldSample(0.0) {
			sampled++
		}
	}
	if sampled > 0 {
		t.Error("0% sampling should never sample")
	}

	// Test 100% sampling
	sampled = 0
	for i := 0; i < 1000; i++ {
		if shouldSample(1.0) {
			sampled++
		}
	}
	if sampled != 1000 {
		t.Error("100% sampling should always sample")
	}

	// Test 50% sampling (approximate)
	sampled = 0
	iterations := 10000
	for i := 0; i < iterations; i++ {
		if shouldSample(0.5) {
			sampled++
		}
	}
	// Allow 10% variance
	minSamples := int(float64(iterations) * 0.45)
	maxSamples := int(float64(iterations) * 0.55)
	if sampled < minSamples || sampled > maxSamples {
		t.Errorf("50%% sampling should be around %d, got %d", iterations/2, sampled)
	}
}
