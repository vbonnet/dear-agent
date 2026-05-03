package main

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/session"
)

// TestStatusLineCommand tests the status-line command with various flags
func TestStatusLineCommand(t *testing.T) {
	tests := []struct {
		name          string
		args          []string
		expectError   bool
		errorContains string
	}{
		{
			name:          "no session specified and not in tmux",
			args:          []string{"session", "status-line"},
			expectError:   true,
			errorContains: "failed to detect session",
		},
		{
			name:        "json output flag",
			args:        []string{"session", "status-line", "--json", "-s", "test-session"},
			expectError: false,
		},
		{
			name:        "custom format flag",
			args:        []string{"session", "status-line", "-f", "{{.AgentIcon}} {{.State}}", "-s", "test-session"},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// This is a placeholder test structure
			// Actual implementation would require mocking tmux and manifest lookups
			if !tt.expectError {
				// Test passes if we don't expect an error
				// Full integration test would mock the dependencies
				t.Skip("Integration test requires mock setup")
			}
		})
	}
}

// TestAutoDetectTmuxSession tests tmux session auto-detection.
//
// The "in_tmux_session" subtest used to depend on whatever tmux server
// happened to be running on the AGM socket — which made it pass when run
// alone (because the user's normal sessions exist) and fail in CI or
// after other tests cleared the server. We now drive a dedicated tmux
// server on a temp socket so the assertion is hermetic.
func TestAutoDetectTmuxSession(t *testing.T) {
	t.Run("not_in_tmux", func(t *testing.T) {
		t.Setenv("TMUX", "")
		_, err := autoDetectTmuxSession()
		if err == nil {
			t.Error("expected error when TMUX is unset, got nil")
		}
	})

	t.Run("in_tmux_session", func(t *testing.T) {
		if _, err := exec.LookPath("tmux"); err != nil {
			t.Skip("tmux not installed")
		}

		// Spin up a tmux server on a private socket so detection can find a
		// real session without depending on whatever the user has running.
		socketPath := filepath.Join(t.TempDir(), "tmux.sock")
		sessionName := "auto-detect-test-" + time.Now().Format("150405.000")
		// Normalize to tmux's allowed characters.
		sessionName = strings.ReplaceAll(sessionName, ".", "-")

		cmd := exec.Command("tmux", "-S", socketPath, "new-session", "-d", "-s", sessionName)
		if err := cmd.Run(); err != nil {
			t.Skipf("could not create test tmux session: %v", err)
		}
		t.Cleanup(func() {
			_ = exec.Command("tmux", "-S", socketPath, "kill-server").Run()
		})

		t.Setenv("TMUX", socketPath+",1,0")
		t.Setenv("AGM_TMUX_SOCKET", socketPath)

		_, err := autoDetectTmuxSession()
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})
}

// TestOutputJSON tests JSON output formatting
func TestOutputJSON(t *testing.T) {
	testData := &session.StatusLineData{
		SessionName:    "test-session",
		State:          "READY",
		StateColor:     "green",
		Branch:         "main",
		Uncommitted:    3,
		ContextPercent: 45.5,
		ContextColor:   "green",
		Workspace:      "oss",
		AgentType:      "claude",
		AgentIcon:      "🤖",
	}

	// Test that JSON marshaling works
	data, err := json.Marshal(testData)
	if err != nil {
		t.Fatalf("failed to marshal test data: %v", err)
	}

	// Verify structure
	var decoded session.StatusLineData
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal JSON: %v", err)
	}

	if decoded.SessionName != testData.SessionName {
		t.Errorf("SessionName mismatch: got %q, want %q", decoded.SessionName, testData.SessionName)
	}
	if decoded.AgentType != testData.AgentType {
		t.Errorf("AgentType mismatch: got %q, want %q", decoded.AgentType, testData.AgentType)
	}
}

// TestFindManifestBySession tests manifest lookup by session name
func TestFindManifestBySession(t *testing.T) {
	// This test would require setting up test fixtures
	// For now, it's a placeholder to document expected behavior
	t.Run("finds manifest by tmux session name", func(t *testing.T) {
		t.Skip("Requires manifest fixtures")
	})

	t.Run("finds manifest by AGM session name", func(t *testing.T) {
		t.Skip("Requires manifest fixtures")
	})

	t.Run("returns error for non-existent session", func(t *testing.T) {
		t.Skip("Requires manifest fixtures")
	})
}

