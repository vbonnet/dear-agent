package harnessexec

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/agm/internal/launchparity"
)

const helperMarker = "AGM_HARNESSEXEC_HELPER"

func TestPrivateExecutorHelper(t *testing.T) {
	if os.Getenv(helperMarker) != "1" {
		return
	}
	separator := -1
	for i, arg := range os.Args {
		if arg == "--" {
			separator = i
			break
		}
	}
	if separator < 0 || separator+1 >= len(os.Args) {
		fmt.Fprintln(os.Stderr, "private executor helper missing protocol")
		os.Exit(96)
	}
	if err := Run(os.Args[separator+1], os.Args[separator+2:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(97)
	}
	os.Exit(98)
}

func TestCodexExecutorFiltersEnvironmentAndPreservesArguments(t *testing.T) {
	envOutput, argvOutput := runExecutorCanary(t, "codex", CodexProtocol, []string{
		"--session", "codex-canary",
		"--model", "gpt-test",
		"--workdir", "/tmp/work dir",
		"--sandbox", "workspace-write",
		"--approval", "never",
		"--add-dir", "/tmp/extra",
	}, []string{
		"OPENAI_API_KEY=openai-canary-secret",
		"CODEX_ACCESS_TOKEN=codex-access-canary-secret",
		"CODEX_HOME=/tmp/codex-home",
		"CODEX_API_KEY=exec-only-canary-secret",
		"CLAUDE_CODE_OAUTH_TOKEN=claude-canary-secret",
		"ANTHROPIC_API_KEY=anthropic-canary-secret",
		"GITHUB_TOKEN=github-canary-secret",
		"GOOGLE_API_KEY=google-canary-secret",
		"ENGRAM_TOKEN=engram-canary-secret",
		"OTEL_EXPORTER_OTLP_HEADERS=otel-canary-secret",
		"SSH_AUTH_SOCK=/tmp/ssh-agent-canary",
		"ARBITRARY_SECRET=arbitrary-canary-secret",
	})

	for _, want := range []string{
		"AGM_SESSION_NAME=codex-canary",
		"OPENAI_API_KEY=openai-canary-secret",
		"CODEX_ACCESS_TOKEN=codex-access-canary-secret",
		"CODEX_HOME=/tmp/codex-home",
	} {
		if !strings.Contains(envOutput, want+"\n") {
			t.Errorf("Codex child environment missing %q:\n%s", want, envOutput)
		}
	}
	for _, banned := range []string{
		"exec-only-canary-secret",
		"claude-canary-secret",
		"anthropic-canary-secret",
		"github-canary-secret",
		"google-canary-secret",
		"engram-canary-secret",
		"otel-canary-secret",
		"ssh-agent-canary",
		"arbitrary-canary-secret",
	} {
		if strings.Contains(envOutput, banned) || strings.Contains(argvOutput, banned) {
			t.Errorf("Codex child leaked banned canary %q; env=%q argv=%q", banned, envOutput, argvOutput)
		}
	}
	wantArgs := []string{
		"-m", "gpt-test", "-C", "/tmp/work dir", "-s", "workspace-write",
		"--add-dir", "/tmp/extra", "-a", "never",
	}
	if got := outputLines(argvOutput); !reflect.DeepEqual(got, wantArgs) {
		t.Fatalf("Codex argv = %q, want %q", got, wantArgs)
	}
}

func TestClaudeExecutorPassesOAuthOnlyOutOfBand(t *testing.T) {
	envOutput, argvOutput := runExecutorCanary(t, "claude", ClaudeProtocol, []string{
		"--session", "claude-canary",
		"--model", "claude-test",
		"--add-dir", "/tmp/work",
		"--auto-mode",
		"--permission", "auto",
	}, []string{
		"CLAUDE_CODE_OAUTH_TOKEN=claude-oauth-canary-secret",
		"ANTHROPIC_API_KEY=anthropic-canary-secret",
		"CLAUDECODE=nested-canary",
	})

	if !strings.Contains(envOutput, "CLAUDE_CODE_OAUTH_TOKEN=claude-oauth-canary-secret\n") {
		t.Fatalf("Claude child did not receive OAuth through its environment:\n%s", envOutput)
	}
	for _, banned := range []string{"anthropic-canary-secret", "nested-canary"} {
		if strings.Contains(envOutput, banned) || strings.Contains(argvOutput, banned) {
			t.Errorf("Claude child leaked conflicting canary %q; env=%q argv=%q", banned, envOutput, argvOutput)
		}
	}
	if strings.Contains(argvOutput, "claude-oauth-canary-secret") {
		t.Fatalf("Claude OAuth appeared in argv: %q", argvOutput)
	}
	wantArgs := []string{"--model", "claude-test", "--add-dir", "/tmp/work", "--enable-auto-mode", "--permission-mode", "auto"}
	if got := outputLines(argvOutput); !reflect.DeepEqual(got, wantArgs) {
		t.Fatalf("Claude argv = %q, want %q", got, wantArgs)
	}
}

func TestBuildCommandsContainNoAmbientCredentialValues(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "openai-command-canary")
	t.Setenv("CODEX_ACCESS_TOKEN", "codex-command-canary")
	t.Setenv("CLAUDE_CODE_OAUTH_TOKEN", "claude-command-canary")
	t.Setenv("ANTHROPIC_API_KEY", "anthropic-command-canary")

	commands := []string{
		BuildCodexCommand(CodexLaunch{
			SessionName: "codex", Model: "gpt-test", WorkDir: "/tmp/work",
			Sandbox: "workspace-write",
		}),
		BuildClaudeCommand(ClaudeLaunch{
			SessionName: "claude", Model: "claude-test", AddDirs: []string{"/tmp/work"},
		}),
	}
	for _, command := range commands {
		for _, canary := range []string{
			"openai-command-canary", "codex-command-canary",
			"claude-command-canary", "anthropic-command-canary",
		} {
			if strings.Contains(command, canary) {
				t.Errorf("launch command exposed credential canary %q: %s", canary, command)
			}
		}
	}
}

