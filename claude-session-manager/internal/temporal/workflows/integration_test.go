package workflows

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
	"go.temporal.io/sdk/testsuite"
)

// WorkflowTestSuite is the test suite for Temporal workflows
type WorkflowTestSuite struct {
	suite.Suite
	testsuite.WorkflowTestSuite
	env *testsuite.TestWorkflowEnvironment
}

// SetupTest sets up the test environment before each test
func (s *WorkflowTestSuite) SetupTest() {
	s.env = s.NewTestWorkflowEnvironment()
}

// TearDownTest cleans up after each test
func (s *WorkflowTestSuite) TearDownTest() {
	s.env.AssertExpectations(s.T())
}

// TestWorkflowTestSuite runs the test suite
func TestWorkflowTestSuite(t *testing.T) {
	suite.Run(t, new(WorkflowTestSuite))
}

// Test_SessionWorkflow_BasicLifecycle tests basic session workflow lifecycle
func (s *WorkflowTestSuite) Test_SessionWorkflow_BasicLifecycle() {
	// Mock CreateSessionActivity
	s.env.OnActivity("CreateSessionActivity", mock.Anything, mock.Anything).Return("session created", nil)

	// Start workflow
	input := SessionWorkflowInput{
		SessionID:   "test-session-001",
		SessionName: "my-test-session",
		WorkingDir:  "/tmp/test",
		Agent:       "claude",
		Project:     "/tmp/test",
		Tags:        []string{"test"},
	}

	s.env.RegisterDelayedCallback(func() {
		// Query session state after workflow starts
		val, err := s.env.QueryWorkflow(QuerySessionState)
		s.NoError(err)

		var state SessionWorkflowState
		err = val.Get(&state)
		s.NoError(err)
		s.Equal(SessionStateActive, state.State)
		s.Equal("test-session-001", state.SessionID)
		s.Equal("my-test-session", state.SessionName)
	}, 100*time.Millisecond)

	// Send archive signal to complete workflow
	s.env.RegisterDelayedCallback(func() {
		s.env.SignalWorkflow(SignalArchive, nil)
	}, 200*time.Millisecond)

	// Mock ArchiveSessionActivity
	s.env.OnActivity("ArchiveSessionActivity", mock.Anything, "test-session-001").Return(nil)

	// Execute workflow
	s.env.ExecuteWorkflow(SessionWorkflow, input)

	s.True(s.env.IsWorkflowCompleted())
	s.NoError(s.env.GetWorkflowError())
}

// Test_SessionWorkflow_StateTransitions tests session state transitions
func (s *WorkflowTestSuite) Test_SessionWorkflow_StateTransitions() {
	// Mock activities
	s.env.OnActivity("CreateSessionActivity", mock.Anything, mock.Anything).Return("session created", nil)
	s.env.OnActivity("StopSessionActivity", mock.Anything, "test-session-002").Return(nil)
	s.env.OnActivity("ActivateSessionActivity", mock.Anything, "test-session-002").Return(nil)
	s.env.OnActivity("ArchiveSessionActivity", mock.Anything, "test-session-002").Return(nil)

	input := SessionWorkflowInput{
		SessionID:   "test-session-002",
		SessionName: "state-test-session",
		WorkingDir:  "/tmp/test",
		Agent:       "claude",
		Project:     "/tmp/test",
		Tags:        []string{},
	}

	// Test state transitions
	s.env.RegisterDelayedCallback(func() {
		// Send stop signal
		s.env.SignalWorkflow(SignalStop, nil)
	}, 100*time.Millisecond)

	s.env.RegisterDelayedCallback(func() {
		// Verify stopped state
		val, _ := s.env.QueryWorkflow(QuerySessionState)
		var state SessionWorkflowState
		val.Get(&state)
		s.Equal(SessionStateStopped, state.State)

		// Send activate signal
		s.env.SignalWorkflow(SignalActivate, nil)
	}, 200*time.Millisecond)

	s.env.RegisterDelayedCallback(func() {
		// Verify active state
		val, _ := s.env.QueryWorkflow(QuerySessionState)
		var state SessionWorkflowState
		val.Get(&state)
		s.Equal(SessionStateActive, state.State)

		// Send archive signal
		s.env.SignalWorkflow(SignalArchive, nil)
	}, 300*time.Millisecond)

	// Execute workflow
	s.env.ExecuteWorkflow(SessionWorkflow, input)

	s.True(s.env.IsWorkflowCompleted())
	s.NoError(s.env.GetWorkflowError())
}

