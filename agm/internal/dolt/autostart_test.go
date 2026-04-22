package dolt

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExpandTilde(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("cannot determine home directory")
	}

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"tilde path", "~/foo/bar", filepath.Join(home, "foo/bar")},
		{"absolute path", "/usr/bin/script", "/usr/bin/script"},
		{"relative path", "relative/path", "relative/path"},
		{"tilde only", "~", "~"},
		{"empty", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := expandTilde(tt.input)
			if got != tt.expected {
				t.Errorf("expandTilde(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestTryAutoStart_ScriptNotFound(t *testing.T) {
	err := tryAutoStart("/nonexistent/script.sh")
	if err == nil {
		t.Fatal("expected error for non-existent script")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error should mention 'not found': %v", err)
	}
}

func TestTryAutoStart_ScriptNotExecutable(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "script-*.sh")
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString("#!/bin/bash\nexit 0\n")
	f.Close()
	os.Chmod(f.Name(), 0644)

	err = tryAutoStart(f.Name())
	if err == nil {
		t.Fatal("expected error for non-executable script")
	}
	if !strings.Contains(err.Error(), "not executable") {
		t.Errorf("error should mention 'not executable': %v", err)
	}
}

func TestTryAutoStart_ScriptSucceeds(t *testing.T) {
	script := filepath.Join(t.TempDir(), "start.sh")
	os.WriteFile(script, []byte("#!/bin/bash\nexit 0\n"), 0755)

	err := tryAutoStart(script)
	if err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
}

func TestTryAutoStart_ScriptFails(t *testing.T) {
	script := filepath.Join(t.TempDir(), "fail.sh")
	os.WriteFile(script, []byte("#!/bin/bash\necho 'something went wrong'\nexit 1\n"), 0755)

	err := tryAutoStart(script)
	if err == nil {
		t.Fatal("expected error for failing script")
	}
	if !strings.Contains(err.Error(), "failed") {
		t.Errorf("error should mention 'failed': %v", err)
	}
}

func TestNewAdapter_NilConfig(t *testing.T) {
	_, err := New(nil)
	if err == nil {
		t.Fatal("expected error for nil config")
	}
	if !strings.Contains(err.Error(), "config cannot be nil") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestNewAdapter_EmptyWorkspace(t *testing.T) {
	_, err := New(&Config{Port: "3307"})
	if err == nil {
		t.Fatal("expected error for empty workspace")
	}
	if !strings.Contains(err.Error(), "workspace cannot be empty") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestNewAdapter_EmptyPort(t *testing.T) {
	_, err := New(&Config{Workspace: "test"})
	if err == nil {
		t.Fatal("expected error for empty port")
	}
	if !strings.Contains(err.Error(), "port cannot be empty") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestNewAdapter_ConnectionFailsNoScript(t *testing.T) {
	// Use a port nothing listens on
	_, err := New(&Config{
		Workspace: "test",
		Port:      "19999",
		Host:      "127.0.0.1",
		Database:  "test",
		User:      "root",
	})
	if err == nil {
		t.Fatal("expected error when server not running")
	}
	if !strings.Contains(err.Error(), "failed to connect to Dolt") {
		t.Errorf("error should mention connection failure: %v", err)
	}
	if !strings.Contains(err.Error(), "Hint:") {
		t.Errorf("error should include hint: %v", err)
	}
	if !strings.Contains(err.Error(), "19999") {
		t.Errorf("error should include port number: %v", err)
	}
}

func TestNewAdapter_ConnectionFailsWithScript(t *testing.T) {
	// Use a port nothing listens on, with a start script path (won't be invoked
	// because isRunningInTest() returns true)
	_, err := New(&Config{
		Workspace:   "test",
		Port:        "19999",
		Host:        "127.0.0.1",
		Database:    "test",
		User:        "root",
		StartScript: "/tmp/fake-start.sh",
	})
	if err == nil {
		t.Fatal("expected error when server not running")
	}
	// In test mode, auto-start is skipped, so we get the hint message
	if !strings.Contains(err.Error(), "Hint:") {
		t.Errorf("error should include hint: %v", err)
	}
}

func TestHintFormatting(t *testing.T) {
	// Test hint includes start script when configured
	port := "3307"
	script := "~/.local/bin/ensure-dolt-agm.sh"

	hint := fmt.Sprintf("Hint: Ensure Dolt server is running on port %s", port)
	if script != "" {
		hint += fmt.Sprintf("\nStart with: %s", script)
	}

	if !strings.Contains(hint, port) {
		t.Error("hint should contain port")
	}
	if !strings.Contains(hint, script) {
		t.Error("hint should contain script path")
	}

	// Test hint without script
	hint2 := fmt.Sprintf("Hint: Ensure Dolt server is running on port %s", port)
	if strings.Contains(hint2, "Start with:") {
		t.Error("hint without script should not contain 'Start with:'")
	}
}

func TestDefaultConfig_StartScriptDetection(t *testing.T) {
	// Save original lookupEnv
	origLookupEnv := lookupEnv
	defer func() { lookupEnv = origLookupEnv }()

	// Create a temp script
	tmpDir := t.TempDir()
	scriptPath := filepath.Join(tmpDir, "ensure-dolt-agm.sh")
	os.WriteFile(scriptPath, []byte("#!/bin/bash\nexit 0\n"), 0755)

	// Mock environment with AGM_DOLT_START_SCRIPT pointing to our script
	lookupEnv = func(key string) (string, bool) {
		switch key {
		case "WORKSPACE":
			return "test", true
		case "AGM_DOLT_START_SCRIPT":
			return scriptPath, true
		case "ENGRAM_TEST_MODE":
			return "1", true
		case "ENGRAM_TEST_WORKSPACE":
			return "test", true
		default:
			return "", false
		}
	}

	cfg, err := DefaultConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.StartScript != scriptPath {
		t.Errorf("StartScript = %q, want %q", cfg.StartScript, scriptPath)
	}
}

func TestDefaultConfig_DefaultStartScriptPath(t *testing.T) {
	origLookupEnv := lookupEnv
	defer func() { lookupEnv = origLookupEnv }()

	// Mock environment without AGM_DOLT_START_SCRIPT
	lookupEnv = func(key string) (string, bool) {
		switch key {
		case "WORKSPACE":
			return "test", true
		case "ENGRAM_TEST_MODE":
			return "1", true
		case "ENGRAM_TEST_WORKSPACE":
			return "test", true
		default:
			return "", false
		}
	}

	cfg, err := DefaultConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// The default path is ~/.local/bin/ensure-dolt-agm.sh
	// It should be set if the file exists, empty otherwise
	home, _ := os.UserHomeDir()
	defaultPath := filepath.Join(home, ".local", "bin", "ensure-dolt-agm.sh")
	if _, statErr := os.Stat(defaultPath); statErr == nil {
		if cfg.StartScript != defaultPath {
			t.Errorf("StartScript should be %q when default exists, got %q", defaultPath, cfg.StartScript)
		}
	} else {
		if cfg.StartScript != "" {
			t.Errorf("StartScript should be empty when default doesn't exist, got %q", cfg.StartScript)
		}
	}
}
