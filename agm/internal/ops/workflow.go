package ops

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

// WorkflowDefinition represents a YAML workflow file.
type WorkflowDefinition struct {
	Name  string           `yaml:"name" json:"name"`
	Tasks []TaskDefinition `yaml:"tasks" json:"tasks"`
}

// TaskDefinition represents a single task in a workflow.
type TaskDefinition struct {
	ID        string   `yaml:"id" json:"id"`
	Command   string   `yaml:"command" json:"command"`
	DependsOn []string `yaml:"depends_on,omitempty" json:"depends_on,omitempty"`
}

// TaskStatus represents the execution state of a task.
type TaskStatus string

// Task execution state values.
const (
	TaskStatusPending TaskStatus = "pending"
	TaskStatusRunning TaskStatus = "running"
	TaskStatusSuccess TaskStatus = "success"
	TaskStatusFailed  TaskStatus = "failed"
	TaskStatusSkipped TaskStatus = "skipped"
)

// TaskResult holds the result of executing a single task.
type TaskResult struct {
	ID        string        `json:"id"`
	Status    TaskStatus    `json:"status"`
	Output    string        `json:"output,omitempty"`
	Error     string        `json:"error,omitempty"`
	Duration  time.Duration `json:"duration"`
	StartedAt time.Time     `json:"started_at,omitempty"`
}

// WorkflowResult holds the result of executing a workflow.
type WorkflowResult struct {
	Operation string        `json:"operation"`
	Name      string        `json:"name"`
	Success   bool          `json:"success"`
	Tasks     []TaskResult  `json:"tasks"`
	Duration  time.Duration `json:"duration"`
}

// ParseWorkflow parses a YAML workflow definition.
func ParseWorkflow(data []byte) (*WorkflowDefinition, error) {
	var w WorkflowDefinition
	if err := yaml.Unmarshal(data, &w); err != nil {
		return nil, fmt.Errorf("invalid workflow YAML: %w", err)
	}
	if w.Name == "" {
		return nil, fmt.Errorf("workflow name is required")
	}
	if len(w.Tasks) == 0 {
		return nil, fmt.Errorf("workflow must have at least one task")
	}
	// Validate task IDs are unique and non-empty
	seen := make(map[string]bool, len(w.Tasks))
	for _, t := range w.Tasks {
		if t.ID == "" {
			return nil, fmt.Errorf("task ID is required")
		}
		if t.Command == "" {
			return nil, fmt.Errorf("task %q: command is required", t.ID)
		}
		if seen[t.ID] {
			return nil, fmt.Errorf("duplicate task ID: %q", t.ID)
		}
		seen[t.ID] = true
	}
	// Validate dependency references
	for _, t := range w.Tasks {
		for _, dep := range t.DependsOn {
			if !seen[dep] {
				return nil, fmt.Errorf("task %q depends on unknown task %q", t.ID, dep)
			}
			if dep == t.ID {
				return nil, fmt.Errorf("task %q cannot depend on itself", t.ID)
			}
		}
	}
	return &w, nil
}

// LoadWorkflow loads a workflow YAML file from disk.
func LoadWorkflow(path string) (*WorkflowDefinition, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read workflow file: %w", err)
	}
	return ParseWorkflow(data)
}

// TopologicalSort returns task IDs in dependency order.
// Returns an error if the graph contains a cycle.
func TopologicalSort(w *WorkflowDefinition) ([][]string, error) {
	// Build adjacency and in-degree maps
	inDegree := make(map[string]int, len(w.Tasks))
	dependents := make(map[string][]string, len(w.Tasks))
	for _, t := range w.Tasks {
		inDegree[t.ID] = len(t.DependsOn)
		for _, dep := range t.DependsOn {
			dependents[dep] = append(dependents[dep], t.ID)
		}
	}

	// Kahn's algorithm: collect tasks in topological layers
	// Each layer contains tasks that can run in parallel.
	var layers [][]string
	var queue []string

	// Start with tasks that have no dependencies
	for _, t := range w.Tasks {
		if inDegree[t.ID] == 0 {
			queue = append(queue, t.ID)
		}
	}

	processed := 0
	for len(queue) > 0 {
		// All tasks in queue can run in parallel (same layer)
		layer := make([]string, len(queue))
		copy(layer, queue)
		layers = append(layers, layer)

		var nextQueue []string
		for _, id := range queue {
			processed++
			for _, dep := range dependents[id] {
				inDegree[dep]--
				if inDegree[dep] == 0 {
					nextQueue = append(nextQueue, dep)
				}
			}
		}
		queue = nextQueue
	}

	if processed != len(w.Tasks) {
		return nil, fmt.Errorf("workflow contains a dependency cycle")
	}

	return layers, nil
}

