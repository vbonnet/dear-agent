package integration

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"

	"github.com/vbonnet/ai-tools/claude-session-manager/internal/temporal/activities"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/temporal/workflows"
)

// MCPServiceWorkflowTestSuite tests the MCP service workflow integration
type MCPServiceWorkflowTestSuite struct {
	suite.Suite
	temporalClient client.Client
	worker         worker.Worker
	taskQueue      string
}

// SetupSuite sets up the test suite (runs once before all tests)
func (s *MCPServiceWorkflowTestSuite) SetupSuite() {
	// Create Temporal client
	c, err := client.Dial(client.Options{
		HostPort: "localhost:7233",
	})
	if err != nil {
		s.T().Fatalf("Failed to create Temporal client: %v", err)
	}
	s.temporalClient = c

	// Create unique task queue for this test suite
	s.taskQueue = fmt.Sprintf("mcp-service-test-%d", time.Now().Unix())

	// Create worker
	s.worker = worker.New(s.temporalClient, s.taskQueue, worker.Options{})

	// Register workflow
	s.worker.RegisterWorkflow(workflows.MCPServiceWorkflow)

	// Register activities
	s.worker.RegisterActivity(activities.StartMCPActivity)
	s.worker.RegisterActivity(activities.StopMCPActivity)
	s.worker.RegisterActivity(activities.HealthCheckActivity)
	s.worker.RegisterActivity(activities.CheckMCPProcessActivity)
	s.worker.RegisterActivity(activities.CleanupMCPDataActivity)

	// Start worker
	err = s.worker.Start()
	if err != nil {
		s.T().Fatalf("Failed to start worker: %v", err)
	}
}

// TearDownSuite tears down the test suite (runs once after all tests)
func (s *MCPServiceWorkflowTestSuite) TearDownSuite() {
	if s.worker != nil {
		s.worker.Stop()
	}
	if s.temporalClient != nil {
		s.temporalClient.Close()
	}
}

// TestMCPServiceWorkflowTestSuite runs the test suite
func TestMCPServiceWorkflowTestSuite(t *testing.T) {
	suite.Run(t, new(MCPServiceWorkflowTestSuite))
}

// Test_MCPServiceWorkflow_BasicStartStop tests basic MCP service lifecycle
func (s *MCPServiceWorkflowTestSuite) Test_MCPServiceWorkflow_BasicStartStop() {
	ctx := context.Background()

	// Get home directory for server path
	homeDir, err := os.UserHomeDir()
	require.NoError(s.T(), err)

	serverPath := filepath.Join(homeDir, "src/mcp-global-sharing/packages/mcp-http-server/dist/server.js")

	// Check if server script exists, skip test if not
	if _, err := os.Stat(serverPath); os.IsNotExist(err) {
		s.T().Skip("MCP HTTP server not found, skipping test")
		return
	}

	// Create workflow options
	workflowID := fmt.Sprintf("mcp-service-test-%d", time.Now().Unix())
	workflowOptions := client.StartWorkflowOptions{
		ID:        workflowID,
		TaskQueue: s.taskQueue,
	}

	// Configure MCP service
	config := workflows.MCPServiceConfig{
		Name:       "googledocs-test",
		MCPCommand: "npx -y @modelcontextprotocol/server-googledocs",
		Port:       8001,
		ServerPath: serverPath,
	}

	// Start workflow
	workflowRun, err := s.temporalClient.ExecuteWorkflow(ctx, workflowOptions, workflows.MCPServiceWorkflow, config)
	require.NoError(s.T(), err)

	// Wait a moment for workflow to initialize
	time.Sleep(500 * time.Millisecond)

	// Send start signal
	err = s.temporalClient.SignalWorkflow(ctx, workflowID, "", workflows.SignalStartMCP, nil)
	require.NoError(s.T(), err)

	// Wait for MCP server to start
	time.Sleep(2 * time.Second)

	// Query workflow state
	resp, err := s.temporalClient.QueryWorkflow(ctx, workflowID, "", workflows.QueryMCPState)
	require.NoError(s.T(), err)

	var state workflows.MCPServiceWorkflowState
	err = resp.Get(&state)
	require.NoError(s.T(), err)

	// Verify server is running
	assert.Equal(s.T(), workflows.MCPStateRunning, state.State, "MCP service should be running")
	assert.Greater(s.T(), state.PID, 0, "PID should be set")
	assert.Equal(s.T(), 8001, state.Port, "Port should be 8001")
	assert.Equal(s.T(), "googledocs-test", state.Name, "Name should match")

	// Perform health check
	err = s.temporalClient.SignalWorkflow(ctx, workflowID, "", workflows.SignalHealthMCP, nil)
	require.NoError(s.T(), err)

	// Wait for health check to complete
	time.Sleep(2 * time.Second)

	// Query state again to verify health check
	resp, err = s.temporalClient.QueryWorkflow(ctx, workflowID, "", workflows.QueryMCPState)
	require.NoError(s.T(), err)

	err = resp.Get(&state)
	require.NoError(s.T(), err)

	// Note: Health check might fail if MCP server doesn't start properly
	// but we should at least verify the workflow is still running
	assert.Equal(s.T(), workflows.MCPStateRunning, state.State, "MCP service should still be running after health check")

	// Send stop signal
	err = s.temporalClient.SignalWorkflow(ctx, workflowID, "", workflows.SignalStopMCP, nil)
	require.NoError(s.T(), err)

	// Wait for workflow to complete
	err = workflowRun.Get(ctx, nil)
	assert.NoError(s.T(), err, "Workflow should complete successfully")

	// Cleanup test data
	ctx2 := context.Background()
	err = activities.CleanupMCPDataActivity(ctx2, "googledocs-test")
	assert.NoError(s.T(), err)
}

