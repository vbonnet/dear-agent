package tmux

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPaneCurrentPathPreservesNonASCIIBytes is the regression test for the
// transliteration failure: without -u, tmux rewrites every non-ASCII and
// control byte of a format result to "_", so PaneCurrentPath reports a
// directory that does not exist and EnsureSessionWorkDir concludes the pane
// landed somewhere else. It only ever reproduced on hosts that export no
// locale (launchd, cron, CI, a container, a bare agent shell).
//
// This needs no locale to be installed: -u is what makes it pass, which is
// why there is no skip here.
func TestPaneCurrentPathPreservesNonASCIIBytes(t *testing.T) {
	skipIfNoTmux(t)
	// Establish the condition the fix is for. Without this the test passes on
	// any developer or CI host that already exports a UTF-8 locale, even with
	// both -u flags removed, which would leave TMUX-52 with no real guard.
	t.Setenv("LC_ALL", "C")
	t.Setenv("LC_CTYPE", "C")
	t.Setenv("LANG", "C")
	_, cleanup := setupTestSocket(t)
	defer cleanup()
	setupTestState(t)

	for name, leaf := range map[string]string{
		"non-ASCII":         "café-❯",
		"control character": "valid\tworkdir",
	} {
		t.Run(name, func(t *testing.T) {
			workDir := filepath.Join(t.TempDir(), leaf)
			require.NoError(t, os.Mkdir(workDir, 0o755))

			sessionName := "locale-fidelity-" + sanitizeNewSessionName(name)
			require.NoError(t, NewSession(sessionName, workDir))
			defer killSession(sessionName)

			panePath, err := PaneCurrentPath(sessionName)
			require.NoError(t, err)
			assert.True(t, samePath(panePath, workDir),
				"pane_current_path %q lost bytes from workdir %q (tmux ASCII transliteration)", panePath, workDir)
		})
	}
}

// TestSessionPanesRunUnderUTF8Locale is the regression test for the second,
// independent failure mode: panes inherit the server's environment, and an
// interactive shell running under LC_CTYPE=C mangles the multi-byte characters
// of a literal send-keys command line. Harness launch commands carry
// box-drawing glyphs, so such a pane never execs the harness and readiness
// detection reports WRONG_HARNESS.
func TestSessionPanesRunUnderUTF8Locale(t *testing.T) {
	skipIfNoTmux(t)
	skipIfNoSessionLocalePin(t)
	socketPath, cleanup := setupTestSocket(t)
	defer cleanup()
	setupTestState(t)

	sessionName := "locale-pane-env"
	require.NoError(t, NewSession(sessionName, t.TempDir()))
	defer killSession(sessionName)

	assert.True(t, localeIsUTF8ForShell(sessionEnvLookup(t, socketPath, sessionName)),
		"session environment must be UTF-8 so pane shells keep multi-byte input intact")
}

// TestSessionPanesUTF8OnPreexistingASCIIServer pins the stale-server case.
// AGM's tmux server is long-lived: one started before this fix (or by any
// process without a locale) keeps its ASCII environment for its whole life,
// and tmux's update-environment does not carry LC_* across from the client.
// The session pin has to override what the session would inherit.
//
// The server here exports LC_ALL=C rather than nothing, because LC_ALL
// outranks LC_CTYPE: pinning LC_CTYPE alone would be silently undone, and a
// test that only unset the variables would not notice.
func TestSessionPanesUTF8OnPreexistingASCIIServer(t *testing.T) {
	skipIfNoTmux(t)
	skipIfNoSessionLocalePin(t)
	socketPath, cleanup := setupTestSocket(t)
	defer cleanup()
	setupTestState(t)

	seed := exec.Command("tmux", "-S", socketPath, "new-session", "-d", "-s", "ascii-seed")
	seed.Env = append(strippedLocaleEnv(), "LC_ALL=C")
	require.NoError(t, seed.Run(), "start ASCII-locale tmux server")
	defer func() { _ = exec.Command("tmux", "-S", socketPath, "kill-server").Run() }()

	// Assert the shape of the pin here rather than against the live socket:
	// this server is deterministically non-UTF-8, so args are always emitted.
	args := SessionLocaleArgs(socketPath)
	require.Len(t, args, 4, "expected two -e assignments, got %v", args)
	pinned := map[string]string{}
	for i := 0; i+1 < len(args); i += 2 {
		require.Equal(t, "-e", args[i])
		name, value, _ := strings.Cut(args[i+1], "=")
		pinned[name] = value
	}
	assert.Equal(t, "", pinned["LC_ALL"], "LC_ALL must be emptied so it cannot outrank the pinned LC_CTYPE")
	assert.True(t, localeNameIsUTF8(pinned["LC_CTYPE"]), "LC_CTYPE must be a UTF-8 locale, got %q", pinned["LC_CTYPE"])
	_, hasMessages := pinned["LC_MESSAGES"]
	assert.False(t, hasMessages, "LC_MESSAGES must not be pinned; pane programs keep their own language")
	if available, ok := installedLocales(); ok {
		assert.Contains(t, available, pinned["LC_CTYPE"], "pinned locale must be installed on this host")
	}

	sessionName := "locale-stale-server"
	require.NoError(t, NewSession(sessionName, t.TempDir()))
	defer killSession(sessionName)

	assert.True(t, localeIsUTF8ForShell(sessionEnvLookup(t, socketPath, sessionName)),
		"session on a pre-existing LC_ALL=C server must still get a UTF-8 pane environment")
}

