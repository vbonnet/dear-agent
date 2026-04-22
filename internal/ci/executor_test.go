package ci_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vbonnet/dear-agent/internal/ci"
)

// testExecutor runs standard contract tests against any PipelineExecutor implementation.
// Call this from each executor's test file:
//
//	func TestActExecutor(t *testing.T) {
//	    executor := act.NewExecutor()
//	    testExecutor(t, executor)
//	}
//
//nolint:unused // Reserved for future executor implementations
func testExecutor(t *testing.T, executor ci.PipelineExecutor) {
	t.Run("Execute_success", func(t *testing.T) {
		t.Parallel()
		testExecuteSuccess(t, executor)
	})

	t.Run("Execute_failure", func(t *testing.T) {
		t.Parallel()
		testExecuteFailure(t, executor)
	})

	t.Run("Execute_with_timeout", func(t *testing.T) {
		t.Parallel()
		testExecuteWithTimeout(t, executor)
	})

	t.Run("Validate_invalid_config", func(t *testing.T) {
		t.Parallel()
		testValidateInvalidConfig(t, executor)
	})
}

func testExecuteSuccess(t *testing.T, executor ci.PipelineExecutor) {
	ctx := context.Background()

	req := ci.PipelineRequest{
		EventType:    ci.EventPush,
		WorkflowPath: ".github/workflows/test.yml",
		WorkingDir:   t.TempDir(),
		Secrets:      map[string]string{"TEST_SECRET": "test_value"},
		Vars:         map[string]string{"TEST_VAR": "test_var_value"},
	}

	// Execute pipeline
	result, err := executor.Execute(ctx, req)
	require.NoError(t, err, "Execute should not return infrastructure error")
	require.NotNil(t, result, "Result should not be nil")

	// Verify result structure
	assert.NotEmpty(t, result.ExecutorName, "ExecutorName should be set")
	assert.GreaterOrEqual(t, result.ExitCode, 0, "ExitCode should be non-negative")
	assert.NotNil(t, result.Steps, "Steps should be initialized (even if empty)")
	assert.False(t, result.StartedAt.IsZero(), "StartedAt should be set")
	assert.False(t, result.FinishedAt.IsZero(), "FinishedAt should be set")
	assert.GreaterOrEqual(t, result.Duration, time.Duration(0), "Duration should be non-negative")

	// Success should match exit code
	if result.Success {
		assert.Equal(t, 0, result.ExitCode, "Success=true should have ExitCode=0")
	} else {
		assert.NotEqual(t, 0, result.ExitCode, "Success=false should have non-zero ExitCode")
	}

	// Verify timing consistency
	assert.True(t, result.FinishedAt.After(result.StartedAt) || result.FinishedAt.Equal(result.StartedAt),
		"FinishedAt should be >= StartedAt")
}

func testExecuteFailure(t *testing.T, executor ci.PipelineExecutor) {
	// For MockExecutor, we configure a failure result
	if mock, ok := executor.(*ci.MockExecutor); ok {
		now := time.Now()
		mock.SetExecuteResult(&ci.PipelineResult{
			Success:  false,
			ExitCode: 1,
			Output:   "Pipeline failed\n",
			Steps: []ci.StepResult{
				{
					Name:       "Failing step",
					Status:     ci.StepStatusFailure,
					ExitCode:   1,
					Output:     "Step failed\n",
					Duration:   10 * time.Millisecond,
					StartedAt:  now,
					FinishedAt: now.Add(10 * time.Millisecond),
				},
			},
			Duration:   10 * time.Millisecond,
			StartedAt:  now,
			FinishedAt: now.Add(10 * time.Millisecond),
		})
	}

	ctx := context.Background()
	req := ci.PipelineRequest{
		EventType:    ci.EventPush,
		WorkflowPath: ".github/workflows/test.yml",
		WorkingDir:   t.TempDir(),
	}

	// Execute should NOT return error for pipeline failure
	result, err := executor.Execute(ctx, req)
	require.NoError(t, err, "Execute should not return error for pipeline failure")
	require.NotNil(t, result, "Result should not be nil")

	// For mocked failures, verify the failure state
	if mock, ok := executor.(*ci.MockExecutor); ok {
		_ = mock // Suppress unused warning
		assert.False(t, result.Success, "Success should be false for failed pipeline")
		assert.NotEqual(t, 0, result.ExitCode, "ExitCode should be non-zero for failure")
	}
}