// Test_MCPServiceWorkflow_Restart tests MCP service restart
func (s *MCPServiceWorkflowTestSuite) Test_MCPServiceWorkflow_Restart() {
	ctx := context.Background()

	// Get home directory for server path
	homeDir, err := os.UserHomeDir()
	require.NoError(s.T(), err)

	serverPath := filepath.Join(homeDir, "src/mcp-global-sharing/packages/mcp-http-server/dist/server.js")

	// Check if server script exists, skip test if not
	if _, err := os.Stat(serverPath); os.IsNotExist(err) {
		s.T().Skip("MCP HTTP server not found, skipping test")
		return
	}

	// Create workflow options
	workflowID := fmt.Sprintf("mcp-service-restart-test-%d", time.Now().Unix())
	workflowOptions := client.StartWorkflowOptions{
		ID:        workflowID,
		TaskQueue: s.taskQueue,
	}

	// Configure MCP service
	config := workflows.MCPServiceConfig{
		Name:       "googledocs-restart-test",
		MCPCommand: "npx -y @modelcontextprotocol/server-googledocs",
		Port:       8002,
		ServerPath: serverPath,
	}

	// Start workflow
	workflowRun, err := s.temporalClient.ExecuteWorkflow(ctx, workflowOptions, workflows.MCPServiceWorkflow, config)
	require.NoError(s.T(), err)

	// Wait for workflow to initialize
	time.Sleep(500 * time.Millisecond)

	// Send start signal
	err = s.temporalClient.SignalWorkflow(ctx, workflowID, "", workflows.SignalStartMCP, nil)
	require.NoError(s.T(), err)

	// Wait for MCP server to start
	time.Sleep(2 * time.Second)

	// Query workflow state
	resp, err := s.temporalClient.QueryWorkflow(ctx, workflowID, "", workflows.QueryMCPState)
	require.NoError(s.T(), err)

	var state workflows.MCPServiceWorkflowState
	err = resp.Get(&state)
	require.NoError(s.T(), err)

	originalPID := state.PID
	assert.Greater(s.T(), originalPID, 0, "Original PID should be set")

	// Send restart signal
	err = s.temporalClient.SignalWorkflow(ctx, workflowID, "", workflows.SignalRestartMCP, nil)
	require.NoError(s.T(), err)

	// Wait for restart to complete
	time.Sleep(3 * time.Second)

	// Query state again
	resp, err = s.temporalClient.QueryWorkflow(ctx, workflowID, "", workflows.QueryMCPState)
	require.NoError(s.T(), err)

	err = resp.Get(&state)
	require.NoError(s.T(), err)

	// Verify server was restarted
	assert.Equal(s.T(), workflows.MCPStateRunning, state.State, "MCP service should be running after restart")
	assert.NotEqual(s.T(), originalPID, state.PID, "PID should have changed after restart")
	assert.Greater(s.T(), state.PID, 0, "New PID should be set")

	// Stop workflow
	err = s.temporalClient.SignalWorkflow(ctx, workflowID, "", workflows.SignalStopMCP, nil)
	require.NoError(s.T(), err)

	// Wait for workflow to complete
	err = workflowRun.Get(ctx, nil)
	assert.NoError(s.T(), err, "Workflow should complete successfully")

	// Cleanup test data
	ctx2 := context.Background()
	err = activities.CleanupMCPDataActivity(ctx2, "googledocs-restart-test")
	assert.NoError(s.T(), err)
}

