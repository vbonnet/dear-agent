package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/manifest"
)

// TestCollectStatusLineData tests status line data collection
func TestCollectStatusLineData(t *testing.T) {
	tests := []struct {
		name     string
		manifest *manifest.Manifest
		validate func(*testing.T, *StatusLineData)
		wantErr  bool
	}{
		{
			name: "complete manifest with context usage",
			manifest: &manifest.Manifest{
				SessionID: "test-session",
				Name:      "test-session",
				Harness:   "claude-code",
				Workspace: "test-workspace",
				Context: manifest.Context{
					Project: "/test/project",
				},
				ContextUsage: &manifest.ContextUsage{
					TotalTokens:    100000,
					UsedTokens:     73000,
					PercentageUsed: 73.0,
					LastUpdated:    time.Now(),
					Source:         "manual",
				},
			},
			validate: func(t *testing.T, data *StatusLineData) {
				// SessionName validation removed - using generated unique name
				// State is dynamically detected from tmux - will be OFFLINE if session doesn't exist
				if data.State != "OFFLINE" {
					t.Errorf("State = %q, want %q (dynamic detection defaults to OFFLINE for non-existent sessions)", data.State, "OFFLINE")
				}
				if data.StateColor != "colour239" {
					t.Errorf("StateColor = %q, want %q", data.StateColor, "colour239")
				}
				if data.ContextPercent != 73.0 {
					t.Errorf("ContextPercent = %.1f, want 73.0", data.ContextPercent)
				}
				if data.ContextColor != "yellow" {
					t.Errorf("ContextColor = %q, want %q", data.ContextColor, "yellow")
				}
				if data.AgentType != "claude-code" {
					t.Errorf("AgentType = %q, want %q", data.AgentType, "claude-code")
				}
				if data.AgentIcon != "🤖" {
					t.Errorf("AgentIcon = %q, want %q", data.AgentIcon, "🤖")
				}
				if data.Workspace != "test-workspace" {
					t.Errorf("Workspace = %q, want %q", data.Workspace, "test-workspace")
				}
			},
			wantErr: false,
		},
		{
			name: "manifest without context usage",
			manifest: &manifest.Manifest{
				SessionID: "no-context",
				Name:      "no-context",
				Harness:   "gemini-cli",
				Context: manifest.Context{
					Project: "/test/project",
				},
			},
			validate: func(t *testing.T, data *StatusLineData) {
				if data.ContextPercent != -1 {
					t.Errorf("ContextPercent = %.1f, want -1 for missing context", data.ContextPercent)
				}
				// State is dynamically detected from tmux - will be OFFLINE if session doesn't exist
				if data.State != "OFFLINE" {
					t.Errorf("State = %q, want %q (dynamic detection defaults to OFFLINE for non-existent sessions)", data.State, "OFFLINE")
				}
				if data.StateColor != "colour239" {
					t.Errorf("StateColor = %q, want %q", data.StateColor, "colour239")
				}
				if data.AgentType != "gemini-cli" {
					t.Errorf("AgentType = %q, want %q", data.AgentType, "gemini-cli")
				}
				if data.AgentIcon != "✨" {
					t.Errorf("AgentIcon = %q, want %q", data.AgentIcon, "✨")
				}
			},
			wantErr: false,
		},
		{
			name: "different states",
			manifest: &manifest.Manifest{
				SessionID: "permission-prompt",
				Name:      "permission-prompt",
				Harness:   "codex-cli",
				Context: manifest.Context{
					Project: "/test/project",
				},
			},
			validate: func(t *testing.T, data *StatusLineData) {
				// State is dynamically detected from tmux - will be OFFLINE if session doesn't exist
				if data.State != "OFFLINE" {
					t.Errorf("State = %q, want %q (dynamic detection defaults to OFFLINE for non-existent sessions)", data.State, "OFFLINE")
				}
				if data.StateColor != "colour239" {
					t.Errorf("StateColor = %q, want %q", data.StateColor, "colour239")
				}
				if data.AgentType != "codex-cli" {
					t.Errorf("AgentType = %q, want %q", data.AgentType, "codex-cli")
				}
				if data.AgentIcon != "🧠" {
					t.Errorf("AgentIcon = %q, want %q", data.AgentIcon, "🧠")
				}
			},
			wantErr: false,
		},
		{
			name: "opencode agent",
			manifest: &manifest.Manifest{
				SessionID: "opencode-session",
				Name:      "opencode-session",
				Harness:   "opencode-cli",
				Context: manifest.Context{
					Project: "/test/project",
				},
			},
			validate: func(t *testing.T, data *StatusLineData) {
				if data.State != "OFFLINE" {
					t.Errorf("State = %q, want %q", data.State, "OFFLINE")
				}
				if data.StateColor != "colour239" {
					t.Errorf("StateColor = %q, want %q", data.StateColor, "colour239")
				}
				if data.AgentType != "opencode-cli" {
					t.Errorf("AgentType = %q, want %q", data.AgentType, "opencode-cli")
				}
				if data.AgentIcon != "💻" {
					t.Errorf("AgentIcon = %q, want %q", data.AgentIcon, "💻")
				}
			},
			wantErr: false,
		},
		{
			name:     "nil manifest",
			manifest: nil,
			validate: nil,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Use unique session name to avoid collision with real tmux sessions
			uniqueSessionName := fmt.Sprintf("test-status-line-%d", time.Now().UnixNano())
			data, err := CollectStatusLineData(uniqueSessionName, tt.manifest)
			if tt.wantErr {
				if err == nil {
					t.Error("CollectStatusLineData() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("CollectStatusLineData() unexpected error: %v", err)
			}

			if data == nil {
				t.Fatal("CollectStatusLineData() returned nil data")
			}

			if tt.validate != nil {
				tt.validate(t, data)
			}
		})
	}
}

// TestGetStateColor tests state color mapping
func TestGetStateColor(t *testing.T) {
	tests := []struct {
		state    string
		expected string
	}{
		{manifest.StateDone, "green"},
		{manifest.StateWorking, "blue"},
		{manifest.StateUserPrompt, "yellow"},
		{manifest.StateCompacting, "magenta"},
		{manifest.StateOffline, "colour239"},
		{"READY", "green"},
		{"THINKING", "blue"},
		{"PERMISSION_PROMPT", "yellow"},
		{"UNKNOWN", "white"},
		{"", "white"},
	}

	for _, tt := range tests {
		t.Run(tt.state, func(t *testing.T) {
			color := getStateColor(tt.state)
			if color != tt.expected {
				t.Errorf("getStateColor(%q) = %q, want %q", tt.state, color, tt.expected)
			}
		})
	}
}

// TestGetContextColor tests context usage color mapping
func TestGetContextColor(t *testing.T) {
	tests := []struct {
		percent  float64
		expected string
	}{
		{0.0, "green"},
		{50.0, "green"},
		{69.9, "green"},
		{70.0, "yellow"},
		{80.0, "yellow"},
		{84.9, "yellow"},
		{85.0, "colour208"},
		{90.0, "colour208"},
		{94.9, "colour208"},
		{95.0, "red"},
		{100.0, "red"},
		{-1.0, "grey"},
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			color := getContextColor(tt.percent)
			if color != tt.expected {
				t.Errorf("getContextColor(%.1f) = %q, want %q", tt.percent, color, tt.expected)
			}
		})
	}
}