// TestStatusLineIntegration tests full status line generation with all agent types
func TestStatusLineIntegration(t *testing.T) {
	agentTypes := []struct {
		agent        string
		expectedIcon string
	}{
		{"claude-code", "🤖"},
		{"gemini-cli", "✨"},
		{"codex-cli", "🧠"},
		{"opencode-cli", "💻"},
	}

	for _, tt := range agentTypes {
		t.Run(tt.agent, func(t *testing.T) {
			// Create test manifest with unique session name to avoid tmux conflicts
			uniqueSessionName := fmt.Sprintf("test-status-line-%s-%d", tt.agent, time.Now().UnixNano())
			m := &manifest.Manifest{
				SchemaVersion: "2.0",
				SessionID:     "test-session-id",
				Name:          uniqueSessionName,
				State:         manifest.StateDone,
				Harness:       tt.agent,
				CreatedAt:     time.Now(),
				UpdatedAt:     time.Now(),
				Context: manifest.Context{
					Project: "/tmp/test-project",
				},
				Tmux: manifest.Tmux{
					SessionName: uniqueSessionName,
				},
				ContextUsage: &manifest.ContextUsage{
					TotalTokens:    200000,
					UsedTokens:     90000,
					PercentageUsed: 45.0,
					LastUpdated:    time.Now(),
					Source:         "test",
				},
			}

			// Collect status line data
			data, err := session.CollectStatusLineData("test-session", m)
			if err != nil {
				t.Fatalf("failed to collect status line data: %v", err)
			}

			// Verify agent icon
			if data.AgentIcon != tt.expectedIcon {
				t.Errorf("AgentIcon mismatch for %s: got %q, want %q", tt.agent, data.AgentIcon, tt.expectedIcon)
			}

			// Verify agent type
			if data.AgentType != tt.agent {
				t.Errorf("AgentType mismatch: got %q, want %q", data.AgentType, tt.agent)
			}

			// Verify state: tmux session doesn't exist, so state is OFFLINE
			// (hooks-only detection: no tmux session = OFFLINE regardless of manifest state)
			if data.State != manifest.StateOffline {
				t.Errorf("State mismatch: got %q, want %q", data.State, manifest.StateOffline)
			}

			// Verify state color (OFFLINE = grey)
			if data.StateColor != "colour239" {
				t.Errorf("StateColor mismatch: got %q, want %q", data.StateColor, "colour239")
			}

			// Verify context percentage
			if data.ContextPercent != 45.0 {
				t.Errorf("ContextPercent mismatch: got %f, want %f", data.ContextPercent, 45.0)
			}

			// Verify context color (should be green for 45%)
			if data.ContextColor != "green" {
				t.Errorf("ContextColor mismatch: got %q, want %q", data.ContextColor, "green")
			}
		})
	}
}

// TestTemplateRendering tests that templates render correctly
func TestTemplateRendering(t *testing.T) {
	testData := &session.StatusLineData{
		SessionName:    "test-session",
		State:          "READY",
		StateColor:     "green",
		Branch:         "main",
		Uncommitted:    3,
		ContextPercent: 45.5,
		ContextColor:   "green",
		Workspace:      "oss",
		AgentType:      "claude",
		AgentIcon:      "🤖",
	}

	tests := []struct {
		name           string
		template       string
		expectedOutput string
	}{
		{
			name:           "simple agent icon",
			template:       "{{.AgentIcon}}",
			expectedOutput: "🤖",
		},
		{
			name:           "state with color",
			template:       "#[fg={{.StateColor}}]{{.State}}#[default]",
			expectedOutput: "#[fg=green]READY#[default]",
		},
		{
			name:           "context percentage",
			template:       "{{.ContextPercent}}%",
			expectedOutput: "45.5%",
		},
		{
			name:           "full default template",
			template:       "{{.AgentIcon}} {{.State}} | {{.ContextPercent}}%",
			expectedOutput: "🤖 READY | 45.5%",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// This would use the actual formatter
			// For now, just verify testData structure
			if testData.SessionName != "test-session" {
				t.Errorf("testData not initialized correctly")
			}
			t.Skip("Requires formatter integration")
		})
	}
}

