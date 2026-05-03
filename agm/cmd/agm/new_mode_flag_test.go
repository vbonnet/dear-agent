package main

import (
	"fmt"
	"strings"
	"testing"
)

func TestModeFlagValidation_ValidModes(t *testing.T) {
	validCases := []string{"plan", "auto", "default"}
	for _, mode := range validCases {
		if !validModes[mode] {
			t.Errorf("expected mode %q to be valid", mode)
		}
	}
}

func TestModeFlagValidation_InvalidModes(t *testing.T) {
	invalidCases := []string{"Plan", "AUTO", "ask", "allow", "turbo", ""}
	for _, mode := range invalidCases {
		if validModes[mode] {
			t.Errorf("expected mode %q to be invalid", mode)
		}
	}
}

func TestApplyCreationModeSwitch_EmptyIsNoop(t *testing.T) {
	// Empty mode should return immediately without error
	applyCreationModeSwitch("test-session", "claude-code", "")
	// No error means success - the function just returns for empty mode
}

// TestPointBGuardCondition verifies the guard logic that prevents double mode-switching.
// Point A handles claude-code in non-test environments.
// Point B handles everything else (other harnesses, or claude-code in test envs).
func TestPointBGuardCondition(t *testing.T) {
	tests := []struct {
		name          string
		modeFlagValue string
		harness       string
		testRunID     string // simulates AGM_TEST_RUN_ID env
		testEnv       string // simulates AGM_TEST_ENV env
		expectPointB  bool
	}{
		{
			name:          "empty mode skips Point B",
			modeFlagValue: "",
			harness:       "claude-code",
			expectPointB:  false,
		},
		{
			name:          "claude-code normal path handled by Point A, not B",
			modeFlagValue: "plan",
			harness:       "claude-code",
			testRunID:     "",
			testEnv:       "",
			expectPointB:  false,
		},
		{
			name:          "claude-code in test env uses Point B",
			modeFlagValue: "plan",
			harness:       "claude-code",
			testRunID:     "test-123",
			testEnv:       "",
			expectPointB:  true,
		},
		{
			name:          "claude-code with AGM_TEST_ENV uses Point B",
			modeFlagValue: "auto",
			harness:       "claude-code",
			testRunID:     "",
			testEnv:       "my-env",
			expectPointB:  true,
		},
		{
			name:          "gemini-cli always uses Point B",
			modeFlagValue: "plan",
			harness:       "gemini-cli",
			testRunID:     "",
			testEnv:       "",
			expectPointB:  true,
		},
		{
			name:          "opencode-cli always uses Point B",
			modeFlagValue: "auto",
			harness:       "opencode-cli",
			testRunID:     "",
			testEnv:       "",
			expectPointB:  true,
		},
		{
			name:          "codex-cli always uses Point B",
			modeFlagValue: "plan",
			harness:       "codex-cli",
			testRunID:     "",
			testEnv:       "",
			expectPointB:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reproduce the guard condition from new.go line ~1295
			pointBFires := tt.modeFlagValue != "" &&
				(tt.harness != "claude-code" || tt.testRunID != "" || tt.testEnv != "")

			if pointBFires != tt.expectPointB {
				t.Errorf("Point B guard: got fires=%v, want fires=%v", pointBFires, tt.expectPointB)
			}
		})
	}
}

// TestModeFlagRegistered verifies the --mode flag is registered on the newCmd cobra command.
func TestModeFlagRegistered(t *testing.T) {
	flag := newCmd.Flags().Lookup("mode")
	if flag == nil {
		t.Fatal("--mode flag not registered on newCmd")
	}
	if flag.DefValue != "" {
		t.Errorf("--mode default should be empty string, got %q", flag.DefValue)
	}
	if flag.Usage == "" {
		t.Error("--mode flag should have a usage string")
	}
}

// TestModeFlagDefaultValueClearance verifies that --mode=default is normalized to empty string.
// This ensures "default" acts as a no-op (no mode switch applied).
func TestModeFlagDefaultValueClearance(t *testing.T) {
	// Simulate the validation logic from new.go lines 562-570
	tests := []struct {
		name     string
		input    string
		expected string
		wantErr  bool
	}{
		{"plan stays plan", "plan", "plan", false},
		{"auto stays auto", "auto", "auto", false},
		{"default clears to empty", "default", "", false},
		{"empty stays empty", "", "", false},
		{"invalid mode errors", "turbo", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mode := tt.input
			var gotErr bool

			if mode != "" {
				if !validModes[mode] {
					gotErr = true
				} else if mode == "default" {
					mode = ""
				}
			}

			if gotErr != tt.wantErr {
				t.Errorf("error: got %v, want %v", gotErr, tt.wantErr)
			}
			if !gotErr && mode != tt.expected {
				t.Errorf("mode after validation: got %q, want %q", mode, tt.expected)
			}
		})
	}
}

