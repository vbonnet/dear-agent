package context

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDetectCLI(t *testing.T) {
	registry := createTestRegistry(t)
	detector := NewDetector(registry)

	tests := []struct {
		name     string
		envVar   string
		envValue string
		expected CLI
	}{
		{"Claude", "CLAUDE_SESSION_ID", "test-123", CLIClaude},
		{"Gemini", "GEMINI_SESSION_ID", "test-456", CLIGemini},
		{"OpenCode", "OPENCODE_SESSION_ID", "test-789", CLIOpenCode},
		{"Codex", "CODEX_SESSION_ID", "test-abc", CLICodex},
		{"Pi", "PI_SESSION_ID", "test-pi", CLIPi},
		{"Antigravity", "AGY_CONVERSATION_ID", "test-agy", CLIAgy},
		{"Unknown", "", "", CLIUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear all env vars
			t.Setenv("CLAUDE_SESSION_ID", "") // restored on test cleanup
			os.Unsetenv("CLAUDE_SESSION_ID")
			t.Setenv("GEMINI_SESSION_ID", "") // restored on test cleanup
			os.Unsetenv("GEMINI_SESSION_ID")
			t.Setenv("OPENCODE_SESSION_ID", "") // restored on test cleanup
			os.Unsetenv("OPENCODE_SESSION_ID")
			t.Setenv("CODEX_SESSION_ID", "") // restored on test cleanup
			os.Unsetenv("CODEX_SESSION_ID")
			t.Setenv("PI_SESSION_ID", "")
			os.Unsetenv("PI_SESSION_ID")

			// Set test env var
			if tt.envVar != "" {
				t.Setenv(tt.envVar, tt.envValue)
				defer os.Unsetenv(tt.envVar)
			}

			cli := detector.DetectCLI()
			assert.Equal(t, tt.expected, cli)
		})
	}
}

func TestExtractFromSystemReminder(t *testing.T) {
	registry := createTestRegistry(t)
	detector := NewDetector(registry)

	tests := []struct {
		name          string
		text          string
		expectedUsed  int
		expectedTotal int
		expectedModel string
		shouldFail    bool
	}{
		{
			name: "valid system reminder",
			text: `<system-reminder>Token usage: 42184/200000; 157816 remaining</system-reminder>
You are powered by the model named Sonnet 4.5. The exact model ID is claude-sonnet-4-5@20250929.`,
			expectedUsed:  42184,
			expectedTotal: 200000,
			expectedModel: "claude-sonnet-4-5",
			shouldFail:    false,
		},
		{
			name:          "without model ID",
			text:          `Token usage: 100000/200000; 100000 remaining`,
			expectedUsed:  100000,
			expectedTotal: 200000,
			expectedModel: "claude-sonnet-4.5", // Default fallback
			shouldFail:    false,
		},
		{
			name:       "invalid format",
			text:       "No token usage here",
			shouldFail: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			usage, err := detector.extractFromSystemReminder(tt.text, "test-session")

			if tt.shouldFail {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expectedUsed, usage.UsedTokens)
			assert.Equal(t, tt.expectedTotal, usage.TotalTokens)
			assert.Equal(t, tt.expectedModel, usage.ModelID)
			assert.Equal(t, "claude-cli", usage.Source)
			assert.Equal(t, "test-session", usage.SessionID)

			// Check percentage calculation
			expectedPct := float64(tt.expectedUsed) / float64(tt.expectedTotal) * 100.0
			assert.InDelta(t, expectedPct, usage.PercentageUsed, 0.1)
		})
	}
}

