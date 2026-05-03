package autoconfig

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Rollback thresholds.
const (
	QualityScoreMin     = 0.70
	CostIncreaseMaxPct  = 30.0
	MaxConsecutiveFails = 3
)

// RollbackState tracks post-modification monitoring.
type RollbackState struct {
	ModificationTime  time.Time  `json:"modification_time"`
	SessionID         string     `json:"session_id"`
	Proposals         []Proposal `json:"proposals"`
	MonitoredSessions int        `json:"monitored_sessions"`
	ConsecutiveFails  int        `json:"consecutive_fails"`
	Suspended         bool       `json:"suspended"`
	Reverted          bool       `json:"reverted"`
	RevertReason      string     `json:"revert_reason,omitempty"`
}

// RollbackController monitors sessions after a modification and reverts
// if quality degrades beyond thresholds.
type RollbackController struct {
	project   string
	monitorN  int // sessions to monitor after modification
	state     *RollbackState
	statePath string
}

// NewRollbackController creates a controller for the given project.
// monitorN is the number of sessions to watch after a modification.
func NewRollbackController(project string, monitorN int) (*RollbackController, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	statePath := filepath.Join(home, ".engram", "auto-config", project+".rollback.json")

	rc := &RollbackController{
		project:   project,
		monitorN:  monitorN,
		statePath: statePath,
	}

	rc.state, _ = rc.loadState()
	if rc.state == nil {
		rc.state = &RollbackState{}
	}

	return rc, nil
}

// RecordModification sets the controller to monitor for the given modification.
func (rc *RollbackController) RecordModification(sessionID string, proposals []Proposal) error {
	rc.state = &RollbackState{
		ModificationTime: time.Now(),
		SessionID:        sessionID,
		Proposals:        proposals,
	}
	return rc.saveState()
}

// CheckSession evaluates a new session summary against thresholds.
// Returns (shouldRevert, reason).
func (rc *RollbackController) CheckSession(summary SessionSummary, baseline *Baseline) (bool, string) {
	if rc.state == nil || rc.state.Reverted || rc.state.Suspended {
		return false, ""
	}

	if rc.state.MonitoredSessions >= rc.monitorN {
		// Monitoring window complete, no issues.
		return false, ""
	}

	rc.state.MonitoredSessions++

	failed := false
	var reason string

	// Check quality score (average of phase scores).
	if len(summary.PhaseScores) > 0 {
		var total float64
		for _, v := range summary.PhaseScores {
			total += v
		}
		avg := total / float64(len(summary.PhaseScores))
		if avg < QualityScoreMin {
			failed = true
			reason = fmt.Sprintf("quality score %.2f below threshold %.2f", avg, QualityScoreMin)
		}
	}

	// Check cost increase vs baseline.
	if baseline != nil && baseline.AvgCostUSD > 0 && summary.TotalCostUSD > 0 {
		increase := ((summary.TotalCostUSD - baseline.AvgCostUSD) / baseline.AvgCostUSD) * 100
		if increase > CostIncreaseMaxPct {
			failed = true
			reason = fmt.Sprintf("cost increase %.1f%% exceeds max %.1f%%", increase, CostIncreaseMaxPct)
		}
	}

	if failed {
		rc.state.ConsecutiveFails++
	} else {
		rc.state.ConsecutiveFails = 0
	}

	_ = rc.saveState()

	if rc.state.ConsecutiveFails >= MaxConsecutiveFails {
		rc.state.Suspended = true
		rc.state.RevertReason = fmt.Sprintf("suspended after %d consecutive failures: %s", MaxConsecutiveFails, reason)
		_ = rc.saveState()
		return true, rc.state.RevertReason
	}

	return false, ""
}

// Revert removes the auto-config file and marks the state as reverted.
func (rc *RollbackController) Revert(reason string) error {
	path, err := AutoConfigPath(rc.project)
	if err != nil {
		return err
	}

	// Remove auto-config.
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("rollback: remove config: %w", err)
	}

	rc.state.Reverted = true
	rc.state.RevertReason = reason
	return rc.saveState()
}

// IsSuspended returns true if the controller has been suspended.
func (rc *RollbackController) IsSuspended() bool {
	return rc.state != nil && rc.state.Suspended
}

// State returns the current rollback state (for inspection/testing).
func (rc *RollbackController) State() *RollbackState {
	return rc.state
}

func (rc *RollbackController) loadState() (*RollbackState, error) {
	data, err := os.ReadFile(rc.statePath)
	if err != nil {
		return nil, err
	}
	var s RollbackState
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

func (rc *RollbackController) saveState() error {
	if err := os.MkdirAll(filepath.Dir(rc.statePath), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(rc.state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(rc.statePath, data, 0o600)
}
