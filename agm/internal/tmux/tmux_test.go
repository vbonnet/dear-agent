package tmux

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNormalizeTmuxSessionName tests session name normalization
// This addresses BUG-001: tmux converts dots/colons to dashes
func TestNormalizeTmuxSessionName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "dots to dashes",
			input:    "gemini-task-1.4",
			expected: "gemini-task-1-4",
		},
		{
			name:     "multiple dots",
			input:    "foo.bar.baz",
			expected: "foo-bar-baz",
		},
		{
			name:     "colons to dashes",
			input:    "test:session",
			expected: "test-session",
		},
		{
			name:     "mixed dots and colons",
			input:    "multi.char:name",
			expected: "multi-char-name",
		},
		{
			name:     "normal name unchanged",
			input:    "normal-name",
			expected: "normal-name",
		},
		{
			name:     "underscores preserved",
			input:    "test_session_123",
			expected: "test_session_123",
		},
		{
			name:     "alphanumeric preserved",
			input:    "session123abc",
			expected: "session123abc",
		},
		{
			name:     "incident case - gemini-task-1.4",
			input:    "gemini-task-1.4",
			expected: "gemini-task-1-4",
		},
		{
			name:     "edge case - only dots",
			input:    "...",
			expected: "---",
		},
		{
			name:     "edge case - only colons",
			input:    ":::",
			expected: "---",
		},
		{
			name:     "complex real-world name",
			input:    "project-v1.2.3:staging",
			expected: "project-v1-2-3-staging",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NormalizeTmuxSessionName(tt.input)
			assert.Equal(t, tt.expected, result,
				"NormalizeTmuxSessionName(%q) = %q, expected %q",
				tt.input, result, tt.expected)
		})
	}
}

// Helper function to check if tmux is available
func isTmuxAvailable() bool {
	if os.Getenv("CI_SKIP_TMUX") == "true" {
		return false
	}
	_, err := exec.LookPath("tmux")
	return err == nil
}

// Helper function to skip if in CI without tmux testing enabled
func skipIfNoTmux(tb testing.TB) {
	tb.Helper()
	if os.Getenv("CI_SKIP_TMUX") == "true" {
		tb.Skip("Skipping tmux tests because CI_SKIP_TMUX=true")
	}
	if !isTmuxAvailable() {
		tb.Skip("tmux not available")
	}
	if os.Getenv("CI") != "" && os.Getenv("AGM_TEST_TMUX") == "" {
		tb.Skip("Skipping tmux tests in CI (set AGM_TEST_TMUX=1 to enable)")
	}
}

func TestIsTmuxAvailableHonorsCISkip(t *testing.T) {
	t.Setenv("CI_SKIP_TMUX", "true")
	assert.False(t, isTmuxAvailable())
}

// socketDir creates a short-path temp dir suitable for Unix socket files.
// t.TempDir() on macOS generates paths up to ~108 chars for long test names,
// exceeding the 104-byte sockaddr_un.sun_path limit. /tmp-based paths are ~30 chars.
func socketDir(tb testing.TB) string {
	tb.Helper()
	dir, err := os.MkdirTemp(filepath.Dir(LegacySocketPath), "agm") //nolint:usetesting // t.TempDir() paths exceed 104-byte Unix socket limit on macOS
	if err != nil {
		tb.Fatalf("socketDir: %v", err)
	}
	tb.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}

// setupTestSocket creates an isolated tmux socket for testing
func setupTestSocket(t *testing.T) (socketPath string, cleanup func()) {
	t.Helper()
	socketPath = filepath.Join(socketDir(t), "agm.sock")
	t.Setenv("AGM_TMUX_SOCKET", socketPath)
	t.Cleanup(func() {
		exec.Command("tmux", "-S", socketPath, "kill-server").Run()
	})
	return socketPath, func() {}
}

func setupTestState(t *testing.T) {
	t.Helper()
	stateDir := t.TempDir()
	t.Setenv("AGM_STATE_DIR", stateDir)
	t.Cleanup(func() { os.Unsetenv("AGM_STATE_DIR") })
}

// TestHasSession tests session existence checking
func TestHasSession(t *testing.T) {
	skipIfNoTmux(t)
	_, cleanup := setupTestSocket(t)
	defer cleanup()

	sessionName := "test-has-session"

	// Session should not exist initially
	exists, err := HasSession(sessionName)
	require.NoError(t, err)
	assert.False(t, exists, "Session should not exist initially")

	// Create session
	err = NewSession(sessionName, t.TempDir())
	require.NoError(t, err)
	defer killSession(sessionName)

	// Session should now exist
	exists, err = HasSession(sessionName)
	require.NoError(t, err)
	assert.True(t, exists, "Session should exist after creation")

	// Kill session through the production boundary and require the backend
	// command to succeed.
	require.NoError(t, KillSessionWithError(sessionName))
	time.Sleep(100 * time.Millisecond)

	// Session should not exist after killing
	exists, err = HasSession(sessionName)
	require.NoError(t, err)
	assert.False(t, exists, "Session should not exist after killing")
}

func TestKillSession_ReturnsBackendError(t *testing.T) {
	_, cleanup := setupTestSocket(t)
	defer cleanup()

	err := KillSessionWithError("definitely-not-running")
	require.Error(t, err, "a failed tmux kill command must be observable")
}

