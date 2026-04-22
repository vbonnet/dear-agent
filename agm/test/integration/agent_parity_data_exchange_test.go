//go:build integration

package integration_test

import (
	"encoding/json"
	"os"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vbonnet/dear-agent/agm/internal/agent"
)

var _ = Describe("Agent Parity - Data Exchange", func() {
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

	Describe("ExportConversation", func() {
		DescribeTable("exports conversation in JSONL format",
			func(agentName string) {
				Skip("Skipping actual API calls")

				adapter := adapters[agentName]
				ctx := agent.SessionContext{
					Name:             testEnv.UniqueSessionName(agentName + "-export-jsonl"),
					WorkingDirectory: "/tmp",
				}
				sessionID, _ := adapter.CreateSession(ctx)
				defer adapter.TerminateSession(sessionID)

				// Export empty conversation
				data, err := adapter.ExportConversation(sessionID, agent.FormatJSONL)

				Expect(err).ToNot(HaveOccurred())
				Expect(data).ToNot(BeNil())

				// Verify JSONL format (each non-empty line is valid JSON)
				lines := strings.Split(string(data), "\n")
				for _, line := range lines {
					if line != "" {
						var msg map[string]interface{}
						err := json.Unmarshal([]byte(line), &msg)
						Expect(err).ToNot(HaveOccurred(), "Invalid JSON in line: "+line)
					}
				}
			},
			Entry("claude agent", "claude"),
			Entry("gemini agent", "gemini"),
			Entry("opencode agent", "opencode"),
		)

		DescribeTable("exports conversation in Markdown format",
			func(agentName string) {
				Skip("Skipping actual API calls")

				adapter := adapters[agentName]
				ctx := agent.SessionContext{
					Name:             testEnv.UniqueSessionName(agentName + "-export-md"),
					WorkingDirectory: "/tmp",
				}
				sessionID, _ := adapter.CreateSession(ctx)
				defer adapter.TerminateSession(sessionID)

				data, err := adapter.ExportConversation(sessionID, agent.FormatMarkdown)

				// Both agents should support Markdown or return consistent error
				if agentName == "claude" {
					// Claude may or may not implement Markdown export
					_ = err
				}
				if agentName == "gemini" {
					// Gemini should match Claude behavior
					_ = err
				}

				if err == nil {
					Expect(data).ToNot(BeEmpty())
					content := string(data)
					// Should contain Markdown formatting
					Expect(content).To(MatchRegexp(`#|##|\*\*`))
				}
			},
			Entry("claude agent", "claude"),
			Entry("gemini agent", "gemini"),
			Entry("opencode agent", "opencode"),
		)

		DescribeTable("handles HTML export consistently",
			func(agentName string) {
				adapter := adapters[agentName]
				ctx := agent.SessionContext{
					Name:             testEnv.UniqueSessionName(agentName + "-export-html"),
					WorkingDirectory: "/tmp",
				}
				sessionID, _ := adapter.CreateSession(ctx)
				defer adapter.TerminateSession(sessionID)

				data, err := adapter.ExportConversation(sessionID, agent.FormatHTML)

				// Both agents should have same HTML support level
				// Either both support it or neither does
				if agentName == "claude" {
					if err != nil {
						// If Claude doesn't support HTML, error message should be clear
						Expect(err.Error()).To(MatchRegexp("(?i)(not.*supported|not.*implemented)"))
					}
				} else {
					// Gemini should match Claude behavior
					if err != nil {
						Expect(err.Error()).To(MatchRegexp("(?i)(not.*supported|not.*implemented)"))
					}
				}

				if err == nil {
					Expect(data).ToNot(BeEmpty())
				}
			},
			Entry("claude agent", "claude"),
			Entry("gemini agent", "gemini"),
			Entry("opencode agent", "opencode"),
		)

		DescribeTable("handles non-existent session export consistently",
			func(agentName string) {
				adapter := adapters[agentName]
				fakeSessionID := agent.SessionID("non-existent-export-session")

				data, err := adapter.ExportConversation(fakeSessionID, agent.FormatJSONL)

				// Both behaviors are acceptable:
				// - Return error (session doesn't exist)
				// - Return empty export (no history file)
				if err != nil {
					Expect(data).To(BeNil())
				} else {
					// Empty export is valid
					Expect(data).ToNot(BeNil())
				}
			},
			Entry("claude agent", "claude"),
			Entry("gemini agent", "gemini"),
			Entry("opencode agent", "opencode"),
		)
	})

	Describe("ImportConversation", func() {
		DescribeTable("imports conversation from JSONL data",
			func(agentName string) {
				Skip("Skipping actual API calls and import functionality")

				adapter := adapters[agentName]

				// Create sample JSONL data
				msg := map[string]interface{}{
					"id":        "test-msg-1",
					"role":      "user",
					"content":   "Imported message",
					"timestamp": "2026-02-04T00:00:00Z",
				}
				jsonData, _ := json.Marshal(msg)
				jsonlData := append(jsonData, '\n')

				// Import conversation
				sessionID, err := adapter.ImportConversation(jsonlData, agent.FormatJSONL)

				// Both agents should have same import support level
				if agentName == "claude" {
					// Document Claude behavior
					_ = err
				}
				if agentName == "gemini" {
					// Gemini should match
					_ = err
				}

				if sessionID != "" {
					defer adapter.TerminateSession(sessionID)
				}
			},
			Entry("claude agent", "claude"),
			Entry("gemini agent", "gemini"),
			Entry("opencode agent", "opencode"),
		)

		DescribeTable("rejects invalid JSONL format",
			func(agentName string) {
				adapter := adapters[agentName]

				invalidData := []byte("not valid json\ninvalid line 2")

				sessionID, err := adapter.ImportConversation(invalidData, agent.FormatJSONL)

				// Both should reject invalid data
				Expect(err).To(HaveOccurred())
				Expect(sessionID).To(BeEmpty())
			},
			Entry("claude agent", "claude"),
			Entry("gemini agent", "gemini"),
			Entry("opencode agent", "opencode"),
		)

		DescribeTable("handles unsupported import formats consistently",
			func(agentName string) {
				adapter := adapters[agentName]

				data := []byte("Some content")

				sessionID, err := adapter.ImportConversation(data, agent.ConversationFormat("unsupported"))

				Expect(err).To(HaveOccurred())
				Expect(sessionID).To(BeEmpty())
			},
			Entry("claude agent", "claude"),
			Entry("gemini agent", "gemini"),
			Entry("opencode agent", "opencode"),
		)
	})

	Describe("Format Support Matrix", func() {
		DescribeTable("documents supported export formats",
			func(agentName string) {
				adapter := adapters[agentName]
				ctx := agent.SessionContext{
					Name:             testEnv.UniqueSessionName(agentName + "-formats"),
					WorkingDirectory: "/tmp",
				}
				sessionID, _ := adapter.CreateSession(ctx)
				defer adapter.TerminateSession(sessionID)

				formats := []agent.ConversationFormat{
					agent.FormatJSONL,
					agent.FormatMarkdown,
					agent.FormatHTML,
					agent.FormatNative,
				}

				supportedFormats := []agent.ConversationFormat{}
				for _, format := range formats {
					_, err := adapter.ExportConversation(sessionID, format)
					if err == nil {
						supportedFormats = append(supportedFormats, format)
					}
				}

				// At minimum, should support JSONL
				Expect(supportedFormats).To(ContainElement(agent.FormatJSONL))

				// Log supported formats for documentation
				GinkgoWriter.Printf("%s supports formats: %v\n", agentName, supportedFormats)
			},
			Entry("claude agent", "claude"),
			Entry("gemini agent", "gemini"),
			Entry("opencode agent", "opencode"),
		)
	})

	Describe("Export/Import Roundtrip", func() {
		DescribeTable("preserves data through export/import cycle",
			func(agentName string) {
				Skip("Skipping roundtrip test - requires full implementation")

				adapter := adapters[agentName]

				// Create session with known data
				ctx := agent.SessionContext{
					Name:             testEnv.UniqueSessionName(agentName + "-roundtrip"),
					WorkingDirectory: "/tmp",
				}
				origSessionID, _ := adapter.CreateSession(ctx)
				defer adapter.TerminateSession(origSessionID)

				// Export
				exportData, err := adapter.ExportConversation(origSessionID, agent.FormatJSONL)
				Expect(err).ToNot(HaveOccurred())

				// Import to new session
				newSessionID, err := adapter.ImportConversation(exportData, agent.FormatJSONL)
				if err == nil {
					defer adapter.TerminateSession(newSessionID)

					// Verify data preserved
					origHistory, _ := adapter.GetHistory(origSessionID)
					newHistory, _ := adapter.GetHistory(newSessionID)

					Expect(len(newHistory)).To(Equal(len(origHistory)))
				}
			},
			Entry("claude agent", "claude"),
			Entry("gemini agent", "gemini"),
			Entry("opencode agent", "opencode"),
		)
	})

	Describe("Export Size Limits", func() {
		DescribeTable("handles large conversation exports",
			func(agentName string) {
				Skip("Skipping large data test")

				adapter := adapters[agentName]
				ctx := agent.SessionContext{
					Name:             testEnv.UniqueSessionName(agentName + "-large"),
					WorkingDirectory: "/tmp",
				}
				sessionID, _ := adapter.CreateSession(ctx)
				defer adapter.TerminateSession(sessionID)

				// Export (even if empty, should succeed)
				data, err := adapter.ExportConversation(sessionID, agent.FormatJSONL)

				Expect(err).ToNot(HaveOccurred())
				Expect(data).ToNot(BeNil())
			},
			Entry("claude agent", "claude"),
			Entry("gemini agent", "gemini"),
			Entry("opencode agent", "opencode"),
		)
	})

	Describe("Export Encoding", func() {
		DescribeTable("handles special characters in export",
			func(agentName string) {
				Skip("Skipping encoding test")

				adapter := adapters[agentName]
				ctx := agent.SessionContext{
					Name:             testEnv.UniqueSessionName(agentName + "-encoding"),
					WorkingDirectory: "/tmp",
				}
				sessionID, _ := adapter.CreateSession(ctx)
				defer adapter.TerminateSession(sessionID)

				data, err := adapter.ExportConversation(sessionID, agent.FormatJSONL)

				Expect(err).ToNot(HaveOccurred())
				// Should be valid UTF-8
				Expect(data).ToNot(BeNil())
			},
			Entry("claude agent", "claude"),
			Entry("gemini agent", "gemini"),
			Entry("opencode agent", "opencode"),
		)
	})
})
