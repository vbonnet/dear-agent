// Package testcontext provides per-run test sandbox isolation for AGM.
//
// Each test run gets a unique ID and fully isolated paths:
//   - Tmux socket:   /tmp/agm-test-{id}.sock
//   - Sessions dir:  /tmp/agm-test-{id}/sessions/
//   - Home dir:      /tmp/agm-test-{id}/home/
//   - SQLite DB:     /tmp/agm-test-{id}/agm.db
//   - State dir:     /tmp/agm-test-{id}/state/
//   - Lock file:     /tmp/agm-test-{id}/agm.lock
//
// Environment variables are propagated to child commands so all AGM
// components use the isolated paths:
//
//	AGM_TEST_RUN_ID, AGM_TEST_ENV, AGM_TMUX_SOCKET, AGM_SESSIONS_DIR,
//	AGM_DB_PATH, AGM_STATE_DIR, AGM_LOCK_PATH
package testcontext

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/google/uuid"
)

const (
	testEnvironmentPrefix = "agm-test-"
	testEnvironmentRoot   = "/tmp"
	maxNamedEnvironment   = 64
)

// Environment variable names for test sandbox isolation.
const (
	EnvTestRunID   = "AGM_TEST_RUN_ID"
	EnvTestEnv     = "AGM_TEST_ENV"
	EnvTmuxSocket  = "AGM_TMUX_SOCKET"
	EnvSessionsDir = "AGM_SESSIONS_DIR"
	EnvDBPath      = "AGM_DB_PATH"
	EnvStateDir    = "AGM_STATE_DIR"
	EnvLockPath    = "AGM_LOCK_PATH"
)

// AuthMode controls how LLM credentials are forwarded into a test environment.
type AuthMode string

const (
	// AuthModeInherit symlinks credential files/dirs from the host HOME.
	AuthModeInherit AuthMode = "inherit"
	// AuthModeEnv relies on environment variables only (no file symlinks).
	AuthModeEnv AuthMode = "env"
	// AuthModeNone provides complete isolation with no auth forwarding.
	AuthModeNone AuthMode = "none"
)

// credentialPaths lists the relative paths (from HOME) that should be
// symlinked in AuthModeInherit. Only directories/files that exist on the
// host are symlinked; missing sources are silently skipped.
var credentialPaths = []string{
	".claude",
	".codex",
	filepath.Join(".config", "gcloud"),
	filepath.Join(".config", "opencode"),
}

// TestContext holds all isolated paths for a single test run.
type TestContext struct {
	RunID       string
	BaseDir     string
	HomeDir     string
	HostHome    string
	SocketPath  string
	SessionsDir string
	DBPath      string
	StateDir    string
	LockPath    string
}

// New creates a new TestContext with a unique run ID and isolated paths.
func New() *TestContext {
	id := uuid.New().String()[:8] // short UUID for readability
	return newWithID(id)
}

// NewNamed creates a TestContext with a validated user-chosen name instead of
// a random ID. Validation happens before any path is derived so a cleanup can
// never escape the test-environment root through a crafted name.
func NewNamed(name string) (*TestContext, error) {
	if err := ValidateName(name); err != nil {
		return nil, err
	}
	return newWithID(name), nil
}

// newWithID is the shared constructor for New and NewNamed.
func newWithID(id string) *TestContext {
	// Keep tmux socket paths below macOS's Unix-domain socket limit. os.TempDir
	// expands to a much longer /var/folders path there, while /tmp is stable on
	// every Unix platform on which AGM's tmux integration runs.
	baseDir := filepath.Join(testEnvironmentRoot, testEnvironmentPrefix+id)
	return &TestContext{
		RunID:       id,
		BaseDir:     baseDir,
		HomeDir:     filepath.Join(baseDir, "home"),
		SocketPath:  filepath.Join(testEnvironmentRoot, testEnvironmentPrefix+id+".sock"),
		SessionsDir: filepath.Join(baseDir, "sessions"),
		DBPath:      filepath.Join(baseDir, "agm.db"),
		StateDir:    filepath.Join(baseDir, "state"),
		LockPath:    filepath.Join(baseDir, "agm.lock"),
	}
}

// LoadNamed reconstructs a TestContext from a validated known name.
// It does not verify that the directory exists.
func LoadNamed(name string) (*TestContext, error) {
	return NewNamed(name)
}