// Test_SessionWorkflow_ClientAttachment tests client attach/detach
func (s *WorkflowTestSuite) Test_SessionWorkflow_ClientAttachment() {
	// Mock activities
	s.env.OnActivity("CreateSessionActivity", mock.Anything, mock.Anything).Return("session created", nil)
	s.env.OnActivity("ArchiveSessionActivity", mock.Anything, mock.Anything).Return(nil)

	input := SessionWorkflowInput{
		SessionID:   "test-session-003",
		SessionName: "client-test-session",
		WorkingDir:  "/tmp/test",
		Agent:       "claude",
		Project:     "/tmp/test",
	}

	s.env.RegisterDelayedCallback(func() {
		// Attach client
		s.env.SignalWorkflow(SignalAttach, nil)
	}, 50*time.Millisecond)

	s.env.RegisterDelayedCallback(func() {
		// Verify 1 client attached
		val, _ := s.env.QueryWorkflow(QuerySessionState)
		var state SessionWorkflowState
		val.Get(&state)
		s.Equal(1, state.AttachedClients)

		// Attach another client
		s.env.SignalWorkflow(SignalAttach, nil)
	}, 100*time.Millisecond)

	s.env.RegisterDelayedCallback(func() {
		// Verify 2 clients attached
		val, _ := s.env.QueryWorkflow(QuerySessionState)
		var state SessionWorkflowState
		val.Get(&state)
		s.Equal(2, state.AttachedClients)

		// Detach client
		s.env.SignalWorkflow(SignalDetach, nil)
	}, 150*time.Millisecond)

	s.env.RegisterDelayedCallback(func() {
		// Verify 1 client attached
		val, _ := s.env.QueryWorkflow(QuerySessionState)
		var state SessionWorkflowState
		val.Get(&state)
		s.Equal(1, state.AttachedClients)

		// Archive to complete
		s.env.SignalWorkflow(SignalArchive, nil)
	}, 200*time.Millisecond)

	// Execute workflow
	s.env.ExecuteWorkflow(SessionWorkflow, input)

	s.True(s.env.IsWorkflowCompleted())
	s.NoError(s.env.GetWorkflowError())
}

// Test_MonitorWorkflow_BasicMonitoring tests basic monitoring functionality
func (s *WorkflowTestSuite) Test_MonitorWorkflow_BasicMonitoring() {
	// Mock activities
	outputLines := []OutputLine{
		{Timestamp: time.Now(), Stream: "stdout", Content: "Normal output"},
		{Timestamp: time.Now(), Stream: "stderr", Content: "ERROR: Test error"},
	}
	s.env.OnActivity("FetchSessionOutputActivity", mock.Anything, "test-session-004").Return(outputLines, nil)
	s.env.OnActivity("LogEscalationActivity", mock.Anything, mock.Anything).Return(nil)
	s.env.OnActivity("SendNotificationActivity", mock.Anything, mock.Anything).Return(NotificationResult{
		Success:   true,
		Message:   "Notification sent",
		Timestamp: time.Now(),
	}, nil)
	s.env.OnActivity("StoreEscalationRecordActivity", mock.Anything, mock.Anything).Return(nil)

	input := MonitorWorkflowInput{
		SessionID:       "test-session-004",
		MonitorInterval: 100 * time.Millisecond,
		EscalationRules: []EscalationRule{
			{
				Name:        "Error Pattern",
				Patterns:    []string{"ERROR"},
				Severity:    "high",
				NotifyAfter: 1,
			},
		},
	}

	// Let monitoring run for a bit, then stop
	s.env.RegisterDelayedCallback(func() {
		s.env.SignalWorkflow(SignalStopMonitoring, nil)
	}, 500*time.Millisecond)

	// Execute workflow
	s.env.ExecuteWorkflow(MonitorWorkflow, input)

	s.True(s.env.IsWorkflowCompleted())
	s.NoError(s.env.GetWorkflowError())
}

