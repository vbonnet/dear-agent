// Package daemon provides background daemon monitoring.
package daemon

import (
	"fmt"
	"maps"
	"sync"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/contracts"
)

// MetricsCollector collects and tracks daemon performance metrics
type MetricsCollector struct {
	mu                     sync.RWMutex
	startTime              time.Time
	totalMessagesDelivered int64
	totalMessagesFailed    int64
	totalDeliveryAttempts  int64
	lastPollTime           time.Time
	lastPollDuration       time.Duration
	queueDepth             int
	stateDetectionAccuracy map[string]int64 // state -> count of correct detections
	stateDetectionErrors   int64
	deliveryLatencies      []time.Duration // last N delivery latencies
	maxLatencyHistory      int
}

// NewMetricsCollector creates a new metrics collector
func NewMetricsCollector() *MetricsCollector {
	slo := contracts.Load()
	histSize := slo.Daemon.LatencyHistorySize

	return &MetricsCollector{
		startTime:              time.Now(),
		stateDetectionAccuracy: make(map[string]int64),
		deliveryLatencies:      make([]time.Duration, 0, histSize),
		maxLatencyHistory:      histSize,
	}
}

// RecordDeliveryAttempt records a delivery attempt
func (m *MetricsCollector) RecordDeliveryAttempt(success bool, latency time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.totalDeliveryAttempts++
	if success {
		m.totalMessagesDelivered++
	} else {
		m.totalMessagesFailed++
	}

	// Track delivery latency
	m.deliveryLatencies = append(m.deliveryLatencies, latency)
	if len(m.deliveryLatencies) > m.maxLatencyHistory {
		m.deliveryLatencies = m.deliveryLatencies[1:]
	}
}

// RecordStateDetection records a successful state detection
func (m *MetricsCollector) RecordStateDetection(state string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.stateDetectionAccuracy[state]++
}

// RecordStateDetectionError records a failed state detection
func (m *MetricsCollector) RecordStateDetectionError() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.stateDetectionErrors++
}

// UpdateQueueDepth updates the current queue depth
func (m *MetricsCollector) UpdateQueueDepth(depth int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.queueDepth = depth
}

// RecordPoll records poll timing information
func (m *MetricsCollector) RecordPoll(duration time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.lastPollTime = time.Now()
	m.lastPollDuration = duration
}

// GetMetrics returns current metrics snapshot
func (m *MetricsCollector) GetMetrics() MetricsSnapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()

	snapshot := MetricsSnapshot{
		Uptime:                 time.Since(m.startTime),
		TotalMessagesDelivered: m.totalMessagesDelivered,
		TotalMessagesFailed:    m.totalMessagesFailed,
		TotalDeliveryAttempts:  m.totalDeliveryAttempts,
		CurrentQueueDepth:      m.queueDepth,
		LastPollTime:           m.lastPollTime,
		LastPollDuration:       m.lastPollDuration,
		StateDetectionAccuracy: make(map[string]int64),
		StateDetectionErrors:   m.stateDetectionErrors,
	}

	// Copy state detection map
	maps.Copy(snapshot.StateDetectionAccuracy, m.stateDetectionAccuracy)

	// Calculate latency statistics
	if len(m.deliveryLatencies) > 0 {
		var total time.Duration
		min := m.deliveryLatencies[0]
		max := m.deliveryLatencies[0]

		for _, lat := range m.deliveryLatencies {
			total += lat
			if lat < min {
				min = lat
			}
			if lat > max {
				max = lat
			}
		}

		snapshot.AvgDeliveryLatency = total / time.Duration(len(m.deliveryLatencies))
		snapshot.MinDeliveryLatency = min
		snapshot.MaxDeliveryLatency = max
	}

	// Calculate success rate
	if m.totalDeliveryAttempts > 0 {
		snapshot.SuccessRate = float64(m.totalMessagesDelivered) / float64(m.totalDeliveryAttempts) * 100
	}

	return snapshot
}

// MetricsSnapshot represents a point-in-time snapshot of daemon metrics
type MetricsSnapshot struct {
	Uptime                 time.Duration
	TotalMessagesDelivered int64
	TotalMessagesFailed    int64
	TotalDeliveryAttempts  int64
	CurrentQueueDepth      int
	LastPollTime           time.Time
	LastPollDuration       time.Duration
	AvgDeliveryLatency     time.Duration
	MinDeliveryLatency     time.Duration
	MaxDeliveryLatency     time.Duration
	SuccessRate            float64
	StateDetectionAccuracy map[string]int64
	StateDetectionErrors   int64
}

// String returns a human-readable representation of metrics
func (s MetricsSnapshot) String() string {
	return fmt.Sprintf(
		"Uptime: %v\n"+
			"Messages Delivered: %d\n"+
			"Messages Failed: %d\n"+
			"Total Attempts: %d\n"+
			"Success Rate: %.2f%%\n"+
			"Current Queue Depth: %d\n"+
			"Avg Delivery Latency: %v\n"+
			"Min/Max Latency: %v / %v\n"+
			"Last Poll: %v (took %v)\n"+
			"State Detection Errors: %d",
		s.Uptime,
		s.TotalMessagesDelivered,
		s.TotalMessagesFailed,
		s.TotalDeliveryAttempts,
		s.SuccessRate,
		s.CurrentQueueDepth,
		s.AvgDeliveryLatency,
		s.MinDeliveryLatency,
		s.MaxDeliveryLatency,
		s.LastPollTime.Format(time.RFC3339),
		s.LastPollDuration,
		s.StateDetectionErrors,
	)
}