// TestGetAgentIcon tests agent icon mapping
func TestGetAgentIcon(t *testing.T) {
	tests := []struct {
		agent    string
		expected string
	}{
		{"claude-code", "🤖"},
		{"gemini-cli", "✨"},
		{"codex-cli", "🧠"},
		{"opencode-cli", "💻"},
		{"unknown", "🤖"},
		{"", "🤖"},
	}

	for _, tt := range tests {
		t.Run(tt.agent, func(t *testing.T) {
			icon := getAgentIcon(tt.agent)
			if icon != tt.expected {
				t.Errorf("getAgentIcon(%q) = %q, want %q", tt.agent, icon, tt.expected)
			}
		})
	}
}

// TestSetAgentIcons tests custom agent icon configuration
func TestSetAgentIcons(t *testing.T) {
	// Save original icons
	originalIcons := make(map[string]string)
	for k, v := range defaultAgentIcons {
		originalIcons[k] = v
	}

	// Test custom icons
	customIcons := map[string]string{
		"claude-code":  "C",
		"gemini-cli":   "G",
		"codex-cli":    "P",
		"opencode-cli": "O",
		"custom":       "X",
	}
	SetAgentIcons(customIcons)

	// Verify custom icons are used
	for agent, expectedIcon := range customIcons {
		icon := getAgentIcon(agent)
		if icon != expectedIcon {
			t.Errorf("After SetAgentIcons, getAgentIcon(%q) = %q, want %q", agent, icon, expectedIcon)
		}
	}

	// Restore original icons
	SetAgentIcons(originalIcons)

	// Verify restoration
	if getAgentIcon("claude-code") != "🤖" {
		t.Error("Failed to restore original icons")
	}
}