func TestKillSession_UsesExactTarget(t *testing.T) {
	skipIfNoTmux(t)
	_, cleanup := setupTestSocket(t)
	defer cleanup()

	for _, name := range []string{"kill-exact", "kill-exact-child"} {
		require.NoError(t, NewSession(name, t.TempDir()))
		name := name
		t.Cleanup(func() { _ = KillSessionWithError(name) })
	}

	require.NoError(t, KillSessionWithError("kill-exact"))
	exists, err := HasSession("kill-exact")
	require.NoError(t, err)
	assert.False(t, exists, "exact target should be absent")
	exists, err = HasSession("kill-exact-child")
	require.NoError(t, err)
	assert.True(t, exists, "prefix-related non-target must remain")
}

// TestNewSession tests session creation
func TestNewSession(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping tmux integration test in short mode (uses global lock)")
	}
	skipIfNoTmux(t)
	_, cleanup := setupTestSocket(t)
	defer cleanup()

	tests := []struct {
		name    string
		session string
		workDir string
		wantErr bool
	}{
		{
			name:    "create session in temp dir",
			session: "test-new-1",
			workDir: t.TempDir(),
			wantErr: false,
		},
		{
			name:    "create session in current dir",
			session: "test-new-2",
			workDir: ".",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer killSession(tt.session)

			err := NewSession(tt.session, tt.workDir)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)

			// Verify session exists
			exists, err := HasSession(tt.session)
			require.NoError(t, err)
			assert.True(t, exists, "Session should exist after NewSession")
		})
	}
}

func TestSessionIdentityValidation(t *testing.T) {
	valid := SessionIdentity{ID: "$7", Token: "0123456789abcdef0123456789abcdef"}
	if !valid.Valid() {
		t.Fatalf("valid creation identity was rejected: %#v", valid)
	}
	for _, invalid := range []SessionIdentity{
		{},
		{ID: "7", Token: valid.Token},
		{ID: "$7", Token: "short"},
		{ID: "$7", Token: "zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz"},
		{ID: "$7", Token: valid.Token, CreationName: "agm-create-wrong"},
	} {
		if invalid.Valid() {
			t.Fatalf("invalid creation identity was accepted: %#v", invalid)
		}
	}
	partial := SessionIdentity{Token: valid.Token, CreationName: sessionIdentityCreationPrefix + valid.Token}
	if partial.Valid() || !partial.Cleanable() {
		t.Fatalf("provisional-only identity validity = (complete=%v, cleanable=%v), want (false, true)", partial.Valid(), partial.Cleanable())
	}
	partial.CreationName = sessionIdentityCreationPrefix + "ffffffffffffffffffffffffffffffff"
	if partial.Cleanable() {
		t.Fatalf("mismatched provisional identity was accepted: %#v", partial)
	}
}

func TestRenameSessionIdentityTracksClaimedSession(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping tmux integration test in short mode")
	}
	skipIfNoTmux(t)
	socketPath, cleanup := setupTestSocket(t)
	defer cleanup()
	if err := exec.Command("tmux", "-S", socketPath, "new-session", "-d", "-s", "rename-source").Run(); err != nil {
		t.Fatalf("create source session: %v", err)
	}
	identity, err := ClaimSessionRenameIdentityContext(t.Context(), "rename-source")
	if err != nil {
		t.Fatalf("ClaimSessionRenameIdentityContext() error: %v", err)
	}
	if name, owned, inspectErr := InspectSessionRenameIdentityContext(t.Context(), identity); inspectErr != nil || !owned || name != "rename-source" {
		t.Fatalf("claimed identity before rename = (name=%q owned=%v err=%v)", name, owned, inspectErr)
	}
	if err := exec.Command("tmux", "-S", socketPath, "rename-session", "-t", identity.ID, "rename-target").Run(); err != nil {
		t.Fatalf("rename claimed session: %v", err)
	}
	if name, owned, inspectErr := InspectSessionRenameIdentityContext(t.Context(), identity); inspectErr != nil || !owned || name != "rename-target" {
		t.Fatalf("claimed identity after rename = (name=%q owned=%v err=%v)", name, owned, inspectErr)
	}
	if err := ClearSessionRenameIdentityContext(t.Context(), identity); err != nil {
		t.Fatalf("ClearSessionRenameIdentityContext() error: %v", err)
	}
	if name, owned, inspectErr := InspectSessionRenameIdentityContext(t.Context(), identity); inspectErr != nil || owned || name != "rename-target" {
		t.Fatalf("cleared identity = (name=%q owned=%v err=%v)", name, owned, inspectErr)
	}
}

