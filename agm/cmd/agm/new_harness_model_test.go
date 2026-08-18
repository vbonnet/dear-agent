package main

import (
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/agm/internal/agent"
	"github.com/vbonnet/dear-agent/agm/internal/ops"
)

// TestBuildClaudeCommand_BracketedModelQuoted is the regression test for ce-rpet:
// when the resolved model name contains glob metacharacters like [1m], the
// resulting shell command must quote the value so zsh does not expand it as a
// character-class glob. Prior to the fix, `claude --model claude-sonnet-4-6[1m]`
// would fail with "zsh: no matches found: claude-sonnet-4-6[1m]".
func TestBuildClaudeCommand_BracketedModelQuoted(t *testing.T) {
	resolvedModel := agent.ResolveModelFullName("claude-code", "sonnet")
	cmd := testLaunchCommand(ops.HarnessLaunchSpec{
		Harness: "claude-code", Model: "sonnet", SessionName: "test-session", WorkDir: "/tmp/work",
	})

	// The [1m] suffix must be inside single quotes so the shell never sees bare [1m].
	if !strings.Contains(cmd, "--model '"+resolvedModel+"'") {
		t.Errorf("bracketed model not shell-quoted in command; zsh would glob [1m]: %s", cmd)
	}

	// Negative: the unquoted form must not appear.
	if strings.Contains(cmd, "--model "+resolvedModel) {
		t.Errorf("unquoted bracketed model in command would cause zsh glob failure: %s", cmd)
	}
}

// TestBuildClaudeCommand_SessionNameQuoted verifies that a session name containing
// shell-special characters is quoted in the private protocol argument.
func TestBuildClaudeCommand_SessionNameQuoted(t *testing.T) {
	cmd := testLaunchCommand(ops.HarnessLaunchSpec{
		Harness: "claude-code", Model: "sonnet", SessionName: "my-session", WorkDir: "/tmp/work",
	})
	if !strings.Contains(cmd, "--session 'my-session'") {
		t.Errorf("session name not quoted in command: %s", cmd)
	}
}

// TestBuildClaudeCommand_PersistentOmitsExit verifies that --persistent omits
// "&& exit" from the harness command so supervisor sessions survive their
// Claude turn/loop ending without dropping to a bare shell (ce-pzca).
func TestBuildClaudeCommand_PersistentOmitsExit(t *testing.T) {
	cmd := testLaunchCommand(ops.HarnessLaunchSpec{
		Harness: "claude-code", Model: "sonnet", SessionName: "sup-session", WorkDir: "/tmp/work", Persistent: true,
	})
	if strings.Contains(cmd, "&& exit") {
		t.Errorf("persistent=true: command still has '&& exit': %s", cmd)
	}
	// Must still contain all other required parts.
	for _, want := range []string{"agm __exec-claude", "--session 'sup-session'", "--add-dir '/tmp/work'"} {
		if !strings.Contains(cmd, want) {
			t.Errorf("persistent=true: command missing %q: %s", want, cmd)
		}
	}
}

// TestBuildClaudeCommand_NonPersistentHasExit verifies that non-persistent
// (default) sessions keep the "&& exit" suffix for clean teardown.
func TestBuildClaudeCommand_NonPersistentHasExit(t *testing.T) {
	cmd := testLaunchCommand(ops.HarnessLaunchSpec{
		Harness: "claude-code", Model: "sonnet", SessionName: "worker-session", WorkDir: "/tmp/work",
	})
	if !strings.Contains(cmd, "&& exit") {
		t.Errorf("persistent=false: command missing '&& exit': %s", cmd)
	}
}