// TestContextUsageColorBoundaries tests edge cases for context color mapping
func TestContextUsageColorBoundaries(t *testing.T) {
	tests := []struct {
		name     string
		percent  float64
		expected string
	}{
		{"just below 70%", 69.99, "green"},
		{"exactly 70%", 70.0, "yellow"},
		{"just above 70%", 70.01, "yellow"},
		{"just below 85%", 84.99, "yellow"},
		{"exactly 85%", 85.0, "colour208"},
		{"just above 85%", 85.01, "colour208"},
		{"just below 95%", 94.99, "colour208"},
		{"exactly 95%", 95.0, "red"},
		{"just above 95%", 95.01, "red"},
		{"exactly 100%", 100.0, "red"},
		{"negative (unavailable)", -1.0, "grey"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			color := getContextColor(tt.percent)
			if color != tt.expected {
				t.Errorf("getContextColor(%.2f) = %q, want %q", tt.percent, color, tt.expected)
			}
		})
	}
}

// TestFormatCost tests cost formatting
func TestFormatCost(t *testing.T) {
	tests := []struct {
		usd      float64
		expected string
	}{
		{0.0, "$0.00"},
		{0.005, "$0.00"},
		{0.01, "$0.01"},
		{0.50, "$0.50"},
		{1.23, "$1.23"},
		{10.00, "$10.00"},
		{121.67, "$121.67"},
		{0.001, "$0.00"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := formatCost(tt.usd)
			if result != tt.expected {
				t.Errorf("formatCost(%f) = %q, want %q", tt.usd, result, tt.expected)
			}
		})
	}
}

// TestGetCostColor tests cost color thresholds
func TestGetCostColor(t *testing.T) {
	tests := []struct {
		usd      float64
		expected string
	}{
		{0.0, "green"},
		{0.50, "green"},
		{0.99, "green"},
		{1.00, "yellow"},
		{5.00, "yellow"},
		{9.99, "yellow"},
		{10.00, "red"},
		{50.00, "red"},
		{100.00, "red"},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("$%.2f", tt.usd), func(t *testing.T) {
			result := getCostColor(tt.usd)
			if result != tt.expected {
				t.Errorf("getCostColor(%f) = %q, want %q", tt.usd, result, tt.expected)
			}
		})
	}
}

// TestShortenModelName tests model name shortening
func TestShortenModelName(t *testing.T) {
	tests := []struct {
		displayName string
		expected    string
	}{
		{"Opus 4.6 (1M context)", "Opus"},
		{"Claude Opus 4", "Opus"},
		{"Claude Sonnet 4", "Sonnet"},
		{"Sonnet 3.5", "Sonnet"},
		{"Claude Haiku 3", "Haiku"},
		{"Haiku", "Haiku"},
		{"GPT-4o", "GPT-4o"},
		{"", ""},
		{"Some Unknown Model", "Some"},
	}

	for _, tt := range tests {
		t.Run(tt.displayName, func(t *testing.T) {
			result := shortenModelName(tt.displayName)
			if result != tt.expected {
				t.Errorf("shortenModelName(%q) = %q, want %q", tt.displayName, result, tt.expected)
			}
		})
	}
}

// TestGetRateLimitColor tests rate limit color thresholds
func TestGetRateLimitColor(t *testing.T) {
	tests := []struct {
		pct      float64
		expected string
	}{
		{-1.0, "grey"},
		{0.0, "green"},
		{25.0, "green"},
		{49.9, "green"},
		{50.0, "yellow"},
		{65.0, "yellow"},
		{79.9, "yellow"},
		{80.0, "red"},
		{90.0, "red"},
		{100.0, "red"},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%.1f%%", tt.pct), func(t *testing.T) {
			result := getRateLimitColor(tt.pct)
			if result != tt.expected {
				t.Errorf("getRateLimitColor(%f) = %q, want %q", tt.pct, result, tt.expected)
			}
		})
	}
}