func testExecuteWithTimeout(t *testing.T, executor ci.PipelineExecutor) {
	// Configure mock to simulate slow execution
	if mock, ok := executor.(*ci.MockExecutor); ok {
		mock.SetExecuteDelay(100 * time.Millisecond)
	}

	// Create context that's already cancelled
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	req := ci.PipelineRequest{
		EventType:    ci.EventPush,
		WorkflowPath: ".github/workflows/test.yml",
		WorkingDir:   t.TempDir(),
	}

	// Execute should fail due to cancelled context
	_, err := executor.Execute(ctx, req)
	assert.Error(t, err, "Execute should fail with cancelled context")

	// Error should be context-related
	assert.True(t, errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded),
		"Error should be context.Canceled or context.DeadlineExceeded")
}

func testValidateInvalidConfig(t *testing.T, executor ci.PipelineExecutor) {
	// Configure mock to return validation error
	if mock, ok := executor.(*ci.MockExecutor); ok {
		mock.SetValidateError(ci.ErrWorkflowInvalid("invalid.yml", errors.New("syntax error")))
	}

	ctx := context.Background()
	req := ci.PipelineRequest{
		EventType:    ci.EventPush,
		WorkflowPath: "invalid-workflow.yml",
		WorkingDir:   t.TempDir(),
	}

	// Validate should return error for invalid configuration
	err := executor.Validate(ctx, req)

	// For MockExecutor with injected error, verify it fails
	if mock, ok := executor.(*ci.MockExecutor); ok {
		_ = mock // Suppress unused warning
		require.Error(t, err, "Validate should fail for invalid configuration")

		// Check it's the right error type
		var ciErr *ci.Error
		if assert.True(t, errors.As(err, &ciErr), "Error should be *ci.Error") {
			assert.Equal(t, ci.ErrCodeWorkflowInvalid, ciErr.Code,
				"Error code should be ErrCodeWorkflowInvalid")
		}
	}
}

// TestMockExecutor verifies the MockExecutor satisfies the contract
func TestMockExecutor(t *testing.T) {
	// Note: We don't use a shared executor because contract tests may modify mock state
	// Each sub-test creates its own executor instance in the testExecutor helper

	t.Run("Execute_success", func(t *testing.T) {
		t.Parallel()
		testExecuteSuccess(t, ci.NewMockExecutor())
	})

	t.Run("Execute_failure", func(t *testing.T) {
		t.Parallel()
		testExecuteFailure(t, ci.NewMockExecutor())
	})

	t.Run("Execute_with_timeout", func(t *testing.T) {
		t.Parallel()
		testExecuteWithTimeout(t, ci.NewMockExecutor())
	})

	t.Run("Validate_invalid_config", func(t *testing.T) {
		t.Parallel()
		testValidateInvalidConfig(t, ci.NewMockExecutor())
	})
}

// TestMockExecutorCustomName verifies custom naming
func TestMockExecutorCustomName(t *testing.T) {
	customName := "custom-mock"
	executor := ci.NewMockExecutorWithName(customName)
	require.NotNil(t, executor)

	assert.Equal(t, customName, executor.Name(), "Name should match custom name")
}

// TestMockExecutorErrorInjection tests error injection capabilities
func TestMockExecutorErrorInjection(t *testing.T) {
	t.Run("Execute_error_injection", func(t *testing.T) {
		t.Parallel()
		executor := ci.NewMockExecutor()
		expectedErr := ci.NewError(ci.ErrCodeExecutorNotFound, "injected error")
		executor.SetExecuteError(expectedErr)

		ctx := context.Background()
		req := ci.PipelineRequest{
			EventType:    ci.EventPush,
			WorkflowPath: "test.yml",
			WorkingDir:   t.TempDir(),
		}

		_, err := executor.Execute(ctx, req)
		require.Error(t, err, "Execute should fail with injected error")
		assert.Equal(t, expectedErr, err, "Error should match injected error")
	})

	t.Run("Validate_error_injection", func(t *testing.T) {
		t.Parallel()
		executor := ci.NewMockExecutor()
		expectedErr := ci.NewError(ci.ErrCodeWorkflowInvalid, "injected validation error")
		executor.SetValidateError(expectedErr)

		ctx := context.Background()
		req := ci.PipelineRequest{
			EventType:    ci.EventPush,
			WorkflowPath: "invalid.yml",
			WorkingDir:   t.TempDir(),
		}

		err := executor.Validate(ctx, req)
		require.Error(t, err, "Validate should fail with injected error")
		assert.Equal(t, expectedErr, err, "Error should match injected error")
	})
}