func TestProtocolRecognition(t *testing.T) {
	for _, protocol := range []string{CodexProtocol, ClaudeProtocol, ExpiryProtocol} {
		if !IsProtocol(protocol) {
			t.Errorf("IsProtocol(%q) = false", protocol)
		}
	}
	if IsProtocol("new") || IsProtocol("") {
		t.Fatal("public or empty command was recognized as a private protocol")
	}
}

func TestCodexRequestReconstructsValidatedNativeArguments(t *testing.T) {
	request, err := parseCodex([]string{
		"--session", "session", "--model", "gpt-test", "--workdir", "/tmp/work",
		"--sandbox", "workspace-write", "--approval", "never",
		"--add-dir", "/tmp/one", "--add-dir", "/tmp/two",
		"--resume-id", "thread-123", "--remote",
	})
	if err != nil {
		t.Fatalf("parse Codex request: %v", err)
	}
	want := []string{
		"resume", "--remote", "unix://", "-m", "gpt-test", "-C", "/tmp/work",
		"-s", "workspace-write", "--add-dir", "/tmp/one", "--add-dir", "/tmp/two",
		"-a", "never", "thread-123",
	}
	if got := request.argv(); !reflect.DeepEqual(got, want) {
		t.Fatalf("Codex argv = %q, want %q", got, want)
	}

	local, err := parseCodex([]string{
		"--session", "session", "--model", "gpt-test", "--workdir", "/tmp/work",
		"--sandbox", "read-only", "--resume-id", "thread-456",
	})
	if err != nil {
		t.Fatalf("parse local Codex resume: %v", err)
	}
	if got, want := local.argv(), []string{
		"-m", "gpt-test", "-C", "/tmp/work", "-s", "read-only", "resume", "thread-456",
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("local Codex argv = %q, want %q", got, want)
	}
}