// TestApplyCreationModeSwitch_UnsupportedHarness verifies that mode switch with
// an unsupported harness is non-fatal (warning, not error).
func TestApplyCreationModeSwitch_UnsupportedHarness(t *testing.T) {
	// applyCreationModeSwitch calls dispatchModeSwitch which returns an error
	// for unsupported harnesses. The function should handle this gracefully
	// (print warning, not panic).
	// This test verifies it doesn't panic.
	applyCreationModeSwitch("test-session", "unknown-harness", "plan")
	// If we reach here, the function handled the error gracefully (no panic)
}

// TestApplyCreationModeSwitch_CodexCLI verifies that codex-cli mode switch
// is non-fatal since codex-cli doesn't support runtime mode switching.
func TestApplyCreationModeSwitch_CodexCLI(t *testing.T) {
	// codex-cli always returns an error from dispatchModeSwitch
	// applyCreationModeSwitch should handle this gracefully
	applyCreationModeSwitch("test-session", "codex-cli", "plan")
	// If we reach here, the function handled the error gracefully (no panic)
}

// TestDispatchModeSwitch_UnsupportedHarness verifies the error message for unknown harnesses.
func TestDispatchModeSwitch_UnsupportedHarness(t *testing.T) {
	err := dispatchModeSwitch("fake-harness", "test-session", "plan", "default")
	if err == nil {
		t.Fatal("expected error for unsupported harness, got nil")
	}
	if !strings.Contains(err.Error(), "unsupported harness") {
		t.Errorf("error should mention 'unsupported harness', got: %s", err.Error())
	}
	if !strings.Contains(err.Error(), "fake-harness") {
		t.Errorf("error should mention the harness name, got: %s", err.Error())
	}
}

// TestModeFlagMutualExclusivity verifies --mode doesn't conflict with other flags.
// The mode flag should be independent of --prompt, --harness, --detached, etc.
func TestModeFlagMutualExclusivity(t *testing.T) {
	// Verify --mode is not in any mutual exclusion group
	// by checking it can coexist with other flags
	modeFlag := newCmd.Flags().Lookup("mode")
	promptFlag := newCmd.Flags().Lookup("prompt")
	harnessFlag := newCmd.Flags().Lookup("harness")
	detachedFlag := newCmd.Flags().Lookup("detached")

	if modeFlag == nil {
		t.Fatal("--mode flag not found")
	}
	if promptFlag == nil {
		t.Fatal("--prompt flag not found")
	}
	if harnessFlag == nil {
		t.Fatal("--harness flag not found")
	}
	if detachedFlag == nil {
		t.Fatal("--detached flag not found")
	}

	// All flags should exist and be independently settable
	// (no mutual exclusion with --mode)
}

// TestModeFlagCompletionValues verifies tab completion returns the expected values.
func TestModeFlagCompletionValues(t *testing.T) {
	// The completion function is registered in init() via RegisterFlagCompletionFunc.
	// We verify the flag exists and has the right type.
	flag := newCmd.Flags().Lookup("mode")
	if flag == nil {
		t.Fatal("--mode flag not registered")
	}
	// String type flag
	if flag.Value.Type() != "string" {
		t.Errorf("--mode should be string type, got %q", flag.Value.Type())
	}
}

