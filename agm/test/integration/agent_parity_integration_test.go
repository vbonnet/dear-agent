//go:build integration

package integration_test

import (
	"fmt"
	"os"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vbonnet/dear-agent/agm/internal/agent"
)

var _ = Describe("Agent Parity - End-to-End Integration", func() {
	var adapters map[string]agent.Agent

	BeforeEach(func() {
		adapters = make(map[string]agent.Agent)

		claudeAdapter, err := agent.NewClaudeAdapter(nil)
		Expect(err).ToNot(HaveOccurred())
		adapters["claude"] = claudeAdapter

		os.Setenv("GEMINI_API_KEY", "test-api-key-for-testing")
		geminiAdapter, err := agent.NewGeminiAdapter(&agent.GeminiConfig{
			APIKey: "test-api-key-for-testing",
		})
		Expect(err).ToNot(HaveOccurred())
		adapters["gemini"] = geminiAdapter

		opencodeAdapter, err := agent.NewOpenCodeAdapter(nil)
		Expect(err).ToNot(HaveOccurred())
		adapters["opencode"] = opencodeAdapter
	})

	AfterEach(func() {
		os.Unsetenv("GEMINI_API_KEY")
	})

	Describe("Complete Session Lifecycle", func() {
		DescribeTable("executes full session lifecycle",
			func(agentName string) {
				adapter := adapters[agentName]

				// 1. Create session
				ctx := agent.SessionContext{
					Name:             testEnv.UniqueSessionName(agentName + "-lifecycle"),
					WorkingDirectory: "/tmp",
					Project:          "test-project",
				}

				sessionID, err := adapter.CreateSession(ctx)
				Expect(err).ToNot(HaveOccurred())
				Expect(sessionID).ToNot(BeEmpty())

				// 2. Verify session is active
				status, err := adapter.GetSessionStatus(sessionID)
				Expect(err).ToNot(HaveOccurred())
				Expect(status).To(Equal(agent.StatusActive))

				// 3. Get initial history (should be empty or minimal)
				history, err := adapter.GetHistory(sessionID)
				Expect(err).ToNot(HaveOccurred())
				Expect(history).ToNot(BeNil())
				initialLength := len(history)

				// 4. Export conversation (may be empty for new session)
				exportData, err := adapter.ExportConversation(sessionID, agent.FormatJSONL)
				// Export should succeed even for empty history
				if err == nil {
					Expect(exportData).ToNot(BeNil())
				}

				// 5. Verify capabilities
				caps := adapter.Capabilities()
				Expect(caps.ModelName).ToNot(BeEmpty())
				Expect(caps.MaxContextWindow).To(BeNumerically(">", 0))

				// 6. Resume session (should be no-op for active session)
				err = adapter.ResumeSession(sessionID)
				Expect(err).ToNot(HaveOccurred())

				// 7. Verify still active
				status, _ = adapter.GetSessionStatus(sessionID)
				Expect(status).To(Equal(agent.StatusActive))

				// 8. Terminate session
				err = adapter.TerminateSession(sessionID)
				Expect(err).ToNot(HaveOccurred())

				// 9. Verify terminated
				status, _ = adapter.GetSessionStatus(sessionID)
				Expect(status).To(Equal(agent.StatusTerminated))

				// Log completion
				GinkgoWriter.Printf("%s completed full lifecycle: %d initial messages\n",
					agentName, initialLength)
			},
			Entry("claude agent", "claude"),
			Entry("gemini agent", "gemini"),
			Entry("opencode agent", "opencode"),
		)
	})

	Describe("Multi-Session Coordination", func() {
		DescribeTable("manages multiple sessions with different configurations",
			func(agentName string) {
				adapter := adapters[agentName]
				sessions := make(map[string]agent.SessionID)

				// Create sessions with different contexts
				configs := []struct {
					name    string
					project string
					workDir string
				}{
					{"session-1", "project-a", "/tmp"},
					{"session-2", "project-b", "/home/user"},
					{"session-3", "project-c", "/var/tmp"},
				}

				// Create all sessions
				for _, cfg := range configs {
					ctx := agent.SessionContext{
						Name:             testEnv.UniqueSessionName(agentName + "-" + cfg.name),
						WorkingDirectory: cfg.workDir,
						Project:          cfg.project,
					}
					sessionID, err := adapter.CreateSession(ctx)
					Expect(err).ToNot(HaveOccurred())
					sessions[cfg.name] = sessionID
				}

				// Verify all are active
				for _, sessionID := range sessions {
					status, err := adapter.GetSessionStatus(sessionID)
					Expect(err).ToNot(HaveOccurred())
					Expect(status).To(Equal(agent.StatusActive))
				}

				// Cleanup all sessions
				for _, sessionID := range sessions {
					err := adapter.TerminateSession(sessionID)
					Expect(err).ToNot(HaveOccurred())
				}

				// Verify all terminated
				for _, sessionID := range sessions {
					status, _ := adapter.GetSessionStatus(sessionID)
					Expect(status).To(Equal(agent.StatusTerminated))
				}

				GinkgoWriter.Printf("%s managed %d concurrent sessions\n",
					agentName, len(sessions))
			},
			Entry("claude agent", "claude"),
			Entry("gemini agent", "gemini"),
			Entry("opencode agent", "opencode"),
		)
	})

	Describe("Session Persistence and Recovery", func() {
		DescribeTable("preserves session across adapter recreation",
			func(agentName string) {
				adapter := adapters[agentName]

				// Create session
				ctx := agent.SessionContext{
					Name:             testEnv.UniqueSessionName(agentName + "-persist"),
					WorkingDirectory: "/tmp",
				}
				sessionID, err := adapter.CreateSession(ctx)
				Expect(err).ToNot(HaveOccurred())
				defer adapter.TerminateSession(sessionID)

				// Verify active
				status, _ := adapter.GetSessionStatus(sessionID)
				Expect(status).To(Equal(agent.StatusActive))

				// Create new adapter instance
				var newAdapter agent.Agent
				if agentName == "claude" {
					newAdapter, err = agent.NewClaudeAdapter(nil)
				} else if agentName == "gemini" {
					newAdapter, err = agent.NewGeminiAdapter(&agent.GeminiConfig{
						APIKey: "test-api-key-for-testing",
					})
				} else if agentName == "opencode" {
					newAdapter, err = agent.NewOpenCodeAdapter(nil)
				}
				Expect(err).ToNot(HaveOccurred())

				// Verify session still exists via new adapter
				status, err = newAdapter.GetSessionStatus(sessionID)
				Expect(err).ToNot(HaveOccurred())
				Expect(status).To(Equal(agent.StatusActive))

				// Cleanup via new adapter
				err = newAdapter.TerminateSession(sessionID)
				Expect(err).ToNot(HaveOccurred())
			},
			Entry("claude agent", "claude"),
			Entry("gemini agent", "gemini"),
			Entry("opencode agent", "opencode"),
		)
	})

	Describe("Error Recovery and Edge Cases", func() {
		DescribeTable("handles double termination gracefully",
			func(agentName string) {
				adapter := adapters[agentName]
				ctx := agent.SessionContext{
					Name:             testEnv.UniqueSessionName(agentName + "-doubleterm"),
					WorkingDirectory: "/tmp",
				}
				sessionID, _ := adapter.CreateSession(ctx)

				// First termination
				err := adapter.TerminateSession(sessionID)
				Expect(err).ToNot(HaveOccurred())

				// Second termination (should handle gracefully)
				err = adapter.TerminateSession(sessionID)
				// May return error or succeed idempotently
				_ = err
			},
			Entry("claude agent", "claude"),
			Entry("gemini agent", "gemini"),
			Entry("opencode agent", "opencode"),
		)

		DescribeTable("handles operations on non-existent sessions consistently",
			func(agentName string) {
				adapter := adapters[agentName]
				fakeSessionID := agent.SessionID("totally-fake-session-xyz")

				// Resume should error
				err1 := adapter.ResumeSession(fakeSessionID)
				Expect(err1).To(HaveOccurred())

				// Terminate may succeed idempotently or return error
				err2 := adapter.TerminateSession(fakeSessionID)
				_ = err2

				// GetHistory may return empty or error (both valid)
				history, err3 := adapter.GetHistory(fakeSessionID)
				if err3 != nil {
					Expect(history).To(BeNil())
				} else {
					Expect(history).ToNot(BeNil())
				}

				// SendMessage should error
				msg := agent.Message{Role: agent.RoleUser, Content: "test"}
				err4 := adapter.SendMessage(fakeSessionID, msg)
				Expect(err4).To(HaveOccurred())
			},
			Entry("claude agent", "claude"),
			Entry("gemini agent", "gemini"),
			Entry("opencode agent", "opencode"),
		)
	})

	Describe("Performance and Scalability", func() {
		DescribeTable("creates and terminates sessions efficiently",
			func(agentName string) {
				adapter := adapters[agentName]

				startTime := time.Now()
				sessionCount := 5
				sessions := []agent.SessionID{}

				// Create multiple sessions
				for i := 0; i < sessionCount; i++ {
					ctx := agent.SessionContext{
						Name:             testEnv.UniqueSessionName(fmt.Sprintf("%s-perf-%d", agentName, i)),
						WorkingDirectory: "/tmp",
					}
					sessionID, err := adapter.CreateSession(ctx)
					Expect(err).ToNot(HaveOccurred())
					sessions = append(sessions, sessionID)
				}

				creationTime := time.Since(startTime)

				// Cleanup
				for _, sessionID := range sessions {
					adapter.TerminateSession(sessionID)
				}

				cleanupTime := time.Since(startTime)

				GinkgoWriter.Printf("%s performance: %d sessions created in %v, total time %v\n",
					agentName, sessionCount, creationTime, cleanupTime)

				// Sessions should be created reasonably fast
				Expect(creationTime).To(BeNumerically("<", 30*time.Second))
			},
			Entry("claude agent", "claude"),
			Entry("gemini agent", "gemini"),
			Entry("opencode agent", "opencode"),
		)
	})

	Describe("Feature Parity Summary", func() {
		It("generates comprehensive feature comparison", func() {
			claudeAdapter := adapters["claude"]
			geminiAdapter := adapters["gemini"]

			GinkgoWriter.Println("\n=== COMPREHENSIVE FEATURE PARITY REPORT ===\n")

			// Basic metadata
			GinkgoWriter.Println("1. AGENT METADATA")
			GinkgoWriter.Printf("   Claude: %s (version: %s)\n", claudeAdapter.Name(), claudeAdapter.Version())
			GinkgoWriter.Printf("   Gemini: %s (version: %s)\n", geminiAdapter.Name(), geminiAdapter.Version())

			// Capabilities
			claudeCaps := claudeAdapter.Capabilities()
			geminiCaps := geminiAdapter.Capabilities()

			GinkgoWriter.Println("\n2. CAPABILITIES")
			compareCapability := func(name string, claude, gemini bool) {
				match := "✓"
				if claude != gemini {
					match = "✗"
				}
				GinkgoWriter.Printf("   %s %-25s | Claude: %-5v | Gemini: %-5v\n", match, name, claude, gemini)
			}

			compareCapability("Slash Commands", claudeCaps.SupportsSlashCommands, geminiCaps.SupportsSlashCommands)
			compareCapability("Hooks", claudeCaps.SupportsHooks, geminiCaps.SupportsHooks)
			compareCapability("Tools", claudeCaps.SupportsTools, geminiCaps.SupportsTools)
			compareCapability("Vision", claudeCaps.SupportsVision, geminiCaps.SupportsVision)
			compareCapability("Multimodal", claudeCaps.SupportsMultimodal, geminiCaps.SupportsMultimodal)
			compareCapability("Streaming", claudeCaps.SupportsStreaming, geminiCaps.SupportsStreaming)
			compareCapability("System Prompts", claudeCaps.SupportsSystemPrompts, geminiCaps.SupportsSystemPrompts)

			GinkgoWriter.Printf("\n   Context Window: Claude: %d tokens | Gemini: %d tokens\n",
				claudeCaps.MaxContextWindow, geminiCaps.MaxContextWindow)

			// Session management
			GinkgoWriter.Println("\n3. SESSION MANAGEMENT")
			testSession := func(adapter agent.Agent, name string) bool {
				ctx := agent.SessionContext{
					Name:             testEnv.UniqueSessionName(name + "-summary"),
					WorkingDirectory: "/tmp",
				}
				sessionID, err := adapter.CreateSession(ctx)
				if err != nil {
					return false
				}
				defer adapter.TerminateSession(sessionID)
				return true
			}

			claudeSession := testSession(claudeAdapter, "claude")
			geminiSession := testSession(geminiAdapter, "gemini")

			GinkgoWriter.Printf("   CreateSession: Claude: %v | Gemini: %v\n", claudeSession, geminiSession)

			GinkgoWriter.Println("\n=== END REPORT ===\n")
		})
	})
})