func TestClaudeRequestReconstructsValidatedNativeArguments(t *testing.T) {
	request, err := parseClaude([]string{
		"--session", "session", "--session-id", "session-id", "--model", "claude-test",
		"--add-dir", "/tmp/one", "--add-dir", "/tmp/two", "--auto-mode",
		"--permission", "plan", "--max-budget-usd", "12.50", "--forward-telemetry",
		"--resume-id", "native-claude-id", "--workdir", "/tmp/resume",
	})
	if err != nil {
		t.Fatalf("parse Claude request: %v", err)
	}
	want := []string{
		"--model", "claude-test", "--add-dir", "/tmp/one", "--add-dir", "/tmp/two",
		"--enable-auto-mode", "--permission-mode", "plan", "--max-budget-usd", "12.50",
		"--resume", "native-claude-id",
	}
	if got := request.argv(); !reflect.DeepEqual(got, want) {
		t.Fatalf("Claude argv = %q, want %q", got, want)
	}
	launch := request.launch()
	if launch.SessionName != "session" || launch.SessionID != "session-id" ||
		launch.ResumeID != "native-claude-id" || launch.WorkDir != "/tmp/resume" || !launch.ForwardTelemetry {
		t.Fatalf("Claude launch metadata not preserved: %+v", launch)
	}
}

func TestClaudeResumeChangesDirectoryBeforeDirectReplacement(t *testing.T) {
	originalLookPathInEnvironment := lookPathInEnvironment
	originalReplaceProcess := replaceProcess
	originalChangeDirectory := changeDirectory
	originalResolveClaudeOAuth := resolveClaudeOAuth
	t.Cleanup(func() {
		lookPathInEnvironment = originalLookPathInEnvironment
		replaceProcess = originalReplaceProcess
		changeDirectory = originalChangeDirectory
		resolveClaudeOAuth = originalResolveClaudeOAuth
	})
	lookPathInEnvironment = func(string, []string) (string, error) { return "/fixed/claude", nil }
	resolveClaudeOAuth = func() string { return "" }
	var changedTo string
	changeDirectory = func(path string) error {
		changedTo = path
		return nil
	}
	var gotArgv, gotEnv []string
	replaceProcess = func(_ string, argv, env []string) error {
		gotArgv = append([]string(nil), argv...)
		gotEnv = append([]string(nil), env...)
		return nil
	}

	err := Run(ClaudeProtocol, []string{
		"--session", "claude-resume", "--resume-id", "native-claude-id", "--workdir", "/tmp/resume",
	})
	if err == nil || !strings.Contains(err.Error(), "returned unexpectedly") {
		t.Fatalf("Claude resume Run error = %v", err)
	}
	if changedTo != "/tmp/resume" {
		t.Fatalf("Claude resume working directory = %q, want /tmp/resume", changedTo)
	}
	if want := []string{"claude", "--resume", "native-claude-id"}; !reflect.DeepEqual(gotArgv, want) {
		t.Fatalf("Claude resume argv = %q, want %q", gotArgv, want)
	}
	if got := environmentMap(gotEnv)["PWD"]; got != "/tmp/resume" {
		t.Fatalf("Claude resume PWD = %q, want /tmp/resume", got)
	}
}

