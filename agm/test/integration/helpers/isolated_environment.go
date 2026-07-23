//go:build integration

// Package helpers provides isolated, test-owned AGM integration runtimes.
package helpers

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"unicode"

	"github.com/vbonnet/dear-agent/agm/internal/testcontext"
)

// IsolatedEnvironment owns every mutable dependency used by a real AGM
// integration test. Commands run a source-built AGM binary with an explicit
// environment, and tmux cleanup is limited to the environment's unique socket
// and exactly registered session names.
type IsolatedEnvironment struct {
	Context       *testcontext.TestContext
	SourceRoot    string
	HomeDir       string
	StateDir      string
	SessionsDir   string
	DBPath        string
	TmuxSocket    string
	SessionPrefix string
	WorkDir       string
	BinDir        string
	AGMBinary     string

	mu          sync.Mutex
	owned       map[string]struct{}
	cleanupOnce sync.Once
	cleanupErr  error
}

// NewIsolatedEnvironment allocates an isolated runtime and builds the AGM
// binary from the current checkout. It never resolves an installed AGM from
// PATH.
func NewIsolatedEnvironment(t testing.TB) *IsolatedEnvironment {
	t.Helper()

	tc := testcontext.New()
	if err := tc.EnsureDirs(); err != nil {
		t.Fatalf("create isolated AGM paths: %v", err)
	}
	sourceRoot, err := findSourceRoot()
	if err != nil {
		_ = tc.Cleanup()
		t.Fatalf("find dear-agent source root: %v", err)
	}

	env := &IsolatedEnvironment{
		Context:       tc,
		SourceRoot:    sourceRoot,
		HomeDir:       tc.HomeDir,
		StateDir:      tc.StateDir,
		SessionsDir:   tc.SessionsDir,
		DBPath:        tc.DBPath,
		TmuxSocket:    tc.SocketPath,
		SessionPrefix: "agm-it-" + tc.RunID + "-",
		WorkDir:       filepath.Join(tc.BaseDir, "work"),
		BinDir:        filepath.Join(tc.BaseDir, "bin"),
		owned:         make(map[string]struct{}),
	}
	env.AGMBinary = filepath.Join(env.BinDir, "agm")

	for _, dir := range []string{
		env.WorkDir, env.BinDir, filepath.Join(tc.BaseDir, "config"),
		filepath.Join(tc.BaseDir, "cache"), filepath.Join(tc.BaseDir, "data"),
	} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			_ = tc.Cleanup()
			t.Fatalf("create isolated directory %s: %v", dir, err)
		}
	}

	build := exec.Command("go", "build", "-o", env.AGMBinary, "./agm/cmd/agm")
	build.Dir = sourceRoot
	build.Env = os.Environ()
	if output, err := build.CombinedOutput(); err != nil {
		_ = tc.Cleanup()
		t.Fatalf("build source AGM binary: %v\n%s", err, output)
	}

	t.Cleanup(func() {
		if err := env.Cleanup(); err != nil {
			t.Errorf("clean isolated AGM environment: %v", err)
		}
	})
	return env
}

// Environ returns an explicit, credential-free process environment. Tests may
// append synthetic credentials to a command after construction when required.
func (e *IsolatedEnvironment) Environ() []string {
	values := map[string]string{
		"HOME":                  e.HomeDir,
		"PWD":                   e.WorkDir,
		"PATH":                  e.BinDir + string(os.PathListSeparator) + os.Getenv("PATH"),
		"SHELL":                 "/bin/sh",
		"TMPDIR":                e.Context.BaseDir,
		"XDG_CONFIG_HOME":       filepath.Join(e.Context.BaseDir, "config"),
		"XDG_CACHE_HOME":        filepath.Join(e.Context.BaseDir, "cache"),
		"XDG_DATA_HOME":         filepath.Join(e.Context.BaseDir, "data"),
		"AGM_HOME":              filepath.Join(e.Context.BaseDir, "agm-home"),
		"AGM_CONFIG_DIR":        filepath.Join(e.Context.BaseDir, "config", "agm"),
		"AGM_STATE_DIR":         e.StateDir,
		"WORKSPACE":             "test",
		"ENGRAM_TEST_MODE":      "1",
		"ENGRAM_TEST_WORKSPACE": "test",
		"NO_COLOR":              "1",
	}
	for _, name := range []string{"USER", "LOGNAME", "TERM", "LANG", "LC_ALL", "LC_CTYPE"} {
		if value, ok := os.LookupEnv(name); ok {
			values[name] = value
		}
	}
	for _, entry := range e.Context.Environ() {
		name, value, ok := strings.Cut(entry, "=")
		if ok {
			values[name] = value
		}
	}

	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	sort.Strings(names)
	environ := make([]string, 0, len(names))
	for _, name := range names {
		environ = append(environ, name+"="+values[name])
	}
	return environ
}

// Command constructs a source-built AGM subprocess rooted in the isolated
// work directory.
func (e *IsolatedEnvironment) Command(args ...string) *exec.Cmd {
	cmd := exec.Command(e.AGMBinary, args...)
	cmd.Dir = e.WorkDir
	cmd.Env = e.Environ()
	return cmd
}

// TmuxCommand constructs a command against only this environment's socket.
func (e *IsolatedEnvironment) TmuxCommand(args ...string) *exec.Cmd {
	fullArgs := append([]string{"-S", e.TmuxSocket}, args...)
	// #nosec G702 -- fixed tmux executable and argv avoid shell interpolation.
	cmd := exec.Command("tmux", fullArgs...)
	cmd.Env = e.Environ()
	return cmd
}

