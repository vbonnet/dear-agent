package harnessexec

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/shellquote"
	"github.com/vbonnet/dear-agent/agm/internal/tmux"
)

func TestMain(m *testing.M) {
	original := scheduleHandoffExpiry
	scheduleHandoffExpiry = func(string, string, time.Time, bool) (io.Closer, error) { return nil, nil }
	code := m.Run()
	scheduleHandoffExpiry = original
	os.Exit(code)
}

func TestResolveSubmissionPreservesUncertainAndCancelsConfirmedFailure(t *testing.T) {
	for _, tc := range []struct {
		name          string
		submissionErr error
		wantUncertain bool
		wantErr       bool
		wantCancelled bool
	}{
		{
			name:          "uncertain acknowledgement preserves handoff",
			submissionErr: tmux.MarkPromptSubmissionUncertain(errors.New("lost acknowledgement")),
			wantUncertain: true,
		},
		{
			name:          "confirmed failure cancels handoff",
			submissionErr: errors.New("send rejected"),
			wantErr:       true,
			wantCancelled: true,
		},
		{name: "confirmed success"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cancelled := false
			uncertain, err := ResolveSubmission(tc.submissionErr, func() error {
				cancelled = true
				return nil
			})
			if uncertain != tc.wantUncertain {
				t.Fatalf("uncertain = %v, want %v", uncertain, tc.wantUncertain)
			}
			if (err != nil) != tc.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tc.wantErr)
			}
			if cancelled != tc.wantCancelled {
				t.Fatalf("cancelled = %v, want %v", cancelled, tc.wantCancelled)
			}
		})
	}
}