func TestExtractModelIDFromText(t *testing.T) {
	registry := createTestRegistry(t)
	detector := NewDetector(registry)

	tests := []struct {
		name     string
		text     string
		expected string
	}{
		{
			"exact model ID",
			"The exact model ID is claude-sonnet-4-5@20250929.",
			"claude-sonnet-4-5",
		},
		{
			"model ID with version stripped",
			"The exact model ID is claude-opus-4-6@20260101.",
			"claude-opus-4-6",
		},
		{
			"fallback to model name - Sonnet 4.5",
			"You are powered by Sonnet 4.5.",
			"claude-sonnet-4.5",
		},
		{
			"fallback to model name - Opus 4.6",
			"You are powered by Opus 4.6.",
			"claude-opus-4.6",
		},
		{
			"fallback to model name - Sonnet 4.6",
			"You are powered by Sonnet 4.6.",
			"claude-sonnet-4.6",
		},
		{
			"fallback to model name - Haiku 4.5",
			"You are powered by Haiku 4.5.",
			"claude-haiku-4.5",
		},
		{
			"no model found",
			"Some other text",
			"",
		},
		{
			"model ID without version suffix",
			"The exact model ID is claude-sonnet-4-5.",
			"claude-sonnet-4-5",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := detector.extractModelIDFromText(tt.text)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestEstimateFromMessageCount(t *testing.T) {
	tests := []struct {
		messageCount   int
		maxTokens      int
		expectedTokens int
	}{
		{10, 200000, 1950},     // 10 * 150 * 1.3
		{50, 200000, 9750},     // 50 * 150 * 1.3
		{100, 200000, 19500},   // 100 * 150 * 1.3
		{2000, 200000, 200000}, // Capped at max
		{0, 200000, 0},         // Zero messages
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			usage := EstimateFromMessageCount(tt.messageCount, tt.maxTokens)

			assert.Equal(t, tt.maxTokens, usage.TotalTokens)
			assert.Equal(t, tt.expectedTokens, usage.UsedTokens)
			assert.Equal(t, "heuristic", usage.Source)

			expectedPct := float64(tt.expectedTokens) / float64(tt.maxTokens) * 100.0
			assert.InDelta(t, expectedPct, usage.PercentageUsed, 0.1)
		})
	}
}

func TestDetectFromHeuristic(t *testing.T) {
	registry := createTestRegistry(t)
	detector := NewDetector(registry)

	usage, err := detector.DetectFromHeuristic()
	require.NoError(t, err)
	assert.NotNil(t, usage)
	assert.Equal(t, "heuristic", usage.Source)
	assert.Equal(t, "default", usage.ModelID)
	assert.Equal(t, 200000, usage.TotalTokens)
	assert.Greater(t, usage.UsedTokens, 0)
	assert.Greater(t, usage.PercentageUsed, 0.0)
}

func TestDetect(t *testing.T) {
	registry := createTestRegistry(t)
	detector := NewDetector(registry)

	clearHarnessSessionIDs(t)

	usage, err := detector.Detect()
	require.NoError(t, err)
	assert.NotNil(t, usage)
	assert.Equal(t, "heuristic", usage.Source)
}

func TestDetectFromClaudeNoSession(t *testing.T) {
	registry := createTestRegistry(t)
	detector := NewDetector(registry)

	t.Setenv("CLAUDE_SESSION_ID", "") // restored on test cleanup
	os.Unsetenv("CLAUDE_SESSION_ID")

	_, err := detector.DetectFromClaude()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "CLAUDE_SESSION_ID not set")
}

func TestDetectFromGeminiUsesMarkedHeuristic(t *testing.T) {
	registry := createTestRegistry(t)
	detector := NewDetector(registry)

	clearHarnessSessionIDs(t)
	t.Setenv("GEMINI_SESSION_ID", "test-session-123")
	usage, err := detector.Detect()
	require.NoError(t, err)
	assert.Equal(t, "gemini-cli", usage.Source)
	assert.Equal(t, "gemini-3.5-flash", usage.ModelID)
	assert.Equal(t, "test-session-123", usage.SessionID)
	assert.True(t, usage.Estimated)
}

func TestDetectFromOpenCodeUsesMarkedHeuristic(t *testing.T) {
	registry := createTestRegistry(t)
	detector := NewDetector(registry)

	clearHarnessSessionIDs(t)
	t.Setenv("OPENCODE_SESSION_ID", "test-session-456")
	usage, err := detector.Detect()
	require.NoError(t, err)
	assert.Equal(t, "opencode-cli", usage.Source)
	assert.Equal(t, "z-ai/glm-5.2", usage.ModelID)
	assert.True(t, usage.Estimated)
}