// StartTmuxServer starts this environment's owned server with a sentinel
// session and pins a non-login shell plus the isolated PATH. This prevents host
// shell startup files from replacing fake harness binaries during tests.
func (e *IsolatedEnvironment) StartTmuxServer(sentinel string) error {
	if err := e.RegisterSession(sentinel); err != nil {
		return err
	}
	if output, err := e.TmuxCommand("new-session", "-d", "-s", sentinel, "sleep 300").CombinedOutput(); err != nil {
		return fmt.Errorf("start isolated tmux server: %w: %s", err, output)
	}
	settings := [][]string{
		{"set-option", "-g", "default-shell", "/bin/sh"},
		{"set-option", "-g", "default-command", "/bin/sh"},
		{"set-environment", "-g", "PATH", e.BinDir + string(os.PathListSeparator) + os.Getenv("PATH")},
	}
	for _, args := range settings {
		if output, err := e.TmuxCommand(args...).CombinedOutput(); err != nil {
			return fmt.Errorf("configure isolated tmux server: %w: %s", err, output)
		}
	}
	return nil
}

// SessionName returns an owned, run-specific tmux session name.
func (e *IsolatedEnvironment) SessionName(suffix string) string {
	clean := strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' {
			return r
		}
		return '-'
	}, suffix)
	clean = strings.Trim(clean, "-_")
	if clean == "" {
		clean = "session"
	}
	return e.SessionPrefix + clean
}

// RegisterSession records one exact owned target for cleanup.
func (e *IsolatedEnvironment) RegisterSession(name string) error {
	if !strings.HasPrefix(name, e.SessionPrefix) {
		return fmt.Errorf("session %q is outside owned prefix %q", name, e.SessionPrefix)
	}
	e.mu.Lock()
	e.owned[name] = struct{}{}
	e.mu.Unlock()
	return nil
}

// WriteExecutable installs a test-owned fake harness at the front of PATH.
func (e *IsolatedEnvironment) WriteExecutable(name, script string) error {
	if name == "" || filepath.Base(name) != name {
		return fmt.Errorf("invalid executable name %q", name)
	}
	// #nosec G306 -- the owner-only test harness must be executable.
	return os.WriteFile(filepath.Join(e.BinDir, name), []byte(script), 0700)
}

// BuildGoExecutable compiles a test-owned process whose executable basename is
// name. Use this instead of a shell script when production liveness checks must
// observe the harness process name through tmux or the OS process table.
func (e *IsolatedEnvironment) BuildGoExecutable(name, source string) error {
	if name == "" || filepath.Base(name) != name {
		return fmt.Errorf("invalid executable name %q", name)
	}
	sourceDir := filepath.Join(e.Context.BaseDir, "fake-"+name)
	if err := os.MkdirAll(sourceDir, 0700); err != nil {
		return fmt.Errorf("create fake executable source directory: %w", err)
	}
	sourcePath := filepath.Join(sourceDir, "main.go")
	if err := os.WriteFile(sourcePath, []byte(source), 0600); err != nil {
		return fmt.Errorf("write fake executable source: %w", err)
	}
	command := exec.Command("go", "build", "-o", filepath.Join(e.BinDir, name), sourcePath)
	command.Dir = e.SourceRoot
	command.Env = os.Environ()
	if output, err := command.CombinedOutput(); err != nil {
		return fmt.Errorf("build fake executable %q: %w: %s", name, err, output)
	}
	return nil
}

// HasSession checks this environment's socket only.
func (e *IsolatedEnvironment) HasSession(name string) bool {
	return e.TmuxCommand("has-session", "-t", name).Run() == nil
}

// CapturePane returns the visible pane contents from this environment's socket.
func (e *IsolatedEnvironment) CapturePane(name string) (string, error) {
	output, err := e.TmuxCommand("capture-pane", "-p", "-t", name).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("capture isolated pane %q: %w: %s", name, err, output)
	}
	return string(output), nil
}

// Cleanup removes only exact registered sessions, this environment's owned
// tmux server, and this environment's paths. It is safe to call repeatedly.
func (e *IsolatedEnvironment) Cleanup() error {
	e.cleanupOnce.Do(func() {
		e.mu.Lock()
		names := make([]string, 0, len(e.owned))
		for name := range e.owned {
			names = append(names, name)
		}
		e.mu.Unlock()
		sort.Strings(names)

		for _, name := range names {
			if output, err := e.TmuxCommand("kill-session", "-t", name).CombinedOutput(); err != nil && !missingTmuxTarget(output) {
				e.cleanupErr = errors.Join(e.cleanupErr, fmt.Errorf("kill owned tmux session %q: %w: %s", name, err, output))
			}
		}
		if output, err := e.TmuxCommand("kill-server").CombinedOutput(); err != nil && !missingTmuxTarget(output) {
			e.cleanupErr = errors.Join(e.cleanupErr, fmt.Errorf("kill owned tmux server: %w: %s", err, output))
		}
		e.cleanupErr = errors.Join(e.cleanupErr, e.Context.Cleanup())
	})
	return e.cleanupErr
}

func missingTmuxTarget(output []byte) bool {
	message := strings.ToLower(string(output))
	return strings.Contains(message, "no server running") ||
		strings.Contains(message, "no such file or directory") ||
		strings.Contains(message, "can't find session")
}

func findSourceRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		goMod := filepath.Join(dir, "go.mod")
		if data, readErr := os.ReadFile(goMod); readErr == nil && strings.Contains(string(data), "github.com/vbonnet/dear-agent") {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("could not find dear-agent go.mod")
		}
		dir = parent
	}
}
