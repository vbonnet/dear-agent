package budget

import (
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/manifest"
)

// Tracker computes context budget status for sessions.
type Tracker struct {
	configs map[string]Config // sessionID -> Config
}

// NewTracker creates a new budget tracker.
func NewTracker() *Tracker {
	return &Tracker{
		configs: make(map[string]Config),
	}
}

// SetConfig sets the budget config for a specific session.
func (t *Tracker) SetConfig(sessionID string, cfg Config) {
	t.configs[sessionID] = cfg
}

// getConfig returns the config for a session, using defaults if not explicitly set.
func (t *Tracker) getConfig(sessionID string, role Role) Config {
	if cfg, ok := t.configs[sessionID]; ok {
		return cfg
	}
	return DefaultConfig(role)
}

// Check computes the budget status for a session based on its manifest.
func (t *Tracker) Check(m *manifest.Manifest) Status {
	role := inferRole(m)
	cfg := t.getConfig(m.SessionID, role)

	status := Status{
		SessionID: m.SessionID,
		Role:      role,
		Threshold: cfg.ThresholdPercent,
		CheckedAt: time.Now(),
	}

	if m.ContextUsage != nil {
		status.UsedTokens = m.ContextUsage.UsedTokens
		status.TotalTokens = m.ContextUsage.TotalTokens
		status.PercentageUsed = m.ContextUsage.PercentageUsed
	}

	status.Level = classify(status.PercentageUsed, cfg)

	return status
}

// CheckBatch computes budget status for multiple sessions.
func (t *Tracker) CheckBatch(manifests []*manifest.Manifest) []Status {
	results := make([]Status, 0, len(manifests))
	for _, m := range manifests {
		results = append(results, t.Check(m))
	}
	return results
}

// classify determines the budget level based on usage and config.
func classify(percentUsed float64, cfg Config) Level {
	switch {
	case percentUsed >= cfg.CriticalPercent:
		return LevelCritical
	case percentUsed >= cfg.ThresholdPercent:
		return LevelWarning
	default:
		return LevelOK
	}
}

// inferRole determines the session role from manifest metadata.
// Uses tags, name patterns, and parent session presence to infer role.
func inferRole(m *manifest.Manifest) Role {
	// Check tags first
	for _, tag := range m.Context.Tags {
		if r := RoleFromString(tag); r != RoleDefault {
			return r
		}
	}

	// Sessions with parent are workers
	if m.ParentSessionID != nil {
		return RoleWorker
	}

	// Sessions with monitors are orchestrators
	if len(m.Monitors) > 0 {
		return RoleOrchestrator
	}

	return RoleDefault
}
