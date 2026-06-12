package main

import (
	"strings"
	"testing"
)

func TestRun_Help(t *testing.T) {
	err := run([]string{"-h"})
	if err != nil {
		t.Errorf("run(-h) = %v, want nil", err)
	}
}

func TestRun_MissingCArg(t *testing.T) {
	err := run([]string{"-C"})
	if err == nil {
		t.Fatal("run(-C) expected error for missing directory, got nil")
	}
	if !strings.Contains(err.Error(), "-C") {
		t.Errorf("error %q should mention -C flag", err)
	}
}

func TestRun_MissingTimeoutArg(t *testing.T) {
	err := run([]string{"--timeout"})
	if err == nil {
		t.Fatal("run(--timeout) expected error for missing duration, got nil")
	}
	if !strings.Contains(err.Error(), "--timeout") {
		t.Errorf("error %q should mention --timeout flag", err)
	}
}

func TestRun_InvalidTimeout(t *testing.T) {
	err := run([]string{"--timeout", "notaduration"})
	if err == nil {
		t.Fatal("run(--timeout notaduration) expected error, got nil")
	}
	if !strings.Contains(err.Error(), "--timeout") {
		t.Errorf("error %q should mention --timeout flag", err)
	}
}

func TestUsageContainsBinaryName(t *testing.T) {
	if !strings.Contains(usage, "safe-push") {
		t.Error("usage string should contain binary name 'safe-push'")
	}
}
