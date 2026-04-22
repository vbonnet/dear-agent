package autoconfig

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Proposal represents a single configuration change proposed by the
// automated retrospective.
type Proposal struct {
	Key       string  `json:"key"`
	OldValue  string  `json:"old_value,omitempty"`
	NewValue  string  `json:"new_value"`
	Reason    string  `json:"reason"`
	Magnitude float64 `json:"magnitude"` // 0-1, size of change
}

// Retrospective is the structured output of the retrospective hook.
type Retrospective struct {
	SessionID   string       `json:"session_id"`
	Timestamp   time.Time    `json:"timestamp"`
	Regressions []Regression `json:"regressions,omitempty"`
	Proposals   []Proposal   `json:"proposals,omitempty"`
}

// Regression captures a detected quality/cost/efficiency regression.
type Regression struct {
	Metric   string  `json:"metric"`
	Baseline float64 `json:"baseline"`
	Current  float64 `json:"current"`
	DeltaPct float64 `json:"delta_pct"`
	Severity string  `json:"severity"` // "warning" or "critical"
}

// ModificationLog records applied config changes for audit.
type ModificationLog struct {
	Timestamp time.Time  `json:"timestamp"`
	SessionID string     `json:"session_id"`
	Proposals []Proposal `json:"proposals"`
	Applied   bool       `json:"applied"`
	Reason    string     `json:"reason,omitempty"`
}

// Bounds defines the valid range for self-modification parameters.
type Bounds struct {
	MaxMagnitude float64 `json:"max_magnitude"` // max single-change size
	MaxChanges   int     `json:"max_changes"`   // max changes per session
}

// DefaultBounds returns conservative self-modification bounds.
func DefaultBounds() Bounds {
	return Bounds{
		MaxMagnitude: 0.3,
		MaxChanges:   5,
	}
}

// Engine reads retrospective proposals, validates them within bounds,
// and writes approved changes to the auto-config file.
type Engine struct {
	bounds  Bounds
	project string
}

// NewEngine creates a new self-modification engine.
func NewEngine(project string, bounds Bounds) *Engine {
	return &Engine{
		bounds:  bounds,
		project: project,
	}
}

// AutoConfigPath returns the path for auto-generated config.
func AutoConfigPath(project string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".engram", "auto-config", project+".yaml"), nil
}

// ModificationsLogPath returns the path for the modifications log.
func ModificationsLogPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".engram", "traces", "modifications.jsonl"), nil
}

// Apply validates and applies proposals from a retrospective. Returns
// the list of actually applied proposals.
func (e *Engine) Apply(retro Retrospective) ([]Proposal, error) {
	var applied []Proposal

	for _, p := range retro.Proposals {
		if !e.validate(p) {
			continue
		}
		if len(applied) >= e.bounds.MaxChanges {
			break
		}
		applied = append(applied, p)
	}

	if len(applied) == 0 {
		return nil, nil
	}

	// Write auto-config.
	if err := e.writeConfig(applied); err != nil {
		return nil, fmt.Errorf("engine: write config: %w", err)
	}

	// Log modification.
	if err := e.logModification(retro.SessionID, applied, true); err != nil {
		return applied, fmt.Errorf("engine: log: %w", err)
	}

	return applied, nil
}

func (e *Engine) validate(p Proposal) bool {
	return p.Magnitude >= 0 && p.Magnitude <= e.bounds.MaxMagnitude
}

func (e *Engine) writeConfig(proposals []Proposal) error {
	path, err := AutoConfigPath(e.project)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	// Build a simple YAML from proposals.
	cfg := make(map[string]string, len(proposals))
	for _, p := range proposals {
		cfg[p.Key] = p.NewValue
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}

	// Write as YAML-compatible JSON (valid YAML subset).
	return os.WriteFile(path, data, 0o644)
}

func (e *Engine) logModification(sessionID string, proposals []Proposal, applied bool) error {
	logPath, err := ModificationsLogPath()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		return err
	}

	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	entry := ModificationLog{
		Timestamp: time.Now(),
		SessionID: sessionID,
		Proposals: proposals,
		Applied:   applied,
	}

	return json.NewEncoder(f).Encode(entry)
}