// TestMockExecutorResultCustomization tests result customization
func TestMockExecutorResultCustomization(t *testing.T) {
	executor := ci.NewMockExecutor()
	ctx := context.Background()

	customResult := &ci.PipelineResult{
		Success:  false,
		ExitCode: 42,
		Output:   "Custom output\n",
		Steps: []ci.StepResult{
			{
				Name:       "Custom step",
				Status:     ci.StepStatusSuccess,
				ExitCode:   0,
				Output:     "Step output\n",
				Duration:   100 * time.Millisecond,
				StartedAt:  time.Now(),
				FinishedAt: time.Now().Add(100 * time.Millisecond),
			},
		},
		Duration:   200 * time.Millisecond,
		StartedAt:  time.Now(),
		FinishedAt: time.Now().Add(200 * time.Millisecond),
	}
	executor.SetExecuteResult(customResult)

	req := ci.PipelineRequest{
		EventType:    ci.EventPush,
		WorkflowPath: "test.yml",
		WorkingDir:   t.TempDir(),
	}

	result, err := executor.Execute(ctx, req)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Verify custom result fields
	assert.Equal(t, customResult.Success, result.Success)
	assert.Equal(t, customResult.ExitCode, result.ExitCode)
	assert.Equal(t, customResult.Output, result.Output)
	assert.Equal(t, len(customResult.Steps), len(result.Steps))
	if len(result.Steps) > 0 {
		assert.Equal(t, customResult.Steps[0].Name, result.Steps[0].Name)
	}
}

// TestMockExecutorOutputCallback tests streaming output
func TestMockExecutorOutputCallback(t *testing.T) {
	executor := ci.NewMockExecutor()
	ctx := context.Background()

	// Configure output events
	events := []ci.PipelineEvent{
		{
			Type:      ci.EventKindStart,
			Timestamp: time.Now(),
		},
		{
			Type:      ci.EventKindOutput,
			StepName:  "test-step",
			Output:    "Test output\n",
			Timestamp: time.Now(),
		},
		{
			Type:      ci.EventKindEnd,
			Timestamp: time.Now(),
		},
	}
	executor.SetOutputEvents(events)

	// Collect events from callback
	var receivedEvents []ci.PipelineEvent
	callback := func(event ci.PipelineEvent) {
		receivedEvents = append(receivedEvents, event)
	}

	req := ci.PipelineRequest{
		EventType:      ci.EventPush,
		WorkflowPath:   "test.yml",
		WorkingDir:     t.TempDir(),
		OutputCallback: callback,
	}

	_, err := executor.Execute(ctx, req)
	require.NoError(t, err)

	// Verify events were received
	assert.Equal(t, len(events), len(receivedEvents), "Should receive all events")
	for i, event := range events {
		assert.Equal(t, event.Type, receivedEvents[i].Type, "Event type should match")
		if event.StepName != "" {
			assert.Equal(t, event.StepName, receivedEvents[i].StepName, "Step name should match")
		}
		if event.Output != "" {
			assert.Equal(t, event.Output, receivedEvents[i].Output, "Output should match")
		}
	}
}

// TestMockExecutorRequestTracking tests request tracking
func TestMockExecutorRequestTracking(t *testing.T) {
	executor := ci.NewMockExecutor()
	ctx := context.Background()

	// Execute multiple requests
	for i := 0; i < 3; i++ {
		req := ci.PipelineRequest{
			EventType:    ci.EventPush,
			WorkflowPath: "test.yml",
			WorkingDir:   t.TempDir(),
		}
		_, err := executor.Execute(ctx, req)
		require.NoError(t, err)
	}

	// Verify execution count
	assert.Equal(t, 3, executor.GetExecuteCount(), "Should track execution count")

	// Verify requests are tracked
	requests := executor.GetExecutedRequests()
	assert.Equal(t, 3, len(requests), "Should track all requests")

	// Validate multiple requests
	for i := 0; i < 2; i++ {
		req := ci.PipelineRequest{
			EventType:    ci.EventPush,
			WorkflowPath: "test.yml",
			WorkingDir:   t.TempDir(),
		}
		_ = executor.Validate(ctx, req)
	}

	// Verify validation count
	assert.Equal(t, 2, executor.GetValidateCount(), "Should track validation count")

	// Test reset
	executor.Reset()
	assert.Equal(t, 0, executor.GetExecuteCount(), "Count should reset to 0")
	assert.Equal(t, 0, executor.GetValidateCount(), "Count should reset to 0")
}

// TestMockExecutorConcurrency verifies thread-safety
func TestMockExecutorConcurrency(t *testing.T) {
	executor := ci.NewMockExecutor()
	ctx := context.Background()

	// Execute multiple pipelines concurrently
	const numGoroutines = 10
	results := make(chan error, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			req := ci.PipelineRequest{
				EventType:    ci.EventPush,
				WorkflowPath: "test.yml",
				WorkingDir:   t.TempDir(),
			}
			_, err := executor.Execute(ctx, req)
			results <- err
		}()
	}

	// Verify all succeeded
	for i := 0; i < numGoroutines; i++ {
		err := <-results
		assert.NoError(t, err, "Concurrent Execute should succeed")
	}

	// Verify count is accurate
	assert.Equal(t, numGoroutines, executor.GetExecuteCount(), "Should track all executions")
}
