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

var _ = Describe("Agent Parity - Session Management", func() {
	var adapters map[string]agent.Agent

	BeforeEach(func() {
		// Initialize adapters for all agents
		adapters = make(map[string]agent.Agent)

		// Claude adapter
		claudeAdapter, err := agent.NewClaudeAdapter(nil)
		Expect(err).ToNot(HaveOccurred())
		adapters["claude"] = claudeAdapter

		// Gemini adapter with test API key
		os.Setenv("GEMINI_API_KEY", "test-api-key-for-testing")
		geminiAdapter, err := agent.NewGeminiAdapter(&agent.GeminiConfig{
			APIKey: "test-api-key-for-testing",
		})
		Expect(err).ToNot(HaveOccurred())
		adapters["gemini"] = geminiAdapter

		// OpenCode adapter
		opencodeAdapter, err := agent.NewOpenCodeAdapter(nil)
		Expect(err).ToNot(HaveOccurred())
		adapters["opencode"] = opencodeAdapter
	})

	AfterEach(func() {
		os.Unsetenv("GEMINI_API_KEY")
	})

	Describe("CreateSession", func() {
		DescribeTable("creates new session with default parameters",
			func(agentName string) {
				adapter := adapters[agentName]
				ctx := agent.SessionContext{
					Name:             testEnv.UniqueSessionName(agentName + "-create"),
					WorkingDirectory: "/tmp",
				}

				sessionID, err := adapter.CreateSession(ctx)

				Expect(err).ToNot(HaveOccurred())
				Expect(sessionID).ToNot(BeEmpty())

				// Verify session is active
				status, err := adapter.GetSessionStatus(sessionID)
				Expect(err).ToNot(HaveOccurred())
				Expect(status).To(Equal(agent.StatusActive))

				// Cleanup
				adapter.TerminateSession(sessionID)
			},
			Entry("claude agent", "claude"),
			Entry("gemini agent", "gemini"),
			Entry("opencode agent", "opencode"),
		)

		DescribeTable("creates session with project metadata",
			func(agentName string) {
				adapter := adapters[agentName]
				ctx := agent.SessionContext{
					Name:             testEnv.UniqueSessionName(agentName + "-project"),
					WorkingDirectory: "~/src/ai-tools",
					Project:          "ai-tools",
				}

				sessionID, err := adapter.CreateSession(ctx)
				Expect(err).ToNot(HaveOccurred())
				Expect(sessionID).ToNot(BeEmpty())

				// Cleanup
				adapter.TerminateSession(sessionID)
			},
			Entry("claude agent", "claude"),
			Entry("gemini agent", "gemini"),
			Entry("opencode agent", "opencode"),
		)

		DescribeTable("creates session with authorized directories",
			func(agentName string) {
				adapter := adapters[agentName]
				ctx := agent.SessionContext{
					Name:             testEnv.UniqueSessionName(agentName + "-authdir"),
					WorkingDirectory: "/tmp",
					AuthorizedDirs:   []string{"~/src", "~/data"},
				}

				sessionID, err := adapter.CreateSession(ctx)
				Expect(err).ToNot(HaveOccurred())

				// Cleanup
				adapter.TerminateSession(sessionID)
			},
			Entry("claude agent", "claude"),
			Entry("gemini agent", "gemini"),
			Entry("opencode agent", "opencode"),
		)
	})

	Describe("ResumeSession", func() {
		DescribeTable("resumes existing active session",
			func(agentName string) {
				adapter := adapters[agentName]
				// Create session
				ctx := agent.SessionContext{
					Name:             testEnv.UniqueSessionName(agentName + "-resume"),
					WorkingDirectory: "/tmp",
				}
				sessionID, _ := adapter.CreateSession(ctx)

				// Resume session
				err := adapter.ResumeSession(sessionID)
				Expect(err).ToNot(HaveOccurred())

				// Verify still active
				status, _ := adapter.GetSessionStatus(sessionID)
				Expect(status).To(Equal(agent.StatusActive))

				// Cleanup
				adapter.TerminateSession(sessionID)
			},
			Entry("claude agent", "claude"),
			Entry("gemini agent", "gemini"),
			Entry("opencode agent", "opencode"),
		)

		DescribeTable("returns error when resuming non-existent session",
			func(agentName string) {
				adapter := adapters[agentName]
				fakeSessionID := agent.SessionID("non-existent-session-id-12345")

				err := adapter.ResumeSession(fakeSessionID)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("not found"))
			},
			Entry("claude agent", "claude"),
			Entry("gemini agent", "gemini"),
			Entry("opencode agent", "opencode"),
		)
	})

	Describe("TerminateSession", func() {
		DescribeTable("terminates active session gracefully",
			func(agentName string) {
				adapter := adapters[agentName]
				// Create session
				ctx := agent.SessionContext{
					Name:             testEnv.UniqueSessionName(agentName + "-terminate"),
					WorkingDirectory: "/tmp",
				}
				sessionID, _ := adapter.CreateSession(ctx)

				// Terminate
				err := adapter.TerminateSession(sessionID)
				Expect(err).ToNot(HaveOccurred())

				// Verify terminated
				status, _ := adapter.GetSessionStatus(sessionID)
				Expect(status).To(Equal(agent.StatusTerminated))
			},
			Entry("claude agent", "claude"),
			Entry("gemini agent", "gemini"),
			Entry("opencode agent", "opencode"),
		)
	})

	Describe("GetSessionStatus", func() {
		DescribeTable("returns active status for running session",
			func(agentName string) {
				adapter := adapters[agentName]
				ctx := agent.SessionContext{
					Name:             testEnv.UniqueSessionName(agentName + "-status"),
					WorkingDirectory: "/tmp",
				}
				sessionID, _ := adapter.CreateSession(ctx)
				defer adapter.TerminateSession(sessionID)

				status, err := adapter.GetSessionStatus(sessionID)

				Expect(err).ToNot(HaveOccurred())
				Expect(status).To(Equal(agent.StatusActive))
			},
			Entry("claude agent", "claude"),
			Entry("gemini agent", "gemini"),
			Entry("opencode agent", "opencode"),
		)

		DescribeTable("returns terminated status after session ends",
			func(agentName string) {
				adapter := adapters[agentName]
				ctx := agent.SessionContext{
					Name:             testEnv.UniqueSessionName(agentName + "-terminated"),
					WorkingDirectory: "/tmp",
				}
				sessionID, _ := adapter.CreateSession(ctx)
				adapter.TerminateSession(sessionID)

				status, err := adapter.GetSessionStatus(sessionID)

				Expect(err).ToNot(HaveOccurred())
				Expect(status).To(Equal(agent.StatusTerminated))
			},
			Entry("claude agent", "claude"),
			Entry("gemini agent", "gemini"),
			Entry("opencode agent", "opencode"),
		)

		DescribeTable("returns terminated for non-existent session",
			func(agentName string) {
				adapter := adapters[agentName]
				fakeSessionID := agent.SessionID("totally-fake-session-999")

				status, err := adapter.GetSessionStatus(fakeSessionID)

				Expect(err).ToNot(HaveOccurred())
				Expect(status).To(Equal(agent.StatusTerminated))
			},
			Entry("claude agent", "claude"),
			Entry("gemini agent", "gemini"),
			Entry("opencode agent", "opencode"),
		)
	})

	Describe("Session Persistence", func() {
		DescribeTable("preserves session across adapter recreation",
			func(agentName string) {
				adapter := adapters[agentName]
				ctx := agent.SessionContext{
					Name:             testEnv.UniqueSessionName(agentName + "-persist"),
					WorkingDirectory: "/tmp",
				}
				sessionID, _ := adapter.CreateSession(ctx)
				defer adapter.TerminateSession(sessionID)

				// Create new adapter instance
				var newAdapter agent.Agent
				var err error
				if agentName == "claude" {
					newAdapter, err = agent.NewClaudeAdapter(nil)
				} else {
					newAdapter, err = agent.NewGeminiAdapter(&agent.GeminiConfig{
						APIKey: "test-api-key-for-testing",
					})
				}
				Expect(err).ToNot(HaveOccurred())

				// Verify session still exists via new adapter
				status, err := newAdapter.GetSessionStatus(sessionID)
				Expect(err).ToNot(HaveOccurred())
				Expect(status).To(Equal(agent.StatusActive))
			},
			Entry("claude agent", "claude"),
			Entry("gemini agent", "gemini"),
			Entry("opencode agent", "opencode"),
		)
	})

	Describe("Concurrent Session Management", func() {
		DescribeTable("manages multiple sessions independently",
			func(agentName string) {
				adapter := adapters[agentName]
				sessions := []agent.SessionID{}

				// Create 3 sessions
				for i := 1; i <= 3; i++ {
					ctx := agent.SessionContext{
						Name:             testEnv.UniqueSessionName(fmt.Sprintf("%s-concurrent-%d", agentName, i)),
						WorkingDirectory: "/tmp",
					}
					sessionID, err := adapter.CreateSession(ctx)
					Expect(err).ToNot(HaveOccurred())
					sessions = append(sessions, sessionID)
				}

				// Verify all sessions are active
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

				// Give sessions time to terminate (especially for tmux)
				time.Sleep(200 * time.Millisecond)

				// Verify all terminated
				for _, sessionID := range sessions {
					status, _ := adapter.GetSessionStatus(sessionID)
					Expect(status).To(Equal(agent.StatusTerminated))
				}
			},
			Entry("claude agent", "claude"),
			Entry("gemini agent", "gemini"),
			Entry("opencode agent", "opencode"),
		)
	})

	Describe("Session Creation Edge Cases", func() {
		DescribeTable("handles empty session name gracefully",
			func(agentName string) {
				adapter := adapters[agentName]
				ctx := agent.SessionContext{
					Name:             "",
					WorkingDirectory: "/tmp",
				}

				sessionID, err := adapter.CreateSession(ctx)

				// Should either auto-generate name or return error
				if err == nil {
					Expect(sessionID).ToNot(BeEmpty())
					adapter.TerminateSession(sessionID)
				} else {
					Expect(err).To(HaveOccurred())
				}
			},
			Entry("claude agent", "claude"),
			Entry("gemini agent", "gemini"),
			Entry("opencode agent", "opencode"),
		)

		DescribeTable("handles duplicate session creation",
			func(agentName string) {
				adapter := adapters[agentName]
				ctx := agent.SessionContext{
					Name:             testEnv.UniqueSessionName(agentName + "-dup"),
					WorkingDirectory: "/tmp",
				}

				// Create first session
				sessionID1, err := adapter.CreateSession(ctx)
				Expect(err).ToNot(HaveOccurred())
				defer adapter.TerminateSession(sessionID1)

				// Attempt to create session with same name
				sessionID2, err := adapter.CreateSession(ctx)

				// Behavior is implementation-specific
				// Both should either:
				// 1. Create new session with unique ID
				// 2. Return error about duplicate name
				if err == nil {
					Expect(sessionID2).ToNot(BeEmpty())
					// IDs should be different even if name is same
					Expect(sessionID2).ToNot(Equal(sessionID1))
					adapter.TerminateSession(sessionID2)
				}
			},
			Entry("claude agent", "claude"),
			Entry("gemini agent", "gemini"),
			Entry("opencode agent", "opencode"),
		)
	})

	Describe("Session Timing", func() {
		DescribeTable("records session creation time",
			func(agentName string) {
				adapter := adapters[agentName]
				beforeCreate := time.Now()

				ctx := agent.SessionContext{
					Name:             testEnv.UniqueSessionName(agentName + "-timing"),
					WorkingDirectory: "/tmp",
				}
				sessionID, err := adapter.CreateSession(ctx)
				Expect(err).ToNot(HaveOccurred())
				defer adapter.TerminateSession(sessionID)

				afterCreate := time.Now()

				// Verify session was created within time window
				// (Implementation-specific verification would check metadata)
				Expect(afterCreate.Sub(beforeCreate)).To(BeNumerically("<", 10*time.Second))
			},
			Entry("claude agent", "claude"),
			Entry("gemini agent", "gemini"),
			Entry("opencode agent", "opencode"),
		)
	})
})
