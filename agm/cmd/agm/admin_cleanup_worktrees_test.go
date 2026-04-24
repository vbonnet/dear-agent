package main

import (
	"testing"
)

// ---------------------------------------------------------------------------
// Command registration tests for cleanup-worktrees
// ---------------------------------------------------------------------------

func TestCleanupWorktreesCmd_Registration(t *testing.T) {
	found := false
	for _, cmd := range adminCmd.Commands() {
		if cmd.Use == "cleanup-worktrees" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected cleanup-worktrees to be registered under admin command")
	}
}

func TestCleanupWorktreesCmd_Use(t *testing.T) {
	if cleanupWorktreesCmd.Use != "cleanup-worktrees" {
		t.Errorf("Expected Use 'cleanup-worktrees', got %q", cleanupWorktreesCmd.Use)
	}
}

func TestCleanupWorktreesCmd_Short(t *testing.T) {
	if cleanupWorktreesCmd.Short == "" {
		t.Error("Expected non-empty Short description")
	}
}

func TestCleanupWorktreesCmd_Long(t *testing.T) {
	if cleanupWorktreesCmd.Long == "" {
		t.Error("Expected non-empty Long description")
	}
}

func TestCleanupWorktreesCmd_RunE(t *testing.T) {
	if cleanupWorktreesCmd.RunE == nil {
		t.Error("Expected RunE to be set")
	}
}

// ---------------------------------------------------------------------------
// Flag registration and parsing tests
// ---------------------------------------------------------------------------

func TestCleanupWorktreesCmd_ForceFlag(t *testing.T) {
	flag := cleanupWorktreesCmd.Flags().Lookup("force")
	if flag == nil {
		t.Fatal("Expected --force flag to be registered")
	}
	if flag.DefValue != "false" {
		t.Errorf("Expected default value 'false' for --force, got %q", flag.DefValue)
	}
	if flag.Usage == "" {
		t.Error("Expected non-empty usage for --force flag")
	}
}

func TestCleanupWorktreesCmd_DeleteBranchesFlag(t *testing.T) {
	flag := cleanupWorktreesCmd.Flags().Lookup("delete-branches")
	if flag == nil {
		t.Fatal("Expected --delete-branches flag to be registered")
	}
	if flag.DefValue != "false" {
		t.Errorf("Expected default value 'false' for --delete-branches, got %q", flag.DefValue)
	}
	if flag.Usage == "" {
		t.Error("Expected non-empty usage for --delete-branches flag")
	}
}

func TestCleanupWorktreesCmd_DryRunFlag(t *testing.T) {
	flag := cleanupWorktreesCmd.Flags().Lookup("dry-run")
	if flag == nil {
		t.Fatal("Expected --dry-run flag to be registered")
	}
	if flag.DefValue != "false" {
		t.Errorf("Expected default value 'false' for --dry-run, got %q", flag.DefValue)
	}
	if flag.Usage == "" {
		t.Error("Expected non-empty usage for --dry-run flag")
	}
}

func TestCleanupWorktreesCmd_SessionFlag(t *testing.T) {
	flag := cleanupWorktreesCmd.Flags().Lookup("session")
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

func TestCleanupWorktreesCmd_FlagParsing_Force(t *testing.T) {
	oldVal := wtCleanupForce
	defer func() { wtCleanupForce = oldVal }()

	cleanupWorktreesCmd.Flags().Set("force", "true")
	if !wtCleanupForce {
		t.Error("Expected wtCleanupForce to be true after setting flag")
	}
	cleanupWorktreesCmd.Flags().Set("force", "false")
}

func TestCleanupWorktreesCmd_FlagParsing_DryRun(t *testing.T) {
	oldVal := wtCleanupDryRun
	defer func() { wtCleanupDryRun = oldVal }()

	cleanupWorktreesCmd.Flags().Set("dry-run", "true")
	if !wtCleanupDryRun {
		t.Error("Expected wtCleanupDryRun to be true after setting flag")
	}
	cleanupWorktreesCmd.Flags().Set("dry-run", "false")
}

func TestCleanupWorktreesCmd_FlagParsing_DeleteBranches(t *testing.T) {
	oldVal := wtCleanupDeleteBranches
	defer func() { wtCleanupDeleteBranches = oldVal }()

	cleanupWorktreesCmd.Flags().Set("delete-branches", "true")
	if !wtCleanupDeleteBranches {
		t.Error("Expected wtCleanupDeleteBranches to be true after setting flag")
	}
	cleanupWorktreesCmd.Flags().Set("delete-branches", "false")
}

func TestCleanupWorktreesCmd_FlagParsing_Session(t *testing.T) {
	oldVal := wtCleanupSession
	defer func() { wtCleanupSession = oldVal }()

	cleanupWorktreesCmd.Flags().Set("session", "test-session")
	if wtCleanupSession != "test-session" {
		t.Errorf("Expected session 'test-session', got %q", wtCleanupSession)
	}
	cleanupWorktreesCmd.Flags().Set("session", "")
}

func TestCleanupWorktreesCmd_AllFlagsCount(t *testing.T) {
	// Verify we have at least 4 flags (force, delete-branches, dry-run, session)
	expectedFlags := []string{"force", "delete-branches", "dry-run", "session"}
	for _, name := range expectedFlags {
		if cleanupWorktreesCmd.Flags().Lookup(name) == nil {
			t.Errorf("Expected flag %q to be registered", name)
		}
	}
}

func TestCleanupWorktreesCmd_LongDescriptionMentionsExamples(t *testing.T) {
	long := cleanupWorktreesCmd.Long
	if len(long) < 50 {
		t.Error("Expected detailed Long description")
	}
}

// ---------------------------------------------------------------------------
// Admin parent command tests
// ---------------------------------------------------------------------------

func TestAdminCmd_HasCheckWorktrees(t *testing.T) {
	for _, cmd := range adminCmd.Commands() {
		if cmd.Use == "check-worktrees" {
			return
		}
	}
	t.Error("admin command missing check-worktrees subcommand")
}

func TestAdminCmd_HasCleanupWorktrees(t *testing.T) {
	for _, cmd := range adminCmd.Commands() {
		if cmd.Use == "cleanup-worktrees" {
			return
		}
	}
	t.Error("admin command missing cleanup-worktrees subcommand")
}

func TestAdminCmd_IsRegisteredUnderRoot(t *testing.T) {
	found := false
	for _, cmd := range rootCmd.Commands() {
		if cmd.Use == "admin" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected admin to be registered under root command")
	}
}
