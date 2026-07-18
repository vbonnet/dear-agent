package main

import (
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/agm/internal/ops"
)

// TestClaudeEnvUnsetFlags verifies the ce-84l2 fix: when an OAuth (Max-plan)
// token is injected into a spawned session, ANTHROPIC_API_KEY must also be
// unset so a stray metered key inherited from the long-lived tmux server
// environment cannot shadow the OAuth token and route the session through the
// metered API. CLAUDECODE is always unset regardless.
func TestClaudeEnvUnsetFlags(t *testing.T) {
	withOAuth := ops.BuildHarnessLaunchCommand(ops.HarnessLaunchSpec{
		Harness: "claude-code", Model: "sonnet", SessionName: "oauth", WorkDir: "/tmp/work",
		OAuthToken: "test-oauth-token",
	}).Command
	if !strings.Contains(withOAuth, "-u CLAUDECODE") {
		t.Errorf("with OAuth: %q missing -u CLAUDECODE", withOAuth)
	}
	if !strings.Contains(withOAuth, "-u ANTHROPIC_API_KEY") {
		t.Errorf("with OAuth: %q must unset ANTHROPIC_API_KEY so the metered key can't shadow OAuth", withOAuth)
	}

	withoutOAuth := ops.BuildHarnessLaunchCommand(ops.HarnessLaunchSpec{
		Harness: "claude-code", Model: "sonnet", SessionName: "no-oauth", WorkDir: "/tmp/work",
		DisableOAuth: true,
	}).Command
	if !strings.Contains(withoutOAuth, "-u CLAUDECODE") {
		t.Errorf("without OAuth: %q missing -u CLAUDECODE", withoutOAuth)
	}
	if strings.Contains(withoutOAuth, "ANTHROPIC_API_KEY") {
		t.Errorf("without OAuth: %q must not unset ANTHROPIC_API_KEY (no OAuth token to protect)", withoutOAuth)
	}
}
