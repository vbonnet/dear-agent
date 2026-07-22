package harnessexec

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPreparedClaudeCommandCarriesCallerOnlyOAuthAndTelemetry(t *testing.T) {
	stateDir := t.TempDir()
	t.Setenv("AGM_STATE_DIR", stateDir)
	t.Setenv("ANTHROPIC_API_KEY", "stale-pane-api-key")
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "stale-pane-endpoint")

	originalExecutablePath := executablePath
	originalLookPath := lookPath
	originalReplaceProcess := replaceProcess
	originalResolveClaudeOAuth := resolveClaudeOAuth
	t.Cleanup(func() {
		executablePath = originalExecutablePath
		lookPath = originalLookPath
		replaceProcess = originalReplaceProcess
		resolveClaudeOAuth = originalResolveClaudeOAuth
	})
	executablePath = func() (string, error) { return "/opt/agm/bin/agm", nil }
	resolveClaudeOAuth = func() string { return "caller-only-oauth" }

	prepared, err := PrepareClaudeCommand(ClaudeLaunch{
		SessionName: "handoff-claude", Model: "claude-test", AddDirs: []string{"/tmp/work"},
		ForwardTelemetry: true,
	}, []string{
		"ANTHROPIC_API_KEY=caller-api-key",
		"OTEL_EXPORTER_OTLP_ENDPOINT=caller-endpoint",
		"OTEL_EXPORTER_OTLP_HEADERS=authorization=caller-telemetry",
	})
	if err != nil {
		t.Fatalf("prepare Claude command: %v", err)
	}
	for _, secret := range []string{"caller-only-oauth", "caller-api-key", "caller-endpoint", "caller-telemetry"} {
		if strings.Contains(prepared.Command, secret) {
			t.Fatalf("prepared command exposed %q: %s", secret, prepared.Command)
		}
	}
	if !strings.HasPrefix(prepared.Command, "'/opt/agm/bin/agm' "+ClaudeProtocol) {
		t.Fatalf("prepared command did not pin current AGM executable: %s", prepared.Command)
	}
	assertPrivateHandoffMode(t, prepared.path)

	var childEnvironment []string
	lookPath = func(string) (string, error) { return "/fixed/claude", nil }
	replaceProcess = func(_ string, _ []string, env []string) error {
		childEnvironment = append([]string(nil), env...)
		return nil
	}
	resolveClaudeOAuth = func() string { return "" }
	err = Run(ClaudeProtocol, []string{
		"--handoff", prepared.path,
		"--session", "handoff-claude", "--model", "claude-test", "--add-dir", "/tmp/work",
		"--forward-telemetry",
	})
	if err == nil || !strings.Contains(err.Error(), "returned unexpectedly") {
		t.Fatalf("run prepared Claude command: %v", err)
	}
	values := environmentMap(childEnvironment)
	for name, want := range map[string]string{
		"CLAUDE_CODE_OAUTH_TOKEN":     "caller-only-oauth",
		"OTEL_EXPORTER_OTLP_ENDPOINT": "caller-endpoint",
		"OTEL_EXPORTER_OTLP_HEADERS":  "authorization=caller-telemetry",
	} {
		if got := values[name]; got != want {
			t.Errorf("Claude child %s = %q, want %q", name, got, want)
		}
	}
	if _, ok := values["ANTHROPIC_API_KEY"]; ok {
		t.Fatal("Claude child retained stale Anthropic API key beside caller OAuth")
	}
	if _, err := os.Stat(prepared.path); !os.IsNotExist(err) {
		t.Fatalf("consumed Claude handoff still exists: %v", err)
	}
}

