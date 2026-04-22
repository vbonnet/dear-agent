package main

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// getSessionName tests
// ---------------------------------------------------------------------------

func TestGetSessionName_FromEnvVar(t *testing.T) {
	t.Setenv("CLAUDE_SESSION_NAME", "my-session")
	assert.Equal(t, "my-session", getSessionName())
}

func TestGetSessionName_EnvVarTakesPrecedence(t *testing.T) {
	// Even when tmux might be available, the env var should win.
	t.Setenv("CLAUDE_SESSION_NAME", "env-session")
	name := getSessionName()
	assert.Equal(t, "env-session", name)
}

func TestGetSessionName_EmptyEnvFallsBackToTmux(t *testing.T) {
	t.Setenv("CLAUDE_SESSION_NAME", "")
	name := getSessionName()
	// Result depends on whether tmux is available in the test env.
	// The function must not panic regardless.
	tmuxName := detectTmuxSession()
	assert.Equal(t, tmuxName, name)
}

func TestGetSessionName_UnsetEnvFallsBackToTmux(t *testing.T) {
	t.Setenv("CLAUDE_SESSION_NAME", "")
	os.Unsetenv("CLAUDE_SESSION_NAME")
	name := getSessionName()
	tmuxName := detectTmuxSession()
	assert.Equal(t, tmuxName, name)
}

func TestGetSessionName_VariousEnvValues(t *testing.T) {
	tests := []struct {
		name     string
		envValue string
		want     string
	}{
		{"simple name", "alpha", "alpha"},
		{"with hyphens", "my-long-session-42", "my-long-session-42"},
		{"with underscores", "session_name_1", "session_name_1"},
		{"with dots", "user.session.3", "user.session.3"},
		{"unicode", "sesi\u00f3n", "sesi\u00f3n"},
		{"spaces in name", "session with spaces", "session with spaces"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("CLAUDE_SESSION_NAME", tt.envValue)
			assert.Equal(t, tt.want, getSessionName())
		})
	}
}

// ---------------------------------------------------------------------------
// detectTmuxSession tests
// ---------------------------------------------------------------------------

func TestDetectTmuxSession_ReturnsString(t *testing.T) {
	result := detectTmuxSession()
	assert.IsType(t, "", result)
}

func TestDetectTmuxSession_NoCrash(t *testing.T) {
	// Ensure repeated calls don't panic or leak resources.
	for i := 0; i < 5; i++ {
		_ = detectTmuxSession()
	}
}

// ---------------------------------------------------------------------------
// buildAgmCommand tests
// ---------------------------------------------------------------------------

func TestBuildAgmCommand_ArgsCorrect(t *testing.T) {
	cmd := buildAgmCommand("test-session", "READY", "sessionend-hook")

	require.NotNil(t, cmd)
	expected := []string{"agm", "session", "state", "set", "test-session", "READY", "--source", "sessionend-hook"}
	assert.Equal(t, expected, cmd.Args)
}

func TestBuildAgmCommand_DifferentState(t *testing.T) {
	cmd := buildAgmCommand("s1", "WORKING", "post-tool-hook")

	expected := []string{"agm", "session", "state", "set", "s1", "WORKING", "--source", "post-tool-hook"}
	assert.Equal(t, expected, cmd.Args)
}

func TestBuildAgmCommand_StdoutAndStderrToStderr(t *testing.T) {
	cmd := buildAgmCommand("sess", "READY", "sessionend-hook")
	assert.Equal(t, os.Stderr, cmd.Stdout, "stdout should be redirected to stderr")
	assert.Equal(t, os.Stderr, cmd.Stderr, "stderr should remain stderr")
}

func TestBuildAgmCommand_SpecialCharactersInSession(t *testing.T) {
	cmd := buildAgmCommand("session/with/slashes", "READY", "sessionend-hook")
	assert.Equal(t, "session/with/slashes", cmd.Args[4])
}

func TestBuildAgmCommand_EmptySessionName(t *testing.T) {
	cmd := buildAgmCommand("", "READY", "sessionend-hook")
	require.NotNil(t, cmd)
	assert.Equal(t, "", cmd.Args[4])
}

// ---------------------------------------------------------------------------
// run() tests
// ---------------------------------------------------------------------------

func TestRun_WithSessionName_ReturnsTrue(t *testing.T) {
	t.Setenv("CLAUDE_SESSION_NAME", "run-test-session")
	// run() will attempt to exec agm which likely isn't in PATH during tests,
	// but it ignores errors (advisory hook). It should still return true.
	result := run()
	assert.True(t, result)
}

func TestRun_WithoutSessionName_ReturnsFalse(t *testing.T) {
	t.Setenv("CLAUDE_SESSION_NAME", "")
	os.Unsetenv("CLAUDE_SESSION_NAME")

	// If tmux is available, run() will detect a session and return true.
	// If tmux is not available, run() returns false.
	tmuxName := detectTmuxSession()
	result := run()
	if tmuxName == "" {
		assert.False(t, result, "no session available, run should return false")
	} else {
		assert.True(t, result, "tmux session detected, run should return true")
	}
}

func TestRun_WithEnvName_AttemptsAgmCall(t *testing.T) {
	// Verify run() doesn't panic even when agm binary is missing.
	t.Setenv("CLAUDE_SESSION_NAME", "agm-missing-test")
	assert.NotPanics(t, func() {
		run()
	})
}

func TestRun_EmptySessionSkipsAgm(t *testing.T) {
	// Force no session by setting env to empty and ensuring tmux fallback
	// returns empty (use PATH manipulation to hide tmux).
	origPath := os.Getenv("PATH")
	t.Setenv("PATH", "/nonexistent")
	t.Setenv("CLAUDE_SESSION_NAME", "")
	os.Unsetenv("CLAUDE_SESSION_NAME")

	result := run()
	assert.False(t, result, "with no session name, run should return false")

	// Restore PATH (t.Setenv already handles cleanup).
	_ = origPath
}

// ---------------------------------------------------------------------------
// Integration-style: full flow
// ---------------------------------------------------------------------------

func TestIntegration_EnvToCommand(t *testing.T) {
	t.Setenv("CLAUDE_SESSION_NAME", "integration-test")

	name := getSessionName()
	require.Equal(t, "integration-test", name)

	cmd := buildAgmCommand(name, "READY", "sessionend-hook")
	assert.Equal(t, []string{"agm", "session", "state", "set",
		"integration-test", "READY", "--source", "sessionend-hook"}, cmd.Args)
}

func TestIntegration_RunWithSession(t *testing.T) {
	t.Setenv("CLAUDE_SESSION_NAME", "full-flow-test")
	// run() calls getSessionName -> buildAgmCommand -> cmd.Run
	// cmd.Run will fail (no agm binary) but error is ignored.
	assert.NotPanics(t, func() {
		result := run()
		assert.True(t, result)
	})
}
