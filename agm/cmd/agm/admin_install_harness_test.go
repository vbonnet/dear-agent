package main

import (
	"encoding/json"
	"testing"

	"github.com/vbonnet/dear-agent/agm/internal/ops"
)

func TestInstallHarnessCmd_InvalidHarness(t *testing.T) {
	// Reset flags
	installHarnessJSON = true
	installHarnessQuiet = false

	cmd := installHarnessCmd
	cmd.SetArgs([]string{"invalid-harness"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("Expected error for invalid harness")
	}
}

func TestInstallHarnessCmd_ValidHarness(t *testing.T) {
	// Reset flags
	installHarnessJSON = true
	installHarnessQuiet = false

	tests := []string{"codex", "gemini", "opencode"}

	for _, harness := range tests {
		t.Run(harness, func(t *testing.T) {
			// This test just verifies the command structure
			// Actual installation behavior depends on system state
			cmd := installHarnessCmd

			if cmd.Use != "install-harness <harness>" {
				t.Fatalf("Unexpected command use: %s", cmd.Use)
			}

			// Verify command accepts the harness argument
			// (ValidArgs is not required for ExactArgs; command validates at runtime)
			cmd.SetArgs([]string{harness})
		})
	}
}

func TestInstallHarnessCmd_JSONOutput(t *testing.T) {
	// Test that JSON output can be parsed
	result := &ops.HarnessInstallResult{
		Success:  true,
		Harness:  "codex",
		Message:  "Test message",
		Version:  "1.0.0",
		Path:     "/usr/bin/codex",
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
	validTypes := []string{"codex", "gemini", "opencode"}

	for _, harness := range validTypes {
		_, err := ops.ValidateHarness(harness)
		if err != nil {
			t.Fatalf("ValidateHarness(%s) failed: %v", harness, err)
		}
	}
}

func TestInstallHarnessJSONFlag(t *testing.T) {
	// Reset flags to defaults
	oldJSON := installHarnessJSON
	oldQuiet := installHarnessQuiet
	defer func() {
		installHarnessJSON = oldJSON
		installHarnessQuiet = oldQuiet
	}()

	// Test that flags are properly defined
	if installHarnessCmd.Flags().Lookup("json") == nil {
		t.Fatal("--json flag not found")
	}

	if installHarnessCmd.Flags().Lookup("quiet") == nil {
		t.Fatal("--quiet flag not found")
	}
}