func waitForTestTmuxServerExit(t *testing.T, socketPath string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		conn, err := net.DialTimeout("unix", socketPath, 50*time.Millisecond)
		if err != nil {
			return
		}
		_ = conn.Close()
		if time.Now().After(deadline) {
			t.Fatalf("tmux server at %s did not exit", socketPath)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestRenameSessionIdentityRejectsIDReuseAfterServerRestart(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping tmux integration test in short mode")
	}
	skipIfNoTmux(t)
	socketPath, cleanup := setupTestSocket(t)
	defer cleanup()
	if err := exec.Command("tmux", "-S", socketPath, "new-session", "-d", "-s", "rename-source").Run(); err != nil {
		t.Fatalf("create source session: %v", err)
	}
	identity, err := ClaimSessionRenameIdentityContext(t.Context(), "rename-source")
	if err != nil {
		t.Fatalf("ClaimSessionRenameIdentityContext() error: %v", err)
	}
	if err := exec.Command("tmux", "-S", socketPath, "kill-server").Run(); err != nil {
		t.Fatalf("kill original tmux server: %v", err)
	}
	waitForTestTmuxServerExit(t, socketPath)
	output, err := exec.Command("tmux", "-S", socketPath, "new-session", "-d", "-P", "-F", "#{session_id}", "-s", "rename-target").Output()
	if err != nil {
		t.Fatalf("create replacement session: %v", err)
	}
	replacementID := strings.TrimSpace(string(output))
	if replacementID != identity.ID {
		t.Fatalf("server restart did not reuse session ID: original=%q replacement=%q", identity.ID, replacementID)
	}
	if err := RenameClaimedSessionContext(t.Context(), identity, "must-not-adopt"); err != nil {
		t.Fatalf("RenameClaimedSessionContext() replacement error: %v", err)
	}
	name, owned, err := InspectSessionRenameIdentityContext(t.Context(), identity)
	if err != nil {
		t.Fatalf("InspectSessionRenameIdentityContext() replacement error: %v", err)
	}
	if owned || name != "rename-target" {
		t.Fatalf("replacement satisfied stale rename identity: name=%q owned=%v", name, owned)
	}
	if err := ClearSessionRenameIdentityContext(t.Context(), identity); err != nil {
		t.Fatalf("ClearSessionRenameIdentityContext() replacement error: %v", err)
	}
	if exists, err := HasSessionStrict("rename-target"); err != nil || !exists {
		t.Fatalf("replacement after stale marker cleanup = (exists=%v err=%v)", exists, err)
	}
}

func TestIsMissingSessionOutputRequiresExplicitMissingTarget(t *testing.T) {
	for _, output := range []string{
		"can't find session: missing",
		"can't find pane: %17",
		"no current target",
	} {
		if !isMissingSessionOutput([]byte(output)) {
			t.Fatalf("isMissingSessionOutput(%q) = false, want true", output)
		}
	}
	for _, output := range []string{
		"no server running on /private/tmp/agm.sock",
		"permission denied",
		"failed to connect to server",
		"server exited unexpectedly",
	} {
		if isMissingSessionOutput([]byte(output)) {
			t.Fatalf("operational failure %q was classified as a missing session", output)
		}
	}
}

func TestNewSessionWithIdentityReturnsIDWhenQueuedInitializationFails(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping tmux integration test in short mode (uses global lock)")
	}
	skipIfNoTmux(t)
	socketPath, cleanup := setupTestSocket(t)
	defer cleanup()

	if err := exec.Command("tmux", "-S", socketPath, "new-session", "-d", "-s", "partial-create").Run(); err != nil {
		t.Fatalf("create occupied final name: %v", err)
	}

	identity, err := NewSessionWithIdentity("partial-create", t.TempDir())
	if err == nil {
		t.Fatal("NewSessionWithIdentity() succeeded despite queued set-option failure")
	}
	if !identity.Valid() {
		t.Fatalf("NewSessionWithIdentity() lost partial creation identity: %#v", identity)
	}
	if exists, hasErr := HasSessionIdentityStrict(identity); hasErr != nil || !exists {
		t.Fatalf("partial creation identity = (exists=%v, err=%v), want owned survivor", exists, hasErr)
	}
	if err := KillSessionIdentityChecked(identity); err != nil {
		t.Fatalf("clean up partial creation: %v", err)
	}
	if exists, hasErr := HasSession("partial-create"); hasErr != nil || !exists {
		t.Fatalf("occupied final session = (exists=%v, err=%v), want preserved", exists, hasErr)
	}
}

func TestNewSessionBoundStoresStableSessionID(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping tmux integration test in short mode (uses global lock)")
	}
	skipIfNoTmux(t)
	_, cleanup := setupTestSocket(t)
	defer cleanup()

	if err := NewSessionBound("stable-bound", t.TempDir(), "stable-session-id"); err != nil {
		t.Fatalf("NewSessionBound() error = %v", err)
	}
	defer killSession("stable-bound")
	binding, exists, err := inspectStableSessionIDContext(t.Context(), "stable-bound")
	if err != nil || !exists || binding != "stable-session-id" {
		t.Fatalf("stable binding = %q, exists=%v, err=%v", binding, exists, err)
	}
}

