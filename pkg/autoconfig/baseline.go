// Package autoconfig implements a self-learning feedback loop that
// adjusts engram configuration based on session telemetry.
package autoconfig

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// DefaultWindowSize is the number of sessions kept in the rolling baseline.
const DefaultWindowSize = 10

// SessionSummary mirrors the output of the session summary hook.
type SessionSummary struct {
	SessionID       string             `json:"session_id"`
	Timestamp       time.Time          `json:"timestamp"`
	TotalCostUSD    float64            `json:"total_cost_usd"`
	TokenEfficiency float64            `json:"token_efficiency"`
	PhaseScores     map[string]float64 `json:"phase_scores"`
	SpanCount       int                `json:"span_count"`
	DurationMs      float64            `json:"duration_ms"`
	Anomalies       []string           `json:"anomalies,omitempty"`
}

// Baseline holds rolling statistics computed from recent sessions.
type Baseline struct {
	ProjectHash string           `json:"project_hash"`
	UpdatedAt   time.Time        `json:"updated_at"`
	WindowSize  int              `json:"window_size"`
	Sessions    []SessionSummary `json:"sessions"`

	// Computed aggregates.
	AvgCostUSD         float64            `json:"avg_cost_usd"`
	AvgTokenEfficiency float64            `json:"avg_token_efficiency"`
	AvgPhaseScores     map[string]float64 `json:"avg_phase_scores"`
	P50CostUSD         float64            `json:"p50_cost_usd"`
	P90CostUSD         float64            `json:"p90_cost_usd"`
	P50Efficiency      float64            `json:"p50_efficiency"`
	P90Efficiency      float64            `json:"p90_efficiency"`
}

// ProjectHash computes a stable hash for a project name.
func ProjectHash(projectName string) string {
	h := sha256.Sum256([]byte(projectName))
	return fmt.Sprintf("%x", h[:8])
}

// BaselinePath returns the filesystem path for a project's baseline.
func BaselinePath(projectHash string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".engram", "baselines", projectHash+".json"), nil
}

// LoadBaseline reads the baseline for a project hash. Returns an empty
// baseline if the file doesn't exist.
func LoadBaseline(projectHash string) (*Baseline, error) {
	path, err := BaselinePath(projectHash)
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return &Baseline{
			ProjectHash: projectHash,
			WindowSize:  DefaultWindowSize,
		}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load baseline: %w", err)
	}

	var b Baseline
	if err := json.Unmarshal(data, &b); err != nil {
		return nil, fmt.Errorf("parse baseline: %w", err)
	}
	return &b, nil
}

// AddSession appends a summary and recomputes aggregates. Trims the
// window to the configured size.
func (b *Baseline) AddSession(s SessionSummary) {
	b.Sessions = append(b.Sessions, s)

	// Trim to window size.
	if len(b.Sessions) > b.WindowSize {
		b.Sessions = b.Sessions[len(b.Sessions)-b.WindowSize:]
	}

	b.recompute()
}

// Save writes the baseline to disk.
func (b *Baseline) Save() error {
	path, err := BaselinePath(b.ProjectHash)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("save baseline: mkdir: %w", err)
	}

	data, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		return fmt.Errorf("save baseline: marshal: %w", err)
	}
	return os.WriteFile(path, data, 0o644)
}

func (b *Baseline) recompute() {
	n := len(b.Sessions)
	if n == 0 {
		return
	}

	b.UpdatedAt = time.Now()

	// Averages.
	var totalCost, totalEff float64
	phaseAccum := make(map[string]float64)
	phaseCounts := make(map[string]int)

	costs := make([]float64, n)
	effs := make([]float64, n)

	for i, s := range b.Sessions {
		totalCost += s.TotalCostUSD
		totalEff += s.TokenEfficiency
		costs[i] = s.TotalCostUSD
		effs[i] = s.TokenEfficiency
		for phase, score := range s.PhaseScores {
			phaseAccum[phase] += score
			phaseCounts[phase]++
		}
	}

	b.AvgCostUSD = totalCost / float64(n)
	b.AvgTokenEfficiency = totalEff / float64(n)

	b.AvgPhaseScores = make(map[string]float64, len(phaseAccum))
	for phase, sum := range phaseAccum {
		b.AvgPhaseScores[phase] = sum / float64(phaseCounts[phase])
	}

	// Percentiles.
	sort.Float64s(costs)
	sort.Float64s(effs)
	b.P50CostUSD = percentile(costs, 0.50)
	b.P90CostUSD = percentile(costs, 0.90)
	b.P50Efficiency = percentile(effs, 0.50)
	b.P90Efficiency = percentile(effs, 0.90)
}

func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := p * float64(len(sorted)-1)
	lower := int(math.Floor(idx))
	upper := int(math.Ceil(idx))
	if lower == upper || upper >= len(sorted) {
		return sorted[lower]
	}
	frac := idx - float64(lower)
	return sorted[lower]*(1-frac) + sorted[upper]*frac
}
