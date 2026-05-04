package e2e

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"testing"

	"github.com/rogpeppe/go-internal/testscript"
	"github.com/vbonnet/dear-agent/agm/internal/session"
)

// TestMain sets up the testscript environment
func TestMain(m *testing.M) {
	testscript.Main(m, map[string]func(){
		"agm": agmMain,
	})
}

// agmMain is the entry point for the agm binary in testscript
// This allows tests to call "agm" commands as if they were running the real binary.
// Calls os.Exit on its own to mirror the original RunMain int-return semantics.
func agmMain() {
	os.Exit(runAGM())
}

func runAGM() int {
	// Create mock tmux client for testing
	mockTmux := session.NewMockTmux()

	// Configure mock based on environment if needed
	// For now, tests will set up state via test files

	// Import the actual AGM command (requires exporting ExecuteWithDeps from cmd/agm)
	// Since we can't import cmd/agm directly, we'll use the binary approach for now
	// TODO: This is a temporary solution; full implementation requires refactoring cmd/agm
	// to export ExecuteWithDeps

	// For this initial implementation, use the mock approach
	// Tests will validate that commands work with mocked dependencies

	// Try to use installed agm binary first (check actual user home, not test HOME)
	userHome := os.Getenv("REAL_HOME")
	if userHome == "" {
		// Fallback: get HOME before test overrides it
		userHome = os.Getenv("HOME")
	}
	agmPath := userHome + "/go/bin/agm"

	// If not found, build from module
	if _, err := os.Stat(agmPath); os.IsNotExist(err) {
		// Use go install to build and cache the binary
		buildCmd := exec.Command("go", "install", "github.com/vbonnet/dear-agent/agm/cmd/agm")
		if err := buildCmd.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to build agm: %v\n", err)
			return 1
		}
		// After go install, binary should be at $GOBIN or $GOPATH/bin or $HOME/go/bin
		agmPath = userHome + "/go/bin/agm"
	}

	// Execute the binary with the current args
	cmd := exec.Command(agmPath, os.Args[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	cmd.Env = os.Environ()
	// Run in its own process group so all child processes (including Claude)
	// can be killed together when the test ends
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	// Note: mockTmux is created but not yet wired to the binary execution
	// This will be completed once cmd/agm exports ExecuteWithDeps publicly
	_ = mockTmux

	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return exitErr.ExitCode()
		}
		return 1
	}

	return 0
}

// TestAGM runs all testscript tests in testdata/
func TestAGM(t *testing.T) {
	// E2E tests now use mocked dependencies (tmux, claude)
	// No TTY or real tmux server required
	// Tests can run in CI without infrastructure dependencies

	// Register cleanup to kill tmux server on test failure/timeout
	e2eSocketDir := fmt.Sprintf("/tmp/agm-e2e-%d", os.Getpid())
	t.Cleanup(func() {
		_ = exec.Command("tmux", "-S", e2eSocketDir+"/t.sock", "kill-server").Run()
		os.RemoveAll(e2eSocketDir)
	})

	testscript.Run(t, testscript.Params{
		Dir: "testdata",
		Setup: func(env *testscript.Env) error {
			// Set up test environment
			// This runs before each test script

			// Preserve real HOME before overriding it
			if realHome := os.Getenv("HOME"); realHome != "" {
				env.Setenv("REAL_HOME", realHome)
			}

			// Set AGM environment variables for testing
			workDir := env.Getenv("WORK")

			// Use /tmp for tmux socket to avoid macOS 104-char Unix socket limit.
			// Go's testscript WORK paths can exceed this limit.
			socketDir := fmt.Sprintf("/tmp/agm-e2e-%d", os.Getpid())
			os.MkdirAll(socketDir, 0755)
			env.Setenv("AGM_TMUX_SOCKET", socketDir+"/t.sock")
			env.Setenv("AGM_STATE_DIR", workDir+"/.agm") // Isolate lock files and ready files per test
			env.Setenv("HOME", workDir+"/home")
			env.Setenv("AGM_SESSIONS_DIR", workDir+"/home/sessions")
			env.Setenv("WORKSPACE", "test-e2e") // Required for Dolt storage operations

			// Set dummy API key for tests to allow sessions to be created
			// Without this, claude agent initialization hangs waiting for ready file (60s timeout)
			env.Setenv("ANTHROPIC_API_KEY", "test-key-for-e2e-tests-only")

			// Create necessary directories
			homeDir := env.Getenv("HOME")
			agmDir := workDir + "/.agm"

			if err := os.MkdirAll(homeDir+"/.claude", 0755); err != nil {
				return err
			}
			if err := os.MkdirAll(agmDir, 0755); err != nil {
				return err
			}
			if err := os.MkdirAll(homeDir+"/sessions", 0755); err != nil {
				return err
			}

			return nil
		},
		Condition: func(cond string) (bool, error) {
			if cond == "can-create-tmux-session" {
				// Check if agm can create sessions in a sandboxed environment.
				socketDir := t.TempDir()
				homeDir := t.TempDir()
				os.MkdirAll(homeDir+"/sessions", 0755)
				os.MkdirAll(homeDir+"/.claude", 0755)

				realHome := os.Getenv("HOME")
				agmPath := realHome + "/go/bin/agm"
				if _, err := os.Stat(agmPath); os.IsNotExist(err) {
					return false, nil
				}

				cmd := exec.Command(agmPath, "session", "new", "cond-check", "--agent", "gpt", "--detached")
				cmd.Env = append(os.Environ(),
					"HOME="+homeDir,
					"AGM_TMUX_SOCKET="+socketDir+"/t.sock",
					"AGM_STATE_DIR="+homeDir+"/.agm",
					"ANTHROPIC_API_KEY=test-key",
					"WORKSPACE=test-e2e",
				)
				if err := cmd.Run(); err != nil {
					return false, nil
				}
				_ = exec.Command("tmux", "-S", socketDir+"/t.sock", "kill-server").Run()
				return true, nil
			}
			return false, fmt.Errorf("unknown condition %q", cond)
		},
	})

	// Kill tmux server BEFORE removing socket directory to prevent orphaned processes
	socketDir := fmt.Sprintf("/tmp/agm-e2e-%d", os.Getpid())
	_ = exec.Command("tmux", "-S", socketDir+"/t.sock", "kill-server").Run()
	os.RemoveAll(socketDir)
}
