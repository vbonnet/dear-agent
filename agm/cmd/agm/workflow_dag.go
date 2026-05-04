package main

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/ops"
)

var workflowCreateCmd = &cobra.Command{
	Use:   "create <name>",
	Short: "Create a workflow from a YAML definition",
	Long: `Create a DAG-based workflow from a YAML definition file.

The YAML file defines tasks with dependencies. Tasks without dependencies
can run in parallel, while dependent tasks wait for their prerequisites.

Example YAML format:
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

Examples:
  agm workflow create build-pipeline -f pipeline.yaml
  agm workflow create deploy -f deploy.yaml`,
	Args: cobra.ExactArgs(1),
	RunE: runWorkflowCreate,
}

var workflowRunCmd = &cobra.Command{
	Use:   "run <name>",
	Short: "Execute a workflow respecting task dependencies",
	Long: `Execute a DAG-based workflow. Tasks are executed in topological order,
with independent tasks running in parallel.

If a task fails, all downstream dependent tasks are skipped.

Examples:
  agm workflow run build-pipeline
  agm workflow run deploy --timeout 5m`,
	Args: cobra.ExactArgs(1),
	RunE: runWorkflowRun,
}

var workflowStatusCmd = &cobra.Command{
	Use:   "status <name>",
	Short: "Show workflow definition and dependency graph",
	Long: `Display the structure of a workflow including tasks,
dependencies, and execution layers (groups of parallelizable tasks).

Examples:
  agm workflow status build-pipeline
  agm workflow status deploy`,
	Args: cobra.ExactArgs(1),
	RunE: runWorkflowStatus,
}

var (
	workflowFile    string
	workflowTimeout time.Duration
)

func init() {
	workflowCmd.AddCommand(workflowCreateCmd)
	workflowCmd.AddCommand(workflowRunCmd)
	workflowCmd.AddCommand(workflowStatusCmd)

	workflowCreateCmd.Flags().StringVarP(&workflowFile, "file", "f", "", "YAML workflow definition file (required)")
	_ = workflowCreateCmd.MarkFlagRequired("file")

	workflowRunCmd.Flags().DurationVar(&workflowTimeout, "timeout", 10*time.Minute, "maximum execution time for the workflow")
}

func workflowDir() string {
	return ops.WorkflowDir()
}

func runWorkflowCreate(_ *cobra.Command, args []string) error {
	name := args[0]

	w, err := ops.LoadWorkflow(workflowFile)
	if err != nil {
		return fmt.Errorf("failed to load workflow: %w", err)
	}

	// Override name from CLI argument
	w.Name = name

	// Validate DAG (catch cycles early)
	if _, err := ops.TopologicalSort(w); err != nil {
		return fmt.Errorf("invalid workflow graph: %w", err)
	}

	dir := workflowDir()
	path, err := ops.SaveWorkflow(w, dir)
	if err != nil {
		return err
	}

	fmt.Printf("Workflow %q created: %s\n", name, path)
	fmt.Printf("  Tasks: %d\n", len(w.Tasks))

	// Show task summary
	for _, t := range w.Tasks {
		deps := ""
		if len(t.DependsOn) > 0 {
			deps = fmt.Sprintf(" (depends on: %s)", strings.Join(t.DependsOn, ", "))
		}
		fmt.Printf("    - %s: %s%s\n", t.ID, t.Command, deps)
	}

	return nil
}

func runWorkflowRun(_ *cobra.Command, args []string) error {
	name := args[0]

	// Load workflow from standard directory
	dir := workflowDir()
	path := filepath.Join(dir, name+".yaml")

	w, err := ops.LoadWorkflow(path)
	if err != nil {
		return fmt.Errorf("workflow %q not found: %w", name, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), workflowTimeout)
	defer cancel()

	fmt.Printf("Running workflow: %s\n", w.Name)
	fmt.Printf("  Tasks: %d\n", len(w.Tasks))
	fmt.Println()

	result, err := ops.ExecuteWorkflow(ctx, w, ops.DefaultCommandRunner)
	if err != nil {
		return fmt.Errorf("workflow execution failed: %w", err)
	}

	if outputFormat == "json" {
		data, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal result: %w", err)
		}
		fmt.Println(string(data))
		return nil
	}

	// Text output
	for _, tr := range result.Tasks {
		statusIcon := statusSymbol(tr.Status)
		fmt.Printf("  %s %s (%s)\n", statusIcon, tr.ID, tr.Duration.Round(time.Millisecond))
		if tr.Error != "" {
			fmt.Printf("    error: %s\n", tr.Error)
		}
		if tr.Output != "" && tr.Status == ops.TaskStatusFailed {
			// Only show output for failed tasks to keep output clean
			fmt.Printf("    output: %s\n", strings.TrimSpace(tr.Output))
		}
	}

	fmt.Printf("\nTotal: %s\n", result.Duration.Round(time.Millisecond))

	if !result.Success {
		return fmt.Errorf("workflow %q failed", name)
	}

	fmt.Println("Workflow completed successfully.")
	return nil
}

func runWorkflowStatus(_ *cobra.Command, args []string) error {
	name := args[0]

	dir := workflowDir()
	path := filepath.Join(dir, name+".yaml")

	w, err := ops.LoadWorkflow(path)
	if err != nil {
		return fmt.Errorf("workflow %q not found: %w", name, err)
	}

	layers, err := ops.TopologicalSort(w)
	if err != nil {
		return fmt.Errorf("invalid workflow graph: %w", err)
	}

	if outputFormat == "json" {
		out := struct {
			Name   string     `json:"name"`
			Tasks  int        `json:"task_count"`
			Layers [][]string `json:"layers"`
		}{
			Name:   w.Name,
			Tasks:  len(w.Tasks),
			Layers: layers,
		}
		data, err := json.MarshalIndent(out, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal status: %w", err)
		}
		fmt.Println(string(data))
		return nil
	}

	fmt.Printf("Workflow: %s\n", w.Name)
	fmt.Printf("Tasks: %d\n", len(w.Tasks))
	fmt.Println()

	// Show tasks with dependencies
	fmt.Println("Tasks:")
	taskMap := make(map[string]*ops.TaskDefinition, len(w.Tasks))
	for i := range w.Tasks {
		taskMap[w.Tasks[i].ID] = &w.Tasks[i]
	}
	for _, t := range w.Tasks {
		deps := "none"
		if len(t.DependsOn) > 0 {
			deps = strings.Join(t.DependsOn, ", ")
		}
		fmt.Printf("  %s\n", t.ID)
		fmt.Printf("    command:  %s\n", t.Command)
		fmt.Printf("    depends:  %s\n", deps)
	}

	fmt.Println()
	fmt.Println("Execution layers (tasks in same layer run in parallel):")
	for i, layer := range layers {
		fmt.Printf("  Layer %d: %s\n", i+1, strings.Join(layer, ", "))
	}

	return nil
}

func statusSymbol(s ops.TaskStatus) string {
	//nolint:exhaustive // intentional partial: handles the relevant subset
	switch s {
	case ops.TaskStatusSuccess:
		return "[OK]"
	case ops.TaskStatusFailed:
		return "[FAIL]"
	case ops.TaskStatusSkipped:
		return "[SKIP]"
	case ops.TaskStatusRunning:
		return "[..]"
	default:
		return "[--]"
	}
}