// Test_MonitorWorkflow_EscalationThreshold tests escalation threshold logic
func (s *WorkflowTestSuite) Test_MonitorWorkflow_EscalationThreshold() {
	// Mock activity to return error output
	outputLines := []OutputLine{
		{Timestamp: time.Now(), Stream: "stderr", Content: "ERROR: First error"},
	}
	s.env.OnActivity("FetchSessionOutputActivity", mock.Anything, "test-session-005").Return(outputLines, nil)

	// We should NOT see escalation activities after first error (threshold is 2)
	input := MonitorWorkflowInput{
		SessionID:       "test-session-005",
		MonitorInterval: 100 * time.Millisecond,
		EscalationRules: []EscalationRule{
			{
				Name:        "Error Pattern",
				Patterns:    []string{"ERROR"},
				Severity:    "high",
				NotifyAfter: 2, // Threshold of 2
			},
		},
	}

	// Stop monitoring quickly before threshold is reached
	s.env.RegisterDelayedCallback(func() {
		s.env.SignalWorkflow(SignalStopMonitoring, nil)
	}, 150*time.Millisecond)

	// Execute workflow
	s.env.ExecuteWorkflow(MonitorWorkflow, input)

	s.True(s.env.IsWorkflowCompleted())
	s.NoError(s.env.GetWorkflowError())
}

// Test_MonitorWorkflow_StartStopResume tests starting and stopping monitoring
func (s *WorkflowTestSuite) Test_MonitorWorkflow_StartStopResume() {
	s.env.OnActivity("FetchSessionOutputActivity", mock.Anything, "test-session-006").Return([]OutputLine{}, nil)

	input := MonitorWorkflowInput{
		SessionID:       "test-session-006",
		MonitorInterval: 100 * time.Millisecond,
		EscalationRules: []EscalationRule{},
	}

	// Stop monitoring
	s.env.RegisterDelayedCallback(func() {
		s.env.SignalWorkflow(SignalStopMonitoring, nil)
	}, 100*time.Millisecond)

	// Verify stopped
	s.env.RegisterDelayedCallback(func() {
		val, _ := s.env.QueryWorkflow(QueryMonitorState)
		var state MonitorWorkflowState
		val.Get(&state)
		s.False(state.IsMonitoring)

		// Restart monitoring
		s.env.SignalWorkflow(SignalStartMonitoring, nil)
	}, 200*time.Millisecond)

	// Verify restarted
	s.env.RegisterDelayedCallback(func() {
		val, _ := s.env.QueryWorkflow(QueryMonitorState)
		var state MonitorWorkflowState
		val.Get(&state)
		s.True(state.IsMonitoring)

		// Final stop to exit workflow
		s.env.SignalWorkflow(SignalStopMonitoring, nil)
	}, 300*time.Millisecond)

	// Execute workflow
	s.env.ExecuteWorkflow(MonitorWorkflow, input)

	s.True(s.env.IsWorkflowCompleted())
	s.NoError(s.env.GetWorkflowError())
}

// Test_EscalationWorkflow_BasicNotification tests basic escalation workflow
func (s *WorkflowTestSuite) Test_EscalationWorkflow_BasicNotification() {
	// Mock activities
	s.env.OnActivity("LogEscalationActivity", mock.Anything, mock.Anything).Return(nil)
	s.env.OnActivity("SendNotificationActivity", mock.Anything, mock.Anything).Return(NotificationResult{
		Success:   true,
		Message:   "Notification sent",
		Timestamp: time.Now(),
	}, nil)
	s.env.OnActivity("StoreEscalationRecordActivity", mock.Anything, mock.Anything).Return(nil)

	input := EscalationWorkflowInput{
		SessionID:   "test-session-007",
		RuleName:    "Test Rule",
		Severity:    "high",
		MatchedText: "ERROR: Test error",
		Timestamp:   time.Now(),
		NotificationChannels: []NotificationChannel{
			{Type: "log", Target: "escalation-high", Priority: 0},
		},
	}

	// Execute workflow
	s.env.ExecuteWorkflow(EscalationWorkflow, input)

	s.True(s.env.IsWorkflowCompleted())
	s.NoError(s.env.GetWorkflowError())

	var result string
	err := s.env.GetWorkflowResult(&result)
	s.NoError(err)
	s.Contains(result, "1 notifications sent")
}