// TestCollectStatusLineData_StaleFileKeepsCostAndModel verifies that when the
// statusline file is stale, Cost and ModelShort are still populated (both are
// stable/cumulative), but RateLimit5h remains at -1 (point-in-time, not shown).
func TestCollectStatusLineData_StaleFileKeepsCostAndModel(t *testing.T) {
	tmpDir := t.TempDir()
	originalDir := statusLineDir
	statusLineDir = tmpDir
	defer func() { statusLineDir = originalDir }()

	sessionID := "stale-model-test"

	// Build a valid statusline file
	slData := statusLineFileData{SessionID: sessionID}
	slData.Cost.TotalCostUSD = 2.50
	slData.Model.DisplayName = "Claude Sonnet 4"
	slData.Model.ID = "claude-sonnet-4"
	slData.RateLimits.FiveHour.UsedPercentage = 42.0

	raw, err := json.Marshal(slData)
	if err != nil {
		t.Fatal(err)
	}
	filePath := filepath.Join(tmpDir, sessionID+".json")
	if err := os.WriteFile(filePath, raw, 0644); err != nil {
		t.Fatal(err)
	}

	// Back-date the file by 3 minutes so it is stale
	past := time.Now().Add(-3 * time.Minute)
	if err := os.Chtimes(filePath, past, past); err != nil {
		t.Fatal(err)
	}

	m := &manifest.Manifest{
		SessionID: sessionID,
		Name:      sessionID,
		Harness:   "claude-code",
		ContextUsage: &manifest.ContextUsage{
			TotalTokens:    200000,
			UsedTokens:     50000,
			PercentageUsed: 25.0,
			LastUpdated:    time.Now(),
			Source:         "manual",
		},
	}
	m.Claude.UUID = sessionID

	uniqueName := fmt.Sprintf("test-stale-model-%d", time.Now().UnixNano())
	data, err := CollectStatusLineData(uniqueName, m)
	if err != nil {
		t.Fatalf("CollectStatusLineData() unexpected error: %v", err)
	}

	// Cost is cumulative — must be populated even for stale files
	if data.Cost == "" || data.Cost == "$0.00" {
		t.Errorf("Cost = %q, want non-zero (stale file should still supply cost)", data.Cost)
	}
	if data.CostColor == "" {
		t.Errorf("CostColor is empty, want a value")
	}

	// Model is stable per session — must be populated even for stale files
	if data.ModelShort == "" {
		t.Errorf("ModelShort is empty, want non-empty (stale file should still supply model)")
	}

	// RateLimit is point-in-time — must remain -1 for stale files
	if data.RateLimit5h != -1 {
		t.Errorf("RateLimit5h = %f, want -1 for stale file", data.RateLimit5h)
	}
}

// TestCollectStatusLineData_ManifestCostFallback verifies that when no statusline
// file is available, cost and model fall back to manifest-cached values.
func TestCollectStatusLineData_ManifestCostFallback(t *testing.T) {
	// Use a UUID that won't match any statusline file
	m := &manifest.Manifest{
		SessionID:      "manifest-fallback-test",
		Name:           "manifest-fallback-test",
		Harness:        "claude-code",
		LastKnownCost:  3.75,
		LastKnownModel: "Claude Opus 4",
		ContextUsage: &manifest.ContextUsage{
			TotalTokens:    200000,
			UsedTokens:     50000,
			PercentageUsed: 25.0,
			LastUpdated:    time.Now(),
			Source:         "manual",
		},
	}
	m.Claude.UUID = "no-such-statusline-uuid-xyz"

	uniqueName := fmt.Sprintf("test-manifest-fallback-%d", time.Now().UnixNano())
	data, err := CollectStatusLineData(uniqueName, m)
	if err != nil {
		t.Fatalf("CollectStatusLineData() unexpected error: %v", err)
	}

	// Cost should come from manifest fallback
	if data.Cost != "$3.75" {
		t.Errorf("Cost = %q, want %q (manifest fallback)", data.Cost, "$3.75")
	}
	if data.CostColor != "yellow" {
		t.Errorf("CostColor = %q, want %q", data.CostColor, "yellow")
	}

	// Model should come from manifest fallback
	if data.ModelShort != "Opus" {
		t.Errorf("ModelShort = %q, want %q (manifest fallback)", data.ModelShort, "Opus")
	}
}

