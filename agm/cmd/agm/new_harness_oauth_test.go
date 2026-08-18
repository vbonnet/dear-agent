package main

import (
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/agm/internal/ops"
)

// TestClaudePrivateExecutorProtocol verifies that OAuth is resolved only after
// the token-free tmux command reaches AGM's private executor.
func TestClaudePrivateExecutorProtocol(t *testing.T) {
	t.Setenv("CLAUDE_CODE_OAUTH_TOKEN", "oauth-command-canary")
	t.Setenv("ANTHROPIC_API_KEY", "anthropic-command-canary")
	withOAuth := ops.BuildHarnessLaunchCommand(ops.HarnessLaunchSpec{
		Harness: "claude-code", Model: "sonnet", SessionName: "oauth", WorkDir: "/tmp/work",
	}).Command
	if !strings.Contains(withOAuth, "agm __exec-claude") {
		t.Errorf("with OAuth: %q missing private executor", withOAuth)
	}
	for _, secret := range []string{"oauth-command-canary", "anthropic-command-canary"} {
		if strings.Contains(withOAuth, secret) {
			t.Errorf("with OAuth: command exposed %q: %q", secret, withOAuth)
		}
	}

	withoutOAuth := ops.BuildHarnessLaunchCommand(ops.HarnessLaunchSpec{
		Harness: "claude-code", Model: "sonnet", SessionName: "no-oauth", WorkDir: "/tmp/work",
		DisableOAuth: true,
	}).Command
	if !strings.Contains(withoutOAuth, "--disable-oauth") {
		t.Errorf("without OAuth: %q missing private disable-oauth instruction", withoutOAuth)
	}
}
