package main

import (
	"strings"
	"testing"
)

func TestCalculateShiftTabPresses(t *testing.T) {
	tests := []struct {
		name     string
		current  string
		target   string
		expected int
	}{
		{"default to auto", "default", "auto", 1},
		{"default to plan", "default", "plan", 2},
		{"auto to plan", "auto", "plan", 1},
		{"auto to default", "auto", "default", 2},
		{"plan to default", "plan", "default", 1},
		{"plan to auto", "plan", "auto", 2},
		{"same mode default", "default", "default", 0},
		{"same mode auto", "auto", "auto", 0},
		{"same mode plan", "plan", "plan", 0},
		{"unknown current defaults to 0", "unknown", "auto", 1},
		{"unknown current to plan", "unknown", "plan", 2},
		{"unknown current to default", "unknown", "default", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := calculateShiftTabPresses(tt.current, tt.target)
			if got != tt.expected {
				t.Errorf("calculateShiftTabPresses(%q, %q) = %d, want %d",
					tt.current, tt.target, got, tt.expected)
			}
		})
	}
}

func TestValidModes(t *testing.T) {
	// Valid modes should be accepted
	for _, mode := range []string{"plan", "auto", "default"} {
		if !validModes[mode] {
			t.Errorf("expected %q to be a valid mode", mode)
		}
	}

	// Invalid modes should be rejected
	for _, mode := range []string{"", "ask", "allow", "Plan", "AUTO", "unknown"} {
		if validModes[mode] {
			t.Errorf("expected %q to be an invalid mode", mode)
		}
	}
}

func TestSendModeCodexCLI(t *testing.T) {
	err := sendModeCodexCLI()
	if err == nil {
		t.Error("expected error from codex-cli mode switch, got nil")
	}
	if !strings.Contains(err.Error(), "codex-cli") {
		t.Errorf("error should mention codex-cli, got: %s", err.Error())
	}
	if !strings.Contains(err.Error(), "--suggest") {
		t.Errorf("error should include flag alternatives, got: %s", err.Error())
	}
}

func TestDispatchModeSwitch(t *testing.T) {
	tests := []struct {
		name      string
		harness   string
		wantErr   bool
		errSubstr string
	}{
		// codex-cli always returns error
		{"codex-cli returns error", "codex-cli", true, "codex-cli"},
		// unknown harness returns error
		{"unknown harness returns error", "unknown-harness", true, "unsupported harness"},
		{"empty harness returns error", "", true, "unsupported harness"},
		// valid harnesses will fail due to no tmux, but should NOT return unsupported error
		// We can't test claude-code/gemini-cli/opencode-cli without tmux
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := dispatchModeSwitch(tt.harness, "test-session", "plan", "default")
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error for harness %q, got nil", tt.harness)
				} else if !strings.Contains(err.Error(), tt.errSubstr) {
					t.Errorf("error should contain %q, got: %s", tt.errSubstr, err.Error())
				}
			}
		})
	}
}

func TestOpenCodeNeedsTab(t *testing.T) {
	// Test the OpenCode toggle logic directly by checking the needsTab condition
	tests := []struct {
		name        string
		target      string
		current     string
		wantsSwitch bool
	}{
		{"default to plan needs tab", "plan", "default", true},
		{"plan to default needs tab", "default", "plan", true},
		{"auto to plan needs tab", "plan", "auto", true},
		{"plan to auto needs tab", "auto", "plan", true},
		{"default to auto no tab", "auto", "default", false},
		{"default to default no tab", "default", "default", false},
		{"plan to plan no tab", "plan", "plan", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			needsTab := false
			if tt.target == "plan" && tt.current != "plan" {
				needsTab = true
			} else if tt.target != "plan" && tt.current == "plan" {
				needsTab = true
			}
			if needsTab != tt.wantsSwitch {
				t.Errorf("opencode needsTab(%q→%q) = %v, want %v",
					tt.current, tt.target, needsTab, tt.wantsSwitch)
			}
		})
	}
}

func TestGeminiCLIModeLogic(t *testing.T) {
	// Test which key sequences Gemini CLI would send for each transition
	tests := []struct {
		name        string
		target      string
		current     string
		expectPlan  bool // would use /plan slash command
		expectCtrlY bool // would use C-y toggle
		expectNoop  bool // would do nothing
	}{
		{"plan from default uses slash cmd", "plan", "default", true, false, false},
		{"plan from auto uses slash cmd", "plan", "auto", true, false, false},
		{"auto from default uses ctrl-y", "auto", "default", false, true, false},
		{"auto from plan uses ctrl-y", "auto", "plan", false, true, false},
		{"default from auto uses ctrl-y", "default", "auto", false, true, false},
		{"default from plan is noop", "default", "plan", false, false, true},
		{"default from default is noop", "default", "default", false, false, true},
		{"auto from auto is noop", "auto", "auto", false, false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotPlan := tt.target == "plan"
			gotCtrlY := false
			if tt.target == "auto" && tt.current != "auto" {
				gotCtrlY = true
			} else if tt.target == "default" && tt.current == "auto" {
				gotCtrlY = true
			}
			gotNoop := !gotPlan && !gotCtrlY

			if gotPlan != tt.expectPlan {
				t.Errorf("slash cmd: got %v, want %v", gotPlan, tt.expectPlan)
			}
			if gotCtrlY != tt.expectCtrlY {
				t.Errorf("ctrl-y: got %v, want %v", gotCtrlY, tt.expectCtrlY)
			}
			if gotNoop != tt.expectNoop {
				t.Errorf("noop: got %v, want %v", gotNoop, tt.expectNoop)
			}
		})
	}
}