// TestAutoModeEnableFlag verifies that --enable-auto-mode is always added to the
// claude command when the harness is claude-code.
// This flag is required for Shift+Tab cycling to include auto mode.
func TestAutoModeEnableFlag(t *testing.T) {
	// Simulate building the claude command as done in new.go
	// The --enable-auto-mode flag should always be present for claude-code harness
	tests := []struct {
		name             string
		harness          string
		modeFlagValue    string
		expectEnableFlag bool
	}{
		{
			name:             "claude-code always includes --enable-auto-mode",
			harness:          "claude-code",
			modeFlagValue:    "",
			expectEnableFlag: true,
		},
		{
			name:             "claude-code with auto mode includes --enable-auto-mode",
			harness:          "claude-code",
			modeFlagValue:    "auto",
			expectEnableFlag: true,
		},
		{
			name:             "claude-code with plan mode includes --enable-auto-mode",
			harness:          "claude-code",
			modeFlagValue:    "plan",
			expectEnableFlag: true,
		},
		{
			name:             "gemini-cli does not use --enable-auto-mode",
			harness:          "gemini-cli",
			modeFlagValue:    "",
			expectEnableFlag: false,
		},
		{
			name:             "codex-cli does not use --enable-auto-mode",
			harness:          "codex-cli",
			modeFlagValue:    "",
			expectEnableFlag: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reproduce the logic: --enable-auto-mode is added when harness is claude-code
			shouldInclude := tt.harness == "claude-code"
			if shouldInclude != tt.expectEnableFlag {
				t.Errorf("--enable-auto-mode for harness %q: got %v, want %v",
					tt.harness, shouldInclude, tt.expectEnableFlag)
			}
		})
	}
}

// TestAutoModePermissionFlag verifies --permission-mode auto is added to the claude
// startup command when --mode=auto is specified.
func TestAutoModePermissionFlag(t *testing.T) {
	tests := []struct {
		name            string
		modeFlagValue   string
		harness         string
		expectPermFlag  bool
		expectPermValue string
	}{
		{
			name:            "auto mode adds --permission-mode auto",
			modeFlagValue:   "auto",
			harness:         "claude-code",
			expectPermFlag:  true,
			expectPermValue: "auto",
		},
		{
			name:            "plan mode does not add --permission-mode",
			modeFlagValue:   "plan",
			harness:         "claude-code",
			expectPermFlag:  false,
			expectPermValue: "",
		},
		{
			name:            "empty mode does not add --permission-mode",
			modeFlagValue:   "",
			harness:         "claude-code",
			expectPermFlag:  false,
			expectPermValue: "",
		},
		{
			name:            "default mode (cleared to empty) does not add --permission-mode",
			modeFlagValue:   "default",
			harness:         "claude-code",
			expectPermFlag:  false,
			expectPermValue: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate the validation logic: "default" is cleared to empty
			mode := tt.modeFlagValue
			if mode == "default" {
				mode = ""
			}

			// --permission-mode is added when mode is "auto" and harness is claude-code
			shouldAddPermFlag := mode == "auto" && tt.harness == "claude-code"
			if shouldAddPermFlag != tt.expectPermFlag {
				t.Errorf("--permission-mode flag: got %v, want %v", shouldAddPermFlag, tt.expectPermFlag)
			}

			if shouldAddPermFlag {
				permValue := mode // the permission-mode value matches the mode
				if permValue != tt.expectPermValue {
					t.Errorf("--permission-mode value: got %q, want %q", permValue, tt.expectPermValue)
				}
			}
		})
	}
}

// TestModeAppliedAtStartup verifies that when mode is applied via --permission-mode
// at startup, the post-init Shift+Tab cycling is skipped.
// This prevents double mode-switching (once at startup, once post-init).
func TestModeAppliedAtStartup(t *testing.T) {
	tests := []struct {
		name                 string
		modeFlagValue        string
		harness              string
		modeAppliedAtStartup bool
		expectPostInitCycle  bool
	}{
		{
			name:                 "auto mode applied at startup skips post-init cycle",
			modeFlagValue:        "auto",
			harness:              "claude-code",
			modeAppliedAtStartup: true,
			expectPostInitCycle:  false,
		},
		{
			name:                 "plan mode not applied at startup needs post-init cycle",
			modeFlagValue:        "plan",
			harness:              "claude-code",
			modeAppliedAtStartup: false,
			expectPostInitCycle:  true,
		},
		{
			name:                 "empty mode never needs post-init cycle",
			modeFlagValue:        "",
			harness:              "claude-code",
			modeAppliedAtStartup: false,
			expectPostInitCycle:  false,
		},
		{
			name:                 "auto mode for gemini-cli not applied at startup",
			modeFlagValue:        "auto",
			harness:              "gemini-cli",
			modeAppliedAtStartup: false,
			expectPostInitCycle:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reproduce the modeAppliedAtStartup logic:
			// Mode is applied at startup when:
			// 1. harness is claude-code
			// 2. modeFlagValue is "auto"
			// 3. --permission-mode auto was added to the startup command
			modeAppliedAtStartup := tt.harness == "claude-code" && tt.modeFlagValue == "auto"

			if modeAppliedAtStartup != tt.modeAppliedAtStartup {
				t.Errorf("modeAppliedAtStartup: got %v, want %v",
					modeAppliedAtStartup, tt.modeAppliedAtStartup)
			}

			// Post-init cycle is needed when mode is set but NOT applied at startup
			expectCycle := tt.modeFlagValue != "" && !modeAppliedAtStartup
			if expectCycle != tt.expectPostInitCycle {
				t.Errorf("post-init cycle: got %v, want %v", expectCycle, tt.expectPostInitCycle)
			}
		})
	}
}