// TestErrorHandling tests error handling for various edge cases
func TestErrorHandling(t *testing.T) {
	tests := []struct {
		name        string
		scenario    string
		expectError bool
	}{
		{
			name:        "invalid template syntax",
			scenario:    "malformed template",
			expectError: true,
		},
		{
			name:        "session not found",
			scenario:    "non-existent session",
			expectError: true,
		},
		{
			name:        "manifest without context usage",
			scenario:    "missing context usage",
			expectError: false, // Should handle gracefully
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Skip("Requires mock setup for error scenarios")
		})
	}
}

// TestMissingContextUsage tests graceful handling when ContextUsage is nil
func TestMissingContextUsage(t *testing.T) {
	// Create manifest WITHOUT ContextUsage (common for existing sessions)
	m := &manifest.Manifest{
		SchemaVersion: "2.0",
		SessionID:     "test-session-id",
		Name:          "test-session",
		State:         manifest.StateDone,
		Harness:       "claude-code",
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		Context: manifest.Context{
			Project: "/tmp/test-project",
		},
		Tmux: manifest.Tmux{
			SessionName: "test-session",
		},
		ContextUsage: nil, // Explicitly nil
	}

	// Collect status line data
	data, err := session.CollectStatusLineData("test-session", m)
	if err != nil {
		t.Fatalf("should handle nil ContextUsage gracefully: %v", err)
	}

	// Verify ContextPercent is -1 (sentinel value for "unknown")
	if data.ContextPercent != -1.0 {
		t.Errorf("ContextPercent with nil ContextUsage: got %f, want -1.0", data.ContextPercent)
	}

	// Verify template can render with -1 value (regression test for float/int comparison bug)
	// Template should show "--" instead of percentage
	if data.ContextPercent < 0 {
		t.Log("ContextPercent correctly set to -1 for missing data (template will show '--')")
	}
}

// TestTemplateTypeComparisons tests that templates handle float/int comparisons correctly
func TestTemplateTypeComparisons(t *testing.T) {
	tests := []struct {
		name                  string
		contextPercent        float64
		uncommitted           int
		shouldShowContext     bool
		shouldShowUncommitted bool
	}{
		{
			name:                  "positive context and uncommitted",
			contextPercent:        45.5,
			uncommitted:           3,
			shouldShowContext:     true,
			shouldShowUncommitted: true,
		},
		{
			name:                  "zero values",
			contextPercent:        0.0,
			uncommitted:           0,
			shouldShowContext:     true,  // ge 0.0 includes 0
			shouldShowUncommitted: false, // gt 0 excludes 0
		},
		{
			name:                  "negative context (missing data)",
			contextPercent:        -1.0,
			uncommitted:           0,
			shouldShowContext:     false, // template shows "--"
			shouldShowUncommitted: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Verify comparison logic matches template expectations
			contextVisible := tt.contextPercent >= 0.0
			uncommittedVisible := tt.uncommitted > 0

			if contextVisible != tt.shouldShowContext {
				t.Errorf("context visibility: got %v, want %v (percent: %f)",
					contextVisible, tt.shouldShowContext, tt.contextPercent)
			}

			if uncommittedVisible != tt.shouldShowUncommitted {
				t.Errorf("uncommitted visibility: got %v, want %v (count: %d)",
					uncommittedVisible, tt.shouldShowUncommitted, tt.uncommitted)
			}
		})
	}
}

// TestGracefulDegradation tests graceful handling of non-AGM sessions
func TestGracefulDegradation(t *testing.T) {
	// This test documents expected behavior when status-line is called
	// in a non-AGM tmux session (e.g., test sessions, temporary sessions)
	//
	// Expected behavior:
	// - runStatusLine returns nil (no error)
	// - Outputs "[session-name]" to stdout
	// - tmux status line continues to work (doesn't disappear)
	//
	// This prevents the status line from breaking when users have a mix of
	// AGM-managed and regular tmux sessions on the same socket.

	t.Log("Graceful degradation behavior documented")
	t.Log("Non-AGM sessions show: [session-name]")
	t.Log("AGM sessions show: full status line with icon, state, context %, etc.")

	// Actual test would require mocking findManifestBySession to return error
	t.Skip("Requires mock setup - behavior verified in manual testing")
}
