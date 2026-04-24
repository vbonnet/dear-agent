package main

import (
	"testing"
	"time"
)

func TestSessionCompactCommandMetadata(t *testing.T) {
	if sessionCompactCmd.Use != "compact <identifier>" {
		t.Errorf("Use = %q, want %q", sessionCompactCmd.Use, "compact <identifier>")
	}
	if sessionCompactCmd.Short == "" {
		t.Error("Short description should not be empty")
	}
	if sessionCompactCmd.RunE == nil {
		t.Error("RunE should be set")
	}
	if sessionCompactCmd.Args == nil {
		t.Error("Args validator should be set")
	}
}

func TestSessionCompactFlags(t *testing.T) {
	tests := []struct {
		name     string
		flag     string
		defValue string
	}{
		{"compact-args", "compact-args", ""},
		{"monitor", "monitor", "true"},
		{"timeout", "timeout", "5m0s"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flag := sessionCompactCmd.Flags().Lookup(tt.flag)
			if flag == nil {
				t.Fatalf("--%s flag should be registered", tt.flag)
			}
			if flag.DefValue != tt.defValue {
				t.Errorf("--%s default = %q, want %q", tt.flag, flag.DefValue, tt.defValue)
			}
			if flag.Usage == "" {
				t.Errorf("--%s should have a usage description", tt.flag)
			}
		})
	}
}

func TestSessionCompactRegistered(t *testing.T) {
	found := false
	for _, cmd := range sessionCmd.Commands() {
		if cmd.Name() == "compact" {
			found = true
			break
		}
	}
	if !found {
		t.Error("compact should be registered as a subcommand of session")
	}
}

func TestSessionCompactTimeoutDefault(t *testing.T) {
	flag := sessionCompactCmd.Flags().Lookup("timeout")
	if flag == nil {
		t.Fatal("--timeout flag should be registered")
	}

	// Parse the default value to verify it's a valid duration
	d, err := time.ParseDuration(flag.DefValue)
	if err != nil {
		t.Fatalf("--timeout default should be a valid duration: %v", err)
	}
	if d != 5*time.Minute {
		t.Errorf("--timeout default = %v, want 5m", d)
	}
}