func TestPreparedCodexCommandCarriesCallerAllowlistAndPreservesPaneIdentity(t *testing.T) {
	t.Setenv("AGM_STATE_DIR", t.TempDir())
	t.Setenv("OPENAI_API_KEY", "stale-pane-openai")
	t.Setenv("CODEX_ACCESS_TOKEN", "stale-pane-codex")
	t.Setenv("TMUX", "live-pane-tmux")
	t.Setenv("TMUX_PANE", "%9")

	originalExecutablePath := executablePath
	originalLookPath := lookPath
	originalReplaceProcess := replaceProcess
	t.Cleanup(func() {
		executablePath = originalExecutablePath
		lookPath = originalLookPath
		replaceProcess = originalReplaceProcess
	})
	executablePath = func() (string, error) { return "/opt/agm/bin/agm", nil }

	prepared, err := PrepareCodexCommand(CodexLaunch{
		SessionName: "handoff-codex", Model: "gpt-test", WorkDir: "/tmp/work", Sandbox: "workspace-write",
	}, []string{
		"PATH=/caller/bin", "HOME=/caller/home", "TMUX=stale-caller-tmux", "TMUX_PANE=%1",
		"OPENAI_API_KEY=caller-openai", "CODEX_ACCESS_TOKEN=caller-codex",
		"ANTHROPIC_API_KEY=rejected-anthropic",
	})
	if err != nil {
		t.Fatalf("prepare Codex command: %v", err)
	}
	for _, secret := range []string{"caller-openai", "caller-codex", "rejected-anthropic"} {
		if strings.Contains(prepared.Command, secret) {
			t.Fatalf("prepared command exposed %q: %s", secret, prepared.Command)
		}
	}
	assertPrivateHandoffMode(t, prepared.path)

	var childEnvironment []string
	lookPath = func(string) (string, error) { return "/fixed/codex", nil }
	replaceProcess = func(_ string, _ []string, env []string) error {
		childEnvironment = append([]string(nil), env...)
		return nil
	}
	err = Run(CodexProtocol, []string{
		"--handoff", prepared.path,
		"--session", "handoff-codex", "--model", "gpt-test", "--workdir", "/tmp/work", "--sandbox", "workspace-write",
	})
	if err == nil || !strings.Contains(err.Error(), "returned unexpectedly") {
		t.Fatalf("run prepared Codex command: %v", err)
	}
	values := environmentMap(childEnvironment)
	for name, want := range map[string]string{
		"OPENAI_API_KEY":     "caller-openai",
		"CODEX_ACCESS_TOKEN": "caller-codex",
		"TMUX":               "live-pane-tmux",
		"TMUX_PANE":          "%9",
	} {
		if got := values[name]; got != want {
			t.Errorf("Codex child %s = %q, want %q", name, got, want)
		}
	}
	if _, ok := values["ANTHROPIC_API_KEY"]; ok {
		t.Fatal("Codex child inherited rejected Anthropic credential")
	}
	if got := values["CODEX_ACCESS_TOKEN"]; got != "caller-codex" {
		t.Fatalf("Codex child CODEX_ACCESS_TOKEN = %q, want caller snapshot", got)
	}
	if _, err := os.Stat(prepared.path); !os.IsNotExist(err) {
		t.Fatalf("consumed Codex handoff still exists: %v", err)
	}
}