func TestDetectFromCodexUsesMarkedHeuristic(t *testing.T) {
	registry := createTestRegistry(t)
	detector := NewDetector(registry)

	clearHarnessSessionIDs(t)
	t.Setenv("CODEX_SESSION_ID", "test-session-789")
	usage, err := detector.Detect()
	require.NoError(t, err)
	assert.Equal(t, "codex-cli", usage.Source)
	assert.Equal(t, "gpt-5.5", usage.ModelID)
	assert.True(t, usage.Estimated)
}

func TestDetectFromPiUsesMarkedHeuristic(t *testing.T) {
	registry := createTestRegistry(t)
	detector := NewDetector(registry)

	clearHarnessSessionIDs(t)
	t.Setenv("PI_SESSION_ID", "test-pi-session")
	usage, err := detector.Detect()
	require.NoError(t, err)
	assert.Equal(t, "pi-cli", usage.Source)
	assert.Equal(t, "claude-sonnet-4.6", usage.ModelID)
	assert.Equal(t, "test-pi-session", usage.SessionID)
	assert.True(t, usage.Estimated)
}

func clearHarnessSessionIDs(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"CLAUDE_SESSION_ID",
		"GEMINI_SESSION_ID",
		"OPENCODE_SESSION_ID",
		"CODEX_SESSION_ID",
		"PI_SESSION_ID",
		"AGY_CONVERSATION_ID",
		"AGY_SESSION_ID",
		"ANTIGRAVITY_SESSION_ID",
	} {
		t.Setenv(key, "")
	}
}

func TestDetectFromSessionUnsupportedCLI(t *testing.T) {
	registry := createTestRegistry(t)
	detector := NewDetector(registry)

	_, err := detector.DetectFromSession("test-session", CLIUnknown)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported CLI type")
}

func TestDetectFromClaudeSessionNoToolResult(t *testing.T) {
	registry := createTestRegistry(t)
	detector := NewDetector(registry)

	// Ensure no CLAUDE_TOOL_RESULT env var
	t.Setenv("CLAUDE_TOOL_RESULT", "") // restored on test cleanup
	os.Unsetenv("CLAUDE_TOOL_RESULT")

	// This will try to read a conversation file that doesn't exist
	_, err := detector.DetectFromClaudeSession("nonexistent-session-id")
	assert.Error(t, err)
}

func TestDetectFromClaudeSessionWithToolResult(t *testing.T) {
	registry := createTestRegistry(t)
	detector := NewDetector(registry)

	t.Setenv("CLAUDE_TOOL_RESULT", "Token usage: 50000/200000; 150000 remaining")
	defer os.Unsetenv("CLAUDE_TOOL_RESULT")

	usage, err := detector.DetectFromClaudeSession("test-session")
	require.NoError(t, err)
	assert.Equal(t, 50000, usage.UsedTokens)
	assert.Equal(t, 200000, usage.TotalTokens)
	assert.Equal(t, "claude-cli", usage.Source)
	assert.Equal(t, "test-session", usage.SessionID)
}

func TestDetectWithModel(t *testing.T) {
	registry := createTestRegistry(t)
	detector := NewDetector(registry)

	// Clear all CLI env vars to get heuristic fallback
	t.Setenv("CLAUDE_SESSION_ID", "") // restored on test cleanup
	os.Unsetenv("CLAUDE_SESSION_ID")
	t.Setenv("GEMINI_SESSION_ID", "") // restored on test cleanup
	os.Unsetenv("GEMINI_SESSION_ID")
	t.Setenv("OPENCODE_SESSION_ID", "") // restored on test cleanup
	os.Unsetenv("OPENCODE_SESSION_ID")
	t.Setenv("CODEX_SESSION_ID", "") // restored on test cleanup
	os.Unsetenv("CODEX_SESSION_ID")

	usage, err := detector.DetectWithModel("test-model")
	require.NoError(t, err)
	assert.Equal(t, "test-model", usage.ModelID)
	assert.Equal(t, 200000, usage.TotalTokens)
}