// Alert represents a monitoring alert
type Alert struct {
	Level     AlertLevel
	Timestamp time.Time
	Message   string
	Metric    string
	Value     any
	Threshold any
}

// AlertLevel represents the severity of an alert
type AlertLevel int

const (
	AlertLevelInfo AlertLevel = iota
	AlertLevelWarning
	AlertLevelCritical
)

func (l AlertLevel) String() string {
	switch l {
	case AlertLevelInfo:
		return "INFO"
	case AlertLevelWarning:
		return "WARNING"
	case AlertLevelCritical:
		return "CRITICAL"
	default:
		return "UNKNOWN"
	}
}

// AlertRule defines conditions for triggering alerts
type AlertRule struct {
	Name      string
	Condition func(MetricsSnapshot) *Alert
}

// GetDefaultAlertRules returns the default set of alert rules
func GetDefaultAlertRules() []AlertRule {
	slo := contracts.Load()
	da := slo.DaemonAlerts

	return []AlertRule{
		{
			Name: "High Queue Depth",
			Condition: func(m MetricsSnapshot) *Alert {
				if m.CurrentQueueDepth > da.QueueDepthCritical {
					return &Alert{
						Level:     AlertLevelCritical,
						Timestamp: time.Now(),
						Message:   "Queue depth exceeds critical threshold",
						Metric:    "queue_depth",
						Value:     m.CurrentQueueDepth,
						Threshold: da.QueueDepthCritical,
					}
				}
				if m.CurrentQueueDepth > da.QueueDepthWarning {
					return &Alert{
						Level:     AlertLevelWarning,
						Timestamp: time.Now(),
						Message:   "Queue depth exceeds warning threshold",
						Metric:    "queue_depth",
						Value:     m.CurrentQueueDepth,
						Threshold: da.QueueDepthWarning,
					}
				}
				return nil
			},
		},
		{
			Name: "High Failure Rate",
			Condition: func(m MetricsSnapshot) *Alert {
				if m.TotalDeliveryAttempts > 10 && m.SuccessRate < da.SuccessRateCritical {
					return &Alert{
						Level:     AlertLevelCritical,
						Timestamp: time.Now(),
						Message:   "Delivery success rate critically low",
						Metric:    "success_rate",
						Value:     m.SuccessRate,
						Threshold: da.SuccessRateCritical,
					}
				}
				if m.TotalDeliveryAttempts > 10 && m.SuccessRate < da.SuccessRateWarning {
					return &Alert{
						Level:     AlertLevelWarning,
						Timestamp: time.Now(),
						Message:   "Delivery success rate below normal",
						Metric:    "success_rate",
						Value:     m.SuccessRate,
						Threshold: da.SuccessRateWarning,
					}
				}
				return nil
			},
		},
		{
			Name: "High Delivery Latency",
			Condition: func(m MetricsSnapshot) *Alert {
				if m.AvgDeliveryLatency > da.LatencyCritical.Duration {
					return &Alert{
						Level:     AlertLevelCritical,
						Timestamp: time.Now(),
						Message:   "Average delivery latency critically high",
						Metric:    "avg_latency",
						Value:     m.AvgDeliveryLatency,
						Threshold: da.LatencyCritical.Duration,
					}
				}
				if m.AvgDeliveryLatency > da.LatencyWarning.Duration {
					return &Alert{
						Level:     AlertLevelWarning,
						Timestamp: time.Now(),
						Message:   "Average delivery latency elevated",
						Metric:    "avg_latency",
						Value:     m.AvgDeliveryLatency,
						Threshold: da.LatencyWarning.Duration,
					}
				}
				return nil
			},
		},
		{
			Name: "Daemon Not Polling",
			Condition: func(m MetricsSnapshot) *Alert {
				if !m.LastPollTime.IsZero() && time.Since(m.LastPollTime) > da.PollTimeout.Duration {
					return &Alert{
						Level:     AlertLevelCritical,
						Timestamp: time.Now(),
						Message:   fmt.Sprintf("Daemon has not polled for over %s", da.PollTimeout.Duration),
						Metric:    "last_poll",
						Value:     m.LastPollTime,
						Threshold: da.PollTimeout.Duration,
					}
				}
				return nil
			},
		},
		{
			Name: "High State Detection Error Rate",
			Condition: func(m MetricsSnapshot) *Alert {
				totalDetections := m.StateDetectionErrors
				for _, count := range m.StateDetectionAccuracy {
					totalDetections += count
				}
				if totalDetections > 10 {
					errorRate := float64(m.StateDetectionErrors) / float64(totalDetections) * 100
					if errorRate > da.StateErrorRateCritical {
						return &Alert{
							Level:     AlertLevelCritical,
							Timestamp: time.Now(),
							Message:   "State detection error rate critically high",
							Metric:    "state_detection_error_rate",
							Value:     errorRate,
							Threshold: da.StateErrorRateCritical,
						}
					}
					if errorRate > da.StateErrorRateWarning {
						return &Alert{
							Level:     AlertLevelWarning,
							Timestamp: time.Now(),
							Message:   "State detection error rate elevated",
							Metric:    "state_detection_error_rate",
							Value:     errorRate,
							Threshold: da.StateErrorRateWarning,
						}
					}
				}
				return nil
			},
		},
	}
}

// CheckAlerts evaluates all alert rules against current metrics
func CheckAlerts(metrics MetricsSnapshot, rules []AlertRule) []*Alert {
	var alerts []*Alert

	for _, rule := range rules {
		if alert := rule.Condition(metrics); alert != nil {
			alerts = append(alerts, alert)
		}
	}

	return alerts
}