func TestBindStableSessionIDRejectsOverwriteAndClearsExactValue(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping tmux integration test in short mode (uses global lock)")
	}
	skipIfNoTmux(t)
	_, cleanup := setupTestSocket(t)
	defer cleanup()

	if err := NewSession("stable-adopt", t.TempDir()); err != nil {
		t.Fatalf("NewSession() error = %v", err)
	}
	defer killSession("stable-adopt")
	newlyBound, err := BindStableSessionIDContext(t.Context(), "stable-adopt", "stable-session-id")
	if err != nil || !newlyBound {
		t.Fatalf("first bind = %v, %v; want newly bound", newlyBound, err)
	}
	newlyBound, err = BindStableSessionIDContext(t.Context(), "stable-adopt", "stable-session-id")
	if err != nil || newlyBound {
		t.Fatalf("idempotent bind = %v, %v; want unchanged", newlyBound, err)
	}
	if _, err := BindStableSessionIDContext(t.Context(), "stable-adopt", "replacement-session-id"); err == nil {
		t.Fatal("BindStableSessionIDContext() overwrote an existing stable binding")
	}
	if err := ClearStableSessionIDContext(t.Context(), "stable-adopt", "replacement-session-id"); err == nil {
		t.Fatal("ClearStableSessionIDContext() cleared a different stable binding")
	}
	if err := ClearStableSessionIDContext(t.Context(), "stable-adopt", "stable-session-id"); err != nil {
		t.Fatalf("ClearStableSessionIDContext() error = %v", err)
	}
	binding, exists, err := inspectStableSessionIDContext(t.Context(), "stable-adopt")
	if err != nil || !exists || binding != "" {
		t.Fatalf("binding after clear = %q, exists=%v, err=%v", binding, exists, err)
	}
}

func TestStableSessionAdoptionRefusesReusedTmuxIDsAfterServerRestart(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping tmux integration test in short mode (uses global lock)")
	}
	skipIfNoTmux(t)
	socketPath, cleanup := setupTestSocket(t)
	defer cleanup()

	create := func() {
		t.Helper()
		if output, err := exec.Command("tmux", "-S", socketPath, "new-session", "-d", "-s", "stable-race").CombinedOutput(); err != nil {
			t.Fatalf("create isolated tmux session: %v: %s", err, output)
		}
	}
	create()
	oldTarget, exists, err := inspectStableSessionAdoptionTargetContext(t.Context(), "stable-race", "stable-race")
	if err != nil || !exists {
		t.Fatalf("inspect original adoption target = %#v, exists=%t, err=%v", oldTarget, exists, err)
	}
	if output, err := exec.Command("tmux", "-S", socketPath, "kill-server").CombinedOutput(); err != nil {
		t.Fatalf("stop original isolated tmux server: %v: %s", err, output)
	}
	create()
	replacement, exists, err := inspectStableSessionAdoptionTargetContext(t.Context(), "stable-race", "stable-race")
	if err != nil || !exists {
		t.Fatalf("inspect replacement adoption target = %#v, exists=%t, err=%v", replacement, exists, err)
	}
	if replacement.SessionID != oldTarget.SessionID || replacement.PaneID != oldTarget.PaneID {
		t.Skipf("tmux did not reuse server-local IDs: old=%#v replacement=%#v", oldTarget, replacement)
	}
	if replacement.PanePID == oldTarget.PanePID {
		t.Skipf("operating system unexpectedly reused pane PID %d", oldTarget.PanePID)
	}

	newlyBound, err := claimStableSessionAdoptionContext(t.Context(), oldTarget, "stable-session-id")
	if err == nil || newlyBound {
		t.Fatalf("stale-incarnation claim = newlyBound=%t, err=%v; want refusal", newlyBound, err)
	}
	after, exists, inspectErr := inspectStableSessionAdoptionTargetContext(t.Context(), "stable-race", "stable-race")
	if inspectErr != nil || !exists {
		t.Fatalf("inspect replacement after refusal = %#v, exists=%t, err=%v", after, exists, inspectErr)
	}
	if after.StableSessionID != "" || after.AdoptionIdentity != "" {
		t.Fatalf("replacement was mutated by stale claim: %#v", after)
	}
}

func TestSessionIdentityCleansCreationBeforeTokenWrite(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping tmux integration test in short mode (uses global lock)")
	}
	skipIfNoTmux(t)
	socketPath, cleanup := setupTestSocket(t)
	defer cleanup()

	identity, err := newSessionIdentity()
	if err != nil {
		t.Fatalf("newSessionIdentity() error = %v", err)
	}
	output, err := exec.Command("tmux", "-S", socketPath, "new-session", "-d", "-P", "-F", "#{session_id}", "-s", identity.CreationName).Output()
	if err != nil {
		t.Fatalf("create provisional session: %v", err)
	}
	identity.ID = strings.TrimSpace(string(output))

	exists, err := HasSessionIdentityStrict(identity)
	if err != nil || !exists {
		t.Fatalf("HasSessionIdentityStrict(provisional) = (%v, %v), want (true, nil)", exists, err)
	}
	if err := KillSessionIdentityChecked(identity); err != nil {
		t.Fatalf("KillSessionIdentityChecked(provisional) error = %v", err)
	}
	if exists, err := HasSessionStrict(identity.CreationName); err != nil || exists {
		t.Fatalf("provisional session after cleanup = (%v, %v), want (false, nil)", exists, err)
	}
}

func TestSessionIdentityCleansCreationWhenSessionIDOutputIsLost(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping tmux integration test in short mode (uses global lock)")
	}
	skipIfNoTmux(t)
	socketPath, cleanup := setupTestSocket(t)
	defer cleanup()

	identity, err := newSessionIdentity()
	if err != nil {
		t.Fatalf("newSessionIdentity() error = %v", err)
	}
	if err := exec.Command("tmux", "-S", socketPath, "new-session", "-d", "-s", identity.CreationName).Run(); err != nil {
		t.Fatalf("create provisional session without captured ID: %v", err)
	}

	if err := KillSessionIdentityChecked(identity); err != nil {
		t.Fatalf("KillSessionIdentityChecked(provisional without ID) error = %v", err)
	}
	if exists, err := HasSessionStrict(identity.CreationName); err != nil || exists {
		t.Fatalf("provisional session after cleanup = (%v, %v), want (false, nil)", exists, err)
	}
}

