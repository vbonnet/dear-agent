package main

import (
	"testing"

	"github.com/spf13/cobra"
)

// ---------------------------------------------------------------------------
// Command registration tests for check-worktrees
// ---------------------------------------------------------------------------

func TestCheckWorktreesCmd_Registration(t *testing.T) {
	// Verify the command is registered under adminCmd
	found := false
	for _, cmd := range adminCmd.Commands() {
		if cmd.Use == "check-worktrees" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected check-worktrees to be registered under admin command")
	}
}

func TestCheckWorktreesCmd_Use(t *testing.T) {
	if checkWorktreesCmd.Use != "check-worktrees" {
		t.Errorf("Expected Use 'check-worktrees', got %q", checkWorktreesCmd.Use)
	}
}

func TestCheckWorktreesCmd_Short(t *testing.T) {
	if checkWorktreesCmd.Short == "" {
		t.Error("Expected non-empty Short description")
	}
}

func TestCheckWorktreesCmd_Long(t *testing.T) {
	if checkWorktreesCmd.Long == "" {
		t.Error("Expected non-empty Long description")
	}
}

func TestCheckWorktreesCmd_RunE(t *testing.T) {
	if checkWorktreesCmd.RunE == nil {
		t.Error("Expected RunE to be set")
	}
}

func TestCheckWorktreesCmd_SessionFlag(t *testing.T) {
	flag := checkWorktreesCmd.Flags().Lookup("session")
	if flag == nil {
		t.Fatal("Expected --session flag to be registered")
	}
	if flag.DefValue != "" {
		t.Errorf("Expected default value '' for --session, got %q", flag.DefValue)
	}
	if flag.Usage == "" {
		t.Error("Expected non-empty usage for --session flag")
	}
}

func TestCheckWorktreesCmd_SessionFlagParsing(t *testing.T) {
	// Reset flag state
	oldVal := checkWorktreesSession
	defer func() { checkWorktreesSession = oldVal }()

	cmd := &cobra.Command{Use: "test"}
	cmd.AddCommand(checkWorktreesCmd)

	// Test flag parsing
	checkWorktreesCmd.Flags().Set("session", "my-session")
	if checkWorktreesSession != "my-session" {
		t.Errorf("Expected session flag 'my-session', got %q", checkWorktreesSession)
	}

	// Reset
	checkWorktreesCmd.Flags().Set("session", "")
}

func TestCheckWorktreesCmd_LongDescriptionMentionsExitCodes(t *testing.T) {
	long := checkWorktreesCmd.Long
	expectations := []string{"exit gate", "Exit codes:", "orphaned"}
	for _, exp := range expectations {
		found := false
		for i := 0; i <= len(long)-len(exp); i++ {
			if long[i:i+len(exp)] == exp {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected Long description to contain %q", exp)
		}
	}
}
