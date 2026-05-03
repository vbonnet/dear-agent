package session

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/vbonnet/dear-agent/agm/internal/manifest"
)

func TestStatusLineAutoDetection(t *testing.T) {
	// Setup temp directory for conversation log
	tempDir := t.TempDir()
	sessionUUID := "auto-detect-test-uuid"
	logPath := filepath.Join(tempDir, ".claude", "projects", sessionUUID, "conversation.jsonl")

	if err := os.MkdirAll(filepath.Dir(logPath), 0755); err != nil {
		t.Fatal(err)
	}

	// Write conversation log with token usage
	content := `{"type":"user_message","content":"Test","timestamp":"2026-03-15T10:00:00Z"}
{"type":"system_reminder","content":"<system-reminder>Token usage: 60000/200000; 140000 remaining</system-reminder>","timestamp":"2026-03-15T10:00:05Z"}
`
	if err := os.WriteFile(logPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	// Mock home directory
	t.Setenv("HOME", tempDir)

	// Clear cache
	ClearDetectorCache()

	// Test Case 1: Manifest WITHOUT ContextUsage (should auto-detect)
	t.Run("auto_detect_from_conversation_log", func(t *testing.T) {
		m := &manifest.Manifest{
			Claude: manifest.Claude{
				UUID: sessionUUID,
			},
			Tmux: manifest.Tmux{
				SessionName: "test-session",
			},
			Harness:   "claude-code",
			Workspace: "oss",
			// NO ContextUsage field - should trigger auto-detection
		}

		data, err := CollectStatusLineData("test-session", m)
		if err != nil {
			t.Fatalf("CollectStatusLineData() error = %v", err)
		}

		// Should have detected 60000/200000 = 30%
		if data.ContextPercent != 30.0 {
			t.Errorf("ContextPercent = %.1f, want 30.0 (auto-detected from log)", data.ContextPercent)
		}

		if data.ContextColor != "green" {
			t.Errorf("ContextColor = %s, want green (30%% is low usage)", data.ContextColor)
		}
	})

	// Test Case 2: Manifest WITH ContextUsage (should use manifest, not log)
	t.Run("prefer_manifest_over_auto_detect", func(t *testing.T) {
		ClearDetectorCache()

		m := &manifest.Manifest{
			Claude: manifest.Claude{
				UUID: sessionUUID,
			},
			Tmux: manifest.Tmux{
				SessionName: "test-session-2",
			},
			Harness:   "claude-code",
			Workspace: "oss",
			ContextUsage: &manifest.ContextUsage{
				TotalTokens:    200000,
				UsedTokens:     150000,
				PercentageUsed: 75.0, // Different from log (30%)
				Source:         "hook",
			},
		}

		data, err := CollectStatusLineData("test-session-2", m)
		if err != nil {
			t.Fatalf("CollectStatusLineData() error = %v", err)
		}

		// Should use manifest value (75%), NOT log value (30%)
		if data.ContextPercent != 75.0 {
			t.Errorf("ContextPercent = %.1f, want 75.0 (from manifest, not log)", data.ContextPercent)
		}

		if data.ContextColor != "yellow" {
			t.Errorf("ContextColor = %s, want yellow (75%% is warning level)", data.ContextColor)
		}
	})

	// Test Case 3: No ContextUsage AND no conversation log (should show unavailable)
	t.Run("unavailable_when_no_sources", func(t *testing.T) {
		ClearDetectorCache()

		m := &manifest.Manifest{
			Claude: manifest.Claude{
				UUID: "nonexistent-session",
			},
			Tmux: manifest.Tmux{
				SessionName: "test-session-3",
			},
			Harness:   "claude-code",
			Workspace: "oss",
			// No ContextUsage and UUID doesn't match any log file
		}

		data, err := CollectStatusLineData("test-session-3", m)
		if err != nil {
			t.Fatalf("CollectStatusLineData() error = %v", err)
		}

		// Should show unavailable (-1)
		if data.ContextPercent != -1 {
			t.Errorf("ContextPercent = %.1f, want -1 (unavailable)", data.ContextPercent)
		}

		if data.ContextColor != "grey" {
			t.Errorf("ContextColor = %s, want grey (unavailable)", data.ContextColor)
		}
	})
}

func TestStatusLineAutoDetectionFallbackChain(t *testing.T) {
	// This test verifies the fallback chain:
	// manifest.ContextUsage → conversation log → unavailable (-1)

	tempDir := t.TempDir()
	sessionUUID := "fallback-test-uuid"
	logPath := filepath.Join(tempDir, ".claude", "projects", sessionUUID, "conversation.jsonl")

	if err := os.MkdirAll(filepath.Dir(logPath), 0755); err != nil {
		t.Fatal(err)
	}

	content := `{"type":"system_reminder","content":"Token usage: 40000/200000","timestamp":"2026-03-15T10:00:00Z"}
`
	if err := os.WriteFile(logPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("HOME", tempDir)

	ClearDetectorCache()

	tests := []struct {
		name            string
		manifestContext *manifest.ContextUsage
		sessionUUID     string
		wantPercent     float64
		wantSource      string // For documentation purposes
	}{
		{
			name: "step1_manifest_available",
			manifestContext: &manifest.ContextUsage{
				PercentageUsed: 80.0,
				Source:         "hook",
			},
			sessionUUID: sessionUUID,
			wantPercent: 80.0,
			wantSource:  "manifest (hook)",
		},
		{
			name:            "step2_fallback_to_log",
			manifestContext: nil, // No manifest context
			sessionUUID:     sessionUUID,
			wantPercent:     20.0, // 40000/200000 = 20%
			wantSource:      "conversation_log",
		},
		{
			name:            "step3_unavailable",
			manifestContext: nil,
			sessionUUID:     "nonexistent",
			wantPercent:     -1,
			wantSource:      "unavailable",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ClearDetectorCache()

			m := &manifest.Manifest{
				Claude: manifest.Claude{
					UUID: tt.sessionUUID,
				},
				Tmux: manifest.Tmux{
					SessionName: "fallback-test",
				},
				Harness:      "claude-code",
				Workspace:    "oss",
				ContextUsage: tt.manifestContext,
			}

			data, err := CollectStatusLineData("fallback-test", m)
			if err != nil {
				t.Fatalf("CollectStatusLineData() error = %v", err)
			}

			if data.ContextPercent != tt.wantPercent {
				t.Errorf("ContextPercent = %.1f, want %.1f (source: %s)",
					data.ContextPercent, tt.wantPercent, tt.wantSource)
			}
		})
	}
}