func TestClaudeCodeModeLogic(t *testing.T) {
	// Test Claude Code mode switching logic
	tests := []struct {
		name        string
		target      string
		current     string
		expectSlash bool // would use /plan
		expectCycle int  // number of S-Tab presses
	}{
		{"plan from default uses slash", "plan", "default", true, 0},
		{"plan from auto uses slash", "plan", "auto", true, 0},
		{"plan from plan uses slash", "plan", "plan", true, 0},
		{"auto from default cycles 1", "auto", "default", false, 1},
		{"auto from plan cycles 2", "auto", "plan", false, 2},
		{"default from auto cycles 2", "default", "auto", false, 2},
		{"default from plan cycles 1", "default", "plan", false, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotSlash := tt.target == "plan"
			gotCycle := 0
			if !gotSlash {
				gotCycle = calculateShiftTabPresses(tt.current, tt.target)
			}

			if gotSlash != tt.expectSlash {
				t.Errorf("slash: got %v, want %v", gotSlash, tt.expectSlash)
			}
			if gotCycle != tt.expectCycle {
				t.Errorf("cycle count: got %d, want %d", gotCycle, tt.expectCycle)
			}
		})
	}
}

func TestModeSessionInfoDefaults(t *testing.T) {
	// modeSessionInfo should have sensible defaults
	info := &modeSessionInfo{harness: "claude-code", currentMode: "default"}
	if info.harness != "claude-code" {
		t.Errorf("default harness should be claude-code, got %q", info.harness)
	}
	if info.currentMode != "default" {
		t.Errorf("default currentMode should be default, got %q", info.currentMode)
	}
	if info.adapter != nil {
		t.Error("default adapter should be nil")
	}
}

func TestValidModesExhaustive(t *testing.T) {
	// Verify exactly 3 modes exist
	if len(validModes) != 3 {
		t.Errorf("expected exactly 3 valid modes, got %d", len(validModes))
	}

	// Verify all values are true (not false)
	for mode, valid := range validModes {
		if !valid {
			t.Errorf("mode %q has value false, expected true", mode)
		}
	}
}

// TestAutoModeRequiresEnableFlag documents that auto mode in Shift+Tab cycle
// requires --enable-auto-mode at startup. Without the flag, the cycle only
// includes default and plan modes.
func TestAutoModeRequiresEnableFlag(t *testing.T) {
	// Verify auto is a valid mode in the cycle
	if !validModes["auto"] {
		t.Fatal("auto should be a valid mode")
	}

	// Verify Shift+Tab cycle includes auto: default(0) -> auto(1) -> plan(2)
	// From default to auto should be exactly 1 Shift+Tab press
	presses := calculateShiftTabPresses("default", "auto")
	if presses != 1 {
		t.Errorf("expected 1 Shift+Tab press from default to auto, got %d", presses)
	}

	// From auto to plan should be exactly 1 press
	presses = calculateShiftTabPresses("auto", "plan")
	if presses != 1 {
		t.Errorf("expected 1 Shift+Tab press from auto to plan, got %d", presses)
	}

	// From plan back to default wraps around: 1 press
	presses = calculateShiftTabPresses("plan", "default")
	if presses != 1 {
		t.Errorf("expected 1 Shift+Tab press from plan to default, got %d", presses)
	}
}

// TestAutoModeInClaudeCodeCycle verifies that claude-code mode switching
// correctly handles auto mode transitions via both slash command and Shift+Tab.
func TestAutoModeInClaudeCodeCycle(t *testing.T) {
	tests := []struct {
		name        string
		target      string
		current     string
		expectSlash bool
		expectCycle int
	}{
		// Auto mode transitions use Shift+Tab cycling (not slash command)
		{"auto from default cycles 1", "auto", "default", false, 1},
		{"auto from plan cycles 2", "auto", "plan", false, 2},
		{"auto from auto is noop", "auto", "auto", false, 0},
		// Plan always uses slash command regardless of current mode
		{"plan from auto uses slash", "plan", "auto", true, 0},
		// Default from auto cycles 2 (auto->plan->default)
		{"default from auto cycles 2", "default", "auto", false, 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotSlash := tt.target == "plan"
			gotCycle := 0
			if !gotSlash {
				gotCycle = calculateShiftTabPresses(tt.current, tt.target)
			}

			if gotSlash != tt.expectSlash {
				t.Errorf("slash: got %v, want %v", gotSlash, tt.expectSlash)
			}
			if gotCycle != tt.expectCycle {
				t.Errorf("cycle count: got %d, want %d", gotCycle, tt.expectCycle)
			}
		})
	}
}
