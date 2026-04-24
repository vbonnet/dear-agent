package main

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/config"
)

// TestHeaderPrinting tests that the header is printed to stderr for non-version commands
func TestHeaderPrinting(t *testing.T) {
	// Save and restore global cfg and stderr
	oldCfg := cfg
	oldStderr := os.Stderr
	defer func() {
		cfg = oldCfg
		os.Stderr = oldStderr
	}()

	// Create a temporary config
	cfg = &config.Config{
		SessionsDir: t.TempDir(),
		LogLevel:    "info",
	}

	// Capture stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("Failed to create pipe: %v", err)
	}
	os.Stderr = w

	// Create a test command that is not "version"
	testCmd := &cobra.Command{
		Use: "test",
		RunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
	}

	// Run the PersistentPreRunE hook
	err = rootCmd.PersistentPreRunE(testCmd, []string{})
	if err != nil {
		t.Fatalf("PersistentPreRunE failed: %v", err)
	}

	// Close the write end and read stderr
	w.Close()
	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	// Verify header was printed
	if !strings.Contains(output, "agm") {
		t.Errorf("Expected header to contain 'agm', got: %s", output)
	}
	if !strings.Contains(output, Version) {
		t.Errorf("Expected header to contain version '%s', got: %s", Version, output)
	}
	// Should contain a path (either real path or "unknown")
	if !strings.Contains(output, "/") && !strings.Contains(output, "unknown") {
		t.Errorf("Expected header to contain binary path, got: %s", output)
	}
}

// TestHeaderNotPrintedForVersionCommand tests that header is skipped for version command
func TestHeaderNotPrintedForVersionCommand(t *testing.T) {
	// Save and restore global cfg and stderr
	oldCfg := cfg
	oldStderr := os.Stderr
	defer func() {
		cfg = oldCfg
		os.Stderr = oldStderr
	}()

	// Create a temporary config
	cfg = &config.Config{
		SessionsDir: t.TempDir(),
		LogLevel:    "info",
	}

	// Capture stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("Failed to create pipe: %v", err)
	}
	os.Stderr = w

	// Use the actual version command
	err = rootCmd.PersistentPreRunE(versionCmd, []string{})
	if err != nil {
		t.Fatalf("PersistentPreRunE failed: %v", err)
	}

	// Close the write end and read stderr
	w.Close()
	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	// Verify header was NOT printed (stderr should be empty for version command)
	if output != "" {
		t.Errorf("Expected no header for version command, got: %s", output)
	}
}

// TestHeaderFormat tests the exact format of the header
func TestHeaderFormat(t *testing.T) {
	// Save and restore global cfg and stderr
	oldCfg := cfg
	oldStderr := os.Stderr
	defer func() {
		cfg = oldCfg
		os.Stderr = oldStderr
	}()

	// Create a temporary config
	cfg = &config.Config{
		SessionsDir: t.TempDir(),
		LogLevel:    "info",
	}

	// Capture stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("Failed to create pipe: %v", err)
	}
	os.Stderr = w

	// Create a test command
	testCmd := &cobra.Command{
		Use: "list",
		RunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
	}

	// Run the PersistentPreRunE hook
	err = rootCmd.PersistentPreRunE(testCmd, []string{})
	if err != nil {
		t.Fatalf("PersistentPreRunE failed: %v", err)
	}

	// Close the write end and read stderr
	w.Close()
	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := strings.TrimSpace(buf.String())

	// Verify format: "agm <version> (<path>)"
	if !strings.HasPrefix(output, "agm ") {
		t.Errorf("Expected header to start with 'agm ', got: %s", output)
	}
	if !strings.Contains(output, "(") || !strings.Contains(output, ")") {
		t.Errorf("Expected header to contain parentheses around path, got: %s", output)
	}
}
