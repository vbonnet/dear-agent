package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// ---------------------------------------------------------------------------
// Command registration
// ---------------------------------------------------------------------------

func TestSessionResourceCleanupCmd_Registration(t *testing.T) {
	found := false
	for _, cmd := range sessionCmd.Commands() {
		if cmd.Use == "cleanup [session-name]" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected 'cleanup' to be registered under session command")
	}
}

func TestSessionResourceCleanupCmd_Short(t *testing.T) {
	if sessionResourceCleanupCmd.Short == "" {
		t.Error("Expected non-empty Short description")
	}
}

func TestSessionResourceCleanupCmd_Long(t *testing.T) {
	if len(sessionResourceCleanupCmd.Long) < 50 {
		t.Error("Expected detailed Long description")
	}
}

func TestSessionResourceCleanupCmd_RunE(t *testing.T) {
	if sessionResourceCleanupCmd.RunE == nil {
		t.Error("Expected RunE to be set")
	}
}

// ---------------------------------------------------------------------------
// Flag registration
// ---------------------------------------------------------------------------

func TestSessionResourceCleanupCmd_DryRunFlag(t *testing.T) {
	flag := sessionResourceCleanupCmd.Flags().Lookup("dry-run")
	if flag == nil {
		t.Fatal("Expected --dry-run flag")
	}
	if flag.DefValue != "false" {
		t.Errorf("Expected default false, got %q", flag.DefValue)
	}
}

func TestSessionResourceCleanupCmd_ForceFlag(t *testing.T) {
	flag := sessionResourceCleanupCmd.Flags().Lookup("force")
	if flag == nil {
		t.Fatal("Expected --force flag")
	}
	if flag.DefValue != "false" {
		t.Errorf("Expected default false, got %q", flag.DefValue)
	}
}

func TestSessionResourceCleanupCmd_FlagParsing_DryRun(t *testing.T) {
	old := srcDryRun
	defer func() { srcDryRun = old }()

	if err := sessionResourceCleanupCmd.Flags().Set("dry-run", "true"); err != nil {
		t.Fatal(err)
	}
	if !srcDryRun {
		t.Error("Expected srcDryRun to be true after setting flag")
	}
	sessionResourceCleanupCmd.Flags().Set("dry-run", "false") //nolint:errcheck
}

func TestSessionResourceCleanupCmd_FlagParsing_Force(t *testing.T) {
	old := srcForce
	defer func() { srcForce = old }()

	if err := sessionResourceCleanupCmd.Flags().Set("force", "true"); err != nil {
		t.Fatal(err)
	}
	if !srcForce {
		t.Error("Expected srcForce to be true after setting flag")
	}
	sessionResourceCleanupCmd.Flags().Set("force", "false") //nolint:errcheck
}

// ---------------------------------------------------------------------------
// expandHome helper
// ---------------------------------------------------------------------------

func TestExpandHome_TildePrefix(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("cannot determine home dir")
	}
	got := expandHome("~/foo/bar")
	want := filepath.Join(home, "foo", "bar")
	if got != want {
		t.Errorf("expandHome(~/foo/bar) = %q, want %q", got, want)
	}
}

func TestExpandHome_NoTilde(t *testing.T) {
	input := "/absolute/path"
	got := expandHome(input)
	if got != input {
		t.Errorf("expandHome(%q) = %q, want %q", input, got, input)
	}
}

func TestExpandHome_EmptyString(t *testing.T) {
	got := expandHome("")
	if got != "" {
		t.Errorf("expandHome('') = %q, want ''", got)
	}
}

// ---------------------------------------------------------------------------
// isBranchNotFound helper
// ---------------------------------------------------------------------------

func TestIsBranchNotFound_NilError(t *testing.T) {
	if isBranchNotFound(nil) {
		t.Error("Expected false for nil error")
	}
}

func TestIsBranchNotFound_NotFoundError(t *testing.T) {
	err := fmt.Errorf("branch not found")
	if !isBranchNotFound(err) {
		t.Error("Expected true for 'not found' error")
	}
}

func TestIsBranchNotFound_DidNotMatchError(t *testing.T) {
	err := fmt.Errorf("error: branch 'foo' did not match any branch known to git")
	if !isBranchNotFound(err) {
		t.Error("Expected true for 'did not match' error")
	}
}

func TestIsBranchNotFound_OtherError(t *testing.T) {
	err := fmt.Errorf("permission denied")
	if isBranchNotFound(err) {
		t.Error("Expected false for unrelated error")
	}
}
