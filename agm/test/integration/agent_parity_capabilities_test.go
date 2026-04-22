//go:build integration

package integration_test

import (
	"os"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vbonnet/dear-agent/agm/internal/agent"
)

var _ = Describe("Agent Parity - Capabilities", func() {
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

	Describe("Basic Metadata", func() {
		DescribeTable("returns valid agent name",
			func(agentName string) {
				adapter := adapters[agentName]
				name := adapter.Name()

				Expect(name).ToNot(BeEmpty())
				Expect(name).To(Equal(agentName))
			},
			Entry("claude agent", "claude"),
			Entry("gemini agent", "gemini"),
			Entry("opencode agent", "opencode"),
		)

		DescribeTable("returns valid version string",
			func(agentName string) {
				adapter := adapters[agentName]
				version := adapter.Version()

				Expect(version).ToNot(BeEmpty())
				// Version should contain model identifier
				Expect(version).To(MatchRegexp(`(?i)(claude|gemini|sonnet|flash|pro)`))
			},
			Entry("claude agent", "claude"),
			Entry("gemini agent", "gemini"),
			Entry("opencode agent", "opencode"),
		)
	})

	Describe("Capabilities Structure", func() {
		DescribeTable("returns valid capabilities struct",
			func(agentName string) {
				adapter := adapters[agentName]
				caps := adapter.Capabilities()

				Expect(caps.ModelName).ToNot(BeEmpty())
				Expect(caps.MaxContextWindow).To(BeNumerically(">", 0))
				Expect(caps.MaxContextWindow).To(BeNumerically("<", 10000000)) // Sanity check
			},
			Entry("claude agent", "claude"),
			Entry("gemini agent", "gemini"),
			Entry("opencode agent", "opencode"),
		)

		DescribeTable("model name matches version",
			func(agentName string) {
				adapter := adapters[agentName]
				caps := adapter.Capabilities()
				version := adapter.Version()

				// Capabilities model name should be related to version
				Expect(caps.ModelName).To(ContainSubstring(version))
			},
			Entry("claude agent", "claude"),
			Entry("gemini agent", "gemini"),
			Entry("opencode agent", "opencode"),
		)
	})

	Describe("Agent-Specific Capabilities", func() {
		It("claude supports slash commands (CLI agent)", func() {
			adapter := adapters["claude"]
			caps := adapter.Capabilities()

			Expect(caps.SupportsSlashCommands).To(BeTrue())
		})

		It("gemini does not support slash commands (API agent)", func() {
			adapter := adapters["gemini"]
			caps := adapter.Capabilities()

			Expect(caps.SupportsSlashCommands).To(BeFalse())
		})

		It("claude has smaller context window than gemini", func() {
			claudeCaps := adapters["claude"].Capabilities()
			geminiCaps := adapters["gemini"].Capabilities()

			// Claude: ~200K tokens
			// Gemini: 1M+ tokens
			Expect(claudeCaps.MaxContextWindow).To(BeNumerically("<", geminiCaps.MaxContextWindow))
		})
	})

	Describe("Common Feature Support", func() {
		DescribeTable("both agents support tools/functions",
			func(agentName string) {
				adapter := adapters[agentName]
				caps := adapter.Capabilities()

				Expect(caps.SupportsTools).To(BeTrue())
			},
			Entry("claude agent", "claude"),
			Entry("gemini agent", "gemini"),
			Entry("opencode agent", "opencode"),
		)

		DescribeTable("both agents support vision",
			func(agentName string) {
				adapter := adapters[agentName]
				caps := adapter.Capabilities()

				Expect(caps.SupportsVision).To(BeTrue())
			},
			Entry("claude agent", "claude"),
			Entry("gemini agent", "gemini"),
			Entry("opencode agent", "opencode"),
		)

		DescribeTable("both agents support streaming",
			func(agentName string) {
				adapter := adapters[agentName]
				caps := adapter.Capabilities()

				Expect(caps.SupportsStreaming).To(BeTrue())
			},
			Entry("claude agent", "claude"),
			Entry("gemini agent", "gemini"),
			Entry("opencode agent", "opencode"),
		)

		DescribeTable("both agents support system prompts",
			func(agentName string) {
				adapter := adapters[agentName]
				caps := adapter.Capabilities()

				Expect(caps.SupportsSystemPrompts).To(BeTrue())
			},
			Entry("claude agent", "claude"),
			Entry("gemini agent", "gemini"),
			Entry("opencode agent", "opencode"),
		)
	})

	Describe("Advanced Features", func() {
		DescribeTable("multimodal support",
			func(agentName string) {
				adapter := adapters[agentName]
				caps := adapter.Capabilities()

				// Document multimodal support status
				GinkgoWriter.Printf("%s multimodal support: %v\n", agentName, caps.SupportsMultimodal)

				// Both should report their actual multimodal capabilities
				// (audio, video beyond just images)
				_ = caps.SupportsMultimodal
			},
			Entry("claude agent", "claude"),
			Entry("gemini agent", "gemini"),
			Entry("opencode agent", "opencode"),
		)

		DescribeTable("hooks support",
			func(agentName string) {
				adapter := adapters[agentName]
				caps := adapter.Capabilities()

				// Hooks are AGM-level feature, not agent-specific
				// Both should report same value
				GinkgoWriter.Printf("%s hooks support: %v\n", agentName, caps.SupportsHooks)

				// Value should be consistent across agents
				_ = caps.SupportsHooks
			},
			Entry("claude agent", "claude"),
			Entry("gemini agent", "gemini"),
			Entry("opencode agent", "opencode"),
		)
	})

	Describe("Context Window Details", func() {
		It("claude has reasonable context window (100K-500K tokens)", func() {
			adapter := adapters["claude"]
			caps := adapter.Capabilities()

			Expect(caps.MaxContextWindow).To(BeNumerically(">=", 100000))
			Expect(caps.MaxContextWindow).To(BeNumerically("<=", 500000))
		})

		It("gemini has large context window (500K-2M tokens)", func() {
			adapter := adapters["gemini"]
			caps := adapter.Capabilities()

			Expect(caps.MaxContextWindow).To(BeNumerically(">=", 500000))
			Expect(caps.MaxContextWindow).To(BeNumerically("<=", 2000000))
		})
	})

	Describe("Model Naming Conventions", func() {
		It("claude model name contains 'claude'", func() {
			adapter := adapters["claude"]
			caps := adapter.Capabilities()

			Expect(caps.ModelName).To(MatchRegexp(`(?i)claude`))
		})

		It("gemini model name contains 'gemini'", func() {
			adapter := adapters["gemini"]
			caps := adapter.Capabilities()

			Expect(caps.ModelName).To(MatchRegexp(`(?i)gemini`))
		})

		It("model names indicate version/tier", func() {
			claudeCaps := adapters["claude"].Capabilities()
			geminiCaps := adapters["gemini"].Capabilities()

			// Should contain tier/version indicators
			Expect(claudeCaps.ModelName).To(MatchRegexp(`(?i)(sonnet|opus|haiku|3|4)`))
			Expect(geminiCaps.ModelName).To(MatchRegexp(`(?i)(pro|flash|ultra|1\.|2\.)`))
		})
	})

	Describe("Capabilities Consistency", func() {
		It("capabilities don't change across adapter instances", func() {
			// Create multiple Claude adapters
			adapter1, _ := agent.NewClaudeAdapter(nil)
			adapter2, _ := agent.NewClaudeAdapter(nil)

			caps1 := adapter1.Capabilities()
			caps2 := adapter2.Capabilities()

			// Capabilities should be identical
			Expect(caps1).To(Equal(caps2))

			// Same for Gemini
			os.Setenv("GEMINI_API_KEY", "test-key")
			gemini1, _ := agent.NewGeminiAdapter(&agent.GeminiConfig{APIKey: "test-key"})
			gemini2, _ := agent.NewGeminiAdapter(&agent.GeminiConfig{APIKey: "test-key"})

			geminiCaps1 := gemini1.Capabilities()
			geminiCaps2 := gemini2.Capabilities()

			Expect(geminiCaps1).To(Equal(geminiCaps2))
		})
	})

	Describe("Feature Documentation", func() {
		It("generates capability comparison matrix", func() {
			claudeCaps := adapters["claude"].Capabilities()
			geminiCaps := adapters["gemini"].Capabilities()

			GinkgoWriter.Println("\n=== Agent Capability Comparison ===")
			GinkgoWriter.Printf("%-30s | %-15s | %-15s\n", "Feature", "Claude", "Gemini")
			GinkgoWriter.Println(strings.Repeat("-", 65))
			GinkgoWriter.Printf("%-30s | %-15v | %-15v\n", "Slash Commands", claudeCaps.SupportsSlashCommands, geminiCaps.SupportsSlashCommands)
			GinkgoWriter.Printf("%-30s | %-15v | %-15v\n", "Hooks", claudeCaps.SupportsHooks, geminiCaps.SupportsHooks)
			GinkgoWriter.Printf("%-30s | %-15v | %-15v\n", "Tools", claudeCaps.SupportsTools, geminiCaps.SupportsTools)
			GinkgoWriter.Printf("%-30s | %-15v | %-15v\n", "Vision", claudeCaps.SupportsVision, geminiCaps.SupportsVision)
			GinkgoWriter.Printf("%-30s | %-15v | %-15v\n", "Multimodal", claudeCaps.SupportsMultimodal, geminiCaps.SupportsMultimodal)
			GinkgoWriter.Printf("%-30s | %-15v | %-15v\n", "Streaming", claudeCaps.SupportsStreaming, geminiCaps.SupportsStreaming)
			GinkgoWriter.Printf("%-30s | %-15v | %-15v\n", "System Prompts", claudeCaps.SupportsSystemPrompts, geminiCaps.SupportsSystemPrompts)
			GinkgoWriter.Printf("%-30s | %-15d | %-15d\n", "Max Context (tokens)", claudeCaps.MaxContextWindow, geminiCaps.MaxContextWindow)
			GinkgoWriter.Printf("%-30s | %-15s | %-15s\n", "Model", claudeCaps.ModelName, geminiCaps.ModelName)
		})
	})
})
