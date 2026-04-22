package main

import "testing"

func TestBuildCompactCommand(t *testing.T) {
	tests := []struct {
		name     string
		args     string
		expected string
	}{
		{"no args", "", "/compact"},
		{"empty whitespace", "   ", "/compact"},
		{"with instructions", "preserve context about X", "/compact preserve context about X"},
		{"with leading whitespace", "  preserve auth context  ", "/compact preserve auth context"},
		{"single word", "everything", "/compact everything"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildCompactCommand(tt.args)
			if got != tt.expected {
				t.Errorf("buildCompactCommand(%q) = %q, want %q", tt.args, got, tt.expected)
			}
		})
	}
}

func TestSendCompactCommandMetadata(t *testing.T) {
	if sendCompactCmd.Use != "compact <session-name>" {
		t.Errorf("Use = %q, want %q", sendCompactCmd.Use, "compact <session-name>")
	}
	if sendCompactCmd.Short == "" {
		t.Error("Short description should not be empty")
	}
	if sendCompactCmd.RunE == nil {
		t.Error("RunE should be set")
	}
	if sendCompactCmd.Args == nil {
		t.Error("Args validator should be set")
	}
}

func TestSendCompactRegistered(t *testing.T) {
	found := false
	for _, cmd := range sendGroupCmd.Commands() {
		if cmd.Name() == "compact" {
			found = true
			break
		}
	}
	if !found {
		t.Error("compact should be registered as a subcommand of send")
	}
}

func TestSendCompactFlagRegistration(t *testing.T) {
	flags := []struct {
		name     string
		defValue string
	}{
		{"focus", ""},
		{"verify", "false"},
		{"dry-run", "false"},
	}

	for _, f := range flags {
		t.Run(f.name, func(t *testing.T) {
			flag := sendCompactCmd.Flags().Lookup(f.name)
			if flag == nil {
				t.Fatalf("--%s flag should be registered", f.name)
			}
			if flag.DefValue != f.defValue {
				t.Errorf("--%s default = %q, want %q", f.name, flag.DefValue, f.defValue)
			}
			if flag.Usage == "" {
				t.Errorf("--%s should have a usage description", f.name)
			}
		})
	}
}

func TestSendCompactOldArgsFlagRemoved(t *testing.T) {
	flag := sendCompactCmd.Flags().Lookup("args")
	if flag != nil {
		t.Error("--args flag should be removed (replaced by --focus)")
	}
}

func TestAgmBaseDir(t *testing.T) {
	t.Setenv("AGM_HOME", "/tmp/test-agm")
	dir := agmBaseDir()
	if dir != "/tmp/test-agm" {
		t.Errorf("agmBaseDir() = %q, want %q", dir, "/tmp/test-agm")
	}
}