// Test_EscalationWorkflow_CriticalWithFallback tests critical escalation with fallback
func (s *WorkflowTestSuite) Test_EscalationWorkflow_CriticalWithFallback() {
	// Mock activities - primary notification fails
	s.env.OnActivity("LogEscalationActivity", mock.Anything, mock.Anything).Return(nil)

	// First call fails
	s.env.OnActivity("SendNotificationActivity", mock.Anything, mock.MatchedBy(func(input NotificationInput) bool {
		return input.Severity == "critical"
	})).Return(NotificationResult{
		Success: false,
		Message: "Failed to send",
	}, fmt.Errorf("notification failed"))

	// Fallback notification succeeds
	s.env.OnActivity("SendNotificationActivity", mock.Anything, mock.MatchedBy(func(input NotificationInput) bool {
		return input.Severity == "critical-fallback"
	})).Return(NotificationResult{
		Success:   true,
		Message:   "Fallback sent",
		Timestamp: time.Now(),
	}, nil)

	s.env.OnActivity("StoreEscalationRecordActivity", mock.Anything, mock.Anything).Return(nil)

	input := EscalationWorkflowInput{
		SessionID:   "test-session-008",
		RuleName:    "Critical Rule",
		Severity:    "critical",
		MatchedText: "CRITICAL ERROR",
		Timestamp:   time.Now(),
		NotificationChannels: []NotificationChannel{
			{Type: "webhook", Target: "http://example.com", Priority: 0},
		},
	}

	// Execute workflow
	s.env.ExecuteWorkflow(EscalationWorkflow, input)

	s.True(s.env.IsWorkflowCompleted())
	s.NoError(s.env.GetWorkflowError())
}

// Test_EscalationWorkflow_AllNotificationsFail tests complete notification failure
func (s *WorkflowTestSuite) Test_EscalationWorkflow_AllNotificationsFail() {
	// Mock activities - all notifications fail
	s.env.OnActivity("LogEscalationActivity", mock.Anything, mock.Anything).Return(nil)
	s.env.OnActivity("SendNotificationActivity", mock.Anything, mock.Anything).Return(
		NotificationResult{Success: false, Message: "Failed"},
		fmt.Errorf("all notifications failed"),
	)
	s.env.OnActivity("StoreEscalationRecordActivity", mock.Anything, mock.Anything).Return(nil)

	input := EscalationWorkflowInput{
		SessionID:   "test-session-009",
		RuleName:    "Test Rule",
		Severity:    "medium", // Not critical, so no fallback
		MatchedText: "ERROR",
		Timestamp:   time.Now(),
		NotificationChannels: []NotificationChannel{
			{Type: "log", Target: "test", Priority: 0},
		},
	}

	// Execute workflow
	s.env.ExecuteWorkflow(EscalationWorkflow, input)

	s.True(s.env.IsWorkflowCompleted())
	// Should have error because all notifications failed
	s.Error(s.env.GetWorkflowError())
}

// Test_SessionWorkflow_ActivityFailure tests handling of activity failures
func (s *WorkflowTestSuite) Test_SessionWorkflow_ActivityFailure() {
	// Mock CreateSessionActivity to fail
	s.env.OnActivity("CreateSessionActivity", mock.Anything, mock.Anything).Return(
		"", fmt.Errorf("failed to create session"),
	)

	input := SessionWorkflowInput{
		SessionID:   "test-session-010",
		SessionName: "fail-test-session",
		WorkingDir:  "/tmp/test",
		Agent:       "claude",
		Project:     "/tmp/test",
	}

	// Execute workflow
	s.env.ExecuteWorkflow(SessionWorkflow, input)

	s.True(s.env.IsWorkflowCompleted())
	// Should fail due to activity failure
	s.Error(s.env.GetWorkflowError())
}

