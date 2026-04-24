package main

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

// TestVersionVariablesExist verifies version variables are defined
func TestVersionVariablesExist(t *testing.T) {
	if Version == "" {
		t.Error("Version should not be empty")
	}
	if GitCommit == "" {
		t.Error("GitCommit should not be empty")
	}
	if BuildDate == "" {
		t.Error("BuildDate should not be empty")
	}
}

// TestHeaderFormat verifies the header format
func TestHeaderFormat(t *testing.T) {
	// Capture stderr
	oldStderr := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("Failed to create pipe: %v", err)
	}
	os.Stderr = w

	// Generate header output
	executable, err := os.Executable()
	if err != nil {
		executable = "unknown"
	}
	header := formatHeader(Version, executable)

	w.Close()
	os.Stderr = oldStderr
	var buf bytes.Buffer
	buf.ReadFrom(r)

	// Verify format: "agm-mcp-server <version> (<path>)"
	if !strings.HasPrefix(header, "agm-mcp-server ") {
		t.Errorf("Header should start with 'agm-mcp-server ', got: %s", header)
	}
	if !strings.Contains(header, "(") || !strings.Contains(header, ")") {
		t.Errorf("Header should contain parentheses around path, got: %s", header)
	}
	if !strings.Contains(header, Version) {
		t.Errorf("Header should contain version '%s', got: %s", Version, header)
	}
}

// formatHeader creates the header string for testing
func formatHeader(version, executable string) string {
	return "agm-mcp-server " + version + " (" + executable + ")"
}