// ValidateName rejects names that could escape the owned temporary root,
// inject terminal control characters, or exceed the short macOS socket budget.
func ValidateName(name string) error {
	switch {
	case name == "":
		return fmt.Errorf("test environment name must not be empty")
	case len(name) > maxNamedEnvironment:
		return fmt.Errorf("test environment name must not exceed %d bytes", maxNamedEnvironment)
	case filepath.IsAbs(name):
		return fmt.Errorf("test environment name must be relative")
	case strings.ContainsAny(name, `/\`):
		return fmt.Errorf("test environment name must not contain path separators")
	}
	for _, r := range name {
		if unicode.IsControl(r) {
			return fmt.Errorf("test environment name must not contain control characters")
		}
	}
	return nil
}

// ListNamed returns the centrally validated environments created beneath the
// same short root used by NewNamed, LoadNamed, and Cleanup.
func ListNamed() ([]*TestContext, error) {
	entries, err := os.ReadDir(testEnvironmentRoot)
	if err != nil {
		return nil, err
	}
	contexts := make([]*TestContext, 0)
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), testEnvironmentPrefix) {
			continue
		}
		name := strings.TrimPrefix(entry.Name(), testEnvironmentPrefix)
		if err := ValidateName(name); err != nil {
			continue
		}
		contexts = append(contexts, newWithID(name))
	}
	return contexts, nil
}

// FromEnv reconstructs a TestContext from environment variables.
// Returns (nil, false) if neither AGM_TEST_RUN_ID nor AGM_TEST_ENV is set.
func FromEnv() (*TestContext, bool) {
	runID := os.Getenv(EnvTestRunID)
	if runID == "" {
		runID = os.Getenv(EnvTestEnv)
	}
	if runID == "" {
		return nil, false
	}
	if err := ValidateName(runID); err != nil {
		return nil, false
	}
	sessionsDir := os.Getenv(EnvSessionsDir)
	baseDir := filepath.Dir(sessionsDir)
	if sessionsDir == "" {
		baseDir = filepath.Join(testEnvironmentRoot, testEnvironmentPrefix+runID)
	}
	stateDir := os.Getenv(EnvStateDir)
	if stateDir == "" {
		stateDir = filepath.Join(baseDir, "state")
	}
	return &TestContext{
		RunID:       runID,
		BaseDir:     baseDir,
		HomeDir:     filepath.Join(baseDir, "home"),
		SocketPath:  os.Getenv(EnvTmuxSocket),
		SessionsDir: sessionsDir,
		DBPath:      os.Getenv(EnvDBPath),
		StateDir:    stateDir,
		LockPath:    os.Getenv(EnvLockPath),
	}, true
}

// SetEnv sets all test sandbox environment variables in the current process,
// including HOME override and AGM_TEST_ENV marker.
func (tc *TestContext) SetEnv() error {
	vars := map[string]string{
		EnvTestRunID:   tc.RunID,
		EnvTestEnv:     tc.RunID,
		EnvTmuxSocket:  tc.SocketPath,
		EnvSessionsDir: tc.SessionsDir,
		EnvDBPath:      tc.DBPath,
		EnvStateDir:    tc.StateDir,
		EnvLockPath:    tc.LockPath,
		"HOME":         tc.HomeDir,
	}
	for k, v := range vars {
		if err := os.Setenv(k, v); err != nil {
			return fmt.Errorf("failed to set %s: %w", k, err)
		}
	}
	return nil
}

// UnsetEnv removes all test sandbox environment variables.
func (tc *TestContext) UnsetEnv() {
	for _, k := range []string{
		EnvTestRunID, EnvTestEnv, EnvTmuxSocket,
		EnvSessionsDir, EnvDBPath, EnvStateDir, EnvLockPath,
	} {
		os.Unsetenv(k)
	}
	// Note: we do NOT unset HOME here -- caller should restore it separately
	// if needed (e.g. via t.Setenv which auto-restores).
}

// Environ returns the environment variables as a slice of KEY=VALUE strings,
// suitable for appending to exec.Cmd.Env.
func (tc *TestContext) Environ() []string {
	return []string{
		fmt.Sprintf("%s=%s", EnvTestRunID, tc.RunID),
		fmt.Sprintf("%s=%s", EnvTestEnv, tc.RunID),
		fmt.Sprintf("%s=%s", EnvTmuxSocket, tc.SocketPath),
		fmt.Sprintf("%s=%s", EnvSessionsDir, tc.SessionsDir),
		fmt.Sprintf("%s=%s", EnvDBPath, tc.DBPath),
		fmt.Sprintf("%s=%s", EnvStateDir, tc.StateDir),
		fmt.Sprintf("%s=%s", EnvLockPath, tc.LockPath),
		fmt.Sprintf("HOME=%s", tc.HomeDir),
	}
}

// EnsureDirs creates the base directory, home directory, and sessions subdirectory.
func (tc *TestContext) EnsureDirs() error {
	if err := os.MkdirAll(tc.SessionsDir, 0700); err != nil {
		return err
	}
	if err := os.MkdirAll(tc.StateDir, 0700); err != nil {
		return err
	}
	return os.MkdirAll(tc.HomeDir, 0700)
}

// Cleanup removes the socket file and the entire base directory tree.
func (tc *TestContext) Cleanup() error {
	// Remove socket (lives outside baseDir)
	os.Remove(tc.SocketPath)
	return os.RemoveAll(tc.BaseDir)
}

// ForwardAuth symlinks LLM credential directories from the host HOME into
// the test environment's HomeDir based on the specified AuthMode.
//
// In AuthModeInherit: symlinks .claude/, .codex/, .config/gcloud/, .config/opencode/
// In AuthModeEnv: no-op (credentials come from env vars in CI)
// In AuthModeNone: no-op (complete isolation)
//
// Missing source directories are silently skipped. The HostHome field is
// set to hostHome for later reference.
func (tc *TestContext) ForwardAuth(hostHome string, mode AuthMode) error {
	tc.HostHome = hostHome

	if mode == AuthModeEnv || mode == AuthModeNone {
		return nil
	}

	if mode != AuthModeInherit {
		return fmt.Errorf("unknown auth mode: %q", mode)
	}

	for _, relPath := range credentialPaths {
		src := filepath.Join(hostHome, relPath)

		// Skip if source does not exist
		if _, err := os.Stat(src); os.IsNotExist(err) {
			continue
		}

		dst := filepath.Join(tc.HomeDir, relPath)

		// Create parent directories
		if err := os.MkdirAll(filepath.Dir(dst), 0700); err != nil {
			return fmt.Errorf("failed to create parent dir for %s: %w", relPath, err)
		}

		// Create symlink
		if err := os.Symlink(src, dst); err != nil {
			return fmt.Errorf("failed to symlink %s: %w", relPath, err)
		}
	}

	return nil
}