func TestDetectFromSession(t *testing.T) {
	registry := createTestRegistry(t)
	detector := NewDetector(registry)

	for _, tt := range []struct {
		cli    CLI
		source string
	}{
		{CLIGemini, "gemini-cli"},
		{CLIOpenCode, "opencode-cli"},
		{CLICodex, "codex-cli"},
		{CLIPi, "pi-cli"},
		{CLIAgy, "agy"},
	} {
		t.Run(string(tt.cli), func(t *testing.T) {
			usage, err := detector.DetectFromSession("session", tt.cli)
			require.NoError(t, err)
			assert.Equal(t, tt.source, usage.Source)
			assert.Equal(t, "session", usage.SessionID)
			assert.True(t, usage.Estimated)
		})
	}

	t.Run("unsupported CLI", func(t *testing.T) {
		_, err := detector.DetectFromSession("session-4", CLIUnknown)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported CLI type")
	})

	t.Run("claude session without file uses heuristic", func(t *testing.T) {
		os.Unsetenv("CLAUDE_TOOL_RESULT")
		usage, err := detector.DetectFromSession("nonexistent-session", CLIClaude)
		require.NoError(t, err)
		assert.True(t, usage.Estimated)
	})
}

func TestExtractFromConversationFile(t *testing.T) {
	registry := createTestRegistry(t)
	detector := NewDetector(registry)

	// Test with a nonexistent session
	_, err := detector.extractFromConversationFile("completely-nonexistent-session-id-12345")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "conversation file not found")
}

func TestExtractFromConversationFileWithRealFile(t *testing.T) {
	registry := createTestRegistry(t)
	detector := NewDetector(registry)

	// Create a fake conversation file in the expected location
	home, err := os.UserHomeDir()
	require.NoError(t, err)

	sessionID := "test-conv-session-xyz"
	sessDir := filepath.Join(home, ".claude", "sessions", sessionID)
	err = os.MkdirAll(sessDir, 0755)
	require.NoError(t, err)
	defer os.RemoveAll(sessDir)

	convPath := filepath.Join(sessDir, "conversation.jsonl")
	convContent := `{"role": "system", "content": "Token usage: 30000/200000; 170000 remaining\nThe exact model ID is claude-sonnet-4-5@20250929."}`
	err = os.WriteFile(convPath, []byte(convContent), 0644)
	require.NoError(t, err)

	usage, err := detector.extractFromConversationFile(sessionID)
	require.NoError(t, err)
	assert.Equal(t, 30000, usage.UsedTokens)
	assert.Equal(t, 200000, usage.TotalTokens)
	assert.Equal(t, "claude-sonnet-4-5", usage.ModelID)
}

func TestDetectDispatchesByCLI(t *testing.T) {
	registry := createTestRegistry(t)
	detector := NewDetector(registry)

	// Clear all env vars first
	t.Setenv("CLAUDE_SESSION_ID", "") // restored on test cleanup
	os.Unsetenv("CLAUDE_SESSION_ID")
	os.Unsetenv("GEMINI_SESSION_ID")
	os.Unsetenv("OPENCODE_SESSION_ID")
	t.Setenv("CODEX_SESSION_ID", "") // restored on test cleanup
	os.Unsetenv("CODEX_SESSION_ID")

	t.Run("dispatches to gemini", func(t *testing.T) {
		t.Setenv("GEMINI_SESSION_ID", "g-session")
		defer os.Unsetenv("GEMINI_SESSION_ID")

		usage, err := detector.Detect()
		require.NoError(t, err)
		assert.Equal(t, "gemini-cli", usage.Source)
	})

	t.Run("dispatches to opencode", func(t *testing.T) {
		t.Setenv("OPENCODE_SESSION_ID", "oc-session")
		defer os.Unsetenv("OPENCODE_SESSION_ID")

		usage, err := detector.Detect()
		require.NoError(t, err)
		assert.Equal(t, "opencode-cli", usage.Source)
	})

	t.Run("dispatches to codex", func(t *testing.T) {
		t.Setenv("CODEX_SESSION_ID", "cx-session")
		defer os.Unsetenv("CODEX_SESSION_ID")

		usage, err := detector.Detect()
		require.NoError(t, err)
		assert.Equal(t, "codex-cli", usage.Source)
	})
}

