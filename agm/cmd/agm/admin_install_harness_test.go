package main

import (
	"encoding/json"
	"testing"

	"github.com/vbonnet/dear-agent/agm/internal/ops"
)

func TestInstallHarnessCmd_InvalidHarness(t *testing.T) {
	err := executeFreshCommandForTest(t, newInstallHarnessCommand, []string{"invalid-harness"})
	if err == nil {
		t.Fatal("Expected error for invalid harness")
	}
}

func TestInstallHarnessCmd_ValidHarness(t *testing.T) {
	tests := []string{"codex", "gemini", "opencode", "pi"}

	for _, harness := range tests {
		t.Run(harness, func(t *testing.T) {
			// This test just verifies the command structure
			// Actual installation behavior depends on system state
			cmd := newInstallHarnessCommand()

			if cmd.Use != "install-harness <harness>" {
				t.Fatalf("Unexpected command use: %s", cmd.Use)
			}

			if err := cmd.Args(cmd, []string{harness}); err != nil {
				t.Fatalf("command rejected valid harness argument %q: %v", harness, err)
			}
		})
	}
}

func TestInstallHarnessCmd_JSONOutput(t *testing.T) {
	// Test that JSON output can be parsed
	result := &ops.HarnessInstallResult{
		Success: true,
		Harness: "codex",
		Message: "Test message",
		Version: "1.0.0",
		Path:    "/usr/bin/codex",
	}

	jsonStr, err := ops.ResultToJSON(result)
	if err != nil {
		t.Fatalf("Failed to marshal result: %v", err)
	}

	var parsed ops.HarnessInstallResult
	err = json.Unmarshal([]byte(jsonStr), &parsed)
	if err != nil {
		t.Fatalf("Failed to unmarshal JSON: %v", err)
	}

	if parsed.Harness != "codex" {
		t.Fatalf("Parsed harness = %s, expected codex", parsed.Harness)
	}

	if !parsed.Success {
		t.Fatal("Parsed success = false, expected true")
	}
}

func TestInstallCodexCmd_Exists(t *testing.T) {
	// Verify the command exists
	if installCodexCmd.Use != "install-codex" {
		t.Fatalf("Unexpected command use: %s", installCodexCmd.Use)
	}

	if !installCodexCmd.Hidden {
		t.Fatal("Expected install-codex command to be hidden")
	}
}

func TestInstallHarnessValidateHarnessTypes(t *testing.T) {
	// Test that all valid harness types are accepted
	validTypes := []string{"codex", "gemini", "opencode", "pi"}

	for _, harness := range validTypes {
		_, err := ops.ValidateHarness(harness)
		if err != nil {
			t.Fatalf("ValidateHarness(%s) failed: %v", harness, err)
		}
	}
}

func TestInstallHarnessJSONFlag(t *testing.T) {
	cmd := newInstallHarnessCommand()

	// Test that flags are properly defined
	if cmd.Flags().Lookup("json") == nil {
		t.Fatal("--json flag not found")
	}

	if cmd.Flags().Lookup("quiet") == nil {
		t.Fatal("--quiet flag not found")
	}
}
