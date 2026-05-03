package ops

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCollectWorkflowMetrics_AllSuccess(t *testing.T) {
	result := &WorkflowResult{
		Name: "build-and-test",
		Tasks: []TaskResult{
			{ID: "lint", Status: TaskStatusSuccess, Duration: 100 * time.Millisecond},
			{ID: "build", Status: TaskStatusSuccess, Duration: 200 * time.Millisecond},
			{ID: "test", Status: TaskStatusSuccess, Duration: 300 * time.Millisecond},
		},
		Duration: 600 * time.Millisecond,
		Success:  true,
	}

	m := CollectWorkflowMetrics(result, 3)

	if m.TasksTotal != 3 {
		t.Errorf("TasksTotal = %d, want 3", m.TasksTotal)
	}
	if m.TasksCompleted != 3 {
		t.Errorf("TasksCompleted = %d, want 3", m.TasksCompleted)
	}
	if m.TasksFailed != 0 {
		t.Errorf("TasksFailed = %d, want 0", m.TasksFailed)
	}
	if m.TasksSkipped != 0 {
		t.Errorf("TasksSkipped = %d, want 0", m.TasksSkipped)
	}
	if m.DAGDepth != 3 {
		t.Errorf("DAGDepth = %d, want 3", m.DAGDepth)
	}
	if m.WorkflowName != "build-and-test" {
		t.Errorf("WorkflowName = %q, want %q", m.WorkflowName, "build-and-test")
	}
	if m.DurationSecs <= 0 {
		t.Errorf("DurationSecs = %f, want > 0", m.DurationSecs)
	}
	if m.RecordedAt == "" {
		t.Error("RecordedAt should not be empty")
	}
}

func TestCollectWorkflowMetrics_MixedResults(t *testing.T) {
	result := &WorkflowResult{
		Name: "partial-fail",
		Tasks: []TaskResult{
			{ID: "a", Status: TaskStatusSuccess},
			{ID: "b", Status: TaskStatusFailed},
			{ID: "c", Status: TaskStatusSkipped},
			{ID: "d", Status: TaskStatusSkipped},
		},
		Duration: time.Second,
		Success:  false,
	}

	m := CollectWorkflowMetrics(result, 2)

	if m.TasksTotal != 4 {
		t.Errorf("TasksTotal = %d, want 4", m.TasksTotal)
	}
	if m.TasksCompleted != 1 {
		t.Errorf("TasksCompleted = %d, want 1", m.TasksCompleted)
	}
	if m.TasksFailed != 1 {
		t.Errorf("TasksFailed = %d, want 1", m.TasksFailed)
	}
	if m.TasksSkipped != 2 {
		t.Errorf("TasksSkipped = %d, want 2", m.TasksSkipped)
	}
	if m.DAGDepth != 2 {
		t.Errorf("DAGDepth = %d, want 2", m.DAGDepth)
	}
}

func TestCollectWorkflowMetrics_EmptyWorkflow(t *testing.T) {
	result := &WorkflowResult{
		Name:     "empty",
		Tasks:    []TaskResult{},
		Duration: 0,
		Success:  true,
	}

	m := CollectWorkflowMetrics(result, 0)

	if m.TasksTotal != 0 {
		t.Errorf("TasksTotal = %d, want 0", m.TasksTotal)
	}
	if m.TasksCompleted != 0 {
		t.Errorf("TasksCompleted = %d, want 0", m.TasksCompleted)
	}
}

func TestRecordAndLoadWorkflowMetrics(t *testing.T) {
	// Set up temp HOME so metrics go to a temp dir
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	// Ensure no existing metrics
	loaded, err := LoadWorkflowMetrics()
	if err != nil {
		t.Fatalf("unexpected error loading empty metrics: %v", err)
	}
	if loaded != nil {
		t.Fatal("expected nil for nonexistent metrics file")
	}

	// Record metrics
	m := &WorkflowMetrics{
		TasksTotal:     5,
		TasksCompleted: 3,
		TasksFailed:    1,
		TasksSkipped:   1,
		DurationSecs:   2.5,
		DAGDepth:       3,
		WorkflowName:   "test-workflow",
		RecordedAt:     time.Now().Format(time.RFC3339),
	}

	if err := RecordWorkflowMetrics(m); err != nil {
		t.Fatalf("failed to record metrics: %v", err)
	}

	// Verify file exists
	metricsFile := filepath.Join(tmpDir, ".agm", "metrics", "workflow-last.json")
	if _, err := os.Stat(metricsFile); os.IsNotExist(err) {
		t.Fatal("metrics file was not created")
	}

	// Load and verify
	loaded, err = LoadWorkflowMetrics()
	if err != nil {
		t.Fatalf("failed to load metrics: %v", err)
	}
	if loaded == nil {
		t.Fatal("loaded metrics should not be nil")
	}
	if loaded.TasksTotal != 5 {
		t.Errorf("TasksTotal = %d, want 5", loaded.TasksTotal)
	}
	if loaded.TasksCompleted != 3 {
		t.Errorf("TasksCompleted = %d, want 3", loaded.TasksCompleted)
	}
	if loaded.TasksFailed != 1 {
		t.Errorf("TasksFailed = %d, want 1", loaded.TasksFailed)
	}
	if loaded.TasksSkipped != 1 {
		t.Errorf("TasksSkipped = %d, want 1", loaded.TasksSkipped)
	}
	if loaded.DurationSecs != 2.5 {
		t.Errorf("DurationSecs = %f, want 2.5", loaded.DurationSecs)
	}
	if loaded.DAGDepth != 3 {
		t.Errorf("DAGDepth = %d, want 3", loaded.DAGDepth)
	}
	if loaded.WorkflowName != "test-workflow" {
		t.Errorf("WorkflowName = %q, want %q", loaded.WorkflowName, "test-workflow")
	}
}

func TestLoadWorkflowMetrics_InvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	os.Setenv("HOME", tmpDir)

	metricsDir := filepath.Join(tmpDir, ".agm", "metrics")
	os.MkdirAll(metricsDir, 0o755)
	os.WriteFile(filepath.Join(metricsDir, "workflow-last.json"), []byte("{invalid"), 0o644)

	_, err := LoadWorkflowMetrics()
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}