func TestPreparedClaudeCommandClearsCallerAbsentPaneState(t *testing.T) {
	t.Setenv("AGM_STATE_DIR", t.TempDir())
	t.Setenv("CLAUDE_CODE_OAUTH_TOKEN", "stale-pane-oauth")
	t.Setenv("ANTHROPIC_API_KEY", "stale-pane-api")
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "stale-pane-endpoint")
	t.Setenv("OTEL_EXPORTER_OTLP_HEADERS", "stale-pane-headers")

	originalExecutablePath := executablePath
	originalLookPath := lookPath
	originalReplaceProcess := replaceProcess
	originalResolveClaudeOAuth := resolveClaudeOAuth
	t.Cleanup(func() {
		executablePath = originalExecutablePath
		lookPath = originalLookPath
		replaceProcess = originalReplaceProcess
		resolveClaudeOAuth = originalResolveClaudeOAuth
	})
	executablePath = func() (string, error) { return "/opt/agm/bin/agm", nil }
	resolveClaudeOAuth = func() string { return "" }
	prepared, err := PrepareClaudeCommand(ClaudeLaunch{
		SessionName: "clear-stale", DisableOAuth: true,
	}, nil)
	if err != nil {
		t.Fatalf("prepare Claude command: %v", err)
	}
	lookPath = func(string) (string, error) { return "/fixed/claude", nil }
	var childEnvironment []string
	replaceProcess = func(_ string, _ []string, env []string) error {
		childEnvironment = append([]string(nil), env...)
		return nil
	}
	err = Run(ClaudeProtocol, []string{
		"--handoff", prepared.path, "--session", "clear-stale", "--disable-oauth",
	})
	if err == nil || !strings.Contains(err.Error(), "returned unexpectedly") {
		t.Fatalf("run prepared Claude command: %v", err)
	}
	values := environmentMap(childEnvironment)
	for _, name := range []string{
		"CLAUDE_CODE_OAUTH_TOKEN", "ANTHROPIC_API_KEY",
		"OTEL_EXPORTER_OTLP_ENDPOINT", "OTEL_EXPORTER_OTLP_HEADERS",
	} {
		if value, ok := values[name]; ok {
			t.Errorf("Claude child retained caller-absent %s=%q from stale pane", name, value)
		}
	}
}

func TestPreparedCodexCommandClearsCallerAbsentPaneCredentials(t *testing.T) {
	t.Setenv("AGM_STATE_DIR", t.TempDir())
	t.Setenv("OPENAI_API_KEY", "stale-pane-openai")
	t.Setenv("CODEX_ACCESS_TOKEN", "stale-pane-codex")

	originalExecutablePath := executablePath
	originalLookPath := lookPath
	originalReplaceProcess := replaceProcess
	t.Cleanup(func() {
		executablePath = originalExecutablePath
		lookPath = originalLookPath
		replaceProcess = originalReplaceProcess
	})
	executablePath = func() (string, error) { return "/opt/agm/bin/agm", nil }
	prepared, err := PrepareCodexCommand(CodexLaunch{
		SessionName: "clear-stale", Model: "gpt-test", WorkDir: "/tmp/work", Sandbox: "workspace-write",
	}, nil)
	if err != nil {
		t.Fatalf("prepare Codex command: %v", err)
	}
	lookPath = func(string) (string, error) { return "/fixed/codex", nil }
	var childEnvironment []string
	replaceProcess = func(_ string, _ []string, env []string) error {
		childEnvironment = append([]string(nil), env...)
		return nil
	}
	err = Run(CodexProtocol, []string{
		"--handoff", prepared.path,
		"--session", "clear-stale", "--model", "gpt-test", "--workdir", "/tmp/work", "--sandbox", "workspace-write",
	})
	if err == nil || !strings.Contains(err.Error(), "returned unexpectedly") {
		t.Fatalf("run prepared Codex command: %v", err)
	}
	values := environmentMap(childEnvironment)
	for _, name := range []string{"OPENAI_API_KEY", "CODEX_ACCESS_TOKEN"} {
		if value, ok := values[name]; ok {
			t.Errorf("Codex child retained caller-absent %s=%q from stale pane", name, value)
		}
	}
}

func TestPreparedCommandCancelRemovesUndeliveredHandoff(t *testing.T) {
	t.Setenv("AGM_STATE_DIR", t.TempDir())
	originalExecutablePath := executablePath
	t.Cleanup(func() { executablePath = originalExecutablePath })
	executablePath = func() (string, error) { return "/opt/agm/bin/agm", nil }

	prepared, err := PrepareCodexCommand(CodexLaunch{
		SessionName: "cancel-codex", Model: "gpt-test", WorkDir: "/tmp/work", Sandbox: "workspace-write",
	}, []string{"OPENAI_API_KEY=cancel-canary"})
	if err != nil {
		t.Fatalf("prepare Codex command: %v", err)
	}
	if err := prepared.Cancel(); err != nil {
		t.Fatalf("cancel handoff: %v", err)
	}
	if err := prepared.Cancel(); err != nil {
		t.Fatalf("cancel handoff twice: %v", err)
	}
	if _, err := os.Stat(prepared.path); !os.IsNotExist(err) {
		t.Fatalf("cancelled handoff still exists: %v", err)
	}
}

