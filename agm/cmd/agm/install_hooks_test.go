package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestAddHookRegistration(t *testing.T) {
	tests := []struct {
		name      string
		initial   map[string]interface{}
		reg       hookRegistration
		wantAdded bool
	}{
		{
			name:    "add to empty hooks map",
			initial: map[string]interface{}{},
			reg: hookRegistration{
				Event:   "PostToolUse",
				Command: "~/.claude/hooks/posttool-agm-state-notify",
				Timeout: 5,
			},
			wantAdded: true,
		},
		{
			name: "skip duplicate command",
			initial: map[string]interface{}{
				"PostToolUse": []interface{}{
					map[string]interface{}{
						"hooks": []interface{}{
							map[string]interface{}{
								"command": "~/.claude/hooks/posttool-agm-state-notify",
								"type":    "command",
							},
						},
					},
				},
			},
			reg: hookRegistration{
				Event:   "PostToolUse",
				Command: "~/.claude/hooks/posttool-agm-state-notify",
				Timeout: 5,
			},
			wantAdded: false,
		},
		{
			name: "add to existing event with other hooks",
			initial: map[string]interface{}{
				"PostToolUse": []interface{}{
					map[string]interface{}{
						"hooks": []interface{}{
							map[string]interface{}{
								"command": "some-other-hook",
								"type":    "command",
							},
						},
					},
				},
			},
			reg: hookRegistration{
				Event:   "PostToolUse",
				Command: "~/.claude/hooks/posttool-agm-state-notify",
				Timeout: 5,
			},
			wantAdded: true,
		},
		{
			name:    "add with matcher",
			initial: map[string]interface{}{},
			reg: hookRegistration{
				Event:   "PreToolUse",
				Command: "~/.claude/hooks/agm-pretool-test-session-guard",
				Timeout: 5,
				Matcher: "Bash",
			},
			wantAdded: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := addHookRegistration(tt.initial, tt.reg)
			if got != tt.wantAdded {
				t.Errorf("addHookRegistration() = %v, want %v", got, tt.wantAdded)
			}

			if tt.wantAdded {
				// Verify the hook was added to the correct event
				eventGroups, ok := tt.initial[tt.reg.Event].([]interface{})
				if !ok || len(eventGroups) == 0 {
					t.Fatal("hook event array not found after adding")
				}

				// Check last group has our command
				lastGroup := eventGroups[len(eventGroups)-1].(map[string]interface{})
				hooks := lastGroup["hooks"].([]interface{})
				lastHook := hooks[0].(map[string]interface{})
				if lastHook["command"] != tt.reg.Command {
					t.Errorf("command = %v, want %v", lastHook["command"], tt.reg.Command)
				}
				if tt.reg.Matcher != "" {
					if lastGroup["matcher"] != tt.reg.Matcher {
						t.Errorf("matcher = %v, want %v", lastGroup["matcher"], tt.reg.Matcher)
					}
				}
			}
		})
	}
}