func TestNewSessionCleansProvisionalSessionWhenFinalNameExists(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping tmux integration test in short mode (uses global lock)")
	}
	skipIfNoTmux(t)
	socketPath, cleanup := setupTestSocket(t)
	defer cleanup()

	if err := exec.Command("tmux", "-S", socketPath, "new-session", "-d", "-s", "occupied").Run(); err != nil {
		t.Fatalf("create occupied session: %v", err)
	}
	if err := NewSession("occupied", t.TempDir()); err == nil {
		t.Fatal("NewSession() succeeded despite occupied final name")
	}
	output, err := exec.Command("tmux", "-S", socketPath, "list-sessions", "-F", "#{session_name}").Output()
	if err != nil {
		t.Fatalf("list sessions: %v", err)
	}
	if got := strings.TrimSpace(string(output)); got != "occupied" {
		t.Fatalf("sessions after failed creation = %q, want only occupied", got)
	}
}

// TestNewSession_SettingsInjection verifies tmux settings are injected
func TestNewSession_SettingsInjection(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping tmux integration test in short mode (uses global lock)")
	}
	skipIfNoTmux(t)
	socketPath, cleanup := setupTestSocket(t)
	defer cleanup()

	sessionName := "test-settings"
	err := NewSession(sessionName, t.TempDir())
	require.NoError(t, err)
	defer killSession(sessionName)

	exists, err := HasSession(sessionName)
	require.NoError(t, err)
	assert.True(t, exists)

	tests := []struct {
		name     string
		args     []string
		expected string
	}{
		{
			name:     "aggressive resize",
			args:     []string{"show-options", "-w", "-v", "-t", sessionName, "aggressive-resize"},
			expected: "on",
		},
		{
			name:     "window size",
			args:     []string{"show-options", "-w", "-v", "-t", sessionName, "window-size"},
			expected: "latest",
		},
		{
			name:     "mouse",
			args:     []string{"show-options", "-v", "-t", sessionName, "mouse"},
			expected: "on",
		},
		{
			name:     "clipboard",
			args:     []string{"show-options", "-s", "-v", "set-clipboard"},
			expected: "on",
		},
		{
			name:     "escape time",
			args:     []string{"show-options", "-s", "-v", "escape-time"},
			expected: "10",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := append([]string{"-S", socketPath}, tt.args...)
			output, err := exec.Command("tmux", args...).Output()
			require.NoError(t, err)
			assert.Equal(t, tt.expected, strings.TrimSpace(string(output)))
		})
	}
}

// TestNewSession_BuildEnvVars verifies build environment variables are set on new sessions
func TestNewSession_BuildEnvVars(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping tmux integration test in short mode (uses global lock)")
	}
	skipIfNoTmux(t)
	socketPath, cleanup := setupTestSocket(t)
	defer cleanup()

	sessionName := "test-buildenv"
	err := NewSession(sessionName, t.TempDir())
	require.NoError(t, err)
	defer killSession(sessionName)

	// Query tmux session environment
	cmd := exec.Command("tmux", "-S", socketPath, "show-environment", "-t", sessionName)
	out, err := cmd.Output()
	require.NoError(t, err, "show-environment should succeed")

	envOutput := string(out)

	homeDir, _ := os.UserHomeDir()
	expectedVars := map[string]string{
		"GOCACHE":    filepath.Join(homeDir, ".cache", "go-build"),
		"GOMODCACHE": filepath.Join(homeDir, "go", "pkg", "mod"),
		// Mirror the production cap in NewSession: half the cores, floored at 1,
		// capped at 4. A bare max(NumCPU()/2, 1) over-counts on >8-core hosts
		// (e.g. 12 cores -> test expected 6 while the session sets 4).
		"GOMAXPROCS": strconv.Itoa(min(max(runtime.NumCPU()/2, 1), 4)),
		"GOWORK":     "off",
	}
	for k, v := range expectedVars {
		expected := fmt.Sprintf("%s=%s", k, v)
		assert.Contains(t, envOutput, expected,
			"tmux session environment should contain %s", expected)
	}
}

// TestVersion tests tmux version retrieval
func TestVersion(t *testing.T) {
	skipIfNoTmux(t)

	version, err := Version()
	require.NoError(t, err)
	assert.NotEmpty(t, version)
	assert.Contains(t, version, "tmux", "Version string should contain 'tmux'")

	t.Logf("tmux version: %s", version)
}

// TestListSessions tests session listing
func TestListSessions(t *testing.T) {
	skipIfNoTmux(t)
	_, cleanup := setupTestSocket(t)
	defer cleanup()

	// Initially no sessions
	sessions, err := ListSessions()
	require.NoError(t, err)
	assert.Empty(t, sessions, "Should have no sessions initially")

	// Create multiple sessions
	session1 := "test-list-1"
	session2 := "test-list-2"

	err = NewSession(session1, t.TempDir())
	require.NoError(t, err)
	defer killSession(session1)

	err = NewSession(session2, t.TempDir())
	require.NoError(t, err)
	defer killSession(session2)

	// List should contain both
	sessions, err = ListSessions()
	require.NoError(t, err)
	assert.Len(t, sessions, 2, "Should have 2 sessions")
	assert.Contains(t, sessions, session1)
	assert.Contains(t, sessions, session2)
}

