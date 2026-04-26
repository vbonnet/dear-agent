package evaluation

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"strconv"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/vbonnet/dear-agent/agm/internal/logging"
)

const defaultEvalSampleRate = 0.10

var logger = logging.DefaultLogger()

// Alerter defines the interface for sending alerts.
type Alerter interface {
	Alert(ctx context.Context, message string) error
}

// ThresholdConfig defines metric thresholds for alerting.
type ThresholdConfig struct {
	MinSuccessRate     float64 // Minimum acceptable success rate (%)
	MaxP95ResponseTime float64 // Maximum acceptable P95 latency (ms)
	MaxP99ResponseTime float64 // Maximum acceptable P99 latency (ms)
	MinSatisfaction    float64 // Minimum acceptable satisfaction ratio
}

// DefaultThresholds returns production-ready threshold values.
func DefaultThresholds() ThresholdConfig {
	return ThresholdConfig{
		MinSuccessRate:     95.0, // 95% sessions should succeed
		MaxP95ResponseTime: 2000, // 2 seconds for P95
		MaxP99ResponseTime: 5000, // 5 seconds for P99
		MinSatisfaction:    0.80, // 80% satisfaction ratio
	}
}

// OnlineEvaluator monitors production traffic and sends alerts on drift.
type OnlineEvaluator struct {
	judge      Judge
	alerters   []Alerter
	thresholds ThresholdConfig
	collector  *MetricsCollector
	sampleRate float64
	mu         sync.RWMutex
	stopCh     chan struct{}
	running    bool
}

// NewOnlineEvaluator creates a new online evaluator.
func NewOnlineEvaluator(judge Judge, alerters []Alerter, thresholds ThresholdConfig) *OnlineEvaluator {
	return &OnlineEvaluator{
		judge:      judge,
		alerters:   alerters,
		thresholds: thresholds,
		collector:  NewMetricsCollector(),
		stopCh:     make(chan struct{}),
	}
}

// MonitorProduction starts monitoring production traffic at the given sample rate.
// sampleRate should be between 0 and 1 (e.g., 0.1 for 10% sampling).
func (oe *OnlineEvaluator) MonitorProduction(ctx context.Context, sampleRate float64) error {
	if sampleRate <= 0 || sampleRate > 1 {
		return fmt.Errorf("sample rate must be between 0 and 1, got %.2f", sampleRate)
	}

	oe.mu.Lock()
	if oe.running {
		oe.mu.Unlock()
		return fmt.Errorf("monitor already running")
	}
	oe.running = true
	oe.sampleRate = sampleRate
	oe.stopCh = make(chan struct{}) // Recreate channel for restart
	oe.mu.Unlock()

	return nil
}

// Stop halts the monitoring process.
func (oe *OnlineEvaluator) Stop() {
	oe.mu.Lock()
	defer oe.mu.Unlock()

	if oe.running {
		close(oe.stopCh)
		oe.running = false
	}
}

// IsRunning returns true if the evaluator is currently monitoring.
func (oe *OnlineEvaluator) IsRunning() bool {
	oe.mu.RLock()
	defer oe.mu.RUnlock()
	return oe.running
}

// ProcessEvent processes a production session event.
// It samples events based on the configured sample rate.
func (oe *OnlineEvaluator) ProcessEvent(ctx context.Context, event SessionEvent) error {
	oe.mu.RLock()
	if !oe.running {
		oe.mu.RUnlock()
		return fmt.Errorf("evaluator not running")
	}
	sampleRate := oe.sampleRate
	oe.mu.RUnlock()

	// Sample the event
	sampled := shouldSample(sampleRate)

	tracer := otel.Tracer("engram/evaluation")
	ctx, span := tracer.Start(ctx, "evaluation.sample",
		trace.WithAttributes(
			attribute.String("session.id", event.SessionID),
			attribute.Float64("evaluation.sample_rate", sampleRate),
			attribute.Bool("evaluation.sampled", sampled),
		))
	defer span.End()

	if !sampled {
		return nil
	}

	// Add event to collector
	oe.mu.Lock()
	oe.collector.AddEvent(event)
	oe.mu.Unlock()

	span.SetAttributes(
		attribute.Bool("session.success", event.Success),
		attribute.Float64("session.response_time_ms", event.ResponseTime),
	)

	return nil
}

