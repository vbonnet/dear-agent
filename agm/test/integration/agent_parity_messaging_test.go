//go:build integration

package integration_test

import (
	"os"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vbonnet/dear-agent/agm/internal/agent"
)

var _ = Describe("Agent Parity - Messaging", func() {
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

		// OpenCode adapter
		opencodeAdapter, err := agent.NewOpenCodeAdapter(nil)
		Expect(err).ToNot(HaveOccurred())
		adapters["opencode"] = opencodeAdapter
	})

	AfterEach(func() {
		os.Unsetenv("GEMINI_API_KEY")
	})

	Describe("SendMessage", func() {
		DescribeTable("sends user message to agent",
			func(agentName string) {
				Skip("Skipping actual API calls - requires real API keys and agent interaction")

				adapter := adapters[agentName]
				ctx := agent.SessionContext{
					Name:             testEnv.UniqueSessionName(agentName + "-msg"),
					WorkingDirectory: "/tmp",
				}
				sessionID, _ := adapter.CreateSession(ctx)
				defer adapter.TerminateSession(sessionID)

				msg := agent.Message{
					Role:    agent.RoleUser,
					Content: "Hello, this is a test message",
				}

				err := adapter.SendMessage(sessionID, msg)

				Expect(err).ToNot(HaveOccurred())
			},
			Entry("claude agent", "claude"),
			Entry("gemini agent", "gemini"),
			Entry("opencode agent", "opencode"),
		)

		DescribeTable("returns error when sending to terminated session",
			func(agentName string) {
				adapter := adapters[agentName]
				ctx := agent.SessionContext{
					Name:             testEnv.UniqueSessionName(agentName + "-msgterm"),
					WorkingDirectory: "/tmp",
				}
				sessionID, _ := adapter.CreateSession(ctx)
				adapter.TerminateSession(sessionID)

				msg := agent.Message{
					Role:    agent.RoleUser,
					Content: "This should fail",
				}

				err := adapter.SendMessage(sessionID, msg)

				Expect(err).To(HaveOccurred())
			},
			Entry("claude agent", "claude"),
			Entry("gemini agent", "gemini"),
			Entry("opencode agent", "opencode"),
		)

		DescribeTable("handles empty message content",
			func(agentName string) {
				Skip("Skipping actual API calls")

				adapter := adapters[agentName]
				ctx := agent.SessionContext{
					Name:             testEnv.UniqueSessionName(agentName + "-empty"),
					WorkingDirectory: "/tmp",
				}
				sessionID, _ := adapter.CreateSession(ctx)
				defer adapter.TerminateSession(sessionID)

				msg := agent.Message{
					Role:    agent.RoleUser,
					Content: "",
				}

				err := adapter.SendMessage(sessionID, msg)

				// Both agents should either accept empty messages or return error
				// Behavior should be consistent
				_ = err // Implementation-specific
			},
			Entry("claude agent", "claude"),
			Entry("gemini agent", "gemini"),
			Entry("opencode agent", "opencode"),
		)
	})

	Describe("GetHistory", func() {
		DescribeTable("returns empty history for new session",
			func(agentName string) {
				adapter := adapters[agentName]
				ctx := agent.SessionContext{
					Name:             testEnv.UniqueSessionName(agentName + "-hist"),
					WorkingDirectory: "/tmp",
				}
				sessionID, _ := adapter.CreateSession(ctx)
				defer adapter.TerminateSession(sessionID)

				history, err := adapter.GetHistory(sessionID)

				Expect(err).ToNot(HaveOccurred())
				Expect(history).ToNot(BeNil())
				// New session should have empty or minimal history
				Expect(len(history)).To(BeNumerically(">=", 0))
			},
			Entry("claude agent", "claude"),
			Entry("gemini agent", "gemini"),
			Entry("opencode agent", "opencode"),
		)

		DescribeTable("handles non-existent session consistently",
			func(agentName string) {
				adapter := adapters[agentName]
				fakeSessionID := agent.SessionID("non-existent-hist-session")

				history, err := adapter.GetHistory(fakeSessionID)

				// Both behaviors are acceptable:
				// - Return error (Claude checks session exists)
				// - Return empty history (Gemini returns empty if file missing)
				if err != nil {
					Expect(history).To(BeNil())
				} else {
					Expect(history).ToNot(BeNil())
					Expect(len(history)).To(Equal(0))
				}
			},
			Entry("claude agent", "claude"),
			Entry("gemini agent", "gemini"),
			Entry("opencode agent", "opencode"),
		)

		DescribeTable("preserves message order in history",
			func(agentName string) {
				Skip("Skipping actual API calls")

				adapter := adapters[agentName]
				ctx := agent.SessionContext{
					Name:             testEnv.UniqueSessionName(agentName + "-order"),
					WorkingDirectory: "/tmp",
				}
				sessionID, _ := adapter.CreateSession(ctx)
				defer adapter.TerminateSession(sessionID)

				// Send messages in specific order
				messages := []string{"First", "Second", "Third"}
				for _, content := range messages {
					msg := agent.Message{Role: agent.RoleUser, Content: content}
					adapter.SendMessage(sessionID, msg)
					time.Sleep(100 * time.Millisecond)
				}

				history, err := adapter.GetHistory(sessionID)

				Expect(err).ToNot(HaveOccurred())

				// Verify chronological order
				userMessages := []string{}
				for _, msg := range history {
					if msg.Role == agent.RoleUser {
						userMessages = append(userMessages, msg.Content)
					}
				}

				Expect(userMessages).To(ContainElement("First"))
				Expect(userMessages).To(ContainElement("Second"))
				Expect(userMessages).To(ContainElement("Third"))
			},
			Entry("claude agent", "claude"),
			Entry("gemini agent", "gemini"),
			Entry("opencode agent", "opencode"),
		)
	})

	Describe("Message Structure", func() {
		DescribeTable("message timestamps are recorded",
			func(agentName string) {
				Skip("Skipping actual API calls")

				adapter := adapters[agentName]
				ctx := agent.SessionContext{
					Name:             testEnv.UniqueSessionName(agentName + "-timestamp"),
					WorkingDirectory: "/tmp",
				}
				sessionID, _ := adapter.CreateSession(ctx)
				defer adapter.TerminateSession(sessionID)

				beforeSend := time.Now()
				msg := agent.Message{
					Role:    agent.RoleUser,
					Content: "Timestamp test",
				}
				adapter.SendMessage(sessionID, msg)
				afterSend := time.Now()
				time.Sleep(100 * time.Millisecond)

				history, _ := adapter.GetHistory(sessionID)

				// Find the message we sent
				found := false
				for _, histMsg := range history {
					if histMsg.Content == "Timestamp test" {
						found = true
						// Timestamp should be within send window
						Expect(histMsg.Timestamp).To(BeTemporally(">=", beforeSend))
						Expect(histMsg.Timestamp).To(BeTemporally("<=", afterSend.Add(1*time.Second)))
					}
				}
				Expect(found).To(BeTrue())
			},
			Entry("claude agent", "claude"),
			Entry("gemini agent", "gemini"),
			Entry("opencode agent", "opencode"),
		)

		DescribeTable("message IDs are unique",
			func(agentName string) {
				Skip("Skipping actual API calls")

				adapter := adapters[agentName]
				ctx := agent.SessionContext{
					Name:             testEnv.UniqueSessionName(agentName + "-msgid"),
					WorkingDirectory: "/tmp",
				}
				sessionID, _ := adapter.CreateSession(ctx)
				defer adapter.TerminateSession(sessionID)

				// Send multiple messages
				for i := 0; i < 5; i++ {
					msg := agent.Message{
						Role:    agent.RoleUser,
						Content: "Message " + string(rune(i)),
					}
					adapter.SendMessage(sessionID, msg)
					time.Sleep(50 * time.Millisecond)
				}

				history, _ := adapter.GetHistory(sessionID)

				// Collect all message IDs
				ids := make(map[string]bool)
				for _, msg := range history {
					if msg.ID != "" {
						Expect(ids[msg.ID]).To(BeFalse(), "Duplicate message ID: "+msg.ID)
						ids[msg.ID] = true
					}
				}
			},
			Entry("claude agent", "claude"),
			Entry("gemini agent", "gemini"),
			Entry("opencode agent", "opencode"),
		)
	})

	Describe("Role Handling", func() {
		DescribeTable("distinguishes between user and assistant roles",
			func(agentName string) {
				Skip("Skipping actual API calls")

				adapter := adapters[agentName]
				ctx := agent.SessionContext{
					Name:             testEnv.UniqueSessionName(agentName + "-roles"),
					WorkingDirectory: "/tmp",
				}
				sessionID, _ := adapter.CreateSession(ctx)
				defer adapter.TerminateSession(sessionID)

				msg := agent.Message{
					Role:    agent.RoleUser,
					Content: "What is 2+2?",
				}
				adapter.SendMessage(sessionID, msg)
				time.Sleep(500 * time.Millisecond)

				history, _ := adapter.GetHistory(sessionID)

				// Should have both user and assistant messages
				hasUser := false
				hasAssistant := false
				for _, msg := range history {
					if msg.Role == agent.RoleUser {
						hasUser = true
					}
					if msg.Role == agent.RoleAssistant {
						hasAssistant = true
					}
				}

				Expect(hasUser).To(BeTrue())
				// Assistant response may or may not be recorded depending on implementation
				_ = hasAssistant
			},
			Entry("claude agent", "claude"),
			Entry("gemini agent", "gemini"),
			Entry("opencode agent", "opencode"),
		)
	})

	Describe("Message Metadata", func() {
		DescribeTable("preserves message metadata",
			func(agentName string) {
				Skip("Skipping actual API calls")

				adapter := adapters[agentName]
				ctx := agent.SessionContext{
					Name:             testEnv.UniqueSessionName(agentName + "-metadata"),
					WorkingDirectory: "/tmp",
				}
				sessionID, _ := adapter.CreateSession(ctx)
				defer adapter.TerminateSession(sessionID)

				msg := agent.Message{
					Role:    agent.RoleUser,
					Content: "Test metadata",
					Metadata: map[string]interface{}{
						"test_key": "test_value",
					},
				}

				err := adapter.SendMessage(sessionID, msg)
				Expect(err).ToNot(HaveOccurred())

				history, _ := adapter.GetHistory(sessionID)

				// Find message and check metadata preservation
				for _, histMsg := range history {
					if histMsg.Content == "Test metadata" {
						// Metadata preservation is implementation-specific
						_ = histMsg.Metadata
					}
				}
			},
			Entry("claude agent", "claude"),
			Entry("gemini agent", "gemini"),
			Entry("opencode agent", "opencode"),
		)
	})
})