// Test_MonitorWorkflow_ChildWorkflow tests child workflow execution
func (s *WorkflowTestSuite) Test_MonitorWorkflow_ChildWorkflow() {
	// Mock activities
	outputLines := []OutputLine{
		{Timestamp: time.Now(), Stream: "stderr", Content: "ERROR: Critical issue"},
	}
	s.env.OnActivity("FetchSessionOutputActivity", mock.Anything, "test-session-011").Return(outputLines, nil)

	// Register child workflow (EscalationWorkflow)
	s.env.RegisterWorkflow(EscalationWorkflow)
	s.env.OnActivity("LogEscalationActivity", mock.Anything, mock.Anything).Return(nil)
	s.env.OnActivity("SendNotificationActivity", mock.Anything, mock.Anything).Return(NotificationResult{
		Success:   true,
		Message:   "Sent",
		Timestamp: time.Now(),
	}, nil)
	s.env.OnActivity("StoreEscalationRecordActivity", mock.Anything, mock.Anything).Return(nil)

	input := MonitorWorkflowInput{
		SessionID:       "test-session-011",
		MonitorInterval: 100 * time.Millisecond,
		EscalationRules: []EscalationRule{
			{
				Name:        "Error Pattern",
				Patterns:    []string{"ERROR"},
				Severity:    "high",
				NotifyAfter: 1,
			},
		},
	}

	// Stop monitoring after escalation
	s.env.RegisterDelayedCallback(func() {
		s.env.SignalWorkflow(SignalStopMonitoring, nil)
	}, 300*time.Millisecond)

	// Execute workflow
	s.env.ExecuteWorkflow(MonitorWorkflow, input)

	s.True(s.env.IsWorkflowCompleted())
	s.NoError(s.env.GetWorkflowError())
}

// Test_parseOutputForEscalations tests the pattern matching function
func Test_parseOutputForEscalations(t *testing.T) {
	lines := []OutputLine{
		{Timestamp: time.Now(), Stream: "stdout", Content: "Normal output"},
		{Timestamp: time.Now(), Stream: "stderr", Content: "ERROR: Something went wrong"},
		{Timestamp: time.Now(), Stream: "stderr", Content: "WARNING: Low memory"},
		{Timestamp: time.Now(), Stream: "stdout", Content: "ERROR: Another error"},
	}

	rules := []EscalationRule{
		{
			Name:        "Error Pattern",
			Patterns:    []string{"ERROR"},
			Severity:    "high",
			NotifyAfter: 1,
		},
		{
			Name:        "Warning Pattern",
			Patterns:    []string{"WARNING"},
			Severity:    "medium",
			NotifyAfter: 1,
		},
	}

	matches := parseOutputForEscalations(lines, rules, "test-session")

	assert.Len(t, matches, 3)
	assert.Equal(t, "Error Pattern", matches[0].RuleName)
	assert.Equal(t, "Warning Pattern", matches[1].RuleName)
	assert.Equal(t, "Error Pattern", matches[2].RuleName)
	assert.Contains(t, matches[0].MatchedText, "ERROR")
	assert.Contains(t, matches[1].MatchedText, "WARNING")
}

// Test_getDefaultNotificationChannels tests default notification channel selection
func Test_getDefaultNotificationChannels(t *testing.T) {
	tests := []struct {
		severity       string
		expectedCount  int
		expectedTypes  []string
	}{
		{"critical", 2, []string{"log", "webhook"}},
		{"high", 1, []string{"log"}},
		{"medium", 1, []string{"log"}},
		{"low", 1, []string{"log"}},
		{"unknown", 1, []string{"log"}},
	}

	for _, tt := range tests {
		t.Run(tt.severity, func(t *testing.T) {
			channels := getDefaultNotificationChannels(tt.severity)
			assert.Len(t, channels, tt.expectedCount)

			for i, expectedType := range tt.expectedTypes {
				assert.Equal(t, expectedType, channels[i].Type)
			}
		})
	}
}

// Test_getRetryPolicyForSeverity tests retry policy generation
func Test_getRetryPolicyForSeverity(t *testing.T) {
	tests := []struct {
		severity        string
		expectedMaxAttempts int
	}{
		{"critical", 5},
		{"high", 3},
		{"medium", 2},
		{"low", 2},
		{"unknown", 1},
	}

	for _, tt := range tests {
		t.Run(tt.severity, func(t *testing.T) {
			policy := getRetryPolicyForSeverity(tt.severity)
			assert.Equal(t, tt.expectedMaxAttempts, policy.MaximumAttempts)
		})
	}
}

// Test_SessionState_String tests SessionState string representation
func Test_SessionState_String(t *testing.T) {
	states := []SessionState{
		SessionStateCreated,
		SessionStateActive,
		SessionStateStopped,
		SessionStateArchived,
	}

	for _, state := range states {
		assert.NotEmpty(t, string(state))
	}
}
