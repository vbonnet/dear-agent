package activities

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// TestLaunchAgentActivity_ValidInput tests launching with valid parameters
func TestLaunchAgentActivity_ValidInput(t *testing.T) {
	// Create temporary working directory
	tempDir := t.TempDir()

	_ = LaunchAgentInput{
		SessionName: "test-session",
		SessionID:   "test-123",
		WorkDir:     tempDir,
		AgentType:   "claude",
		Environment: map[string]string{
			"TEST_VAR": "test_value",
		},
	}

	// Mock the agent command with /bin/true (always succeeds)
	// In real tests, you'd use a mock or stub
	t.Skip("Skipping actual process launch - requires mocking")
}

// TestLaunchAgentActivity_InvalidWorkDir tests with non-existent working directory
func TestLaunchAgentActivity_InvalidWorkDir(t *testing.T) {
	input := LaunchAgentInput{
		SessionName: "test-session",
		SessionID:   "test-123",
		WorkDir:     "/nonexistent/directory",
		AgentType:   "claude",
	}

	_, err := LaunchAgentActivity(context.Background(), input)
	if err == nil {
		t.Error("Expected error for non-existent working directory")
	}

	if !strings.Contains(err.Error(), "working directory does not exist") {
		t.Errorf("Expected 'working directory does not exist' error, got: %v", err)
	}
}

// TestLaunchAgentActivity_EmptySessionName tests validation
func TestLaunchAgentActivity_EmptySessionName(t *testing.T) {
	input := LaunchAgentInput{
		SessionName: "",
		WorkDir:     "/tmp",
		AgentType:   "claude",
	}

	_, err := LaunchAgentActivity(context.Background(), input)
	if err == nil {
		t.Error("Expected error for empty session name")
	}

	if !strings.Contains(err.Error(), "session name cannot be empty") {
		t.Errorf("Expected 'session name cannot be empty' error, got: %v", err)
	}
}

// TestLaunchAgentActivity_UnsupportedAgentType tests invalid agent type
func TestLaunchAgentActivity_UnsupportedAgentType(t *testing.T) {
	tempDir := t.TempDir()

	input := LaunchAgentInput{
		SessionName: "test-session",
		WorkDir:     tempDir,
		AgentType:   "invalid-agent",
	}

	_, err := LaunchAgentActivity(context.Background(), input)
	if err == nil {
		t.Error("Expected error for unsupported agent type")
	}

	if !strings.Contains(err.Error(), "unsupported agent type") {
		t.Errorf("Expected 'unsupported agent type' error, got: %v", err)
	}
}

// TestGetSessionDataDir tests session directory path generation
func TestGetSessionDataDir(t *testing.T) {
	sessionID := "test-session-123"
	dir, err := GetSessionDataDir(sessionID)
	if err != nil {
		t.Fatalf("GetSessionDataDir failed: %v", err)
	}

	if !strings.Contains(dir, ".agm/sessions/test-session-123") {
		t.Errorf("Expected path to contain '.agm/sessions/test-session-123', got: %s", dir)
	}
}

// TestEnsureSessionDir tests session directory creation
func TestEnsureSessionDir(t *testing.T) {
	sessionID := "test-session-" + time.Now().Format("20060102150405")
	dir, err := EnsureSessionDir(sessionID)
	if err != nil {
		t.Fatalf("EnsureSessionDir failed: %v", err)
	}

	// Verify directory was created
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Errorf("Session directory was not created: %s", dir)
	}

	// Cleanup
	defer os.RemoveAll(filepath.Dir(dir))
}

// TestMonitorOutputActivity_NoEscalations tests monitoring with clean output
func TestMonitorOutputActivity_NoEscalations(t *testing.T) {
	input := MonitorOutputInput{
		SessionID: "test-session",
		PID:       12345,
		Reader:    strings.NewReader("Normal output line 1\nNormal output line 2\n"),
		Timeout:   5 * time.Second,
	}

	output, err := MonitorOutputActivity(context.Background(), input)
	if err != nil {
		t.Fatalf("MonitorOutputActivity failed: %v", err)
	}

	if output.LinesRead != 2 {
		t.Errorf("Expected 2 lines read, got %d", output.LinesRead)
	}

	if len(output.Escalations) != 0 {
		t.Errorf("Expected 0 escalations, got %d", len(output.Escalations))
	}

	if !output.Completed {
		t.Error("Expected monitoring to complete")
	}
}