func TestPreparedCommandUsesCoInstalledAGMFromCompanionBinary(t *testing.T) {
	t.Setenv("AGM_STATE_DIR", t.TempDir())
	binDir := t.TempDir()
	agmPath := filepath.Join(binDir, "agm")
	if err := os.WriteFile(agmPath, []byte("test executable"), 0700); err != nil {
		t.Fatalf("write co-installed AGM: %v", err)
	}
	originalExecutablePath := executablePath
	t.Cleanup(func() { executablePath = originalExecutablePath })
	executablePath = func() (string, error) { return filepath.Join(binDir, "agm-mcp-server"), nil }

	prepared, err := PrepareCodexCommand(CodexLaunch{
		SessionName: "companion-codex", Model: "gpt-test", WorkDir: "/tmp/work", Sandbox: "workspace-write",
	}, nil)
	if err != nil {
		t.Fatalf("prepare Codex command from companion: %v", err)
	}
	t.Cleanup(func() { _ = prepared.Cancel() })
	if !strings.HasPrefix(prepared.Command, shellQuote(agmPath)+" "+CodexProtocol) {
		t.Fatalf("companion command did not pin co-installed AGM: %s", prepared.Command)
	}
}

func TestPreparedCommandUsesRenamedCurrentAGMExecutable(t *testing.T) {
	t.Setenv("AGM_STATE_DIR", t.TempDir())
	originalExecutablePath := executablePath
	t.Cleanup(func() { executablePath = originalExecutablePath })
	executablePath = func() (string, error) { return "/opt/agm/bin/agm-v2026.07", nil }

	prepared, err := PrepareCodexCommand(CodexLaunch{
		SessionName: "renamed-codex", Model: "gpt-test", WorkDir: "/tmp/work", Sandbox: "workspace-write",
	}, nil)
	if err != nil {
		t.Fatalf("prepare Codex command from renamed AGM: %v", err)
	}
	t.Cleanup(func() { _ = prepared.Cancel() })
	if !strings.HasPrefix(prepared.Command, "'/opt/agm/bin/agm-v2026.07' "+CodexProtocol) {
		t.Fatalf("renamed AGM command did not pin current executable: %s", prepared.Command)
	}
}

func TestPreparedCommandMakesRelativeStateDirectoryAbsolute(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("AGM_STATE_DIR", "relative-state")
	originalExecutablePath := executablePath
	t.Cleanup(func() { executablePath = originalExecutablePath })
	executablePath = func() (string, error) { return "/opt/agm/bin/agm", nil }

	prepared, err := PrepareCodexCommand(CodexLaunch{
		SessionName: "absolute-handoff", Model: "gpt-test", WorkDir: "/tmp/work", Sandbox: "workspace-write",
	}, nil)
	if err != nil {
		t.Fatalf("prepare Codex command with relative state directory: %v", err)
	}
	t.Cleanup(func() { _ = prepared.Cancel() })
	if !filepath.IsAbs(prepared.path) {
		t.Fatalf("handoff path = %q, want absolute", prepared.path)
	}
	if !strings.Contains(prepared.Command, "--handoff "+shellQuote(prepared.path)) {
		t.Fatalf("prepared command omitted absolute handoff path: %s", prepared.Command)
	}
}