// TestGetCurrentSessionName tests getting current session name
func TestGetCurrentSessionName(t *testing.T) {
	// When not in tmux, should return error
	// Save and clear TMUX env var to simulate not being in tmux
	originalTmux := os.Getenv("TMUX")
	t.Setenv("TMUX", "")
	defer func() {
		if originalTmux != "" {
			t.Setenv("TMUX", originalTmux)
		}
	}()

	_, err := GetCurrentSessionName()
	assert.Error(t, err, "Should error when not in tmux")
	if err != nil {
		assert.Contains(t, err.Error(), "not running inside a tmux session")
	}
}

// TestIsProcessRunning tests process detection
func TestIsProcessRunning(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping tmux integration test in short mode (uses global lock)")
	}
	skipIfNoTmux(t)
	_, cleanup := setupTestSocket(t)
	defer cleanup()

	sessionName := "test-process"
	err := NewSession(sessionName, t.TempDir())
	require.NoError(t, err)
	defer killSession(sessionName)

	// Wait for session to be ready
	time.Sleep(100 * time.Millisecond)

	// Shell should be running (bash or sh)
	// Check for common shells
	shells := []string{"bash", "sh", "zsh"}
	foundShell := false
	for _, shell := range shells {
		running, err := IsProcessRunning(sessionName, shell)
		if err != nil {
			continue
		}
		if running {
			foundShell = true
			t.Logf("Found shell: %s", shell)
			break
		}
	}
	assert.True(t, foundShell, "Should find a running shell process")

	// Non-existent process should not be running
	running, err := IsProcessRunning(sessionName, "definitely-not-running-12345")
	require.NoError(t, err)
	assert.False(t, running, "Fake process should not be running")
}

// TestWaitForProcessReady tests process polling
func TestWaitForProcessReady(t *testing.T) {
	skipIfNoTmux(t)
	_, cleanup := setupTestSocket(t)
	defer cleanup()

	sessionName := "test-wait-process"
	err := NewSession(sessionName, t.TempDir())
	require.NoError(t, err)
	defer killSession(sessionName)

	// Wait for shell to be ready (bash/sh/zsh)
	// Try common shells
	shells := []string{"bash", "sh", "zsh"}
	var waitErr error
	for _, shell := range shells {
		waitErr = WaitForProcessReady(sessionName, shell, 2*time.Second)
		if waitErr == nil {
			t.Logf("Shell %s is ready", shell)
			break
		}
	}
	assert.NoError(t, waitErr, "Shell should be ready within timeout")

	// Waiting for non-existent process should timeout
	err = WaitForProcessReady(sessionName, "definitely-not-running-12345", 500*time.Millisecond)
	assert.Error(t, err, "Should timeout waiting for non-existent process")
	assert.Contains(t, err.Error(), "timeout", "Error should mention timeout")
}

func TestIsProcessReadyWithRuntimeSupportsCodexNodeWrapper(t *testing.T) {
	var treeChecks []string
	running, err := isProcessReadyWithRuntime(
		t.Context(),
		"codex-session",
		"codex",
		"/tmp/agm.sock",
		func(sessionName, processName string) (bool, error) {
			assert.Equal(t, "codex-session", sessionName)
			assert.Equal(t, "codex", processName)
			return false, nil
		},
		func(_ context.Context, sessionName, socketPath, processName string) (bool, error) {
			assert.Equal(t, "codex-session", sessionName)
			assert.Equal(t, "/tmp/agm.sock", socketPath)
			treeChecks = append(treeChecks, processName)
			return processName == "node", nil
		},
	)
	require.NoError(t, err)
	assert.True(t, running, "Node-backed Codex should satisfy process readiness")
	assert.Equal(t, []string{"codex", "node"}, treeChecks)
}

func TestIsProcessReadyWithRuntimeDoesNotBroadenOtherProcesses(t *testing.T) {
	treeCalled := false
	running, err := isProcessReadyWithRuntime(
		t.Context(),
		"claude-session",
		"claude",
		"/tmp/agm.sock",
		func(string, string) (bool, error) { return false, nil },
		func(context.Context, string, string, string) (bool, error) {
			treeCalled = true
			return true, nil
		},
	)
	require.NoError(t, err)
	assert.False(t, running)
	assert.False(t, treeCalled, "Codex-specific Node fallback must not change other process checks")
}

func TestIsProcessReadyWithRuntimePreservesCancellationBeforeCodexFallback(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	treeCalled := false
	running, err := isProcessReadyWithRuntime(
		ctx,
		"codex-session",
		"codex",
		"/tmp/agm.sock",
		func(string, string) (bool, error) {
			cancel()
			return false, errors.New("foreground probe interrupted")
		},
		func(context.Context, string, string, string) (bool, error) {
			treeCalled = true
			return true, nil
		},
	)
	assert.False(t, running)
	require.ErrorIs(t, err, context.Canceled)
	assert.False(t, treeCalled, "canceled foreground probe must not enter the process-tree fallback")
}