// TestMonitorOutputActivity_ErrorPattern tests error detection
func TestMonitorOutputActivity_ErrorPattern(t *testing.T) {
	input := MonitorOutputInput{
		SessionID: "test-session",
		PID:       12345,
		Reader:    strings.NewReader("Normal line\nError: something went wrong\nAnother line\n"),
		Timeout:   5 * time.Second,
	}

	output, err := MonitorOutputActivity(context.Background(), input)
	if err != nil {
		t.Fatalf("MonitorOutputActivity failed: %v", err)
	}

	if len(output.Escalations) != 1 {
		t.Errorf("Expected 1 escalation, got %d", len(output.Escalations))
	}

	if output.Escalations[0].Type != "error" {
		t.Errorf("Expected escalation type 'error', got '%s'", output.Escalations[0].Type)
	}
}

// TestMonitorOutputActivity_PromptPattern tests prompt detection
func TestMonitorOutputActivity_PromptPattern(t *testing.T) {
	input := MonitorOutputInput{
		SessionID: "test-session",
		PID:       12345,
		Reader:    strings.NewReader("Do you want to continue? (yes/no): "),
		Timeout:   5 * time.Second,
	}

	output, err := MonitorOutputActivity(context.Background(), input)
	if err != nil {
		t.Fatalf("MonitorOutputActivity failed: %v", err)
	}

	if len(output.Escalations) != 1 {
		t.Errorf("Expected 1 escalation, got %d", len(output.Escalations))
	}

	if output.Escalations[0].Type != "prompt" {
		t.Errorf("Expected escalation type 'prompt', got '%s'", output.Escalations[0].Type)
	}
}

// TestMonitorOutputActivity_MultipleEscalations tests multiple pattern matches
func TestMonitorOutputActivity_MultipleEscalations(t *testing.T) {
	input := MonitorOutputInput{
		SessionID: "test-session",
		PID:       12345,
		Reader: strings.NewReader(
			"Warning: deprecated feature\n" +
				"Error: connection failed\n" +
				"Continue? (y/n): \n",
		),
		Timeout: 5 * time.Second,
	}

	output, err := MonitorOutputActivity(context.Background(), input)
	if err != nil {
		t.Fatalf("MonitorOutputActivity failed: %v", err)
	}

	if len(output.Escalations) != 3 {
		t.Errorf("Expected 3 escalations, got %d", len(output.Escalations))
	}
}

// TestMonitorOutputActivity_MaxLines tests line limit
func TestMonitorOutputActivity_MaxLines(t *testing.T) {
	// Generate 150 lines
	var sb strings.Builder
	for i := 0; i < 150; i++ {
		sb.WriteString("Line ")
		sb.WriteString(string(rune('0' + (i % 10))))
		sb.WriteString("\n")
	}

	input := MonitorOutputInput{
		SessionID: "test-session",
		PID:       12345,
		Reader:    strings.NewReader(sb.String()),
		Timeout:   5 * time.Second,
		MaxLines:  100,
	}

	output, err := MonitorOutputActivity(context.Background(), input)
	if err != nil {
		t.Fatalf("MonitorOutputActivity failed: %v", err)
	}

	if output.LinesRead != 100 {
		t.Errorf("Expected 100 lines read (max), got %d", output.LinesRead)
	}

	if output.Completed {
		t.Error("Expected monitoring to not complete (hit max lines)")
	}
}

// TestMonitorOutputActivity_EmptySessionID tests validation
func TestMonitorOutputActivity_EmptySessionID(t *testing.T) {
	input := MonitorOutputInput{
		SessionID: "",
		Reader:    strings.NewReader("test"),
	}

	_, err := MonitorOutputActivity(context.Background(), input)
	if err == nil {
		t.Error("Expected error for empty session ID")
	}
}

// TestMonitorOutputActivity_NilReader tests validation
func TestMonitorOutputActivity_NilReader(t *testing.T) {
	input := MonitorOutputInput{
		SessionID: "test-session",
		Reader:    nil,
	}

	_, err := MonitorOutputActivity(context.Background(), input)
	if err == nil {
		t.Error("Expected error for nil reader")
	}
}

// TestDetectEscalation tests single-line escalation detection
func TestDetectEscalation(t *testing.T) {
	testCases := []struct {
		line         string
		expectEvent  bool
		expectedType string
	}{
		{"Normal line", false, ""},
		{"Error: something failed", true, "error"},
		{"Warning: deprecated", true, "warning"},
		{"Continue? (yes/no)", true, "prompt"},
		{"Fatal: crash", true, "error"},
		{"Permission denied", true, "error"},
	}

	for _, tc := range testCases {
		event := DetectEscalation(tc.line)
		if tc.expectEvent {
			if event == nil {
				t.Errorf("Expected escalation for line '%s', got nil", tc.line)
			} else if event.Type != tc.expectedType {
				t.Errorf("For line '%s', expected type '%s', got '%s'", tc.line, tc.expectedType, event.Type)
			}
		} else {
			if event != nil {
				t.Errorf("Expected no escalation for line '%s', got %v", tc.line, event)
			}
		}
	}
}