func TestExecutorConsumesHandoffBeforeHarnessLookup(t *testing.T) {
	t.Setenv("AGM_STATE_DIR", t.TempDir())
	originalExecutablePath := executablePath
	originalLookPath := lookPath
	t.Cleanup(func() {
		executablePath = originalExecutablePath
		lookPath = originalLookPath
	})
	executablePath = func() (string, error) { return "/opt/agm/bin/agm", nil }
	lookPath = func(string) (string, error) { return "", os.ErrNotExist }

	prepared, err := PrepareCodexCommand(CodexLaunch{
		SessionName: "missing-codex", Model: "gpt-test", WorkDir: "/tmp/work", Sandbox: "workspace-write",
	}, nil)
	if err != nil {
		t.Fatalf("prepare Codex command: %v", err)
	}
	err = Run(CodexProtocol, []string{
		"--handoff", prepared.path,
		"--session", "missing-codex", "--model", "gpt-test", "--workdir", "/tmp/work", "--sandbox", "workspace-write",
	})
	if err == nil || !strings.Contains(err.Error(), "resolve codex executable") {
		t.Fatalf("run with missing Codex executable: %v", err)
	}
	if _, err := os.Stat(prepared.path); !os.IsNotExist(err) {
		t.Fatalf("handoff survived failed harness lookup: %v", err)
	}
}

func TestConsumeHandoffRejectsCrossHarnessAndPublicState(t *testing.T) {
	t.Setenv("AGM_STATE_DIR", t.TempDir())

	if _, err := stageHandoff(CodexProtocol, []string{"ANTHROPIC_API_KEY=must-not-cross"}); err == nil {
		t.Fatal("Codex staging accepted an Anthropic credential")
	}

	wrongProtocol, err := stageHandoff(ClaudeProtocol, nil)
	if err != nil {
		t.Fatalf("stage wrong-protocol handoff: %v", err)
	}
	if _, err := consumeHandoff(wrongProtocol, CodexProtocol); err == nil {
		t.Fatal("Codex executor accepted a Claude handoff")
	}
	if err := os.Remove(wrongProtocol); err != nil {
		t.Fatalf("remove rejected wrong-protocol handoff: %v", err)
	}

	public, err := stageHandoff(CodexProtocol, nil)
	if err != nil {
		t.Fatalf("stage public-mode handoff: %v", err)
	}
	if err := os.Chmod(public, 0644); err != nil {
		t.Fatalf("make handoff public: %v", err)
	}
	if _, err := consumeHandoff(public, CodexProtocol); err == nil {
		t.Fatal("executor accepted a group/world-readable handoff")
	}
	if err := os.Remove(public); err != nil {
		t.Fatalf("remove rejected public handoff: %v", err)
	}
}

func TestConsumeHandoffRejectsTrailingAndOversizedContent(t *testing.T) {
	t.Setenv("AGM_STATE_DIR", t.TempDir())

	for name, suffix := range map[string]string{
		"trailing JSON": "{}",
		"oversized":     strings.Repeat(" ", handoffMaxSize),
	} {
		t.Run(name, func(t *testing.T) {
			path, err := stageHandoff(CodexProtocol, nil)
			if err != nil {
				t.Fatalf("stage handoff: %v", err)
			}
			file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0600)
			if err != nil {
				t.Fatalf("open handoff for corruption: %v", err)
			}
			if _, err := file.WriteString(suffix); err != nil {
				_ = file.Close()
				t.Fatalf("corrupt handoff: %v", err)
			}
			if err := file.Close(); err != nil {
				t.Fatalf("close corrupted handoff: %v", err)
			}
			if _, err := consumeHandoff(path, CodexProtocol); err == nil {
				t.Fatal("executor accepted a corrupted handoff")
			}
			if err := os.Remove(path); err != nil {
				t.Fatalf("remove rejected handoff: %v", err)
			}
		})
	}
}

func assertPrivateHandoffMode(t *testing.T, path string) {
	t.Helper()
	fileInfo, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat handoff: %v", err)
	}
	if got := fileInfo.Mode().Perm(); got != 0600 {
		t.Fatalf("handoff mode = %o, want 600", got)
	}
	dirInfo, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatalf("stat handoff directory: %v", err)
	}
	if got := dirInfo.Mode().Perm(); got != 0700 {
		t.Fatalf("handoff directory mode = %o, want 700", got)
	}
}
