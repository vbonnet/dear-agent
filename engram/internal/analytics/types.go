package analytics

import "time"

// Session represents a complete Wayfinder session
type Session struct {
	ID          string         `json:"session_id"`
	ProjectPath string         `json:"project_path"`
	StartTime   time.Time      `json:"start_time"`
	EndTime     time.Time      `json:"end_time"`
	Status      string         `json:"status"` // completed, incomplete, failed
	Phases      []Phase        `json:"phases"`
	Metrics     SessionMetrics `json:"metrics"`
}

// Phase represents a single Wayfinder phase (D1-D4, S5-S11)
type Phase struct {
	Name      string                 `json:"name"` // D1, D2, ... S11
	StartTime time.Time              `json:"start_time"`
	EndTime   time.Time              `json:"end_time"`
	Duration  time.Duration          `json:"duration_ms"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
	Metrics   PhaseMetrics           `json:"metrics,omitempty"` // S8 Group 1: Phase-level metrics
}

// PhaseMetrics contains phase-level telemetry (S8 Group 1)
type PhaseMetrics struct {
	Outcome          string `json:"outcome"`            // success, partial, skipped
	CompletionTimeMs int64  `json:"completion_time_ms"` // Time to complete phase
	ErrorCount       int    `json:"error_count"`        // Errors during phase
	ReworkCount      int    `json:"rework_count"`       // Rework iterations
}

// SessionMetrics contains aggregated session statistics
type SessionMetrics struct {
	TotalDuration time.Duration `json:"total_duration_ms"`
	AITime        time.Duration `json:"ai_time_ms"`   // Active work time
	WaitTime      time.Duration `json:"wait_time_ms"` // User wait time
	PhaseCount    int           `json:"phase_count"`

	// Cost calculation (Sonnet 4.5 pricing)
	InputTokens   int64   `json:"input_tokens,omitempty"`
	OutputTokens  int64   `json:"output_tokens,omitempty"`
	EstimatedCost float64 `json:"estimated_cost_usd,omitempty"`

	// S8 Group 1: Wayfinder ROI metrics
	QualityScore float64  `json:"quality_score,omitempty"` // Formula: 1.0 - (rework*0.2) - (error*0.1)
	ErrorCount   int      `json:"error_count"`             // Total errors across all phases
	ReworkPhases []string `json:"rework_phases,omitempty"` // Phases that required rework
}

// SessionSummary contains aggregate statistics across multiple sessions
type SessionSummary struct {
	TotalSessions     int `json:"total_sessions"`
	CompletedSessions int `json:"completed_sessions"`
	FailedSessions    int `json:"failed_sessions"`

	TotalDuration   time.Duration `json:"total_duration_ms"`
	AverageDuration time.Duration `json:"average_duration_ms"`

	TotalAITime   time.Duration `json:"total_ai_time_ms"`
	TotalWaitTime time.Duration `json:"total_wait_time_ms"`

	TotalCost   float64 `json:"total_cost_usd"`
	AverageCost float64 `json:"average_cost_usd"`
}

// ParsedEvent represents a Wayfinder event from telemetry
type ParsedEvent struct {
	Type       string                 `json:"type"`
	Timestamp  time.Time              `json:"timestamp"`
	SessionID  string                 `json:"session_id"`
	Phase      string                 `json:"phase,omitempty"`
	EventTopic string                 `json:"event_topic"`
	Data       map[string]interface{} `json:"data"`
}