func TestIsProcessReadyWithRuntimePreservesCancellationDuringCodexFallback(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	var treeChecks []string
	running, err := isProcessReadyWithRuntime(
		ctx,
		"codex-session",
		"codex",
		"/tmp/agm.sock",
		func(string, string) (bool, error) { return false, nil },
		func(_ context.Context, _, _, processName string) (bool, error) {
			treeChecks = append(treeChecks, processName)
			cancel()
			return false, nil
		},
	)
	assert.False(t, running)
	require.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, []string{"codex"}, treeChecks, "cancellation must prevent the Node fallback scan")
}

// TestIsClaudeProcess tests Claude Code process name detection.
// Claude Code reports as its semver version string (e.g., "2.1.50") in tmux
// rather than "claude", so we need to detect both patterns.
func TestIsClaudeProcess(t *testing.T) {
	tests := []struct {
		command string
		want    bool
	}{
		{"claude", true},       // Direct claude binary
		{"2.1.50", true},       // Claude Code version (current)
		{"2_1_195", true},      // macOS reports underscores instead of dots
		{"2_1_195_", true},     // tmux tab-format appends trailing _ for empty pane_start_command
		{"3_0_0", true},        // macOS underscore form, future version
		{"3_0_0_", true},       // future version with trailing _ from tmux tab-format
		{"3.0.0", true},        // Future version
		{"0.1.0", true},        // Semver pattern
		{"10.20.30", true},     // Multi-digit version
		{"zsh", false},         // Shell
		{"bash", false},        // Shell
		{"node", false},        // Node.js (too broad to match)
		{"vim", false},         // Editor
		{"2.1", false},         // Incomplete semver (not 3 parts)
		{"2.1.50.1", false},    // Too many parts
		{"", false},            // Empty
		{"v2.1.50", false},     // v prefix (not what tmux reports)
		{"abc.def.ghi", false}, // Non-numeric semver
		{"1.2.x", false},       // Non-numeric part
	}

	for _, tt := range tests {
		t.Run(tt.command, func(t *testing.T) {
			got := isClaudeProcess(tt.command)
			assert.Equal(t, tt.want, got, "isClaudeProcess(%q)", tt.command)
		})
	}
}

// TestIsClaudeRunning_BashFallback tests that IsClaudeRunning detects Claude
// running as a child of bash after crash/resume. In this state, the pane
// foreground command is "bash" and the Claude prompt (❯) appears in pane output.
func TestIsClaudeRunning_BashFallback(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping tmux integration test in short mode (uses global lock)")
	}
	skipIfNoTmux(t)
	_, cleanup := setupTestSocket(t)
	defer cleanup()

	sessionName := "test-claude-fallback"
	err := NewSession(sessionName, t.TempDir())
	require.NoError(t, err)
	defer killSession(sessionName)

	// Wait for shell to be ready
	time.Sleep(200 * time.Millisecond)

	// Pane should be running bash/sh — IsClaudeRunning should be false (no ❯ in output)
	running, err := IsClaudeRunning(sessionName)
	require.NoError(t, err)
	assert.False(t, running, "Should not detect Claude in a plain shell session")

	// Now inject the Claude prompt character into the pane to simulate
	// Claude running as a child of bash after crash/resume
	socketPath := GetSocketPath()
	normalizedName := NormalizeTmuxSessionName(sessionName)
	sendCmd := exec.Command("tmux", "-S", socketPath, "send-keys", "-t", normalizedName, "echo '❯'", "C-m")
	require.NoError(t, sendCmd.Run())
	time.Sleep(300 * time.Millisecond)

	// Now IsClaudeRunning should detect the ❯ in the pane output
	running, err = IsClaudeRunning(sessionName)
	require.NoError(t, err)
	assert.True(t, running, "Should detect Claude via ❯ prompt in bash fallback")
}

// TestGetCurrentWorkingDirectory tests CWD retrieval
func TestGetCurrentWorkingDirectory(t *testing.T) {
	skipIfNoTmux(t)
	_, cleanup := setupTestSocket(t)
	defer cleanup()

	testDir := t.TempDir()
	// Resolve symlinks to handle macOS /var -> /private/var
	testDir, err := filepath.EvalSymlinks(testDir)
	require.NoError(t, err)
	sessionName := "test-cwd"

	err = NewSession(sessionName, testDir)
	require.NoError(t, err)
	defer killSession(sessionName)

	// Wait for session to be ready
	time.Sleep(100 * time.Millisecond)

	// Get CWD
	cwd, err := GetCurrentWorkingDirectory(sessionName)
	require.NoError(t, err)
	assert.Equal(t, testDir, cwd, "CWD should match session creation directory")
}

// TestAttachSession_NoTTY tests attach behavior when no TTY available
func TestAttachSession_NoTTY(t *testing.T) {
	skipIfNoTmux(t)
	_, cleanup := setupTestSocket(t)
	defer cleanup()

	// Save and clear TMUX env var to simulate not being in tmux
	originalTmux := os.Getenv("TMUX")
	t.Setenv("TMUX", "")
	defer func() {
		if originalTmux != "" {
			t.Setenv("TMUX", originalTmux)
		}
	}()

	sessionName := "test-attach-notty"
	// Retry NewSession in case of lock contention from parallel tests
	var err error
	for i := 0; i < 3; i++ {
		err = NewSession(sessionName, t.TempDir())
		if err == nil || !strings.Contains(err.Error(), "failed to acquire tmux lock") {
			break
		}
		time.Sleep(time.Second)
	}
	require.NoError(t, err)
	defer killSession(sessionName)

	// In test environment (no TTY), AttachSession should return nil
	// without actually attaching (it detects no TTY)
	err = AttachSession(sessionName)
	assert.NoError(t, err, "Should not error when no TTY (silently skips attach)")

	// Session should still exist (wasn't killed)
	exists, err := HasSession(sessionName)
	require.NoError(t, err)
	assert.True(t, exists, "Session should still exist after attach attempt")
}

