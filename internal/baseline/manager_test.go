package baseline

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/internal/benchmark"
	"github.com/vbonnet/dear-agent/internal/common"
)

func TestManager_SaveAndLoad(t *testing.T) {
	tmpDir := t.TempDir()
	baselinePath := filepath.Join(tmpDir, "baseline.json")

	manager := NewManager(baselinePath)

	// Create baseline
	baseline := NewBaseline()
	baseline.GitCommit = "abc123"
	baseline.GitBranch = "main"
	baseline.Scenarios["small"] = NewScenarioBaseline("small")
	baseline.Scenarios["small"].MedianMS = 15.5
	baseline.Scenarios["small"].Runs = 10

	// Save
	err := manager.Save(baseline)
	if err != nil {
		t.Fatalf("Save() failed: %v", err)
	}

	// Load
	loaded, err := manager.Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	// Verify
	if loaded.GitCommit != "abc123" {
		t.Errorf("git_commit = %q, want %q", loaded.GitCommit, "abc123")
	}

	if loaded.GitBranch != "main" {
		t.Errorf("git_branch = %q, want %q", loaded.GitBranch, "main")
	}

	if len(loaded.Scenarios) != 1 {
		t.Errorf("scenarios count = %d, want 1", len(loaded.Scenarios))
	}

	small, exists := loaded.Scenarios["small"]
	if !exists {
		t.Fatal("scenario 'small' not found")
	}

	if small.MedianMS != 15.5 {
		t.Errorf("median_ms = %.2f, want 15.5", small.MedianMS)
	}
}

func TestManager_Load_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	baselinePath := filepath.Join(tmpDir, "nonexistent.json")

	manager := NewManager(baselinePath)
	_, err := manager.Load()

	if !errors.Is(err, common.ErrBaselineNotFound) {
		t.Errorf("Load() error = %v, want %v", err, common.ErrBaselineNotFound)
	}
}

func TestManager_Load_InvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	baselinePath := filepath.Join(tmpDir, "invalid.json")

	// Write invalid JSON
	os.WriteFile(baselinePath, []byte("{ invalid json }"), 0644)

	manager := NewManager(baselinePath)
	_, err := manager.Load()

	if err == nil {
		t.Error("Load() should fail with invalid JSON")
	}
}

func TestManager_Save_InvalidBaseline(t *testing.T) {
	tmpDir := t.TempDir()
	baselinePath := filepath.Join(tmpDir, "baseline.json")

	manager := NewManager(baselinePath)

	// Create invalid baseline (missing schema version)
	baseline := &Baseline{
		SchemaVersion: "",
		Scenarios:     make(map[string]*ScenarioBaseline),
	}

	err := manager.Save(baseline)
	if err == nil {
		t.Error("Save() should fail with invalid baseline")
	}
}

func TestManager_Update_NewBaseline(t *testing.T) {
	// Skip if not in git repo
	if _, err := common.GetCurrentCommit(); err != nil {
		t.Skip("not in git repo, skipping test")
	}

	tmpDir := t.TempDir()
	baselinePath := filepath.Join(tmpDir, "baseline.json")

	manager := NewManager(baselinePath)

	// Create benchmark result
	result := &benchmark.BenchmarkResult{
		Command:    "echo test",
		Scenario:   "small",
		Runs:       10,
		WarmupRuns: 1,
		MedianMS:   12.5,
		MeanMS:     13.0,
		StddevMS:   1.2,
		P95MS:      15.0,
		P99MS:      16.0,
		CVPercent:  9.2,
	}

	// Update (should create new baseline)
	err := manager.Update("small", result, "initial baseline")
	if err != nil {
		t.Fatalf("Update() failed: %v", err)
	}

	// Verify baseline was created
	baseline, err := manager.Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	if len(baseline.Scenarios) != 1 {
		t.Errorf("scenarios count = %d, want 1", len(baseline.Scenarios))
	}

	small, exists := baseline.Scenarios["small"]
	if !exists {
		t.Fatal("scenario 'small' not found")
	}

	if small.MedianMS != 12.5 {
		t.Errorf("median_ms = %.2f, want 12.5", small.MedianMS)
	}

	if small.Runs != 10 {
		t.Errorf("runs = %d, want 10", small.Runs)
	}

	// History should be empty for initial baseline
	if len(small.History) != 0 {
		t.Errorf("history length = %d, want 0 for initial baseline", len(small.History))
	}
}

