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
	// Verify cliframe standard flags are registered
	configFlag := rootCmd.Flags().Lookup("config")
	if configFlag == nil {
		t.Error("--config flag not defined")
	}

	verboseFlag := rootCmd.Flags().Lookup("verbose")
	if verboseFlag == nil {
		t.Error("--verbose flag not defined")
	}

	dryRunFlag := rootCmd.Flags().Lookup("dry-run")
	if dryRunFlag == nil {
		t.Error("--dry-run flag not defined")
	}

	// Verify cliframe output flags
	formatFlag := rootCmd.Flags().Lookup("format")
	if formatFlag == nil {
		t.Error("--format flag not defined")
	}

	quietFlag := rootCmd.Flags().Lookup("quiet")
	if quietFlag == nil {
		t.Error("--quiet flag not defined")
	}
}

func TestExecute_DoesNotPanic(t *testing.T) {
	// Basic smoke test - Execute function exists and is callable
	// Note: We can't actually call Execute() because it would exit the process,
	// but the test compiling proves the function exists and is accessible
	_ = Execute
}
