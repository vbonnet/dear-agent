package ops

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// WorkflowMetrics holds observability data for workflow executions.
type WorkflowMetrics struct {
	TasksTotal    int     `json:"tasks_total"`
	TasksCompleted int    `json:"tasks_completed"`
	TasksFailed   int     `json:"tasks_failed"`
	TasksSkipped  int     `json:"tasks_skipped"`
	DurationSecs  float64 `json:"execution_duration_seconds"`
	DAGDepth      int     `json:"dag_depth"`
	WorkflowName  string  `json:"workflow_name"`
	RecordedAt    string  `json:"recorded_at"`
}

// CollectWorkflowMetrics extracts metrics from a WorkflowResult and its definition.
func CollectWorkflowMetrics(result *WorkflowResult, dagDepth int) *WorkflowMetrics {
	m := &WorkflowMetrics{
		TasksTotal:   len(result.Tasks),
		DurationSecs: result.Duration.Seconds(),
		DAGDepth:     dagDepth,
		WorkflowName: result.Name,
		RecordedAt:   time.Now().Format(time.RFC3339),
	}

	for _, t := range result.Tasks {
		switch t.Status {
		case TaskStatusSuccess:
			m.TasksCompleted++
		case TaskStatusFailed:
			m.TasksFailed++
		case TaskStatusSkipped:
			m.TasksSkipped++
		}
	}

	return m
}

// workflowMetricsPath returns the path for the workflow metrics file.
func workflowMetricsPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".agm", "metrics", "workflow-last.json")
}

// RecordWorkflowMetrics saves workflow metrics to disk.
func RecordWorkflowMetrics(m *WorkflowMetrics) error {
	path := workflowMetricsPath()
	if path == "" {
		return fmt.Errorf("cannot determine metrics path")
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create metrics dir: %w", err)
	}

	data, err := json.Marshal(m)
	if err != nil {
		return fmt.Errorf("marshal workflow metrics: %w", err)
	}

	return os.WriteFile(path, data, 0o600)
}

// LoadWorkflowMetrics loads the most recent workflow metrics from disk.
// Returns nil (no error) if no metrics file exists yet.
func LoadWorkflowMetrics() (*WorkflowMetrics, error) {
	path := workflowMetricsPath()
	if path == "" {
		return nil, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read workflow metrics: %w", err)
	}

	var m WorkflowMetrics
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse workflow metrics: %w", err)
	}
	return &m, nil
}
