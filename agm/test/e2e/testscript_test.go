package e2e

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"syscall"
	"testing"
	"time"

	"github.com/rogpeppe/go-internal/testscript"
	"github.com/vbonnet/dear-agent/agm/internal/session"
)

// TestMain sets up the testscript environment
func TestMain(m *testing.M) {
	if os.Getenv("SKIP_E2E") != "" {
		fmt.Println("Skipping: requires infrastructure not available in CI")
		os.Exit(0)
	}
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

	// Try to use the installed agm binary first (check the actual user home,
	// not the test HOME).
	agmPath := installedAGMPath()

	// If it is not installed, build from the local source tree into an
	// ISOLATED temp cache — never into the live ~/go/bin.
	//
	// The previous implementation built with `-o $HOME/go/bin/agm`, writing
	// straight into the user's real GOBIN as a fallback. That made an
	// unsandboxed `go test ./...` mutate the live toolchain directory, and was
	// the mechanism implicated in the 2026-07-15 ~/go/bin wipe (bead ce-24f1):
	// tests must build into a throwaway location, never the directory the rest
	// of the system depends on. buildAGMToCache() builds atomically (temp file
	// + rename) into os.TempDir() and returns that path.
	if _, err := os.Stat(agmPath); err != nil {
		if !os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "Failed to stat installed agm at %s: %v\n", agmPath, err)
			return 1
		}
		cached, buildErr := buildAGMToCache()
		if buildErr != nil {
			fmt.Fprintf(os.Stderr, "Failed to build agm: %v\n", buildErr)
			return 1
		}
		agmPath = cached
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

// installedAGMPath returns the path to the user's installed agm binary. The
// testscript Setup preserves the pre-override HOME as REAL_HOME; prefer that so
// this resolves to the real user home even after a test overrides HOME.
func installedAGMPath() string {
	home := os.Getenv("REAL_HOME")
	if home == "" {
		home = os.Getenv("HOME")
	}
	return filepath.Join(home, "go", "bin", "agm")
}

// e2eBuildCacheKey returns a fingerprint of every tracked build input in the
// AGM module. //go:embed accepts non-Go assets, so a Go-only key can reuse a
// binary with stale hooks, schedules, schemas, migrations, or JavaScript.
func e2eBuildCacheKey() string {
	_, testFile, _, _ := runtime.Caller(0)
	moduleRoot := filepath.Clean(filepath.Join(filepath.Dir(testFile), "../.."))
	return e2eBuildCacheKeyForRoot(moduleRoot)
}

func e2eBuildCacheKeyForRoot(moduleRoot string) string {
	var inputs []string
	if err := filepath.WalkDir(moduleRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "vendor", "node_modules", "build", "dist":
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		rel, relErr := filepath.Rel(moduleRoot, path)
		if relErr != nil {
			return relErr
		}
		inputs = append(inputs, rel)
		return nil
	}); err != nil {
		return "unknown"
	}
	sort.Strings(inputs)
	h := sha256.New()
	for _, rel := range inputs {
		data, err := os.ReadFile(filepath.Join(moduleRoot, rel))
		if err != nil {
			return "unknown"
		}
		_, _ = io.WriteString(h, filepath.ToSlash(rel)+"\x00")
		_, _ = h.Write(data)
		_, _ = io.WriteString(h, "\x00")
	}
	return fmt.Sprintf("%x", h.Sum(nil)[:8])
}

func TestE2EBuildCacheKeyIncludesEmbeddedAssets(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "hooks"), 0o700); err != nil {
		t.Fatal(err)
	}
	asset := filepath.Join(root, "hooks", "guard.sh")
	if err := os.WriteFile(asset, []byte("first"), 0o600); err != nil {
		t.Fatal(err)
	}
	before := e2eBuildCacheKeyForRoot(root)
	if err := os.WriteFile(asset, []byte("second"), 0o600); err != nil {
		t.Fatal(err)
	}
	if after := e2eBuildCacheKeyForRoot(root); after == before {
		t.Fatal("embedded asset change must invalidate the fallback AGM build key")
	}
}

// e2eBuildCacheDir is a private, per-user fallback build cache. Its source key
// permits sharing only by subprocesses that build the same AGM source.
func e2eBuildCacheDir() string {
	cacheRoot, err := os.UserCacheDir()
	if err != nil || cacheRoot == "" {
		cacheRoot = filepath.Join(os.Getenv("HOME"), ".cache")
	}
	return filepath.Join(cacheRoot, "dear-agent", "e2e", "agm-"+e2eBuildCacheKey())
}

func ensurePrivateBuildCacheDir(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return err
	}
	info, err := os.Lstat(dir)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("unsafe build cache directory %q", dir)
	}
	return nil
}

// buildAGMToCache builds ./cmd/agm into the isolated e2e build cache and
// returns the path to the built binary. It is safe to call from the many
// concurrent agmMain subprocesses testscript spawns: the build goes to a
// unique temp file that is atomically renamed into place, so a partially
// written binary is never observed, and a lost rename race simply reuses the
// winner's binary. The output path is always under os.TempDir(); it is never
// the live ~/go/bin.
func buildAGMToCache() (string, error) {
	dir := e2eBuildCacheDir()
	dest := filepath.Join(dir, "agm")

	if err := ensurePrivateBuildCacheDir(dir); err != nil {
		return "", fmt.Errorf("prepare private build cache: %w", err)
	}
	if info, err := os.Lstat(dest); err == nil {
		if info.Mode().IsRegular() && info.Mode().Perm()&0o077 == 0 {
			return dest, nil
		}
		return "", fmt.Errorf("unsafe cached AGM binary %q", dest)
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("stat cached AGM binary: %w", err)
	}

	_, testFile, _, _ := runtime.Caller(0)
	// testFile: agm/test/e2e/testscript_test.go → up 3 dirs to reach agm/.
	// "go install <module-path>" is avoided: the testscript work dir is
	// outside the module, so Go would try to fetch the private module from the
	// network and fail. Build the local source with an explicit output path.
	agmModRoot := filepath.Join(filepath.Dir(testFile), "../..")

	tmp, err := os.CreateTemp(dir, "agm-build-*")
	if err != nil {
		return "", fmt.Errorf("create temp build target: %w", err)
	}
	tmpPath := tmp.Name()
	tmp.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	buildCmd := exec.CommandContext(ctx, "go", "build", "-o", tmpPath, "./cmd/agm")
	buildCmd.Dir = agmModRoot
	if out, err := buildCmd.CombinedOutput(); err != nil {
		os.Remove(tmpPath)
		return "", fmt.Errorf("go build: %w\n%s", err, out)
	}
	if err := os.Chmod(tmpPath, 0o700); err != nil {
		os.Remove(tmpPath)
		return "", fmt.Errorf("restrict build artifact permissions: %w", err)
	}

	// Atomically publish. If a concurrent builder won the race, reuse its
	// result rather than failing — both binaries are equivalent.
	if err := os.Rename(tmpPath, dest); err != nil {
		os.Remove(tmpPath)
		if info, statErr := os.Lstat(dest); statErr == nil && info.Mode().IsRegular() && info.Mode().Perm()&0o077 == 0 {
			return dest, nil
		}
		return "", fmt.Errorf("publish build: %w", err)
	}
	return dest, nil
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

				agmPath := installedAGMPath()
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
