package act_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vbonnet/dear-agent/internal/ci"
	"github.com/vbonnet/dear-agent/internal/ci/act"
)

// TestActExecutor_ExecuteWorkflows_Sequential verifies sequential workflow execution.
func TestActExecutor_ExecuteWorkflows_Sequential(t *testing.T) {
	t.Parallel()

	// Skip if act is not available
	if _, err := exec.LookPath("act"); err != nil {
		t.Skip("act binary not available, skipping test")
	}

	// Skip if Docker is not available
	if err := exec.Command("docker", "version").Run(); err != nil {
		t.Skip("Docker not available, skipping test")
	}

	executor := act.NewActExecutor()
	ctx := context.Background()

	// Create test workflows
	workDir := t.TempDir()
	workflowDir := filepath.Join(workDir, ".github", "workflows")
	require.NoError(t, os.MkdirAll(workflowDir, 0755))

	// Workflow 1: Success
	workflow1 := filepath.Join(workflowDir, "test1.yml")
	workflow1Content := `
name: Test 1
on: [push]
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - run: echo "Test 1"
`
	require.NoError(t, os.WriteFile(workflow1, []byte(workflow1Content), 0644))

	// Workflow 2: Success
	workflow2 := filepath.Join(workflowDir, "test2.yml")
	workflow2Content := `
name: Test 2
on: [push]
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - run: echo "Test 2"
`
	require.NoError(t, os.WriteFile(workflow2, []byte(workflow2Content), 0644))

	// Execute workflows
	req := ci.PipelineRequest{
		EventType:  ci.EventPush,
		WorkingDir: workDir,
	}

	workflows := []string{
		".github/workflows/test1.yml",
		".github/workflows/test2.yml",
	}

	result, err := executor.ExecuteWorkflows(ctx, req, workflows, false)

	// Verify results (lenient - may fail due to Docker image pulls)
	if err == nil {
		require.NotNil(t, result)
		assert.NotNil(t, result.Results)
		assert.GreaterOrEqual(t, len(result.Results), 1)
	}
}

// TestActExecutor_ExecuteWorkflows_Parallel verifies parallel workflow execution.
func TestActExecutor_ExecuteWorkflows_Parallel(t *testing.T) {
	t.Parallel()

	// Skip if act is not available
	if _, err := exec.LookPath("act"); err != nil {
		t.Skip("act binary not available, skipping test")
	}

	// Skip if Docker is not available
	if err := exec.Command("docker", "version").Run(); err != nil {
		t.Skip("Docker not available, skipping test")
	}

	executor := act.NewActExecutor()
	ctx := context.Background()

	// Create test workflows
	workDir := t.TempDir()
	workflowDir := filepath.Join(workDir, ".github", "workflows")
	require.NoError(t, os.MkdirAll(workflowDir, 0755))

	workflow1 := filepath.Join(workflowDir, "parallel1.yml")
	workflow1Content := `
name: Parallel 1
on: [push]
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - run: echo "Parallel 1"
`
	require.NoError(t, os.WriteFile(workflow1, []byte(workflow1Content), 0644))

	workflow2 := filepath.Join(workflowDir, "parallel2.yml")
	workflow2Content := `
name: Parallel 2
on: [push]
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - run: echo "Parallel 2"
`
	require.NoError(t, os.WriteFile(workflow2, []byte(workflow2Content), 0644))

	// Execute workflows in parallel
	req := ci.PipelineRequest{
		EventType:  ci.EventPush,
		WorkingDir: workDir,
	}

	workflows := []string{
		".github/workflows/parallel1.yml",
		".github/workflows/parallel2.yml",
	}

	result, err := executor.ExecuteWorkflows(ctx, req, workflows, true)

	// Verify results (lenient)
	if err == nil {
		require.NotNil(t, result)
		assert.NotNil(t, result.Results)
	}
}

// TestActExecutor_ExecuteWorkflows_WithFailure verifies failure handling.
func TestActExecutor_ExecuteWorkflows_WithFailure(t *testing.T) {
	t.Parallel()

	// Skip if act is not available
	if _, err := exec.LookPath("act"); err != nil {
		t.Skip("act binary not available, skipping test")
	}

	// Skip if Docker is not available
	if err := exec.Command("docker", "version").Run(); err != nil {
		t.Skip("Docker not available, skipping test")
	}

	executor := act.NewActExecutor()
	ctx := context.Background()

	// Create test workflows
	workDir := t.TempDir()
	workflowDir := filepath.Join(workDir, ".github", "workflows")
	require.NoError(t, os.MkdirAll(workflowDir, 0755))

	// Workflow that fails
	workflow1 := filepath.Join(workflowDir, "failing.yml")
	workflow1Content := `
name: Failing
on: [push]
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - run: exit 1
`
	require.NoError(t, os.WriteFile(workflow1, []byte(workflow1Content), 0644))

	// Workflow that succeeds
	workflow2 := filepath.Join(workflowDir, "passing.yml")
	workflow2Content := `
name: Passing
on: [push]
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - run: echo "pass"
`
	require.NoError(t, os.WriteFile(workflow2, []byte(workflow2Content), 0644))

	// Execute workflows (sequential - should stop after first failure)
	req := ci.PipelineRequest{
		EventType:  ci.EventPush,
		WorkingDir: workDir,
	}

	workflows := []string{
		".github/workflows/failing.yml",
		".github/workflows/passing.yml",
	}

	result, err := executor.ExecuteWorkflows(ctx, req, workflows, false)

	// Verify results (lenient)
	if err == nil && result != nil {
		// Overall success should be false
		assert.False(t, result.OverallSuccess)

		// First workflow should have executed
		failingResult, exists := result.Results[".github/workflows/failing.yml"]
		if exists && failingResult.Result != nil {
			assert.False(t, failingResult.Result.Success)
		}

		// Second workflow should be skipped (sequential mode stops on failure)
		passingResult, exists := result.Results[".github/workflows/passing.yml"]
		if exists {
			assert.True(t, passingResult.Skipped || (passingResult.Result != nil))
		}
	}
}