func TestManager_Update_ExistingBaseline(t *testing.T) {
	// Skip if not in git repo
	if _, err := common.GetCurrentCommit(); err != nil {
		t.Skip("not in git repo, skipping test")
	}

	tmpDir := t.TempDir()
	baselinePath := filepath.Join(tmpDir, "baseline.json")

	manager := NewManager(baselinePath)

	// Create initial baseline
	initialResult := &benchmark.BenchmarkResult{
		Command:    "echo test",
		Scenario:   "small",
		Runs:       10,
		WarmupRuns: 1,
		MedianMS:   10.0,
		MeanMS:     10.5,
		StddevMS:   1.0,
		P95MS:      12.0,
		P99MS:      13.0,
		CVPercent:  9.5,
	}

	err := manager.Update("small", initialResult, "initial baseline")
	if err != nil {
		t.Fatalf("Update() initial failed: %v", err)
	}

	// Wait to ensure timestamp differs
	time.Sleep(10 * time.Millisecond)

	// Update with new results
	updatedResult := &benchmark.BenchmarkResult{
		Command:    "echo test",
		Scenario:   "small",
		Runs:       10,
		WarmupRuns: 1,
		MedianMS:   15.0,
		MeanMS:     15.5,
		StddevMS:   1.5,
		P95MS:      18.0,
		P99MS:      19.0,
		CVPercent:  10.0,
	}

	err = manager.Update("small", updatedResult, "performance improvement")
	if err != nil {
		t.Fatalf("Update() second failed: %v", err)
	}

	// Verify baseline was updated
	baseline, err := manager.Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	small := baseline.Scenarios["small"]
	if small.MedianMS != 15.0 {
		t.Errorf("median_ms = %.2f, want 15.0 (updated value)", small.MedianMS)
	}

	// History should contain old value
	if len(small.History) != 1 {
		t.Fatalf("history length = %d, want 1", len(small.History))
	}

	historyEntry := small.History[0]
	if historyEntry.MedianMS != 10.0 {
		t.Errorf("history median_ms = %.2f, want 10.0 (old value)", historyEntry.MedianMS)
	}

	if historyEntry.Reason != "performance improvement" {
		t.Errorf("history reason = %q, want %q", historyEntry.Reason, "performance improvement")
	}
}

func TestManager_GetScenario(t *testing.T) {
	tmpDir := t.TempDir()
	baselinePath := filepath.Join(tmpDir, "baseline.json")

	manager := NewManager(baselinePath)

	// Create baseline
	baseline := NewBaseline()
	baseline.Scenarios["small"] = NewScenarioBaseline("small")
	baseline.Scenarios["small"].MedianMS = 15.5
	baseline.Scenarios["small"].Runs = 10

	if err := manager.Save(baseline); err != nil {
		t.Fatalf("Save() failed: %v", err)
	}

	// Get existing scenario
	scenario, err := manager.GetScenario("small")
	if err != nil {
		t.Fatalf("GetScenario() failed: %v", err)
	}

	if scenario.MedianMS != 15.5 {
		t.Errorf("median_ms = %.2f, want 15.5", scenario.MedianMS)
	}

	// Get non-existent scenario
	_, err = manager.GetScenario("nonexistent")
	if err == nil {
		t.Error("GetScenario() should fail for non-existent scenario")
	}
}

func TestManager_ListScenarios(t *testing.T) {
	tmpDir := t.TempDir()
	baselinePath := filepath.Join(tmpDir, "baseline.json")

	manager := NewManager(baselinePath)

	// Create baseline with multiple scenarios
	baseline := NewBaseline()
	baseline.Scenarios["small"] = NewScenarioBaseline("small")
	baseline.Scenarios["small"].Runs = 10
	baseline.Scenarios["medium"] = NewScenarioBaseline("medium")
	baseline.Scenarios["medium"].Runs = 10
	baseline.Scenarios["empty"] = NewScenarioBaseline("empty")
	baseline.Scenarios["empty"].Runs = 1

	if err := manager.Save(baseline); err != nil {
		t.Fatalf("Save() failed: %v", err)
	}

	// List scenarios
	scenarios, err := manager.ListScenarios()
	if err != nil {
		t.Fatalf("ListScenarios() failed: %v", err)
	}

	if len(scenarios) != 3 {
		t.Errorf("scenarios count = %d, want 3", len(scenarios))
	}

	// Verify all scenarios are present (order may vary)
	scenarioMap := make(map[string]bool)
	for _, s := range scenarios {
		scenarioMap[s] = true
	}

	expectedScenarios := []string{"small", "medium", "empty"}
	for _, expected := range expectedScenarios {
		if !scenarioMap[expected] {
			t.Errorf("scenario %q not found in list", expected)
		}
	}
}
