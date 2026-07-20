package main

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/ops"
)

func testLaunchCommand(spec ops.HarnessLaunchSpec) string {
	spec.DisableOAuth = true
	return ops.BuildHarnessLaunchCommand(spec).Command
}

func TestStartAgyHarnessUsesCanonicalLaunchAndWaits(t *testing.T) {
	var sentCommand string
	var waitedSession string
	var waitedTimeout time.Duration
	runtime := agyHarnessRuntime{
		lookPath: func(file string) (string, error) {
			if file != "agy" {
				t.Fatalf("lookPath(%q), want agy", file)
			}
			return "/fixture/agy", nil
		},
		sendCommand: func(sessionName, command string) error {
			if sessionName != "agy-production-seam" {
				t.Fatalf("send session = %q", sessionName)
			}
			sentCommand = command
			return nil
		},
		waitForPrompt: func(sessionName string, timeout time.Duration) error {
			waitedSession, waitedTimeout = sessionName, timeout
			return nil
		},
		sleep: func(time.Duration) {},
	}

	modeApplied, err := startAgyHarnessWithRuntime(ops.HarnessLaunchSpec{
		Harness: "agy", Model: "3.5-flash-low", SessionName: "agy-production-seam",
		WorkDir: "/tmp/agy work", ExtraAddDirs: []string{"/tmp/extra dir"}, PermissionMode: "auto",
	}, runtime)
	if err != nil {
		t.Fatalf("startAgyHarnessWithRuntime: %v", err)
	}
	if !modeApplied {
		t.Fatal("auto permission mode was not reported as applied")
	}
	for _, want := range []string{
		"cd '/tmp/agy work' && agy --model 'Gemini 3.5 Flash (Low)'",
		"--dangerously-skip-permissions",
		"--add-dir '/tmp/extra dir'",
		"&& exit",
	} {
		if !strings.Contains(sentCommand, want) {
			t.Errorf("launch command %q missing %q", sentCommand, want)
		}
	}
	if strings.Contains(sentCommand, "--prompt-interactive") {
		t.Errorf("launch used string-valued prompt flag without a prompt: %q", sentCommand)
	}
	if waitedSession != "agy-production-seam" || waitedTimeout != 90*time.Second {
		t.Fatalf("readiness wait = (%q, %s), want (agy-production-seam, 90s)", waitedSession, waitedTimeout)
	}
}

func TestStartAgyHarnessPropagatesReadinessFailure(t *testing.T) {
	wantErr := errors.New("fixture AGY never became ready")
	runtime := agyHarnessRuntime{
		lookPath:      func(string) (string, error) { return "/fixture/agy", nil },
		sendCommand:   func(string, string) error { return nil },
		waitForPrompt: func(string, time.Duration) error { return wantErr },
		sleep:         func(time.Duration) {},
	}
	_, err := startAgyHarnessWithRuntime(ops.HarnessLaunchSpec{
		Harness: "agy", Model: "3.5-flash", SessionName: "agy-not-ready", WorkDir: "/tmp",
	}, runtime)
	if !errors.Is(err, wantErr) {
		t.Fatalf("readiness error = %v, want %v", err, wantErr)
	}
}

// TestBuildCodexCommand_ModelResolved verifies that a registry alias is resolved
// to its full Codex model name and passed via the `-m` flag.
func TestBuildCodexCommand_ModelResolved(t *testing.T) {
	cmd := testLaunchCommand(ops.HarnessLaunchSpec{
		Harness: "codex-cli", Model: "5.4", SessionName: "test-session", WorkDir: "/tmp/work",
	})

	if !strings.Contains(cmd, "-m 'gpt-5.4'") {
		t.Errorf("resolved model not present in command: %s", cmd)
	}
	// Cross-harness alias should resolve too (opus -> 5.5 -> gpt-5.5).
	cmd = testLaunchCommand(ops.HarnessLaunchSpec{
		Harness: "codex-cli", Model: "opus", SessionName: "test-session", WorkDir: "/tmp/work",
	})
	if !strings.Contains(cmd, "-m 'gpt-5.5'") {
		t.Errorf("cross-harness alias not resolved in command: %s", cmd)
	}
}

