package e2e

import (
	"os"
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
// This allows tests to call "csm" commands
func csmMain() int {
	// Note: This would normally call the main() function from cmd/csm/main.go
	// For now, we return 0 to indicate success
	// In a full implementation, this should:
	// 1. Set up proper args from os.Args
	// 2. Call the real cobra command execution
	// 3. Return appropriate exit code
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
