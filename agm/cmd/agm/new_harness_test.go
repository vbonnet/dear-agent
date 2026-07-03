package main

import (
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/agm/internal/manifest"
)

// TestBuildCodexCommand_ModelResolved verifies that a registry alias is resolved
// to its full Codex model name and passed via the `-m` flag.
func TestBuildCodexCommand_ModelResolved(t *testing.T) {
	saved := modelName
	defer func() { modelName = saved }()
	modelName = "5.4" // resolves to gpt-5.4

	cmd := buildCodexCommand("test-session", "/tmp/work", nil)

	if !strings.Contains(cmd, "-m 'gpt-5.4'") {
		t.Errorf("resolved model not present in command: %s", cmd)
	}
	// Cross-harness alias should resolve too (opus -> 5.5 -> gpt-5.5).
	modelName = "opus"
	cmd = buildCodexCommand("test-session", "/tmp/work", nil)
	if !strings.Contains(cmd, "-m 'gpt-5.5'") {
		t.Errorf("cross-harness alias not resolved in command: %s", cmd)
	}
}

// TestBuildCodexCommand_ShellQuoting verifies that a working directory containing
// shell-special characters is single-quoted so it cannot break out of the command
// line or trigger word-splitting/globbing when pasted into the pane's shell.
func TestBuildCodexCommand_ShellQuoting(t *testing.T) {
	saved := modelName
	defer func() { modelName = saved }()
	modelName = "5.4"

	// A path with a space and a single quote (injection attempt).
	workDir := "/tmp/work dir/'; rm -rf ~ #"
	cmd := buildCodexCommand("test-session", workDir, []string{"/extra/add dir"})

	// The -C value must be wrapped in single quotes with the embedded quote escaped.
	if !strings.Contains(cmd, "-C '/tmp/work dir/'\"'\"'; rm -rf ~ #'") {
		t.Errorf("workdir not safely shell-quoted: %s", cmd)
	}
	// The raw unquoted injection must NOT appear as a bare token.
	if strings.Contains(cmd, "-C /tmp/work dir") {
		t.Errorf("unquoted workdir would allow shell word-splitting/injection: %s", cmd)
	}
	// Extra add-dir must be quoted as well.
	if !strings.Contains(cmd, "--add-dir '/extra/add dir'") {
		t.Errorf("extra add-dir not shell-quoted: %s", cmd)
	}
	// Session name with special chars must be quoted in the env assignment.
	cmd = buildCodexCommand("my session", workDir, nil)
	if !strings.Contains(cmd, "AGM_SESSION_NAME='my session'") {
		t.Errorf("session name not shell-quoted: %s", cmd)
	}
}

// TestBuildCodexCommand_NoClaudeEnvLeak is the security regression for the design
// invariant: the Codex command must never carry Claude OAuth, Anthropic keys, or
// engram/OTEL telemetry env. Codex authenticates via ~/.codex / OPENAI_API_KEY.
func TestBuildCodexCommand_NoClaudeEnvLeak(t *testing.T) {
	saved := modelName
	savedSpawn := spawnSessionID
	defer func() {
		modelName = saved
		spawnSessionID = savedSpawn
	}()
	modelName = "5.4"
	// Even with a spawn session id set (which would inject ENGRAM_SESSION_ID into
	// the Claude command), it must not appear in the Codex command.
	spawnSessionID = "engram-uuid-1234"

	cmd := buildCodexCommand("test-session", "/tmp/work", nil)

	for _, banned := range []string{
		"CLAUDE_CODE_OAUTH_TOKEN",
		"ANTHROPIC_API_KEY",
		"ANTHROPIC_",
		"ENGRAM_SESSION_ID",
		"OTEL_EXPORTER_OTLP_ENDPOINT",
	} {
		if strings.Contains(cmd, banned) {
			t.Errorf("Codex command leaked Claude/engram env %q: %s", banned, cmd)
		}
	}

	// Only CLAUDECODE should be unset, and the default sandbox is workspace-write
	// (no silent full-autonomy bypass).
	if !strings.Contains(cmd, "env -u CLAUDECODE ") {
		t.Errorf("expected CLAUDECODE to be unset: %s", cmd)
	}
	if !strings.Contains(cmd, "-s workspace-write") {
		t.Errorf("expected default workspace-write sandbox: %s", cmd)
	}
	if strings.Contains(cmd, "dangerously-bypass") {
		t.Errorf("full-autonomy bypass must not be a silent default: %s", cmd)
	}
}

func TestBuildCodexCommand_RemoteThreadResume(t *testing.T) {
	cmd := buildCodexCommandForModel("codex-session", "/tmp/work", "5.4", []string{"/tmp/extra dir"}, &manifest.Codex{SessionID: "thr_123"})

	for _, want := range []string{
		"AGM_SESSION_NAME='codex-session'",
		"codex resume --remote unix://",
		"-m 'gpt-5.4'",
		"-C '/tmp/work'",
		"-s workspace-write",
		"--add-dir '/tmp/extra dir'",
		"'thr_123'",
	} {
		if !strings.Contains(cmd, want) {
			t.Errorf("remote Codex command missing %q: %s", want, cmd)
		}
	}
}

func TestBuildAgyCommand_AutoPermissionMode(t *testing.T) {
	cmd := buildAgyCommand("/tmp/agy work", []string{"/tmp/extra dir"}, "auto")

	for _, want := range []string{
		"cd '/tmp/agy work' && agy --dangerously-skip-permissions",
		"--add-dir '/tmp/extra dir'",
		"&& exit",
	} {
		if !strings.Contains(cmd, want) {
			t.Errorf("command %q missing %q", cmd, want)
		}
	}
}

func TestBuildAgyCommand_DefaultPermissionMode(t *testing.T) {
	cmd := buildAgyCommand("/tmp/agy-work", nil, "default")

	if strings.Contains(cmd, "--dangerously-skip-permissions") {
		t.Errorf("default AGY command should not skip permissions: %q", cmd)
	}
	if !strings.Contains(cmd, "cd '/tmp/agy-work' && agy && exit") {
		t.Errorf("unexpected default AGY launch command: %q", cmd)
	}
}
