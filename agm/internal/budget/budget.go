// Package budget provides context budget tracking with thresholds and alerts
// for AGM sessions. It monitors context window usage per session role and
// generates warnings when usage exceeds configured thresholds.
package budget

import (
	"fmt"
	"time"
)

// Role represents a session role with its own budget threshold.
type Role string

// Recognized session role values for budget tracking.
const (
	RoleOrchestrator Role = "orchestrator"
	RoleWorker       Role = "worker"
	RolePlanner      Role = "planner"
	RoleMonitor      Role = "monitor"
	RoleDefault      Role = "default"
)

// DefaultThresholds maps each role to its warning threshold percentage (0-100).
var DefaultThresholds = map[Role]float64{
	RoleOrchestrator: 65.0,
	RoleWorker:       75.0,
	RolePlanner:      70.0,
	RoleMonitor:      80.0,
	RoleDefault:      75.0,
}

// Level indicates the severity of context budget usage.
type Level string

// Budget usage severity levels.
const (
	LevelOK       Level = "OK"       // Under threshold
	LevelWarning  Level = "WARNING"  // At or above threshold
	LevelCritical Level = "CRITICAL" // Above 90%
)

// Config holds budget configuration for a session.
type Config struct {
	Role              Role    // Session role
	ThresholdPercent  float64 // Warning threshold (0-100)
	CriticalPercent   float64 // Critical threshold (0-100)
	AutoCompactSignal bool    // Whether to write signal file for auto-compact
}

// DefaultConfig returns a Config with defaults for the given role.
func DefaultConfig(role Role) Config {
	threshold, ok := DefaultThresholds[role]
	if !ok {
		threshold = DefaultThresholds[RoleDefault]
	}
	return Config{
		Role:              role,
		ThresholdPercent:  threshold,
		CriticalPercent:   90.0,
		AutoCompactSignal: true,
	}
}

// Status represents the current context budget status for a session.
type Status struct {
	SessionID      string    // AGM session ID
	Role           Role      // Session role
	UsedTokens     int       // Currently used tokens
	TotalTokens    int       // Total available tokens
	PercentageUsed float64   // Usage percentage (0-100)
	Threshold      float64   // Warning threshold for this role
	Level          Level     // Current budget level
	CheckedAt      time.Time // When this status was computed
}

// String returns a human-readable summary of the budget status.
func (s Status) String() string {
	return fmt.Sprintf("budget: %.1f%% used (threshold: %.0f%%, level: %s)",
		s.PercentageUsed, s.Threshold, s.Level)
}

// IsOverThreshold returns true if usage exceeds the warning threshold.
func (s Status) IsOverThreshold() bool {
	return s.Level == LevelWarning || s.Level == LevelCritical
}

// RoleFromString converts a string to a Role, returning RoleDefault for unknown values.
func RoleFromString(s string) Role {
	//nolint:exhaustive // intentional partial: handles the relevant subset
	switch Role(s) {
	case RoleOrchestrator, RoleWorker, RolePlanner, RoleMonitor:
		return Role(s)
	default:
		return RoleDefault
	}
}