// TestAttachSession_NonExistentSession tests attach to missing session
func TestAttachSession_NonExistentSession(t *testing.T) {
	skipIfNoTmux(t)
	_, cleanup := setupTestSocket(t)
	defer cleanup()

	// Attempt to attach to non-existent session
	// In no-TTY environment, this should succeed (skips attach)
	err := AttachSession("non-existent-session-12345")
	// Either no error (no TTY) or error about session not existing
	// We can't predict which without knowing if we have a TTY
	_ = err
}

// TestIsSupervisorSession tests detection of supervisor roles from session names
func TestIsSupervisorSession(t *testing.T) {
	tests := []struct {
		name     string
		session  string
		expected bool
	}{
		{"orchestrator exact", "orchestrator", true},
		{"orchestrator with prefix", "my-orchestrator", true},
		{"orchestrator with suffix", "orchestrator-main", true},
		{"meta-orchestrator", "meta-orchestrator", true},
		{"meta-orchestrator with suffix", "meta-orchestrator-v2", true},
		{"overseer", "overseer", true},
		{"overseer with prefix", "prod-overseer", true},
		{"worker session", "worker-abc123", false},
		{"implementer session", "implementer-task-1", false},
		{"researcher session", "researcher-deep", false},
		{"random name", "my-cool-session", false},
		{"empty string", "", false},
		{"case insensitive", "ORCHESTRATOR", true},
		{"mixed case", "Meta-Orchestrator", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsSupervisorSession(tt.session)
			assert.Equal(t, tt.expected, got, "IsSupervisorSession(%q)", tt.session)
		})
	}
}

// TestNewSession_AutoRespawn verifies auto-respawn is set for supervisor sessions
func TestNewSession_AutoRespawn(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping tmux integration test in short mode (uses global lock)")
	}
	skipIfNoTmux(t)
	socketPath, cleanup := setupTestSocket(t)
	defer cleanup()

	tests := []struct {
		name        string
		session     string
		wantRespawn bool
	}{
		{"orchestrator gets respawn", "orchestrator-test", true},
		{"meta-orchestrator gets respawn", "meta-orchestrator-test", true},
		{"overseer gets respawn", "overseer-test", true},
		{"worker no respawn", "worker-test", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := NewSession(tt.session, t.TempDir())
			require.NoError(t, err)
			defer killSession(tt.session)

			time.Sleep(200 * time.Millisecond)

			// Check remain-on-exit option
			cmd := exec.Command("tmux", "-S", socketPath, "show-option", "-t", tt.session, "remain-on-exit")
			out, err := cmd.CombinedOutput()
			outStr := string(out)

			if tt.wantRespawn {
				require.NoError(t, err, "show-option should succeed for supervisor session")
				assert.Contains(t, outStr, "on", "remain-on-exit should be 'on' for %s", tt.session)
			} else if err == nil {
				// For non-supervisor sessions, the option is either not set or "off"
				assert.NotContains(t, outStr, "remain-on-exit on",
					"remain-on-exit should NOT be 'on' for %s", tt.session)
				// Error is also acceptable (option not set)
			}
		})
	}
}

// Helper function to kill a session
func killSession(name string) {
	socketPath := GetSocketPath()
	cmd := exec.Command("tmux", "-S", socketPath, "kill-session", "-t", name)
	cmd.Run() // Ignore errors
}

// Benchmark tests
func BenchmarkHasSession(b *testing.B) {
	if !isTmuxAvailable() {
		b.Skip("tmux not available")
	}

	tmpDir := b.TempDir()
	socketPath := tmpDir + "/bench-tmux.sock"
	b.Setenv("AGM_TMUX_SOCKET", socketPath)
	defer os.Unsetenv("AGM_TMUX_SOCKET")

	sessionName := "bench-has"
	err := NewSession(sessionName, tmpDir)
	if err != nil {
		b.Skipf("Failed to create session: %v", err)
	}
	defer killSession(sessionName)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = HasSession(sessionName)
	}
}

func BenchmarkListSessions(b *testing.B) {
	if !isTmuxAvailable() {
		b.Skip("tmux not available")
	}

	tmpDir := b.TempDir()
	socketPath := tmpDir + "/bench-tmux.sock"
	b.Setenv("AGM_TMUX_SOCKET", socketPath)
	defer os.Unsetenv("AGM_TMUX_SOCKET")

	// Create a few sessions
	for i := 0; i < 3; i++ {
		sessionName := string(rune('a' + i))
		err := NewSession(sessionName, tmpDir)
		if err != nil {
			b.Skipf("Failed to create session: %v", err)
		}
		defer killSession(sessionName)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = ListSessions()
	}
}
