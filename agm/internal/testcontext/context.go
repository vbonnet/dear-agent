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
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
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

	cleanupBaseDirs    []string
	cleanupSocketPaths []string
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
	return newNamedWithRoot(name, testEnvironmentRoot), nil
}

// newWithID constructs a random context beneath the canonical short root.
func newWithID(id string) *TestContext {
	return newWithRoot(id, testEnvironmentRoot)
}

func newWithRoot(id, root string) *TestContext {
	// Keep tmux socket paths below macOS's Unix-domain socket limit. os.TempDir
	// expands to a much longer /var/folders path there, while /tmp is stable on
	// every Unix platform on which AGM's tmux integration runs.
	baseDir := filepath.Join(root, testEnvironmentPrefix+id)
	socketPath := filepath.Join(root, testEnvironmentPrefix+id+".sock")
	return &TestContext{
		RunID:       id,
		BaseDir:     baseDir,
		HomeDir:     filepath.Join(baseDir, "home"),
		SocketPath:  socketPath,
		SessionsDir: filepath.Join(baseDir, "sessions"),
		DBPath:      filepath.Join(baseDir, "agm.db"),
		StateDir:    filepath.Join(baseDir, "state"),
		LockPath:    filepath.Join(baseDir, "agm.lock"),

		cleanupBaseDirs:    []string{baseDir},
		cleanupSocketPaths: []string{socketPath},
	}
}

// LoadNamed reconstructs a TestContext from a validated known name.
// It does not verify that the directory exists. Cleanup includes the exact
// same-name path under the retired os.TempDir root so pre-migration
// environments cannot be reported destroyed while left behind.
func LoadNamed(name string) (*TestContext, error) {
	if err := ValidateName(name); err != nil {
		return nil, err
	}
	return newNamedWithRoot(name, testEnvironmentRoot), nil
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

// ListNamed returns centrally validated environments from the canonical short
// root plus the exact retired os.TempDir root used before the socket-path
// migration. Canonical entries win when both roots contain the same name.
func ListNamed() ([]*TestContext, error) {
	contexts := make([]*TestContext, 0)
	seen := make(map[string]struct{})
	for _, root := range namedEnvironmentRoots() {
		entries, err := os.ReadDir(root)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		for _, entry := range entries {
			if !entry.IsDir() || !strings.HasPrefix(entry.Name(), testEnvironmentPrefix) {
				continue
			}
			name := strings.TrimPrefix(entry.Name(), testEnvironmentPrefix)
			if err := ValidateName(name); err != nil {
				continue
			}
			if _, exists := seen[name]; exists {
				continue
			}
			seen[name] = struct{}{}
			contexts = append(contexts, newNamedWithRoot(name, root))
		}
	}
	sort.Slice(contexts, func(i, j int) bool { return contexts[i].RunID < contexts[j].RunID })
	return contexts, nil
}

func newNamedWithRoot(name, primaryRoot string) *TestContext {
	tc := newWithRoot(name, primaryRoot)
	for _, root := range namedEnvironmentRoots() {
		baseDir := filepath.Join(root, testEnvironmentPrefix+name)
		socketPath := filepath.Join(root, testEnvironmentPrefix+name+".sock")
		if baseDir != tc.BaseDir {
			tc.cleanupBaseDirs = append(tc.cleanupBaseDirs, baseDir)
		}
		if socketPath != tc.SocketPath {
			tc.cleanupSocketPaths = append(tc.cleanupSocketPaths, socketPath)
		}
	}
	return tc
}

func namedEnvironmentRoots() []string {
	roots := []string{testEnvironmentRoot}
	retired := filepath.Clean(os.TempDir())
	if retired != testEnvironmentRoot && filepath.IsAbs(retired) && retired != string(filepath.Separator) {
		roots = append(roots, retired)
	}
	return roots
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

// Cleanup removes only the context's exact socket and directory paths. Named
// contexts also remove their exact same-name paths from the retired os.TempDir
// root so the fixed-root migration does not orphan credentials or tmux state.
func (tc *TestContext) Cleanup() error {
	socketPaths := tc.cleanupSocketPaths
	if len(socketPaths) == 0 {
		socketPaths = []string{tc.SocketPath}
	}
	baseDirs := tc.cleanupBaseDirs
	if len(baseDirs) == 0 {
		baseDirs = []string{tc.BaseDir}
	}

	var cleanupErr error
	for _, socketPath := range socketPaths {
		if err := os.Remove(socketPath); err != nil && !os.IsNotExist(err) {
			cleanupErr = errors.Join(cleanupErr, fmt.Errorf("remove test socket %s: %w", socketPath, err))
		}
	}
	for _, baseDir := range baseDirs {
		if err := os.RemoveAll(baseDir); err != nil {
			cleanupErr = errors.Join(cleanupErr, fmt.Errorf("remove test environment %s: %w", baseDir, err))
		}
	}
	return cleanupErr
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
