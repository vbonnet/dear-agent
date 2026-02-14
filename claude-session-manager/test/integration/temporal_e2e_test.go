//go:build integration

package integration_test

import (
	"context"
	"os"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"

	"github.com/vbonnet/ai-tools/claude-session-manager/internal/temporal/workflows"
)

// Stub activity functions for E2E testing
func CreateSessionActivity(ctx context.Context, input interface{}) (interface{}, error) {
	return nil, nil
}

func ActivateSessionActivity(ctx context.Context, input interface{}) error {
	return nil
}

func StopSessionActivity(ctx context.Context, input interface{}) error {
	return nil
}

func ArchiveSessionActivity(ctx context.Context, input interface{}) error {
	return nil
}

func FetchSessionOutputActivity(ctx context.Context, input interface{}) ([]workflows.OutputLine, error) {
	return nil, nil
}

func LogEscalationActivity(ctx context.Context, input interface{}) error {
	return nil
}

func SendNotificationActivity(ctx context.Context, input interface{}) (workflows.NotificationResult, error) {
	return workflows.NotificationResult{}, nil
}

func StoreEscalationRecordActivity(ctx context.Context, input interface{}) error {
	return nil
}

var _ = Describe("Temporal Backend - End-to-End Integration", func() {
	var (
		temporalClient client.Client
		temporalWorker worker.Worker
		ctx            context.Context
		cancel         context.CancelFunc
	)

	BeforeEach(func() {
		// Set environment variable to use Temporal backend
		os.Setenv("AGM_SESSION_BACKEND", "temporal")

		ctx, cancel = context.WithTimeout(context.Background(), 30*time.Second)

		// Connect to Temporal server (assumes local Temporal running on default port)
		var err error
		temporalClient, err = client.Dial(client.Options{
			HostPort:  "localhost:7233",
			Namespace: "default",
		})
		if err != nil {
			Skip("Temporal server not available (required for E2E tests): " + err.Error())
		}

		// Register and start worker for test workflows
		temporalWorker = worker.New(temporalClient, "agm-test-queue", worker.Options{})

		// Register workflows
		temporalWorker.RegisterWorkflow(workflows.SessionWorkflow)
		temporalWorker.RegisterWorkflow(workflows.MonitorWorkflow)
		temporalWorker.RegisterWorkflow(workflows.EscalationWorkflow)

		// Register activities (stub implementations for E2E testing)
		temporalWorker.RegisterActivity(CreateSessionActivity)
		temporalWorker.RegisterActivity(ActivateSessionActivity)
		temporalWorker.RegisterActivity(StopSessionActivity)
		temporalWorker.RegisterActivity(ArchiveSessionActivity)
		temporalWorker.RegisterActivity(FetchSessionOutputActivity)
		temporalWorker.RegisterActivity(LogEscalationActivity)
		temporalWorker.RegisterActivity(SendNotificationActivity)
		temporalWorker.RegisterActivity(StoreEscalationRecordActivity)

		// Start worker
		err = temporalWorker.Start()
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		if temporalWorker != nil {
			temporalWorker.Stop()
		}
		if temporalClient != nil {
			temporalClient.Close()
		}
		if cancel != nil {
			cancel()
		}
		os.Unsetenv("AGM_SESSION_BACKEND")
	})

	Describe("Session Lifecycle with Temporal Backend", func() {
		It("creates a session with Temporal backend", func() {
			sessionID := "test-temporal-create-" + time.Now().Format("20060102150405")
			workflowID := "session-" + sessionID

			// Start SessionWorkflow
			workflowInput := workflows.SessionWorkflowInput{
				SessionID:   sessionID,
				SessionName: "e2e-test-session",
				WorkingDir:  "/tmp/agm-test",
				Agent:       "claude",
				Project:     "/tmp/agm-test",
				Tags:        []string{"e2e-test"},
			}

			workflowOptions := client.StartWorkflowOptions{
				ID:        workflowID,
				TaskQueue: "agm-test-queue",
			}

			workflowRun, err := temporalClient.ExecuteWorkflow(ctx, workflowOptions, workflows.SessionWorkflow, workflowInput)
			Expect(err).ToNot(HaveOccurred())
			Expect(workflowRun.GetID()).To(Equal(workflowID))

			// Wait for workflow to start
			time.Sleep(200 * time.Millisecond)

			// Query workflow state
			var state workflows.SessionWorkflowState
			result, err := temporalClient.QueryWorkflow(ctx, workflowID, "", workflows.QuerySessionState)
			Expect(err).ToNot(HaveOccurred())

			err = result.Get(&state)
			Expect(err).ToNot(HaveOccurred())

			// Verify session is in active state
			Expect(state.State).To(Equal(workflows.SessionStateActive))
			Expect(state.SessionID).To(Equal(sessionID))
			Expect(state.SessionName).To(Equal("e2e-test-session"))

			// Archive session to complete workflow
			err = temporalClient.SignalWorkflow(ctx, workflowID, "", workflows.SignalArchive, nil)
			Expect(err).ToNot(HaveOccurred())

			// Wait for workflow to complete
			err = workflowRun.Get(ctx, nil)
			Expect(err).ToNot(HaveOccurred())
		})

		It("attaches to an existing Temporal session", func() {
			sessionID := "test-temporal-attach-" + time.Now().Format("20060102150405")
			workflowID := "session-" + sessionID

			// Start SessionWorkflow
			workflowInput := workflows.SessionWorkflowInput{
				SessionID:   sessionID,
				SessionName: "e2e-attach-session",
				WorkingDir:  "/tmp/agm-test",
				Agent:       "claude",
				Project:     "/tmp/agm-test",
			}

			workflowOptions := client.StartWorkflowOptions{
				ID:        workflowID,
				TaskQueue: "agm-test-queue",
			}

			_, err := temporalClient.ExecuteWorkflow(ctx, workflowOptions, workflows.SessionWorkflow, workflowInput)
			Expect(err).ToNot(HaveOccurred())

			// Wait for workflow to start
			time.Sleep(200 * time.Millisecond)

			// Attach client via signal
			err = temporalClient.SignalWorkflow(ctx, workflowID, "", workflows.SignalAttach, nil)
			Expect(err).ToNot(HaveOccurred())

			// Wait for signal processing
			time.Sleep(100 * time.Millisecond)

			// Query workflow state to verify client attached
			var state workflows.SessionWorkflowState
			result, err := temporalClient.QueryWorkflow(ctx, workflowID, "", workflows.QuerySessionState)
			Expect(err).ToNot(HaveOccurred())

			err = result.Get(&state)
			Expect(err).ToNot(HaveOccurred())

			// Verify client count increased
			Expect(state.AttachedClients).To(Equal(1))

			// Detach client
			err = temporalClient.SignalWorkflow(ctx, workflowID, "", workflows.SignalDetach, nil)
			Expect(err).ToNot(HaveOccurred())

			// Wait for signal processing
			time.Sleep(100 * time.Millisecond)

			// Query again to verify client detached
			result, err = temporalClient.QueryWorkflow(ctx, workflowID, "", workflows.QuerySessionState)
			Expect(err).ToNot(HaveOccurred())

			err = result.Get(&state)
			Expect(err).ToNot(HaveOccurred())

			Expect(state.AttachedClients).To(Equal(0))

			// Clean up: Archive session
			err = temporalClient.SignalWorkflow(ctx, workflowID, "", workflows.SignalArchive, nil)
			Expect(err).ToNot(HaveOccurred())
		})

		It("verifies workflow state in Temporal UI", func() {
			sessionID := "test-temporal-state-" + time.Now().Format("20060102150405")
			workflowID := "session-" + sessionID

			// Start SessionWorkflow
			workflowInput := workflows.SessionWorkflowInput{
				SessionID:   sessionID,
				SessionName: "e2e-state-verification",
				WorkingDir:  "/tmp/agm-test",
				Agent:       "claude",
				Project:     "/tmp/agm-test",
			}

			workflowOptions := client.StartWorkflowOptions{
				ID:        workflowID,
				TaskQueue: "agm-test-queue",
			}

			workflowRun, err := temporalClient.ExecuteWorkflow(ctx, workflowOptions, workflows.SessionWorkflow, workflowInput)
			Expect(err).ToNot(HaveOccurred())

			// Wait for workflow to start
			time.Sleep(200 * time.Millisecond)

			// Describe workflow execution to get state
			workflowDescription, err := temporalClient.DescribeWorkflowExecution(ctx, workflowID, "")
			Expect(err).ToNot(HaveOccurred())

			// Verify workflow is running
			Expect(workflowDescription.WorkflowExecutionInfo.GetStatus()).To(Equal(1)) // 1 = RUNNING

			// Query workflow state
			var state workflows.SessionWorkflowState
			result, err := temporalClient.QueryWorkflow(ctx, workflowID, "", workflows.QuerySessionState)
			Expect(err).ToNot(HaveOccurred())

			err = result.Get(&state)
			Expect(err).ToNot(HaveOccurred())

			// Verify workflow state matches expected
			Expect(state.State).To(Equal(workflows.SessionStateActive))
			Expect(state.SessionID).To(Equal(sessionID))

			// Archive to complete
			err = temporalClient.SignalWorkflow(ctx, workflowID, "", workflows.SignalArchive, nil)
			Expect(err).ToNot(HaveOccurred())

			// Wait for completion
			err = workflowRun.Get(ctx, nil)
			Expect(err).ToNot(HaveOccurred())

			// Verify workflow completed
			workflowDescription, err = temporalClient.DescribeWorkflowExecution(ctx, workflowID, "")
			Expect(err).ToNot(HaveOccurred())
			Expect(workflowDescription.WorkflowExecutionInfo.GetStatus()).To(Equal(3)) // 3 = COMPLETED
		})

		It("executes full lifecycle: create → active → stopped → archived", func() {
			sessionID := "test-temporal-lifecycle-" + time.Now().Format("20060102150405")
			workflowID := "session-" + sessionID

			// 1. Create session (start workflow)
			workflowInput := workflows.SessionWorkflowInput{
				SessionID:   sessionID,
				SessionName: "e2e-full-lifecycle",
				WorkingDir:  "/tmp/agm-test",
				Agent:       "claude",
				Project:     "/tmp/agm-test",
				Tags:        []string{"lifecycle-test"},
			}

			workflowOptions := client.StartWorkflowOptions{
				ID:        workflowID,
				TaskQueue: "agm-test-queue",
			}

			workflowRun, err := temporalClient.ExecuteWorkflow(ctx, workflowOptions, workflows.SessionWorkflow, workflowInput)
			Expect(err).ToNot(HaveOccurred())

			// Wait for workflow to start
			time.Sleep(200 * time.Millisecond)

			// 2. Verify ACTIVE state
			var state workflows.SessionWorkflowState
			result, err := temporalClient.QueryWorkflow(ctx, workflowID, "", workflows.QuerySessionState)
			Expect(err).ToNot(HaveOccurred())
			err = result.Get(&state)
			Expect(err).ToNot(HaveOccurred())
			Expect(state.State).To(Equal(workflows.SessionStateActive))

			// 3. Stop session
			err = temporalClient.SignalWorkflow(ctx, workflowID, "", workflows.SignalStop, nil)
			Expect(err).ToNot(HaveOccurred())

			// Wait for state transition
			time.Sleep(200 * time.Millisecond)

			// 4. Verify STOPPED state
			result, err = temporalClient.QueryWorkflow(ctx, workflowID, "", workflows.QuerySessionState)
			Expect(err).ToNot(HaveOccurred())
			err = result.Get(&state)
			Expect(err).ToNot(HaveOccurred())
			Expect(state.State).To(Equal(workflows.SessionStateStopped))

			// 5. Reactivate session
			err = temporalClient.SignalWorkflow(ctx, workflowID, "", workflows.SignalActivate, nil)
			Expect(err).ToNot(HaveOccurred())

			// Wait for state transition
			time.Sleep(200 * time.Millisecond)

			// 6. Verify back to ACTIVE state
			result, err = temporalClient.QueryWorkflow(ctx, workflowID, "", workflows.QuerySessionState)
			Expect(err).ToNot(HaveOccurred())
			err = result.Get(&state)
			Expect(err).ToNot(HaveOccurred())
			Expect(state.State).To(Equal(workflows.SessionStateActive))

			// 7. Archive session (final state)
			err = temporalClient.SignalWorkflow(ctx, workflowID, "", workflows.SignalArchive, nil)
			Expect(err).ToNot(HaveOccurred())

			// 8. Wait for workflow to complete
			err = workflowRun.Get(ctx, nil)
			Expect(err).ToNot(HaveOccurred())

			// 9. Verify workflow completed successfully
			workflowDescription, err := temporalClient.DescribeWorkflowExecution(ctx, workflowID, "")
			Expect(err).ToNot(HaveOccurred())
			Expect(workflowDescription.WorkflowExecutionInfo.GetStatus()).To(Equal(3)) // COMPLETED

			GinkgoWriter.Printf("Full lifecycle test completed for session %s\n", sessionID)
		})
	})

	Describe("Session Crash Resilience", func() {
		It("session survives workflow worker restart", func() {
			sessionID := "test-temporal-resilience-" + time.Now().Format("20060102150405")
			workflowID := "session-" + sessionID

			// Start SessionWorkflow
			workflowInput := workflows.SessionWorkflowInput{
				SessionID:   sessionID,
				SessionName: "e2e-crash-resilience",
				WorkingDir:  "/tmp/agm-test",
				Agent:       "claude",
				Project:     "/tmp/agm-test",
			}

			workflowOptions := client.StartWorkflowOptions{
				ID:        workflowID,
				TaskQueue: "agm-test-queue",
			}

			_, err := temporalClient.ExecuteWorkflow(ctx, workflowOptions, workflows.SessionWorkflow, workflowInput)
			Expect(err).ToNot(HaveOccurred())

			// Wait for workflow to start
			time.Sleep(200 * time.Millisecond)

			// Query initial state
			var stateBefore workflows.SessionWorkflowState
			result, err := temporalClient.QueryWorkflow(ctx, workflowID, "", workflows.QuerySessionState)
			Expect(err).ToNot(HaveOccurred())
			err = result.Get(&stateBefore)
			Expect(err).ToNot(HaveOccurred())
			Expect(stateBefore.State).To(Equal(workflows.SessionStateActive))

			// Simulate crash: Stop worker
			temporalWorker.Stop()

			// Wait a bit (simulating downtime)
			time.Sleep(500 * time.Millisecond)

			// Restart worker
			temporalWorker = worker.New(temporalClient, "agm-test-queue", worker.Options{})
			temporalWorker.RegisterWorkflow(workflows.SessionWorkflow)
			temporalWorker.RegisterActivity(CreateSessionActivity)
			temporalWorker.RegisterActivity(ArchiveSessionActivity)
			err = temporalWorker.Start()
			Expect(err).ToNot(HaveOccurred())

			// Wait for worker to reconnect
			time.Sleep(500 * time.Millisecond)

			// Query state after "crash" - should be unchanged
			var stateAfter workflows.SessionWorkflowState
			result, err = temporalClient.QueryWorkflow(ctx, workflowID, "", workflows.QuerySessionState)
			Expect(err).ToNot(HaveOccurred())
			err = result.Get(&stateAfter)
			Expect(err).ToNot(HaveOccurred())

			// Verify state persisted through "crash"
			Expect(stateAfter.State).To(Equal(workflows.SessionStateActive))
			Expect(stateAfter.SessionID).To(Equal(sessionID))

			GinkgoWriter.Printf("Session %s survived worker restart ✓\n", sessionID)

			// Clean up
			err = temporalClient.SignalWorkflow(ctx, workflowID, "", workflows.SignalArchive, nil)
			Expect(err).ToNot(HaveOccurred())
		})
	})
})
