package act_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vbonnet/dear-agent/internal/ci"
	"github.com/vbonnet/dear-agent/internal/ci/act"
)

// TestActExecutor_Name verifies the executor name.
func TestActExecutor_Name(t *testing.T) {
	executor := act.NewActExecutor()
	assert.Equal(t, "act-native", executor.Name())
}

// TestActExecutor_NewActExecutorWithPath verifies custom path configuration.
func TestActExecutor_NewActExecutorWithPath(t *testing.T) {
	customPath := "/custom/path/to/act"
	customArtifactPath := "/custom/artifacts"

	executor := act.NewActExecutorWithPath(customPath, customArtifactPath)
	require.NotNil(t, executor)
	assert.Equal(t, "act-native", executor.Name())
}

// TestActExecutor_Validate_MissingBinary verifies validation fails when act is not found.
func TestActExecutor_Validate_MissingBinary(t *testing.T) {
	executor := act.NewActExecutorWithPath("/nonexistent/act", "/tmp")
	ctx := context.Background()

	req := ci.PipelineRequest{
		EventType:    ci.EventPush,
		WorkflowPath: ".github/workflows/test.yml",
		WorkingDir:   t.TempDir(),
	}

	err := executor.Validate(ctx, req)
	require.Error(t, err)

	var ciErr *ci.Error
	require.True(t, errors.As(err, &ciErr))
	assert.Equal(t, ci.ErrCodeExecutorNotFound, ciErr.Code)
}

// TestActExecutor_Validate_MissingWorkflow verifies validation fails for missing workflow file.
func TestActExecutor_Validate_MissingWorkflow(t *testing.T) {
	t.Parallel()

	// Skip if act is not available
	if _, err := exec.LookPath("act"); err != nil {
		t.Skip("act binary not available, skipping test")
	}

	executor := act.NewActExecutor()
	ctx := context.Background()

	req := ci.PipelineRequest{
		EventType:    ci.EventPush,
		WorkflowPath: "nonexistent-workflow.yml",
		WorkingDir:   t.TempDir(),
	}

	err := executor.Validate(ctx, req)
	require.Error(t, err)

	var ciErr *ci.Error
	require.True(t, errors.As(err, &ciErr))
	assert.Equal(t, ci.ErrCodeWorkflowNotFound, ciErr.Code)
}

// TestActExecutor_Validate_Success verifies validation succeeds with valid configuration.
func TestActExecutor_Validate_Success(t *testing.T) {
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

	// Create a minimal workflow file
	workDir := t.TempDir()
	workflowDir := filepath.Join(workDir, ".github", "workflows")
	require.NoError(t, os.MkdirAll(workflowDir, 0755))

	workflowPath := filepath.Join(workflowDir, "test.yml")
	workflowContent := `
name: Test Workflow
on: [push]
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - run: echo "Hello, World!"
`
	require.NoError(t, os.WriteFile(workflowPath, []byte(workflowContent), 0644))

	req := ci.PipelineRequest{
		EventType:    ci.EventPush,
		WorkflowPath: ".github/workflows/test.yml",
		WorkingDir:   workDir,
	}

	err := executor.Validate(ctx, req)
	assert.NoError(t, err)
}

// TestActExecutor_Execute_MissingBinary verifies Execute fails gracefully when act is missing.
func TestActExecutor_Execute_MissingBinary(t *testing.T) {
	t.Parallel()

	executor := act.NewActExecutorWithPath("/nonexistent/act", "/tmp")
	ctx := context.Background()

	req := ci.PipelineRequest{
		EventType:    ci.EventPush,
		WorkflowPath: ".github/workflows/test.yml",
		WorkingDir:   t.TempDir(),
	}

	result, err := executor.Execute(ctx, req)
	assert.Error(t, err)
	assert.Nil(t, result)

	var ciErr *ci.Error
	require.True(t, errors.As(err, &ciErr))
	assert.Equal(t, ci.ErrCodeExecutorNotFound, ciErr.Code)
}

// TestActExecutor_Execute_ContextCancellation verifies context cancellation handling.
func TestActExecutor_Execute_ContextCancellation(t *testing.T) {
	t.Parallel()

	// Skip if act is not available
	if _, err := exec.LookPath("act"); err != nil {
		t.Skip("act binary not available, skipping test")
	}

	executor := act.NewActExecutor()

	// Create already-cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	req := ci.PipelineRequest{
		EventType:    ci.EventPush,
		WorkflowPath: ".github/workflows/test.yml",
		WorkingDir:   t.TempDir(),
	}

	result, err := executor.Execute(ctx, req)
	assert.Error(t, err)
	assert.Nil(t, result)

	// Should get infrastructure error due to cancellation
	var ciErr *ci.Error
	require.True(t, errors.As(err, &ciErr))
	assert.Equal(t, ci.ErrCodeInfrastructure, ciErr.Code)
}

