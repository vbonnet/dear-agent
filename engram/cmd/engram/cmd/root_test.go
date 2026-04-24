package cmd

import (
	"strings"
	"testing"
)

// TestRootCommand_Initialization verifies root command sets up correctly
func TestRootCommand_Initialization(t *testing.T) {
	if rootCmd == nil {
		t.Fatal("rootCmd is nil")
	}

	if rootCmd.Use == "" {
		t.Error("rootCmd.Use is empty")
	}

	if rootCmd.Short == "" {
		t.Error("rootCmd.Short is empty")
	}

	if rootCmd.Long == "" {
		t.Error("rootCmd.Long is empty")
	}
}

// TestRootCommand_Subcommands verifies all expected subcommands registered
func TestRootCommand_Subcommands(t *testing.T) {
	expectedSubcommands := []string{"index", "retrieve", "plugin", "doctor", "slashcmd", "config", "completion"}

	for _, expected := range expectedSubcommands {
		found := false
		for _, sub := range rootCmd.Commands() {
			if sub.Name() == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected subcommand %q not found", expected)
		}
	}
}

// TestRootCommand_Version verifies version command works
func TestRootCommand_Version(t *testing.T) {
	versionInfo := getVersionInfo()
	if versionInfo == "" {
		t.Error("getVersionInfo() returned empty string")
	}

	// Should contain version components
	if !strings.Contains(versionInfo, "Commit:") {
		t.Error("version info missing 'Commit:' field")
	}

	if !strings.Contains(versionInfo, "Built:") {
		t.Error("version info missing 'Built:' field")
	}

	if !strings.Contains(versionInfo, "Go:") {
		t.Error("version info missing 'Go:' field")
	}
}

// TestCompletionCommand_Initialization verifies completion command structure
func TestCompletionCommand_Initialization(t *testing.T) {
	if completionCmd == nil {
		t.Fatal("completionCmd is nil")
	}

	if completionCmd.Use == "" {
		t.Error("completionCmd.Use is empty")
	}

	if completionCmd.Short == "" {
		t.Error("completionCmd.Short is empty")
	}

	if completionCmd.Long == "" {
		t.Error("completionCmd.Long is empty")
	}

	// Verify valid args
	validArgs := []string{"bash", "zsh", "fish", "powershell"}
	for _, arg := range validArgs {
		found := false
		for _, valid := range completionCmd.ValidArgs {
			if valid == arg {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("completionCmd.ValidArgs missing %q", arg)
		}
	}
}

// TestCompletionCommand_ValidArgs verifies completion command accepts valid shells
func TestCompletionCommand_ValidArgs(t *testing.T) {
	validShells := []string{"bash", "zsh", "fish", "powershell"}

	for _, shell := range validShells {
		t.Run(shell, func(t *testing.T) {
			found := false
			for _, validArg := range completionCmd.ValidArgs {
				if validArg == shell {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("completion command missing valid arg %q", shell)
			}
		})
	}
}

// TestCompletionCommand_ArgsValidation verifies completion command requires exactly one arg
func TestCompletionCommand_ArgsValidation(t *testing.T) {
	// Verify Args validator is set
	if completionCmd.Args == nil {
		t.Error("completionCmd.Args is nil, want ExactArgs(1)")
	}
}

// TestGetVersionInfo verifies version info formatting
func TestGetVersionInfo(t *testing.T) {
	info := getVersionInfo()

	// Should be non-empty
	if info == "" {
		t.Fatal("getVersionInfo() returned empty string")
	}

	// Should contain all components
	components := []string{"Commit:", "Built:", "Go:"}
	for _, comp := range components {
		if !strings.Contains(info, comp) {
			t.Errorf("getVersionInfo() missing component %q", comp)
		}
	}
}

// TestRootCommand_UsageTemplate verifies root command has usage template
func TestRootCommand_UsageTemplate(t *testing.T) {
	// Verify the command has essential fields for help/usage
	if rootCmd.Use == "" {
		t.Error("rootCmd.Use is empty")
	}

	if rootCmd.Short == "" {
		t.Error("rootCmd.Short is empty")
	}

	if rootCmd.Long == "" {
		t.Error("rootCmd.Long is empty")
	}

	// Verify subcommands are registered (help functionality depends on this)
	if len(rootCmd.Commands()) == 0 {
		t.Error("rootCmd has no subcommands registered")
	}
}