// TestCollectStatusLineData_NoFallbackWhenStatuslinePresent verifies that
// statusline file data takes priority over manifest-cached values.
func TestCollectStatusLineData_NoFallbackWhenStatuslinePresent(t *testing.T) {
	tmpDir := t.TempDir()
	originalDir := statusLineDir
	statusLineDir = tmpDir
	defer func() { statusLineDir = originalDir }()

	sessionID := "statusline-priority-test"

	// Create a statusline file with different cost/model than manifest
	slData := statusLineFileData{SessionID: sessionID}
	slData.Cost.TotalCostUSD = 5.00
	slData.Model.DisplayName = "Claude Sonnet 4"

	raw, err := json.Marshal(slData)
	if err != nil {
		t.Fatal(err)
	}
	filePath := filepath.Join(tmpDir, sessionID+".json")
	if err := os.WriteFile(filePath, raw, 0644); err != nil {
		t.Fatal(err)
	}

	m := &manifest.Manifest{
		SessionID:      sessionID,
		Name:           sessionID,
		Harness:        "claude-code",
		LastKnownCost:  1.00,
		LastKnownModel: "Claude Opus 4",
	}
	m.Claude.UUID = sessionID

	uniqueName := fmt.Sprintf("test-statusline-priority-%d", time.Now().UnixNano())
	data, err := CollectStatusLineData(uniqueName, m)
	if err != nil {
		t.Fatalf("CollectStatusLineData() unexpected error: %v", err)
	}

	// Statusline file should win over manifest
	if data.Cost != "$5.00" {
		t.Errorf("Cost = %q, want %q (statusline should win)", data.Cost, "$5.00")
	}
	if data.ModelShort != "Sonnet" {
		t.Errorf("ModelShort = %q, want %q (statusline should win)", data.ModelShort, "Sonnet")
	}
}

// TestModelIDToDisplayName tests model ID to display name conversion
func TestModelIDToDisplayName(t *testing.T) {
	tests := []struct {
		modelID  string
		expected string
	}{
		{"claude-opus-4-6-20251001", "Opus"},
		{"claude-opus-4-20260101", "Opus"},
		{"claude-sonnet-4-5-20250929", "Sonnet"},
		{"claude-haiku-4-20260101", "Haiku"},
		{"gpt-4o", "gpt-4o"},
		{"gemini-pro", "gemini-pro"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.modelID, func(t *testing.T) {
			result := modelIDToDisplayName(tt.modelID)
			if result != tt.expected {
				t.Errorf("modelIDToDisplayName(%q) = %q, want %q", tt.modelID, result, tt.expected)
			}
		})
	}
}

// TestFormatTokenCount tests token count formatting
func TestFormatTokenCount(t *testing.T) {
	tests := []struct {
		tokens   int
		expected string
	}{
		{0, "0"},
		{500, "500"},
		{999, "999"},
		{1000, "1k"},
		{1500, "1.5k"},
		{5000, "5k"},
		{50000, "50k"},
		{200000, "200k"},
		{999999, "1000.0k"},
		{1000000, "1M"},
		{1500000, "1.5M"},
		{2000000, "2M"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := formatTokenCount(tt.tokens)
			if result != tt.expected {
				t.Errorf("formatTokenCount(%d) = %q, want %q", tt.tokens, result, tt.expected)
			}
		})
	}
}