// TestCheckpointStateActivity_Create tests creating a checkpoint
func TestCheckpointStateActivity_Create(t *testing.T) {
	sessionID := "test-checkpoint-" + time.Now().Format("20060102150405")

	input := CheckpointStateInput{
		SessionID:     sessionID,
		SessionName:   "test-session",
		WorkflowID:    "workflow-123",
		WorkflowRunID: "run-456",
		State: map[string]interface{}{
			"step":    1,
			"status":  "running",
			"counter": 42,
		},
		Metadata: map[string]string{
			"agent": "claude",
		},
		CheckpointType: "periodic",
	}

	output, err := CheckpointStateActivity(context.Background(), input)
	if err != nil {
		t.Fatalf("CheckpointStateActivity failed: %v", err)
	}

	if !output.Success {
		t.Error("Expected checkpoint to succeed")
	}

	if output.SessionID != sessionID {
		t.Errorf("Expected session ID '%s', got '%s'", sessionID, output.SessionID)
	}

	// Verify checkpoint file was created
	if _, err := os.Stat(output.CheckpointPath); os.IsNotExist(err) {
		t.Errorf("Checkpoint file was not created: %s", output.CheckpointPath)
	}

	// Cleanup
	defer os.RemoveAll(filepath.Dir(output.CheckpointPath))
}

// TestCheckpointStateActivity_LoadCheckpoint tests loading a checkpoint
func TestCheckpointStateActivity_LoadCheckpoint(t *testing.T) {
	sessionID := "test-load-" + time.Now().Format("20060102150405")

	// Create a checkpoint
	input := CheckpointStateInput{
		SessionID:     sessionID,
		SessionName:   "test-session",
		WorkflowID:    "workflow-123",
		WorkflowRunID: "run-456",
		State: map[string]interface{}{
			"test_key": "test_value",
		},
	}

	_, err := CheckpointStateActivity(context.Background(), input)
	if err != nil {
		t.Fatalf("Failed to create checkpoint: %v", err)
	}

	// Load the checkpoint
	checkpoint, err := LoadCheckpointActivity(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("LoadCheckpointActivity failed: %v", err)
	}

	if checkpoint.SessionID != sessionID {
		t.Errorf("Expected session ID '%s', got '%s'", sessionID, checkpoint.SessionID)
	}

	if checkpoint.WorkflowID != "workflow-123" {
		t.Errorf("Expected workflow ID 'workflow-123', got '%s'", checkpoint.WorkflowID)
	}

	if checkpoint.State["test_key"] != "test_value" {
		t.Errorf("Expected state['test_key'] = 'test_value', got %v", checkpoint.State["test_key"])
	}

	// Cleanup
	sessionDir, _ := GetSessionDataDir(sessionID)
	defer os.RemoveAll(sessionDir)
}

// TestCheckpointStateActivity_EmptySessionID tests validation
func TestCheckpointStateActivity_EmptySessionID(t *testing.T) {
	input := CheckpointStateInput{
		SessionID:  "",
		WorkflowID: "workflow-123",
	}

	_, err := CheckpointStateActivity(context.Background(), input)
	if err == nil {
		t.Error("Expected error for empty session ID")
	}
}

// TestCheckpointStateActivity_EmptyWorkflowID tests validation
func TestCheckpointStateActivity_EmptyWorkflowID(t *testing.T) {
	input := CheckpointStateInput{
		SessionID:  "test-session",
		WorkflowID: "",
	}

	_, err := CheckpointStateActivity(context.Background(), input)
	if err == nil {
		t.Error("Expected error for empty workflow ID")
	}
}

// TestTerminateSessionActivity_ProcessTermination tests process killing
func TestTerminateSessionActivity_ProcessTermination(t *testing.T) {
	sessionID := "test-terminate-" + time.Now().Format("20060102150405")

	// Ensure session dir exists
	sessionDir, err := EnsureSessionDir(sessionID)
	if err != nil {
		t.Fatalf("Failed to create session dir: %v", err)
	}
	defer os.RemoveAll(sessionDir)

	// Start a sleep process to kill
	cmd := "sleep"
	args := []string{"60"}
	process, err := os.StartProcess(
		"/bin/sleep",
		append([]string{cmd}, args...),
		&os.ProcAttr{
			Files: []*os.File{os.Stdin, os.Stdout, os.Stderr},
		},
	)
	if err != nil {
		t.Fatalf("Failed to start test process: %v", err)
	}

	input := TerminateSessionInput{
		SessionID:    sessionID,
		SessionName:  "test-session",
		PID:          process.Pid,
		GracePeriod:  2 * time.Second,
		ForceKill:    true,
		CleanupFiles: false,
	}

	output, err := TerminateSessionActivity(context.Background(), input)
	if err != nil {
		t.Fatalf("TerminateSessionActivity failed: %v", err)
	}

	if !output.ProcessKilled {
		t.Error("Expected process to be killed")
	}

	// Verify process is dead
	err = process.Signal(syscall.Signal(0))
	if err == nil {
		t.Error("Process should be dead but is still running")
		process.Kill()
	}
}