func TestPreparedClaudeCommandCarriesCallerOnlyOAuthAndTelemetry(t *testing.T) {
	stateDir := t.TempDir()
	t.Setenv("AGM_STATE_DIR", stateDir)
	t.Setenv("ANTHROPIC_API_KEY", "stale-pane-api-key")
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "stale-pane-endpoint")

	originalExecutablePath := executablePath
	originalLookPathInEnvironment := lookPathInEnvironment
	originalReplaceProcess := replaceProcess
	originalResolveClaudeOAuth := resolveClaudeOAuth
	t.Cleanup(func() {
		executablePath = originalExecutablePath
		lookPathInEnvironment = originalLookPathInEnvironment
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
	lookPathInEnvironment = func(string, []string) (string, error) { return "/fixed/claude", nil }
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

func TestPreparedCodexCommandCarriesCallerAllowlistAndPreservesPaneRuntime(t *testing.T) {
	t.Setenv("AGM_STATE_DIR", t.TempDir())
	t.Setenv("OPENAI_API_KEY", "stale-pane-openai")
	t.Setenv("CODEX_ACCESS_TOKEN", "stale-pane-codex")
	t.Setenv("TMUX", "live-pane-tmux")
	t.Setenv("TMUX_PANE", "%9")
	t.Setenv("TERM", "tmux-256color")
	t.Setenv("COLORTERM", "truecolor")
	t.Setenv("TERM_PROGRAM", "tmux")
	t.Setenv("TERM_PROGRAM_VERSION", "3.6a")

	originalExecutablePath := executablePath
	originalLookPathInEnvironment := lookPathInEnvironment
	originalReplaceProcess := replaceProcess
	t.Cleanup(func() {
		executablePath = originalExecutablePath
		lookPathInEnvironment = originalLookPathInEnvironment
		replaceProcess = originalReplaceProcess
	})
	executablePath = func() (string, error) { return "/opt/agm/bin/agm", nil }

	prepared, err := PrepareCodexCommand(CodexLaunch{
		SessionName: "handoff-codex", Model: "gpt-test", WorkDir: "/tmp/work", Sandbox: "workspace-write",
	}, []string{
		"PATH=/caller/bin", "HOME=/caller/home", "PWD=/stale/caller/work",
		"TMUX=stale-caller-tmux", "TMUX_PANE=%1",
		"TERM=dumb", "COLORTERM=stale-caller-color",
		"TERM_PROGRAM=CodexDesktop", "TERM_PROGRAM_VERSION=0.0",
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
	lookPathInEnvironment = func(string, []string) (string, error) { return "/fixed/codex", nil }
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
		"OPENAI_API_KEY":       "caller-openai",
		"CODEX_ACCESS_TOKEN":   "caller-codex",
		"TMUX":                 "live-pane-tmux",
		"TMUX_PANE":            "%9",
		"TERM":                 "tmux-256color",
		"COLORTERM":            "truecolor",
		"TERM_PROGRAM":         "tmux",
		"TERM_PROGRAM_VERSION": "3.6a",
		"PWD":                  "/tmp/work",
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

func TestPreparedCodexCommandResolvesExecutableFromCallerPATH(t *testing.T) {
	t.Setenv("AGM_STATE_DIR", t.TempDir())
	staleBin := t.TempDir()
	callerBin := t.TempDir()
	for _, path := range []string{
		filepath.Join(staleBin, "codex"),
		filepath.Join(callerBin, "codex"),
	} {
		if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0700); err != nil {
			t.Fatalf("write fake Codex executable: %v", err)
		}
	}
	t.Setenv("PATH", staleBin)

	originalExecutablePath := executablePath
	originalReplaceProcess := replaceProcess
	t.Cleanup(func() {
		executablePath = originalExecutablePath
		replaceProcess = originalReplaceProcess
	})
	executablePath = func() (string, error) { return "/opt/agm/bin/agm", nil }

	prepared, err := PrepareCodexCommand(CodexLaunch{
		SessionName: "caller-path", Model: "gpt-test", WorkDir: "/tmp/work", Sandbox: "workspace-write",
	}, []string{"PATH=" + callerBin, "HOME=/caller/home"})
	if err != nil {
		t.Fatalf("prepare Codex command: %v", err)
	}
	var gotPath string
	var gotEnvironment []string
	replaceProcess = func(path string, _ []string, environment []string) error {
		gotPath = path
		gotEnvironment = append([]string(nil), environment...)
		return nil
	}
	err = Run(CodexProtocol, []string{
		"--handoff", prepared.path,
		"--session", "caller-path", "--model", "gpt-test",
		"--workdir", "/tmp/work", "--sandbox", "workspace-write",
	})
	if err == nil || !strings.Contains(err.Error(), "returned unexpectedly") {
		t.Fatalf("run prepared Codex command: %v", err)
	}
	if want := filepath.Join(callerBin, "codex"); gotPath != want {
		t.Fatalf("Codex executable = %q, want caller PATH executable %q (stale pane PATH was %q)", gotPath, want, staleBin)
	}
	if got := environmentMap(gotEnvironment)["PATH"]; got != callerBin {
		t.Fatalf("Codex child PATH = %q, want caller PATH %q", got, callerBin)
	}
}

func TestPreparedClaudeCommandClearsCallerAbsentPaneState(t *testing.T) {
	t.Setenv("AGM_STATE_DIR", t.TempDir())
	t.Setenv("CLAUDE_CODE_OAUTH_TOKEN", "stale-pane-oauth")
	t.Setenv("ANTHROPIC_API_KEY", "stale-pane-api")
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "stale-pane-endpoint")
	t.Setenv("OTEL_EXPORTER_OTLP_HEADERS", "stale-pane-headers")

	originalExecutablePath := executablePath
	originalLookPathInEnvironment := lookPathInEnvironment
	originalReplaceProcess := replaceProcess
	originalResolveClaudeOAuth := resolveClaudeOAuth
	t.Cleanup(func() {
		executablePath = originalExecutablePath
		lookPathInEnvironment = originalLookPathInEnvironment
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
	lookPathInEnvironment = func(string, []string) (string, error) { return "/fixed/claude", nil }
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
	originalLookPathInEnvironment := lookPathInEnvironment
	originalReplaceProcess := replaceProcess
	t.Cleanup(func() {
		executablePath = originalExecutablePath
		lookPathInEnvironment = originalLookPathInEnvironment
		replaceProcess = originalReplaceProcess
	})
	executablePath = func() (string, error) { return "/opt/agm/bin/agm", nil }
	prepared, err := PrepareCodexCommand(CodexLaunch{
		SessionName: "clear-stale", Model: "gpt-test", WorkDir: "/tmp/work", Sandbox: "workspace-write",
	}, nil)
	if err != nil {
		t.Fatalf("prepare Codex command: %v", err)
	}
	lookPathInEnvironment = func(string, []string) (string, error) { return "/fixed/codex", nil }
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

func TestPrepareCommandsRejectTerminalControlsBeforeStagingHandoff(t *testing.T) {
	stateDir := filepath.Join(t.TempDir(), "must-not-exist")
	t.Setenv("AGM_STATE_DIR", stateDir)

	tests := []struct {
		name    string
		prepare func() (PreparedCommand, error)
	}{
		{
			name: "Codex newline in workdir",
			prepare: func() (PreparedCommand, error) {
				return PrepareCodexCommand(CodexLaunch{
					SessionName: "codex", Model: "gpt-test", WorkDir: "/tmp/safe\ninjected",
					Sandbox: "workspace-write",
				}, nil)
			},
		},
		{
			name: "Claude bracketed paste escape in add-dir",
			prepare: func() (PreparedCommand, error) {
				return PrepareClaudeCommand(ClaudeLaunch{
					SessionName: "claude", WorkDir: "/tmp/work",
					AddDirs: []string{"/tmp/safe\x1b[201~\n"},
				}, nil)
			},
		},
		{
			name: "Claude invalid UTF-8 resume ID",
			prepare: func() (PreparedCommand, error) {
				return PrepareClaudeCommand(ClaudeLaunch{
					SessionName: "claude", ResumeID: string([]byte{'i', 'd', 0xff}),
				}, nil)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prepared, err := tt.prepare()
			if err == nil {
				_ = prepared.Cancel()
				t.Fatal("Prepare command succeeded, want terminal-control rejection")
			}
			if prepared.Command != "" {
				t.Fatalf("prepared pane command %q before validation", prepared.Command)
			}
			if !strings.Contains(err.Error(), "invalid harness launch request") {
				t.Fatalf("Prepare error = %v, want launch validation", err)
			}
		})
	}

	if _, err := os.Stat(stateDir); !os.IsNotExist(err) {
		t.Fatalf("private handoff state created before validation: %v", err)
	}
}

func TestPrepareCodexRejectsGeneratedTerminalControlsBeforeBuildingCommand(t *testing.T) {
	originalExecutablePath := executablePath
	t.Cleanup(func() { executablePath = originalExecutablePath })
	launch := CodexLaunch{
		SessionName: "codex", Model: "gpt-test", WorkDir: "/tmp/work",
		Sandbox: "workspace-write",
	}

	t.Run("resolved executable", func(t *testing.T) {
		stateDir := filepath.Join(t.TempDir(), "must-not-exist")
		t.Setenv("AGM_STATE_DIR", stateDir)
		executablePath = func() (string, error) {
			return "/opt/agm/bin/agm\x1b[201~\x15/quit", nil
		}

		prepared, err := PrepareCodexCommand(launch, nil)
		if err == nil {
			_ = prepared.Cancel()
			t.Fatal("Prepare command accepted a generated executable with terminal controls")
		}
		if prepared.Command != "" || !strings.Contains(err.Error(), "private executable contains control characters") {
			t.Fatalf("Prepare result = %#v, %v; want generated executable validation", prepared, err)
		}
		if _, statErr := os.Stat(stateDir); !os.IsNotExist(statErr) {
			t.Fatalf("handoff state created before executable validation: %v", statErr)
		}
	})

	t.Run("resolved handoff root", func(t *testing.T) {
		stateDir := filepath.Join(t.TempDir(), "state\x1b[201~\x15/quit")
		t.Setenv("AGM_STATE_DIR", stateDir)
		executablePath = func() (string, error) { return "/opt/agm/bin/agm", nil }

		prepared, err := PrepareCodexCommand(launch, nil)
		if err == nil {
			_ = prepared.Cancel()
			t.Fatal("Prepare command accepted a generated handoff path with terminal controls")
		}
		if prepared.Command != "" || !strings.Contains(err.Error(), "private handoff directory contains control characters") {
			t.Fatalf("Prepare result = %#v, %v; want generated handoff validation", prepared, err)
		}
		if _, statErr := os.Stat(stateDir); !os.IsNotExist(statErr) {
			t.Fatalf("unsafe handoff root created before path validation: %v", statErr)
		}
	})
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

func TestPreparedCommandSchedulesIndependentExpiration(t *testing.T) {
	t.Setenv("AGM_STATE_DIR", t.TempDir())
	originalExecutablePath := executablePath
	originalScheduler := scheduleHandoffExpiry
	t.Cleanup(func() {
		executablePath = originalExecutablePath
		scheduleHandoffExpiry = originalScheduler
	})
	executablePath = func() (string, error) { return "/opt/agm/bin/agm", nil }
	var gotExecutable, gotPath string
	var gotDeadline time.Time
	var gotDeferred bool
	scheduleHandoffExpiry = func(executable, path string, deadline time.Time, deferred bool) (io.Closer, error) {
		gotExecutable, gotPath, gotDeadline = executable, path, deadline
		gotDeferred = deferred
		return nil, nil
	}

	startedAt := time.Now()
	prepared, err := PrepareCodexCommand(CodexLaunch{
		SessionName: "expiring-codex", Model: "gpt-test", WorkDir: "/tmp/work", Sandbox: "workspace-write",
	}, []string{"OPENAI_API_KEY=expiry-canary"})
	if err != nil {
		t.Fatalf("prepare Codex command: %v", err)
	}
	t.Cleanup(func() { _ = prepared.Cancel() })
	if gotExecutable != "/opt/agm/bin/agm" || gotPath != prepared.path {
		t.Fatalf("expiration scheduled for executable=%q path=%q, want current AGM and %q", gotExecutable, gotPath, prepared.path)
	}
	if gotDeadline.Before(startedAt.Add(handoffMaxAge)) || gotDeadline.After(time.Now().Add(handoffMaxAge)) {
		t.Fatalf("expiration deadline %s is not one bounded lifetime from preparation", gotDeadline)
	}
	if gotDeferred {
		t.Fatal("ordinary detached launch unexpectedly requested a producer liveness lease")
	}
}

func TestPreparedDeferredCommandSchedulesProducerLease(t *testing.T) {
	t.Setenv("AGM_STATE_DIR", t.TempDir())
	originalExecutablePath := executablePath
	originalScheduler := scheduleHandoffExpiry
	t.Cleanup(func() {
		executablePath = originalExecutablePath
		scheduleHandoffExpiry = originalScheduler
	})
	executablePath = func() (string, error) { return "/opt/agm/bin/agm", nil }
	lease := &recordingCloser{}
	var gotDeferred bool
	scheduleHandoffExpiry = func(_ string, _ string, _ time.Time, deferred bool) (io.Closer, error) {
		gotDeferred = deferred
		return lease, nil
	}

	prepared, err := PrepareCodexCommand(CodexLaunch{
		SessionName: "deferred-codex", Model: "gpt-test", WorkDir: "/tmp/work",
		Sandbox: "workspace-write", DeferUntilProducerExit: true,
	}, []string{"OPENAI_API_KEY=deferred-canary"})
	if err != nil {
		t.Fatalf("prepare deferred Codex command: %v", err)
	}
	if !gotDeferred {
		t.Fatal("deferred launch did not request a producer liveness lease")
	}
	payload, err := os.ReadFile(prepared.path)
	if err != nil {
		t.Fatalf("read deferred handoff: %v", err)
	}
	var handoff launchHandoff
	if err := json.Unmarshal(payload, &handoff); err != nil {
		t.Fatalf("decode deferred handoff: %v", err)
	}
	if !handoff.DeferredUntilProducerExit {
		t.Fatal("deferred handoff omitted its producer-liveness marker")
	}
	if err := prepared.Cancel(); err != nil {
		t.Fatalf("cancel deferred command: %v", err)
	}
	if !lease.closed {
		t.Fatal("cancelling a deferred command did not release its producer lease")
	}
}

func TestPreparedCommandRemovesHandoffWhenExpirationCannotBeScheduled(t *testing.T) {
	stateDir := t.TempDir()
	t.Setenv("AGM_STATE_DIR", stateDir)
	originalExecutablePath := executablePath
	originalScheduler := scheduleHandoffExpiry
	t.Cleanup(func() {
		executablePath = originalExecutablePath
		scheduleHandoffExpiry = originalScheduler
	})
	executablePath = func() (string, error) { return "/opt/agm/bin/agm", nil }
	scheduleHandoffExpiry = func(string, string, time.Time, bool) (io.Closer, error) {
		return nil, errors.New("scheduler unavailable")
	}

	_, err := PrepareClaudeCommand(ClaudeLaunch{SessionName: "expiry-failure"}, nil)
	if err == nil || !strings.Contains(err.Error(), "schedule private launch handoff expiration") {
		t.Fatalf("prepare error = %v, want expiration scheduling failure", err)
	}
	entries, readErr := os.ReadDir(filepath.Join(stateDir, "private-launch"))
	if readErr != nil {
		t.Fatalf("read private launch directory: %v", readErr)
	}
	if len(entries) != 0 {
		t.Fatalf("unscheduled private handoff remained on disk: %v", entries)
	}
}

func TestDeferredHandoffRemainsLiveUntilProducerExitThenExpires(t *testing.T) {
	t.Setenv("AGM_STATE_DIR", t.TempDir())
	path, err := stageHandoff(CodexProtocol, []string{"OPENAI_API_KEY=deferred-expiry-canary"}, true)
	if err != nil {
		t.Fatalf("stage deferred handoff: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(path) })
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("create producer lease: %v", err)
	}
	t.Cleanup(func() {
		_ = reader.Close()
		_ = writer.Close()
	})
	oldModTime := time.Now().Add(-time.Hour)
	if err := os.Chtimes(path, oldModTime, oldModTime); err != nil {
		t.Fatalf("age deferred handoff before lease refresh: %v", err)
	}
	done := make(chan error, 1)
	go func() {
		done <- expireDeferredHandoff(path, reader, 125*time.Millisecond, 25*time.Millisecond)
	}()

	initial, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat initial deferred handoff: %v", err)
	}
	time.Sleep(175 * time.Millisecond)
	live, err := os.Stat(path)
	if err != nil {
		t.Fatalf("deferred handoff expired while producer was live: %v", err)
	}
	if !live.ModTime().After(initial.ModTime()) {
		t.Fatalf("deferred handoff lease did not refresh mtime: initial=%s live=%s", initial.ModTime(), live.ModTime())
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("release producer lease: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("deferred handoff vanished without its bounded post-exit lifetime: %v", err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("expire deferred handoff: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("deferred handoff did not expire after producer exit")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("post-exit deferred handoff still exists: %v", err)
	}
}

func TestExpiryProtocolRemovesUnconsumedHandoffAtDeadline(t *testing.T) {
	t.Setenv("AGM_STATE_DIR", t.TempDir())
	path, err := stageHandoff(CodexProtocol, []string{"OPENAI_API_KEY=expiry-canary"}, false)
	if err != nil {
		t.Fatalf("stage handoff: %v", err)
	}
	err = Run(ExpiryProtocol, []string{
		"--handoff", path,
		"--expires-at", time.Now().Add(-time.Second).UTC().Format(time.RFC3339Nano),
	})
	if err != nil {
		t.Fatalf("run expiration protocol: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expired private handoff still exists: %v", err)
	}
}

func TestDetachedExpiryHelperIsReapedAsynchronously(t *testing.T) {
	t.Setenv("AGM_STATE_DIR", t.TempDir())
	path, err := stageHandoff(CodexProtocol, nil, false)
	if err != nil {
		t.Fatalf("stage handoff: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(path) })
	originalReaper := reapExpiryProcess
	t.Cleanup(func() { reapExpiryProcess = originalReaper })
	reaped := make(chan error, 1)
	reapExpiryProcess = func(cmd *exec.Cmd) {
		go func() { reaped <- cmd.Wait() }()
	}

	if _, err := startHandoffExpiry(os.Args[0], path, time.Now().Add(50*time.Millisecond), false); err != nil {
		t.Fatalf("start asynchronously reaped helper: %v", err)
	}
	select {
	case err := <-reaped:
		if err != nil {
			t.Fatalf("expiration helper exit: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("expiration helper was not asynchronously reaped")
	}
}

func TestDetachedExpiryHelperInterceptsGoTestBinaryBeforeTestsRun(t *testing.T) {
	t.Setenv("AGM_STATE_DIR", t.TempDir())
	path, err := stageHandoff(CodexProtocol, []string{"OPENAI_API_KEY=detached-expiry-canary"}, false)
	if err != nil {
		t.Fatalf("stage handoff: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(path) })

	if _, err := startHandoffExpiry(os.Args[0], path, time.Now().Add(250*time.Millisecond), false); err != nil {
		t.Fatalf("start detached expiration helper from Go test binary: %v", err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		_, statErr := os.Stat(path)
		if os.IsNotExist(statErr) {
			return
		}
		if statErr != nil {
			t.Fatalf("inspect detached handoff: %v", statErr)
		}
		if time.Now().After(deadline) {
			t.Fatal("detached expiration helper re-entered tests or failed to remove the handoff")
		}
		time.Sleep(25 * time.Millisecond)
	}
}

func TestConsumeHandoffUsesDeferredLeaseFreshnessAndUnlinksRejections(t *testing.T) {
	t.Setenv("AGM_STATE_DIR", t.TempDir())
	for name, deferred := range map[string]bool{
		"ordinary": false,
		"deferred": true,
	} {
		t.Run(name, func(t *testing.T) {
			path, err := stageHandoff(CodexProtocol, nil, deferred)
			if err != nil {
				t.Fatalf("stage handoff: %v", err)
			}
			payload, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read handoff: %v", err)
			}
			var handoff launchHandoff
			if err := json.Unmarshal(payload, &handoff); err != nil {
				t.Fatalf("decode handoff: %v", err)
			}
			handoff.CreatedAt = time.Now().Add(-2 * handoffMaxAge).UTC().Format(time.RFC3339Nano)
			payload, err = json.Marshal(handoff)
			if err != nil {
				t.Fatalf("encode handoff: %v", err)
			}
			if err := os.WriteFile(path, append(payload, '\n'), 0600); err != nil {
				t.Fatalf("rewrite handoff timestamp: %v", err)
			}

			_, consumeErr := consumeHandoff(path, CodexProtocol)
			if deferred && consumeErr != nil {
				t.Fatalf("deferred handoff rejected recent producer lease freshness: %v", consumeErr)
			}
			if !deferred && consumeErr == nil {
				t.Fatal("ordinary handoff accepted expired creation time")
			}
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Fatalf("consumed or rejected one-shot handoff still exists: %v", err)
			}
		})
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
	if !strings.HasPrefix(prepared.Command, shellquote.Quote(agmPath)+" "+CodexProtocol) {
		t.Fatalf("companion command did not pin co-installed AGM: %s", prepared.Command)
	}
}

func TestPreparedCommandUsesMatchingVersionedAGMFromReleaseCompanion(t *testing.T) {
	t.Setenv("AGM_STATE_DIR", t.TempDir())
	binDir := t.TempDir()
	suffix := "-" + runtime.GOOS + "-" + runtime.GOARCH
	agmPath := filepath.Join(binDir, "agm"+suffix)
	if err := os.WriteFile(agmPath, []byte("test executable"), 0700); err != nil {
		t.Fatalf("write versioned co-installed AGM: %v", err)
	}
	originalExecutablePath := executablePath
	t.Cleanup(func() { executablePath = originalExecutablePath })
	executablePath = func() (string, error) {
		return filepath.Join(binDir, "agm-mcp-server"+suffix), nil
	}

	prepared, err := PrepareClaudeCommand(ClaudeLaunch{
		SessionName: "release-companion-claude", WorkDir: "/tmp/work",
	}, nil)
	if err != nil {
		t.Fatalf("prepare Claude command from versioned release companion: %v", err)
	}
	t.Cleanup(func() { _ = prepared.Cancel() })
	if !strings.HasPrefix(prepared.Command, shellquote.Quote(agmPath)+" "+ClaudeProtocol) {
		t.Fatalf("versioned companion command did not pin matching AGM artifact: %s", prepared.Command)
	}
}

func TestPreparedCommandRejectsCompanionWithoutCoInstalledAGM(t *testing.T) {
	t.Setenv("AGM_STATE_DIR", t.TempDir())
	binDir := t.TempDir()
	pathDir := t.TempDir()
	pathAGM := filepath.Join(pathDir, "agm")
	if err := os.WriteFile(pathAGM, []byte("stale executable"), 0700); err != nil {
		t.Fatalf("write PATH AGM: %v", err)
	}
	t.Setenv("PATH", pathDir)
	originalExecutablePath := executablePath
	t.Cleanup(func() { executablePath = originalExecutablePath })
	executablePath = func() (string, error) { return filepath.Join(binDir, "agm-mcp-server"), nil }

	_, err := PrepareCodexCommand(CodexLaunch{
		SessionName: "missing-companion-agm", Model: "gpt-test", WorkDir: "/tmp/work", Sandbox: "workspace-write",
	}, nil)
	if err == nil {
		t.Fatal("companion without a co-installed AGM used an unverified PATH fallback")
	}
	if !strings.Contains(err.Error(), "find co-installed AGM private executor") {
		t.Fatalf("unexpected companion resolution error: %v", err)
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
	if !strings.Contains(prepared.Command, "--handoff "+shellquote.Quote(prepared.path)) {
		t.Fatalf("prepared command omitted absolute handoff path: %s", prepared.Command)
	}
}

func TestExecutorConsumesHandoffBeforeHarnessLookup(t *testing.T) {
	t.Setenv("AGM_STATE_DIR", t.TempDir())
	originalExecutablePath := executablePath
	originalLookPathInEnvironment := lookPathInEnvironment
	t.Cleanup(func() {
		executablePath = originalExecutablePath
		lookPathInEnvironment = originalLookPathInEnvironment
	})
	executablePath = func() (string, error) { return "/opt/agm/bin/agm", nil }
	lookPathInEnvironment = func(string, []string) (string, error) { return "", os.ErrNotExist }

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

	if _, err := stageHandoff(CodexProtocol, []string{"ANTHROPIC_API_KEY=must-not-cross"}, false); err == nil {
		t.Fatal("Codex staging accepted an Anthropic credential")
	}

	wrongProtocol, err := stageHandoff(ClaudeProtocol, nil, false)
	if err != nil {
		t.Fatalf("stage wrong-protocol handoff: %v", err)
	}
	if _, err := consumeHandoff(wrongProtocol, CodexProtocol); err == nil {
		t.Fatal("Codex executor accepted a Claude handoff")
	}
	if _, err := os.Stat(wrongProtocol); !os.IsNotExist(err) {
		t.Fatalf("rejected wrong-protocol handoff still exists: %v", err)
	}

	public, err := stageHandoff(CodexProtocol, nil, false)
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

func TestConsumeHandoffPreservesFilesOutsidePrivateStagingNamespace(t *testing.T) {
	stateDir := t.TempDir()
	t.Setenv("AGM_STATE_DIR", stateDir)
	stagingDir := filepath.Join(stateDir, "private-launch")
	if err := os.Mkdir(stagingDir, 0700); err != nil {
		t.Fatalf("create staging directory: %v", err)
	}
	tests := map[string]string{
		"unrelated owner-only file":        filepath.Join(t.TempDir(), "id_ed25519"),
		"staging-shaped file outside root": filepath.Join(t.TempDir(), "launch-forged.json"),
		"invalid name inside staging root": filepath.Join(stagingDir, "config.json"),
	}
	for name, path := range tests {
		t.Run(name, func(t *testing.T) {
			const content = "owner-only content that must survive"
			if err := os.WriteFile(path, []byte(content), 0600); err != nil {
				t.Fatalf("write protected test file: %v", err)
			}
			if _, err := consumeHandoff(path, CodexProtocol); err == nil {
				t.Fatal("executor accepted a file outside the private staging namespace")
			}
			got, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("rejected file was removed: %v", err)
			}
			if string(got) != content {
				t.Fatalf("rejected file content changed: %q", got)
			}
		})
	}
}

func TestConsumeHandoffRejectsTrailingAndOversizedContent(t *testing.T) {
	t.Setenv("AGM_STATE_DIR", t.TempDir())

	for name, suffix := range map[string]string{
		"trailing JSON": "{}",
		"oversized":     strings.Repeat(" ", handoffMaxSize),
	} {
		t.Run(name, func(t *testing.T) {
			path, err := stageHandoff(CodexProtocol, nil, false)
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
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Fatalf("rejected corrupted handoff still exists: %v", err)
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

type recordingCloser struct {
	closed bool
}

func (c *recordingCloser) Close() error {
	c.closed = true
	return nil
}
