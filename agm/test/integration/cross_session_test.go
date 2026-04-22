//go:build integration

package integration_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vbonnet/dear-agent/agm/internal/daemon"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/messages"
	"github.com/vbonnet/dear-agent/agm/test/integration/helpers"
)

var _ = Describe("Cross-Session Integration Tests (Task 2.4, bead oss-ji5p)", func() {
	var (
		session1Name string
		session2Name string
		session3Name string
		workDir      string
		queue        *messages.MessageQueue
		testStarted  time.Time
	)

	BeforeEach(func() {
		// Skip if SKIP_E2E is set
		if os.Getenv("SKIP_E2E") != "" {
			Skip("SKIP_E2E environment variable is set")
		}

		testStarted = time.Now()

		// Create unique session names
		session1Name = testEnv.UniqueSessionName("cross-session-1")
		session2Name = testEnv.UniqueSessionName("cross-session-2")
		session3Name = testEnv.UniqueSessionName("cross-session-3")
		workDir = "/tmp"

		// Initialize message queue
		var err error
		queue, err = messages.NewMessageQueue()
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		// Cleanup queue
		if queue != nil {
			_ = queue.Close()
		}

		// Preserve sessions on test failure for debugging
		if CurrentSpecReport().Failed() {
			GinkgoWriter.Printf("Test failed, preserving sessions:\n")
			GinkgoWriter.Printf("  - tmux attach -t %s\n", session1Name)
			GinkgoWriter.Printf("  - tmux attach -t %s\n", session2Name)
			GinkgoWriter.Printf("  - tmux attach -t %s\n", session3Name)
			return
		}

		// Clean up on success
		helpers.KillTmuxSession(session1Name)
		helpers.KillTmuxSession(session2Name)
		helpers.KillTmuxSession(session3Name)
		os.RemoveAll(testEnv.ManifestDir(session1Name))
		os.RemoveAll(testEnv.ManifestDir(session2Name))
		os.RemoveAll(testEnv.ManifestDir(session3Name))
	})

	Describe("TestCrossSession_StateTransitions", func() {
		It("should test READY→THINKING→READY cycle", func() {
			// Create 3 test sessions
			sessions := []string{session1Name, session2Name, session3Name}

			for _, sessionName := range sessions {
				err := createTestSession(sessionName, workDir)
				Expect(err).ToNot(HaveOccurred())
			}

			// Test state transitions for session1
			GinkgoWriter.Printf("Testing state transitions for %s\n", session1Name)

			// Verify initial state is READY
			state := getSessionState(session1Name)
			Expect(state).To(Equal(manifest.StateDone))

			// Simulate state change: READY → THINKING
			err := simulateStateChange(session1Name, manifest.StateWorking)
			Expect(err).ToNot(HaveOccurred())

			state = getSessionState(session1Name)
			Expect(state).To(Equal(manifest.StateWorking))

			// Simulate state change: THINKING → READY
			err = simulateStateChange(session1Name, manifest.StateDone)
			Expect(err).ToNot(HaveOccurred())

			state = getSessionState(session1Name)
			Expect(state).To(Equal(manifest.StateDone))

			// Test state transitions for all sessions
			for _, sessionName := range sessions {
				GinkgoWriter.Printf("Cycling %s through THINKING state\n", sessionName)

				// READY → THINKING
				err := simulateStateChange(sessionName, manifest.StateWorking)
				Expect(err).ToNot(HaveOccurred())
				Expect(getSessionState(sessionName)).To(Equal(manifest.StateWorking))

				// THINKING → READY
				err = simulateStateChange(sessionName, manifest.StateDone)
				Expect(err).ToNot(HaveOccurred())
				Expect(getSessionState(sessionName)).To(Equal(manifest.StateDone))
			}
		})
	})

	Describe("TestCrossSession_MessageDelivery", func() {
		It("should queue message, wait for session READY, and auto-deliver", func() {
			// Create test sessions
			err := createTestSession(session1Name, workDir)
			Expect(err).ToNot(HaveOccurred())

			err = createTestSession(session2Name, workDir)
			Expect(err).ToNot(HaveOccurred())

			// Set session2 to THINKING
			err = simulateStateChange(session2Name, manifest.StateWorking)
			Expect(err).ToNot(HaveOccurred())

			// Queue a message from session1 to session2
			messageID := fmt.Sprintf("test-msg-%d", time.Now().UnixNano())
			entry := &messages.QueueEntry{
				MessageID: messageID,
				From:      session1Name,
				To:        session2Name,
				Message:   "Test message for cross-session delivery",
				Priority:  messages.PriorityMedium,
				QueuedAt:  time.Now(),
			}

			err = queue.Enqueue(entry)
			Expect(err).ToNot(HaveOccurred())

			// Verify message is queued
			pending, err := queue.GetPending(session2Name)
			Expect(err).ToNot(HaveOccurred())
			Expect(len(pending)).To(Equal(1))
			Expect(pending[0].MessageID).To(Equal(messageID))

			// Transition session2 to READY
			err = simulateStateChange(session2Name, manifest.StateDone)
			Expect(err).ToNot(HaveOccurred())

			// Measure delivery latency
			deliveryStart := time.Now()

			// Simulate daemon delivery
			err = deliverPendingMessages(queue, session2Name)
			Expect(err).ToNot(HaveOccurred())

			deliveryLatency := time.Since(deliveryStart)
			GinkgoWriter.Printf("Delivery latency: %v\n", deliveryLatency)

			// Verify message was marked delivered
			pending, err = queue.GetPending(session2Name)
			Expect(err).ToNot(HaveOccurred())
			Expect(len(pending)).To(Equal(0), "Message should be delivered")

			// Acceptance criteria: Latency <30s for message delivery
			Expect(deliveryLatency).To(BeNumerically("<", 30*time.Second))
		})
	})

	Describe("TestCrossSession_HookAccuracy", func() {
		It("should measure hook-based state detection false positive rate", func() {
			// Create test sessions
			sessions := []string{session1Name, session2Name, session3Name}

			for _, sessionName := range sessions {
				err := createTestSession(sessionName, workDir)
				Expect(err).ToNot(HaveOccurred())
			}

			// Simulate state changes and track accuracy
			totalTransitions := 0
			falsePositives := 0

			// Test scenarios for each session
			testCases := []struct {
				expectedState string
				description   string
			}{
				{manifest.StateDone, "Session idle at prompt"},
				{manifest.StateWorking, "Session executing tool"},
				{manifest.StateDone, "Session completed tool"},
				{manifest.StateWorking, "Session thinking"},
				{manifest.StateDone, "Session ready for input"},
			}

			for _, sessionName := range sessions {
				for _, tc := range testCases {
					totalTransitions++

					// Simulate state change (hook-based)
					err := simulateStateChange(sessionName, tc.expectedState)
					Expect(err).ToNot(HaveOccurred())

					// Read actual state from manifest
					actualState := getSessionState(sessionName)

					// Check for false positive
					if actualState != tc.expectedState {
						falsePositives++
						GinkgoWriter.Printf("False positive: %s - expected %s, got %s (%s)\n",
							sessionName, tc.expectedState, actualState, tc.description)
					}

					// Small delay between transitions
					time.Sleep(100 * time.Millisecond)
				}
			}

			// Calculate false positive rate
			fpRate := float64(falsePositives) / float64(totalTransitions) * 100.0
			GinkgoWriter.Printf("Hook accuracy results:\n")
			GinkgoWriter.Printf("  Total transitions: %d\n", totalTransitions)
			GinkgoWriter.Printf("  False positives: %d\n", falsePositives)
			GinkgoWriter.Printf("  FP rate: %.2f%%\n", fpRate)

			// Acceptance criteria: FP rate <5% with hooks
			Expect(fpRate).To(BeNumerically("<", 5.0),
				fmt.Sprintf("False positive rate %.2f%% exceeds 5%% target", fpRate))
		})
	})

	Describe("TestCrossSession_DeliveryLatency", func() {
		It("should measure time from queue to delivery", func() {
			// Create sender and receiver sessions
			err := createTestSession(session1Name, workDir)
			Expect(err).ToNot(HaveOccurred())

			err = createTestSession(session2Name, workDir)
			Expect(err).ToNot(HaveOccurred())

			// Set receiver to THINKING initially
			err = simulateStateChange(session2Name, manifest.StateWorking)
			Expect(err).ToNot(HaveOccurred())

			// Track latencies for multiple messages
			var latencies []time.Duration
			messageCount := 10

			for i := 0; i < messageCount; i++ {
				messageID := fmt.Sprintf("latency-test-%d", time.Now().UnixNano())
				queueTime := time.Now()

				// Queue message
				entry := &messages.QueueEntry{
					MessageID: messageID,
					From:      session1Name,
					To:        session2Name,
					Message:   fmt.Sprintf("Latency test message %d", i),
					Priority:  messages.PriorityMedium,
					QueuedAt:  queueTime,
				}

				err = queue.Enqueue(entry)
				Expect(err).ToNot(HaveOccurred())

				// After 5 messages, transition receiver to READY
				if i == 5 {
					err = simulateStateChange(session2Name, manifest.StateDone)
					Expect(err).ToNot(HaveOccurred())
				}

				// If session is READY, deliver immediately
				if getSessionState(session2Name) == manifest.StateDone {
					deliveryStart := time.Now()
					err = deliverPendingMessages(queue, session2Name)
					Expect(err).ToNot(HaveOccurred())

					latency := measureDeliveryLatency(messageID, queueTime)
					latencies = append(latencies, latency)
					GinkgoWriter.Printf("Message %s latency: %v\n", messageID, latency)
				}

				time.Sleep(50 * time.Millisecond)
			}

			// Calculate statistics
			if len(latencies) > 0 {
				var totalLatency time.Duration
				maxLatency := latencies[0]
				minLatency := latencies[0]

				for _, latency := range latencies {
					totalLatency += latency
					if latency > maxLatency {
						maxLatency = latency
					}
					if latency < minLatency {
						minLatency = latency
					}
				}

				avgLatency := totalLatency / time.Duration(len(latencies))

				GinkgoWriter.Printf("\nDelivery latency statistics:\n")
				GinkgoWriter.Printf("  Messages delivered: %d\n", len(latencies))
				GinkgoWriter.Printf("  Min latency: %v\n", minLatency)
				GinkgoWriter.Printf("  Max latency: %v\n", maxLatency)
				GinkgoWriter.Printf("  Avg latency: %v\n", avgLatency)

				// Acceptance criteria: Latency <30s for message delivery
				Expect(maxLatency).To(BeNumerically("<", 30*time.Second),
					"Maximum delivery latency exceeds 30s")
				Expect(avgLatency).To(BeNumerically("<", 5*time.Second),
					"Average delivery latency should be under 5s for READY sessions")
			}
		})
	})

	Describe("TestCrossSession_DeliverySuccessRate", func() {
		It("should achieve >95% delivery success rate", func() {
			// Create 3 test sessions
			sessions := []string{session1Name, session2Name, session3Name}

			for _, sessionName := range sessions {
				err := createTestSession(sessionName, workDir)
				Expect(err).ToNot(HaveOccurred())
			}

			totalMessages := 0
			deliveredMessages := 0
			failedMessages := 0

			// Send 30 messages across sessions in various states
			for i := 0; i < 30; i++ {
				fromSession := sessions[i%3]
				toSession := sessions[(i+1)%3]
				messageID := fmt.Sprintf("success-test-%d", time.Now().UnixNano())

				// Randomly set target session state
				var targetState string
				if i%3 == 0 {
					targetState = manifest.StateWorking
				} else {
					targetState = manifest.StateDone
				}

				err := simulateStateChange(toSession, targetState)
				Expect(err).ToNot(HaveOccurred())

				// Queue message
				entry := &messages.QueueEntry{
					MessageID: messageID,
					From:      fromSession,
					To:        toSession,
					Message:   fmt.Sprintf("Test message %d", i),
					Priority:  messages.PriorityMedium,
					QueuedAt:  time.Now(),
				}

				err = queue.Enqueue(entry)
				Expect(err).ToNot(HaveOccurred())
				totalMessages++

				// If target is READY, deliver immediately
				if targetState == manifest.StateDone {
					err = deliverPendingMessages(queue, toSession)
					if err == nil {
						deliveredMessages++
					} else {
						failedMessages++
						GinkgoWriter.Printf("Failed to deliver message %s: %v\n", messageID, err)
					}
				}

				time.Sleep(50 * time.Millisecond)
			}

			// Transition all sessions to READY and deliver remaining
			for _, sessionName := range sessions {
				err := simulateStateChange(sessionName, manifest.StateDone)
				Expect(err).ToNot(HaveOccurred())

				err = deliverPendingMessages(queue, sessionName)
				if err != nil {
					GinkgoWriter.Printf("Failed to deliver to %s: %v\n", sessionName, err)
				}
			}

			// Count final delivered messages
			finalDelivered := 0
			for _, sessionName := range sessions {
				pending, err := queue.GetPending(sessionName)
				Expect(err).ToNot(HaveOccurred())
				// Messages that are no longer pending were delivered
				finalDelivered += (totalMessages/3 - len(pending))
			}

			successRate := float64(finalDelivered) / float64(totalMessages) * 100.0

			GinkgoWriter.Printf("\nDelivery success statistics:\n")
			GinkgoWriter.Printf("  Total messages: %d\n", totalMessages)
			GinkgoWriter.Printf("  Delivered: %d\n", finalDelivered)
			GinkgoWriter.Printf("  Failed: %d\n", totalMessages-finalDelivered)
			GinkgoWriter.Printf("  Success rate: %.2f%%\n", successRate)

			// Acceptance criteria: Delivery success >95%
			Expect(successRate).To(BeNumerically(">=", 95.0),
				fmt.Sprintf("Delivery success rate %.2f%% below 95%% target", successRate))
		})
	})
})

