package unit_test

import (
	"fmt"
	"os"
	"os/exec"
	"testing"

	"github.com/rogpeppe/go-internal/testscript"
)

// TestMain sets up the testscript environment for unit tests
func TestMain(m *testing.M) {
	testscript.Main(m, map[string]func(){
		"agm": agmMain,
		"csm": agmMain, // csm is an alias for agm
	})
}

// agmMain is the entry point for the agm binary in testscript.
// This allows tests to call "agm" commands as if they were running the real binary.
func agmMain() {
	// Determine user's home directory for binary location
	userHome := os.Getenv("REAL_HOME")
	if userHome == "" {
		// Fallback: get HOME before test overrides it
		userHome = os.Getenv("HOME")
	}

	// Try using installed agm binary first
	agmPath := userHome + "/go/bin/agm"

	// If not found, build from module
	if _, err := os.Stat(agmPath); os.IsNotExist(err) {
		// Use go install to build and cache the binary
		buildCmd := exec.Command("go", "install", "github.com/vbonnet/ai-tools/claude-session-manager/cmd/agm")
		if err := buildCmd.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to build agm: %v\n", err)
			os.Exit(1)
		}
		agmPath = userHome + "/go/bin/agm"
	}

	// Execute the binary with the current args
	cmd := exec.Command(agmPath, os.Args[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	cmd.Env = os.Environ()

	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		os.Exit(1)
	}
}

// TestScripts runs all testscript tests from testdata/script/*.txtar.
//
// These tests verify CLI behavior using testscript DSL:
// - exec: Execute command (must succeed)
// - ! exec: Execute command (must fail)
// - stdout/stderr: Verify output patterns
// - exists: Verify file/directory exists
// - env: Set environment variables
//
// Tests run in isolated temp directories with:
// - $HOME set to test work directory
// - $AGM_TEST_MODE=true for test detection
// - Automatic cleanup after test
func TestScripts(t *testing.T) {
	testscript.Run(t, testscript.Params{
		Dir: "../../testdata/script",
		Setup: func(env *testscript.Env) error {
			// Preserve real HOME before overriding it
			if realHome := os.Getenv("HOME"); realHome != "" {
				env.Setenv("REAL_HOME", realHome)
			}

			// Set up isolated test environment
			workDir := env.Getenv("WORK")
			env.Setenv("HOME", workDir+"/home")
			env.Setenv("AGM_TMUX_SOCKET", workDir+"/test-tmux.sock")
			env.Setenv("AGM_STATE_DIR", workDir+"/.agm")
			env.Setenv("AGM_TEST_MODE", "true")

			// Set dummy API key for tests (prevents 60s timeout on agent initialization)
			env.Setenv("ANTHROPIC_API_KEY", "test-key-for-unit-tests-only")

			// Create necessary directories
			homeDir := env.Getenv("HOME")
			agmDir := workDir + "/.agm"

			if err := os.MkdirAll(homeDir+"/.claude", 0755); err != nil {
				return fmt.Errorf("failed to create .claude dir: %w", err)
			}
			if err := os.MkdirAll(agmDir, 0755); err != nil {
				return fmt.Errorf("failed to create .agm dir: %w", err)
			}
			if err := os.MkdirAll(homeDir+"/sessions", 0755); err != nil {
				return fmt.Errorf("failed to create sessions dir: %w", err)
			}
			if err := os.MkdirAll(homeDir+"/sessions-test", 0755); err != nil {
				return fmt.Errorf("failed to create sessions-test dir: %w", err)
			}

			return nil
		},
	})
}
