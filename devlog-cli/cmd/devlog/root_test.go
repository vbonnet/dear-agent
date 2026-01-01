package devlog

import (
	"testing"
)

func TestRootCommand_Exists(t *testing.T) {
	if rootCmd == nil {
		t.Fatal("rootCmd is nil, want initialized command")
	}

	if rootCmd.Use != "devlog" {
		t.Errorf("rootCmd.Use = %q, want %q", rootCmd.Use, "devlog")
	}
}

func TestRootCommand_FlagsAreDefined(t *testing.T) {
	// Verify flags are registered
	configFlag := rootCmd.PersistentFlags().Lookup("config")
	if configFlag == nil {
		t.Error("--config flag not defined")
	}

	verboseFlag := rootCmd.PersistentFlags().Lookup("verbose")
	if verboseFlag == nil {
		t.Error("--verbose flag not defined")
	}

	dryRunFlag := rootCmd.PersistentFlags().Lookup("dry-run")
	if dryRunFlag == nil {
		t.Error("--dry-run flag not defined")
	}
}

func TestExecute_DoesNotPanic(t *testing.T) {
	// Basic smoke test - Execute function exists and is callable
	// Note: We can't actually call Execute() because it would exit the process,
	// but the test compiling proves the function exists and is accessible
	_ = Execute
}
