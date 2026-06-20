package main

import (
	"strings"
	"testing"
)

// TestBuildClaudeCommand_BracketedModelQuoted is the regression test for ce-rpet:
// when the resolved model name contains glob metacharacters like [1m], the
// resulting shell command must quote the value so zsh does not expand it as a
// character-class glob. Prior to the fix, `claude --model claude-sonnet-4-6[1m]`
// would fail with "zsh: no matches found: claude-sonnet-4-6[1m]".
func TestBuildClaudeCommand_BracketedModelQuoted(t *testing.T) {
	saved := modelName
	defer func() { modelName = saved }()
	modelName = "sonnet" // resolves to claude-sonnet-4-6[1m]

	cmd, _ := buildClaudeCommand("test-session", "/tmp/work", nil)

	// The [1m] suffix must be inside single quotes so the shell never sees bare [1m].
	if !strings.Contains(cmd, "--model 'claude-sonnet-4-6[1m]'") {
		t.Errorf("bracketed model not shell-quoted in command; zsh would glob [1m]: %s", cmd)
	}

	// Negative: the unquoted form must not appear.
	if strings.Contains(cmd, "--model claude-sonnet-4-6[1m]") {
		t.Errorf("unquoted bracketed model in command would cause zsh glob failure: %s", cmd)
	}
}

// TestBuildClaudeCommand_SessionNameQuoted verifies that a session name containing
// shell-special characters is quoted in the AGM_SESSION_NAME assignment.
func TestBuildClaudeCommand_SessionNameQuoted(t *testing.T) {
	saved := modelName
	defer func() { modelName = saved }()
	modelName = "sonnet"

	cmd, _ := buildClaudeCommand("my-session", "/tmp/work", nil)
	if !strings.Contains(cmd, "AGM_SESSION_NAME='my-session'") {
		t.Errorf("session name not quoted in command: %s", cmd)
	}
}