func TestClaudeResolvesRelativePATHAfterEnteringWorkDir(t *testing.T) {
	workDir := t.TempDir()
	binDir := filepath.Join(workDir, "bin")
	if err := os.Mkdir(binDir, 0o700); err != nil {
		t.Fatalf("create project-local bin: %v", err)
	}
	claudePath := filepath.Join(binDir, "claude")
	if err := os.WriteFile(claudePath, []byte("test executable"), 0o700); err != nil {
		t.Fatalf("write project-local Claude: %v", err)
	}
	t.Setenv("PATH", "bin")
	t.Setenv("PWD", "/stale/pane/work")

	originalLookPathInEnvironment := lookPathInEnvironment
	originalReplaceProcess := replaceProcess
	originalChangeDirectory := changeDirectory
	originalResolveClaudeOAuth := resolveClaudeOAuth
	t.Cleanup(func() {
		lookPathInEnvironment = originalLookPathInEnvironment
		replaceProcess = originalReplaceProcess
		changeDirectory = originalChangeDirectory
		resolveClaudeOAuth = originalResolveClaudeOAuth
	})
	entered := false
	changeDirectory = func(path string) error {
		if path != workDir {
			t.Fatalf("Claude working directory = %q, want %q", path, workDir)
		}
		entered = true
		return nil
	}
	lookPathInEnvironment = func(name string, environment []string) (string, error) {
		if !entered {
			t.Fatal("Claude executable resolved before entering the target workdir")
		}
		return resolveExecutableInEnvironment(name, environment)
	}
	resolveClaudeOAuth = func() string { return "" }
	var gotPath string
	replaceProcess = func(path string, _ []string, _ []string) error {
		gotPath = path
		return nil
	}

	err := Run(ClaudeProtocol, []string{
		"--session", "relative-path-claude", "--workdir", workDir,
	})
	if err == nil || !strings.Contains(err.Error(), "returned unexpectedly") {
		t.Fatalf("Claude Run error = %v", err)
	}
	if gotPath != claudePath {
		t.Fatalf("Claude executable = %q, want project-local %q", gotPath, claudePath)
	}
}

func TestEnvironmentContracts(t *testing.T) {
	parent := []string{
		"PATH=/usr/bin:/bin", "HOME=/tmp/home", "OPENAI_API_KEY=openai-allowed",
		"CODEX_ACCESS_TOKEN=codex-allowed", "GITHUB_TOKEN=github-rejected",
		"CLAUDE_CODE_OAUTH_TOKEN=old-claude", "ANTHROPIC_API_KEY=old-anthropic",
		"CLAUDECODE=nested", "OTEL_EXPORTER_OTLP_ENDPOINT=collector:4317",
		"OTEL_EXPORTER_OTLP_HEADERS=authorization=old", "OTEL_TRACES_EXPORTER=old", "ENGRAM_SESSION_ID=old-session",
		"ARBITRARY_VALUE=preserved-for-claude",
	}
	codex := environmentMap(CodexEnvironment(parent, "codex-session"))
	for name, want := range map[string]string{
		"PATH": "/usr/bin:/bin", "HOME": "/tmp/home", "OPENAI_API_KEY": "openai-allowed",
		"CODEX_ACCESS_TOKEN": "codex-allowed", "AGM_SESSION_NAME": "codex-session",
	} {
		if got := codex[name]; got != want {
			t.Errorf("Codex environment %s = %q, want %q", name, got, want)
		}
	}
	for _, rejected := range []string{
		"GITHUB_TOKEN", "CLAUDE_CODE_OAUTH_TOKEN", "ANTHROPIC_API_KEY",
		"OTEL_EXPORTER_OTLP_ENDPOINT", "ENGRAM_SESSION_ID", "ARBITRARY_VALUE",
	} {
		if _, ok := codex[rejected]; ok {
			t.Errorf("Codex environment retained %s", rejected)
		}
	}

	claude := environmentMap(ClaudeEnvironment(parent, ClaudeLaunch{
		SessionName: "claude-session", SessionID: "new-session", ForwardTelemetry: true,
	}, "fresh-oauth"))
	for name, want := range map[string]string{
		"CLAUDE_CODE_OAUTH_TOKEN": "fresh-oauth", "AGM_SESSION_NAME": "claude-session",
		"ENGRAM_SESSION_ID": "new-session", "OTEL_EXPORTER_OTLP_ENDPOINT": "collector:4317",
		"OTEL_EXPORTER_OTLP_HEADERS": "authorization=old",
		"OTEL_TRACES_EXPORTER":       "otlp", "OTEL_EXPORTER_OTLP_PROTOCOL": "grpc",
		"CLAUDE_CODE_ENABLE_TELEMETRY": "1", "CLAUDE_CODE_ENHANCED_TELEMETRY_BETA": "1",
		"ARBITRARY_VALUE": "preserved-for-claude",
	} {
		if got := claude[name]; got != want {
			t.Errorf("Claude environment %s = %q, want %q", name, got, want)
		}
	}
	for _, rejected := range []string{"ANTHROPIC_API_KEY", "CLAUDECODE"} {
		if _, ok := claude[rejected]; ok {
			t.Errorf("Claude OAuth environment retained %s", rejected)
		}
	}

	disabled := environmentMap(ClaudeEnvironment(parent, ClaudeLaunch{
		SessionName: "claude-session", DisableOAuth: true,
	}, ""))
	if _, ok := disabled["CLAUDE_CODE_OAUTH_TOKEN"]; ok {
		t.Fatal("disabled Claude OAuth environment retained the OAuth token")
	}
	if got := disabled["ANTHROPIC_API_KEY"]; got != "old-anthropic" {
		t.Fatalf("disabled Claude OAuth environment lost API key: %q", got)
	}
	for _, name := range []string{"OTEL_EXPORTER_OTLP_ENDPOINT", "OTEL_EXPORTER_OTLP_HEADERS"} {
		if _, ok := disabled[name]; ok {
			t.Fatalf("disabled Claude telemetry retained %s", name)
		}
	}
}

