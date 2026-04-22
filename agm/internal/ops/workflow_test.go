package ops

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestParseWorkflow_Valid(t *testing.T) {
	yaml := `
name: build-and-test
tasks:
  - id: lint
    command: "golangci-lint run ./..."
  - id: build
    command: "go build ./..."
    depends_on: [lint]
  - id: test
    command: "go test ./..."
    depends_on: [build]
  - id: report
    command: "echo done"
    depends_on: [test]
`
	w, err := ParseWorkflow([]byte(yaml))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if w.Name != "build-and-test" {
		t.Errorf("expected name 'build-and-test', got %q", w.Name)
	}
	if len(w.Tasks) != 4 {
		t.Errorf("expected 4 tasks, got %d", len(w.Tasks))
	}
	if w.Tasks[1].DependsOn[0] != "lint" {
		t.Errorf("expected build to depend on lint, got %v", w.Tasks[1].DependsOn)
	}
}

func TestParseWorkflow_MissingName(t *testing.T) {
	yaml := `
tasks:
  - id: lint
    command: "lint"
`
	_, err := ParseWorkflow([]byte(yaml))
	if err == nil {
		t.Fatal("expected error for missing name")
	}
	if !strings.Contains(err.Error(), "name is required") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestParseWorkflow_NoTasks(t *testing.T) {
	yaml := `
name: empty
tasks: []
`
	_, err := ParseWorkflow([]byte(yaml))
	if err == nil {
		t.Fatal("expected error for empty tasks")
	}
	if !strings.Contains(err.Error(), "at least one task") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestParseWorkflow_DuplicateID(t *testing.T) {
	yaml := `
name: dup
tasks:
  - id: lint
    command: "lint"
  - id: lint
    command: "lint2"
`
	_, err := ParseWorkflow([]byte(yaml))
	if err == nil {
		t.Fatal("expected error for duplicate ID")
	}
	if !strings.Contains(err.Error(), "duplicate task ID") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestParseWorkflow_EmptyTaskID(t *testing.T) {
	yaml := `
name: bad
tasks:
  - id: ""
    command: "lint"
`
	_, err := ParseWorkflow([]byte(yaml))
	if err == nil {
		t.Fatal("expected error for empty task ID")
	}
}

func TestParseWorkflow_EmptyCommand(t *testing.T) {
	yaml := `
name: bad
tasks:
  - id: lint
    command: ""
`
	_, err := ParseWorkflow([]byte(yaml))
	if err == nil {
		t.Fatal("expected error for empty command")
	}
}

func TestParseWorkflow_UnknownDependency(t *testing.T) {
	yaml := `
name: bad-dep
tasks:
  - id: build
    command: "build"
    depends_on: [nonexistent]
`
	_, err := ParseWorkflow([]byte(yaml))
	if err == nil {
		t.Fatal("expected error for unknown dependency")
	}
	if !strings.Contains(err.Error(), "unknown task") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestParseWorkflow_SelfDependency(t *testing.T) {
	yaml := `
name: self-dep
tasks:
  - id: build
    command: "build"
    depends_on: [build]
`
	_, err := ParseWorkflow([]byte(yaml))
	if err == nil {
		t.Fatal("expected error for self dependency")
	}
	if !strings.Contains(err.Error(), "cannot depend on itself") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestTopologicalSort_Linear(t *testing.T) {
	w := &WorkflowDefinition{
		Name: "linear",
		Tasks: []TaskDefinition{
			{ID: "a", Command: "a"},
			{ID: "b", Command: "b", DependsOn: []string{"a"}},
			{ID: "c", Command: "c", DependsOn: []string{"b"}},
		},
	}
	layers, err := TopologicalSort(w)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(layers) != 3 {
		t.Fatalf("expected 3 layers, got %d: %v", len(layers), layers)
	}
	if layers[0][0] != "a" {
		t.Errorf("expected first layer [a], got %v", layers[0])
	}
	if layers[1][0] != "b" {
		t.Errorf("expected second layer [b], got %v", layers[1])
	}
	if layers[2][0] != "c" {
		t.Errorf("expected third layer [c], got %v", layers[2])
	}
}

func TestTopologicalSort_Parallel(t *testing.T) {
	w := &WorkflowDefinition{
		Name: "parallel",
		Tasks: []TaskDefinition{
			{ID: "a", Command: "a"},
			{ID: "b", Command: "b"},
			{ID: "c", Command: "c", DependsOn: []string{"a", "b"}},
		},
	}
	layers, err := TopologicalSort(w)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(layers) != 2 {
		t.Fatalf("expected 2 layers, got %d: %v", len(layers), layers)
	}
	if len(layers[0]) != 2 {
		t.Errorf("expected first layer to have 2 tasks, got %v", layers[0])
	}
	if layers[1][0] != "c" {
		t.Errorf("expected second layer [c], got %v", layers[1])
	}
}

func TestTopologicalSort_Diamond(t *testing.T) {
	// Diamond: a -> b, a -> c, b -> d, c -> d
	w := &WorkflowDefinition{
		Name: "diamond",
		Tasks: []TaskDefinition{
			{ID: "a", Command: "a"},
			{ID: "b", Command: "b", DependsOn: []string{"a"}},
			{ID: "c", Command: "c", DependsOn: []string{"a"}},
			{ID: "d", Command: "d", DependsOn: []string{"b", "c"}},
		},
	}
	layers, err := TopologicalSort(w)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(layers) != 3 {
		t.Fatalf("expected 3 layers, got %d: %v", len(layers), layers)
	}
	if len(layers[0]) != 1 || layers[0][0] != "a" {
		t.Errorf("layer 0: expected [a], got %v", layers[0])
	}
	if len(layers[1]) != 2 {
		t.Errorf("layer 1: expected 2 tasks, got %v", layers[1])
	}
	if len(layers[2]) != 1 || layers[2][0] != "d" {
		t.Errorf("layer 2: expected [d], got %v", layers[2])
	}
}

func TestTopologicalSort_Cycle(t *testing.T) {
	w := &WorkflowDefinition{
		Name: "cycle",
		Tasks: []TaskDefinition{
			{ID: "a", Command: "a", DependsOn: []string{"c"}},
			{ID: "b", Command: "b", DependsOn: []string{"a"}},
			{ID: "c", Command: "c", DependsOn: []string{"b"}},
		},
	}
	_, err := TopologicalSort(w)
	if err == nil {
		t.Fatal("expected error for cycle")
	}
	if !strings.Contains(err.Error(), "cycle") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestTopologicalSort_AllIndependent(t *testing.T) {
	w := &WorkflowDefinition{
		Name: "independent",
		Tasks: []TaskDefinition{
			{ID: "a", Command: "a"},
			{ID: "b", Command: "b"},
			{ID: "c", Command: "c"},
		},
	}
	layers, err := TopologicalSort(w)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(layers) != 1 {
		t.Fatalf("expected 1 layer, got %d: %v", len(layers), layers)
	}
	if len(layers[0]) != 3 {
		t.Errorf("expected 3 tasks in layer, got %v", layers[0])
	}
}

func TestExecuteWorkflow_Success(t *testing.T) {
	w := &WorkflowDefinition{
		Name: "test",
		Tasks: []TaskDefinition{
			{ID: "a", Command: "echo a"},
			{ID: "b", Command: "echo b", DependsOn: []string{"a"}},
		},
	}

	runner := func(_ context.Context, cmd string) (string, error) {
		return cmd + " output", nil
	}

	result, err := ExecuteWorkflow(context.Background(), w, runner)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Error("expected success")
	}
	if len(result.Tasks) != 2 {
		t.Fatalf("expected 2 task results, got %d", len(result.Tasks))
	}
	for _, tr := range result.Tasks {
		if tr.Status != TaskStatusSuccess {
			t.Errorf("task %s: expected success, got %s", tr.ID, tr.Status)
		}
	}
}

func TestExecuteWorkflow_FailurePropagation(t *testing.T) {
	w := &WorkflowDefinition{
		Name: "fail-chain",
		Tasks: []TaskDefinition{
			{ID: "a", Command: "fail"},
			{ID: "b", Command: "echo b", DependsOn: []string{"a"}},
			{ID: "c", Command: "echo c", DependsOn: []string{"b"}},
		},
	}

	runner := func(_ context.Context, cmd string) (string, error) {
		if cmd == "fail" {
			return "", fmt.Errorf("command failed")
		}
		return "ok", nil
	}

	result, err := ExecuteWorkflow(context.Background(), w, runner)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Success {
		t.Error("expected failure")
	}
	if result.Tasks[0].Status != TaskStatusFailed {
		t.Errorf("task a: expected failed, got %s", result.Tasks[0].Status)
	}
	if result.Tasks[1].Status != TaskStatusSkipped {
		t.Errorf("task b: expected skipped, got %s", result.Tasks[1].Status)
	}
	if result.Tasks[2].Status != TaskStatusSkipped {
		t.Errorf("task c: expected skipped, got %s", result.Tasks[2].Status)
	}
}

func TestExecuteWorkflow_ParallelExecution(t *testing.T) {
	w := &WorkflowDefinition{
		Name: "parallel",
		Tasks: []TaskDefinition{
			{ID: "a", Command: "a"},
			{ID: "b", Command: "b"},
			{ID: "c", Command: "c", DependsOn: []string{"a", "b"}},
		},
	}

	var mu sync.Mutex
	var runningCount int
	var maxRunning int

	runner := func(_ context.Context, _ string) (string, error) {
		mu.Lock()
		runningCount++
		if runningCount > maxRunning {
			maxRunning = runningCount
		}
		mu.Unlock()

		time.Sleep(10 * time.Millisecond) // Simulate work

		mu.Lock()
		runningCount--
		mu.Unlock()

		return "ok", nil
	}

	result, err := ExecuteWorkflow(context.Background(), w, runner)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Error("expected success")
	}
	if maxRunning < 2 {
		t.Errorf("expected parallel execution (max concurrent >= 2), got %d", maxRunning)
	}
}

func TestExecuteWorkflow_ContextCancellation(t *testing.T) {
	w := &WorkflowDefinition{
		Name: "cancel",
		Tasks: []TaskDefinition{
			{ID: "a", Command: "slow"},
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	runner := func(ctx context.Context, _ string) (string, error) {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(1 * time.Second):
			return "ok", nil
		}
	}

	result, err := ExecuteWorkflow(ctx, w, runner)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Success {
		t.Error("expected failure due to cancellation")
	}
	if result.Tasks[0].Status != TaskStatusFailed {
		t.Errorf("expected failed status, got %s", result.Tasks[0].Status)
	}
}

func TestExecuteWorkflow_PartialFailure(t *testing.T) {
	// a and b are independent; a fails, b succeeds; c depends on both
	w := &WorkflowDefinition{
		Name: "partial",
		Tasks: []TaskDefinition{
			{ID: "a", Command: "fail"},
			{ID: "b", Command: "ok"},
			{ID: "c", Command: "ok", DependsOn: []string{"a", "b"}},
		},
	}

	runner := func(_ context.Context, cmd string) (string, error) {
		if cmd == "fail" {
			return "", fmt.Errorf("failed")
		}
		return "ok", nil
	}

	result, err := ExecuteWorkflow(context.Background(), w, runner)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Success {
		t.Error("expected failure")
	}
	// a failed, b succeeded, c skipped (depends on a which failed)
	if result.Tasks[0].Status != TaskStatusFailed {
		t.Errorf("task a: expected failed, got %s", result.Tasks[0].Status)
	}
	if result.Tasks[1].Status != TaskStatusSuccess {
		t.Errorf("task b: expected success, got %s", result.Tasks[1].Status)
	}
	if result.Tasks[2].Status != TaskStatusSkipped {
		t.Errorf("task c: expected skipped, got %s", result.Tasks[2].Status)
	}
}

func TestParseWorkflow_InvalidYAML(t *testing.T) {
	_, err := ParseWorkflow([]byte("{{invalid"))
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}
