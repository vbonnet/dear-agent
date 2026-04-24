package main

import (
	"testing"

	"github.com/spf13/cobra"
)

// newEnvDefaultsTestCmd creates a fresh cobra.Command with the same flags
// that resolveEnvVarDefaults expects, avoiding shared state between tests.
func newEnvDefaultsTestCmd() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Flags().String("harness", "", "")
	cmd.Flags().String("model", "", "")
	cmd.Flags().String("mode", "", "")
	return cmd
}

func TestEnvVarDefaults_HarnessEnvUsed(t *testing.T) {
	origHarness := harnessName
	defer func() { harnessName = origHarness }()
	harnessName = ""

	t.Setenv("AGM_DEFAULT_HARNESS", "gemini-cli")

	cmd := newEnvDefaultsTestCmd()
	resolveEnvVarDefaults(cmd)

	if harnessName != "gemini-cli" {
		t.Errorf("expected harnessName to be %q, got %q", "gemini-cli", harnessName)
	}
}

func TestEnvVarDefaults_HarnessFlagWins(t *testing.T) {
	origHarness := harnessName
	defer func() { harnessName = origHarness }()
	harnessName = "claude-code"

	t.Setenv("AGM_DEFAULT_HARNESS", "gemini-cli")

	cmd := newEnvDefaultsTestCmd()
	cmd.Flags().Set("harness", "claude-code") // marks Changed

	resolveEnvVarDefaults(cmd)

	if harnessName != "claude-code" {
		t.Errorf("expected flag to win with %q, got %q", "claude-code", harnessName)
	}
}

func TestEnvVarDefaults_ModelEnvUsed(t *testing.T) {
	origModel := modelName
	defer func() { modelName = origModel }()
	modelName = ""

	t.Setenv("AGM_DEFAULT_MODEL", "opus[1m]")

	cmd := newEnvDefaultsTestCmd()
	resolveEnvVarDefaults(cmd)

	if modelName != "opus[1m]" {
		t.Errorf("expected modelName to be %q, got %q", "opus[1m]", modelName)
	}
}

func TestEnvVarDefaults_ModelFlagWins(t *testing.T) {
	origModel := modelName
	defer func() { modelName = origModel }()
	modelName = "sonnet"

	t.Setenv("AGM_DEFAULT_MODEL", "opus[1m]")

	cmd := newEnvDefaultsTestCmd()
	cmd.Flags().Set("model", "sonnet") // marks Changed

	resolveEnvVarDefaults(cmd)

	if modelName != "sonnet" {
		t.Errorf("expected flag to win with %q, got %q", "sonnet", modelName)
	}
}

func TestEnvVarDefaults_ModeEnvUsed(t *testing.T) {
	origMode := modeFlagValue
	defer func() { modeFlagValue = origMode }()
	modeFlagValue = ""

	t.Setenv("AGM_DEFAULT_MODE", "plan")

	cmd := newEnvDefaultsTestCmd()
	resolveEnvVarDefaults(cmd)

	if modeFlagValue != "plan" {
		t.Errorf("expected modeFlagValue to be %q, got %q", "plan", modeFlagValue)
	}
}

func TestEnvVarDefaults_ModeFlagWins(t *testing.T) {
	origMode := modeFlagValue
	defer func() { modeFlagValue = origMode }()
	modeFlagValue = "auto"

	t.Setenv("AGM_DEFAULT_MODE", "plan")

	cmd := newEnvDefaultsTestCmd()
	cmd.Flags().Set("mode", "auto") // marks Changed

	resolveEnvVarDefaults(cmd)

	if modeFlagValue != "auto" {
		t.Errorf("expected flag to win with %q, got %q", "auto", modeFlagValue)
	}
}

func TestEnvVarDefaults_EmptyEnvNoEffect(t *testing.T) {
	origHarness := harnessName
	origModel := modelName
	origMode := modeFlagValue
	defer func() {
		harnessName = origHarness
		modelName = origModel
		modeFlagValue = origMode
	}()
	harnessName = ""
	modelName = ""
	modeFlagValue = ""

	// Explicitly clear env vars that may be set in the test environment
	t.Setenv("AGM_DEFAULT_HARNESS", "")
	t.Setenv("AGM_DEFAULT_MODEL", "")
	t.Setenv("AGM_DEFAULT_MODE", "")

	cmd := newEnvDefaultsTestCmd()
	resolveEnvVarDefaults(cmd)

	if harnessName != "" {
		t.Errorf("expected harnessName to remain empty, got %q", harnessName)
	}
	if modelName != "" {
		t.Errorf("expected modelName to remain empty, got %q", modelName)
	}
	if modeFlagValue != "" {
		t.Errorf("expected modeFlagValue to remain empty, got %q", modeFlagValue)
	}
}

func TestEnvVarDefaults_InvalidPassesThrough(t *testing.T) {
	origMode := modeFlagValue
	defer func() { modeFlagValue = origMode }()
	modeFlagValue = ""

	// "turbo" is not a valid mode, but resolveEnvVarDefaults should not validate --
	// it just copies the env value. Validation happens later in RunE.
	t.Setenv("AGM_DEFAULT_MODE", "turbo")

	cmd := newEnvDefaultsTestCmd()
	resolveEnvVarDefaults(cmd)

	if modeFlagValue != "turbo" {
		t.Errorf("expected modeFlagValue to be %q (pass-through), got %q", "turbo", modeFlagValue)
	}
}