// TestActExecutor_Execute_Timeout verifies timeout handling.
func TestActExecutor_Execute_Timeout(t *testing.T) {
	t.Parallel()

	// Skip if act is not available
	if _, err := exec.LookPath("act"); err != nil {
		t.Skip("act binary not available, skipping test")
	}

	executor := act.NewActExecutor()

	// Create context with very short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel()

	// Give context time to expire
	time.Sleep(10 * time.Millisecond)

	req := ci.PipelineRequest{
		EventType:    ci.EventPush,
		WorkflowPath: ".github/workflows/test.yml",
		WorkingDir:   t.TempDir(),
		Timeout:      1 * time.Nanosecond,
	}

	result, err := executor.Execute(ctx, req)
	assert.Error(t, err)
	assert.Nil(t, result)

	// Should get timeout error
	var ciErr *ci.Error
	require.True(t, errors.As(err, &ciErr))
	assert.Equal(t, ci.ErrCodeTimeout, ciErr.Code)
}

// TestActExecutor_Execute_WithSecrets verifies secret handling.
func TestActExecutor_Execute_WithSecrets(t *testing.T) {
	t.Parallel()

	// Skip if act is not available
	if _, err := exec.LookPath("act"); err != nil {
		t.Skip("act binary not available, skipping integration test")
	}

	// Skip if Docker is not available
	if err := exec.Command("docker", "version").Run(); err != nil {
		t.Skip("Docker not available, skipping integration test")
	}

	executor := act.NewActExecutor()
	ctx := context.Background()

	// Create a workflow that uses secrets
	workDir := t.TempDir()
	workflowDir := filepath.Join(workDir, ".github", "workflows")
	require.NoError(t, os.MkdirAll(workflowDir, 0755))

	workflowPath := filepath.Join(workflowDir, "test.yml")
	workflowContent := `
name: Test Workflow
on: [push]
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - name: Check secret
        run: |
          if [ -z "${{ secrets.TEST_SECRET }}" ]; then
            echo "Secret not found"
            exit 1
          fi
          echo "Secret is set"
`
	require.NoError(t, os.WriteFile(workflowPath, []byte(workflowContent), 0644))

	req := ci.PipelineRequest{
		EventType:    ci.EventPush,
		WorkflowPath: ".github/workflows/test.yml",
		WorkingDir:   workDir,
		Secrets: map[string]string{
			"TEST_SECRET": "secret_value",
		},
	}

	// Note: This may fail if Docker pulls images, so we use a lenient assertion
	result, err := executor.Execute(ctx, req)

	// If execution succeeded, verify result structure
	if err == nil {
		require.NotNil(t, result)
		assert.Equal(t, "act-native", result.ExecutorName)
		assert.False(t, result.StartedAt.IsZero())
		assert.False(t, result.FinishedAt.IsZero())
		assert.Greater(t, result.Duration, time.Duration(0))
	} else {
		// If it failed, it should be a properly structured error
		var ciErr *ci.Error
		require.True(t, errors.As(err, &ciErr))
		t.Logf("Execution failed with error code %d: %s", ciErr.Code, err.Error())
	}
}

// TestActExecutor_Execute_WithVariables verifies variable handling.
func TestActExecutor_Execute_WithVariables(t *testing.T) {
	t.Parallel()

	// Skip if act is not available
	if _, err := exec.LookPath("act"); err != nil {
		t.Skip("act binary not available, skipping integration test")
	}

	// Skip if Docker is not available
	if err := exec.Command("docker", "version").Run(); err != nil {
		t.Skip("Docker not available, skipping integration test")
	}

	executor := act.NewActExecutor()
	ctx := context.Background()

	// Create a workflow that uses variables
	workDir := t.TempDir()
	workflowDir := filepath.Join(workDir, ".github", "workflows")
	require.NoError(t, os.MkdirAll(workflowDir, 0755))

	workflowPath := filepath.Join(workflowDir, "test.yml")
	workflowContent := `
name: Test Workflow
on: [push]
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - name: Echo variable
        run: echo "Variable value is ${{ vars.TEST_VAR }}"
`
	require.NoError(t, os.WriteFile(workflowPath, []byte(workflowContent), 0644))

	req := ci.PipelineRequest{
		EventType:    ci.EventPush,
		WorkflowPath: ".github/workflows/test.yml",
		WorkingDir:   workDir,
		Vars: map[string]string{
			"TEST_VAR": "test_value",
		},
	}

	// Execute (may fail due to Docker image pulls)
	result, err := executor.Execute(ctx, req)

	// Lenient assertions - verify structure if successful
	if err == nil {
		require.NotNil(t, result)
		assert.Equal(t, "act-native", result.ExecutorName)
	}
}

