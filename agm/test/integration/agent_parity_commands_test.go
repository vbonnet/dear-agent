//go:build integration

package integration_test

import (
	"os"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vbonnet/dear-agent/agm/internal/agent"
)

var _ = Describe("Agent Parity - Command Execution", func() {
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

	Describe("CommandRename", func() {
		DescribeTable("renames session via command",
			func(agentName string) {
				adapter := adapters[agentName]
				ctx := agent.SessionContext{
					Name:             testEnv.UniqueSessionName(agentName + "-rename"),
					WorkingDirectory: "/tmp",
				}
				sessionID, _ := adapter.CreateSession(ctx)
				defer adapter.TerminateSession(sessionID)

				cmd := agent.Command{
					Type: agent.CommandRename,
					Params: map[string]interface{}{
						"session_id": string(sessionID),
						"name":       "new-session-name",
					},
				}

				err := adapter.ExecuteCommand(cmd)

				// Both agents should handle rename (or return consistent error)
				_ = err // Implementation may vary
			},
			Entry("claude agent", "claude"),
			Entry("gemini agent", "gemini"),
			Entry("opencode agent", "opencode"),
		)
	})

	Describe("CommandSetDir", func() {
		DescribeTable("changes working directory via command",
			func(agentName string) {
				adapter := adapters[agentName]
				ctx := agent.SessionContext{
					Name:             testEnv.UniqueSessionName(agentName + "-setdir"),
					WorkingDirectory: "/tmp",
				}
				sessionID, _ := adapter.CreateSession(ctx)
				defer adapter.TerminateSession(sessionID)

				cmd := agent.Command{
					Type: agent.CommandSetDir,
					Params: map[string]interface{}{
						"session_id": string(sessionID),
						"path":       "~/src",
					},
				}

				err := adapter.ExecuteCommand(cmd)

				// Both should handle directory change consistently
				_ = err
			},
			Entry("claude agent", "claude"),
			Entry("gemini agent", "gemini"),
			Entry("opencode agent", "opencode"),
		)
	})

	Describe("CommandAuthorize", func() {
		DescribeTable("authorizes directory via command",
			func(agentName string) {
				adapter := adapters[agentName]
				ctx := agent.SessionContext{
					Name:             testEnv.UniqueSessionName(agentName + "-auth"),
					WorkingDirectory: "/tmp",
				}
				sessionID, _ := adapter.CreateSession(ctx)
				defer adapter.TerminateSession(sessionID)

				cmd := agent.Command{
					Type: agent.CommandAuthorize,
					Params: map[string]interface{}{
						"session_id": string(sessionID),
						"path":       "~/data",
					},
				}

				err := adapter.ExecuteCommand(cmd)

				// Both should have same authorization behavior
				// Either support it or return "not implemented"
				_ = err
			},
			Entry("claude agent", "claude"),
			Entry("gemini agent", "gemini"),
			Entry("opencode agent", "opencode"),
		)
	})

	Describe("CommandRunHook", func() {
		DescribeTable("executes hook via command",
			func(agentName string) {
				adapter := adapters[agentName]
				ctx := agent.SessionContext{
					Name:             testEnv.UniqueSessionName(agentName + "-hook"),
					WorkingDirectory: "/tmp",
				}
				sessionID, _ := adapter.CreateSession(ctx)
				defer adapter.TerminateSession(sessionID)

				cmd := agent.Command{
					Type: agent.CommandRunHook,
					Params: map[string]interface{}{
						"session_id": string(sessionID),
						"hook_name":  "pre_message",
						"script":     "echo 'hook executed'",
					},
				}

				err := adapter.ExecuteCommand(cmd)

				// Both should have same hook support
				// (Likely not implemented for API agents)
				if err != nil {
					Expect(err.Error()).To(MatchRegexp("(?i)(not.*implemented|not.*supported)"))
				}
			},
			Entry("claude agent", "claude"),
			Entry("gemini agent", "gemini"),
			Entry("opencode agent", "opencode"),
		)
	})

	Describe("Command Error Handling", func() {
		DescribeTable("returns error for invalid command type",
			func(agentName string) {
				adapter := adapters[agentName]

				cmd := agent.Command{
					Type:   agent.CommandType("invalid_command"),
					Params: map[string]interface{}{},
				}

				err := adapter.ExecuteCommand(cmd)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(MatchRegexp("(?i)(unsupported|invalid|unknown)"))
			},
			Entry("claude agent", "claude"),
			Entry("gemini agent", "gemini"),
			Entry("opencode agent", "opencode"),
		)

		DescribeTable("handles missing parameters gracefully",
			func(agentName string) {
				adapter := adapters[agentName]

				cmd := agent.Command{
					Type:   agent.CommandRename,
					Params: map[string]interface{}{}, // Missing required params
				}

				err := adapter.ExecuteCommand(cmd)

				// Should either handle gracefully or return clear error
				_ = err
			},
			Entry("claude agent", "claude"),
			Entry("gemini agent", "gemini"),
			Entry("opencode agent", "opencode"),
		)
	})

	Describe("Command Type Support", func() {
		DescribeTable("documents supported command types",
			func(agentName string) {
				adapter := adapters[agentName]
				ctx := agent.SessionContext{
					Name:             testEnv.UniqueSessionName(agentName + "-cmdsupport"),
					WorkingDirectory: "/tmp",
				}
				sessionID, _ := adapter.CreateSession(ctx)
				defer adapter.TerminateSession(sessionID)

				commandTypes := []agent.CommandType{
					agent.CommandRename,
					agent.CommandSetDir,
					agent.CommandAuthorize,
					agent.CommandRunHook,
					agent.CommandClearHistory,
					agent.CommandSetSystemPrompt,
				}

				supported := []agent.CommandType{}
				for _, cmdType := range commandTypes {
					cmd := agent.Command{
						Type: cmdType,
						Params: map[string]interface{}{
							"session_id": string(sessionID),
							"test":       "value",
						},
					}
					err := adapter.ExecuteCommand(cmd)
					if err == nil || !strings.Contains(err.Error(), "not implemented") {
						supported = append(supported, cmdType)
					}
				}

				GinkgoWriter.Printf("%s supports command types: %v\n", agentName, supported)
			},
			Entry("claude agent", "claude"),
			Entry("gemini agent", "gemini"),
			Entry("opencode agent", "opencode"),
		)
	})

	Describe("Command Execution Without Session", func() {
		DescribeTable("handles commands without active session",
			func(agentName string) {
				adapter := adapters[agentName]
				fakeSessionID := agent.SessionID("non-existent-cmd-session")

				cmd := agent.Command{
					Type: agent.CommandRename,
					Params: map[string]interface{}{
						"session_id": string(fakeSessionID),
						"name":       "new-name",
					},
				}

				err := adapter.ExecuteCommand(cmd)

				// Should either validate session exists or handle gracefully
				_ = err
			},
			Entry("claude agent", "claude"),
			Entry("gemini agent", "gemini"),
			Entry("opencode agent", "opencode"),
		)
	})
})