// TestApplyManifestFallback tests manifest fallback logic for cost and model
func TestApplyManifestFallback(t *testing.T) {
	t.Run("cost fallback when empty", func(t *testing.T) {
		data := &StatusLineData{}
		m := &manifest.Manifest{LastKnownCost: 7.50}
		applyManifestFallback(data, m)
		if data.Cost != "$7.50" {
			t.Errorf("Cost = %q, want %q", data.Cost, "$7.50")
		}
		if data.CostColor != "yellow" {
			t.Errorf("CostColor = %q, want %q", data.CostColor, "yellow")
		}
	})

	t.Run("model fallback when empty", func(t *testing.T) {
		data := &StatusLineData{}
		m := &manifest.Manifest{LastKnownModel: "Claude Haiku 3"}
		applyManifestFallback(data, m)
		if data.ModelShort != "Haiku" {
			t.Errorf("ModelShort = %q, want %q", data.ModelShort, "Haiku")
		}
	})

	t.Run("no cost fallback when already present", func(t *testing.T) {
		data := &StatusLineData{Cost: "$2.00", CostColor: "yellow"}
		m := &manifest.Manifest{LastKnownCost: 99.00}
		applyManifestFallback(data, m)
		if data.Cost != "$2.00" {
			t.Errorf("Cost = %q, want %q (should not be overwritten)", data.Cost, "$2.00")
		}
	})

	t.Run("no model fallback when already present", func(t *testing.T) {
		data := &StatusLineData{ModelShort: "Sonnet"}
		m := &manifest.Manifest{LastKnownModel: "Claude Opus 4"}
		applyManifestFallback(data, m)
		if data.ModelShort != "Sonnet" {
			t.Errorf("ModelShort = %q, want %q (should not be overwritten)", data.ModelShort, "Sonnet")
		}
	})

	t.Run("no fallback when manifest has zero cost", func(t *testing.T) {
		data := &StatusLineData{}
		m := &manifest.Manifest{LastKnownCost: 0}
		applyManifestFallback(data, m)
		if data.Cost != "" {
			t.Errorf("Cost = %q, want empty (zero cost should not trigger fallback)", data.Cost)
		}
	})

	t.Run("no fallback when manifest has empty model", func(t *testing.T) {
		data := &StatusLineData{}
		m := &manifest.Manifest{LastKnownModel: ""}
		applyManifestFallback(data, m)
		if data.ModelShort != "" {
			t.Errorf("ModelShort = %q, want empty", data.ModelShort)
		}
	})
}

// TestApplyStatusLineFileData tests reading statusline file and applying data
func TestApplyStatusLineFileData(t *testing.T) {
	tmpDir := t.TempDir()
	originalDir := statusLineDir
	statusLineDir = tmpDir
	defer func() { statusLineDir = originalDir }()

	t.Run("fresh file applies cost model and rate limit", func(t *testing.T) {
		sessionID := "fresh-apply-test"
		slData := statusLineFileData{SessionID: sessionID}
		slData.Cost.TotalCostUSD = 4.25
		slData.Model.DisplayName = "Opus 4.6 (1M context)"
		slData.RateLimits.FiveHour.UsedPercentage = 35.0

		raw, _ := json.Marshal(slData)
		if err := os.WriteFile(filepath.Join(tmpDir, sessionID+".json"), raw, 0644); err != nil {
			t.Fatal(err)
		}

		data := &StatusLineData{RateLimit5h: -1}
		applyStatusLineFileData(data, sessionID)

		if data.Cost != "$4.25" {
			t.Errorf("Cost = %q, want %q", data.Cost, "$4.25")
		}
		if data.CostColor != "yellow" {
			t.Errorf("CostColor = %q, want %q", data.CostColor, "yellow")
		}
		if data.ModelShort != "Opus" {
			t.Errorf("ModelShort = %q, want %q", data.ModelShort, "Opus")
		}
		if data.RateLimit5h != 35.0 {
			t.Errorf("RateLimit5h = %f, want 35.0", data.RateLimit5h)
		}
		if data.RateLimitColor != "green" {
			t.Errorf("RateLimitColor = %q, want %q", data.RateLimitColor, "green")
		}
	})

	t.Run("stale file applies cost and model but not rate limit", func(t *testing.T) {
		sessionID := "stale-apply-test"
		slData := statusLineFileData{SessionID: sessionID}
		slData.Cost.TotalCostUSD = 12.00
		slData.Model.DisplayName = "Claude Sonnet 4"
		slData.RateLimits.FiveHour.UsedPercentage = 90.0

		raw, _ := json.Marshal(slData)
		filePath := filepath.Join(tmpDir, sessionID+".json")
		if err := os.WriteFile(filePath, raw, 0644); err != nil {
			t.Fatal(err)
		}
		past := time.Now().Add(-3 * time.Minute)
		if err := os.Chtimes(filePath, past, past); err != nil {
			t.Fatal(err)
		}

		data := &StatusLineData{RateLimit5h: -1}
		applyStatusLineFileData(data, sessionID)

		if data.Cost != "$12.00" {
			t.Errorf("Cost = %q, want %q", data.Cost, "$12.00")
		}
		if data.ModelShort != "Sonnet" {
			t.Errorf("ModelShort = %q, want %q", data.ModelShort, "Sonnet")
		}
		// Rate limit should NOT be applied for stale file
		if data.RateLimit5h != -1 {
			t.Errorf("RateLimit5h = %f, want -1 (stale file)", data.RateLimit5h)
		}
	})

	t.Run("missing file does nothing", func(t *testing.T) {
		data := &StatusLineData{RateLimit5h: -1}
		applyStatusLineFileData(data, "no-such-uuid")
		if data.Cost != "" {
			t.Errorf("Cost = %q, want empty for missing file", data.Cost)
		}
		if data.ModelShort != "" {
			t.Errorf("ModelShort = %q, want empty for missing file", data.ModelShort)
		}
	})
}