func TestPortableContextUsagePayloads(t *testing.T) {
	detector := NewDetector(createTestRegistry(t))

	t.Run("nested JSON", func(t *testing.T) {
		t.Setenv("CODEX_CONTEXT_USAGE", `{"context_window":{"used_tokens":75000,"total_tokens":200000},"model_id":"gpt-5.5"}`)
		usage, err := detector.DetectFromSession("codex-session", CLICodex)
		require.NoError(t, err)
		assert.Equal(t, 75000, usage.UsedTokens)
		assert.Equal(t, 200000, usage.TotalTokens)
		assert.Equal(t, "gpt-5.5", usage.ModelID)
		assert.False(t, usage.Estimated)
	})

	t.Run("text counters with commas", func(t *testing.T) {
		t.Setenv("OPENCODE_CONTEXT_USAGE", "Token usage: 125,000 / 200,000")
		usage, err := detector.DetectFromSession("opencode-session", CLIOpenCode)
		require.NoError(t, err)
		assert.Equal(t, 125000, usage.UsedTokens)
		assert.InDelta(t, 62.5, usage.PercentageUsed, 0.01)
		assert.False(t, usage.Estimated)
	})

	t.Run("invalid explicit counters fail loudly", func(t *testing.T) {
		t.Setenv("GEMINI_CONTEXT_USAGE", `{"used_tokens":200001,"total_tokens":200000}`)
		_, err := detector.DetectFromSession("gemini-session", CLIGemini)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid gemini-cli context usage")
	})

	t.Run("Pi structured counters", func(t *testing.T) {
		t.Setenv("PI_CONTEXT_USAGE", `{"context":{"used_tokens":32000,"total_tokens":1000000},"model_id":"claude-sonnet-4.6"}`)
		usage, err := detector.DetectFromSession("pi-session", CLIPi)
		require.NoError(t, err)
		assert.Equal(t, 32000, usage.UsedTokens)
		assert.Equal(t, 1000000, usage.TotalTokens)
		assert.Equal(t, "pi-cli", usage.Source)
		assert.Equal(t, "pi-session", usage.SessionID)
		assert.False(t, usage.Estimated)
	})
}

func TestParseUsageJSONRejectsCountersOutsidePlatformIntRange(t *testing.T) {
	tooLarge := "2147483648"
	if strconv.IntSize == 64 {
		tooLarge = "9223372036854775808"
	}

	_, _, _, ok := parseUsageJSON(fmt.Sprintf(`{"used_tokens":1,"total_tokens":%s}`, tooLarge))
	assert.False(t, ok)
}

func TestParseUsageJSONSelectsNestedCountersDeterministically(t *testing.T) {
	payload := `{"z_route":{"used_tokens":900,"total_tokens":1000,"model_id":"z-model"},"a_route":{"used_tokens":100,"total_tokens":1000,"model_id":"a-model"}}`

	for range 100 {
		used, total, modelID, ok := parseUsageJSON(payload)
		require.True(t, ok)
		assert.Equal(t, 100, used)
		assert.Equal(t, 1000, total)
		assert.Equal(t, "a-model", modelID)
	}
}

func TestCLIConstants(t *testing.T) {
	assert.Equal(t, CLI("claude"), CLIClaude)
	assert.Equal(t, CLI("gemini"), CLIGemini)
	assert.Equal(t, CLI("opencode"), CLIOpenCode)
	assert.Equal(t, CLI("codex"), CLICodex)
	assert.Equal(t, CLI("pi"), CLIPi)
	assert.Equal(t, CLI("agy"), CLIAgy)
	assert.Equal(t, CLI("unknown"), CLIUnknown)
}

func TestZoneConstants(t *testing.T) {
	assert.Equal(t, Zone("safe"), ZoneSafe)
	assert.Equal(t, Zone("warning"), ZoneWarning)
	assert.Equal(t, Zone("danger"), ZoneDanger)
	assert.Equal(t, Zone("critical"), ZoneCritical)
}

func TestPhaseStateConstants(t *testing.T) {
	assert.Equal(t, PhaseState("start"), PhaseStart)
	assert.Equal(t, PhaseState("middle"), PhaseMiddle)
	assert.Equal(t, PhaseState("end"), PhaseEnd)
}
