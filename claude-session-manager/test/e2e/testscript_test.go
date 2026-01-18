package e2e

import (
	"fmt"
	"os"
	"os/exec"
	"testing"

	"github.com/rogpeppe/go-internal/testscript"
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

	// Find the csm binary - should be in $HOME/go/bin/csm
	csmPath := os.Getenv("HOME") + "/go/bin/csm"

	// If not found in home, try building it
	if _, err := os.Stat(csmPath); os.IsNotExist(err) {
		// Fall back to building in-place
		buildCmd := exec.Command("go", "build", "-o", "/tmp/csm-test", "../../cmd/csm")
		if err := buildCmd.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to build csm: %v\n", err)
			return 1
		}
		csmPath = "/tmp/csm-test"
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
	testscript.Run(t, testscript.Params{
		Dir: "testdata",
		Setup: func(env *testscript.Env) error {
			// Set up test environment
			// This runs before each test script

			// Set CSM environment variables for testing
			env.Setenv("CSM_TMUX_SOCKET", env.Getenv("WORK")+"/test-tmux.sock")
			env.Setenv("HOME", env.Getenv("WORK")+"/home")

			// Create necessary directories
			homeDir := env.Getenv("HOME")
			if err := os.MkdirAll(homeDir+"/.claude", 0755); err != nil {
				return err
			}
			if err := os.MkdirAll(homeDir+"/.csm", 0755); err != nil {
				return err
			}
			if err := os.MkdirAll(homeDir+"/sessions", 0755); err != nil {
				return err
			}

			return nil
		},
	})
}