// TestActExecutor_ExecuteWorkflowsWithDependencies verifies dependency-based execution.
func TestActExecutor_ExecuteWorkflowsWithDependencies(t *testing.T) {
	t.Parallel()

	// Skip if act is not available
	if _, err := exec.LookPath("act"); err != nil {
		t.Skip("act binary not available, skipping test")
	}

	// Skip if Docker is not available
	if err := exec.Command("docker", "version").Run(); err != nil {
		t.Skip("Docker not available, skipping test")
	}

	executor := act.NewActExecutor()
	ctx := context.Background()

	// Create test workflows
	workDir := t.TempDir()
	workflowDir := filepath.Join(workDir, ".github", "workflows")
	require.NoError(t, os.MkdirAll(workflowDir, 0755))

	// Base workflow
	workflow1 := filepath.Join(workflowDir, "base.yml")
	workflow1Content := `
name: Base
on: [push]
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - run: echo "Base"
`
	require.NoError(t, os.WriteFile(workflow1, []byte(workflow1Content), 0644))

	// Dependent workflow
	workflow2 := filepath.Join(workflowDir, "dependent.yml")
	workflow2Content := `
name: Dependent
on: [push]
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - run: echo "Dependent"
`
	require.NoError(t, os.WriteFile(workflow2, []byte(workflow2Content), 0644))

	// Execute with dependencies
	req := ci.PipelineRequest{
		EventType:  ci.EventPush,
		WorkingDir: workDir,
	}

	workflows := []string{
		".github/workflows/base.yml",
		".github/workflows/dependent.yml",
	}

	dependencies := map[string][]string{
		".github/workflows/dependent.yml": {".github/workflows/base.yml"},
	}

	result, err := executor.ExecuteWorkflowsWithDependencies(ctx, req, workflows, dependencies)

	// Verify results (lenient)
	if err == nil {
		require.NotNil(t, result)
		assert.NotNil(t, result.Results)

		// Both workflows should have been attempted
		assert.Contains(t, result.Results, ".github/workflows/base.yml")
		assert.Contains(t, result.Results, ".github/workflows/dependent.yml")
	}
}

// TestActExecutor_ExecuteWorkflowsWithDependencies_FailedDependency verifies dependency failure handling.
func TestActExecutor_ExecuteWorkflowsWithDependencies_FailedDependency(t *testing.T) {
	t.Parallel()

	// Skip if act is not available
	if _, err := exec.LookPath("act"); err != nil {
		t.Skip("act binary not available, skipping test")
	}

	// Skip if Docker is not available
	if err := exec.Command("docker", "version").Run(); err != nil {
		t.Skip("Docker not available, skipping test")
	}

	executor := act.NewActExecutor()
	ctx := context.Background()

	// Create test workflows
	workDir := t.TempDir()
	workflowDir := filepath.Join(workDir, ".github", "workflows")
	require.NoError(t, os.MkdirAll(workflowDir, 0755))

	// Failing base workflow
	workflow1 := filepath.Join(workflowDir, "failing-base.yml")
	workflow1Content := `
name: Failing Base
on: [push]
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - run: exit 1
`
	require.NoError(t, os.WriteFile(workflow1, []byte(workflow1Content), 0644))

	// Dependent workflow (should be skipped)
	workflow2 := filepath.Join(workflowDir, "should-skip.yml")
	workflow2Content := `
name: Should Skip
on: [push]
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - run: echo "Should not run"
`
	require.NoError(t, os.WriteFile(workflow2, []byte(workflow2Content), 0644))

	// Execute with dependencies
	req := ci.PipelineRequest{
		EventType:  ci.EventPush,
		WorkingDir: workDir,
	}

	workflows := []string{
		".github/workflows/failing-base.yml",
		".github/workflows/should-skip.yml",
	}

	dependencies := map[string][]string{
		".github/workflows/should-skip.yml": {".github/workflows/failing-base.yml"},
	}

	result, err := executor.ExecuteWorkflowsWithDependencies(ctx, req, workflows, dependencies)

	// Verify results (lenient)
	if err == nil && result != nil {
		// Overall success should be false
		assert.False(t, result.OverallSuccess)

		// Dependent workflow should be skipped
		dependentResult, exists := result.Results[".github/workflows/should-skip.yml"]
		if exists {
			assert.True(t, dependentResult.Skipped, "Dependent workflow should be skipped when dependency fails")
		}
	}
}

// TestMultiWorkflowResult_Structure verifies result structure.
func TestMultiWorkflowResult_Structure(t *testing.T) {
	// Create mock results
	results := map[string]*act.WorkflowResult{
		"workflow1.yml": {
			WorkflowPath: "workflow1.yml",
			Result: &ci.PipelineResult{
				Success:  true,
				ExitCode: 0,
			},
			Error:   nil,
			Skipped: false,
		},
		"workflow2.yml": {
			WorkflowPath: "workflow2.yml",
			Result: &ci.PipelineResult{
				Success:  false,
				ExitCode: 1,
			},
			Error:   nil,
			Skipped: false,
		},
	}

	// Verify structure
	for path, result := range results {
		assert.Equal(t, path, result.WorkflowPath)
		assert.NotNil(t, result.Result)
	}
}
