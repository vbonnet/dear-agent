// Package testcontext provides per-run test sandbox isolation for AGM.
//
// Each test run gets a unique ID and fully isolated paths:
//   - Tmux socket:   /tmp/agm-u-{uid}/agm-test-{id}.sock
//   - Sessions dir:  /tmp/agm-u-{uid}/agm-test-{id}/sessions/
//   - Home dir:      /tmp/agm-u-{uid}/agm-test-{id}/home/
//   - SQLite DB:     /tmp/agm-u-{uid}/agm-test-{id}/agm.db
//   - State dir:     /tmp/agm-u-{uid}/agm-test-{id}/state/
//   - Lock file:     /tmp/agm-u-{uid}/agm-test-{id}/agm.lock
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
	"strconv"
	"strings"
	"syscall"
	"unicode"

	"github.com/google/uuid"
)

const (
	testEnvironmentPrefix    = "agm-test-"
	shortTestEnvironmentRoot = "/tmp"
	maxNewEnvironmentName    = 64
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
	if err := rejectRetiredNamedEnvironmentCollision(name); err != nil {
		return nil, err
	}
	return newNamedWithRoot(name, canonicalEnvironmentRoot())
}

// newWithID constructs a random context beneath the canonical short root.
func newWithID(id string) *TestContext {
	return newWithRoot(id, canonicalEnvironmentRoot())
}