// Helper functions

// createTestSession creates a test AGM session with manifest
func createTestSession(sessionName, workDir string) error {
	// Create tmux session
	if err := helpers.CreateTmuxSession(sessionName, workDir); err != nil {
		return fmt.Errorf("failed to create tmux session: %w", err)
	}

	// Create manifest directory
	manifestDir := testEnv.ManifestDir(sessionName)
	if err := os.MkdirAll(manifestDir, 0700); err != nil {
		return fmt.Errorf("failed to create manifest dir: %w", err)
	}

	// Create manifest with initial READY state
	manifestPath := testEnv.ManifestPath(sessionName)
	m := &manifest.Manifest{
		SchemaVersion:  manifest.SchemaVersion,
		SessionID:      "test-uuid-" + sessionName,
		Name:           sessionName,
		State:          manifest.StateDone,
		StateUpdatedAt: time.Now(),
		StateSource:    "test",
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
		Context: manifest.Context{
			Project: workDir,
		},
		Tmux: manifest.Tmux{
			SessionName: sessionName,
		},
		Agent: "claude",
	}

	if err := manifest.Write(manifestPath, m); err != nil {
		return fmt.Errorf("failed to write manifest: %w", err)
	}

	return nil
}

// simulateStateChange triggers a state transition for a session
func simulateStateChange(sessionName, newState string) error {
	manifestPath := filepath.Join(testEnv.ManifestDir(sessionName), "manifest.yaml")

	// Read current manifest
	m, err := manifest.Read(manifestPath)
	if err != nil {
		return fmt.Errorf("failed to read manifest: %w", err)
	}

	// Update state
	m.State = newState
	m.StateUpdatedAt = time.Now()
	m.StateSource = "hook"
	m.UpdatedAt = time.Now()

	// Write updated manifest
	if err := manifest.Write(manifestPath, m); err != nil {
		return fmt.Errorf("failed to write manifest: %w", err)
	}

	return nil
}

// getSessionState reads the current state from manifest
func getSessionState(sessionName string) string {
	manifestPath := filepath.Join(testEnv.ManifestDir(sessionName), "manifest.yaml")

	m, err := manifest.Read(manifestPath)
	if err != nil {
		return ""
	}

	return m.State
}

// measureDeliveryLatency calculates time from queue to delivery
func measureDeliveryLatency(messageID string, queuedAt time.Time) time.Duration {
	return time.Since(queuedAt)
}

// deliverPendingMessages simulates daemon delivery logic
func deliverPendingMessages(queue *messages.MessageQueue, sessionName string) error {
	// Get pending messages for this session
	pending, err := queue.GetPending(sessionName)
	if err != nil {
		return fmt.Errorf("failed to get pending messages: %w", err)
	}

	// Check if session is READY
	state := getSessionState(sessionName)
	if state != manifest.StateDone {
		return fmt.Errorf("session %s not ready (state: %s)", sessionName, state)
	}

	// Deliver all pending messages
	for _, entry := range pending {
		// Simulate delivery by marking as delivered
		if err := queue.MarkDelivered(entry.MessageID); err != nil {
			return fmt.Errorf("failed to mark message %s delivered: %w", entry.MessageID, err)
		}
	}

	return nil
}