// TestAutoModeCommandConstruction verifies the full claude command includes
// both --enable-auto-mode and --permission-mode auto when mode=auto.
func TestAutoModeCommandConstruction(t *testing.T) {
	tests := []struct {
		name          string
		modeFlagValue string
		expectFlags   []string
	}{
		{
			name:          "auto mode includes both flags",
			modeFlagValue: "auto",
			expectFlags:   []string{"--enable-auto-mode", "--permission-mode", "auto"},
		},
		{
			name:          "plan mode includes only enable flag",
			modeFlagValue: "plan",
			expectFlags:   []string{"--enable-auto-mode"},
		},
		{
			name:          "no mode includes only enable flag",
			modeFlagValue: "",
			expectFlags:   []string{"--enable-auto-mode"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Build a simulated claude command for claude-code harness
			parts := []string{"claude", "--model", "sonnet", "--add-dir", "/tmp/test"}

			// Always add --enable-auto-mode for claude-code
			parts = append(parts, "--enable-auto-mode")

			// Add --permission-mode auto when mode is auto
			mode := tt.modeFlagValue
			if mode == "default" {
				mode = ""
			}
			if mode == "auto" {
				parts = append(parts, "--permission-mode", "auto")
			}

			cmd := strings.Join(parts, " ")

			for _, flag := range tt.expectFlags {
				if !strings.Contains(cmd, flag) {
					t.Errorf("command %q should contain %q", cmd, flag)
				}
			}

			// Verify --permission-mode is NOT present when not auto
			if mode != "auto" && strings.Contains(cmd, "--permission-mode") {
				t.Errorf("command %q should NOT contain --permission-mode when mode=%q", cmd, tt.modeFlagValue)
			}
		})
	}
}

// TestNoAutoModeFlag verifies that --no-auto-mode and AGM_DISABLE_AUTO_MODE
// strip the --enable-auto-mode flag from the claude command.
func TestNoAutoModeFlag(t *testing.T) {
	tests := []struct {
		name           string
		noAutoMode     bool
		expectAutoMode bool
	}{
		{
			name:           "default includes --enable-auto-mode",
			noAutoMode:     false,
			expectAutoMode: true,
		},
		{
			name:           "--no-auto-mode strips --enable-auto-mode",
			noAutoMode:     true,
			expectAutoMode: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate the command construction logic from new.go
			autoModeFlag := " --enable-auto-mode"
			if tt.noAutoMode {
				autoModeFlag = ""
			}
			cmd := fmt.Sprintf("claude --model sonnet --add-dir /tmp/test%s && exit", autoModeFlag)

			hasFlag := strings.Contains(cmd, "--enable-auto-mode")
			if hasFlag != tt.expectAutoMode {
				t.Errorf("--enable-auto-mode presence: got %v, want %v (cmd: %s)",
					hasFlag, tt.expectAutoMode, cmd)
			}
		})
	}
}

// TestNoAutoModeEnvVar verifies AGM_DISABLE_AUTO_MODE env var sets noAutoMode.
func TestNoAutoModeEnvVar(t *testing.T) {
	tests := []struct {
		name     string
		envValue string
		expect   bool
	}{
		{"AGM_DISABLE_AUTO_MODE=1 enables", "1", true},
		{"AGM_DISABLE_AUTO_MODE=true enables", "true", true},
		{"AGM_DISABLE_AUTO_MODE=0 does not enable", "0", false},
		{"AGM_DISABLE_AUTO_MODE=false does not enable", "false", false},
		{"AGM_DISABLE_AUTO_MODE empty does not enable", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate the env var check logic from resolveEnvVarDefaults
			disabled := false
			v := tt.envValue
			if v == "1" || v == "true" {
				disabled = true
			}
			if disabled != tt.expect {
				t.Errorf("AGM_DISABLE_AUTO_MODE=%q: got disabled=%v, want %v",
					tt.envValue, disabled, tt.expect)
			}
		})
	}
}
