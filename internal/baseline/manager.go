package baseline

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/vbonnet/dear-agent/internal/benchmark"
	"github.com/vbonnet/dear-agent/internal/common"
)

// Manager handles loading, saving, and updating baseline files
type Manager struct {
	FilePath string
}

// NewManager creates a new baseline manager for the specified file path
func NewManager(filePath string) *Manager {
	return &Manager{
		FilePath: filePath,
	}
}

// Load reads and validates the baseline file
func (m *Manager) Load() (*Baseline, error) {
	data, err := os.ReadFile(m.FilePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, common.ErrBaselineNotFound
		}
		return nil, fmt.Errorf("failed to read baseline file: %w", err)
	}

	var baseline Baseline
	if err := json.Unmarshal(data, &baseline); err != nil {
		return nil, fmt.Errorf("failed to parse baseline JSON: %w", err)
	}

	if err := ValidateSchema(&baseline); err != nil {
		return nil, fmt.Errorf("invalid baseline schema: %w", err)
	}

	return &baseline, nil
}

// Save writes the baseline to the file with pretty-printed JSON
func (m *Manager) Save(b *Baseline) error {
	if err := ValidateSchema(b); err != nil {
		return fmt.Errorf("invalid baseline: %w", err)
	}

	data, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal baseline: %w", err)
	}

	if err := os.WriteFile(m.FilePath, data, 0600); err != nil {
		return fmt.Errorf("failed to write baseline file: %w", err)
	}

	return nil
}

// Update updates the baseline for a specific scenario with new benchmark results
func (m *Manager) Update(scenario string, result *benchmark.BenchmarkResult, reason string) error {
	// Load existing baseline or create new one
	baseline, err := m.Load()
	if err != nil {
		if errors.Is(err, common.ErrBaselineNotFound) {
			baseline = NewBaseline()
		} else {
			return fmt.Errorf("failed to load baseline: %w", err)
		}
	}

	// Get git metadata
	gitCommit, err := common.GetCurrentCommit()
	if err != nil {
		return fmt.Errorf("failed to get git commit: %w", err)
	}

	gitBranch, err := common.GetCurrentBranch()
	if err != nil {
		return fmt.Errorf("failed to get git branch: %w", err)
	}

	// Update baseline metadata
	baseline.UpdatedAt = time.Now()
	baseline.GitCommit = gitCommit
	baseline.GitBranch = gitBranch

	// Get or create scenario baseline
	scenarioBaseline, exists := baseline.Scenarios[scenario]
	if !exists {
		scenarioBaseline = NewScenarioBaseline(scenario)
		baseline.Scenarios[scenario] = scenarioBaseline
	}

	// Add history entry with old values (if updating existing)
	if exists && scenarioBaseline.MedianMS > 0 {
		historyEntry := HistoryEntry{
			Timestamp: baseline.UpdatedAt,
			GitCommit: gitCommit,
			GitBranch: gitBranch,
			MedianMS:  scenarioBaseline.MedianMS,
			Reason:    reason,
		}
		scenarioBaseline.History = append(scenarioBaseline.History, historyEntry)
	}

	// Update scenario with new benchmark results
	scenarioBaseline.Scenario = scenario
	scenarioBaseline.MedianMS = result.MedianMS
	scenarioBaseline.MeanMS = result.MeanMS
	scenarioBaseline.StddevMS = result.StddevMS
	scenarioBaseline.P95MS = result.P95MS
	scenarioBaseline.P99MS = result.P99MS
	scenarioBaseline.CVPercent = result.CVPercent
	scenarioBaseline.Runs = result.Runs

	// Save updated baseline
	if err := m.Save(baseline); err != nil {
		return fmt.Errorf("failed to save baseline: %w", err)
	}

	return nil
}

// GetScenario retrieves the baseline for a specific scenario
func (m *Manager) GetScenario(scenario string) (*ScenarioBaseline, error) {
	baseline, err := m.Load()
	if err != nil {
		return nil, err
	}

	scenarioBaseline, exists := baseline.Scenarios[scenario]
	if !exists {
		return nil, fmt.Errorf("scenario %q not found in baseline", scenario)
	}

	return scenarioBaseline, nil
}

// ListScenarios returns all scenario names in the baseline
func (m *Manager) ListScenarios() ([]string, error) {
	baseline, err := m.Load()
	if err != nil {
		return nil, err
	}

	scenarios := make([]string, 0, len(baseline.Scenarios))
	for name := range baseline.Scenarios {
		scenarios = append(scenarios, name)
	}

	return scenarios, nil
}