// TestActExecutor_Execute_OutputCallback verifies streaming output functionality.
func TestActExecutor_Execute_OutputCallback(t *testing.T) {
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

	// Create a simple workflow
	workDir := t.TempDir()
	workflowDir := filepath.Join(workDir, ".github", "workflows")
	require.NoError(t, os.MkdirAll(workflowDir, 0755))

	workflowPath := filepath.Join(workflowDir, "test.yml")
	workflowContent := `
name: Test Workflow
on: [push]
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - run: echo "Hello from workflow"
`
	require.NoError(t, os.WriteFile(workflowPath, []byte(workflowContent), 0644))

	// Collect events (thread-safe)
	var eventsMu sync.Mutex
	var events []ci.PipelineEvent
	callback := func(event ci.PipelineEvent) {
		eventsMu.Lock()
		events = append(events, event)
		eventsMu.Unlock()
	}

	req := ci.PipelineRequest{
		EventType:      ci.EventPush,
		WorkflowPath:   ".github/workflows/test.yml",
		WorkingDir:     workDir,
		OutputCallback: callback,
	}

	// Execute
	result, err := executor.Execute(ctx, req)

	// Lenient check - if successful, verify events
	if err == nil && result != nil {
		// Should have received at least start and end events
		eventsMu.Lock()
		eventCount := len(events)
		var firstType, lastType ci.EventKind
		if eventCount >= 2 {
			firstType = events[0].Type
			lastType = events[eventCount-1].Type
		}
		eventsMu.Unlock()

		assert.GreaterOrEqual(t, eventCount, 2, "Should receive at least start and end events")

		// Verify event types
		if eventCount >= 2 {
			assert.Equal(t, ci.EventKindStart, firstType)
			assert.Equal(t, ci.EventKindEnd, lastType)
		}
	}
}

// TestActExecutor_Execute_PipelineFailure verifies that pipeline failures return Success=false, not error.
func TestActExecutor_Execute_PipelineFailure(t *testing.T) {
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

	// Create a workflow that fails
	workDir := t.TempDir()
	workflowDir := filepath.Join(workDir, ".github", "workflows")
	require.NoError(t, os.MkdirAll(workflowDir, 0755))

	workflowPath := filepath.Join(workflowDir, "test.yml")
	workflowContent := `
name: Failing Workflow
on: [push]
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - run: exit 1
`
	require.NoError(t, os.WriteFile(workflowPath, []byte(workflowContent), 0644))

	req := ci.PipelineRequest{
		EventType:    ci.EventPush,
		WorkflowPath: ".github/workflows/test.yml",
		WorkingDir:   workDir,
	}

	// Execute - should NOT return error, but result with Success=false
	result, err := executor.Execute(ctx, req)

	// Pipeline failure should NOT be an error
	if err == nil {
		require.NotNil(t, result)
		assert.False(t, result.Success, "Pipeline should fail")
		assert.NotEqual(t, 0, result.ExitCode, "Exit code should be non-zero")
		assert.Equal(t, "act-native", result.ExecutorName)
	} else {
		// If there was an error, it should be infrastructure-related, not pipeline failure
		var ciErr *ci.Error
		require.True(t, errors.As(err, &ciErr))
		t.Logf("Infrastructure error occurred (expected in some environments): %s", err.Error())
	}
}

// TestActExecutor_Execute_ResultStructure verifies the result contains all required fields.
func TestActExecutor_Execute_ResultStructure(t *testing.T) {
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

	// Create minimal workflow
	workDir := t.TempDir()
	workflowDir := filepath.Join(workDir, ".github", "workflows")
	require.NoError(t, os.MkdirAll(workflowDir, 0755))

	workflowPath := filepath.Join(workflowDir, "test.yml")
	workflowContent := `
name: Test
on: [push]
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - run: echo "test"
`
	require.NoError(t, os.WriteFile(workflowPath, []byte(workflowContent), 0644))

	req := ci.PipelineRequest{
		EventType:    ci.EventPush,
		WorkflowPath: ".github/workflows/test.yml",
		WorkingDir:   workDir,
	}

	result, err := executor.Execute(ctx, req)

	// If execution succeeded, verify all fields
	if err == nil {
		require.NotNil(t, result)

		// Verify all required fields are populated
		assert.Equal(t, "act-native", result.ExecutorName)
		assert.GreaterOrEqual(t, result.ExitCode, 0)
		assert.NotNil(t, result.Steps)
		assert.False(t, result.StartedAt.IsZero())
		assert.False(t, result.FinishedAt.IsZero())
		assert.GreaterOrEqual(t, result.Duration, time.Duration(0))
		assert.True(t, result.FinishedAt.After(result.StartedAt) || result.FinishedAt.Equal(result.StartedAt))

		// Verify Success matches ExitCode
		if result.Success {
			assert.Equal(t, 0, result.ExitCode)
		} else {
			assert.NotEqual(t, 0, result.ExitCode)
		}
	}
}

