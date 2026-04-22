package main

import (
	"testing"

	"github.com/vbonnet/dear-agent/agm/internal/config"
)

func TestShouldEnableSandbox(t *testing.T) {
	tests := []struct {
		name           string
		cfgEnabled     bool
		enableFlag     bool
		disableFlag    bool
		expectedResult bool
	}{
		{
			name:           "no-sandbox flag disables even when config enabled",
			cfgEnabled:     true,
			enableFlag:     false,
			disableFlag:    true,
			expectedResult: false,
		},
		{
			name:           "default config (enabled=true), no flags = sandbox ON",
			cfgEnabled:     true,
			enableFlag:     false,
			disableFlag:    false,
			expectedResult: true,
		},
		{
			name:           "config disabled, no flags = sandbox OFF",
			cfgEnabled:     false,
			enableFlag:     false,
			disableFlag:    false,
			expectedResult: false,
		},
		{
			name:           "no-sandbox overrides config",
			cfgEnabled:     true,
			enableFlag:     false,
			disableFlag:    true,
			expectedResult: false,
		},
		{
			name:           "enable flag still works for backward compat",
			cfgEnabled:     false,
			enableFlag:     true,
			disableFlag:    false,
			expectedResult: true,
		},
		{
			name:           "disable flag takes precedence over enable",
			cfgEnabled:     true,
			enableFlag:     true,
			disableFlag:    true,
			expectedResult: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save and restore original cfg
			originalCfg := cfg
			defer func() { cfg = originalCfg }()

			// Set test config
			cfg = &config.Config{
				Sandbox: config.SandboxConfig{
					Enabled: tt.cfgEnabled,
				},
			}

			result := shouldEnableSandbox(tt.enableFlag, tt.disableFlag)
			if result != tt.expectedResult {
				t.Errorf("shouldEnableSandbox() = %v, want %v", result, tt.expectedResult)
			}
		})
	}
}

func TestSandboxIntegration_Documentation(t *testing.T) {
	t.Log("Task 1.4: AGM Sandbox Integration - Session Creation (sandbox-by-default)")
	t.Log("")
	t.Log("IMPLEMENTATION:")
	t.Log("1. Sandbox is ON by default (config.Sandbox.Enabled=true)")
	t.Log("2. --sandbox flag REMOVED (breaking change)")
	t.Log("3. --no-sandbox flag disables sandbox")
	t.Log("4. --sandbox-provider selects provider (auto, overlayfs, apfs, claudecode-worktree, mock)")
	t.Log("5. SandboxSpec type added for provider-agnostic configuration")
	t.Log("6. ClaudeCodeProvider wraps Claude Code native worktree isolation")
	t.Log("")
	t.Log("FLAGS:")
	t.Log("--no-sandbox        Disable sandbox isolation (sandbox is ON by default)")
	t.Log("--sandbox-provider  Specify provider (auto, overlayfs, apfs, claudecode-worktree, mock)")
	t.Log("")
	t.Log("BEHAVIOR:")
	t.Log("- Default: Sandbox enabled (config.Sandbox.Enabled=true)")
	t.Log("- If --no-sandbox: Sandbox disabled")
	t.Log("- If sandbox enabled: workDir changed to sandbox merged path")
	t.Log("- If error during creation: Sandbox cleaned up automatically")
}