// sessionEnvLookup reads the session's real environment from tmux and adapts
// it to the lookup signature localeIsUTF8 expects, so the pane environment is
// judged by exactly the rule tmux itself applies.
func sessionEnvLookup(t *testing.T, socketPath, sessionName string) func(string) (string, bool) {
	t.Helper()
	out, err := exec.Command("tmux", "-S", socketPath, "show-environment",
		"-t", NormalizeTmuxSessionName(sessionName)).Output()
	require.NoError(t, err)
	t.Logf("session environment:\n%s", out)
	return environBlockLookup(string(out))
}

// environBlockLookup parses `tmux show-environment` output into the lookup
// signature the locale predicates expect.
func environBlockLookup(block string) func(string) (string, bool) {
	return func(key string) (string, bool) {
		for line := range strings.SplitSeq(block, "\n") {
			trimmed := strings.TrimSpace(line)
			// tmux prints a removed variable as "-NAME"; that is "not set".
			if trimmed == "-"+key {
				return "", false
			}
			name, value, found := strings.Cut(trimmed, "=")
			if found && name == key {
				return value, true
			}
		}
		return "", false
	}
}

// strippedLocaleEnv returns the current environment with every locale variable
// removed, reproducing a launchd/cron style context.
func strippedLocaleEnv() []string {
	var env []string
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		if name == "LC_ALL" || name == "LC_CTYPE" || name == "LANG" {
			continue
		}
		env = append(env, entry)
	}
	return env
}

// skipIfNoSessionLocalePin skips tests whose subject is the pane environment
// when this host cannot pin one: either no UTF-8 locale is installed, or the
// tmux server predates `new-session -e`. Both are real host limitations rather
// than product defects, so they are reported as skips with the reason.
func skipIfNoSessionLocalePin(tb testing.TB) {
	tb.Helper()
	if !SessionLocalePinnable() {
		tb.Skip("host cannot pin a session locale: either no UTF-8 locale is installed, " +
			"or tmux predates new-session -e (3.2). See internal/tmux/locale.go")
	}
}

// TestSessionLocaleLeavesValidServerLocaleAlone guards against the pin doing
// harm on a correctly configured host. When the server already exports a valid
// UTF-8 locale, panes need no help, and overwriting it would replace the
// operator's choice with AGM's preferred one. In an LC_ALL-only setup that is
// not a harmless substitution: LC_ALL governs every category, so blanking it
// would change collation, formatting and message language too, contradicting
// the rule that only character classification is AGM's business.
func TestSessionLocaleLeavesValidServerLocaleAlone(t *testing.T) {
	skipIfNoTmux(t)
	skipIfNoSessionLocalePin(t)
	socketPath, cleanup := setupTestSocket(t)
	defer cleanup()
	setupTestState(t)

	// Seed with a locale this host has actually installed, not a hard-coded
	// en_US.UTF-8. On a minimal glibc image exposing only C.utf8,
	// skipIfNoSessionLocalePin passes (a pinnable locale exists) while the
	// hard-coded seed does not, so the production availability check correctly
	// refuses it, SessionLocaleArgs returns a pin, and the assertion below
	// fails for a host reason instead of testing what it claims to test.
	installed := pinnableLocale()
	require.NotEmpty(t, installed, "skipIfNoSessionLocalePin should have skipped without an installed locale")

	seed := exec.Command("tmux", "-S", socketPath, "new-session", "-d", "-s", "utf8-seed")
	seed.Env = append(strippedLocaleEnv(), "LC_ALL="+installed)
	require.NoError(t, seed.Run(), "start UTF-8-locale tmux server")
	defer func() { _ = exec.Command("tmux", "-S", socketPath, "kill-server").Run() }()

	assert.Empty(t, SessionLocaleArgs(socketPath),
		"a server that is already UTF-8 must be left untouched")

	sessionName := "locale-valid-server"
	require.NoError(t, NewSession(sessionName, t.TempDir()))
	defer killSession(sessionName)

	// With no pin there is no session-level environment at all; panes inherit
	// the server's, so that is what has to be checked and what has to be
	// unchanged.
	out, err := exec.Command("tmux", "-S", socketPath, "show-environment", "-g").Output()
	require.NoError(t, err)
	global := environBlockLookup(string(out))
	assert.True(t, localeIsUTF8ForShell(global), "the server must still be UTF-8:\n%s", out)
	value, ok := global("LC_ALL")
	assert.True(t, ok && value == installed,
		"the operator's own LC_ALL must survive untouched, got %q (set=%v)", value, ok)
}
