package lifecycle_test

import (
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
)

// TestHookTestSessionGuard_BlocksTestPattern verifies that the hook blocks
// test-* pattern sessions without --test flag and displays error message
func TestHookTestSessionGuard_BlocksTestPattern(t *testing.T) {
	// Set up environment to simulate Claude Code calling the hook
	hookPath := os.ExpandEnv("$HOME/.claude/hooks/agm-pretool-test-session-guard")

	// Verify hook exists
	if _, err := os.Stat(hookPath); os.IsNotExist(err) {
		t.Skipf("Hook not installed at %s (run 'agm admin install-hooks')", hookPath)
	}

	// Simulate hook invocation for: agm session new test-foo
	cmd := exec.Command(hookPath)
	cmd.Env = append(os.Environ(),
		"CLAUDE_TOOL_NAME=Bash",
		"CLAUDE_TOOL_INPUT=agm session new test-foo",
	)

	// Run hook
	output, err := cmd.CombinedOutput()

	// CRITICAL: Verify hook blocks (exit code 1)
	if err == nil {
		t.Fatal("Hook should have blocked test-* pattern (expected exit code 1, got 0)")
	}

	exitErr := &exec.ExitError{}
	if errors.As(err, &exitErr) {
		if exitErr.ExitCode() != 1 {
			t.Fatalf("Expected exit code 1, got %d", exitErr.ExitCode())
		}
	} else {
		t.Fatalf("Unexpected error type: %v", err)
	}

	// CRITICAL: Verify error message displays
	outputStr := string(output)
	expectedStrings := []string{
		"❌ Test Session Pattern Detected",
		"test-foo",
		"--test flag",
		"agm session new --test test-foo",
		"--allow-test-name",
	}

	for _, expected := range expectedStrings {
		if !strings.Contains(outputStr, expected) {
			t.Errorf("Error message missing expected string: %q\nFull output:\n%s", expected, outputStr)
		}
	}

	// Verify error is on stderr (useful message display)
	if len(outputStr) == 0 {
		t.Fatal("CRITICAL: Hook blocked but no error message displayed (regression detected)")
	}
}

// TestHookTestSessionGuard_AllowsWithTestFlag verifies hook allows --test flag
func TestHookTestSessionGuard_AllowsWithTestFlag(t *testing.T) {
	hookPath := os.ExpandEnv("$HOME/.claude/hooks/agm-pretool-test-session-guard")

	if _, err := os.Stat(hookPath); os.IsNotExist(err) {
		t.Skipf("Hook not installed at %s", hookPath)
	}

	// Simulate: agm session new --test test-foo
	cmd := exec.Command(hookPath)
	cmd.Env = append(os.Environ(),
		"CLAUDE_TOOL_NAME=Bash",
		"CLAUDE_TOOL_INPUT=agm session new --test test-foo",
	)

	output, err := cmd.CombinedOutput()

	// Hook should allow (exit code 0)
	if err != nil {
		t.Fatalf("Hook should have allowed --test flag (exit code 0), got error: %v\nOutput: %s", err, string(output))
	}

	// No error message expected
	if len(output) > 0 {
		t.Errorf("Unexpected output when allowing --test flag: %s", string(output))
	}
}

// TestHookTestSessionGuard_AllowsWithOverrideFlag verifies --allow-test-name override
func TestHookTestSessionGuard_AllowsWithOverrideFlag(t *testing.T) {
	hookPath := os.ExpandEnv("$HOME/.claude/hooks/agm-pretool-test-session-guard")

	if _, err := os.Stat(hookPath); os.IsNotExist(err) {
		t.Skipf("Hook not installed at %s", hookPath)
	}

	// Simulate: agm session new test-foo --allow-test-name
	cmd := exec.Command(hookPath)
	cmd.Env = append(os.Environ(),
		"CLAUDE_TOOL_NAME=Bash",
		"CLAUDE_TOOL_INPUT=agm session new test-foo --allow-test-name",
	)

	output, err := cmd.CombinedOutput()

	// Hook should allow (exit code 0)
	if err != nil {
		t.Fatalf("Hook should have allowed --allow-test-name flag (exit code 0), got error: %v\nOutput: %s", err, string(output))
	}

	// No error message expected
	if len(output) > 0 {
		t.Errorf("Unexpected output when allowing --allow-test-name flag: %s", string(output))
	}
}

