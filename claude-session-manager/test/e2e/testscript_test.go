package e2e

import (
	"fmt"
	"os"
	"os/exec"
	"testing"

	"github.com/rogpeppe/go-internal/testscript"
	"golang.org/x/term"
)

// TestMain sets up the testscript environment
func TestMain(m *testing.M) {
	os.Exit(testscript.RunMain(m, map[string]func() int{
		"csm": csmMain,
	}))
}

// csmMain is the entry point for the csm binary in testscript
// This allows tests to call "csm" commands as if they were running the real binary
func csmMain() int {
	// Execute the real csm binary (built from cmd/csm)
	// This ensures tests run against actual implementation

	// Try to use installed csm binary first (check actual user home, not test HOME)
	userHome := os.Getenv("REAL_HOME")
	if userHome == "" {
		// Fallback: get HOME before test overrides it
		userHome = os.Getenv("HOME")
	}
	csmPath := userHome + "/go/bin/csm"

	// If not found, build from module
	if _, err := os.Stat(csmPath); os.IsNotExist(err) {
		// Use go install to build and cache the binary
		buildCmd := exec.Command("go", "install", "github.com/vbonnet/ai-tools/claude-session-manager/cmd/csm")
		if err := buildCmd.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to build csm: %v\n", err)
			return 1
		}
		// After go install, binary should be at $GOBIN or $GOPATH/bin or $HOME/go/bin
		csmPath = userHome + "/go/bin/csm"
	}

	// Execute the binary with the current args
	cmd := exec.Command(csmPath, os.Args[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	cmd.Env = os.Environ()

	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return exitErr.ExitCode()
		}
		return 1
	}

	return 0
}

// TestCSM runs all testscript tests in testdata/
func TestCSM(t *testing.T) {
	// Skip e2e tests when no TTY available
	// E2E tests require real tmux server + Claude CLI environment
	// Deferred to Phase 2 (proper test infrastructure setup)
	//
	// Note: CSM_STATE_DIR isolation is working (fixed lock contention bug)
	// but tests still need full tmux+Claude environment to complete
	if !term.IsTerminal(int(os.Stdout.Fd())) {
		t.Skip("Skipping e2e tests: no TTY available (requires real tmux server + Claude CLI)")
	}

	testscript.Run(t, testscript.Params{
		Dir: "testdata",
		Setup: func(env *testscript.Env) error {
			// Set up test environment
			// This runs before each test script

			// Preserve real HOME before overriding it
			if realHome := os.Getenv("HOME"); realHome != "" {
				env.Setenv("REAL_HOME", realHome)
			}

			// Set CSM environment variables for testing
			workDir := env.Getenv("WORK")
			env.Setenv("CSM_TMUX_SOCKET", workDir+"/test-tmux.sock")
			env.Setenv("CSM_STATE_DIR", workDir+"/.csm") // Isolate lock files and ready files per test
			env.Setenv("HOME", workDir+"/home")

			// Set dummy API key for tests to allow sessions to be created
			// Without this, claude agent initialization hangs waiting for ready file (60s timeout)
			env.Setenv("ANTHROPIC_API_KEY", "test-key-for-e2e-tests-only")

			// Create necessary directories
			homeDir := env.Getenv("HOME")
			csmDir := workDir + "/.csm"

			if err := os.MkdirAll(homeDir+"/.claude", 0755); err != nil {
				return err
			}
			if err := os.MkdirAll(csmDir, 0755); err != nil {
				return err
			}
			if err := os.MkdirAll(homeDir+"/sessions", 0755); err != nil {
				return err
			}

			return nil
		},
	})
}
