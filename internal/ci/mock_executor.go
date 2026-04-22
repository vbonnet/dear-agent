// Package ci provides ci functionality.
package ci

import (
	"context"
	"sync"
	"time"
)

// MockExecutor is a fake implementation of PipelineExecutor for testing.
// It doesn't actually execute pipelines, just tracks state in memory.
//
// Thread-safe: All methods are protected by a mutex.
type MockExecutor struct {
	mu sync.Mutex

	// name is the executor identifier
	name string

	// executedRequests tracks all Execute() calls
	executedRequests []PipelineRequest

	// validatedRequests tracks all Validate() calls
	validatedRequests []PipelineRequest

	// executeResult is the result to return from Execute()
	// If nil, generates a default success result
	executeResult *PipelineResult

	// executeErr is the error to return from Execute()
	executeErr error

	// validateErr is the error to return from Validate()
	validateErr error

	// executeDelay simulates execution time
	executeDelay time.Duration

	// validateDelay simulates validation time
	validateDelay time.Duration

	// outputEvents are events to send to OutputCallback
	outputEvents []PipelineEvent
}

// NewMockExecutor creates a new mock executor with default settings.
func NewMockExecutor() *MockExecutor {
	return &MockExecutor{
		name:              "mock",
		executedRequests:  make([]PipelineRequest, 0),
		validatedRequests: make([]PipelineRequest, 0),
	}
}

// NewMockExecutorWithName creates a mock executor with a custom name.
func NewMockExecutorWithName(name string) *MockExecutor {
	mock := NewMockExecutor()
	mock.name = name
	return mock
}

// Execute implements PipelineExecutor.Execute.
func (m *MockExecutor) Execute(ctx context.Context, req PipelineRequest) (*PipelineResult, error) {
	m.mu.Lock()
	m.executedRequests = append(m.executedRequests, req)
	executeErr := m.executeErr
	executeResult := m.executeResult
	executeDelay := m.executeDelay
	outputEvents := m.outputEvents
	m.mu.Unlock()

	// Simulate execution delay
	if executeDelay > 0 {
		select {
		case <-time.After(executeDelay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	// Check for context cancellation
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	// Send output events if callback provided
	if req.OutputCallback != nil && len(outputEvents) > 0 {
		for _, event := range outputEvents {
			req.OutputCallback(event)
		}
	}

	// Return injected error if set
	if executeErr != nil {
		return nil, executeErr
	}

	// Return injected result if set
	if executeResult != nil {
		// Set executor name
		result := *executeResult
		result.ExecutorName = m.name
		return &result, nil
	}

	// Generate default success result
	now := time.Now()
	result := &PipelineResult{
		Success:      true,
		ExitCode:     0,
		Output:       "Mock execution successful\n",
		Steps:        []StepResult{},
		Duration:     executeDelay,
		StartedAt:    now.Add(-executeDelay),
		FinishedAt:   now,
		ExecutorName: m.name,
	}

	return result, nil
}

// Validate implements PipelineExecutor.Validate.
func (m *MockExecutor) Validate(ctx context.Context, req PipelineRequest) error {
	m.mu.Lock()
	m.validatedRequests = append(m.validatedRequests, req)
	validateErr := m.validateErr
	validateDelay := m.validateDelay
	m.mu.Unlock()

	// Simulate validation delay
	if validateDelay > 0 {
		select {
		case <-time.After(validateDelay):
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	// Check for context cancellation
	if ctx.Err() != nil {
		return ctx.Err()
	}

	// Return injected error if set
	if validateErr != nil {
		return validateErr
	}

	return nil
}

// Name implements PipelineExecutor.Name.
func (m *MockExecutor) Name() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.name
}

// Test helper methods

// SetExecuteResult configures the result to return from Execute().
// If nil, a default success result is generated.
func (m *MockExecutor) SetExecuteResult(result *PipelineResult) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.executeResult = result
}

// SetExecuteError configures the error to return from Execute().
func (m *MockExecutor) SetExecuteError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.executeErr = err
}

// SetValidateError configures the error to return from Validate().
func (m *MockExecutor) SetValidateError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.validateErr = err
}

// SetExecuteDelay configures a delay to simulate execution time.
func (m *MockExecutor) SetExecuteDelay(delay time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.executeDelay = delay
}

// SetValidateDelay configures a delay to simulate validation time.
func (m *MockExecutor) SetValidateDelay(delay time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.validateDelay = delay
}

// SetOutputEvents configures events to send to OutputCallback.
func (m *MockExecutor) SetOutputEvents(events []PipelineEvent) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.outputEvents = events
}

// GetExecutedRequests returns all requests passed to Execute().
// Returns a copy to prevent concurrent modification.
func (m *MockExecutor) GetExecutedRequests() []PipelineRequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	requests := make([]PipelineRequest, len(m.executedRequests))
	copy(requests, m.executedRequests)
	return requests
}

// GetValidatedRequests returns all requests passed to Validate().
// Returns a copy to prevent concurrent modification.
func (m *MockExecutor) GetValidatedRequests() []PipelineRequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	requests := make([]PipelineRequest, len(m.validatedRequests))
	copy(requests, m.validatedRequests)
	return requests
}

// GetExecuteCount returns the number of times Execute() was called.
func (m *MockExecutor) GetExecuteCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.executedRequests)
}

// GetValidateCount returns the number of times Validate() was called.
func (m *MockExecutor) GetValidateCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.validatedRequests)
}

// Reset clears all tracked state.
func (m *MockExecutor) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.executedRequests = make([]PipelineRequest, 0)
	m.validatedRequests = make([]PipelineRequest, 0)
	m.executeResult = nil
	m.executeErr = nil
	m.validateErr = nil
	m.executeDelay = 0
	m.validateDelay = 0
	m.outputEvents = nil
}