// TestHookTestSessionGuard_AllowsLegitimateNames verifies non-test-* names pass
func TestHookTestSessionGuard_AllowsLegitimateNames(t *testing.T) {
	hookPath := os.ExpandEnv("$HOME/.claude/hooks/agm-pretool-test-session-guard")

	if _, err := os.Stat(hookPath); os.IsNotExist(err) {
		t.Skipf("Hook not installed at %s", hookPath)
	}

	testCases := []struct {
		name         string
		commandInput string
	}{
		{
			name:         "legitimate name with 'test' substring",
			commandInput: "agm session new my-test-feature",
		},
		{
			name:         "legitimate name with 'testing' word",
			commandInput: "agm session new testing-auth-flow",
		},
		{
			name:         "normal session name",
			commandInput: "agm session new feature-work",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cmd := exec.Command(hookPath)
			cmd.Env = append(os.Environ(),
				"CLAUDE_TOOL_NAME=Bash",
				"CLAUDE_TOOL_INPUT="+tc.commandInput,
			)

			output, err := cmd.CombinedOutput()

			// Hook should allow (exit code 0)
			if err != nil {
				t.Fatalf("Hook should have allowed legitimate name (exit code 0), got error: %v\nOutput: %s", err, string(output))
			}

			// No error message expected
			if len(output) > 0 {
				t.Errorf("Unexpected output for legitimate name: %s", string(output))
			}
		})
	}
}

// TestHookTestSessionGuard_EdgeCases verifies edge case handling
func TestHookTestSessionGuard_EdgeCases(t *testing.T) {
	hookPath := os.ExpandEnv("$HOME/.claude/hooks/agm-pretool-test-session-guard")

	if _, err := os.Stat(hookPath); os.IsNotExist(err) {
		t.Skipf("Hook not installed at %s", hookPath)
	}

	testCases := []struct {
		name         string
		commandInput string
		shouldBlock  bool
		description  string
	}{
		{
			name:         "uppercase test pattern",
			commandInput: "agm session new TEST-FOO",
			shouldBlock:  true,
			description:  "TEST- (uppercase) should be blocked (case-insensitive)",
		},
		{
			name:         "mixed case test pattern",
			commandInput: "agm session new Test-Foo",
			shouldBlock:  true,
			description:  "Test- (mixed case) should be blocked (case-insensitive)",
		},
		{
			name:         "test without dash",
			commandInput: "agm session new test",
			shouldBlock:  false,
			description:  "'test' (no dash) should NOT be blocked (doesn't match test-* pattern)",
		},
		{
			name:         "test with trailing dash only",
			commandInput: "agm session new test-",
			shouldBlock:  true,
			description:  "'test-' should be blocked (matches test-* pattern)",
		},
		{
			name:         "non-bash tool",
			commandInput: "agm session new test-foo",
			shouldBlock:  false,
			description:  "Non-Bash tool should not be checked",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cmd := exec.Command(hookPath)

			toolName := "Bash"
			if tc.name == "non-bash tool" {
				toolName = "Read"
			}

			cmd.Env = append(os.Environ(),
				"CLAUDE_TOOL_NAME="+toolName,
				"CLAUDE_TOOL_INPUT="+tc.commandInput,
			)

			output, err := cmd.CombinedOutput()

			if tc.shouldBlock {
				// Expect exit code 1
				if err == nil {
					t.Fatalf("%s: Hook should have blocked (expected exit code 1, got 0)", tc.description)
				}

				exitErr := &exec.ExitError{}
				if errors.As(err, &exitErr) {
					if exitErr.ExitCode() != 1 {
						t.Fatalf("%s: Expected exit code 1, got %d", tc.description, exitErr.ExitCode())
					}
				}

				// Verify error message present
				if len(output) == 0 {
					t.Fatalf("%s: Hook blocked but no error message displayed", tc.description)
				}
			} else {
				// Expect exit code 0
				if err != nil {
					t.Fatalf("%s: Hook should have allowed (exit code 0), got error: %v\nOutput: %s", tc.description, err, string(output))
				}

				// No error message expected
				if len(output) > 0 && tc.name != "non-bash tool" {
					t.Errorf("%s: Unexpected output: %s", tc.description, string(output))
				}
			}
		})
	}
}

// TestHookTestSessionGuard_GracefulDegradation verifies hook doesn't break on errors
func TestHookTestSessionGuard_GracefulDegradation(t *testing.T) {
	hookPath := os.ExpandEnv("$HOME/.claude/hooks/agm-pretool-test-session-guard")

	if _, err := os.Stat(hookPath); os.IsNotExist(err) {
		t.Skipf("Hook not installed at %s", hookPath)
	}

	testCases := []struct {
		name        string
		envVars     []string
		description string
	}{
		{
			name: "missing CLAUDE_TOOL_NAME",
			envVars: []string{
				"CLAUDE_TOOL_INPUT=agm session new test-foo",
			},
			description: "Hook should allow when CLAUDE_TOOL_NAME missing",
		},
		{
			name: "missing CLAUDE_TOOL_INPUT",
			envVars: []string{
				"CLAUDE_TOOL_NAME=Bash",
			},
			description: "Hook should allow when CLAUDE_TOOL_INPUT missing",
		},
		{
			name:        "missing all env vars",
			envVars:     []string{},
			description: "Hook should allow when all env vars missing",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cmd := exec.Command(hookPath)
			cmd.Env = tc.envVars

			output, err := cmd.CombinedOutput()

			// Hook should allow (graceful degradation)
			if err != nil {
				t.Fatalf("%s: Hook should gracefully allow (exit code 0), got error: %v\nOutput: %s", tc.description, err, string(output))
			}
		})
	}
}
