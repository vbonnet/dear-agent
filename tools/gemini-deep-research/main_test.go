package main

import (
	"bytes"
	"flag"
	"os"
	"strings"
	"testing"
)

func TestMainHelp(t *testing.T) {
	// Save original args and restore after test
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()

	// Test --help flag
	os.Args = []string{"cmd", "--help"}

	// Reset flags for testing
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError)

	// Capture output
	var buf bytes.Buffer
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w
	defer func() { os.Stderr = oldStderr }()

	// This would normally call os.Exit(0), but we can't test that easily
	// So we'll just test that the usage function works correctly
	usageFunc()

	w.Close()
	buf.ReadFrom(r)
	output := buf.String()

	// Verify help text contains key elements
	if !strings.Contains(output, "gemini-deep-research") {
		t.Error("Help text should contain program name")
	}
	if !strings.Contains(output, "--analyze-prompt") {
		t.Error("Help text should contain --analyze-prompt flag")
	}
	if !strings.Contains(output, "--mode") {
		t.Error("Help text should contain --mode flag")
	}
	if !strings.Contains(output, "Examples:") {
		t.Error("Help text should contain examples section")
	}
}

func TestUsageText(t *testing.T) {
	// Capture usage output
	var buf bytes.Buffer
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	usageFunc()

	w.Close()
	os.Stderr = oldStderr
	buf.ReadFrom(r)
	output := buf.String()

	// Check for required sections
	requiredSections := []string{
		"Usage:",
		"Flags:",
		"Positional Arguments:",
		"Examples:",
		"Environment Variables:",
		"Exit Codes:",
	}

	for _, section := range requiredSections {
		if !strings.Contains(output, section) {
			t.Errorf("Usage text missing required section: %s", section)
		}
	}

	// Check for all flags
	requiredFlags := []string{
		"--analyze-prompt",
		"--mode",
		"--type",
		"--output-dir",
		"--timeout",
		"--project",
		"--help",
		"--version",
	}

	for _, flagName := range requiredFlags {
		if !strings.Contains(output, flagName) {
			t.Errorf("Usage text missing flag: %s", flagName)
		}
	}

	// Check for exit codes
	exitCodes := []string{"0", "1", "2", "3", "4", "5", "6"}
	for _, code := range exitCodes {
		if !strings.Contains(output, code) {
			t.Errorf("Usage text missing exit code: %s", code)
		}
	}
}

func TestVersionConstant(t *testing.T) {
	if version == "" {
		t.Error("Version constant should not be empty")
	}

	// Version should follow semantic versioning (basic check)
	if !strings.Contains(version, ".") {
		t.Error("Version should contain dots (semantic versioning)")
	}
}