func newWithRoot(id, root string) *TestContext {
	// Keep tmux socket paths below macOS's Unix-domain socket limit. The
	// canonical root is a short effective-user namespace beneath /tmp rather
	// than the much longer host os.TempDir path.
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

// LoadNamed reconstructs a TestContext from a validated known name. Existing
// environments are resolved in canonical-first order across the current
// per-user root and the retired short and host temporary roots. If no directory
// exists, it returns canonical paths without creating them.
func LoadNamed(name string) (*TestContext, error) {
	if err := validatePathSafeName(name); err != nil {
		return nil, err
	}
	root, err := resolveNamedEnvironmentRoot(name)
	if err != nil {
		return nil, err
	}
	return newNamedWithRoot(name, root)
}

// ValidateName rejects new names that could escape the owned temporary root,
// inject terminal control characters, or exceed the short macOS socket budget.
// LoadNamed and ListNamed deliberately retain path-safe access to longer names
// created before the socket-path limit existed so they can still be destroyed.
func ValidateName(name string) error {
	if err := validatePathSafeName(name); err != nil {
		return err
	}
	if len(name) > maxNewEnvironmentName {
		return fmt.Errorf("test environment name must not exceed %d bytes", maxNewEnvironmentName)
	}
	return nil
}

func validatePathSafeName(name string) error {
	switch {
	case name == "":
		return fmt.Errorf("test environment name must not be empty")
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

// ListNamed returns centrally validated, current-user environments from the
// canonical per-user short root plus the retired global short and host
// temporary roots. Canonical entries win when roots contain the same name.
func ListNamed() ([]*TestContext, error) {
	contexts := make([]*TestContext, 0)
	seen := make(map[string]struct{})
	for _, root := range namedEnvironmentRoots() {
		exists, err := validateNamedEnvironmentRoot(root)
		if err != nil {
			return nil, err
		}
		if !exists {
			continue
		}
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
			if err := validatePathSafeName(name); err != nil {
				continue
			}
			if _, exists := seen[name]; exists {
				continue
			}
			owned, err := ownedPath(filepath.Join(root, entry.Name()), true)
			if err != nil {
				return nil, err
			}
			if !owned {
				continue
			}
			seen[name] = struct{}{}
			tc, err := newNamedWithRoot(name, root)
			if err != nil {
				return nil, err
			}
			contexts = append(contexts, tc)
		}
	}
	sort.Slice(contexts, func(i, j int) bool { return contexts[i].RunID < contexts[j].RunID })
	return contexts, nil
}

func newNamedWithRoot(name, primaryRoot string) (*TestContext, error) {
	primaryExists, err := validateNamedEnvironmentRoot(primaryRoot)
	if err != nil {
		return nil, err
	}
	if !primaryExists && primaryRoot != canonicalEnvironmentRoot() {
		return nil, fmt.Errorf("retired test environment root does not exist: %s", primaryRoot)
	}
	if primaryRoot == canonicalEnvironmentRoot() {
		if _, err := ownedPath(filepath.Join(primaryRoot, testEnvironmentPrefix+name), true); err != nil {
			return nil, err
		}
	}
	tc := newWithRoot(name, primaryRoot)
	for _, root := range namedEnvironmentRoots() {
		exists, err := validateNamedEnvironmentRoot(root)
		if err != nil {
			return nil, err
		}
		if !exists {
			continue
		}
		baseDir := filepath.Join(root, testEnvironmentPrefix+name)
		socketPath := filepath.Join(root, testEnvironmentPrefix+name+".sock")
		if baseDir != tc.BaseDir {
			owned, err := ownedPath(baseDir, true)
			if err != nil {
				return nil, err
			}
			if owned {
				tc.cleanupBaseDirs = append(tc.cleanupBaseDirs, baseDir)
			}
		}
		if socketPath != tc.SocketPath {
			owned, err := ownedPath(socketPath, false)
			if err != nil {
				return nil, err
			}
			if owned {
				tc.cleanupSocketPaths = append(tc.cleanupSocketPaths, socketPath)
			}
		}
	}
	return tc, nil
}

func namedEnvironmentRoots() []string {
	candidates := []string{
		canonicalEnvironmentRoot(),
		shortTestEnvironmentRoot,
		filepath.Clean(os.TempDir()),
	}
	roots := make([]string, 0, len(candidates))
	seen := make(map[string]struct{}, len(candidates))
	for _, candidate := range candidates {
		candidate = filepath.Clean(candidate)
		if !filepath.IsAbs(candidate) || candidate == string(filepath.Separator) {
			continue
		}
		if _, exists := seen[candidate]; exists {
			continue
		}
		seen[candidate] = struct{}{}
		roots = append(roots, candidate)
	}
	return roots
}

func canonicalEnvironmentRoot() string {
	return canonicalEnvironmentRootForUID(os.Geteuid())
}

func canonicalEnvironmentRootForUID(uid int) string {
	return filepath.Join(
		shortTestEnvironmentRoot,
		"agm-u-"+strconv.Itoa(uid),
	)
}

func resolveNamedEnvironmentRoot(name string) (string, error) {
	for _, root := range namedEnvironmentRoots() {
		exists, err := validateNamedEnvironmentRoot(root)
		if err != nil {
			return "", err
		}
		if !exists {
			continue
		}
		baseDir := filepath.Join(root, testEnvironmentPrefix+name)
		owned, err := ownedPath(baseDir, true)
		if err != nil {
			return "", err
		}
		if owned {
			return root, nil
		}
	}
	return canonicalEnvironmentRoot(), nil
}

func rejectRetiredNamedEnvironmentCollision(name string) error {
	canonicalRoot := canonicalEnvironmentRoot()
	for _, root := range namedEnvironmentRoots() {
		if root == canonicalRoot {
			continue
		}
		exists, err := validateNamedEnvironmentRoot(root)
		if err != nil {
			return err
		}
		if !exists {
			continue
		}
		for _, candidate := range []struct {
			path             string
			requireDirectory bool
		}{
			{path: filepath.Join(root, testEnvironmentPrefix+name), requireDirectory: true},
			{path: filepath.Join(root, testEnvironmentPrefix+name+".sock"), requireDirectory: false},
		} {
			owned, err := ownedPath(candidate.path, candidate.requireDirectory)
			if err != nil {
				return err
			}
			if owned {
				return fmt.Errorf(
					"retired test environment %q already exists at %s; destroy it before creating a canonical environment",
					name,
					candidate.path,
				)
			}
		}
	}
	return nil
}

func validateNamedEnvironmentRoot(root string) (bool, error) {
	if root == shortTestEnvironmentRoot {
		return true, nil
	}
	return validateExistingOwnedEnvironmentRoot(root)
}

func validateExistingOwnedEnvironmentRoot(root string) (bool, error) {
	info, err := os.Lstat(root)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("inspect per-user test environment root %s: %w", root, err)
	}
	if !info.IsDir() {
		return false, fmt.Errorf("per-user test environment root is not a directory: %s", root)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return false, fmt.Errorf("per-user test environment root has unsupported ownership metadata: %s", root)
	}
	// #nosec G115 -- effective Unix user IDs are non-negative and Stat_t.Uid is uint32.
	if stat.Uid != uint32(os.Geteuid()) {
		return false, fmt.Errorf("per-user test environment root is not owned by uid %d: %s", os.Geteuid(), root)
	}
	if info.Mode().Perm() != 0700 {
		return false, fmt.Errorf("per-user test environment root is not owner-only: %s", root)
	}
	return true, nil
}

func ownedPath(path string, requireDirectory bool) (bool, error) {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("inspect test environment path %s: %w", path, err)
	}
	if requireDirectory && !info.IsDir() {
		return false, fmt.Errorf("test environment path is not a directory: %s", path)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return false, fmt.Errorf("test environment path has unsupported ownership metadata: %s", path)
	}
	// #nosec G115 -- effective Unix user IDs are non-negative and Stat_t.Uid is uint32.
	return stat.Uid == uint32(os.Geteuid()), nil
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
	if err := validatePathSafeName(runID); err != nil {
		return nil, false
	}
	sessionsDir := os.Getenv(EnvSessionsDir)
	baseDir := filepath.Dir(sessionsDir)
	if sessionsDir == "" {
		baseDir = filepath.Join(canonicalEnvironmentRoot(), testEnvironmentPrefix+runID)
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
	if filepath.Dir(tc.BaseDir) == canonicalEnvironmentRoot() {
		if err := ensureOwnedEnvironmentRoot(canonicalEnvironmentRoot()); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(tc.SessionsDir, 0700); err != nil {
		return err
	}
	if err := os.MkdirAll(tc.StateDir, 0700); err != nil {
		return err
	}
	return os.MkdirAll(tc.HomeDir, 0700)
}

func ensureOwnedEnvironmentRoot(root string) error {
	if err := os.Mkdir(root, 0700); err != nil && !os.IsExist(err) {
		return fmt.Errorf("create per-user test environment root %s: %w", root, err)
	}
	owned, err := ownedPath(root, true)
	if err != nil {
		return err
	}
	if !owned {
		return fmt.Errorf("per-user test environment root is not owned by uid %d: %s", os.Geteuid(), root)
	}
	info, err := os.Lstat(root)
	if err != nil {
		return fmt.Errorf("inspect per-user test environment root %s: %w", root, err)
	}
	if info.Mode().Perm() != 0700 {
		// #nosec G302 -- directories need execute permission; 0700 is owner-only.
		if err := os.Chmod(root, 0700); err != nil {
			return fmt.Errorf("secure per-user test environment root %s: %w", root, err)
		}
	}
	return nil
}

// Cleanup removes only the context's exact socket and directory paths. Named
// contexts also remove owned exact same-name paths from retired roots so root
// migrations do not orphan credentials or tmux state.
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
		if ok, err := pathSafeForCleanup(socketPath, false); err != nil {
			cleanupErr = errors.Join(cleanupErr, err)
			continue
		} else if !ok {
			continue
		}
		if err := os.Remove(socketPath); err != nil && !os.IsNotExist(err) {
			cleanupErr = errors.Join(cleanupErr, fmt.Errorf("remove test socket %s: %w", socketPath, err))
		}
	}
	for _, baseDir := range baseDirs {
		if ok, err := pathSafeForCleanup(baseDir, true); err != nil {
			cleanupErr = errors.Join(cleanupErr, err)
			continue
		} else if !ok {
			continue
		}
		if err := os.RemoveAll(baseDir); err != nil {
			cleanupErr = errors.Join(cleanupErr, fmt.Errorf("remove test environment %s: %w", baseDir, err))
		}
	}
	return cleanupErr
}

func pathSafeForCleanup(path string, requireDirectory bool) (bool, error) {
	root := filepath.Dir(path)
	if root != shortTestEnvironmentRoot {
		exists, err := validateExistingOwnedEnvironmentRoot(root)
		if err != nil {
			return false, err
		}
		if !exists {
			return false, nil
		}
	}
	return ownedPath(path, requireDirectory)
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