// CommandRunner executes a shell command. Abstracted for testing.
type CommandRunner func(ctx context.Context, command string) (string, error)

// DefaultCommandRunner executes commands via /bin/sh.
func DefaultCommandRunner(ctx context.Context, command string) (string, error) {
	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", command)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// ExecuteWorkflow runs a workflow, respecting task dependencies.
// Independent tasks within the same topological layer run in parallel.
func ExecuteWorkflow(ctx context.Context, w *WorkflowDefinition, runner CommandRunner) (*WorkflowResult, error) {
	start := time.Now()

	layers, err := TopologicalSort(w)
	if err != nil {
		return nil, err
	}

	// Build task lookup
	taskDefs := make(map[string]*TaskDefinition, len(w.Tasks))
	for i := range w.Tasks {
		taskDefs[w.Tasks[i].ID] = &w.Tasks[i]
	}

	results := make(map[string]*TaskResult, len(w.Tasks))
	allSuccess := true

	for _, layer := range layers {
		// Check if any dependency has failed or was skipped — propagate skip
		var toRun []string
		for _, id := range layer {
			skip := false
			for _, dep := range taskDefs[id].DependsOn {
				if r, ok := results[dep]; ok && (r.Status == TaskStatusFailed || r.Status == TaskStatusSkipped) {
					skip = true
					break
				}
			}
			if skip {
				results[id] = &TaskResult{
					ID:     id,
					Status: TaskStatusSkipped,
				}
				allSuccess = false
			} else {
				toRun = append(toRun, id)
			}
		}

		if len(toRun) == 0 {
			continue
		}

		// Run tasks in this layer in parallel
		var wg sync.WaitGroup
		var mu sync.Mutex
		wg.Add(len(toRun))

		for _, id := range toRun {
			go func(taskID string) {
				defer wg.Done()
				taskStart := time.Now()

				output, runErr := runner(ctx, taskDefs[taskID].Command)
				elapsed := time.Since(taskStart)

				r := &TaskResult{
					ID:        taskID,
					Output:    output,
					Duration:  elapsed,
					StartedAt: taskStart,
				}

				if runErr != nil {
					r.Status = TaskStatusFailed
					r.Error = runErr.Error()
					mu.Lock()
					allSuccess = false
					mu.Unlock()
				} else {
					r.Status = TaskStatusSuccess
				}

				mu.Lock()
				results[taskID] = r
				mu.Unlock()
			}(id)
		}
		wg.Wait()
	}

	// Build ordered results
	orderedResults := make([]TaskResult, 0, len(w.Tasks))
	for _, t := range w.Tasks {
		if r, ok := results[t.ID]; ok {
			orderedResults = append(orderedResults, *r)
		}
	}

	wr := &WorkflowResult{
		Operation: "workflow.run",
		Name:      w.Name,
		Success:   allSuccess,
		Tasks:     orderedResults,
		Duration:  time.Since(start),
	}

	// Record workflow metrics for observability
	wm := CollectWorkflowMetrics(wr, len(layers))
	_ = RecordWorkflowMetrics(wm) // best-effort; don't fail the workflow

	return wr, nil
}

// WorkflowDir returns the standard workflow directory for the project.
func WorkflowDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".agm/workflows"
	}
	return filepath.Join(home, ".agm", "workflows")
}

// SaveWorkflow writes a workflow definition to the standard directory.
func SaveWorkflow(w *WorkflowDefinition, dir string) (string, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("failed to create workflow directory: %w", err)
	}

	data, err := yaml.Marshal(w)
	if err != nil {
		return "", fmt.Errorf("failed to marshal workflow: %w", err)
	}

	path := filepath.Join(dir, w.Name+".yaml")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return "", fmt.Errorf("failed to write workflow file: %w", err)
	}

	return path, nil
}

// ListWorkflowFiles returns all .yaml files in the workflows directory.
func ListWorkflowFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read workflow directory: %w", err)
	}

	var files []string
	for _, e := range entries {
		if !e.IsDir() && (strings.HasSuffix(e.Name(), ".yaml") || strings.HasSuffix(e.Name(), ".yml")) {
			files = append(files, filepath.Join(dir, e.Name()))
		}
	}
	return files, nil
}
