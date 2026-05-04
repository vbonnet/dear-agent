package eventbus

import (
	"log/slog"
	"sync"
	"sync/atomic"

	"github.com/vbonnet/dear-agent/agm/internal/logging"
)

// StallMetricsSink collects metrics from stall-related events.
// Implements both Sink (for pull-based processing) and Broadcaster (for inline use).
type StallMetricsSink struct {
	detected  atomic.Int64
	recovered atomic.Int64
	escalated atomic.Int64

	// Per-type counters
	mu       sync.RWMutex
	byType   map[string]int64 // stall_type -> count
	bySess   map[string]int64 // session -> count

	logger *slog.Logger
}

// NewStallMetricsSink creates a new metrics sink for stall events.
func NewStallMetricsSink() *StallMetricsSink {
	return &StallMetricsSink{
		byType: make(map[string]int64),
		bySess: make(map[string]int64),
		logger: logging.DefaultLogger(),
	}
}

// HandleEvent processes a stall event and updates metrics.
func (s *StallMetricsSink) HandleEvent(event *Event) error {
	//nolint:exhaustive // intentional partial: handles the relevant subset
	switch event.Type {
	case EventStallDetected:
		s.detected.Add(1)
		var payload StallDetectedPayload
		if err := event.ParsePayload(&payload); err == nil {
			s.mu.Lock()
			s.byType[payload.StallType]++
			s.bySess[payload.Session]++
			s.mu.Unlock()
		}
		s.logger.Info("stall detected",
			"session", event.SessionID,
			"total_detected", s.detected.Load())

	case EventStallRecovered:
		s.recovered.Add(1)
		s.logger.Info("stall recovered",
			"session", event.SessionID,
			"total_recovered", s.recovered.Load())

	case EventStallEscalated:
		s.escalated.Add(1)
		s.logger.Warn("stall escalated",
			"session", event.SessionID,
			"total_escalated", s.escalated.Load())
	}
	return nil
}

// Close is a no-op for the metrics sink.
func (s *StallMetricsSink) Close() error {
	return nil
}

// Snapshot returns current metrics values.
func (s *StallMetricsSink) Snapshot() StallMetrics {
	s.mu.RLock()
	defer s.mu.RUnlock()

	byType := make(map[string]int64, len(s.byType))
	for k, v := range s.byType {
		byType[k] = v
	}
	bySess := make(map[string]int64, len(s.bySess))
	for k, v := range s.bySess {
		bySess[k] = v
	}

	return StallMetrics{
		Detected:    s.detected.Load(),
		Recovered:   s.recovered.Load(),
		Escalated:   s.escalated.Load(),
		ByStallType: byType,
		BySession:   bySess,
	}
}

// StallMetrics holds a point-in-time snapshot of stall metrics.
type StallMetrics struct {
	Detected    int64            `json:"detected"`
	Recovered   int64            `json:"recovered"`
	Escalated   int64            `json:"escalated"`
	ByStallType map[string]int64 `json:"by_stall_type"`
	BySession   map[string]int64 `json:"by_session"`
}