// TestTerminateSessionActivity_InvalidPID tests validation
func TestTerminateSessionActivity_InvalidPID(t *testing.T) {
	input := TerminateSessionInput{
		SessionID: "test-session",
		PID:       -1,
	}

	_, err := TerminateSessionActivity(context.Background(), input)
	if err == nil {
		t.Error("Expected error for invalid PID")
	}
}

// TestTerminateSessionActivity_EmptySessionID tests validation
func TestTerminateSessionActivity_EmptySessionID(t *testing.T) {
	input := TerminateSessionInput{
		SessionID: "",
		PID:       12345,
	}

	_, err := TerminateSessionActivity(context.Background(), input)
	if err == nil {
		t.Error("Expected error for empty session ID")
	}
}

// TestCheckProcessActivity tests process existence checking
func TestCheckProcessActivity(t *testing.T) {
	// Test with current process (should exist)
	exists, err := CheckProcessActivity(context.Background(), os.Getpid())
	if err != nil {
		t.Fatalf("CheckProcessActivity failed: %v", err)
	}

	if !exists {
		t.Error("Expected current process to exist")
	}

	// Test with invalid PID (should not exist)
	exists, err = CheckProcessActivity(context.Background(), 999999)
	if err != nil {
		t.Fatalf("CheckProcessActivity failed: %v", err)
	}

	if exists {
		t.Error("Expected non-existent process to return false")
	}
}

// TestCleanupSessionActivity tests session cleanup
func TestCleanupSessionActivity(t *testing.T) {
	sessionID := "test-cleanup-" + time.Now().Format("20060102150405")

	// Create session directory
	sessionDir, err := EnsureSessionDir(sessionID)
	if err != nil {
		t.Fatalf("Failed to create session dir: %v", err)
	}

	// Create some test files
	testFile := filepath.Join(sessionDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Cleanup
	err = CleanupSessionActivity(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("CleanupSessionActivity failed: %v", err)
	}

	// Verify directory is gone
	if _, err := os.Stat(sessionDir); !os.IsNotExist(err) {
		t.Error("Expected session directory to be removed")
	}
}

// TestFormatEscalations tests escalation formatting
func TestFormatEscalations(t *testing.T) {
	escalations := []EscalationEvent{
		{
			Type:        "error",
			Description: "Test error",
			Line:        "Error: test",
			LineNumber:  10,
		},
		{
			Type:        "warning",
			Description: "Test warning",
			Line:        "Warning: test",
			LineNumber:  20,
		},
	}

	formatted := FormatEscalations(escalations)
	if !strings.Contains(formatted, "2 escalation") {
		t.Error("Expected formatted string to mention 2 escalations")
	}

	if !strings.Contains(formatted, "error") {
		t.Error("Expected formatted string to mention error type")
	}

	if !strings.Contains(formatted, "Line 10") {
		t.Error("Expected formatted string to mention line 10")
	}

	// Test empty escalations
	emptyFormatted := FormatEscalations([]EscalationEvent{})
	if !strings.Contains(emptyFormatted, "No escalations") {
		t.Error("Expected 'No escalations' message for empty list")
	}
}

// MockReader is a reader that simulates slow output
type MockReader struct {
	lines    []string
	current  int
	delay    time.Duration
	readFunc func() (string, error)
}

func (m *MockReader) Read(p []byte) (n int, err error) {
	if m.current >= len(m.lines) {
		return 0, io.EOF
	}

	if m.delay > 0 {
		time.Sleep(m.delay)
	}

	line := m.lines[m.current] + "\n"
	m.current++

	copy(p, []byte(line))
	return len(line), nil
}

// TestMonitorOutputActivity_SlowReader tests timeout behavior
func TestMonitorOutputActivity_SlowReader(t *testing.T) {
	reader := &MockReader{
		lines: []string{"Line 1", "Line 2", "Line 3"},
		delay: 100 * time.Millisecond,
	}

	input := MonitorOutputInput{
		SessionID: "test-session",
		PID:       12345,
		Reader:    reader,
		Timeout:   200 * time.Millisecond,
	}

	output, err := MonitorOutputActivity(context.Background(), input)
	if err != nil {
		t.Fatalf("MonitorOutputActivity failed: %v", err)
	}

	// Should read some lines before timeout
	if output.LinesRead < 1 {
		t.Error("Expected to read at least 1 line before timeout")
	}
}