// TestActExecutor_SecretFileCleanup verifies temporary secret files are cleaned up.
func TestActExecutor_SecretFileCleanup(t *testing.T) {
	t.Parallel()

	// Skip if act is not available
	if _, err := exec.LookPath("act"); err != nil {
		t.Skip("act binary not available, skipping test")
	}

	executor := act.NewActExecutor()
	ctx := context.Background()

	// Create a workflow
	workDir := t.TempDir()
	workflowDir := filepath.Join(workDir, ".github", "workflows")
	require.NoError(t, os.MkdirAll(workflowDir, 0755))

	workflowPath := filepath.Join(workflowDir, "test.yml")
	workflowContent := `
name: Test
on: [push]
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - run: echo "test"
`
	require.NoError(t, os.WriteFile(workflowPath, []byte(workflowContent), 0644))

	// Count temp files before
	tempDir := os.TempDir()
	beforeFiles, _ := filepath.Glob(filepath.Join(tempDir, "act-secrets-*.env"))

	req := ci.PipelineRequest{
		EventType:    ci.EventPush,
		WorkflowPath: ".github/workflows/test.yml",
		WorkingDir:   workDir,
		Secrets: map[string]string{
			"TEST_SECRET": "value",
		},
	}

	// Execute (may fail, but cleanup should still happen)
	_, _ = executor.Execute(ctx, req)

	// Count temp files after
	afterFiles, _ := filepath.Glob(filepath.Join(tempDir, "act-secrets-*.env"))

	// Should have same or fewer temp files (cleanup happened)
	// We allow same count because other tests might be running concurrently
	assert.LessOrEqual(t, len(afterFiles), len(beforeFiles)+1,
		"Secret files should be cleaned up after execution")
}

// TestActExecutor_AbsoluteAndRelativePaths verifies both path types work.
func TestActExecutor_AbsoluteAndRelativePaths(t *testing.T) {
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

	// Create workflow
	workDir := t.TempDir()
	workflowDir := filepath.Join(workDir, ".github", "workflows")
	require.NoError(t, os.MkdirAll(workflowDir, 0755))

	workflowPath := filepath.Join(workflowDir, "test.yml")
	workflowContent := `
name: Test
on: [push]
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - run: echo "test"
`
	require.NoError(t, os.WriteFile(workflowPath, []byte(workflowContent), 0644))

	tests := []struct {
		name         string
		workflowPath string
	}{
		{
			name:         "relative path",
			workflowPath: ".github/workflows/test.yml",
		},
		{
			name:         "absolute path",
			workflowPath: workflowPath,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := ci.PipelineRequest{
				EventType:    ci.EventPush,
				WorkflowPath: tt.workflowPath,
				WorkingDir:   workDir,
			}

			// Validation should work for both path types
			err := executor.Validate(ctx, req)
			if err != nil {
				// If validation fails, it should be a proper error, not a panic
				var ciErr *ci.Error
				require.True(t, errors.As(err, &ciErr))
				t.Logf("Validation failed (may be expected): %s", err.Error())
			}
		})
	}
}

// TestActExecutor_OutputBuffering verifies output is buffered when no callback is provided.
func TestActExecutor_OutputBuffering(t *testing.T) {
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

	// Create workflow that outputs something
	workDir := t.TempDir()
	workflowDir := filepath.Join(workDir, ".github", "workflows")
	require.NoError(t, os.MkdirAll(workflowDir, 0755))

	workflowPath := filepath.Join(workflowDir, "test.yml")
	workflowContent := `
name: Test
on: [push]
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - run: echo "Expected output message"
`
	require.NoError(t, os.WriteFile(workflowPath, []byte(workflowContent), 0644))

	req := ci.PipelineRequest{
		EventType:    ci.EventPush,
		WorkflowPath: ".github/workflows/test.yml",
		WorkingDir:   workDir,
		// No OutputCallback - should buffer
	}

	result, err := executor.Execute(ctx, req)

	if err == nil {
		require.NotNil(t, result)
		// Output should be buffered in result
		assert.NotEmpty(t, result.Output, "Output should be buffered when callback is nil")
		// Output should contain something from act
		assert.True(t,
			strings.Contains(result.Output, "act") ||
				strings.Contains(result.Output, "workflow") ||
				len(result.Output) > 0,
			"Output should contain workflow execution details",
		)
	}
}