func TestRunUsesFixedExecutablesAndDirectReplacement(t *testing.T) {
	originalLookPathInEnvironment := lookPathInEnvironment
	originalReplaceProcess := replaceProcess
	originalResolveClaudeOAuth := resolveClaudeOAuth
	t.Cleanup(func() {
		lookPathInEnvironment = originalLookPathInEnvironment
		replaceProcess = originalReplaceProcess
		resolveClaudeOAuth = originalResolveClaudeOAuth
	})

	var gotPath string
	var gotArgv, gotEnv []string
	lookPathInEnvironment = func(name string, _ []string) (string, error) { return "/fixed/" + name, nil }
	replaceProcess = func(path string, argv, env []string) error {
		gotPath = path
		gotArgv = append([]string(nil), argv...)
		gotEnv = append([]string(nil), env...)
		return nil
	}

	err := Run(CodexProtocol, []string{
		"--session", "codex-session", "--model", "gpt-test", "--workdir", "/tmp/work",
		"--sandbox", "workspace-write",
	})
	if err == nil || !strings.Contains(err.Error(), "returned unexpectedly") {
		t.Fatalf("Codex Run error = %v, want unexpected-return guard", err)
	}
	if gotPath != "/fixed/codex" || len(gotArgv) == 0 || gotArgv[0] != "codex" {
		t.Fatalf("Codex replacement = path %q argv %q", gotPath, gotArgv)
	}
	if got := environmentMap(gotEnv)["AGM_SESSION_NAME"]; got != "codex-session" {
		t.Fatalf("Codex replacement session environment = %q", got)
	}

	resolveClaudeOAuth = func() string { return "resolved-oauth" }
	err = Run(ClaudeProtocol, []string{
		"--session", "claude-session", "--model", "claude-test", "--auto-mode",
	})
	if err == nil || !strings.Contains(err.Error(), "returned unexpectedly") {
		t.Fatalf("Claude Run error = %v, want unexpected-return guard", err)
	}
	if gotPath != "/fixed/claude" || len(gotArgv) == 0 || gotArgv[0] != "claude" {
		t.Fatalf("Claude replacement = path %q argv %q", gotPath, gotArgv)
	}
	if got := environmentMap(gotEnv)["CLAUDE_CODE_OAUTH_TOKEN"]; got != "resolved-oauth" {
		t.Fatalf("Claude replacement OAuth = %q", got)
	}

	lookPathInEnvironment = func(string, []string) (string, error) { return "", errors.New("not found") }
	if err := Run(CodexProtocol, []string{
		"--session", "session", "--model", "model", "--workdir", "/tmp/work",
		"--sandbox", "workspace-write",
	}); err == nil || !strings.Contains(err.Error(), "resolve codex executable") {
		t.Fatalf("missing executable error = %v", err)
	}
	if err := Run("new", nil); err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("unsupported protocol error = %v", err)
	}
}

