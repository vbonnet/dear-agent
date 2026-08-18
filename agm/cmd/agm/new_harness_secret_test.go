package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/agm/internal/debug"
	"github.com/vbonnet/dear-agent/agm/internal/ops"
)

func TestHarnessLaunchDebugLogContainsNoCredentialValues(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("OPENAI_API_KEY", "openai-debug-canary")
	t.Setenv("CODEX_ACCESS_TOKEN", "codex-debug-canary")
	t.Setenv("CLAUDE_CODE_OAUTH_TOKEN", "claude-debug-canary")
	t.Setenv("ANTHROPIC_API_KEY", "anthropic-debug-canary")

	if err := debug.Init(true, "credential-canary"); err != nil {
		t.Fatalf("initialize debug logger: %v", err)
	}
	for _, spec := range []ops.HarnessLaunchSpec{
		{Harness: "codex-cli", Model: "5.4", SessionName: "codex", WorkDir: "/tmp/work"},
		{Harness: "claude-code", Model: "sonnet", SessionName: "claude", WorkDir: "/tmp/work"},
	} {
		debug.Log("Sending command: %s", ops.BuildHarnessLaunchCommand(spec).Command)
	}
	debug.Close()

	logs, err := filepath.Glob(filepath.Join(home, ".agm", "debug", "*.log"))
	if err != nil || len(logs) != 1 {
		t.Fatalf("debug logs = %v, error = %v, want one", logs, err)
	}
	content, err := os.ReadFile(logs[0])
	if err != nil {
		t.Fatalf("read debug log: %v", err)
	}
	for _, canary := range []string{
		"openai-debug-canary", "codex-debug-canary",
		"claude-debug-canary", "anthropic-debug-canary",
	} {
		if strings.Contains(string(content), canary) {
			t.Errorf("debug log exposed credential canary %q", canary)
		}
	}
}