// CheckMetrics computes current metrics and triggers alerts if thresholds are breached.
func (oe *OnlineEvaluator) CheckMetrics(ctx context.Context) error {
	oe.mu.RLock()
	metrics := oe.collector.Compute()
	thresholds := oe.thresholds
	alerters := oe.alerters
	oe.mu.RUnlock()

	// Check each threshold
	violations := []string{}

	if metrics.SessionSuccessRate < thresholds.MinSuccessRate {
		violations = append(violations, fmt.Sprintf(
			"Session success rate %.2f%% below threshold %.2f%%",
			metrics.SessionSuccessRate, thresholds.MinSuccessRate))
	}

	if metrics.AgentResponseTime.P95 > thresholds.MaxP95ResponseTime {
		violations = append(violations, fmt.Sprintf(
			"P95 response time %.2fms exceeds threshold %.2fms",
			metrics.AgentResponseTime.P95, thresholds.MaxP95ResponseTime))
	}

	if metrics.AgentResponseTime.P99 > thresholds.MaxP99ResponseTime {
		violations = append(violations, fmt.Sprintf(
			"P99 response time %.2fms exceeds threshold %.2fms",
			metrics.AgentResponseTime.P99, thresholds.MaxP99ResponseTime))
	}

	if metrics.UserSatisfaction.Ratio < thresholds.MinSatisfaction {
		violations = append(violations, fmt.Sprintf(
			"User satisfaction %.2f below threshold %.2f",
			metrics.UserSatisfaction.Ratio, thresholds.MinSatisfaction))
	}

	// Send alerts for violations
	if len(violations) > 0 {
		message := fmt.Sprintf("EDD Alert: %d threshold violations detected:\n", len(violations))
		for i, v := range violations {
			message += fmt.Sprintf("%d. %s\n", i+1, v)
		}
		message += fmt.Sprintf("\nMetrics collected at: %s", metrics.CollectedAt.Format(time.RFC3339))

		for _, alerter := range alerters {
			if err := alerter.Alert(ctx, message); err != nil {
				logger.Warn("Failed to send alert", "error", err)
			}
		}
	}

	return nil
}

// GetMetrics returns the current metrics snapshot.
func (oe *OnlineEvaluator) GetMetrics() OnlineMetrics {
	oe.mu.RLock()
	defer oe.mu.RUnlock()
	return oe.collector.Compute()
}

// ResetMetrics clears all collected metrics.
func (oe *OnlineEvaluator) ResetMetrics() {
	oe.mu.Lock()
	defer oe.mu.Unlock()
	oe.collector.Reset()
}

// EnvSampleRate reads the evaluation sample rate from ENGRAM_EVAL_SAMPLE_RATE,
// falling back to defaultEvalSampleRate (0.10) if unset or invalid.
func EnvSampleRate() float64 {
	raw := os.Getenv("ENGRAM_EVAL_SAMPLE_RATE")
	if raw == "" {
		return defaultEvalSampleRate
	}
	rate, err := strconv.ParseFloat(raw, 64)
	if err != nil || rate <= 0 || rate > 1 {
		return defaultEvalSampleRate
	}
	return rate
}

// shouldSample returns true if an event should be sampled based on the rate.
func shouldSample(rate float64) bool {
	return rand.Float64() < rate //nolint:gosec // This is for sampling, not cryptographic security
}

// LogAlerter logs alerts to standard output.
type LogAlerter struct{}

// NewLogAlerter creates a new log alerter.
func NewLogAlerter() *LogAlerter {
	return &LogAlerter{}
}

// Alert logs the alert message.
func (la *LogAlerter) Alert(ctx context.Context, message string) error {
	logger.Info("[ALERT]", "message", message)
	return nil
}

// WebhookAlerter sends alerts to a webhook URL.
type WebhookAlerter struct {
	URL string
}

// NewWebhookAlerter creates a new webhook alerter.
func NewWebhookAlerter(url string) *WebhookAlerter {
	return &WebhookAlerter{URL: url}
}

// Alert sends the alert to the webhook (stub implementation).
func (wa *WebhookAlerter) Alert(ctx context.Context, message string) error {
	// In production, this would make an HTTP POST to wa.URL
	logger.Info("[WEBHOOK ALERT]", "url", wa.URL, "message", message)
	return nil
}

// EmailAlerter sends alerts via email.
type EmailAlerter struct {
	To      []string
	Subject string
}

// NewEmailAlerter creates a new email alerter.
func NewEmailAlerter(to []string, subject string) *EmailAlerter {
	return &EmailAlerter{
		To:      to,
		Subject: subject,
	}
}

// Alert sends the alert via email (stub implementation).
func (ea *EmailAlerter) Alert(ctx context.Context, message string) error {
	// In production, this would send an email via SMTP
	logger.Info("[EMAIL ALERT]", "to", ea.To, "subject", ea.Subject, "message", message)
	return nil
}