// Test_MCPServiceWorkflow_StateQueries tests workflow state queries
func (s *MCPServiceWorkflowTestSuite) Test_MCPServiceWorkflow_StateQueries() {
	ctx := context.Background()

	// Get home directory for server path
	homeDir, err := os.UserHomeDir()
	require.NoError(s.T(), err)

	serverPath := filepath.Join(homeDir, "src/mcp-global-sharing/packages/mcp-http-server/dist/server.js")

	// Check if server script exists, skip test if not
	if _, err := os.Stat(serverPath); os.IsNotExist(err) {
		s.T().Skip("MCP HTTP server not found, skipping test")
		return
	}

	// Create workflow options
	workflowID := fmt.Sprintf("mcp-service-query-test-%d", time.Now().Unix())
	workflowOptions := client.StartWorkflowOptions{
		ID:        workflowID,
		TaskQueue: s.taskQueue,
	}

	// Configure MCP service
	config := workflows.MCPServiceConfig{
		Name:       "googledocs-query-test",
		MCPCommand: "npx -y @modelcontextprotocol/server-googledocs",
		Port:       8003,
		ServerPath: serverPath,
	}

	// Start workflow
	workflowRun, err := s.temporalClient.ExecuteWorkflow(ctx, workflowOptions, workflows.MCPServiceWorkflow, config)
	require.NoError(s.T(), err)

	// Wait for workflow to initialize
	time.Sleep(500 * time.Millisecond)

	// Query initial state (should be stopped)
	resp, err := s.temporalClient.QueryWorkflow(ctx, workflowID, "", workflows.QueryMCPState)
	require.NoError(s.T(), err)

	var state workflows.MCPServiceWorkflowState
	err = resp.Get(&state)
	require.NoError(s.T(), err)

	assert.Equal(s.T(), workflows.MCPStateStopped, state.State, "Initial state should be stopped")
	assert.Equal(s.T(), 0, state.PID, "PID should be 0 when stopped")

	// Start the service
	err = s.temporalClient.SignalWorkflow(ctx, workflowID, "", workflows.SignalStartMCP, nil)
	require.NoError(s.T(), err)

	// Wait for start
	time.Sleep(2 * time.Second)

	// Query running state
	resp, err = s.temporalClient.QueryWorkflow(ctx, workflowID, "", workflows.QueryMCPState)
	require.NoError(s.T(), err)

	err = resp.Get(&state)
	require.NoError(s.T(), err)

	assert.Equal(s.T(), workflows.MCPStateRunning, state.State, "State should be running")
	assert.Greater(s.T(), state.PID, 0, "PID should be set")
	assert.Equal(s.T(), 8003, state.Port, "Port should match")
	assert.Equal(s.T(), "googledocs-query-test", state.Name, "Name should match")

	// Stop workflow
	err = s.temporalClient.SignalWorkflow(ctx, workflowID, "", workflows.SignalStopMCP, nil)
	require.NoError(s.T(), err)

	// Wait for workflow to complete
	err = workflowRun.Get(ctx, nil)
	assert.NoError(s.T(), err, "Workflow should complete successfully")

	// Cleanup test data
	ctx2 := context.Background()
	err = activities.CleanupMCPDataActivity(ctx2, "googledocs-query-test")
	assert.NoError(s.T(), err)
}

// Test_HealthCheckActivity tests the health check activity directly
func Test_HealthCheckActivity(t *testing.T) {
	ctx := context.Background()

	// Test with a URL that should fail
	input := workflows.HealthCheckInput{
		URL:     "http://localhost:9999/health",
		Timeout: 2 * time.Second,
	}

	result, err := activities.HealthCheckActivity(ctx, input)

	// Should fail or return unhealthy
	if err == nil {
		assert.Equal(t, "unhealthy", result.Status)
	}
}

// Test_StartMCPActivity_Validation tests input validation for StartMCPActivity
func Test_StartMCPActivity_Validation(t *testing.T) {
	ctx := context.Background()

	// Test empty name
	_, err := activities.StartMCPActivity(ctx, workflows.StartMCPInput{
		Name:       "",
		MCPCommand: "test",
		Port:       8000,
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "name cannot be empty")

	// Test empty command
	_, err = activities.StartMCPActivity(ctx, workflows.StartMCPInput{
		Name:       "test",
		MCPCommand: "",
		Port:       8000,
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "command cannot be empty")

	// Test zero port
	_, err = activities.StartMCPActivity(ctx, workflows.StartMCPInput{
		Name:       "test",
		MCPCommand: "test",
		Port:       0,
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "port cannot be zero")
}

// Test_StopMCPActivity_Validation tests input validation for StopMCPActivity
func Test_StopMCPActivity_Validation(t *testing.T) {
	ctx := context.Background()

	// Test empty name
	_, err := activities.StopMCPActivity(ctx, workflows.StopMCPInput{
		Name: "",
		PID:  123,
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "name cannot be empty")

	// Test invalid PID
	_, err = activities.StopMCPActivity(ctx, workflows.StopMCPInput{
		Name: "test",
		PID:  0,
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid PID")

	// Test negative PID
	_, err = activities.StopMCPActivity(ctx, workflows.StopMCPInput{
		Name: "test",
		PID:  -1,
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid PID")
}
