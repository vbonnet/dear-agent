package cmd

import (
	"bytes"
	"io"
	"os"
	"regexp"
	"strings"
	"testing"
)

// captureOutput captures stdout during function execution
func captureOutput(f func()) string {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	f()

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	io.Copy(&buf, r)
	return buf.String()
}

// TestVersionCommand verifies the version command executes without error
func TestVersionCommand(t *testing.T) {
	output := captureOutput(func() {
		versionCmd.Run(versionCmd, []string{})
	})

	if output == "" {
		t.Error("Expected version output, got empty string")
	}

	if !strings.Contains(output, "engram version") {
		t.Errorf("Expected output to contain 'engram version', got: %s", output)
	}
}

// TestVersionOutputFormat validates the output matches the required format
func TestVersionOutputFormat(t *testing.T) {
	output := captureOutput(func() {
		versionCmd.Run(versionCmd, []string{})
	})

	// AC2: "engram version X.Y.Z (commit: <sha>, built: <date>)"
	pattern := `^engram version .+ \(commit: .+, built: .+\)\n$`
	matched, err := regexp.MatchString(pattern, output)
	if err != nil {
		t.Fatalf("Regex compilation error: %v", err)
	}

	if !matched {
		t.Errorf("Output doesn't match expected format.\nExpected pattern: %s\nGot: %s", pattern, output)
	}

	// Verify it contains the actual version variable values
	if !strings.Contains(output, version) {
		t.Errorf("Expected output to contain version '%s', got: %s", version, output)
	}

	if !strings.Contains(output, commit) {
		t.Errorf("Expected output to contain commit '%s', got: %s", commit, output)
	}

	if !strings.Contains(output, date) {
		t.Errorf("Expected output to contain date '%s', got: %s", date, output)
	}
}

// TestVersionHelp verifies help text is displayed correctly
func TestVersionHelp(t *testing.T) {
	// Test that help flag works
	if versionCmd.Short == "" {
		t.Error("Expected Short description to be set")
	}

	if !strings.Contains(versionCmd.Short, "version information") {
		t.Errorf("Expected Short description to contain 'version information', got: %s", versionCmd.Short)
	}

	if versionCmd.Long == "" {
		t.Error("Expected Long description to be set")
	}

	if !strings.Contains(versionCmd.Long, "Display engram version information") {
		t.Errorf("Expected Long description to contain explanation, got: %s", versionCmd.Long)
	}
}

// TestVersionNoArgs verifies command works without arguments
func TestVersionNoArgs(t *testing.T) {
	// Should work with no args
	output := captureOutput(func() {
		versionCmd.Run(versionCmd, []string{})
	})

	if output == "" {
		t.Error("Expected output when called with no args")
	}

	// Should also work with extra args (cobra ignores them)
	output2 := captureOutput(func() {
		versionCmd.Run(versionCmd, []string{"extra", "args"})
	})

	if output2 == "" {
		t.Error("Expected output when called with extra args")
	}

	// Both outputs should be identical
	if output != output2 {
		t.Error("Expected same output regardless of arguments")
	}
}