func TestExecutorRejectsUnvalidatedArguments(t *testing.T) {
	tests := []struct {
		name     string
		protocol string
		args     []string
	}{
		{name: "unknown flag", protocol: CodexProtocol, args: []string{"--unknown", "value"}},
		{name: "positionals", protocol: CodexProtocol, args: []string{"--session", "s", "extra"}},
		{name: "newline control character", protocol: CodexProtocol, args: []string{"--session", "s\nnext", "--model", "m", "--workdir", "/tmp", "--sandbox", "workspace-write"}},
		{name: "tab control character", protocol: CodexProtocol, args: []string{"--session", "s\tnext", "--model", "m", "--workdir", "/tmp", "--sandbox", "workspace-write"}},
		{name: "escape control character", protocol: ClaudeProtocol, args: []string{"--session", "s\x1bnext", "--model", "m"}},
		{name: "sandbox", protocol: CodexProtocol, args: []string{"--session", "s", "--model", "m", "--workdir", "/tmp", "--sandbox", "unsafe"}},
		{name: "permission", protocol: ClaudeProtocol, args: []string{"--session", "s", "--model", "m", "--permission", "unsafe"}},
		{name: "resume control character", protocol: ClaudeProtocol, args: []string{"--session", "s", "--resume-id", "id\nnext"}},
		{name: "non-finite budget", protocol: ClaudeProtocol, args: []string{"--session", "s", "--model", "m", "--max-budget-usd", "NaN"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var err error
			switch test.protocol {
			case CodexProtocol:
				_, err = parseCodex(test.args)
			case ClaudeProtocol:
				_, err = parseClaude(test.args)
			}
			if err == nil {
				t.Fatal("unvalidated executor request was accepted")
			}
		})
	}
}

func runExecutorCanary(t *testing.T, executable, protocol string, args, extraEnv []string) (string, string) {
	t.Helper()
	dir := t.TempDir()
	binDir := filepath.Join(dir, "bin")
	if err := os.Mkdir(binDir, 0700); err != nil {
		t.Fatalf("create fake binary directory: %v", err)
	}
	envPath := filepath.Join(dir, executable+"-env.txt")
	argvPath := filepath.Join(dir, executable+"-argv.txt")
	script := fmt.Sprintf("#!/bin/sh\nenv | sort > %s\nprintf '%%s\\n' \"$@\" > %s\n", launchparity.ShellQuote(envPath), launchparity.ShellQuote(argvPath))
	if err := os.WriteFile(filepath.Join(binDir, executable), []byte(script), 0700); err != nil {
		t.Fatalf("write fake %s: %v", executable, err)
	}

	helperArgs := append([]string{"-test.run=^TestPrivateExecutorHelper$", "--", protocol}, args...)
	command := exec.Command(os.Args[0], helperArgs...)
	command.Env = append([]string{
		helperMarker + "=1",
		"PATH=" + binDir + ":/usr/bin:/bin",
		"HOME=" + dir,
		"TERM=xterm-256color",
		"LANG=C",
	}, extraEnv...)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("run %s executor canary: %v\n%s", executable, err, output)
	}
	envBytes, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatalf("read %s environment capture: %v", executable, err)
	}
	argvBytes, err := os.ReadFile(argvPath)
	if err != nil {
		t.Fatalf("read %s argv capture: %v", executable, err)
	}
	return string(envBytes), string(argvBytes)
}

func outputLines(output string) []string {
	trimmed := strings.TrimSuffix(output, "\n")
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, "\n")
}
