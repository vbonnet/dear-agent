package main

import (
	"testing"
)

func TestSessionTagCmd_Metadata(t *testing.T) {
	if sessionTagCmd.Use != "tag <session> <tag>" {
		t.Errorf("Use = %q, want %q", sessionTagCmd.Use, "tag <session> <tag>")
	}
	if sessionTagCmd.Short == "" {
		t.Error("Short description should not be empty")
	}
	if sessionTagCmd.RunE == nil {
		t.Error("RunE should be set")
	}
}

func TestSessionTagCmd_Registered(t *testing.T) {
	found := false
	for _, cmd := range sessionCmd.Commands() {
		if cmd.Name() == "tag" {
			found = true
			break
		}
	}
	if !found {
		t.Error("'tag' should be registered as a subcommand of session")
	}
}

func TestSessionTagCmd_RemoveFlag(t *testing.T) {
	flag := sessionTagCmd.Flags().Lookup("remove")
	if flag == nil {
		t.Fatal("--remove flag should be registered")
	}
	if flag.DefValue != "" {
		t.Errorf("--remove default = %q, want %q", flag.DefValue, "")
	}
}

func TestSessionTagCmd_NoArgs(t *testing.T) {
	sessionTagCmd.SetArgs([]string{})
	err := sessionTagCmd.Execute()
	if err == nil {
		t.Error("expected error when no session specified")
	}
}

func TestSessionTagCmd_NoTagAndNoRemove(t *testing.T) {
	// Provide session name only without tag or --remove — should error
	// Reset flag state
	tagRemove = ""
	sessionTagCmd.SetArgs([]string{"some-session"})
	err := sessionTagCmd.Execute()
	if err == nil {
		t.Error("expected error when no tag and no --remove provided")
	}
}