func TestRegisterHooksInSettings(t *testing.T) {
	// Create a temp directory to act as home
	tmpHome := t.TempDir()
	claudeDir := filepath.Join(tmpHome, ".claude")
	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		t.Fatal(err)
	}

	t.Run("creates settings.json if not exists", func(t *testing.T) {
		home := t.TempDir()
		if err := os.MkdirAll(filepath.Join(home, ".claude"), 0755); err != nil {
			t.Fatal(err)
		}

		regs := []hookRegistration{
			{
				Event:   "PostToolUse",
				Command: "~/.claude/hooks/posttool-agm-state-notify",
				Timeout: 5,
			},
		}

		count, err := registerHooksInSettings(home, regs)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if count != 1 {
			t.Errorf("registered count = %d, want 1", count)
		}

		// Verify settings.json was created
		data, err := os.ReadFile(filepath.Join(home, ".claude", "settings.json"))
		if err != nil {
			t.Fatal(err)
		}

		var settings map[string]interface{}
		if err := json.Unmarshal(data, &settings); err != nil {
			t.Fatal(err)
		}

		hooksMap, ok := settings["hooks"].(map[string]interface{})
		if !ok {
			t.Fatal("hooks key not found in settings")
		}
		postTool, ok := hooksMap["PostToolUse"].([]interface{})
		if !ok || len(postTool) != 1 {
			t.Fatal("PostToolUse not found or wrong length")
		}
	})

	t.Run("preserves existing settings", func(t *testing.T) {
		home := t.TempDir()
		if err := os.MkdirAll(filepath.Join(home, ".claude"), 0755); err != nil {
			t.Fatal(err)
		}

		// Write initial settings with existing data
		initial := map[string]interface{}{
			"model": "claude-opus-4-6",
			"hooks": map[string]interface{}{
				"PostToolUse": []interface{}{
					map[string]interface{}{
						"hooks": []interface{}{
							map[string]interface{}{
								"command": "existing-hook",
								"type":    "command",
							},
						},
					},
				},
			},
		}
		data, _ := json.MarshalIndent(initial, "", "  ")
		if err := os.WriteFile(filepath.Join(home, ".claude", "settings.json"), data, 0600); err != nil {
			t.Fatal(err)
		}

		regs := []hookRegistration{
			{
				Event:   "PostToolUse",
				Command: "~/.claude/hooks/posttool-agm-state-notify",
				Timeout: 5,
			},
		}

		count, err := registerHooksInSettings(home, regs)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if count != 1 {
			t.Errorf("registered count = %d, want 1", count)
		}

		// Verify model field preserved
		data, _ = os.ReadFile(filepath.Join(home, ".claude", "settings.json"))
		var settings map[string]interface{}
		json.Unmarshal(data, &settings)
		if settings["model"] != "claude-opus-4-6" {
			t.Error("existing model field was lost")
		}

		// Verify both hooks present
		hooksMap := settings["hooks"].(map[string]interface{})
		postTool := hooksMap["PostToolUse"].([]interface{})
		if len(postTool) != 2 {
			t.Errorf("PostToolUse length = %d, want 2", len(postTool))
		}
	})

	t.Run("idempotent - no duplicates on second run", func(t *testing.T) {
		home := t.TempDir()
		if err := os.MkdirAll(filepath.Join(home, ".claude"), 0755); err != nil {
			t.Fatal(err)
		}

		regs := []hookRegistration{
			{
				Event:   "PostToolUse",
				Command: "~/.claude/hooks/posttool-agm-state-notify",
				Timeout: 5,
			},
			{
				Event:   "PreToolUse",
				Command: "~/.claude/hooks/pretool-agm-mode-tracker",
				Timeout: 5,
			},
		}

		// First run
		count1, err := registerHooksInSettings(home, regs)
		if err != nil {
			t.Fatalf("first run error: %v", err)
		}
		if count1 != 2 {
			t.Errorf("first run count = %d, want 2", count1)
		}

		// Second run - should add nothing
		count2, err := registerHooksInSettings(home, regs)
		if err != nil {
			t.Fatalf("second run error: %v", err)
		}
		if count2 != 0 {
			t.Errorf("second run count = %d, want 0", count2)
		}
	})

	t.Run("registers all AGM hooks", func(t *testing.T) {
		home := t.TempDir()
		if err := os.MkdirAll(filepath.Join(home, ".claude"), 0755); err != nil {
			t.Fatal(err)
		}

		regs := []hookRegistration{
			{Event: "PostToolUse", Command: "~/.claude/hooks/posttool-agm-state-notify", Timeout: 5},
			{Event: "PreToolUse", Command: "~/.claude/hooks/pretool-agm-mode-tracker", Timeout: 5},
			{Event: "PreToolUse", Command: "~/.claude/hooks/agm-pretool-test-session-guard", Timeout: 5},
			{Event: "SessionStart", Command: "~/.claude/hooks/session-start/agm-state-ready", Timeout: 5},
			{Event: "SessionStart", Command: "~/.claude/hooks/session-start/agm-plan-continuity", Timeout: 10},
		}

		count, err := registerHooksInSettings(home, regs)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if count != 5 {
			t.Errorf("registered count = %d, want 5", count)
		}

		data, _ := os.ReadFile(filepath.Join(home, ".claude", "settings.json"))
		var settings map[string]interface{}
		json.Unmarshal(data, &settings)

		hooksMap := settings["hooks"].(map[string]interface{})

		// Check event counts
		postTool := hooksMap["PostToolUse"].([]interface{})
		if len(postTool) != 1 {
			t.Errorf("PostToolUse groups = %d, want 1", len(postTool))
		}
		preTool := hooksMap["PreToolUse"].([]interface{})
		if len(preTool) != 2 {
			t.Errorf("PreToolUse groups = %d, want 2", len(preTool))
		}
		sessionStart := hooksMap["SessionStart"].([]interface{})
		if len(sessionStart) != 2 {
			t.Errorf("SessionStart groups = %d, want 2", len(sessionStart))
		}
	})
}