// TestBuildCodexCommand_ShellQuoting verifies that a working directory containing
// shell-special characters is single-quoted so it cannot break out of the command
// line or trigger word-splitting/globbing when pasted into the pane's shell.
func TestBuildCodexCommand_ShellQuoting(t *testing.T) {
	// A path with a space and a single quote (injection attempt).
	workDir := "/tmp/work dir/'; rm -rf ~ #"
	cmd := testLaunchCommand(ops.HarnessLaunchSpec{
		Harness: "codex-cli", Model: "5.4", SessionName: "test-session", WorkDir: workDir,
		ExtraAddDirs: []string{"/extra/add dir"},
	})

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
	cmd = testLaunchCommand(ops.HarnessLaunchSpec{
		Harness: "codex-cli", Model: "5.4", SessionName: "my session", WorkDir: workDir,
	})
	if !strings.Contains(cmd, "AGM_SESSION_NAME='my session'") {
		t.Errorf("session name not shell-quoted: %s", cmd)
	}
}

// TestBuildCodexCommand_NoClaudeEnvLeak is the security regression for the design
// invariant: the Codex command must never carry Claude OAuth, Anthropic keys, or
// engram/OTEL telemetry env. Codex authenticates via ~/.codex / OPENAI_API_KEY.
func TestBuildCodexCommand_NoClaudeEnvLeak(t *testing.T) {
	cmd := testLaunchCommand(ops.HarnessLaunchSpec{
		Harness: "codex-cli", Model: "5.4", SessionName: "test-session",
		SessionID: "engram-uuid-1234", WorkDir: "/tmp/work", ForwardTelemetry: true,
	})

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
	cmd := testLaunchCommand(ops.HarnessLaunchSpec{
		Harness: "codex-cli", Model: "5.4", SessionName: "codex-session", WorkDir: "/tmp/work",
		ExtraAddDirs: []string{"/tmp/extra dir"}, Codex: &manifest.Codex{SessionID: "thr_123"},
	})

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
	cmd := testLaunchCommand(ops.HarnessLaunchSpec{
		Harness: "agy", Model: "3.5-flash-low", SessionName: "agy", WorkDir: "/tmp/agy work",
		ExtraAddDirs: []string{"/tmp/extra dir"}, PermissionMode: "auto",
	})

	for _, want := range []string{
		"cd '/tmp/agy work' && agy --model 'Gemini 3.5 Flash (Low)' --dangerously-skip-permissions",
		"--add-dir '/tmp/extra dir'",
		"&& exit",
	} {
		if !strings.Contains(cmd, want) {
			t.Errorf("command %q missing %q", cmd, want)
		}
	}
}

func TestBuildAgyCommand_DefaultPermissionMode(t *testing.T) {
	cmd := testLaunchCommand(ops.HarnessLaunchSpec{
		Harness: "agy", Model: "3.5-flash", SessionName: "agy", WorkDir: "/tmp/agy-work",
		PermissionMode: "default",
	})

	if strings.Contains(cmd, "--dangerously-skip-permissions") {
		t.Errorf("default AGY command should not skip permissions: %q", cmd)
	}
	if !strings.Contains(cmd, "cd '/tmp/agy-work' && agy --model 'Gemini 3.5 Flash (Medium)' && exit") {
		t.Errorf("unexpected default AGY launch command: %q", cmd)
	}
	if strings.Contains(cmd, "--prompt-interactive") {
		t.Errorf("bare AGY lifecycle must not use the string-valued prompt flag: %q", cmd)
	}
}

func TestActiveHarnessBuildersHonorPersistentStartupContracts(t *testing.T) {
	tests := []struct {
		name string
		cmd  string
		want string
	}{
		{name: "Codex", cmd: testLaunchCommand(ops.HarnessLaunchSpec{Harness: "codex-cli", Model: "5.4", SessionName: "worker", WorkDir: "/tmp/work", Persistent: true, PermissionMode: "auto"}), want: "-a never"},
		{name: "AGY", cmd: testLaunchCommand(ops.HarnessLaunchSpec{Harness: "agy", Model: "3.5-flash", SessionName: "worker", WorkDir: "/tmp/work", Persistent: true, PermissionMode: "auto"}), want: "agy --model 'Gemini 3.5 Flash (Medium)' --dangerously-skip-permissions"},
		{name: "OpenCode", cmd: testLaunchCommand(ops.HarnessLaunchSpec{Harness: "opencode-cli", Model: "glm-5.2", SessionName: "worker", WorkDir: "/tmp/work", Persistent: true}), want: "opencode attach"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !strings.Contains(tt.cmd, tt.want) {
				t.Fatalf("command %q missing startup contract %q", tt.cmd, tt.want)
			}
			if strings.Contains(tt.cmd, "&& exit") {
				t.Fatalf("persistent command exits pane shell: %q", tt.cmd)
			}
		})
	}
}