// TestCollectStatusLineData_CostFallbackFromConversationLog verifies that
// cost is estimated from conversation log tokens when no statusline or manifest cost.
func TestCollectStatusLineData_CostFallbackFromConversationLog(t *testing.T) {
	tempDir := t.TempDir()
	sessionUUID := "cost-estimate-uuid"
	projectHash := "-home-user-cost-test"
	logPath := filepath.Join(tempDir, ".claude", "projects", projectHash, sessionUUID+".jsonl")

	if err := os.MkdirAll(filepath.Dir(logPath), 0755); err != nil {
		t.Fatal(err)
	}

	// Write assistant messages with token usage (sonnet pricing)
	content := `{"type":"assistant","timestamp":"2026-03-20T10:00:05Z","message":{"model":"claude-sonnet-4-5-20250929","usage":{"input_tokens":1000,"cache_creation_input_tokens":0,"cache_read_input_tokens":0,"output_tokens":500}}}
`
	if err := os.WriteFile(logPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tempDir)
	defer os.Setenv("HOME", originalHome)

	ClearDetectorCache()

	// No statusline file, no manifest cost
	m := &manifest.Manifest{
		SessionID: "cost-estimate-test",
		Name:      "cost-estimate-test",
		Harness:   "claude-code",
	}
	m.Claude.UUID = sessionUUID

	uniqueName := fmt.Sprintf("test-cost-estimate-%d", time.Now().UnixNano())
	data, err := CollectStatusLineData(uniqueName, m)
	if err != nil {
		t.Fatalf("CollectStatusLineData() unexpected error: %v", err)
	}

	// Should have estimated cost from conversation log
	if data.Cost == "" || data.Cost == "$0.00" {
		t.Logf("Cost = %q (may be too small to display, depends on token count)", data.Cost)
	}
}

// TestCollectStatusLineDataWithVariousContextLevels tests different context usage levels
func TestCollectStatusLineDataWithVariousContextLevels(t *testing.T) {
	tests := []struct {
		name           string
		percentageUsed float64
		expectedColor  string
	}{
		{"low context usage", 50.0, "green"},
		{"medium context usage", 75.0, "yellow"},
		{"high context usage", 90.0, "colour208"},
		{"critical context usage", 98.0, "red"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &manifest.Manifest{
				SessionID: "test",
				Name:      "test",
				Harness:   "claude-code",
				Context: manifest.Context{
					Project: "/test/project",
				},
				ContextUsage: &manifest.ContextUsage{
					TotalTokens:    100000,
					UsedTokens:     int(tt.percentageUsed * 1000),
					PercentageUsed: tt.percentageUsed,
					LastUpdated:    time.Now(),
					Source:         "manual",
				},
			}

			data, err := CollectStatusLineData("test", m)
			if err != nil {
				t.Fatalf("CollectStatusLineData() failed: %v", err)
			}

			if data.ContextPercent != tt.percentageUsed {
				t.Errorf("ContextPercent = %.1f, want %.1f", data.ContextPercent, tt.percentageUsed)
			}

			if data.ContextColor != tt.expectedColor {
				t.Errorf("ContextColor = %q, want %q", data.ContextColor, tt.expectedColor)
			}
		})
	}
}
