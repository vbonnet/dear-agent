package cmd

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// TestHeaderPrinting tests that the header is printed to stderr for non-version commands
func TestHeaderPrinting(t *testing.T) {
	// Save and restore stderr
	oldStderr := os.Stderr
	defer func() {
		os.Stderr = oldStderr
	}()

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
	if !strings.Contains(output, "engram") {
		t.Errorf("Expected header to contain 'engram', got: %s", output)
	}
	if !strings.Contains(output, version) {
		t.Errorf("Expected header to contain version '%s', got: %s", version, output)
	}
	// Should contain a path (either real path or "unknown")
	if !strings.Contains(output, "/") && !strings.Contains(output, "unknown") {
		t.Errorf("Expected header to contain binary path, got: %s", output)
	}
}

// TestHeaderNotPrintedForVersionCommand tests that header is skipped for version command
func TestHeaderNotPrintedForVersionCommand(t *testing.T) {
	// Save and restore stderr
	oldStderr := os.Stderr
	defer func() {
		os.Stderr = oldStderr
	}()

	// Capture stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("Failed to create pipe: %v", err)
	}
	os.Stderr = w

	// Create a version command (name must be "version")
	versionCmd := &cobra.Command{
		Use: "version",
		RunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
	}

	// Run the PersistentPreRunE hook
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
	// Save and restore stderr
	oldStderr := os.Stderr
	defer func() {
		os.Stderr = oldStderr
	}()

	// Capture stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("Failed to create pipe: %v", err)
	}
	os.Stderr = w

	// Create a test command
	testCmd := &cobra.Command{
		Use: "doctor",
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

	// Verify format: "engram <version> (<path>)"
	if !strings.HasPrefix(output, "engram ") {
		t.Errorf("Expected header to start with 'engram ', got: %s", output)
	}
	if !strings.Contains(output, "(") || !strings.Contains(output, ")") {
		t.Errorf("Expected header to contain parentheses around path, got: %s", output)
	}
}
